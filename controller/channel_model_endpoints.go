package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
)

func fetchChannelUpstreamModelIDs(channel *model.Channel) ([]string, error) {
	return fetchChannelModelsContext(context.Background(), channel)
}

func fetchChannelModelsContext(parent context.Context, channel *model.Channel) ([]string, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	if channel.Type <= 0 || channel.Type >= len(constant.ChannelBaseURLs) {
		return nil, fmt.Errorf("invalid channel type")
	}
	base := strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/")
	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return nil, fmt.Errorf("获取渠道密钥失败: %w", apiErr)
	}
	headers, err := buildFetchModelsHeaders(channel, strings.TrimSpace(key))
	if err != nil {
		return nil, err
	}
	if channel.Type == constant.ChannelTypeGemini {
		return gemini.FetchGeminiModelsWithHeaders(base, strings.TrimSpace(key), channel.GetSetting().Proxy, headers)
	}
	endpoints, err := channelModelEndpoints(channel.Type, base)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, endpoint := range endpoints {
		names, err := fetchModelPages(ctx, endpoint, channel, headers)
		if err == nil {
			return names, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

// Keep provider paths for official endpoints; a custom URL may expose a compatible catalog.
func channelModelEndpoints(channelType int, base string) ([]string, error) {
	special := false
	if plan, ok := constant.ResolveChannelSpecialBase(base); ok && plan.OpenAIBaseURL != "" {
		base = strings.TrimRight(plan.OpenAIBaseURL, "/")
		special = true
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid model base URL")
	}
	// An explicit API version is already a complete base, including provider-specific prefixes.
	versioned := false
	for _, suffix := range []string{"/v1", "/v2", "/v3", "/v4", "/v1beta"} {
		versioned = versioned || strings.HasSuffix(base, suffix)
	}
	compatible := base + "/v1/models"
	if versioned {
		compatible = base + "/models"
	}
	native := compatible
	if !versioned {
		switch channelType {
		case constant.ChannelTypeAli:
			native = base + "/compatible-mode/v1/models"
		case constant.ChannelTypeZhipu_v4:
			native = base + "/api/paas/v4/models"
		case constant.ChannelTypeVolcEngine, constant.ChannelTypeDoubaoVideo:
			native = base + "/api/v3/models"
		}
	}
	if channelType == constant.ChannelTypeOllama {
		native = strings.TrimSuffix(base, "/v1") + "/api/tags"
		return uniqueModelEndpoints(native, compatible), nil
	}
	official, _ := url.Parse(constant.ChannelBaseURLs[channelType])
	if special || strings.EqualFold(u.Host, official.Host) {
		return uniqueModelEndpoints(native, compatible), nil
	}
	return uniqueModelEndpoints(compatible, native), nil
}

func uniqueModelEndpoints(first, second string) []string {
	if first == second {
		return []string{first}
	}
	return []string{first, second}
}

// Pagination stays on the selected endpoint. Incomplete/malformed catalogs are never synced.
func fetchModelPages(ctx context.Context, endpoint string, channel *model.Channel, headers http.Header) ([]string, error) {
	current, _ := url.Parse(endpoint)
	seen := map[string]bool{}
	var names []string
	for page := 0; page < 100; page++ {
		body, err := getModelDiscoveryBodyContext(ctx, current.String(), channel, headers)
		if err != nil {
			return nil, err
		}
		var result struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
			HasMore       bool   `json:"has_more"`
			LastID        string `json:"last_id"`
			NextPageToken string `json:"next_page_token"`
		}
		if common.Unmarshal(body, &result) != nil {
			return nil, fmt.Errorf("invalid model response")
		}
		var pageNames []string
		switch {
		case result.Data != nil:
			for _, item := range result.Data {
				if strings.TrimSpace(item.ID) == "" {
					return nil, fmt.Errorf("missing model ID")
				}
				pageNames = append(pageNames, item.ID)
			}
		case result.Models != nil && (channel.Type == constant.ChannelTypeCohere || channel.Type == constant.ChannelTypeOllama):
			for _, item := range result.Models {
				if strings.TrimSpace(item.Name) == "" {
					return nil, fmt.Errorf("missing model name")
				}
				pageNames = append(pageNames, item.Name)
			}
		default:
			return nil, fmt.Errorf("missing model catalog")
		}
		names = append(names, pageNames...)
		cursor, param := "", ""
		if result.HasMore {
			cursor, param = result.LastID, "after_id"
			if cursor == "" {
				return nil, fmt.Errorf("missing model cursor")
			}
		} else if result.NextPageToken != "" {
			cursor, param = result.NextPageToken, "page_token"
		} else {
			return normalizeModelNames(names), nil
		}
		if seen[cursor] || len(pageNames) == 0 {
			return nil, fmt.Errorf("invalid model pagination")
		}
		seen[cursor] = true
		query := current.Query()
		query.Set(param, cursor)
		current.RawQuery = query.Encode()
	}
	return nil, fmt.Errorf("model pagination limit exceeded")
}
