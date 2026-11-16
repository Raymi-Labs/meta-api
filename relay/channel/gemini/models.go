package gemini

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
)

func FetchGeminiModels(baseURL, apiKey, proxyURL string) ([]string, error) {
	return FetchGeminiModelsWithHeaders(baseURL, apiKey, proxyURL, nil)
}

// Only upstream data belongs here. Reference catalogs must never feed automatic routing updates.
func FetchGeminiModelsWithHeaders(baseURL, apiKey, proxyURL string, headers http.Header) ([]string, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, fmt.Errorf("invalid model base URL")
	}
	base.Path = strings.TrimRight(base.Path, "/")
	for _, suffix := range []string{"/v1beta", "/v1"} {
		base.Path = strings.TrimSuffix(base.Path, suffix)
	}
	base.RawPath = ""
	original, err := service.NewProxyHttpClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid model proxy")
	}
	client := *original
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	read := func(endpoint string, bearer bool) ([]byte, error) {
		requestCtx, stop := context.WithTimeout(ctx, 8*time.Second)
		defer stop()
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-goog-api-key", apiKey)
		if bearer {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		for k, v := range headers {
			req.Header[k] = append([]string(nil), v...)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("model endpoint request failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("model endpoint HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
		if err != nil || len(data) > 4<<20 {
			return nil, fmt.Errorf("invalid model response size")
		}
		return data, nil
	}
	clean := func(models []string) []string {
		seen := map[string]bool{}
		result := []string{}
		for _, m := range models {
			m = strings.TrimSpace(strings.TrimPrefix(m, "models/"))
			if m != "" && !seen[m] {
				seen[m] = true
				result = append(result, m)
			}
		}
		return result
	}
	if !strings.EqualFold(base.Hostname(), "generativelanguage.googleapis.com") {
		data, e := read(base.String()+"/v1/models", true)
		if e == nil {
			var result struct {
				Data *[]struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if common.Unmarshal(data, &result) == nil && result.Data != nil {
				names := []string{}
				for _, m := range *result.Data {
					names = append(names, m.ID)
				}
				return clean(names), nil
			}
		}
	}
	names := []string{}
	token := ""
	seen := map[string]bool{}
	for page := 0; page < 100; page++ {
		endpoint := *base
		endpoint.Path += "/v1beta/models"
		q := url.Values{}
		if token != "" {
			q.Set("pageToken", token)
		}
		endpoint.RawQuery = q.Encode()
		data, e := read(endpoint.String(), false)
		if e != nil {
			return nil, e
		}
		var result struct {
			Models *[]struct {
				Name string `json:"name"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		if common.Unmarshal(data, &result) != nil || result.Models == nil {
			return nil, fmt.Errorf("invalid Gemini model response")
		}
		for _, m := range *result.Models {
			names = append(names, m.Name)
		}
		token = result.NextPageToken
		if token == "" {
			return clean(names), nil
		}
		if seen[token] {
			return nil, fmt.Errorf("repeated model page token")
		}
		seen[token] = true
	}
	return nil, fmt.Errorf("model pagination limit exceeded")
}
