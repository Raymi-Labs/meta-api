package service

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const requestHasVideoURLKey = "request_has_video_url"
const inputCapabilityRejectionKey = "input_capability_rejection"

// PrepareInputCapability reads only structured Chat content block types. Do not
// inspect text, URLs, filenames or base64 payloads to guess a video's format.
func PrepareInputCapability(c *gin.Context) error {
	if c == nil || c.Request == nil || !isChatInputPath(c.Request.URL.Path) {
		return nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	body, err := storage.Bytes()
	if err != nil {
		return err
	}
	hasVideo := false
	// Project only type strings; copying entire content arrays would duplicate
	// potentially large base64 attachments during routing.
	gjson.GetBytes(body, "messages.#.content.#.type").ForEach(func(_, parts gjson.Result) bool {
		parts.ForEach(func(_, kind gjson.Result) bool {
			hasVideo = kind.String() == "video_url"
			return !hasVideo
		})
		return !hasVideo
	})
	c.Set(requestHasVideoURLKey, hasVideo)
	return nil
}

func isChatInputPath(path string) bool {
	return path == "/v1/chat/completions" || path == "/pg/chat/completions"
}

// ChannelInputCapabilityError describes known local adapter restrictions only.
// Catalog modalities are display metadata, not evidence about a channel.
func ChannelInputCapabilityError(c *gin.Context, ch *model.Channel, requestedModel string) *types.NewAPIError {
	if c == nil || c.Request == nil || ch == nil {
		return nil
	}
	switch ch.Type {
	case constant.ChannelTypeAnthropic, constant.ChannelTypeAws, constant.ChannelTypeVertexAi:
	default:
		return nil
	}
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || ch.GetSetting().PassThroughBodyEnabled {
		return nil
	}
	path := c.Request.URL.Path
	if path == "/v1/responses" || path == "/v1/responses/compact" {
		switch ch.Type {
		case constant.ChannelTypeAnthropic, constant.ChannelTypeAws, constant.ChannelTypeVertexAi:
			return inputCapabilityError("unsupported_endpoint")
		}
	}
	if !isChatInputPath(path) || !c.GetBool(requestHasVideoURLKey) {
		return nil
	}
	upstreamModel := ResolveChannelUpstreamModel(ch, requestedModel)
	claudeConversion := ch.Type == constant.ChannelTypeAnthropic ||
		(ch.Type == constant.ChannelTypeAws && !strings.Contains(upstreamModel, "nova-")) ||
		(ch.Type == constant.ChannelTypeVertexAi && strings.HasPrefix(upstreamModel, "claude"))
	if claudeConversion {
		return inputCapabilityError("unsupported_video_input")
	}
	return nil
}

func inputCapabilityError(code string) *types.NewAPIError {
	message := "当前可用渠道无法将 video_url 转换为 Claude Messages；请改用支持视频输入的模型与渠道，或移除视频后重试。"
	if code == "unsupported_endpoint" {
		message = "当前可用渠道尚未实现 Responses 协议转换；请改用该模型支持的接口，或选择支持 Responses 的渠道。"
	}
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCode(code), http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// Applied after authorization and other availability filters, before priorities.
func inputCapabilityFilter(c *gin.Context, modelName string) model.ChannelFilter {
	return func(ch *model.Channel) bool {
		if err := ChannelInputCapabilityError(c, ch, modelName); err != nil {
			c.Set(inputCapabilityRejectionKey, string(err.GetErrorCode()))
			return false
		}
		return true
	}
}

func InputCapabilitySelectionError(c *gin.Context) *types.NewAPIError {
	if c != nil {
		if code := c.GetString(inputCapabilityRejectionKey); code != "" {
			return inputCapabilityError(code)
		}
	}
	return nil
}

var fileDataParameterRejection = regexp.MustCompile(`^check the request parameter input\[[0-9]+\]\.content\[[0-9]+\]\.file_data\.`)

// Diagnostics only: never broaden retry rules or infer a MIME from a filename.
func describeInputFailure(err *types.NewAPIError) (category, reason, hint string) {
	if err == nil {
		return
	}
	switch string(err.GetErrorCode()) {
	case "unsupported_video_input":
		return "unsupported_video", "当前渠道不支持此视频输入方式", "请使用支持视频的模型与接口；不要把视频直接作为普通文件提交给不支持视频的接口。"
	case "unsupported_endpoint":
		return "unsupported_endpoint", "当前渠道不支持此接口协议", "请核对模型的可用接口，选择支持该协议的渠道。"
	}
	if err.StatusCode != 400 && err.StatusCode != 422 {
		return
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "video_url cannot be converted to claude messages") {
		return describeInputFailure(inputCapabilityError("unsupported_video_input"))
	}
	fileError := strings.Contains(message, "file_data") || strings.Contains(message, "mime")
	rejected := strings.Contains(message, "unsupported") || strings.Contains(message, "not supported") || strings.Contains(message, "invalid")
	if fileError && (rejected || fileDataParameterRejection.MatchString(message)) {
		if strings.Contains(message, "video/") {
			return "unsupported_video", "上游不支持此视频提交格式", "上游拒绝了视频 MIME 类型；请改用支持视频的接口和提交格式。重复发送同一文件通常无法解决。"
		}
		return "unsupported_file", "上游拒绝了文件数据或格式", "请检查接口支持的文件类型、file_data 编码与上传方式；该错误本身不能确定具体视频格式。"
	}
	return
}
