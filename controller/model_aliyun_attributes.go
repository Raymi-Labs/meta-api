package controller

import (
	"bytes"
	"errors"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/net/html"
	"strings"
)

func parseAliyunModelAttributes(data []byte, id, address string) (model.ModelCapabilityMetadata, error) {
	empty := model.ModelCapabilityMetadata{}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return empty, err
	}
	identity := false
	for _, h := range documentNodes(doc, "h1") {
		if normalizeDocumentText(strings.Join(documentLines(h), " ")) == id {
			identity = true
		}
	}
	if !identity {
		return empty, errors.New("Alibaba exact model heading missing")
	}
	e := model.ModelCapabilityMetadata{Provider: "alibaba", Capabilities: model.ModelCapabilities{}}
	found := false
	for _, table := range documentTables(data) {
		h := table[0]
		if len(h) != 4 && len(h) != 2 {
			continue
		}
		if h[0] != "能力项" && h[0] != "参数" {
			continue
		}
		for _, row := range table[1:] {
			if len(row) != len(h) {
				continue
			}
			for i := 0; i < len(row); i += 2 {
				label, value := row[i], row[i+1]
				switch label {
				case "输入模态", "输出模态":
					values := model.ModelModalities{}
					valid := true
					for _, v := range strings.Fields(strings.NewReplacer("、", " ", ",", " ", "文本", "text", "参考音频", "audio", "音频", "audio", "图片", "image", "图像", "image", "视频", "video", "文件", "file").Replace(strings.ToLower(value))) {
						switch v {
						case "text", "image", "audio", "video", "file":
							values = append(values, v)
						case "link": /* URL input is not a separate supported modality in the catalog. */
						default:
							valid = false
						}
					}
					if valid && len(values) > 0 {
						found = true
						if label == "输入模态" {
							e.InputModalities = values
						} else {
							e.OutputModalities = values
						}
					}
				case "最大输出长度":
					e.MaxOutputTokens = documentTokens(value)
				default:
					if label == "联网搜索" {
						label = "web search"
					}
					if label == "上下文缓存" {
						label = "prompt caching"
					}
					if label == "流式输出" {
						label = "streaming"
					}
					applyDocumentField(&e, label, value)
				}
			}
		}
	}
	if !found {
		return empty, errors.New("Alibaba modality matrix missing or changed")
	}
	return markOfficialAttributes(e, address), nil
}
