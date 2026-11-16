package service

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHeaderOverrideAppearsOnlyInAdminConsumeMetadata(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	client := newRelayHttpClient(120 * time.Second)
	t.Cleanup(client.CloseIdleConnections)
	RecordRelayHTTPTimeout(c, client, true, false)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 1, 1)
	require.Contains(t, other["admin_info"], "relay_http_timeout")
	require.NotContains(t, other, "relay_http_timeout")
}

func TestKimiUsageEvidenceOnlyAppearsInAdminMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 1, 1)
	require.NotContains(t, other["admin_info"], "kimi_usage_evidence")
	info.KimiUsageEvidence = &relaycommon.KimiUsageEvidence{
		Cache:     relaycommon.UsageCountEvidence{Status: "reported", Source: "usage.cache_read_tokens", Value: common.GetPointer(0)},
		Reasoning: relaycommon.UsageCountEvidence{Status: "missing"},
	}
	other = GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 1, 1)
	require.NotContains(t, other, "kimi_usage_evidence")
	snapshot := other["admin_info"].(map[string]interface{})["kimi_usage_evidence"].(relaycommon.KimiUsageEvidence)
	*info.KimiUsageEvidence.Cache.Value = 999
	info.KimiUsageEvidence.Reasoning.Status = "changed-later"
	require.Equal(t, 0, *snapshot.Cache.Value)
	require.Equal(t, "missing", snapshot.Reasoning.Status)
	raw, err := common.Marshal(snapshot)
	require.NoError(t, err)
	require.JSONEq(t, `{"cache":{"status":"reported","source":"usage.cache_read_tokens","value":0},"reasoning":{"status":"missing"}}`, string(raw))
}

func TestGenerateTextOtherInfoIncludesParamOverrideAuditAndStreamStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyClaudeIncrementalCache, true)

	start := time.Unix(100, 0)
	info := &relaycommon.RelayInfo{
		StartTime:                start,
		FirstResponseTime:        start.Add(250 * time.Millisecond),
		FirstEffectiveOutputTime: start.Add(500 * time.Millisecond),
		IsStream:                 true,
		ChannelMeta:              &relaycommon.ChannelMeta{},
		ChatToolProtocol:         dto.ChatToolProtocolLegacy,
		ChatToolCount:            3,
		OriginModelName:          "requested",
		ParamOverrideAudit: []string{
			"copy metadata.target_model -> model",
		},
	}
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RecordError("soft failure")
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
	info.ObserveResponseModel("provider-declared")

	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 1, 1)
	observed, ok := other["response_model"].(relaycommon.ResponseModel)
	if !ok || observed.ReturnedModel != "provider-declared" || !observed.Mismatch() {
		t.Fatalf("missing model diagnostics: %#v", other["response_model"])
	}
	serialized, err := common.Marshal(other)
	if err != nil {
		t.Fatal(err)
	}
	var persisted struct {
		Observation map[string]interface{} `json:"response_model"`
	}
	if err := common.Unmarshal(serialized, &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.Observation) != 3 || persisted.Observation["returned_model"] != "provider-declared" {
		t.Fatalf("persist only model names: %#v", persisted.Observation)
	}
	info.ResponseModel.ReturnedModel = "later-change"
	if other["response_model"].(relaycommon.ResponseModel).ReturnedModel != "provider-declared" {
		t.Fatal("log observation must be a detached snapshot")
	}
	if other["first_effective_output_ms"] != float64(500) {
		t.Fatalf("unexpected first_effective_output_ms: %#v", other["first_effective_output_ms"])
	}
	if other["claude_incremental_cache"] != true {
		t.Fatalf("expected claude_incremental_cache audit flag, got %#v", other["claude_incremental_cache"])
	}
	if other["chat_tool_protocol"] != dto.ChatToolProtocolLegacy || other["chat_tools_count"] != 3 {
		t.Fatalf("unexpected chat tool diagnostics: protocol=%#v count=%#v", other["chat_tool_protocol"], other["chat_tools_count"])
	}

	lines, ok := other["po"].([]string)
	if !ok {
		t.Fatalf("expected po to be []string, got %T", other["po"])
	}
	if len(lines) != 1 || lines[0] != "copy metadata.target_model -> model" {
		t.Fatalf("unexpected po: %#v", lines)
	}

	streamStatus, ok := other["stream_status"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected stream_status map, got %T", other["stream_status"])
	}
	if streamStatus["status"] != "error" {
		t.Fatalf("expected stream status error, got %#v", streamStatus["status"])
	}
	if streamStatus["end_reason"] != string(relaycommon.StreamEndReasonTimeout) {
		t.Fatalf("unexpected end_reason: %#v", streamStatus["end_reason"])
	}
	if streamStatus["error_count"] != 1 {
		t.Fatalf("unexpected error_count: %#v", streamStatus["error_count"])
	}
}

func TestGenerateTextOtherInfoIncludesResponsesRequestDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	start := time.Unix(100, 0)
	info := &relaycommon.RelayInfo{
		StartTime:         start,
		FirstResponseTime: start.Add(250 * time.Millisecond),
		IsStream:          true,
		ChannelMeta:       &relaycommon.ChannelMeta{},
		Request: &dto.OpenAIResponsesRequest{
			Input:              []byte(`["new task"]`),
			Instructions:       []byte(`"read workflow first"`),
			PreviousResponseID: "resp_old",
			PromptCacheKey:     []byte(`"trace-1"`),
		},
	}

	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 1, 1)

	diagnostics, ok := other["responses_request_diagnostics"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected responses_request_diagnostics map, got %T", other["responses_request_diagnostics"])
	}
	if diagnostics["previous_response_id_present"] != true {
		t.Fatalf("expected previous_response_id_present true, got %#v", diagnostics["previous_response_id_present"])
	}
	assertHash(t, diagnostics, "previous_response_id_hash", []byte("resp_old"))
	assertHash(t, diagnostics, "input_hash", []byte(`["new task"]`))
	assertHash(t, diagnostics, "instructions_hash", []byte(`"read workflow first"`))
	assertHash(t, diagnostics, "prompt_cache_key_hash", []byte(`"trace-1"`))
}

func TestGenerateTextOtherInfoIncludesResponsesCompletedSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	start := time.Unix(100, 0)
	summary := &relaycommon.ResponsesCompletedSummary{
		Status:                "completed",
		OutputCount:           1,
		OutputTypes:           []string{"message"},
		MessageCount:          1,
		MessageTextChars:      4,
		HasActionableToolCall: false,
	}
	info := &relaycommon.RelayInfo{
		StartTime:                 start,
		FirstResponseTime:         start.Add(250 * time.Millisecond),
		ChannelMeta:               &relaycommon.ChannelMeta{},
		ResponsesCompletedSummary: summary,
	}

	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 1, 1)

	got, ok := other["responses_completed_summary"].(*relaycommon.ResponsesCompletedSummary)
	if !ok {
		t.Fatalf("expected responses_completed_summary, got %T", other["responses_completed_summary"])
	}
	if got != summary {
		t.Fatalf("unexpected responses_completed_summary: %#v", got)
	}
}

func TestGenerateTextOtherInfoResponsesDiagnosticsOmitsAbsentFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/responses/compact", nil)

	start := time.Unix(100, 0)
	info := &relaycommon.RelayInfo{
		StartTime:         start,
		FirstResponseTime: start.Add(250 * time.Millisecond),
		ChannelMeta:       &relaycommon.ChannelMeta{},
		Request: &dto.OpenAIResponsesCompactionRequest{
			Input: []byte(`null`),
		},
	}

	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 1, 1)

	diagnostics, ok := other["responses_request_diagnostics"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected responses_request_diagnostics map, got %T", other["responses_request_diagnostics"])
	}
	if diagnostics["previous_response_id_present"] != false {
		t.Fatalf("expected previous_response_id_present false, got %#v", diagnostics["previous_response_id_present"])
	}
	if _, ok := diagnostics["input_hash"]; ok {
		t.Fatalf("expected input_hash omitted for null input")
	}
	if _, ok := diagnostics["prompt_cache_key_hash"]; ok {
		t.Fatalf("expected prompt_cache_key_hash omitted for compact requests")
	}
}

func TestGenerateTextOtherInfoIncludesTextProtocolConverter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		StartTime:              time.Unix(100, 0),
		ChannelMeta:            &relaycommon.ChannelMeta{},
		RequestConversionChain: []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI},
		TextProtocolPlan: &relaycommon.TextProtocolPlan{
			IncomingFormat: types.RelayFormatClaude,
			UpstreamFormat: types.RelayFormatOpenAI,
			Converter:      relaycommon.TextProtocolConverterClaudeToOpenAIChat,
		},
	}

	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 1, 1, 1)

	if other["request_converter"] != string(relaycommon.TextProtocolConverterClaudeToOpenAIChat) {
		t.Fatalf("unexpected request_converter: %#v", other["request_converter"])
	}
}

func assertHash(t *testing.T, diagnostics map[string]interface{}, key string, raw []byte) {
	t.Helper()
	sum := sha256.Sum256(raw)
	expected := hex.EncodeToString(sum[:])
	if diagnostics[key] != expected {
		t.Fatalf("unexpected %s: got %#v want %s", key, diagnostics[key], expected)
	}
}
