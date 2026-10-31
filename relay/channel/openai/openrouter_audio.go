package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const openRouterAudioUploadLimit = 25 << 20
const openRouterSpeechResponseLimit = 64 << 20

func isOpenRouterGeminiAudio(info *relaycommon.RelayInfo) bool {
	return info != nil && info.ChannelMeta != nil && info.ChannelType == constant.ChannelTypeOpenRouter &&
		strings.HasPrefix(info.UpstreamModelName, "google/gemini-") &&
		(info.RelayMode == relayconstant.RelayModeAudioSpeech || info.RelayMode == relayconstant.RelayModeAudioTranscription || info.RelayMode == relayconstant.RelayModeAudioTranslation)
}

// Speech APIs ignore OpenRouter's chat routing controls. A fresh check of ALL
// endpoints is required, rather than pretending provider.only/max_price work.
// Note: .agents/notes/implemented/feature/2026-11-01-openrouter-gemini-audio-pricing.md
func convertOpenRouterGeminiAudio(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.RelayMode == relayconstant.RelayModeAudioTranslation {
		return nil, errors.New("OpenRouter Gemini audio translation is not supported")
	}
	var body map[string]interface{}
	if info.RelayMode == relayconstant.RelayModeAudioSpeech {
		if request.StreamFormat != "" && request.StreamFormat != "audio" {
			return nil, errors.New("Gemini TTS returns raw PCM, not SSE")
		}
		if request.ResponseFormat != "" && request.ResponseFormat != "pcm" {
			return nil, errors.New("Gemini TTS only supports PCM output")
		}
		if request.Speed != 0 && request.Speed != 1 {
			return nil, errors.New("Gemini TTS does not support changing speed")
		}
		raw, err := common.Marshal(request)
		if err != nil {
			return nil, err
		}
		if err = common.Unmarshal(raw, &body); err != nil {
			return nil, err
		}
		var original map[string]interface{}
		if err = common.UnmarshalBodyReusable(c, &original); err != nil {
			return nil, err
		}
		if provider, exists := original["provider"]; exists {
			body["provider"] = provider
		}
		body["response_format"] = "pcm"
	} else {
		var err error
		body, err = openRouterTranscriptionBody(c, request)
		if err != nil {
			return nil, err
		}
	}
	body["model"] = request.Model
	raw, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	raw, err = relaycommon.ApplyParamOverrideWithRelayInfo(raw, info)
	if err != nil {
		return nil, err
	}
	if err = common.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	if body["model"] != info.UpstreamModelName {
		return nil, errors.New("audio price policy must use the mapped upstream model")
	}
	if err = checkOpenRouterAudioPrices(c.Request.Context(), info, raw); err != nil {
		return nil, err
	}
	// Forward only preferences the speech API actually supports. Prices/allowlist
	// were enforced locally; they are not upstream guarantees on this endpoint.
	if provider, ok := body["provider"].(map[string]interface{}); ok {
		for k := range provider {
			if k != "zdr" && k != "data_collection" {
				delete(provider, k)
			}
		}
	}
	delete(body, "plugins")
	delete(body, "service_tier")
	delete(body, "stream_format")
	raw, err = common.Marshal(body)
	if err != nil {
		return nil, err
	}
	c.Request.Header.Set("Content-Type", "application/json")
	return bytes.NewReader(raw), nil
}

func openRouterTranscriptionBody(c *gin.Context, request dto.AudioRequest) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	if strings.HasPrefix(c.ContentType(), "application/json") {
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			return nil, err
		}
		audio, ok := body["input_audio"].(map[string]interface{})
		if !ok {
			return nil, errors.New("input_audio is required")
		}
		data, ok := audio["data"].(string)
		if !ok || data == "" || len(data) > base64.StdEncoding.EncodedLen(openRouterAudioUploadLimit) {
			return nil, errors.New("invalid or oversized input_audio")
		}
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil || len(decoded) == 0 || len(decoded) > openRouterAudioUploadLimit {
			return nil, errors.New("invalid input_audio base64")
		}
		format, _ := audio["format"].(string)
		if format == "" {
			return nil, errors.New("audio format is required")
		}
	} else {
		form, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return nil, err
		}
		files := form.File["file"]
		if len(files) != 1 || files[0].Size > openRouterAudioUploadLimit {
			return nil, errors.New("one audio file up to 25 MiB is required")
		}
		file, err := files[0].Open()
		if err != nil {
			return nil, err
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, openRouterAudioUploadLimit+1))
		if err != nil || len(data) == 0 || len(data) > openRouterAudioUploadLimit {
			return nil, errors.New("invalid or oversized audio file")
		}
		format := strings.TrimPrefix(strings.ToLower(filepath.Ext(files[0].Filename)), ".")
		if format == "" {
			return nil, errors.New("audio filename must include its format")
		}
		body["input_audio"] = map[string]interface{}{"data": base64.StdEncoding.EncodeToString(data), "format": format}
		for _, k := range []string{"language", "prompt", "response_format"} {
			if values := form.Value[k]; len(values) > 0 {
				body[k] = values[0]
			}
		}
		if values := form.Value["temperature"]; len(values) > 0 {
			var n float64
			if err := common.UnmarshalJsonStr(values[0], &n); err != nil {
				return nil, errors.New("invalid temperature")
			}
			body["temperature"] = n
		}
		if values := form.Value["provider"]; len(values) > 0 {
			var v map[string]interface{}
			if err := common.UnmarshalJsonStr(values[0], &v); err != nil {
				return nil, errors.New("invalid provider preferences")
			}
			body["provider"] = v
		}
		if values := form.Value["timestamp_granularities[]"]; len(values) > 0 {
			body["timestamp_granularities"] = values
		}
	}
	format, _ := body["response_format"].(string)
	if format == "" {
		body["response_format"] = "json"
	} else if format != "json" && format != "verbose_json" {
		return nil, errors.New("OpenRouter transcription supports json or verbose_json")
	}
	// Do not relay arbitrary top-level options capable of selecting another model.
	for k := range body {
		switch k {
		case "model", "input_audio", "language", "prompt", "response_format", "temperature", "provider", "timestamp_granularities":
		default:
			delete(body, k)
		}
	}
	body["model"] = request.Model
	return body, nil
}

func openRouterAudioGET(ctx context.Context, info *relaycommon.RelayInfo, path string) ([]byte, int, error) {
	base := strings.TrimRight(info.ChannelBaseUrl, "/")
	if base == "" {
		base = "https://openrouter.ai/api"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/"+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	client, err := service.GetHttpClientWithProxy(info.ChannelSetting.Proxy)
	if err != nil {
		return nil, 0, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("OpenRouter audio metadata redirects are not allowed")
	}
	res, err := copyClient.Do(req)
	if err != nil {
		return nil, 0, errors.New("OpenRouter audio metadata request failed")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20+1))
	if err != nil || len(b) > 2<<20 {
		return nil, res.StatusCode, errors.New("invalid OpenRouter audio metadata response")
	}
	return b, res.StatusCode, nil
}

func checkOpenRouterAudioPrices(ctx context.Context, info *relaycommon.RelayInfo, body []byte) error {
	var policy struct {
		Provider struct {
			Only []string               `json:"only"`
			Max  map[string]json.Number `json:"max_price"`
		} `json:"provider"`
	}
	if err := common.Unmarshal(body, &policy); err != nil {
		return errors.New("invalid audio price policy")
	}
	if len(policy.Provider.Only) == 0 {
		return errors.New("OpenRouter Gemini audio requires an audited provider allowlist")
	}
	limits := map[string]decimal.Decimal{}
	for _, k := range []string{"prompt", "completion"} {
		v, ok := policy.Provider.Max[k]
		if !ok {
			return fmt.Errorf("audio price ceiling missing: %s", k)
		}
		n, err := decimal.NewFromString(string(v))
		if err != nil || n.IsNegative() {
			return errors.New("invalid audio price ceiling")
		}
		limits[k] = n
	}
	allowed := map[string]bool{}
	for _, tag := range policy.Provider.Only {
		allowed[tag] = true
	}
	parts := strings.Split(info.UpstreamModelName, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("invalid OpenRouter audio model")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	raw, status, err := openRouterAudioGET(ctx, info, "models/"+url.PathEscape(parts[0])+"/"+url.PathEscape(parts[1])+"/endpoints")
	if err != nil || status != http.StatusOK {
		return errors.New("OpenRouter audio price verification unavailable; request paused")
	}
	var response struct {
		Data struct {
			Endpoints []struct {
				Tag         string                     `json:"tag"`
				ServiceTier *string                    `json:"service_tier"`
				Pricing     map[string]json.RawMessage `json:"pricing"`
			} `json:"endpoints"`
		} `json:"data"`
	}
	if err = common.Unmarshal(raw, &response); err != nil || len(response.Data.Endpoints) == 0 {
		return errors.New("OpenRouter audio has no verifiable endpoints")
	}
	for _, e := range response.Data.Endpoints {
		if !allowed[e.Tag] {
			return fmt.Errorf("unreviewed OpenRouter audio provider: %s", e.Tag)
		}
		if e.ServiceTier != nil && *e.ServiceTier != "default" && *e.ServiceTier != "standard" {
			return errors.New("unreviewed OpenRouter audio service tier")
		}
		for _, k := range []string{"prompt", "completion"} {
			raw, ok := e.Pricing[k]
			if !ok {
				return errors.New("OpenRouter audio endpoint price missing")
			}
			n, err := audioPriceDecimal(raw)
			if err != nil || n.IsNegative() || n.Mul(decimal.NewFromInt(1000000)).GreaterThan(limits[k]) {
				return errors.New("OpenRouter audio endpoint exceeds the configured price ceiling")
			}
		}
		for k, raw := range e.Pricing {
			if k == "prompt" || k == "completion" || k == "discount" {
				continue
			}
			n, err := audioPriceDecimal(raw)
			if err != nil || !n.IsZero() {
				return fmt.Errorf("unreviewed OpenRouter audio charge: %s", k)
			}
		}
	}
	return nil
}

func audioPriceDecimal(raw json.RawMessage) (decimal.Decimal, error) {
	var s string
	if len(raw) > 0 && raw[0] == '"' {
		if err := common.Unmarshal(raw, &s); err != nil {
			return decimal.Zero, err
		}
	} else {
		s = string(raw)
	}
	return decimal.NewFromString(s)
}

func openRouterAudioResponseError(err error) *types.NewAPIError {
	return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
}

func openRouterSpeechUsage(ctx context.Context, info *relaycommon.RelayInfo, id string) (*dto.Usage, error) {
	if id == "" {
		return nil, errors.New("OpenRouter speech generation ID is missing")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for attempt := 0; attempt < 4; attempt++ {
		raw, status, err := openRouterAudioGET(ctx, info, "generation?id="+url.QueryEscape(id))
		if err != nil {
			return nil, err
		}
		if status == http.StatusOK {
			var response struct {
				Data struct {
					ID         string `json:"id"`
					Prompt     *int   `json:"native_tokens_prompt"`
					Completion *int   `json:"native_tokens_completion"`
				} `json:"data"`
			}
			if err := common.Unmarshal(raw, &response); err != nil {
				return nil, err
			}
			d := response.Data
			if d.ID == id && d.Prompt != nil && d.Completion != nil && *d.Prompt >= 0 && *d.Completion > 0 {
				u := &dto.Usage{PromptTokens: *d.Prompt, CompletionTokens: *d.Completion, TotalTokens: *d.Prompt + *d.Completion, UsageSource: "openrouter_generation"}
				u.PromptTokensDetails.TextTokens = u.PromptTokens
				u.CompletionTokenDetails.AudioTokens = u.CompletionTokens
				return u, nil
			}
		} else if status != http.StatusNotFound {
			return nil, errors.New("OpenRouter speech usage lookup failed")
		}
		if attempt < 3 {
			timer := time.NewTimer(time.Duration(1<<attempt) * 200 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil, errors.New("OpenRouter speech actual token usage is unavailable; estimated billing is disabled")
}

func openRouterGeminiTTSResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)
	if !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "audio/") {
		return nil, openRouterAudioResponseError(errors.New("OpenRouter speech did not return audio"))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, openRouterSpeechResponseLimit+1))
	if err != nil || len(body) == 0 || len(body) > openRouterSpeechResponseLimit {
		return nil, openRouterAudioResponseError(errors.New("invalid or incomplete OpenRouter speech response"))
	}
	usage, err := openRouterSpeechUsage(c.Request.Context(), info, resp.Header.Get("X-Generation-Id"))
	if err != nil {
		return nil, openRouterAudioResponseError(err)
	}
	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Header("X-Generation-Id", resp.Header.Get("X-Generation-Id"))
	service.IOCopyBytesGracefully(c, resp, body)
	return usage, nil
}

func openRouterGeminiSTTResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if err != nil || len(raw) > 8<<20 {
		return nil, openRouterAudioResponseError(errors.New("invalid OpenRouter transcription response"))
	}
	var data struct {
		Text  *string `json:"text"`
		Usage *struct {
			Input  *int `json:"input_tokens"`
			Output *int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err = common.Unmarshal(raw, &data); err != nil || data.Text == nil || data.Usage == nil {
		return nil, openRouterAudioResponseError(errors.New("OpenRouter transcription text or actual usage missing"))
	}
	if data.Usage.Input == nil || data.Usage.Output == nil || *data.Usage.Input <= 0 || *data.Usage.Output < 0 {
		return nil, openRouterAudioResponseError(errors.New("invalid OpenRouter transcription token usage"))
	}
	u := &dto.Usage{PromptTokens: *data.Usage.Input, CompletionTokens: *data.Usage.Output}
	u.TotalTokens = u.PromptTokens + u.CompletionTokens
	u.PromptTokensDetails.AudioTokens = u.PromptTokens
	u.PromptTokensDetails.TextTokens = 0
	u.CompletionTokenDetails.TextTokens = u.CompletionTokens
	u.UsageSource = "openrouter_audio"
	c.Header("Content-Type", "application/json")
	service.IOCopyBytesGracefully(c, resp, raw)
	return u, nil
}
