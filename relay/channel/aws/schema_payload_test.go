package aws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAWSRequestPreservesToolSchema(t *testing.T) {
	schema := `{"type":"object","properties":{"reasoning":{"type":"string"},"answer":{"type":"integer","minimum":9007199254740993}}}`
	body := `{"model":"claude-test","stream":false,"max_tokens":16,"tools":[{"name":"answer","input_schema":` + schema + `}]}`
	request, err := formatRequest(strings.NewReader(body), http.Header{})
	require.NoError(t, err)
	encoded, err := common.Marshal(request)
	require.NoError(t, err)
	require.Equal(t, schema, gjson.GetBytes(encoded, "tools.0.input_schema").Raw)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true}}}
	encoded, err = buildAwsRequestBody(ctx, info, nil)
	require.NoError(t, err)
	require.Equal(t, schema, gjson.GetBytes(encoded, "tools.0.input_schema").Raw)
	require.False(t, gjson.GetBytes(encoded, "model").Exists())
	require.False(t, gjson.GetBytes(encoded, "stream").Exists())
}

func TestAWSRequestNullToolsRemainOmitted(t *testing.T) {
	request, err := formatRequest(strings.NewReader(`{"max_tokens":16,"tools":null}`), http.Header{})
	require.NoError(t, err)
	encoded, err := common.Marshal(request)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(encoded, "tools").Exists())
}
