package model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type OpenRouterAccountCredits struct {
	TotalCredits *float64 `json:"total_credits"`
	TotalUsage   *float64 `json:"total_usage"`
}

type OpenRouterAccountActivity struct {
	Date               string   `json:"date"`
	Model              string   `json:"model"`
	ModelPermaslug     string   `json:"model_permaslug"`
	EndpointID         string   `json:"endpoint_id"`
	ProviderName       string   `json:"provider_name"`
	Requests           int64    `json:"requests"`
	PromptTokens       int64    `json:"prompt_tokens"`
	CompletionTokens   int64    `json:"completion_tokens"`
	ReasoningTokens    int64    `json:"reasoning_tokens"`
	Usage              *float64 `json:"usage"`
	BYOKUsageInference float64  `json:"byok_usage_inference"`
}

type OpenRouterAccountFinance struct {
	Credits       *OpenRouterAccountCredits   `json:"credits"`
	Activity      []OpenRouterAccountActivity `json:"activity"`
	CreditsError  string                      `json:"credits_error,omitempty"`
	ActivityError string                      `json:"activity_error,omitempty"`
	FetchedAt     int64                       `json:"fetched_at"`
}

func newOpenRouterAccountClient() *http.Client {
	client := newUpstreamAccountRemoteHTTPClient()
	client.Timeout = 20 * time.Second
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return client
}

// Only official read endpoints are reachable; never expose upstream bodies or credentials in errors.
func openRouterAccountRequest(ctx context.Context, client *http.Client, config UpstreamAccountRemoteConfig, path string, target any) error {
	if path != "/api/v1/credits" && path != "/api/v1/activity" {
		return errors.New("invalid OpenRouter endpoint")
	}
	token := strings.TrimSpace(config.AccessToken)
	if token == "" && config.AccessTokenEncrypted != "" {
		var err error
		token, err = decryptUpstreamAccountRemoteSecret(config.AccessTokenEncrypted)
		if err != nil {
			return errors.New("OpenRouter credential unavailable")
		}
	}
	if token == "" {
		return ErrUpstreamAccountRemoteTokenEmpty
	}
	if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterAccountBaseURL+path, nil)
	if err != nil {
		return errors.New("OpenRouter request invalid")
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("OpenRouter request failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return fmt.Errorf("OpenRouter HTTP %d: Management Key required", resp.StatusCode)
		}
		return fmt.Errorf("OpenRouter HTTP %d", resp.StatusCode)
	}
	const maxBody = 16 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return errors.New("OpenRouter response unavailable or too large")
	}
	if err := common.Unmarshal(body, target); err != nil {
		return errors.New("OpenRouter response invalid")
	}
	return nil
}

func fetchOpenRouterCredits(ctx context.Context, client *http.Client, config UpstreamAccountRemoteConfig) (*OpenRouterAccountCredits, error) {
	var response struct {
		Data *OpenRouterAccountCredits `json:"data"`
	}
	if err := openRouterAccountRequest(ctx, client, config, "/api/v1/credits", &response); err != nil {
		return nil, err
	}
	if response.Data == nil || response.Data.TotalCredits == nil || response.Data.TotalUsage == nil {
		return nil, errors.New("OpenRouter credits missing")
	}
	for _, amount := range []float64{*response.Data.TotalCredits, *response.Data.TotalUsage} {
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || amount > float64(math.MaxInt64)/common.QuotaPerUnit {
			return nil, errors.New("OpenRouter credits invalid")
		}
	}
	return response.Data, nil
}

func fetchOpenRouterAccountSnapshot(config UpstreamAccountRemoteConfig) (*upstreamAccountRemoteFetchResult, error) {
	credits, err := fetchOpenRouterCredits(context.Background(), newOpenRouterAccountClient(), config)
	if err != nil {
		return nil, err
	}
	return &upstreamAccountRemoteFetchResult{
		RemoteQuotaPerUnit: common.QuotaPerUnit,
		WalletQuota:        int64(math.Round((*credits.TotalCredits - *credits.TotalUsage) * common.QuotaPerUnit)),
		WalletUsedQuota:    upstreamAccountUSDToQuota(*credits.TotalUsage),
	}, nil
}

func fetchOpenRouterFinance(ctx context.Context, client *http.Client, config UpstreamAccountRemoteConfig) *OpenRouterAccountFinance {
	result := &OpenRouterAccountFinance{Activity: nil, FetchedAt: common.GetTimestamp()}
	credits, err := fetchOpenRouterCredits(ctx, client, config)
	if err != nil {
		result.CreditsError = err.Error()
	} else {
		result.Credits = credits
	}
	var activity struct {
		Data *[]OpenRouterAccountActivity `json:"data"`
	}
	err = openRouterAccountRequest(ctx, client, config, "/api/v1/activity", &activity)
	if err == nil && activity.Data == nil {
		err = errors.New("OpenRouter activity missing")
	}
	if err == nil {
		for _, row := range *activity.Data {
			if _, dateErr := time.Parse("2006-01-02", row.Date); dateErr != nil || row.Usage == nil || *row.Usage < 0 || row.BYOKUsageInference < 0 || row.Requests < 0 || row.PromptTokens < 0 || row.CompletionTokens < 0 || row.ReasoningTokens < 0 {
				err = errors.New("OpenRouter activity invalid")
				break
			}
		}
	}
	if err != nil {
		result.ActivityError = err.Error()
	} else {
		result.Activity = *activity.Data
		sort.SliceStable(result.Activity, func(i, j int) bool { return result.Activity[i].Date > result.Activity[j].Date })
	}
	return result
}

func GetOpenRouterAccountFinance(ctx context.Context, id int) (*OpenRouterAccountFinance, error) {
	account, err := getUpstreamAccountByID(id)
	if err != nil {
		return nil, err
	}
	if account.AccountType != UpstreamAccountTypeOpenRouter {
		return nil, ErrUpstreamAccountTypeUnsupported
	}
	if !account.Enabled {
		return nil, errors.New("OpenRouter account disabled")
	}
	return fetchOpenRouterFinance(ctx, newOpenRouterAccountClient(), account.remoteObserverConfig()), nil
}
