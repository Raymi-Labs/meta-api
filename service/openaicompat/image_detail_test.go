package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestChatToResponsesPreservesImageDetail(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"low", "low"}, {"high", "high"}, {"auto", "auto"}, {"original", "original"},
		{" HIGH ", "high"}, {"", "auto"}, {"bogus", "auto"},
	} {
		for _, shape := range []string{"raw", "pointer", "value"} {
			t.Run(tc.source+"/"+shape, func(t *testing.T) {
				url := "https://example.com/image.png"
				message := dto.Message{Role: "user"}
				switch shape {
				case "raw":
					image := map[string]any{"url": url}
					if tc.source != "" {
						image["detail"] = tc.source
					}
					message.Content = []any{map[string]any{"type": "image_url", "image_url": image}}
					// Token estimation may already have cached the shared high default.
					message.ParseContent()
				case "pointer":
					message.SetMediaContent([]dto.MediaContent{{Type: dto.ContentTypeImageURL, ImageUrl: &dto.MessageImageUrl{Url: url, Detail: tc.source}}})
				case "value":
					message.SetMediaContent([]dto.MediaContent{{Type: dto.ContentTypeImageURL, ImageUrl: dto.MessageImageUrl{Url: url, Detail: tc.source}}})
				}
				got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{Model: "gpt-5", Messages: []dto.Message{message}})
				require.NoError(t, err)
				var input []struct {
					Content []map[string]any `json:"content"`
				}
				require.NoError(t, common.Unmarshal(got.Input, &input))
				require.Len(t, input, 1)
				require.Len(t, input[0].Content, 1)
				require.Equal(t, url, input[0].Content[0]["image_url"])
				require.Equal(t, tc.want, input[0].Content[0]["detail"])
			})
		}
	}
}

func TestChatToResponsesImageDetailKeepsInputOrder(t *testing.T) {
	message := dto.Message{Role: "user", Content: []any{
		map[string]any{"type": "unknown"},
		map[string]any{"type": "image_url", "image_url": "https://example.com/a.png"},
		map[string]any{"type": "text", "text": "next"},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png", "detail": "original"}},
	}}
	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{Model: "gpt-5", Messages: []dto.Message{message}})
	require.NoError(t, err)
	var input []struct {
		Content []map[string]any `json:"content"`
	}
	require.NoError(t, common.Unmarshal(got.Input, &input))
	require.Len(t, input[0].Content, 3)
	require.Equal(t, "auto", input[0].Content[0]["detail"])
	require.Equal(t, "next", input[0].Content[1]["text"])
	require.Equal(t, "original", input[0].Content[2]["detail"])
}
