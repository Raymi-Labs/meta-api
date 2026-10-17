package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestKimiUsageAuthorityConsistentAcrossHandlers(t *testing.T) {
	for _, tc := range []struct {
		name, fields, status string
		cache, reasoning     int
	}{
		{"null", `"prompt_tokens_details":{"cached_tokens":null},"completion_tokens_details":{"reasoning_tokens":null}`, "unknown", 0, 0},
		{"zero", `"prompt_tokens_details":{"cached_tokens":0},"completion_tokens_details":{"reasoning_tokens":0}`, "reported", 0, 0},
		{"negative", `"prompt_tokens_details":{"cached_tokens":-1},"completion_tokens_details":{"reasoning_tokens":-1}`, "invalid", 0, 0},
		{"over_total", `"prompt_tokens_details":{"cached_tokens":101},"completion_tokens_details":{"reasoning_tokens":21}`, "invalid", 0, 0},
		{"positive", `"prompt_tokens_details":{"cached_tokens":50},"completion_tokens_details":{"reasoning_tokens":10}`, "reported", 50, 10},
		{"alias_only", `"vendor_marker":"keep"`, "reported", 80, 18},
	} {
		for _, channel := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeMoonshot} {
			for _, stream := range []bool{false, true} {
				for _, force := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/channel=%d/stream=%t/force=%t", tc.name, channel, stream, force), func(t *testing.T) {
						c, recorder := newResponsesStreamTestContext()
						info := kimiUsageInfo(force)
						info.ChannelType = channel
						fields := `"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,` + tc.fields +
							`,"input_tokens_details":{"cached_tokens":80},"output_tokens_details":{"reasoning_tokens":18}`
						raw := `{"id":"conflict","model":"kimi-k3","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{` + fields + `}}`
						var usage *dto.Usage
						if stream {
							body := "data: " + strings.Replace(raw, `"message":{"role":"assistant","content":"OK"}`, `"delta":{"content":"OK"}`, 1) + "\n\ndata: [DONE]\n\n"
							u, apiErr := OaiStreamHandler(c, info, newResponsesStreamHTTPResponse(body))
							require.Nil(t, apiErr)
							usage = u
						} else {
							u, apiErr := OpenaiHandler(c, info, &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(raw))})
							require.Nil(t, apiErr)
							usage = u
						}
						require.Equal(t, tc.cache, usage.PromptTokensDetails.CachedTokens)
						require.Equal(t, tc.reasoning, usage.CompletionTokenDetails.ReasoningTokens)
						require.Equal(t, tc.status, info.KimiUsageEvidence.Cache.Status)
						require.Equal(t, tc.status, info.KimiUsageEvidence.Reasoning.Status)
						var output rawUsageObject
						if stream {
							events := kimiUsageEvents(t, recorder.Body.String())
							require.Len(t, events, 1)
							output = events[0]
						} else {
							output = usageObject(usageObject(recorder.Body.Bytes())["usage"])
						}
						// Preserve upstream conflicts/unknowns on the wire instead
						// of replacing them with internal zero defaults.
						source := usageObject([]byte(`{` + fields + `}`))
						for key, value := range source {
							require.JSONEq(t, string(value), string(output[key]), key)
						}
					})
				}
			}
		}
	}
}

func TestKimiConflictSnapshotsRequireUnambiguousTotals(t *testing.T) {
	first := `{"prompt_tokens":100,"completion_tokens":20,"cached_tokens":80,"reasoning_tokens":18}`
	for _, tc := range []struct {
		name, prior, tail string
		cache, reasoning  bool
	}{
		{"same_totals", first, `{"prompt_tokens":100,"input_tokens":100,"completion_tokens":20,"output_tokens":20}`, true, true},
		{"input_alias_larger", first, `{"prompt_tokens":100,"input_tokens":200,"completion_tokens":20}`, false, true},
		{"input_alias_smaller", first, `{"prompt_tokens":100,"input_tokens":50,"completion_tokens":20}`, false, true},
		{"input_alias_zero", first, `{"prompt_tokens":100,"input_tokens":0,"completion_tokens":20}`, false, true},
		{"input_alias_null", first, `{"prompt_tokens":100,"input_tokens":null,"completion_tokens":20}`, false, true},
		{"input_alias_negative", first, `{"prompt_tokens":100,"input_tokens":-1,"completion_tokens":20}`, false, true},
		{"input_alias_string", first, `{"prompt_tokens":100,"input_tokens":"100","completion_tokens":20}`, false, true},
		{"output_conflict", first, `{"prompt_tokens":100,"completion_tokens":20,"output_tokens":40}`, true, false},
		{"output_alias_null", first, `{"prompt_tokens":100,"completion_tokens":20,"output_tokens":null}`, true, false},
		{"source_input_conflict", `{"prompt_tokens":100,"input_tokens":200,"completion_tokens":20,"cached_tokens":80,"reasoning_tokens":18}`, `{"prompt_tokens":100,"completion_tokens":20}`, false, true},
		{"source_output_conflict", `{"prompt_tokens":100,"completion_tokens":20,"output_tokens":40,"cached_tokens":80,"reasoning_tokens":18}`, `{"prompt_tokens":100,"completion_tokens":20}`, true, false},
		{"alias_only_equal", `{"input_tokens":100,"output_tokens":20,"cached_tokens":80,"reasoning_tokens":18}`, `{"input_tokens":100,"output_tokens":20}`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var state kimiUsageStreamEvidence
			info := kimiUsageInfo(false)
			state.normalize(info, []byte(`{"id":"same","usage":`+tc.prior+`}`))
			tail := state.normalize(info, []byte(`{"id":"same","usage":`+tc.tail+`}`))
			result := usageObject(usageObject(tail)["usage"])
			_, cache := result["cached_tokens"]
			_, reasoning := result["reasoning_tokens"]
			require.Equal(t, tc.cache, cache)
			require.Equal(t, tc.reasoning, reasoning)
			require.Equal(t, tc.cache, info.KimiUsageEvidence.Cache.RestoredFromPriorChunk)
			require.Equal(t, tc.reasoning, info.KimiUsageEvidence.Reasoning.RestoredFromPriorChunk)
			// An intervening ambiguous frame must invalidate the snapshot;
			// later returning to old totals cannot resurrect the stale count.
			again := state.normalize(info, []byte(`{"id":"same","usage":{"prompt_tokens":100,"completion_tokens":20}}`))
			result = usageObject(usageObject(again)["usage"])
			if !tc.cache {
				require.NotContains(t, result, "cached_tokens")
			}
			if !tc.reasoning {
				require.NotContains(t, result, "reasoning_tokens")
			}
		})
	}
}

func TestKimiConflictingTotalHandlerDoesNotInheritOldCache(t *testing.T) {
	c, recorder := newResponsesStreamTestContext()
	info := kimiUsageInfo(true)
	body := "data: " + `{"id":"same","choices":[{"delta":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":20,"cached_tokens":80,"reasoning_tokens":18}}` + "\n\n" +
		"data: " + `{"id":"same","choices":[],"usage":{"prompt_tokens":100,"input_tokens":200,"completion_tokens":20}}` + "\n\ndata: [DONE]\n\n"
	usage, apiErr := OaiStreamHandler(c, info, newResponsesStreamHTTPResponse(body))
	require.Nil(t, apiErr)
	require.Equal(t, 200, usage.PromptTokens, "keep existing total normalization, not two incompatible policies")
	require.Zero(t, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 18, usage.CompletionTokenDetails.ReasoningTokens, "independent output evidence remains valid")
	events := kimiUsageEvents(t, recorder.Body.String())
	require.Len(t, events, 2)
	require.NotContains(t, events[1], "cached_tokens")
	require.False(t, info.KimiUsageEvidence.Cache.RestoredFromPriorChunk)
}

func TestKimiFreshCountsCannotExceedEitherReportedTotal(t *testing.T) {
	for _, fields := range []string{
		`"prompt_tokens":100,"input_tokens":50,"cached_tokens":80`,
		`"prompt_tokens":50,"input_tokens":100,"cached_tokens":80`,
		`"completion_tokens":20,"output_tokens":10,"reasoning_tokens":18`,
		`"completion_tokens":10,"output_tokens":20,"reasoning_tokens":18`,
	} {
		var state kimiUsageStreamEvidence
		info := kimiUsageInfo(false)
		original := []byte(`{"choices":[],"usage":{` + fields + `}}`)
		require.Equal(t, original, state.normalize(info, original))
		if strings.Contains(fields, "cached_tokens") {
			require.Equal(t, "invalid", info.KimiUsageEvidence.Cache.Status)
		} else {
			require.Equal(t, "invalid", info.KimiUsageEvidence.Reasoning.Status)
		}
	}
}

func TestKimiNegativeTotalAliasesCannotEnterSettlement(t *testing.T) {
	for _, key := range []string{"prompt_tokens", "completion_tokens", "total_tokens", "input_tokens", "output_tokens"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", key, stream), func(t *testing.T) {
				c, recorder := newResponsesStreamTestContext()
				info := kimiUsageInfo(false)
				raw := `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"` + key + `":-1}}`
				if stream {
					body := "data: " + strings.Replace(raw, `"message"`, `"delta"`, 1) + "\n\ndata: [DONE]\n\n"
					_, apiErr := OaiStreamHandler(c, info, newResponsesStreamHTTPResponse(body))
					require.NotNil(t, apiErr)
					require.NotContains(t, recorder.Body.String(), "[DONE]")
				} else {
					_, apiErr := OpenaiHandler(c, info, &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(raw))})
					require.NotNil(t, apiErr)
				}
			})
		}
	}
	// This safeguard is scoped to K3, not a change to all providers.
	info := kimiUsageInfo(false)
	info.UpstreamModelName = "gpt-4.1"
	require.NoError(t, validateKimiUsageTotals(info, &dto.Usage{InputTokens: -1}))
}
