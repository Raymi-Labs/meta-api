package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// Prepare before Distribute so ownership determines routing, never the reverse.
func AudioVoicePrepare() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		id, requestedModel := "", ""
		switch {
		case path == "/v1/audio/voices" && c.Request.Method == http.MethodPost:
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxVoiceAudioBytes+(1<<20))
			if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
				var tooLarge *http.MaxBytesError
				status := http.StatusBadRequest
				if errors.As(err, &tooLarge) {
					status = http.StatusRequestEntityTooLarge
				}
				abortWithOpenAiMessage(c, status, "请使用 multipart 上传 model、name 和 file；录音最大10 MiB")
				return
			}
			defer c.Request.MultipartForm.RemoveAll()
			c.Set(service.AudioVoiceModelContext, strings.TrimSpace(c.PostForm("model")))
		case strings.HasPrefix(path, "/v1/audio/voices/"):
			id = c.Param("voice_id")
		case path == "/v1/audio/speech" && c.Request.Method == http.MethodPost:
			var request struct {
				Voice string `json:"voice"`
				Model string `json:"model"`
			}
			if err := common.UnmarshalBodyReusable(c, &request); err != nil {
				if common.IsRequestBodyTooLargeError(err) {
					abortWithRequestBodyTooLarge(c)
				} else {
					abortWithOpenAiMessage(c, 400, "语音请求格式无效")
				}
				return
			}
			id, requestedModel = request.Voice, request.Model
		case path == "/v1/audio/speech/stream":
			id, requestedModel = c.Query("voice"), c.Query("model")
		}
		if service.IsReservedProviderVoice(id) {
			abortWithOpenAiMessage(c, 403, "请使用本站返回的音色ID")
			return
		}
		if service.IsManagedAudioVoice(id) || strings.HasPrefix(path, "/v1/audio/voices/") {
			userID, tokenID := service.AudioVoiceOwner(c)
			v, err := model.GetAudioVoice(id, userID, tokenID)
			if err != nil {
				abortWithOpenAiMessage(c, 404, "音色不存在或无权访问")
				return
			}
			if strings.HasPrefix(path, "/v1/audio/speech") && (requestedModel != v.Model || v.Status != "ready") {
				abortWithOpenAiMessage(c, 400, "音色尚未就绪、已删除或与请求模型不一致")
				return
			}
			c.Set(service.AudioVoiceContext, v)
			if strings.HasPrefix(path, "/v1/audio/voices/") {
				c.Set(service.AudioVoiceModelContext, v.Model)
			}
		}
		c.Next()
	}
}

func SelectAudioVoiceChannel(c *gin.Context, name, group, clientID string) (*model.Channel, *types.NewAPIError) {
	fail := func(message string) (*model.Channel, *types.NewAPIError) {
		return nil, types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeAccessDenied, 403, types.ErrOptionWithSkipRetry())
	}
	value, _ := c.Get(service.AudioVoiceContext)
	v, ok := value.(*model.AudioVoice)
	if !ok || v == nil || v.Model != name {
		return fail("音色与请求模型不一致")
	}
	if specified := common.GetContextKeyString(c, constant.ContextKeyTokenSpecificChannelId); specified != "" && specified != strconv.Itoa(v.ChannelID) {
		return fail("指定渠道与音色绑定渠道不一致")
	}
	if !IsTokenChannelAllowed(c, v.ChannelID) {
		return fail("当前令牌不允许使用音色所在渠道")
	}
	ch, err := model.GetChannelById(v.ChannelID, true)
	if err != nil {
		return fail("音色所在渠道不可用")
	}
	if err = service.ValidateAudioVoiceChannel(v, ch); err != nil {
		return fail(err.Error())
	}
	if !service.IsChannelAllowedForClient(ch, clientID) || service.IsChannelUnavailableForRequest(ch) || service.IsChannelModelCircuitOpen(ch, name) {
		return fail("音色所在渠道当前不可用")
	}
	matched := ""
	for _, candidate := range service.RequestModelRoutingGroups(c, group, name) {
		if model.IsChannelEnabledForGroupModel(candidate, name, ch.Id) {
			matched = candidate
			break
		}
	}
	if matched == "" {
		return fail("音色所在渠道不在当前模型的授权分组内")
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroup, matched)
	common.SetContextKey(c, constant.ContextKeyAutoGroup, matched)
	if err := SetupContextForSelectedChannel(c, ch, name); err != nil {
		return nil, err
	}
	return ch, nil
}
