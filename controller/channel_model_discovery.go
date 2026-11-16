package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Manual preview only: draft settings are never persisted, reference data never enters auto-sync.
func FetchModels(c *gin.Context) {
	var req struct {
		ChannelID              int     `json:"channel_id"`
		BaseURL                *string `json:"base_url"`
		Type                   int     `json:"type"`
		Key                    string  `json:"key"`
		Proxy                  *string `json:"proxy"`
		HeaderOverride         *string `json:"header_override"`
		AllowReferenceFallback bool    `json:"allow_reference_fallback"`
	}
	if c.ShouldBindJSON(&req) != nil || req.Type <= 0 || req.Type >= len(constant.ChannelBaseURLs) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request"})
		return
	}
	ch := &model.Channel{Type: req.Type}
	if req.ChannelID > 0 {
		saved, err := model.GetChannelById(req.ChannelID, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		ch = saved
	}
	if req.ChannelID > 0 && strings.TrimSpace(req.Key) == "" {
		draft := ""
		if ch.BaseURL != nil {
			draft = strings.TrimSpace(*ch.BaseURL)
		}
		if req.BaseURL != nil {
			draft = strings.TrimSpace(*req.BaseURL)
		}
		if draft == "" {
			draft = constant.ChannelBaseURLs[req.Type]
		}
		oldURL, e1 := url.Parse(ch.GetBaseURL())
		newURL, e2 := url.Parse(draft)
		if e1 != nil || e2 != nil || oldURL.Scheme != newURL.Scheme || !strings.EqualFold(oldURL.Host, newURL.Host) {
			c.JSON(400, gin.H{"success": false, "message": "更换上游域名后，请填写该地址对应的密钥"})
			return
		}
	}
	ch.Type = req.Type
	if req.BaseURL != nil {
		value := strings.TrimSpace(*req.BaseURL)
		ch.BaseURL = &value
	}
	if strings.TrimSpace(req.Key) != "" {
		ch.Key = strings.TrimSpace(strings.Split(strings.TrimSpace(req.Key), "\n")[0])
		ch.ChannelInfo = model.ChannelInfo{}
		ch.Keys = nil
	}
	if req.Proxy != nil {
		setting := ch.GetSetting()
		setting.Proxy = strings.TrimSpace(*req.Proxy)
		ch.SetSetting(setting)
	}
	if req.HeaderOverride != nil {
		ch.HeaderOverride = req.HeaderOverride
	}
	// Bad header JSON must not silently discard custom authentication.
	if ch.HeaderOverride != nil && strings.TrimSpace(*ch.HeaderOverride) != "" {
		var h map[string]interface{}
		if common.UnmarshalJsonStr(*ch.HeaderOverride, &h) != nil {
			c.JSON(400, gin.H{"success": false, "message": "Invalid header override"})
			return
		}
	}
	names, err := fetchChannelModelsContext(c.Request.Context(), ch)
	source := "upstream"
	if err != nil || len(names) == 0 {
		if !req.AllowReferenceFallback {
			c.JSON(200, gin.H{"success": false, "message": "获取模型列表失败"})
			return
		}
		names = nil
		names = fetchOfficialModelReference(c.Request.Context(), ch.Type)
		source = "official_reference"
		if len(names) == 0 {
			names = builtinModelReference(ch.Type)
			source = "builtin_reference"
		}
		if len(names) == 0 {
			c.JSON(200, gin.H{"success": false, "message": "获取模型列表失败"})
			return
		}
	}
	c.JSON(200, gin.H{"success": true, "data": normalizeModelNames(names), "source": source})
}

var officialGeminiModelID = regexp.MustCompile("\\*\\[`(gemini-[A-Za-z0-9._-]+)`\\]\\(/docs/models/[A-Za-z0-9._-]+\\)\\*")

func parseOfficialGeminiReference(data []byte) []string {
	var names []string
	for _, m := range officialGeminiModelID.FindAllSubmatch(data, -1) {
		names = append(names, string(m[1]))
	}
	return normalizeModelNames(names)
}

func fetchOfficialGeminiReference(parent context.Context) []string {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	// A third-party channel key is never sent to Google to obtain reference data.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, geminiPricingURL, nil)
	if err != nil {
		return nil
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil
	}
	return parseOfficialGeminiReference(data)
}

// Model discovery is bounded and never forwards credentials across redirects.
func getModelDiscoveryBodyContext(parent context.Context, endpoint string, channel *model.Channel, headers http.Header) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid model endpoint")
	}
	req.Header = headers.Clone()
	original, err := service.NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return nil, fmt.Errorf("invalid model proxy")
	}
	client := *original
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("model endpoint request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("model endpoint HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil, fmt.Errorf("invalid model response size")
	}
	return data, nil
}

// Official fallback uses a new anonymous channel: no customer credentials, proxy or headers.
func fetchOfficialModelReference(parent context.Context, channelType int) []string {
	if channelType == constant.ChannelTypeGemini {
		return fetchOfficialGeminiReference(parent)
	}
	if channelType <= 0 || channelType >= len(constant.ChannelBaseURLs) {
		return nil
	}
	base := constant.ChannelBaseURLs[channelType]
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" || channelType == constant.ChannelTypeCodex {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	ch := &model.Channel{Type: channelType, BaseURL: &base}
	endpoints, err := channelModelEndpoints(channelType, base)
	if err != nil {
		return nil
	}
	headers := http.Header{}
	if channelType == constant.ChannelTypeAnthropic {
		headers.Set("anthropic-version", "2023-06-01")
	}
	for _, endpoint := range endpoints {
		names, err := fetchModelPages(ctx, endpoint, ch, headers)
		if err == nil && len(names) > 0 {
			return names
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil
}

// Reuse relay and task adapter catalogs; account-owned apps (e.g. Dify) may have no reference list.
func builtinModelReference(channelType int) []string {
	names := append([]string(nil), channelId2Models[channelType]...)
	switch channelType {
	case constant.ChannelTypeMidjourney, constant.ChannelTypeMidjourneyPlus:
		for name := range constant.MidjourneyModel2Action {
			names = append(names, name)
		}
	case constant.ChannelTypeSunoAPI:
		for name := range constant.SunoModel2Action {
			names = append(names, name)
		}
	default:
		if adaptor := relay.GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelType))); adaptor != nil {
			names = append(names, adaptor.GetModelList()...)
		}
		if _, native := common.ChannelType2APIType(channelType); !native && len(names) == 0 {
			adaptor := relay.GetAdaptor(constant.APITypeOpenAI)
			adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channelType}})
			names = append(names, adaptor.GetModelList()...)
		}
	}
	return normalizeModelNames(names)
}
