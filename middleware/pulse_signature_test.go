package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func signedPulseRequest(t *testing.T, body string, timestamp time.Time, nonce string) *http.Request {
	t.Helper()
	const secret = "pulse-test-secret"
	userID := "42"
	role := "pulse-settlement"
	req := httptest.NewRequest(http.MethodPost, "http://example.test/api/internal/pulse/benefits/grant", bytes.NewBufferString(body))
	req.Header.Set(pulseUserHeader, userID)
	req.Header.Set(pulseRoleHeader, role)
	req.Header.Set(pulseTimestampHeader, strconv.FormatInt(timestamp.Unix(), 10))
	req.Header.Set(pulseNonceHeader, nonce)
	canonical := pulseCanonicalPayload(req.Method, req.URL.EscapedPath(), userID, role, timestamp.Unix(), nonce, []byte(body))
	mac := hmac.New(sha256.New, []byte(secret))
	_, err := mac.Write([]byte(canonical))
	require.NoError(t, err)
	req.Header.Set(pulseSignatureHeader, hex.EncodeToString(mac.Sum(nil)))
	return req
}

func TestVerifyPulseServiceRequestAcceptsValidRequestAndRejectsReplay(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	t.Setenv("PULSE_ENV", "test")

	now := time.Now().Truncate(time.Second)
	req := signedPulseRequest(t, `{"source_ref":"grant-1"}`, now, "nonce-valid-1")
	require.True(t, verifyPulseServiceRequest(req, "pulse-test-secret"))
	require.False(t, verifyPulseServiceRequest(req, "pulse-test-secret"), "nonce must be single-use")
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, `{"source_ref":"grant-1"}`, string(body))
}

func TestVerifyPulseServiceRequestRejectsMissingSecretAndTampering(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	t.Setenv("PULSE_ENV", "test")
	now := time.Now().Truncate(time.Second)

	require.False(t, verifyPulseServiceRequest(signedPulseRequest(t, "body", now, "nonce-missing-secret"), ""))
	tampered := signedPulseRequest(t, "body", now, "nonce-tampered")
	tampered.Body = io.NopCloser(bytes.NewBufferString("changed"))
	require.False(t, verifyPulseServiceRequest(tampered, "pulse-test-secret"))
}

func TestVerifyPulseServiceRequestRejectsOversizedBody(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	t.Setenv("PULSE_ENV", "test")

	body := strings.Repeat("x", pulseMaxRequestBodyBytes+1)
	req := signedPulseRequest(t, body, time.Now().Truncate(time.Second), "nonce-oversized")
	require.False(t, verifyPulseServiceRequest(req, "pulse-test-secret"))
}

func TestVerifyPulseServiceRequestFailsClosedInProductionWithoutRedis(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	t.Setenv("PULSE_ENV", "production")
	req := signedPulseRequest(t, "body", time.Now().Truncate(time.Second), "nonce-production-no-redis")
	require.False(t, verifyPulseServiceRequest(req, "pulse-test-secret"))
}

func TestVerifyPulseServiceRequestAcceptsPreviousSecretFromRotationSet(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	t.Setenv("PULSE_ENV", "test")
	t.Setenv("PULSE_SERVICE_HMAC_SECRET", "pulse-current-secret")
	t.Setenv("PULSE_SERVICE_HMAC_SECRET_PREVIOUS", "pulse-test-secret")
	req := signedPulseRequest(t, `{"source_ref":"grant-rotation"}`, time.Now().Truncate(time.Second), "nonce-rotation")
	require.True(t, verifyPulseServiceRequestWithSecrets(req, pulseServiceHMACSecrets()))
}

func TestPulseServiceHMACSecretsRequireActiveKey(t *testing.T) {
	t.Setenv("PULSE_SERVICE_HMAC_SECRET", "")
	t.Setenv("PULSE_SERVICE_HMAC_SECRET_PREVIOUS", "old-secret")
	require.Empty(t, pulseServiceHMACSecrets())
}

func TestPulseHMACSecretsFailClosedForInvalidProductionRotation(t *testing.T) {
	t.Setenv("PULSE_ENV", "production")
	require.Empty(t, pulseHMACSecrets("short", ""))
	require.Empty(t, pulseHMACSecrets(strings.Repeat("c", 32), "short"))
	require.Empty(t, pulseHMACSecrets(strings.Repeat("c", 32), strings.Repeat("c", 32)))
}

func TestPulseServiceAuthStoresVerifiedServiceIdentity(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	t.Setenv("PULSE_ENV", "test")
	t.Setenv("PULSE_SERVICE_HMAC_SECRET", "pulse-test-secret")
	t.Setenv("PULSE_SERVICE_HMAC_SECRET_PREVIOUS", "")

	req := signedPulseRequest(t, `{"source_ref":"grant-context"}`, time.Now().Truncate(time.Second), "nonce-context")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req

	PulseServiceAuth()(ctx)
	userID, exists := ctx.Get(pulseServiceUserIDKey)
	require.True(t, exists)
	require.Equal(t, uint64(42), userID)
	role, exists := ctx.Get("pulse_service_role")
	require.True(t, exists)
	require.Equal(t, "pulse-settlement", role)
	require.False(t, ctx.IsAborted())
}

func signedPulseRoleRequest(t *testing.T, path, role, secret, nonce string) *http.Request {
	t.Helper()
	body := `{"source_ref":"grant-admin"}`
	timestamp := time.Now().Unix()
	req := httptest.NewRequest(http.MethodPost, "http://example.test"+path, strings.NewReader(body))
	req.Header.Set(pulseUserHeader, "42")
	req.Header.Set(pulseRoleHeader, role)
	req.Header.Set(pulseTimestampHeader, strconv.FormatInt(timestamp, 10))
	req.Header.Set(pulseNonceHeader, nonce)
	mac := hmac.New(sha256.New, []byte(secret))
	_, err := mac.Write([]byte(pulseCanonicalPayload(req.Method, req.URL.EscapedPath(), "42", role, timestamp, nonce, []byte(body))))
	require.NoError(t, err)
	req.Header.Set(pulseSignatureHeader, hex.EncodeToString(mac.Sum(nil)))
	return req
}

func TestPulseRollbackRoleAndCredentialsAreIsolated(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	t.Setenv("PULSE_ENV", "test")
	t.Setenv("PULSE_SERVICE_HMAC_SECRET", "worker-secret")
	t.Setenv("PULSE_SERVICE_HMAC_SECRET_PREVIOUS", "old-worker-secret")
	t.Setenv("PULSE_ROLLBACK_HMAC_SECRET", "operator-secret")
	t.Setenv("PULSE_ROLLBACK_HMAC_SECRET_PREVIOUS", "old-operator-secret")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	router.POST("/grant", PulseServiceAuth(), ok)
	router.POST("/query", PulseBenefitQueryAuth(), ok)
	router.POST("/rollback", PulseRollbackAuth(), ok)
	for _, test := range []struct {
		name, path, role, key string
		status                int
	}{
		{"worker-grant", "/grant", "pulse-settlement", "worker-secret", 204},
		{"worker-query", "/query", "pulse-settlement", "worker-secret", 204},
		{"worker-rollback", "/rollback", "pulse-settlement", "worker-secret", 401},
		{"forged-worker-role", "/rollback", "pulse-rollback", "worker-secret", 401},
		{"old-forged-worker-role", "/rollback", "pulse-rollback", "old-worker-secret", 401},
		{"operator-rollback", "/rollback", "pulse-rollback", "operator-secret", 204},
		{"old-operator-rollback", "/rollback", "pulse-rollback", "old-operator-secret", 204},
		{"operator-query", "/query", "pulse-rollback", "operator-secret", 204},
		{"operator-grant", "/grant", "pulse-rollback", "operator-secret", 401},
		{"forged-operator-role", "/grant", "pulse-settlement", "operator-secret", 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, signedPulseRoleRequest(t, test.path, test.role, test.key, "isolated-"+test.name))
			require.Equal(t, test.status, recorder.Code)
		})
	}
}

func TestPulseRollbackKeysFailClosedWhenReusedOrMissing(t *testing.T) {
	t.Setenv("PULSE_ENV", "test")
	t.Setenv("PULSE_SERVICE_HMAC_SECRET", "worker-key")
	t.Setenv("PULSE_SERVICE_HMAC_SECRET_PREVIOUS", "old-worker-key")
	t.Setenv("PULSE_ROLLBACK_HMAC_SECRET_PREVIOUS", "")
	for _, key := range []string{"", "worker-key", "old-worker-key"} {
		t.Setenv("PULSE_ROLLBACK_HMAC_SECRET", key)
		require.Empty(t, pulseRollbackHMACSecrets())
	}
	t.Setenv("PULSE_ROLLBACK_HMAC_SECRET", "operator-key")
	t.Setenv("PULSE_ROLLBACK_HMAC_SECRET_PREVIOUS", "old-worker-key")
	require.Empty(t, pulseRollbackHMACSecrets())
}

func TestPulseBenefitGETQueryAcceptsSignedEmptyBody(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	t.Setenv("PULSE_ENV", "test")
	timestamp := time.Now().Unix()
	const nonce = "query-get-empty-body"
	req := httptest.NewRequest(http.MethodGet, "http://example.test/api/internal/pulse/benefits/query/grant-1", nil)
	req.Body = nil // Real net/http GET requests may have no body at all.
	req.Header.Set(pulseUserHeader, "1")
	req.Header.Set(pulseRoleHeader, "pulse-settlement")
	req.Header.Set(pulseTimestampHeader, strconv.FormatInt(timestamp, 10))
	req.Header.Set(pulseNonceHeader, nonce)
	mac := hmac.New(sha256.New, []byte("query-secret"))
	_, err := mac.Write([]byte(pulseCanonicalPayload(req.Method, req.URL.EscapedPath(), "1", "pulse-settlement", timestamp, nonce, nil)))
	require.NoError(t, err)
	req.Header.Set(pulseSignatureHeader, hex.EncodeToString(mac.Sum(nil)))
	require.True(t, verifyPulseServiceRequest(req, "query-secret"))
}

func TestPulseServiceAuthImmediatelyUsesConsoleRotationAndEnvironmentOverride(t *testing.T) {
	original := common.GetPulseConfigOverrides()
	t.Cleanup(func() { common.ReplacePulseConfig(original) })
	t.Setenv("PULSE_ENV", "production")
	t.Setenv("PULSE_SERVICE_HMAC_SECRET", "old-environment-worker-key")
	oldRedisEnabled, oldRedis := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldRedisEnabled, oldRedis })
	common.ReplacePulseConfig(map[string]string{"PulseEnv": "test", "PulseServiceHMACSecret": "new-console-worker-key"})
	router := gin.New()
	router.POST("/grant", PulseServiceAuth(), func(c *gin.Context) { c.Status(http.StatusOK) })
	for _, test := range []struct {
		key    string
		status int
	}{
		{"old-environment-worker-key", http.StatusUnauthorized},
		{"new-console-worker-key", http.StatusOK},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, signedPulseRoleRequest(t, "/grant", "pulse-settlement", test.key, "console-config-"+test.key))
		require.Equal(t, test.status, response.Code)
	}
	common.ReplacePulseConfig(map[string]string{"PulseEnv": "test", "PulseServiceHMACSecret": ""})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, signedPulseRoleRequest(t, "/grant", "pulse-settlement", "old-environment-worker-key", "cleared-console-key"))
	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestPulseUsageRoleIsolation(t *testing.T) {
	key := strings.Repeat("u", 32)
	t.Setenv("PULSE_ENV", "test")
	t.Setenv("PULSE_USAGE_HMAC_SECRET", key)
	t.Setenv("PULSE_SERVICE_HMAC_SECRET", "worker-key")
	call := func(role, secret, nonce string) bool {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = signedPulseRoleRequest(t, "/api/internal/pulse/usage/yesterday", role, secret, nonce)
		PulseUsageAuth()(ctx)
		return !ctx.IsAborted()
	}
	require.True(t, call("pulse-usage", key, "daily-ok"))
	require.False(t, call("pulse-usage", key, "daily-ok"))
	require.False(t, call("pulse-usage", "worker-key", "daily-wrong-key"))
	require.False(t, call("pulse-settlement", "worker-key", "daily-wrong-role"))
	t.Setenv("PULSE_SERVICE_HMAC_SECRET", key)
	require.False(t, call("pulse-usage", key, "daily-overlap"))
}
