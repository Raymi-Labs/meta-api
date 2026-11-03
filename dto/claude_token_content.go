package dto

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

// Only content blocks are traversed; tool input remains ordinary JSON. Never
// serialize a tool-result image/document's base64 payload as prompt text.
func appendClaudeTokenContent(content any, texts *[]string, files *[]*types.FileMeta) {
	pending := []any{content}
	for len(pending) > 0 {
		item := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if item == nil {
			continue
		}
		if text, ok := item.(string); ok {
			*texts = append(*texts, text)
			continue
		}
		block, ok := item.(ClaudeMediaMessage)
		if !ok {
			blocks, err := common.Any2Type[[]ClaudeMediaMessage](item)
			if err != nil {
				continue
			}
			for i := len(blocks) - 1; i >= 0; i-- {
				pending = append(pending, blocks[i])
			}
			continue
		}
		switch block.Type {
		case "text":
			*texts = append(*texts, block.GetText())
		case "image", "document":
			if block.Source == nil {
				continue
			}
			if block.Source.Type == "text" {
				*texts = append(*texts, common.Interface2String(block.Source.Data))
				continue
			}
			data := block.Source.Url
			if data == "" {
				data = common.Interface2String(block.Source.Data)
			}
			if data != "" {
				fileType := types.FileTypeImage
				if block.Type == "document" {
					fileType = types.FileTypeFile
				}
				*files = append(*files, &types.FileMeta{FileType: fileType, MimeType: block.Source.MediaType, Source: createClaudeFileSource(data)})
			}
		case "tool_use":
			*texts = append(*texts, block.Name)
			if block.Input != nil {
				b, _ := common.Marshal(block.Input)
				*texts = append(*texts, string(b))
			}
		case "tool_result":
			pending = append(pending, block.Content)
		}
	}
}
