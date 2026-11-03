package dto

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestClaudeToolResultMediaTokenMetadata(t *testing.T) {
	payload := strings.Repeat("aB12+/", 200000)
	image := map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": payload}}
	document := map[string]any{"type": "document", "source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": payload}}
	request := &ClaudeRequest{MaxTokens: 128000, Messages: []ClaudeMessage{{Role: "user", Content: []any{
		map[string]any{"type": "tool_result", "tool_use_id": "tool", "content": []any{map[string]any{"type": "text", "text": "visible result"}, image, document}},
		map[string]any{"type": "tool_result", "content": "string result"},
		map[string]any{"type": "tool_use", "name": "search", "input": map[string]any{"q": "count me"}},
	}}}}
	before, err := common.Marshal(request)
	require.NoError(t, err)
	meta := request.GetTokenCountMeta()
	require.Len(t, meta.Files, 2)
	require.Equal(t, types.FileTypeImage, meta.Files[0].FileType)
	require.Equal(t, types.FileTypeFile, meta.Files[1].FileType)
	require.Contains(t, meta.CombineText, "visible result")
	require.Contains(t, meta.CombineText, "string result")
	require.Contains(t, meta.CombineText, "count me")
	require.NotContains(t, meta.CombineText, payload[:100])
	require.Less(t, len(meta.CombineText), 200)
	require.Equal(t, 128000, meta.MaxTokens)
	after, err := common.Marshal(request)
	require.NoError(t, err)
	require.Equal(t, before, after)
	direct := (&ClaudeRequest{Messages: []ClaudeMessage{{Role: "user", Content: []any{image}}}}).GetTokenCountMeta()
	require.Len(t, direct.Files, 1)
	require.NotContains(t, direct.CombineText, payload[:100])
}
