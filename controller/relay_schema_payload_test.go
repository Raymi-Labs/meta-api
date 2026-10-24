package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Reuse the local HTTP relay fixture, but select the OpenAI Chat -> Responses
// bridge. Assertions inspect the bytes received by the server after DTO decode,
// DeepCopy, disabled-field filtering, overrides and protocol conversion.
func TestRelayChatToResponsesPreservesFinalSchemaPayload(t *testing.T) {
	f := newDeepSeekRelayFixture(t, func(w http.ResponseWriter, body []byte) {
		writeDeepSeekResponse(w, body, false)
	})
	f.channel.Type = constant.ChannelTypeOpenAI
	f.channel.SetSetting(dto.ChannelSettings{ChatCompletionsToResponsesMode: dto.ChatCompletionsToResponsesModeEnabled})
	f.channel.ParamOverride = common.GetPointer(`{"operations":[{"mode":"set","path":"max_tokens","value":127},{"mode":"set","path":"response_format.json_schema.name","value":"channel_result"},{"mode":"copy","from":"tools.0.function.parameters","to":"response_format.json_schema.schema"}]}`)
	schema := `{"type":"object","properties":{"reasoning":{"type":"string"},"answer":{"type":"integer","maximum":9007199254740993}},"required":["reasoning","answer"]}`
	body := `{"model":"deepseek-flash","messages":[{"role":"user","content":"Reply OK."}],"max_tokens":16,"service_tier":"priority","safety_identifier":"must-not-forward","tools":[{"type":"function","function":{"name":"answer","parameters":` + schema + `}}],"response_format":{"type":"json_schema","json_schema":{"name":"client_result","strict":true,"schema":` + schema + `}}}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("specific_channel_id", f.channel.Id)
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, f.channel, "deepseek-flash"))
	defer common.CleanupBodyStorage(c)
	Relay(c, types.RelayFormatOpenAI)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "OK", gjson.GetBytes(w.Body.Bytes(), "choices.0.message.content").String())
	select {
	case request := <-f.requests:
		require.Equal(t, "/v1/responses", request.path)
		require.Equal(t, "Bearer deepseek-fixture-key", request.auth)
		for _, path := range []string{"tools.0.parameters", "text.format.schema"} {
			require.Equal(t, schema, gjson.GetBytes(request.body, path).Raw, path)
		}
		require.EqualValues(t, 127, gjson.GetBytes(request.body, "max_output_tokens").Int())
		require.Equal(t, "channel_result", gjson.GetBytes(request.body, "text.format.name").String())
		require.True(t, gjson.GetBytes(request.body, "text.format.strict").Bool())
		require.False(t, gjson.GetBytes(request.body, "service_tier").Exists())
		require.False(t, gjson.GetBytes(request.body, "safety_identifier").Exists())
	case <-time.After(time.Second):
		t.Fatal("no request reached the local HTTP upstream")
	}
	require.Empty(t, f.requests, "schema forwarding must not issue a second request")
}
