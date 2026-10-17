package aws

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestBedrockAllBetaLinesAndThinking(t *testing.T) {
	headers := http.Header{}
	headers.Add("anthropic-beta", " first , ,second")
	headers.Add("anthropic-beta", " third, ")
	request, err := formatRequest(strings.NewReader(`{"thinking":{"type":"adaptive","block_binding":{"keep":true},"future":false}}`), headers)
	require.NoError(t, err)
	require.JSONEq(t, `["first","second","third"]`, string(request.AnthropicBeta))
	encoded, err := common.Marshal(request.Thinking)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"adaptive","block_binding":{"keep":true},"future":false}`, string(encoded))
}
