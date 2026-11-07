package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPulseOperationsProjectionKeepsOnlyRewardRecoveryCounts(t *testing.T) {
	body, err := projectPulseOperationsOverview([]byte(`{"observed_at":"2026-11-08T00:00:00Z","cursors":[{"name":"newapi"}],"periods":[{"usage_event_count":99}],"health":{"ingest_blocked":false},"snapshot":{"ingest_lag_seconds":200,"open_conflict_count":3,"settlement_retry_count":2,"settlement_dead_count":1}}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"observed_at":"2026-11-08T00:00:00Z","snapshot":{"settlement_retry_count":2,"settlement_dead_count":1}}`, string(body))
}

// The admin secret is what the browser must never hold. Verify the proxy signs
// with the admin role and the admin key, and forwards no browser credential.
func TestPulseOpsSignsWithAdminRoleAndAdminSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "pulse-admin-secret-at-least-32-chars"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, pulseOpsOverviewPath, r.URL.EscapedPath())
		require.Equal(t, pulseAdminRole, r.Header.Get(pulseHeaderRole))
		require.Equal(t, "7", r.Header.Get(pulseHeaderUserID))
		require.Empty(t, r.Header.Get("Cookie"))
		timestamp, err := strconv.ParseInt(r.Header.Get(pulseHeaderTimestamp), 10, 64)
		require.NoError(t, err)
		canonical := pulseBFFCanonicalPayload(r.Method, r.URL.EscapedPath(), "7", pulseAdminRole, timestamp, r.Header.Get(pulseHeaderNonce), nil)
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		require.Equal(t, hex.EncodeToString(mac.Sum(nil)), r.Header.Get(pulseHeaderSignature))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"observed_at":"2026-11-08T00:00:00Z","periods":[]}`))
	}))
	defer upstream.Close()
	t.Setenv("PULSE_INTERNAL_URL", upstream.URL)
	t.Setenv("PULSE_ADMIN_HMAC_SECRET", secret)

	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/pulse/ops/overview", nil)
	c.Set("id", 7)
	c.Set("role", common.RoleAdminUser)
	GetPulseOperationsOverview(c)

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"observed_at":"2026-11-08T00:00:00Z"}`, response.Body.String())
}

// The user-facing BFF secret must not grant operations access, so the two
// configurations resolve independently.
func TestPulseOpsRequiresItsOwnSecret(t *testing.T) {
	t.Setenv("PULSE_INTERNAL_URL", "http://pulse.internal")
	t.Setenv("PULSE_USER_BFF_HMAC_SECRET", "user-secret")
	t.Setenv("PULSE_ADMIN_HMAC_SECRET", "")

	_, _, err := pulseOpsConfig()
	require.Error(t, err)
	_, bffSecret, err := pulseBFFConfig()
	require.NoError(t, err)
	require.Equal(t, "user-secret", bffSecret)
}

func TestPulseOpsRejectsNonAdminAndQueryParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream must not be reached")
	}))
	defer upstream.Close()
	t.Setenv("PULSE_INTERNAL_URL", upstream.URL)
	t.Setenv("PULSE_ADMIN_HMAC_SECRET", "pulse-admin-secret-at-least-32-chars")

	cases := []struct {
		name   string
		role   int
		target string
		want   int
	}{
		{"普通用户", common.RoleCommonUser, "/api/pulse/ops/overview", http.StatusForbidden},
		{"未设置角色", 0, "/api/pulse/ops/overview", http.StatusForbidden},
		{"带查询参数", common.RoleAdminUser, "/api/pulse/ops/overview?period_id=1", http.StatusBadRequest},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(http.MethodGet, testCase.target, nil)
			c.Set("id", 7)
			if testCase.role != 0 {
				c.Set("role", testCase.role)
			}
			GetPulseOperationsOverview(c)
			require.Equal(t, testCase.want, response.Code)
		})
	}
}

// A Pulse outage must degrade to 503 rather than surface upstream internals.
func TestPulseOpsDegradesWhenUpstreamFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "pulse_period table is missing", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	t.Setenv("PULSE_INTERNAL_URL", upstream.URL)
	t.Setenv("PULSE_ADMIN_HMAC_SECRET", "pulse-admin-secret-at-least-32-chars")

	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/pulse/ops/overview", nil)
	c.Set("id", 7)
	c.Set("role", common.RoleAdminUser)
	GetPulseOperationsOverview(c)

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.NotContains(t, response.Body.String(), "pulse_period")
}

// An operator who saves the secret in the admin console expects it to take
// effect without touching the deployment environment, so the stored option wins.
// The environment stays as the fallback for instances configured before the
// options existed.
func TestPulseInternalConfigPrefersConsoleOptionOverEnvironment(t *testing.T) {
	const stored = "console-managed-secret-at-least-32-chars"
	t.Setenv("PULSE_INTERNAL_URL", "http://env.pulse.internal")
	t.Setenv("PULSE_ADMIN_HMAC_SECRET", "environment-secret-at-least-32-chars")
	t.Cleanup(func() {
		common.ReplacePulseConfig(nil)
	})

	common.ReplacePulseConfig(map[string]string{"PulseInternalURL": "http://console.pulse.internal", "PulseAdminHMACSecret": stored})
	baseURL, secret, err := pulseOpsConfig()
	require.NoError(t, err)
	require.Equal(t, "http://console.pulse.internal", baseURL.String())
	require.Equal(t, stored, secret)

	// No persisted override means environment fallback.
	common.ReplacePulseConfig(nil)
	baseURL, secret, err = pulseOpsConfig()
	require.NoError(t, err)
	require.Equal(t, "http://env.pulse.internal", baseURL.String())
	require.Equal(t, "environment-secret-at-least-32-chars", secret)
}

func TestGeneratePulseSecretReturnsUsableUniqueSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	call := func(role int) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/pulse/ops/secret/generate", nil)
		c.Set("id", 7)
		c.Set("role", role)
		GeneratePulseSecret(c)
		return response
	}

	require.Equal(t, http.StatusForbidden, call(common.RoleCommonUser).Code)

	seen := make(map[string]bool, 8)
	for i := 0; i < 8; i++ {
		response := call(common.RoleAdminUser)
		require.Equal(t, http.StatusOK, response.Code)
		var payload struct {
			Success bool   `json:"success"`
			Data    string `json:"data"`
		}
		require.NoError(t, common.UnmarshalJsonStr(response.Body.String(), &payload))
		require.True(t, payload.Success)
		// The generated value has to pass the same gate the signing path uses,
		// otherwise the console would hand out secrets Pulse refuses.
		require.True(t, middlewarePulseSecretUsable(payload.Data))
		require.False(t, seen[payload.Data], "生成的密钥必须唯一")
		seen[payload.Data] = true
	}
}
