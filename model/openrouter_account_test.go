package model

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

type openRouterTestTransport func(*http.Request) (*http.Response, error)

func (f openRouterTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func openRouterTestClient(t *testing.T, credits string, activity string, activityStatus int) *http.Client {
	t.Helper()
	return &http.Client{Transport: openRouterTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "openrouter.ai" || r.URL.Scheme != "https" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-secret" || r.Header.Get("New-Api-User") != "" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL)
		}
		body, status := credits, 200
		if r.URL.Path == "/api/v1/activity" {
			body, status = activity, activityStatus
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})}
}
func TestOpenRouterFinancePartialFailureAndEmpty(t *testing.T) {
	config := UpstreamAccountRemoteConfig{AccessToken: "test-secret", BaseURL: "https://attacker.invalid"}
	for _, tc := range []struct {
		name, credits, activity string
		status                  int
		creditOK, activityOK    bool
	}{
		{"valid", `{"data":{"total_credits":50,"total_usage":12.5}}`, `{"data":[{"date":"2026-11-01","model":"m","usage":1.25,"byok_usage_inference":4}]}`, 200, true, true},
		{"empty", `{"data":{"total_credits":0,"total_usage":0}}`, `{"data":[]}`, 200, true, true},
		{"activity-forbidden", `{"data":{"total_credits":50,"total_usage":12.5}}`, `test-secret`, 403, true, false},
		{"credits-missing", `{"data":{"total_credits":50}}`, `{"data":[]}`, 200, false, true},
		{"usage-missing", `{}`, `{"data":[{"date":"2026-11-01"}]}`, 200, false, false},
		{"activity-missing", `{"data":{"total_credits":50,"total_usage":12.5}}`, `{}`, 200, true, false},
		{"activity-invalid", `{}`, `{"data":[{"date":"bad","usage":1}]}`, 200, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := fetchOpenRouterFinance(context.Background(), openRouterTestClient(t, tc.credits, tc.activity, tc.status), config)
			if (result.Credits != nil) != tc.creditOK || (result.Activity != nil) != tc.activityOK {
				t.Fatalf("unexpected result %+v", result)
			}
			if strings.Contains(result.CreditsError+result.ActivityError, "test-secret") {
				t.Fatal("secret leaked")
			}
			if tc.name == "valid" && *result.Activity[0].Usage != 1.25 {
				t.Fatal("BYOK must not be added to usage")
			}
		})
	}
}
func TestOpenRouterRequestFailuresAndRedirect(t *testing.T) {
	for _, status := range []int{301, 302, 401, 403, 429, 500} {
		client := newOpenRouterAccountClient()
		calls := 0
		client.Transport = openRouterTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://attacker.invalid"}}, Body: io.NopCloser(strings.NewReader("test-secret")), Request: r}, nil
		})
		var result any
		err := openRouterAccountRequest(context.Background(), client, UpstreamAccountRemoteConfig{AccessToken: "test-secret"}, "/api/v1/credits", &result)
		if err == nil || calls != 1 || strings.Contains(err.Error(), "test-secret") {
			t.Fatalf("status %d: %v calls=%d", status, err, calls)
		}
	}
	client := &http.Client{Transport: openRouterTestTransport(func(r *http.Request) (*http.Response, error) { return nil, errors.New("test-secret") })}
	var result any
	err := openRouterAccountRequest(context.Background(), client, UpstreamAccountRemoteConfig{AccessToken: "test-secret"}, "/api/v1/credits", &result)
	if err == nil || strings.Contains(err.Error(), "test-secret") {
		t.Fatalf("error leakage: %v", err)
	}
}
func TestOpenRouterAccountStorageAndTypeIsolation(t *testing.T) {
	setupUpstreamAccountTestDB(t)
	allowUpstreamAccountTestPrivateFetch(t)
	saved, err := SaveUpstreamAccount(UpstreamAccount{Name: "OpenRouter", AccountType: UpstreamAccountTypeOpenRouter, BaseURL: "https://attacker.invalid", AccessToken: "test-secret", Enabled: true, UserID: 10})
	if err != nil {
		t.Fatal(err)
	}
	account, err := getUpstreamAccountByID(saved.Id)
	if err != nil {
		t.Fatal(err)
	}
	if account.BaseURL != openRouterAccountBaseURL || account.UserID != 0 || account.ResourceDisplayMode != "wallet" || account.AccessTokenEncrypted == "test-secret" || account.AccessTokenEncrypted == "" {
		t.Fatalf("invalid normalization/storage")
	}
	serialized, _ := common.Marshal(saved)
	if strings.Contains(string(serialized), "test-secret") {
		t.Fatal("secret in API")
	}
	before := account.AccessTokenEncrypted
	account.AccessToken = ""
	if _, err := SaveUpstreamAccount(*account); err != nil {
		t.Fatal(err)
	}
	account, _ = getUpstreamAccountByID(saved.Id)
	if account.AccessTokenEncrypted != before {
		t.Fatal("blank secret replaced stored key")
	}
	account.AccountType = UpstreamAccountTypeNewAPI
	account.UserID = 1
	if _, err := SaveUpstreamAccount(*account); err == nil {
		t.Fatal("must not reuse OpenRouter key for new-api")
	}
	account.AccountType = UpstreamAccountTypeOpenRouter
	account.Enabled = false
	if _, err := SaveUpstreamAccount(*account); err != nil {
		t.Fatal(err)
	}
	if _, err := GetOpenRouterAccountFinance(context.Background(), saved.Id); err == nil {
		t.Fatal("disabled account queried upstream")
	}
}
func TestOpenRouterSnapshotDeltaUsesUsageNotBalance(t *testing.T) {
	delta, _ := upstreamAccountRemoteSnapshotDeltaForConfig(UpstreamAccountRemoteConfig{AccountType: UpstreamAccountTypeOpenRouter}, UpstreamAccountSnapshot{WalletQuota: 100, WalletUsedQuota: 20}, UpstreamAccountSnapshot{WalletQuota: 200, WalletUsedQuota: 30})
	if delta != 10 {
		t.Fatalf("recharge changed usage delta: %d", delta)
	}
}
