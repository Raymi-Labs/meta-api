package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	pulseBFFRole             = "new-api"
	pulseBFFSummaryPath      = "/v1/internal/me/summary"
	pulseBFFRewardsPath      = "/v1/internal/me/rewards"
	pulseBFFMaxResponseBytes = 2 << 20
	pulseBFFUpstreamTimeout  = 3 * time.Second
	pulseHeaderUserID        = "X-Pulse-User-Id"
	pulseHeaderRole          = "X-Pulse-Role"
	pulseHeaderTimestamp     = "X-Pulse-Timestamp"
	pulseHeaderNonce         = "X-Pulse-Nonce"
	pulseHeaderSignature     = "X-Pulse-Signature"
)

var pulseBFFHTTPClient = &http.Client{Timeout: pulseBFFUpstreamTimeout}

type pulseHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// GetPulseSummary proxies the authenticated user's read-only Pulse summary.
// User identity comes exclusively from middleware.UserAuth's context.
func GetPulseSummary(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		writePulseBFFBadRequest(c, "summary 不支持查询参数")
		return
	}
	proxyPulseSignedReadProjected(c, pulseBFFRole, pulseBFFConfig, pulseBFFSummaryPath, nil, pulseBFFHTTPClient, projectPulseSummary)
}

// GetPulseRewards proxies the authenticated user's reward history. Only limit
// is accepted; user_id and all other browser-controlled selectors are rejected.
func GetPulseRewards(c *gin.Context) {
	query := c.Request.URL.Query()
	for key := range query {
		if key != "limit" {
			writePulseBFFBadRequest(c, "不支持的查询参数")
			return
		}
	}
	upstreamQuery := make(url.Values)
	if values, ok := query["limit"]; ok {
		if len(values) != 1 {
			writePulseBFFBadRequest(c, "limit 参数无效")
			return
		}
		limit, err := strconv.Atoi(values[0])
		if err != nil || limit < 1 || limit > 100 {
			writePulseBFFBadRequest(c, "limit 必须是 1 到 100 的整数")
			return
		}
		upstreamQuery.Set("limit", strconv.Itoa(limit))
	}
	proxyPulseRead(c, pulseBFFRewardsPath, upstreamQuery, pulseBFFHTTPClient)
}

func proxyPulseRead(c *gin.Context, upstreamPath string, query url.Values, client pulseHTTPDoer) {
	proxyPulseSignedRead(c, pulseBFFRole, pulseBFFConfig, upstreamPath, query, client)
}

// proxyPulseSignedRead performs the read as the logged-in user under the given
// Pulse role. The role selects which HMAC secret signs the call, so the caller
// decides the privilege level; the browser never sees either.
func proxyPulseSignedRead(c *gin.Context, role string, config func() (*url.URL, string, error), upstreamPath string, query url.Values, client pulseHTTPDoer) {
	proxyPulseSignedReadProjected(c, role, config, upstreamPath, query, client, nil)
}

func proxyPulseSignedReadProjected(c *gin.Context, role string, config func() (*url.URL, string, error), upstreamPath string, query url.Values, client pulseHTTPDoer, project func([]byte) ([]byte, error)) {
	userID := c.GetInt("id")
	if userID <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "未登录"})
		return
	}
	baseURL, secret, err := config()
	if err != nil {
		common.SysError("Pulse BFF 配置无效：" + err.Error())
		writePulseBFFUnavailable(c)
		return
	}

	target := *baseURL
	target.Path = strings.TrimRight(baseURL.Path, "/") + upstreamPath
	target.RawPath = ""
	target.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		writePulseBFFUnavailable(c)
		return
	}
	user := strconv.Itoa(userID)
	timestamp := time.Now().Unix()
	nonce, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		common.SysError("生成 Pulse BFF nonce 失败：" + err.Error())
		writePulseBFFUnavailable(c)
		return
	}
	canonical := pulseBFFCanonicalPayload(req.Method, req.URL.EscapedPath(), user, role, timestamp, nonce, nil)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	req.Header.Set("Accept", "application/json")
	req.Header.Set(pulseHeaderUserID, user)
	req.Header.Set(pulseHeaderRole, role)
	req.Header.Set(pulseHeaderTimestamp, strconv.FormatInt(timestamp, 10))
	req.Header.Set(pulseHeaderNonce, nonce)
	req.Header.Set(pulseHeaderSignature, hex.EncodeToString(mac.Sum(nil)))

	response, err := client.Do(req)
	if err != nil {
		common.SysError("Pulse BFF 请求失败：" + err.Error())
		writePulseBFFUnavailable(c)
		return
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, pulseBFFMaxResponseBytes+1))
	if err != nil || len(body) > pulseBFFMaxResponseBytes {
		common.SysError("Pulse BFF 响应读取失败或超过上限")
		writePulseBFFUnavailable(c)
		return
	}
	if response.StatusCode != http.StatusOK || !common.Valid(body) {
		common.SysError(fmt.Sprintf("Pulse BFF 上游响应无效：status=%d", response.StatusCode))
		writePulseBFFUnavailable(c)
		return
	}
	if project != nil {
		body, err = project(body)
		if err != nil {
			common.SysError("Pulse BFF 响应投影失败")
			writePulseBFFUnavailable(c)
			return
		}
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

func projectPulseSummary(body []byte) ([]byte, error) {
	var upstream map[string]json.RawMessage
	if err := common.Unmarshal(body, &upstream); err != nil {
		return nil, err
	}
	projected := make(map[string]json.RawMessage, 1)
	if tickets, ok := upstream["available_tickets"]; ok {
		projected["available_tickets"] = tickets
	}
	return common.Marshal(projected)
}

func pulseBFFConfig() (*url.URL, string, error) {
	return pulseInternalConfig("PulseUserBFFHMACSecret", "PULSE_USER_BFF_HMAC_SECRET")
}

// pulseInternalConfig resolves the shared upstream URL plus one role's secret.
// Each Pulse role has its own secret, so a leaked user-facing key cannot be
// replayed against an administrative route.
//
// The console-managed option wins over the environment variable: an operator
// who sets the secret in the admin console expects that value to take effect
// without a redeploy. The environment stays as the fallback so instances
// configured before the options existed keep working untouched.
func pulseInternalConfig(optionSecret, secretEnv string) (*url.URL, string, error) {
	cfg := common.GetPulseConfig()
	rawURL := strings.TrimSpace(cfg["PulseInternalURL"])
	secret := strings.TrimSpace(cfg[optionSecret])
	if rawURL == "" || !cfg.SecretUsable(secret) {
		return nil, "", errors.New("PULSE_INTERNAL_URL and a valid " + secretEnv + " are required")
	}
	baseURL, err := url.Parse(rawURL)
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, "", errors.New("PULSE_INTERNAL_URL must be an http(s) URL without credentials, query, or fragment")
	}
	return baseURL, secret, nil
}

func pulseBFFCanonicalPayload(method, path, userID, role string, timestamp int64, nonce string, body []byte) string {
	digest := sha256.Sum256(body)
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(method)), path, userID, role,
		strconv.FormatInt(timestamp, 10), nonce, hex.EncodeToString(digest[:]),
	}, "\n")
}

func writePulseBFFBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": message})
}

func writePulseBFFUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Pulse 服务暂不可用"})
}
