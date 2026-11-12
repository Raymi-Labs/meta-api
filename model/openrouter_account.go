package model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
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
	SiteActivity  []OpenRouterSiteActivity    `json:"site_activity"`
	SiteError     string                      `json:"site_error,omitempty"`
	SiteChannels  int                         `json:"site_channels"`
	StartDate     string                      `json:"start_date"`
	EndDate       string                      `json:"end_date"`
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
	if !validOpenRouterAccountReadPath(path) {
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

var openRouterKeyHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func validOpenRouterAccountReadPath(path string) bool {
	u, err := url.ParseRequestURI(path)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Fragment != "" {
		return false
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return false
	}
	switch u.Path {
	case "/api/v1/credits":
		return len(query) == 0
	case "/api/v1/activity":
		return len(query) == 0 || (len(query) == 1 && len(query["api_key_hash"]) == 1 && openRouterKeyHashPattern.MatchString(query.Get("api_key_hash")))
	case "/api/v1/keys":
		if len(query) != 3 || len(query["include_expired"]) != 1 || query.Get("include_expired") != "true" || len(query["include_disabled"]) != 1 || query.Get("include_disabled") != "true" || len(query["offset"]) != 1 {
			return false
		}
		offset, err := strconv.Atoi(query.Get("offset"))
		return err == nil && offset >= 0 && offset <= 10000
	default:
		return false
	}
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

// Activity days are UTC. The live API also returns SQL-style timestamps, while
// the documented examples use date-only strings. Keep the dashboard contract
// date-only so sorting and inclusive date filters work for both representations.
func normalizeOpenRouterActivityDate(value string) (string, error) {
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339Nano} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format("2006-01-02"), nil
		}
	}
	return "", errors.New("OpenRouter activity date invalid")
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
		for i := range *activity.Data {
			row := &(*activity.Data)[i]
			date, dateErr := normalizeOpenRouterActivityDate(row.Date)
			if dateErr != nil || row.Usage == nil || *row.Usage < 0 || row.BYOKUsageInference < 0 || row.Requests < 0 || row.PromptTokens < 0 || row.CompletionTokens < 0 || row.ReasoningTokens < 0 {
				err = errors.New("OpenRouter activity invalid")
				break
			}
			row.Date = date
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
	client := newOpenRouterAccountClient()
	result := fetchOpenRouterFinance(ctx, client, account.remoteObserverConfig())
	end := time.Now().UTC().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -30)
	result.StartDate, result.EndDate = start.Format("2006-01-02"), end.AddDate(0, 0, -1).Format("2006-01-02")
	if result.Activity != nil {
		filtered := make([]OpenRouterAccountActivity, 0, len(result.Activity))
		for _, row := range result.Activity {
			if row.Date >= result.StartDate && row.Date <= result.EndDate {
				filtered = append(filtered, row)
			}
		}
		result.Activity = filtered
	}
	result.SiteActivity, result.SiteChannels, err = fetchOpenRouterSiteActivity(ctx, client, account, start, end)
	if err != nil {
		result.SiteError = err.Error()
	}
	return result, nil
}
