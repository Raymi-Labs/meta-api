package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type geminiContentRequest struct {
	path, query string
	headers     http.Header
}

func TestGeminiVideoProxyContentCredentialsStayAtConfiguredOrigin(t *testing.T) {
	for _, scenario := range []string{"same-origin", "same-origin-existing-key", "external-signed", "lookup-external", "same-origin-redirect", "cross-origin-redirect-no-overrides"} {
		t.Run(scenario, func(t *testing.T) {
			enableControllerCapacityForTest(t)
			db := useControllerCapacityTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Task{}))
			service.InitHttpClient()
			fetchSetting := system_setting.GetFetchSetting()
			oldSSRF := fetchSetting.EnableSSRFProtection
			fetchSetting.EnableSSRFProtection = false // isolated local HTTP fixtures
			t.Cleanup(func() { fetchSetting.EnableSSRFProtection = oldSSRF })
			const savedKey = "saved-key+reserved&safe"
			const signedQuery = "X-Signature=a%2Fb%2Bz&z=last&x=1"
			originRequests := make(chan geminiContentRequest, 4)
			externalRequests := make(chan geminiContentRequest, 4)
			external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				externalRequests <- geminiContentRequest{r.URL.Path, r.URL.RawQuery, r.Header.Clone()}
				_, _ = io.WriteString(w, "external-video")
			}))
			defer external.Close()
			externalURL := external.URL + "/signed?" + signedQuery
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				originRequests <- geminiContentRequest{r.URL.Path, r.URL.RawQuery, r.Header.Clone()}
				switch r.URL.Path {
				case "/cross-hop":
					http.Redirect(w, r, externalURL, http.StatusFound)
				case "/same-hop":
					http.Redirect(w, r, "/download?"+signedQuery, http.StatusFound)
				case "/v1beta/operations/op":
					payload, err := common.Marshal(map[string]any{"done": true, "response": map[string]any{"videos": []any{map[string]any{"uri": externalURL}}}})
					if err != nil {
						t.Error(err)
					}
					_, _ = w.Write(payload)
				default:
					_, _ = io.WriteString(w, "origin-video")
				}
			}))
			defer origin.Close()
			overrides := `{"X-Owner":"{authenticated_user_id}","X-Key":"{api_key}"}`
			ch := &model.Channel{Id: 993701, Type: constant.ChannelTypeGemini, Key: "rotated-wrong-key", BaseURL: &origin.URL, HeaderOverride: &overrides, Status: common.ChannelStatusEnabled}
			uri := origin.URL + "/download?" + signedQuery
			switch scenario {
			case "same-origin-existing-key":
				uri += "&key=provider-signed-key"
			case "external-signed", "lookup-external":
				uri = externalURL
			case "same-origin-redirect":
				uri = origin.URL + "/same-hop?" + signedQuery
			case "cross-origin-redirect-no-overrides":
				uri = origin.URL + "/cross-hop?" + signedQuery
				ch.HeaderOverride = nil
			}
			require.NoError(t, db.Create(ch).Error)
			data, err := common.Marshal(map[string]string{"uri": uri})
			require.NoError(t, err)
			if scenario == "lookup-external" {
				data = nil
			}
			task := &model.Task{TaskID: "task_gemini_content", UserId: 42, ChannelId: ch.Id, Status: model.TaskStatusSuccess, Data: data,
				PrivateData: model.TaskPrivateData{Key: savedKey, UpstreamTaskID: taskcommon.EncodeLocalTaskID("operations/op")}}
			require.NoError(t, db.Create(task).Error)
			c, recorder := videoTestContext()
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			c.Set("id", 9999) // persisted task owner is authoritative
			VideoProxy(c)
			if scenario == "cross-origin-redirect-no-overrides" {
				require.Equal(t, http.StatusBadGateway, recorder.Code)
			} else {
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			}
			if scenario == "external-signed" || scenario == "lookup-external" {
				require.Len(t, externalRequests, 1)
				observed := <-externalRequests
				require.Equal(t, signedQuery, observed.query)
				for _, header := range []string{"X-Goog-Api-Key", "Authorization", "X-Key", "X-Owner"} {
					require.Empty(t, observed.headers.Get(header), header)
				}
			} else {
				require.Empty(t, externalRequests, "credentialed redirect must not contact the other origin")
			}
			originCount := 1
			if scenario == "external-signed" {
				originCount = 0
			} else if scenario == "same-origin-redirect" {
				originCount = 2
			}
			require.Len(t, originRequests, originCount)
			for len(originRequests) > 0 {
				observed := <-originRequests
				require.Equal(t, savedKey, observed.headers.Get("X-Goog-Api-Key"))
				if ch.HeaderOverride != nil {
					require.Equal(t, "42", observed.headers.Get("X-Owner"))
					require.Equal(t, savedKey, observed.headers.Get("X-Key"))
				}
				if observed.path == "/v1beta/operations/op" {
					require.Empty(t, observed.query)
				} else if scenario != "same-origin-redirect" || observed.path != "/download" {
					require.True(t, strings.HasPrefix(observed.query, signedQuery+"&key="), observed.query)
					q, err := url.ParseQuery(observed.query)
					require.NoError(t, err)
					wantKey := savedKey
					if scenario == "same-origin-existing-key" {
						wantKey = "provider-signed-key"
					}
					require.Equal(t, wantKey, q.Get("key"))
				} else {
					require.Equal(t, signedQuery, observed.query)
				}
			}
		})
	}
}

func TestGeminiContentAPIKeyPreservesSignedQueryBytes(t *testing.T) {
	for _, raw := range []string{
		"https://provider.test/video?z=a%2fb+z&%6bey=provider%2Bkey#part",
		"https://provider.test/video?key=&signature=a%2fb+z",
	} {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		require.NoError(t, addGeminiContentAPIKey(u, "saved&key+"))
		require.Equal(t, raw, u.String())
	}
	u, err := url.Parse("https://provider.test/video?signature=a%2fb+z#part")
	require.NoError(t, err)
	require.NoError(t, addGeminiContentAPIKey(u, "saved&key+"))
	require.Equal(t, "https://provider.test/video?signature=a%2fb+z&key=saved%26key%2B#part", u.String())
	invalid, err := url.Parse("https://provider.test/video?signature=a;b")
	require.NoError(t, err)
	require.Error(t, addGeminiContentAPIKey(invalid, "saved"))
	require.Equal(t, "signature=a;b", invalid.RawQuery)
}

type geminiFailingContentTransport struct{}

func (geminiFailingContentTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("test transport embeds credential URL %s", r.URL.String())
}

func TestGeminiVideoProxyLogsDoNotContainCredentialURL(t *testing.T) {
	enableControllerCapacityForTest(t)
	db := useControllerCapacityTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	service.InitHttpClient()
	client := service.GetHttpClient()
	previous := client.Transport
	client.Transport = geminiFailingContentTransport{}
	t.Cleanup(func() { client.Transport = previous })
	base := "https://provider.example.test"
	ch := &model.Channel{Id: 993702, Type: constant.ChannelTypeGemini, BaseURL: &base}
	require.NoError(t, db.Create(ch).Error)
	task := &model.Task{TaskID: "task_gemini_log", UserId: 42, ChannelId: ch.Id, Status: model.TaskStatusSuccess,
		Data: []byte(`{"uri":"https://provider.example.test/video?signature=secret-signature"}`), PrivateData: model.TaskPrivateData{Key: "secret-saved-key"}}
	require.NoError(t, db.Create(task).Error)
	c, recorder := videoTestContext()
	c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	var logs bytes.Buffer
	previousLogWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logs
	t.Cleanup(func() { gin.DefaultErrorWriter = previousLogWriter })
	VideoProxy(c)
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Contains(t, logs.String(), task.TaskID)
	for _, secret := range []string{"secret-saved-key", "secret-signature", base, "key="} {
		require.NotContains(t, logs.String(), secret)
		require.NotContains(t, recorder.Body.String(), secret)
	}
}
