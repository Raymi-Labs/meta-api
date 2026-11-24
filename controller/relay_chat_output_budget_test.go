package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatOutputBudgetAtHTTPBoundary(t *testing.T) {
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	const old, current, output = "max_tokens", "max_completion_tokens", "max_output_tokens"
	for _, tc := range []struct {
		name, fields, setting, override string
		in, out                         []string
		wantValue                       int64
	}{
		{"old", `"max_tokens":37,`, "", "", []string{old}, []string{current}, 37},
		{"new", `"max_completion_tokens":61,`, "", "", []string{current}, []string{current}, 61},
		{"both", `"max_tokens":37,"max_completion_tokens":61,`, "", "", []string{old, current}, []string{current}, 61},
		{"neither", "", "", "", []string{}, []string{}, 0},
		{"zero", `"max_tokens":0,`, "", "", []string{old}, []string{}, 0},
		{"explicit_null", `"max_completion_tokens":null,`, "", "", []string{current}, []string{}, 0},
		{"passthrough", `"max_tokens":37,`, `{"pass_through_body_enabled":true}`, "", []string{old}, []string{old}, 37},
		{"override", `"max_tokens":37,`, "", `{"max_tokens":83,"model":"override-model"}`, []string{old}, []string{old, current}, 83},
		{"responses", `"max_tokens":37,`, `{"chat_completions_to_responses_mode":"enabled"}`, "", []string{old}, []string{output}, 37},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				const modelID = "gpt-6.1-terra"
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				body := fmt.Sprintf(`{%s"model":"%s","stream":%t,"messages":[{"role":"user","content":"private fixture"}]}`, tc.fields, modelID, stream)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				t.Cleanup(func() { common.CleanupBodyStorage(c) })
				request, err := helper.GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
				require.NoError(t, err)
				channel := model.Channel{Id: 140, Type: constant.ChannelTypeOpenAI, Key: "fixture", BaseURL: common.GetPointer("https://fixture.invalid"), Setting: &tc.setting, ParamOverride: &tc.override}
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, modelID))
				info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, request, nil)
				require.NoError(t, err)
				client := service.GetHttpClient()
				prev := client.Transport
				t.Cleanup(func() { client.Transport = prev })
				path := "/v1/chat/completions"
				if tc.name == "responses" {
					path = "/v1/responses"
				}
				calls := 0
				client.Transport = discoveryTransport(func(req *http.Request) (*http.Response, error) {
					calls++
					require.Equal(t, path, req.URL.Path)
					wire, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					actual := []string{}
					for _, key := range []string{old, current, output} {
						if gjson.GetBytes(wire, key).Exists() {
							actual = append(actual, key)
						}
					}
					require.Equal(t, tc.out, actual)
					if len(actual) > 0 {
						require.Equal(t, tc.wantValue, gjson.GetBytes(wire, actual[0]).Int())
					}
					return &http.Response{StatusCode: 400, Request: req, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"unsupported_parameter","param":"max_tokens","message":"Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead."}}`))}, nil
				})
				apiErr := relay.TextHelper(c, info)
				require.NotNil(t, apiErr)
				require.Equal(t, 1, calls)
				require.NotNil(t, apiErr.Upstream)
				wantModel := modelID
				if tc.name == "override" {
					wantModel = "override-model"
				}
				require.Equal(t, &types.ChatOutputBudget{Protocol: strings.TrimPrefix(path, "/v1/"), Model: wantModel, InboundFields: tc.in, OutboundFields: tc.out, Passthrough: tc.name == "passthrough"}, apiErr.Upstream.OutputBudget)
			})
		}
	}
}
