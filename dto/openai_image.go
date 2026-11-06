package dto

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// Bound image multipliers before any quota arithmetic or provider submission.
const MaxImageN = 128

// ImageBillingQuantity follows the OpenAI default n=1. Only Ali may override
// it with parameters.n; an explicit provider zero is invalid, not a default.
func (i *ImageRequest) ImageBillingQuantity(ali bool) (count int, promptExtend bool, err error) {
	count = 1
	if i.N != nil {
		if *i.N > MaxImageN {
			return 0, false, fmt.Errorf("n must be between 1 and %d", MaxImageN)
		}
		if *i.N != 0 {
			count = int(*i.N)
		}
	}
	if ali {
		if raw, ok := i.Extra["parameters"]; ok {
			var parameters struct {
				N            *uint `json:"n"`
				PromptExtend bool  `json:"prompt_extend"`
			}
			if err := common.Unmarshal(raw, &parameters); err != nil {
				return 0, false, fmt.Errorf("invalid image parameters: %w", err)
			}
			if parameters.N != nil {
				if *parameters.N == 0 || *parameters.N > MaxImageN {
					return 0, false, fmt.Errorf("parameters.n must be between 1 and %d", MaxImageN)
				}
				count = int(*parameters.N)
			}
			promptExtend = parameters.PromptExtend
		}
	}
	return count, promptExtend, nil
}

type ImageRequest struct {
	Model             string          `json:"model"`
	Prompt            string          `json:"prompt" binding:"required"`
	N                 *uint           `json:"n,omitempty"`
	Size              string          `json:"size,omitempty"`
	Quality           string          `json:"quality,omitempty"`
	ResponseFormat    string          `json:"response_format,omitempty"`
	Style             json.RawMessage `json:"style,omitempty"`
	User              json.RawMessage `json:"user,omitempty"`
	ExtraFields       json.RawMessage `json:"extra_fields,omitempty"`
	Background        json.RawMessage `json:"background,omitempty"`
	Moderation        json.RawMessage `json:"moderation,omitempty"`
	OutputFormat      json.RawMessage `json:"output_format,omitempty"`
	OutputCompression json.RawMessage `json:"output_compression,omitempty"`
	// Animation controls are explicit: unknown Extra fields remain excluded.
	FrameCount      *uint           `json:"frame_count,omitempty"`
	FrameDurationMS *uint           `json:"frame_duration_ms,omitempty"`
	PartialImages   json.RawMessage `json:"partial_images,omitempty"`
	Stream          json.RawMessage `json:"stream,omitempty"`
	Images          json.RawMessage `json:"images,omitempty"`
	Mask            json.RawMessage `json:"mask,omitempty"`
	InputFidelity   json.RawMessage `json:"input_fidelity,omitempty"`
	Watermark       *bool           `json:"watermark,omitempty"`
	// zhipu 4v
	WatermarkEnabled json.RawMessage `json:"watermark_enabled,omitempty"`
	UserId           json.RawMessage `json:"user_id,omitempty"`
	Image            json.RawMessage `json:"image,omitempty"`
	// 用匿名参数接收额外参数
	Extra map[string]json.RawMessage `json:"-"`
}

func (i *ImageRequest) UnmarshalJSON(data []byte) error {
	// 先解析成 map[string]interface{}
	var rawMap map[string]json.RawMessage
	if err := common.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// 用 struct tag 获取所有已定义字段名
	knownFields := GetJSONFieldNames(reflect.TypeOf(*i))

	// 再正常解析已定义字段
	type Alias ImageRequest
	var known Alias
	if err := common.Unmarshal(data, &known); err != nil {
		return err
	}
	*i = ImageRequest(known)

	// 提取多余字段
	i.Extra = make(map[string]json.RawMessage)
	for k, v := range rawMap {
		if _, ok := knownFields[k]; !ok {
			i.Extra[k] = v
		}
	}
	return nil
}

// 序列化时需要重新把字段平铺
func (r ImageRequest) MarshalJSON() ([]byte, error) {
	// 将已定义字段转为 map
	type Alias ImageRequest
	alias := Alias(r)
	base, err := common.Marshal(alias)
	if err != nil {
		return nil, err
	}

	var baseMap map[string]json.RawMessage
	if err := common.Unmarshal(base, &baseMap); err != nil {
		return nil, err
	}

	// 不能合并ExtraFields！！！！！！！！
	// 合并 ExtraFields
	//for k, v := range r.Extra {
	//	if _, exists := baseMap[k]; !exists {
	//		baseMap[k] = v
	//	}
	//}

	return common.Marshal(baseMap)
}

func GetJSONFieldNames(t reflect.Type) map[string]struct{} {
	fields := make(map[string]struct{})
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 跳过匿名字段（例如 ExtraFields）
		if field.Anonymous {
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "-" || tag == "" {
			continue
		}

		// 取逗号前字段名（排除 omitempty 等）
		name := tag
		if commaIdx := indexComma(tag); commaIdx != -1 {
			name = tag[:commaIdx]
		}
		fields[name] = struct{}{}
	}
	return fields
}

func indexComma(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			return i
		}
	}
	return -1
}

func (i *ImageRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var sizeRatio = 1.0
	var qualityRatio = 1.0

	if strings.HasPrefix(i.Model, "dall-e") {
		// Size
		if i.Size == "256x256" {
			sizeRatio = 0.4
		} else if i.Size == "512x512" {
			sizeRatio = 0.45
		} else if i.Size == "1024x1024" {
			sizeRatio = 1
		} else if i.Size == "1024x1792" || i.Size == "1792x1024" {
			sizeRatio = 2
		}

		if i.Model == "dall-e-3" && i.Quality == "hd" {
			qualityRatio = 2.0
			if i.Size == "1024x1792" || i.Size == "1792x1024" {
				qualityRatio = 1.5
			}
		}
	}

	// n is applied once in quota calculation via OtherRatio("n").
	return &types.TokenCountMeta{
		CombineText:     i.Prompt,
		MaxTokens:       1584,
		ImagePriceRatio: sizeRatio * qualityRatio,
	}
}

func (i *ImageRequest) IsStream(c *gin.Context) bool {
	if len(i.Stream) > 0 {
		var stream bool
		if err := common.Unmarshal(i.Stream, &stream); err == nil {
			return stream
		}
		if strings.TrimSpace(string(i.Stream)) == "true" {
			return true
		}
	}
	if c == nil || c.Request == nil {
		return false
	}
	if c.Request.Form != nil && c.Request.Form.Get("stream") == "true" {
		return true
	}
	if c.Request.PostForm != nil && c.Request.PostForm.Get("stream") == "true" {
		return true
	}
	return c.Query("stream") == "true"
}

func (i *ImageRequest) SetModelName(modelName string) {
	if modelName != "" {
		i.Model = modelName
	}
}

type ImageResponse struct {
	Data     []ImageData     `json:"data"`
	Created  int64           `json:"created"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}
type ImageData struct {
	Url           string `json:"url"`
	B64Json       string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt"`
}
