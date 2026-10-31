package claude

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestOpenAI2ClaudeStopSequences(t *testing.T) {
	for _, tc := range []struct {
		name         string
		stop         any
		want         []string
		programmatic bool
	}{
		{name: "string", stop: "END", want: []string{"END"}},
		{name: "empty_string", stop: "", want: []string{""}},
		{name: "json_array", stop: []any{"END", "STOP"}, want: []string{"END", "STOP"}},
		{name: "typed_slice", stop: []string{"END", "", "END"}, want: []string{"END", "", "END"}, programmatic: true},
		{name: "mixed_array", stop: []any{"END", 1, nil, true, map[string]any{"text": "no"}, []any{"nested"}, "", "STOP", "END"}, want: []string{"END", "", "STOP", "END"}},
		{name: "invalid_array", stop: []any{1, nil, false}},
		{name: "null"},
		{name: "empty_array", stop: []any{}},
		{name: "empty_typed_slice", stop: []string{}, programmatic: true},
		{name: "number", stop: 7},
		{name: "object", stop: map[string]any{"text": "END"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := dto.GeneralOpenAIRequest{Model: "claude-test", MaxTokens: 128, Stop: tc.stop, Messages: []dto.Message{{Role: "user", Content: "hello"}}}
			original, err := common.Marshal(tc.stop)
			require.NoError(t, err)
			if !tc.programmatic {
				wire, err := common.Marshal(request)
				require.NoError(t, err)
				require.NoError(t, common.Unmarshal(wire, &request))
			}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			var converted *dto.ClaudeRequest
			require.NotPanics(t, func() { converted, err = RequestOpenAI2ClaudeMessage(ctx, request) })
			require.NoError(t, err)
			require.NotNil(t, converted)
			if len(tc.want) == 0 {
				require.Empty(t, converted.StopSequences)
			} else {
				require.Equal(t, tc.want, converted.StopSequences)
			}
			require.EqualValues(t, 128, converted.MaxTokens)
			require.Equal(t, "claude-test", converted.Model)
			after, err := common.Marshal(tc.stop)
			require.NoError(t, err)
			require.Equal(t, string(original), string(after), "parsing must not modify the caller's array")
		})
	}
}
