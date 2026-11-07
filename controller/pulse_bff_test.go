package controller

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPulseBFFRequiresNewAPIAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("test", cookie.NewStore([]byte("cookie-secret"))))
	router.GET("/api/pulse/summary", middleware.UserAuth(), GetPulseSummary)

	request := httptest.NewRequest(http.MethodGet, "/api/pulse/summary", nil)
	request.Header.Set("New-Api-User", "42")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestPulseBFFSignsDerivedIdentityWithoutForwardingBrowserCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "pulse-bff-secret"
	var mu sync.Mutex
	nonces := make([]string, 0, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, pulseBFFRewardsPath, r.URL.EscapedPath())
		require.Equal(t, "20", r.URL.Query().Get("limit"))
		require.Equal(t, "42", r.Header.Get(pulseHeaderUserID))
		require.Equal(t, pulseBFFRole, r.Header.Get(pulseHeaderRole))
		require.Empty(t, r.Header.Get("Cookie"))
		require.Empty(t, r.Header.Get("Authorization"))
		timestamp, err := strconv.ParseInt(r.Header.Get(pulseHeaderTimestamp), 10, 64)
		require.NoError(t, err)
		nonce := r.Header.Get(pulseHeaderNonce)
		require.NotEmpty(t, nonce)
		canonical := pulseBFFCanonicalPayload(r.Method, r.URL.EscapedPath(), "42", pulseBFFRole, timestamp, nonce, nil)
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		require.Equal(t, hex.EncodeToString(mac.Sum(nil)), r.Header.Get(pulseHeaderSignature))
		mu.Lock()
		nonces = append(nonces, nonce)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rewards":[]}`))
	}))
	defer upstream.Close()
	t.Setenv("PULSE_INTERNAL_URL", upstream.URL)
	t.Setenv("PULSE_USER_BFF_HMAC_SECRET", secret)

	router := gin.New()
	router.GET("/api/pulse/rewards", func(c *gin.Context) {
		c.Set("id", 42)
		GetPulseRewards(c)
	})
	for range 2 {
		request := httptest.NewRequest(http.MethodGet, "/api/pulse/rewards?limit=20", nil)
		request.Header.Set(pulseHeaderUserID, "999")
		request.Header.Set("Authorization", "browser-secret")
		request.AddCookie(&http.Cookie{Name: "session", Value: "browser-cookie"})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.JSONEq(t, `{"rewards":[]}`, response.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, nonces, 2)
	require.NotEqual(t, nonces[0], nonces[1])
}

func TestPulseBFFQueryAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 42); c.Next() })
	router.GET("/api/pulse/summary", GetPulseSummary)
	router.GET("/api/pulse/rewards", GetPulseRewards)

	for _, target := range []string{
		"/api/pulse/summary?user_id=999",
		"/api/pulse/rewards?user_id=999",
		"/api/pulse/rewards?limit=0",
		"/api/pulse/rewards?limit=1&limit=2",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		require.Equal(t, http.StatusBadRequest, response.Code, target)
	}
}

func TestPulseSummaryProjectionKeepsTicketsAndDropsRetiredEntitlements(t *testing.T) {
	body, err := projectPulseSummary([]byte(`{"available_tickets":4,"lifetime_contribution_milli":900,"level":{"key":"gold"},"current_period":{"id":1},"ledger":[{"source_ref":"private"}]}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"available_tickets":4}`, string(body))
}

func TestPulseBFFUpstreamFailuresDegradeToServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("PULSE_INTERNAL_URL", "http://pulse.example.test")
	t.Setenv("PULSE_USER_BFF_HMAC_SECRET", "pulse-bff-secret")

	for _, test := range []struct {
		name string
		doer pulseHTTPDoer
	}{
		{name: "timeout", doer: failingPulseDoer{}},
		{name: "upstream 5xx", doer: staticPulseDoer{status: http.StatusInternalServerError, body: []byte(`{"error":"internal"}`)}},
		{name: "invalid json", doer: staticPulseDoer{status: http.StatusOK, body: []byte(`not-json`)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/proxy", func(c *gin.Context) {
				c.Set("id", 42)
				proxyPulseRead(c, pulseBFFSummaryPath, nil, test.doer)
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/proxy", nil))
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
		})
	}
}

type failingPulseDoer struct{}

func (failingPulseDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("upstream timeout")
}

type staticPulseDoer struct {
	status int
	body   []byte
}

func (d staticPulseDoer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: d.status, Body: io.NopCloser(bytes.NewReader(d.body))}, nil
}

func TestPulseBFFRejectsInvalidConfigAndOversizedResponse(t *testing.T) {
	t.Setenv("PULSE_INTERNAL_URL", "https://user:pass@pulse.example.test?bad=1")
	t.Setenv("PULSE_USER_BFF_HMAC_SECRET", "pulse-bff-secret")
	_, _, err := pulseBFFConfig()
	require.Error(t, err)

	gin.SetMode(gin.TestMode)
	t.Setenv("PULSE_INTERNAL_URL", "http://pulse.example.test")
	router := gin.New()
	router.GET("/proxy", func(c *gin.Context) {
		c.Set("id", 42)
		proxyPulseRead(c, pulseBFFSummaryPath, nil, oversizedPulseDoer{})
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/proxy", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
}

type oversizedPulseDoer struct{}

func (oversizedPulseDoer) Do(*http.Request) (*http.Response, error) {
	body := make([]byte, pulseBFFMaxResponseBytes+1)
	for i := range body {
		body[i] = 'x'
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
}
