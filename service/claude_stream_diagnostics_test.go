package service

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestClaudeStreamDiagnosticsOnlyWhitelistedSettings(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := &dto.ClaudeRequest{MaxTokens: 64000, Thinking: &dto.Thinking{Type: "adaptive"}, OutputConfig: []byte(`{"effort":"max","secret":"private prompt"}`)}
	CaptureClaudeRequestSettings(c, req)
	s := relaycommon.NewStreamStatus()
	s.SetEndReason(relaycommon.StreamEndReasonScannerErr, errors.New("read tcp secret-address: connection reset by peer"))
	CaptureClaudeStreamFailure(c, s, false)
	admin := map[string]interface{}{}
	AppendClaudeStreamDiagnostics(c, admin)
	data, err := common.Marshal(admin)
	require.NoError(t, err)
	require.Contains(t, string(data), `"effort":"max"`)
	require.Contains(t, string(data), "connection_reset")
	require.NotContains(t, string(data), "secret")
	require.NotContains(t, string(data), "private prompt")
	req.Thinking.Type = "private-secret"
	req.OutputConfig = []byte(`{"effort":"private-secret"}`)
	CaptureClaudeRequestSettings(c, req)
	AppendClaudeStreamDiagnostics(c, admin)
	data, err = common.Marshal(admin)
	require.NoError(t, err)
	require.NotContains(t, string(data), "private-secret")
}
