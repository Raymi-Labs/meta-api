package kling

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestKlingBearerKey(t *testing.T) {
	for _, key := range []string{"opaque-token", "  opaque-token \n", "sk-gateway", "\tsk-gateway  "} {
		t.Run(strings.TrimSpace(key), func(t *testing.T) {
			token, err := (&TaskAdaptor{}).createJWTTokenWithKey(key)
			require.NoError(t, err)
			require.Equal(t, strings.TrimSpace(key), token)
		})
	}
}

func TestKlingAccessSecretKeyJWT(t *testing.T) {
	before := time.Now().Unix()
	tokenString, err := (&TaskAdaptor{}).createJWTTokenWithKey(" access-key | secret-key ")
	require.NoError(t, err)
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		require.Equal(t, jwt.SigningMethodHS256, token.Method)
		return []byte("secret-key"), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("access-key"))
	require.NoError(t, err)
	require.True(t, token.Valid)
	require.Equal(t, "JWT", token.Header["typ"])
	expires, err := token.Claims.GetExpirationTime()
	require.NoError(t, err)
	require.GreaterOrEqual(t, expires.Unix(), before+1800)
	require.LessOrEqual(t, expires.Unix(), time.Now().Unix()+1800)
	notBefore, err := token.Claims.GetNotBefore()
	require.NoError(t, err)
	require.GreaterOrEqual(t, notBefore.Unix(), before-5)
	require.LessOrEqual(t, notBefore.Unix(), time.Now().Unix()-5)
}

func TestKlingSubmitAndPollCredentials(t *testing.T) {
	service.InitHttpClient()
	for _, tc := range []struct {
		name, key, prefix string
	}{
		{name: "single", key: "opaque-token"},
		{name: "trimmed-single", key: " opaque-token \n"},
		{name: "gateway", key: "sk-gateway", prefix: "/kling"},
		{name: "trimmed-gateway", key: " sk-gateway \n", prefix: "/kling"},
		{name: "jwt", key: " access-key | secret-key "},
	} {
		for _, action := range []string{constant.TaskActionGenerate, constant.TaskActionTextGenerate} {
			t.Run(tc.name+"/"+action, func(t *testing.T) {
				var calls atomic.Int32
				path := tc.prefix + "/v1/videos/text2video"
				if action == constant.TaskActionGenerate {
					path = tc.prefix + "/v1/videos/image2video"
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					wantPath := path
					if r.Method == http.MethodGet {
						wantPath += "/task-1"
					}
					if r.URL.Path != wantPath {
						t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
					}
					header := r.Header.Get("Authorization")
					if tc.name == "jwt" {
						parsed, err := jwt.Parse(strings.TrimPrefix(header, "Bearer "), func(*jwt.Token) (any, error) {
							return []byte("secret-key"), nil
						}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("access-key"))
						if err != nil || !parsed.Valid {
							t.Errorf("invalid JWT: %v", err)
						}
					} else if header != "Bearer "+strings.TrimSpace(tc.key) {
						t.Error("unexpected bearer credential")
					}
					_, _ = io.WriteString(w, `{}`)
				}))
				defer server.Close()
				info := &relaycommon.RelayInfo{
					ChannelMeta:   &relaycommon.ChannelMeta{ApiKey: tc.key, ChannelBaseUrl: server.URL},
					TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: action},
				}
				a := &TaskAdaptor{}
				a.Init(info)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{}`))
				resp, err := a.DoRequest(c, info, strings.NewReader(`{}`))
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				resp, err = a.FetchTask(server.URL, tc.key, map[string]any{"task_id": "task-1", "action": action}, "")
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				require.EqualValues(t, 2, calls.Load())
			})
		}
	}
}

func TestKlingInvalidKeysNeverLeaveAdaptor(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()
	for i, key := range []string{"", " \t\n", "|", "access-key|", "|secret-key", "access-key|secret-key|extra", "access-key| \t"} {
		a := &TaskAdaptor{apiKey: key}
		request := httptest.NewRequest(http.MethodPost, server.URL, nil)
		err := a.BuildRequestHeader(nil, request, nil)
		require.Error(t, err, "case %d", i)
		require.Empty(t, request.Header.Get("Authorization"))
		resp, pollErr := a.FetchTask(server.URL, key, map[string]any{"task_id": "task-1", "action": constant.TaskActionGenerate}, "")
		require.Error(t, pollErr, "case %d", i)
		require.Nil(t, resp)
		for _, err := range []error{err, pollErr} {
			require.NotContains(t, err.Error(), "access-key")
			require.NotContains(t, err.Error(), "secret-key")
		}
	}
	require.Zero(t, calls.Load())
}
