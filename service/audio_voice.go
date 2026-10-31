package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

const AudioVoiceContext = "managed_audio_voice"
const AudioVoiceModelContext = "managed_audio_voice_model"
const AudioVoicePrefix = "meta-voice-"
const MaxVoiceAudioBytes = 10 << 20

var reservedProviderVoice = regexp.MustCompile(`-mv[0-9a-f]{8}-`)

func AudioVoiceOwner(c *gin.Context) (int, int) {
	ownerToken := 0
	if common.GetContextKeyString(c, constant.ContextKeyTokenSourceType) == model.TokenSourceTypeSellableToken {
		ownerToken = common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	}
	return common.GetContextKeyInt(c, constant.ContextKeyUserId), ownerToken
}

func IsManagedAudioVoice(id string) bool     { return strings.HasPrefix(id, AudioVoicePrefix) }
func IsReservedProviderVoice(id string) bool { return reservedProviderVoice.MatchString(id) }
func AudioVoiceDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func AudioVoiceURL(base string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/api-ws/v1/inference") {
		return "", errors.New("音色服务需要阿里渠道的业务空间根地址")
	}
	switch u.Scheme {
	case "https", "wss":
		u.Scheme = "https"
	case "http", "ws":
		u.Scheme = "http"
	default:
		return "", errors.New("音色服务地址协议无效")
	}
	u.Path = "/api/v1/services/audio/tts/customization"
	return u.String(), nil
}

func AudioVoiceCredentialHash(base, key string) string {
	endpoint, _ := AudioVoiceURL(base)
	return AudioVoiceDigest([]byte(endpoint + "\x00" + key))
}

func ValidateAudioVoiceChannel(v *model.AudioVoice, ch *model.Channel) error {
	if ch.Type != constant.ChannelTypeAli || ch.Status != common.ChannelStatusEnabled || ch.ChannelInfo.IsMultiKey || len(ch.GetKeys()) != 1 {
		return errors.New("音色需要已启用的单密钥阿里渠道")
	}
	if _, err := AudioVoiceURL(ch.GetBaseURL()); err != nil {
		return err
	}
	if v != nil && (v.ChannelID != ch.Id || v.CredentialHash != AudioVoiceCredentialHash(ch.GetBaseURL(), ch.Key)) {
		return errors.New("音色绑定的渠道凭据或业务空间已变更，请联系管理员")
	}
	return nil
}

// Called for every run-task, including later tasks on an existing connection.
func ResolveSpeechVoice(c *gin.Context, info *relaycommon.RelayInfo, voice string) (string, error) {
	if bound := c.Query("voice"); c.Request.URL.Path == "/v1/audio/speech/stream" && IsManagedAudioVoice(bound) && bound != voice {
		return "", errors.New("run-task的voice必须与连接URL一致")
	}
	if IsReservedProviderVoice(voice) {
		return "", errors.New("请使用本站返回的音色ID")
	}
	if !IsManagedAudioVoice(voice) {
		return voice, nil
	}
	userID, tokenID := AudioVoiceOwner(c)
	v, err := model.GetAudioVoice(voice, userID, tokenID)
	if err != nil {
		return "", errors.New("音色不存在或无权使用")
	}
	if c.Request.URL.Path == "/v1/audio/speech/stream" && c.Query("voice") != voice {
		return "", errors.New("使用本站音色时，连接URL的voice必须与run-task一致")
	}
	if v.Status != "ready" || v.UpstreamID == "" {
		return "", errors.New("音色尚未就绪或已经删除")
	}
	if v.Model != info.OriginModelName || v.TargetModel != info.UpstreamModelName || v.ChannelID != info.ChannelId || v.CredentialHash != AudioVoiceCredentialHash(info.ChannelBaseUrl, info.ApiKey) {
		return "", errors.New("音色与当前模型、渠道或凭据不匹配")
	}
	return v.UpstreamID, nil
}

type voiceResponse struct {
	Code   string `json:"code"`
	Output *struct {
		VoiceID     string `json:"voice_id"`
		Status      string `json:"status"`
		TargetModel string `json:"target_model"`
		Voices      []struct {
			VoiceID string `json:"voice_id"`
		} `json:"voice_list"`
	} `json:"output"`
}

type VoiceProviderError struct {
	Status   int
	NotFound bool
}

func (e *VoiceProviderError) Error() string {
	return "阿里音色服务请求失败，请稍后查询音色状态"
}

func callVoiceProvider(ctx context.Context, ch *model.Channel, input map[string]any) (*voiceResponse, error) {
	endpoint, err := AudioVoiceURL(ch.GetBaseURL())
	if err != nil {
		return nil, err
	}
	body, err := common.Marshal(map[string]any{"model": "voice-enrollment", "input": input})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("无法创建音色服务请求")
	}
	req.Header.Set("Authorization", "Bearer "+ch.Key)
	req.Header.Set("Content-Type", "application/json")
	client, err := GetHttpClientWithProxy(ch.GetSetting().Proxy)
	if err != nil || client == nil {
		return nil, errors.New("音色服务代理未就绪")
	}
	isolated := *client
	isolated.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := isolated.Do(req)
	if err != nil {
		return nil, errors.New("音色服务连接中断，创建结果可能尚未确认，请查询原音色ID")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("音色服务响应无效，请查询原音色ID")
	}
	var result voiceResponse
	parseErr := common.Unmarshal(data, &result)
	if resp.StatusCode != http.StatusOK || result.Code != "" {
		return nil, &VoiceProviderError{Status: resp.StatusCode, NotFound: resp.StatusCode == 400 && (result.Code == "BadRequest.ResourceNotExist" || result.Code == "BadRequest.VoiceNotFound")}
	}
	if parseErr != nil || result.Output == nil {
		return nil, errors.New("音色服务响应无效，请查询原音色ID")
	}
	return &result, nil
}

func CreateProviderAudioVoice(ctx context.Context, ch *model.Channel, v *model.AudioVoice, mime string, audio []byte) error {
	result, err := callVoiceProvider(ctx, ch, map[string]any{"action": "create_voice", "target_model": v.TargetModel, "prefix": v.Prefix, "audio": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(audio)})
	if err != nil {
		status := "unknown"
		var provider *VoiceProviderError
		if errors.As(err, &provider) && provider.Status >= 400 && provider.Status < 500 && provider.Status != 408 {
			status = "failed"
		}
		if saveErr := model.UpdateAudioVoice(v, status, ""); saveErr != nil {
			return errors.New("创建结果保存失败，请使用原幂等键查询")
		}
		return err
	}
	if !validProviderVoice(v, result.Output.VoiceID) {
		_ = model.UpdateAudioVoice(v, "unknown", "")
		return errors.New("创建结果待确认，请查询原音色ID")
	}
	return model.UpdateAudioVoice(v, "processing", result.Output.VoiceID)
}

func validProviderVoice(v *model.AudioVoice, id string) bool {
	return len(id) <= 191 && strings.HasPrefix(id, v.TargetModel+"-"+v.Prefix+"-")
}

func RefreshAudioVoice(ctx context.Context, ch *model.Channel, v *model.AudioVoice) error {
	if v.Status == "deleted" || v.Status == "failed" || v.Status == "deleting" || v.Status == "delete_failed" {
		return nil
	}
	if v.Status == "creating" && time.Now().Unix()-v.CreatedAt < 60 {
		return nil
	}
	if v.UpstreamID == "" {
		r, err := callVoiceProvider(ctx, ch, map[string]any{"action": "list_voice", "prefix": v.Prefix, "page_index": 0, "page_size": 10})
		if err != nil {
			return err
		}
		if len(r.Output.Voices) != 1 || !validProviderVoice(v, r.Output.Voices[0].VoiceID) {
			if v.Status != "unknown" {
				return model.UpdateAudioVoice(v, "unknown", "")
			}
			return nil
		}
		if err = model.UpdateAudioVoice(v, "processing", r.Output.Voices[0].VoiceID); err != nil {
			return err
		}
	}
	r, err := callVoiceProvider(ctx, ch, map[string]any{"action": "query_voice", "voice_id": v.UpstreamID})
	if err != nil {
		var provider *VoiceProviderError
		if errors.As(err, &provider) && provider.NotFound {
			return model.UpdateAudioVoice(v, "deleted", v.UpstreamID)
		}
		return err
	}
	if r.Output.TargetModel != v.TargetModel {
		return errors.New("上游音色绑定模型不匹配")
	}
	status := "processing"
	switch r.Output.Status {
	case "OK":
		status = "ready"
	case "UNDEPLOYED":
		status = "rejected"
	case "DEPLOYING":
	default:
		return errors.New("未知的上游音色状态")
	}
	return model.UpdateAudioVoice(v, status, v.UpstreamID)
}

func DeleteProviderAudioVoice(ctx context.Context, ch *model.Channel, v *model.AudioVoice) error {
	if v.Status == "deleted" {
		return nil
	}
	if v.Status == "creating" || (v.Status == "deleting" && time.Now().Unix()-v.UpdatedAt < 60) {
		return errors.New("音色正在创建或删除，请稍后查询")
	}
	if v.UpstreamID == "" && v.Status != "failed" {
		if err := RefreshAudioVoice(ctx, ch, v); err != nil {
			return err
		}
		if v.UpstreamID == "" {
			return errors.New("创建结果尚未确认，请稍后查询；不会重新创建音色")
		}
	}
	if err := model.UpdateAudioVoice(v, "deleting", v.UpstreamID); err != nil {
		return err
	}
	if v.UpstreamID != "" {
		_, err := callVoiceProvider(ctx, ch, map[string]any{"action": "delete_voice", "voice_id": v.UpstreamID})
		if err != nil {
			var provider *VoiceProviderError
			if !errors.As(err, &provider) || !provider.NotFound {
				_ = model.UpdateAudioVoice(v, "delete_failed", v.UpstreamID)
				return err
			}
		}
	}
	return model.UpdateAudioVoice(v, "deleted", v.UpstreamID)
}

func AudioVoiceRequestHashes(userID, tokenID int, key, name, target string, data []byte) (string, string) {
	return AudioVoiceDigest([]byte(fmt.Sprintf("%d/%d/%s", userID, tokenID, key))), AudioVoiceDigest([]byte(target + "\x00" + name + "\x00" + AudioVoiceDigest(data)))
}
