package controller

import (
	"bytes"
	"errors"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var voiceIdempotencyKey = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

func audioVoiceError(c *gin.Context, status int, message string, voice *model.AudioVoice) {
	response := gin.H{"error": gin.H{"type": "voice_error", "message": message}}
	if voice != nil {
		response["voice"] = voice
	}
	c.JSON(status, response)
}

func audioVoiceChannel(c *gin.Context, v *model.AudioVoice) (*model.Channel, func(), bool) {
	ch, err := model.GetChannelById(common.GetContextKeyInt(c, constant.ContextKeyChannelId), true)
	if err != nil {
		audioVoiceError(c, 503, "音色所在渠道不可用", v)
		return nil, nil, false
	}
	if err = service.ValidateAudioVoiceChannel(v, ch); err != nil {
		audioVoiceError(c, 400, err.Error(), v)
		return nil, nil, false
	}
	release, capacityErr := acquireFixedChannelCapacityWithWait(c, ch)
	if capacityErr != nil {
		audioVoiceError(c, capacityErr.StatusCode, "音色服务繁忙，请稍后重试", v)
		return nil, nil, false
	}
	if release == nil {
		release = func() {}
	}
	return ch, release, true
}

func CreateAudioVoice(c *gin.Context) {
	key := c.GetHeader("Idempotency-Key")
	if !voiceIdempotencyKey.MatchString(key) {
		audioVoiceError(c, 400, "请提供8–128位 Idempotency-Key（字母、数字、点、下划线、冒号、连字符）", nil)
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 {
		audioVoiceError(c, 400, "name须为1–64个字符", nil)
		return
	}
	form := c.Request.MultipartForm
	if form == nil || len(form.File) != 1 || len(form.File["file"]) != 1 {
		audioVoiceError(c, 400, "请上传一个file录音文件", nil)
		return
	}
	fh := form.File["file"][0]
	if fh.Size <= 0 || fh.Size > service.MaxVoiceAudioBytes {
		audioVoiceError(c, 413, "录音文件须大于0且不超过10 MiB", nil)
		return
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	mime := map[string]string{".wav": "audio/wav", ".mp3": "audio/mpeg", ".m4a": "audio/mp4"}[ext]
	if mime == "" {
		audioVoiceError(c, 400, "录音仅支持WAV、MP3、M4A", nil)
		return
	}
	f, err := fh.Open()
	if err != nil {
		audioVoiceError(c, 400, "无法读取录音文件", nil)
		return
	}
	defer f.Close()
	audio, err := io.ReadAll(io.LimitReader(f, service.MaxVoiceAudioBytes+1))
	if err != nil || len(audio) == 0 || len(audio) > service.MaxVoiceAudioBytes {
		audioVoiceError(c, 413, "录音文件读取失败或超过10 MiB", nil)
		return
	}
	duration, err := common.GetAudioDuration(c.Request.Context(), bytes.NewReader(audio), ext)
	if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration < 5 || duration > 60 {
		audioVoiceError(c, 400, "请上传可解析的5–60秒录音，推荐10–20秒清晰单人语音", nil)
		return
	}
	ch, release, ok := audioVoiceChannel(c, nil)
	if !ok {
		return
	}
	defer release()
	origin := c.GetString(service.AudioVoiceModelContext)
	info := &relaycommon.RelayInfo{OriginModelName: origin, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: origin}}
	if err = helper.ModelMappedHelper(c, info, nil); err != nil || !common.IsAliTTSModel(info.UpstreamModelName) {
		audioVoiceError(c, 400, "当前模型不支持本站阿里声音复刻", nil)
		return
	}
	userID, tokenID := service.AudioVoiceOwner(c)
	if userID <= 0 || common.GetContextKeyInt(c, constant.ContextKeyTokenId) <= 0 {
		audioVoiceError(c, 401, "需要有效的本站API令牌", nil)
		return
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	idem, digest := service.AudioVoiceRequestHashes(userID, tokenID, key, name, origin+"\x00"+info.UpstreamModelName, audio)
	v := &model.AudioVoice{ID: service.AudioVoicePrefix + id, UserID: userID, OwnerTokenID: tokenID, Name: name, Model: origin, TargetModel: info.UpstreamModelName, Status: "creating", ChannelID: ch.Id, CredentialHash: service.AudioVoiceCredentialHash(ch.GetBaseURL(), ch.Key), Prefix: "mv" + id[:8], IdempotencyHash: idem, RequestHash: digest}
	existing, err := model.ReserveAudioVoice(v)
	if err != nil {
		status, message := 500, "无法保存音色创建请求，请使用同一幂等键重试"
		if errors.Is(err, model.ErrAudioVoiceConflict) {
			status, message = 409, err.Error()
		}
		if errors.Is(err, model.ErrAudioVoiceLimit) {
			status, message = 429, err.Error()
		}
		audioVoiceError(c, status, message, nil)
		return
	}
	if existing != nil {
		c.JSON(http.StatusOK, existing)
		return
	}
	if err = service.CreateProviderAudioVoice(c.Request.Context(), ch, v, mime, audio); err != nil {
		audioVoiceError(c, 502, "声音复刻未确认成功，请保留音色ID并查询状态；重复提交须使用同一幂等键", v)
		return
	}
	c.JSON(http.StatusCreated, v)
}

func ListAudioVoices(c *gin.Context) {
	limit, offset := 20, 0
	var err error
	if value := c.Query("limit"); value != "" {
		limit, err = strconv.Atoi(value)
	}
	if err != nil || limit < 1 || limit > 100 {
		audioVoiceError(c, 400, "limit须为1–100", nil)
		return
	}
	if value := c.Query("offset"); value != "" {
		offset, err = strconv.Atoi(value)
	}
	if err != nil || offset < 0 || offset > 100000 {
		audioVoiceError(c, 400, "offset须为0–100000", nil)
		return
	}
	userID, tokenID := service.AudioVoiceOwner(c)
	voices, err := model.ListAudioVoices(userID, tokenID, offset, limit+1)
	if err != nil {
		audioVoiceError(c, 500, "无法查询音色列表", nil)
		return
	}
	more := len(voices) > limit
	if more {
		voices = voices[:limit]
	}
	c.JSON(200, gin.H{"object": "list", "data": voices, "has_more": more, "offset": offset, "limit": limit})
}

func RetrieveAudioVoice(c *gin.Context) {
	value, _ := c.Get(service.AudioVoiceContext)
	v, ok := value.(*model.AudioVoice)
	if !ok {
		audioVoiceError(c, 404, "音色不存在", nil)
		return
	}
	ch, release, ok := audioVoiceChannel(c, v)
	if !ok {
		return
	}
	defer release()
	if err := service.RefreshAudioVoice(c.Request.Context(), ch, v); err != nil {
		audioVoiceError(c, 502, "音色状态刷新失败，请稍后重试", v)
		return
	}
	c.JSON(200, v)
}

func DeleteAudioVoice(c *gin.Context) {
	value, _ := c.Get(service.AudioVoiceContext)
	v, ok := value.(*model.AudioVoice)
	if !ok {
		audioVoiceError(c, 404, "音色不存在", nil)
		return
	}
	ch, release, ok := audioVoiceChannel(c, v)
	if !ok {
		return
	}
	defer release()
	if err := service.DeleteProviderAudioVoice(c.Request.Context(), ch, v); err != nil {
		audioVoiceError(c, 409, "音色尚未删除，请查询状态后重试删除", v)
		return
	}
	c.JSON(200, gin.H{"id": v.ID, "deleted": true})
}
