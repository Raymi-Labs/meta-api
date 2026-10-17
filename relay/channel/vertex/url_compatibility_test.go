package vertex

import (
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestVertexOfficialURLCompatibility(t *testing.T) {
	for region, host := range map[string]string{"global": "aiplatform.googleapis.com", "us": "aiplatform.us.rep.googleapis.com", "eu": "aiplatform.eu.rep.googleapis.com", "us-central1": "us-central1-aiplatform.googleapis.com"} {
		for _, keyType := range []dto.VertexKeyType{dto.VertexKeyTypeJSON, dto.VertexKeyTypeAPIKey} {
			for _, mode := range []int{RequestModeGemini, RequestModeClaude} {
				for _, stream := range []bool{false, true} {
					info := &relaycommon.RelayInfo{OriginModelName: "test", ChannelMeta: &relaycommon.ChannelMeta{ApiVersion: region, ApiKey: "test+key&value", ChannelOtherSettings: dto.ChannelOtherSettings{VertexKeyType: keyType, VertexProjectID: " configured-project "}}}
					project := "configured-project"
					if keyType == dto.VertexKeyTypeJSON {
						info.ApiKey = `{"project_id":"credential-project"}`
						project = "credential-project"
					}
					suffix := "generateContent"
					if stream {
						suffix = "streamGenerateContent?alt=sse"
					}
					a := &Adaptor{RequestMode: mode}
					got, err := a.getRequestUrl(info, "test-model", suffix)
					require.NoError(t, err)
					u, err := url.Parse(got)
					require.NoError(t, err)
					require.Equal(t, host, u.Host)
					publisher := "google"
					if mode == RequestModeClaude {
						publisher = "anthropic"
					}
					require.Equal(t, "/v1/projects/"+project+"/locations/"+region+"/publishers/"+publisher+"/models/test-model:"+strings.Split(suffix, "?")[0], u.Path)
					if stream {
						require.Equal(t, "sse", u.Query().Get("alt"))
					}
					if keyType == dto.VertexKeyTypeAPIKey {
						require.Equal(t, info.ApiKey, u.Query().Get("key"))
					} else {
						require.Empty(t, u.Query().Get("key"))
						require.Equal(t, project, a.AccountCredentials.ProjectID)
					}
				}
			}
		}
	}
}

func TestVertexAPIKeyExpressAndSettings(t *testing.T) {
	var settings dto.ChannelOtherSettings
	require.NoError(t, common.UnmarshalJsonStr(`{"vertex_key_type":"api_key","vertex_project_id":"saved-project"}`, &settings))
	require.Equal(t, "saved-project", settings.VertexProjectID)
	for _, project := range []string{"", "   "} {
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiVersion: "global", ApiKey: "test-key", ChannelOtherSettings: dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeAPIKey, VertexProjectID: project}}}
		got, err := (&Adaptor{RequestMode: RequestModeGemini}).getRequestUrl(info, "test", "generateContent")
		require.NoError(t, err)
		require.Equal(t, "https://aiplatform.googleapis.com/v1/publishers/google/models/test:generateContent?key=test-key", got)
		_, err = (&Adaptor{RequestMode: RequestModeOpenSource}).getRequestUrl(info, "test", "")
		require.Error(t, err)
	}
}
