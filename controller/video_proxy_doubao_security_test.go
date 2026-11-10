package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDoubaoVideoProxyContentAuthentication(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeDoubaoVideo, constant.ChannelTypeVolcEngine} {
		for _, scenario := range []string{"same-origin", "external-cdn", "wrong-task", "other-path", "redirect-external"} {
			t.Run(fmt.Sprintf("%d/%s", channelType, scenario), func(t *testing.T) {
				enableControllerCapacityForTest(t)
				db := useControllerCapacityTestDB(t)
				require.NoError(t, db.AutoMigrate(&model.Task{}))
				service.InitHttpClient()
				fetch := system_setting.GetFetchSetting()
				old := fetch.EnableSSRFProtection
				fetch.EnableSSRFProtection = false // local fixture servers only
				t.Cleanup(func() { fetch.EnableSSRFProtection = old })
				originHeaders := make(chan http.Header, 2)
				externalHeaders := make(chan http.Header, 2)
				external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					externalHeaders <- r.Header.Clone()
					_, _ = io.WriteString(w, "cdn-video")
				}))
				defer external.Close()
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					originHeaders <- r.Header.Clone()
					if r.URL.Path == "/v1/videos/upstream_task/content" {
						if r.Header.Get("Authorization") != "Bearer saved-task-key" {
							w.WriteHeader(http.StatusUnauthorized)
							return
						}
						if scenario == "redirect-external" {
							http.Redirect(w, r, external.URL+"/content", http.StatusFound)
							return
						}
					}
					_, _ = io.WriteString(w, "origin-video")
				}))
				defer origin.Close()
				resultURL := origin.URL + "/v1/videos/upstream_task/content"
				switch scenario {
				case "external-cdn":
					resultURL = external.URL + "/v1/videos/upstream_task/content"
				case "wrong-task":
					resultURL = origin.URL + "/v1/videos/other_task/content"
				case "other-path":
					resultURL = origin.URL + "/untrusted"
				}
				overrides := `{"X-Owner":"{authenticated_user_id}","X-Key":"{api_key}"}`
				ch := &model.Channel{Id: 993754, Type: channelType, Key: "rotated-channel-key", BaseURL: &origin.URL, Status: common.ChannelStatusEnabled, HeaderOverride: &overrides}
				require.NoError(t, db.Create(ch).Error)
				task := &model.Task{TaskID: "task_doubao_content", UserId: 42, ChannelId: ch.Id, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{Key: "saved-task-key", UpstreamTaskID: "upstream_task", ResultURL: resultURL}}
				require.NoError(t, db.Create(task).Error)
				c, recorder := videoTestContext()
				c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
				c.Set("id", 9999)
				VideoProxy(c)
				if scenario == "redirect-external" {
					require.Equal(t, http.StatusBadGateway, recorder.Code)
					require.Empty(t, externalHeaders)
					return
				}
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				var h http.Header
				if scenario == "external-cdn" {
					require.Empty(t, originHeaders)
					require.Len(t, externalHeaders, 1)
					h = <-externalHeaders
				} else {
					require.Len(t, originHeaders, 1)
					h = <-originHeaders
				}
				if scenario == "same-origin" {
					require.Equal(t, "Bearer saved-task-key", h.Get("Authorization"))
					require.Equal(t, "saved-task-key", h.Get("X-Key"))
					require.Equal(t, "42", h.Get("X-Owner"))
				} else {
					for _, name := range []string{"Authorization", "X-Key", "X-Owner"} {
						require.Empty(t, h.Get(name), name)
					}
				}
			})
		}
	}
}

func TestDoubaoTaskContentURLScope(t *testing.T) {
	for _, tc := range []struct {
		name, base, task, target string
		want                     bool
	}{
		{"exact", "https://provider.example", "task_1", "https://provider.example/v1/videos/task_1/content", true},
		{"base-prefix", "https://provider.example/proxy/", "task_1", "https://provider.example/proxy/v1/videos/task_1/content", true},
		{"other-origin", "https://provider.example", "task_1", "https://cdn.example/v1/videos/task_1/content", false},
		{"different-port", "https://provider.example", "task_1", "https://provider.example:8443/v1/videos/task_1/content", false},
		{"downgrade", "https://provider.example", "task_1", "http://provider.example/v1/videos/task_1/content", false},
		{"userinfo", "https://provider.example", "task_1", "https://user@provider.example/v1/videos/task_1/content", false},
		{"query", "https://provider.example", "task_1", "https://provider.example/v1/videos/task_1/content?redirect=https://other.example", false},
		{"fragment", "https://provider.example", "task_1", "https://provider.example/v1/videos/task_1/content#secret", false},
		{"encoded-path", "https://provider.example", "task_1", "https://provider.example/v1/videos/task_1/%63ontent", false},
		{"wrong-task", "https://provider.example", "task_1", "https://provider.example/v1/videos/task_2/content", false},
		{"empty-task", "https://provider.example", "", "https://provider.example/v1/videos//content", false},
		{"unsafe-task", "https://provider.example", "../task_1", "https://provider.example/v1/videos/../task_1/content", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target, err := url.Parse(tc.target)
			require.NoError(t, err)
			require.Equal(t, tc.want, isDoubaoTaskContentURL(constant.ChannelTypeDoubaoVideo, tc.base, tc.task, target))
			require.False(t, isDoubaoTaskContentURL(constant.ChannelTypeXai, tc.base, tc.task, target))
		})
	}
}
