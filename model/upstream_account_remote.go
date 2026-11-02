package model

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"gorm.io/gorm"
)

// Sentinel errors for i18n translation (remote observer)
var (
	ErrUpstreamAccountRemoteMissingURL      = errors.New("upstream_account:remote_missing_url")
	ErrUpstreamAccountRemoteMissingUID      = errors.New("upstream_account:remote_missing_uid")
	ErrUpstreamAccountRemoteMissingToken    = errors.New("upstream_account:remote_missing_token")
	ErrUpstreamAccountRemoteMissingEmail    = errors.New("upstream_account:remote_missing_email")
	ErrUpstreamAccountRemoteMissingPassword = errors.New("upstream_account:remote_missing_password")
	ErrUpstreamAccountRemoteTokenEmpty      = errors.New("upstream_account:remote_token_empty")
	ErrUpstreamAccountRemoteNotNewAPI       = errors.New("upstream_account:remote_not_newapi")
	ErrUpstreamAccountRemoteURLInvalid      = errors.New("upstream_account:remote_url_invalid")
	ErrUpstreamAccountRemoteRequestFail     = errors.New("upstream_account:remote_request_fail")
	ErrUpstreamAccountRemoteLoginNoToken    = errors.New("upstream_account:remote_login_no_token")
	ErrUpstreamAccountRemoteBalanceInvalid  = errors.New("upstream_account:remote_balance_invalid")
)

const (
	upstreamAccountRemoteObserverStatusDisabled      = "disabled"
	upstreamAccountRemoteObserverStatusNotConfigured = "not_configured"
	upstreamAccountRemoteObserverStatusReady         = "ready"
	upstreamAccountRemoteObserverStatusNeedsBaseline = "needs_baseline"
	upstreamAccountRemoteObserverStatusFailed        = "failed"

	upstreamAccountRemoteSnapshotStatusSuccess = "success"
	upstreamAccountRemoteSnapshotStatusFailed  = "failed"

	upstreamAccountRemoteSyncMinIntervalSeconds = 300
)

type UpstreamAccountSnapshot struct {
	Id                 int     `json:"id"`
	SelectionSignature string  `json:"selection_signature" gorm:"type:varchar(255);index:idx_profit_board_remote_snapshot_combo_time,priority:1;index:idx_profit_board_remote_snapshot_combo_hash_time,priority:1"`
	ComboId            string  `json:"combo_id" gorm:"type:varchar(64);index:idx_profit_board_remote_snapshot_combo_time,priority:2;index:idx_profit_board_remote_snapshot_combo_hash_time,priority:2"`
	ConfigHash         string  `json:"config_hash" gorm:"type:varchar(64);index:idx_profit_board_remote_snapshot_combo_hash_time,priority:3"`
	Status             string  `json:"status" gorm:"type:varchar(16);index"`
	ErrorMessage       string  `json:"error_message,omitempty" gorm:"type:text"`
	RemoteQuotaPerUnit float64 `json:"remote_quota_per_unit" gorm:"type:decimal(18,6);default:0"`
	WalletQuota        int64   `json:"wallet_quota" gorm:"type:bigint;default:0"`
	WalletUsedQuota    int64   `json:"wallet_used_quota" gorm:"type:bigint;default:0"`
	SubscriptionStates string  `json:"subscription_states,omitempty" gorm:"type:text"`
	SyncedAt           int64   `json:"synced_at" gorm:"bigint;index:idx_profit_board_remote_snapshot_combo_time,priority:3"`
	CreatedAt          int64   `json:"created_at" gorm:"bigint;index"`
}

func (UpstreamAccountSnapshot) TableName() string {
	return "profit_board_remote_snapshots"
}

func (s *UpstreamAccountSnapshot) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if s.CreatedAt <= 0 {
		s.CreatedAt = now
	}
	if s.SyncedAt <= 0 {
		s.SyncedAt = now
	}
	return nil
}

type upstreamAccountRemoteStatusData struct {
	QN           string  `json:"_qn"`
	QuotaPerUnit float64 `json:"quota_per_unit"`
}

type upstreamAccountRemoteUserSelfData struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Quota       int64  `json:"quota"`
	UsedQuota   int64  `json:"used_quota"`
}

type upstreamAccountRemoteSubscriptionItem struct {
	Subscription UpstreamAccountSubscriptionSnapshot `json:"subscription"`
}

type upstreamAccountRemoteSubscriptionSelfData struct {
	Subscriptions []upstreamAccountRemoteSubscriptionItem `json:"subscriptions"`
}

type upstreamAccountRemoteEnvelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type upstreamAccountSub2APIEnvelope[T any] struct {
	Success *bool  `json:"success,omitempty"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type upstreamAccountSub2APILoginData struct {
	AccessToken string `json:"access_token"`
}

type upstreamAccountSub2APIProfileData struct {
	Balance json.RawMessage `json:"balance"`
}

type upstreamAccountRemoteFetchResult struct {
	RemoteQuotaPerUnit float64
	WalletQuota        int64
	WalletUsedQuota    int64
	Subscriptions      []UpstreamAccountSubscriptionSnapshot
}

func normalizeUpstreamAccountSubscriptionSnapshot(
	subscription UpstreamAccountSubscriptionSnapshot,
) UpstreamAccountSubscriptionSnapshot {
	if subscription.SubscriptionID <= 0 && subscription.ID > 0 {
		subscription.SubscriptionID = subscription.ID
	}
	if subscription.ID <= 0 && subscription.SubscriptionID > 0 {
		subscription.ID = subscription.SubscriptionID
	}
	return subscription
}

func normalizeUpstreamAccountRemoteConfig(config UpstreamAccountRemoteConfig) UpstreamAccountRemoteConfig {
	config.AccountType = strings.ToLower(strings.TrimSpace(config.AccountType))
	if config.AccountType == "" {
		config.AccountType = UpstreamAccountTypeNewAPI
	}
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	config.AccessToken = strings.TrimSpace(config.AccessToken)
	config.AccessTokenMasked = strings.TrimSpace(config.AccessTokenMasked)
	config.AccessTokenEncrypted = strings.TrimSpace(config.AccessTokenEncrypted)
	config.Email = strings.TrimSpace(config.Email)
	config.Password = strings.TrimSpace(config.Password)
	config.PasswordMasked = strings.TrimSpace(config.PasswordMasked)
	config.PasswordEncrypted = strings.TrimSpace(config.PasswordEncrypted)
	if config.UserID < 0 {
		config.UserID = 0
	}
	if config.AccountType == UpstreamAccountTypeOpenRouter {
		config.BaseURL = openRouterAccountBaseURL
		config.UserID = 0
	}
	if config.AccountType == UpstreamAccountTypeSub2API {
		config.UserID = 0
		config.AccessToken = ""
		config.AccessTokenMasked = ""
		config.AccessTokenEncrypted = ""
	}
	return config
}

func upstreamAccountRemoteObserverConfigured(config UpstreamAccountRemoteConfig) bool {
	config = normalizeUpstreamAccountRemoteConfig(config)
	switch config.AccountType {
	case UpstreamAccountTypeOpenRouter:
		return config.AccessToken != "" || config.AccessTokenEncrypted != ""
	case UpstreamAccountTypeSub2API:
		return config.BaseURL != "" && config.Email != "" &&
			(config.Password != "" || config.PasswordEncrypted != "")
	default:
		return config.BaseURL != "" && config.UserID > 0 &&
			(config.AccessToken != "" || config.AccessTokenEncrypted != "")
	}
}

func validateUpstreamAccountRemoteConfig(config UpstreamAccountRemoteConfig) error {
	config = normalizeUpstreamAccountRemoteConfig(config)
	if !config.Enabled {
		return nil
	}
	if config.BaseURL == "" {
		return ErrUpstreamAccountRemoteMissingURL
	}
	switch config.AccountType {
	case UpstreamAccountTypeSub2API:
		if config.Email == "" {
			return ErrUpstreamAccountRemoteMissingEmail
		}
		if config.Password == "" && config.PasswordEncrypted == "" {
			return ErrUpstreamAccountRemoteMissingPassword
		}
	case UpstreamAccountTypeNewAPI, UpstreamAccountTypeOpenRouter:
		if config.AccountType == UpstreamAccountTypeNewAPI && config.UserID <= 0 {
			return ErrUpstreamAccountRemoteMissingUID
		}
		if config.AccessToken == "" && config.AccessTokenEncrypted == "" {
			return ErrUpstreamAccountRemoteMissingToken
		}
	default:
		return ErrUpstreamAccountTypeUnsupported
	}
	if err := validateUpstreamAccountRemoteURL(config.BaseURL); err != nil {
		return fmt.Errorf("%w: %v", ErrUpstreamAccountRemoteURLInvalid, err)
	}
	return nil
}

func validateUpstreamAccountRemoteURL(url string) error {
	fetchSetting := system_setting.GetFetchSetting()
	return common.ValidateURLWithFetchSetting(
		url,
		fetchSetting.EnableSSRFProtection,
		fetchSetting.AllowPrivateIp,
		fetchSetting.DomainFilterMode,
		fetchSetting.IpFilterMode,
		fetchSetting.DomainList,
		fetchSetting.IpList,
		fetchSetting.AllowedPorts,
		fetchSetting.ApplyIPFilterForDomain,
	)
}

func upstreamAccountRemoteSecretKey() []byte {
	sum := sha256.Sum256([]byte(common.CryptoSecret))
	return sum[:]
}

func encryptUpstreamAccountRemoteSecret(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	block, err := aes.NewCipher(upstreamAccountRemoteSecretKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	cipherText := gcm.Seal(nonce, nonce, []byte(raw), nil)
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

func decryptUpstreamAccountRemoteSecret(cipherText string) (string, error) {
	cipherText = strings.TrimSpace(cipherText)
	if cipherText == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(upstreamAccountRemoteSecretKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("cipher text too short")
	}
	nonce := raw[:gcm.NonceSize()]
	data := raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func maskUpstreamAccountRemoteSecret(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= 8 {
		return "****"
	}
	return fmt.Sprintf("%s****%s", raw[:4], raw[len(raw)-4:])
}

func upstreamAccountRemoteObserverConfigHash(config UpstreamAccountRemoteConfig) string {
	config = normalizeUpstreamAccountRemoteConfig(config)
	if config.AccountType == UpstreamAccountTypeSub2API {
		password := strings.TrimSpace(config.Password)
		if password == "" && config.PasswordEncrypted != "" {
			if decrypted, err := decryptUpstreamAccountRemoteSecret(config.PasswordEncrypted); err == nil {
				password = strings.TrimSpace(decrypted)
			}
		}
		if password == "" {
			return ""
		}
		return common.GenerateHMAC(strings.ToLower(config.BaseURL) + "|" + UpstreamAccountTypeSub2API + "|" + strings.ToLower(config.Email) + "|" + password)
	}
	token := strings.TrimSpace(config.AccessToken)
	if token == "" && config.AccessTokenEncrypted != "" {
		if decrypted, err := decryptUpstreamAccountRemoteSecret(config.AccessTokenEncrypted); err == nil {
			token = strings.TrimSpace(decrypted)
		}
	}
	if token == "" {
		return ""
	}
	return common.GenerateHMAC(strings.ToLower(config.BaseURL) + "|" + fmt.Sprintf("%d", config.UserID) + "|" + token)
}

func upstreamAccountQuotaToUSD(quota int64) float64 {
	if quota <= 0 || common.QuotaPerUnit <= 0 {
		return 0
	}
	return roundUpstreamAccountAmount(float64(quota) / common.QuotaPerUnit)
}

func upstreamAccountUSDToQuota(usd float64) int64 {
	if usd <= 0 || common.QuotaPerUnit <= 0 {
		return 0
	}
	return int64(math.Round(usd * common.QuotaPerUnit))
}

func newUpstreamAccountRemoteHTTPClient() *http.Client {
	transport := &http.Transport{}
	if common.TLSInsecureSkipVerify {
		transport.TLSClientConfig = common.InsecureTLSConfig
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			fetchSetting := system_setting.GetFetchSetting()
			if err := common.ValidateURLWithFetchSetting(
				req.URL.String(),
				fetchSetting.EnableSSRFProtection,
				fetchSetting.AllowPrivateIp,
				fetchSetting.DomainFilterMode,
				fetchSetting.IpFilterMode,
				fetchSetting.DomainList,
				fetchSetting.IpList,
				fetchSetting.AllowedPorts,
				fetchSetting.ApplyIPFilterForDomain,
			); err != nil {
				return err
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
	if common.RelayTimeout > 0 {
		client.Timeout = time.Duration(common.RelayTimeout) * time.Second
	} else {
		client.Timeout = 30 * time.Second
	}
	return client
}

func upstreamAccountRemoteRequest[T any](client *http.Client, remoteConfig UpstreamAccountRemoteConfig, path string, target *T) error {
	token := strings.TrimSpace(remoteConfig.AccessToken)
	if token == "" && remoteConfig.AccessTokenEncrypted != "" {
		decrypted, err := decryptUpstreamAccountRemoteSecret(remoteConfig.AccessTokenEncrypted)
		if err != nil {
			return err
		}
		token = strings.TrimSpace(decrypted)
	}
	if token == "" {
		return ErrUpstreamAccountRemoteTokenEmpty
	}
	url := strings.TrimRight(remoteConfig.BaseURL, "/") + path
	if err := validateUpstreamAccountRemoteURL(url); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("New-Api-User", fmt.Sprintf("%d", remoteConfig.UserID))
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w (%d): %s", ErrUpstreamAccountRemoteRequestFail, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	envelope := upstreamAccountRemoteEnvelope[T]{}
	if err := common.Unmarshal(body, &envelope); err != nil {
		return err
	}
	if !envelope.Success {
		if envelope.Message == "" {
			envelope.Message = "远端返回失败"
		}
		return errors.New(envelope.Message)
	}
	*target = envelope.Data
	return nil
}

func buildUpstreamAccountSub2APIURL(baseURL string, path string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(strings.ToLower(baseURL), "/api/v1") && strings.HasPrefix(path, "/api/v1/") {
		return baseURL + strings.TrimPrefix(path, "/api/v1")
	}
	return baseURL + path
}

func upstreamAccountSub2APIRequest[T any](client *http.Client, remoteConfig UpstreamAccountRemoteConfig, method string, path string, body any, token string, target *T) error {
	url := buildUpstreamAccountSub2APIURL(remoteConfig.BaseURL, path)
	if err := validateUpstreamAccountRemoteURL(url); err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		payload, err := common.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(token) != "" {
		token = strings.TrimSpace(token)
		if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = "Bearer " + token
		}
		req.Header.Set("Authorization", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w (%d): %s", ErrUpstreamAccountRemoteRequestFail, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	envelope := upstreamAccountSub2APIEnvelope[T]{}
	if err := common.Unmarshal(respBody, &envelope); err != nil {
		return err
	}
	if envelope.Success != nil && !*envelope.Success {
		if envelope.Message == "" {
			envelope.Message = "远端返回失败"
		}
		return errors.New(envelope.Message)
	}
	*target = envelope.Data
	return nil
}

func decryptUpstreamAccountRemotePassword(config UpstreamAccountRemoteConfig) (string, error) {
	password := strings.TrimSpace(config.Password)
	if password != "" {
		return password, nil
	}
	if config.PasswordEncrypted == "" {
		return "", ErrUpstreamAccountRemoteMissingPassword
	}
	decrypted, err := decryptUpstreamAccountRemoteSecret(config.PasswordEncrypted)
	if err != nil {
		return "", err
	}
	password = strings.TrimSpace(decrypted)
	if password == "" {
		return "", ErrUpstreamAccountRemoteMissingPassword
	}
	return password, nil
}

func parseUpstreamAccountSub2APIBalance(raw json.RawMessage) (float64, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return 0, ErrUpstreamAccountRemoteBalanceInvalid
	}
	var numeric float64
	if err := common.Unmarshal(raw, &numeric); err == nil {
		if numeric < 0 || math.IsNaN(numeric) || math.IsInf(numeric, 0) {
			return 0, ErrUpstreamAccountRemoteBalanceInvalid
		}
		return numeric, nil
	}
	var text string
	if err := common.Unmarshal(raw, &text); err != nil {
		return 0, ErrUpstreamAccountRemoteBalanceInvalid
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, ErrUpstreamAccountRemoteBalanceInvalid
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, ErrUpstreamAccountRemoteBalanceInvalid
	}
	return value, nil
}

func fetchUpstreamAccountSub2APIObserver(client *http.Client, remoteConfig UpstreamAccountRemoteConfig) (*upstreamAccountRemoteFetchResult, error) {
	password, err := decryptUpstreamAccountRemotePassword(remoteConfig)
	if err != nil {
		return nil, err
	}
	loginData := upstreamAccountSub2APILoginData{}
	if err := upstreamAccountSub2APIRequest(
		client,
		remoteConfig,
		http.MethodPost,
		"/api/v1/auth/login",
		map[string]string{
			"email":    remoteConfig.Email,
			"password": password,
		},
		"",
		&loginData,
	); err != nil {
		return nil, err
	}
	token := strings.TrimSpace(loginData.AccessToken)
	if token == "" {
		return nil, ErrUpstreamAccountRemoteLoginNoToken
	}
	profileData := upstreamAccountSub2APIProfileData{}
	if err := upstreamAccountSub2APIRequest(
		client,
		remoteConfig,
		http.MethodGet,
		"/api/v1/user/profile",
		nil,
		token,
		&profileData,
	); err != nil {
		return nil, err
	}
	balanceUSD, err := parseUpstreamAccountSub2APIBalance(profileData.Balance)
	if err != nil {
		return nil, err
	}
	return &upstreamAccountRemoteFetchResult{
		RemoteQuotaPerUnit: common.QuotaPerUnit,
		WalletQuota:        upstreamAccountUSDToQuota(balanceUSD),
		WalletUsedQuota:    0,
		Subscriptions:      []UpstreamAccountSubscriptionSnapshot{},
	}, nil
}

func fetchUpstreamAccountRemoteObserver(remoteConfig UpstreamAccountRemoteConfig) (*upstreamAccountRemoteFetchResult, error) {
	if err := validateUpstreamAccountRemoteConfig(remoteConfig); err != nil {
		return nil, err
	}
	client := newUpstreamAccountRemoteHTTPClient()
	remoteConfig = normalizeUpstreamAccountRemoteConfig(remoteConfig)
	if remoteConfig.AccountType == UpstreamAccountTypeOpenRouter {
		return fetchOpenRouterAccountSnapshot(remoteConfig)
	}
	if remoteConfig.AccountType == UpstreamAccountTypeSub2API {
		return fetchUpstreamAccountSub2APIObserver(client, remoteConfig)
	}

	statusData := upstreamAccountRemoteStatusData{}
	if err := upstreamAccountRemoteRequest(client, remoteConfig, "/api/status", &statusData); err != nil {
		return nil, err
	}
	qn := strings.TrimSpace(statusData.QN)
	if qn != "new-api" && !(qn == "" && statusData.QuotaPerUnit > 0) {
		return nil, ErrUpstreamAccountRemoteNotNewAPI
	}

	selfData := upstreamAccountRemoteUserSelfData{}
	if err := upstreamAccountRemoteRequest(client, remoteConfig, "/api/user/self", &selfData); err != nil {
		return nil, err
	}

	subData := upstreamAccountRemoteSubscriptionSelfData{}
	// subscription 端点在部分 new-api 改编版中不存在，失败时降级为空
	_ = upstreamAccountRemoteRequest(client, remoteConfig, "/api/subscription/self", &subData)

	subscriptions := make([]UpstreamAccountSubscriptionSnapshot, 0, len(subData.Subscriptions))
	for _, item := range subData.Subscriptions {
		subscription := normalizeUpstreamAccountSubscriptionSnapshot(item.Subscription)
		if subscription.SubscriptionID <= 0 {
			continue
		}
		subscriptions = append(subscriptions, subscription)
	}
	sort.Slice(subscriptions, func(i, j int) bool {
		if subscriptions[i].EndTime == subscriptions[j].EndTime {
			return subscriptions[i].SubscriptionID < subscriptions[j].SubscriptionID
		}
		return subscriptions[i].EndTime < subscriptions[j].EndTime
	})

	return &upstreamAccountRemoteFetchResult{
		RemoteQuotaPerUnit: statusData.QuotaPerUnit,
		WalletQuota:        selfData.Quota,
		WalletUsedQuota:    selfData.UsedQuota,
		Subscriptions:      subscriptions,
	}, nil
}

func buildUpstreamAccountSnapshot(selectionSignature string, comboID string, config UpstreamAccountRemoteConfig, fetchResult *upstreamAccountRemoteFetchResult, status string, err error) UpstreamAccountSnapshot {
	snapshot := UpstreamAccountSnapshot{
		SelectionSignature: selectionSignature,
		ComboId:            comboID,
		ConfigHash:         upstreamAccountRemoteObserverConfigHash(config),
		Status:             status,
		SyncedAt:           common.GetTimestamp(),
		CreatedAt:          common.GetTimestamp(),
	}
	if err != nil {
		snapshot.ErrorMessage = err.Error()
		return snapshot
	}
	if fetchResult == nil {
		return snapshot
	}
	snapshot.RemoteQuotaPerUnit = fetchResult.RemoteQuotaPerUnit
	snapshot.WalletQuota = fetchResult.WalletQuota
	snapshot.WalletUsedQuota = fetchResult.WalletUsedQuota
	if payload, marshalErr := common.Marshal(fetchResult.Subscriptions); marshalErr == nil {
		snapshot.SubscriptionStates = string(payload)
	}
	return snapshot
}

func parseUpstreamAccountRemoteSubscriptions(raw string) []UpstreamAccountSubscriptionSnapshot {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []UpstreamAccountSubscriptionSnapshot{}
	}
	subscriptions := make([]UpstreamAccountSubscriptionSnapshot, 0)
	if err := common.UnmarshalJsonStr(raw, &subscriptions); err != nil {
		return []UpstreamAccountSubscriptionSnapshot{}
	}
	for index := range subscriptions {
		subscriptions[index] = normalizeUpstreamAccountSubscriptionSnapshot(
			subscriptions[index],
		)
	}
	return subscriptions
}

func getLatestUpstreamAccountSnapshot(selectionSignature string, comboID string) (*UpstreamAccountSnapshot, error) {
	snapshot := &UpstreamAccountSnapshot{}
	if err := DB.Where("selection_signature = ? AND combo_id = ?", selectionSignature, comboID).
		Order("synced_at desc, id desc").
		First(snapshot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return snapshot, nil
}

func getLatestUpstreamAccountRemoteSuccessSnapshot(selectionSignature string, comboID string, configHash string) (*UpstreamAccountSnapshot, error) {
	snapshot := &UpstreamAccountSnapshot{}
	query := DB.Where("selection_signature = ? AND combo_id = ? AND status = ?", selectionSignature, comboID, upstreamAccountRemoteSnapshotStatusSuccess)
	if configHash != "" {
		query = query.Where("config_hash = ?", configHash)
	}
	if err := query.Order("synced_at desc, id desc").First(snapshot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return snapshot, nil
}

func countUpstreamAccountRemoteSuccessSnapshots(selectionSignature string, comboID string, configHash string) (int64, error) {
	var count int64
	query := DB.Model(&UpstreamAccountSnapshot{}).
		Where("selection_signature = ? AND combo_id = ? AND status = ?", selectionSignature, comboID, upstreamAccountRemoteSnapshotStatusSuccess)
	if configHash != "" {
		query = query.Where("config_hash = ?", configHash)
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func listUpstreamAccountRemoteSuccessSnapshots(selectionSignature string, comboID string, configHash string, startTimestamp int64, endTimestamp int64) ([]UpstreamAccountSnapshot, error) {
	snapshots := make([]UpstreamAccountSnapshot, 0)
	if configHash == "" {
		return snapshots, nil
	}
	if startTimestamp > 0 {
		previous := UpstreamAccountSnapshot{}
		if err := DB.Where("selection_signature = ? AND combo_id = ? AND config_hash = ? AND status = ? AND synced_at < ?",
			selectionSignature, comboID, configHash, upstreamAccountRemoteSnapshotStatusSuccess, startTimestamp).
			Order("synced_at desc, id desc").
			Limit(1).
			Find(&previous).Error; err != nil {
			return nil, err
		}
		if previous.Id > 0 {
			snapshots = append(snapshots, previous)
		}
	}
	current := make([]UpstreamAccountSnapshot, 0)
	query := DB.Where("selection_signature = ? AND combo_id = ? AND config_hash = ? AND status = ?",
		selectionSignature, comboID, configHash, upstreamAccountRemoteSnapshotStatusSuccess)
	if startTimestamp > 0 {
		query = query.Where("synced_at >= ?", startTimestamp)
	}
	if endTimestamp > 0 {
		query = query.Where("synced_at <= ?", endTimestamp)
	}
	if err := query.Order("synced_at asc, id asc").Find(&current).Error; err != nil {
		return nil, err
	}
	snapshots = append(snapshots, current...)
	return snapshots, nil
}

func upstreamAccountRemoteObserverNeedsSync(latest *UpstreamAccountSnapshot, configHash string, force bool) bool {
	return upstreamAccountRemoteObserverNeedsSyncWithMinInterval(latest, configHash, force, upstreamAccountRemoteSyncMinIntervalSeconds)
}

func upstreamAccountRemoteObserverNeedsSyncWithMinInterval(latest *UpstreamAccountSnapshot, configHash string, force bool, minIntervalSeconds int64) bool {
	if force || latest == nil {
		return true
	}
	if latest.ConfigHash != configHash {
		return true
	}
	if latest.Status != upstreamAccountRemoteSnapshotStatusSuccess {
		return true
	}
	if minIntervalSeconds < 0 {
		minIntervalSeconds = upstreamAccountRemoteSyncMinIntervalSeconds
	}
	return common.GetTimestamp()-latest.SyncedAt >= minIntervalSeconds
}

func syncUpstreamAccountRemoteObserverSnapshot(selectionSignature string, batch UpstreamAccountSnapshotSubject, config UpstreamAccountRemoteConfig, force bool) (*UpstreamAccountSnapshot, *UpstreamAccountSnapshot, error) {
	return syncUpstreamAccountRemoteObserverSnapshotWithMinInterval(selectionSignature, batch, config, force, upstreamAccountRemoteSyncMinIntervalSeconds)
}

func syncUpstreamAccountRemoteObserverSnapshotWithMinInterval(selectionSignature string, batch UpstreamAccountSnapshotSubject, config UpstreamAccountRemoteConfig, force bool, minIntervalSeconds int64) (*UpstreamAccountSnapshot, *UpstreamAccountSnapshot, error) {
	config = normalizeUpstreamAccountRemoteConfig(config)
	latestAny, err := getLatestUpstreamAccountSnapshot(selectionSignature, batch.Id)
	if err != nil {
		return nil, nil, err
	}
	configHash := upstreamAccountRemoteObserverConfigHash(config)
	if !upstreamAccountRemoteObserverNeedsSyncWithMinInterval(latestAny, configHash, force, minIntervalSeconds) {
		latestSuccess, successErr := getLatestUpstreamAccountRemoteSuccessSnapshot(selectionSignature, batch.Id, configHash)
		return latestAny, latestSuccess, successErr
	}

	fetchResult, fetchErr := fetchUpstreamAccountRemoteObserver(config)
	snapshot := buildUpstreamAccountSnapshot(selectionSignature, batch.Id, config, fetchResult, upstreamAccountRemoteSnapshotStatusSuccess, nil)
	if fetchErr != nil {
		snapshot = buildUpstreamAccountSnapshot(selectionSignature, batch.Id, config, nil, upstreamAccountRemoteSnapshotStatusFailed, fetchErr)
	}
	if err := DB.Create(&snapshot).Error; err != nil {
		return nil, nil, err
	}
	latestAny = &snapshot
	latestSuccess, successErr := getLatestUpstreamAccountRemoteSuccessSnapshot(selectionSignature, batch.Id, configHash)
	if successErr != nil {
		return latestAny, nil, successErr
	}
	return latestAny, latestSuccess, nil
}

func summarizeUpstreamAccountRemoteSubscriptions(subscriptions []UpstreamAccountSubscriptionSnapshot) (int64, int64) {
	total := int64(0)
	used := int64(0)
	unlimited := false
	for _, item := range subscriptions {
		used += item.AmountUsed
		if item.AmountTotal <= 0 {
			unlimited = true
			continue
		}
		total += item.AmountTotal
	}
	if unlimited {
		return 0, used
	}
	return total, used
}

func buildUpstreamAccountSnapshotState(selectionSignature string, batch UpstreamAccountSnapshotSubject, config UpstreamAccountRemoteConfig, latestAny *UpstreamAccountSnapshot, latestSuccess *UpstreamAccountSnapshot, periodUsedUSD float64) UpstreamAccountState {
	state := UpstreamAccountState{
		BatchId:       batch.Id,
		BatchName:     batch.Name,
		Enabled:       config.Enabled,
		Configured:    upstreamAccountRemoteObserverConfigured(config),
		Status:        upstreamAccountRemoteObserverStatusDisabled,
		PeriodUsedUSD: roundUpstreamAccountAmount(periodUsedUSD),
	}
	switch {
	case !config.Enabled:
		state.Status = upstreamAccountRemoteObserverStatusDisabled
		return state
	case !state.Configured:
		state.Status = upstreamAccountRemoteObserverStatusNotConfigured
		return state
	}
	if latestAny != nil {
		state.LastSyncedAt = latestAny.SyncedAt
		if latestAny.Status == upstreamAccountRemoteSnapshotStatusFailed {
			state.Status = upstreamAccountRemoteObserverStatusFailed
			state.ErrorMessage = latestAny.ErrorMessage
		}
	}
	if latestSuccess != nil {
		state.LastSuccessAt = latestSuccess.SyncedAt
		state.RemoteQuotaPerUnit = latestSuccess.RemoteQuotaPerUnit
		state.QuotaPerUnitMismatch = latestSuccess.RemoteQuotaPerUnit > 0 && latestSuccess.RemoteQuotaPerUnit != common.QuotaPerUnit
		state.WalletBalanceUSD = upstreamAccountQuotaToUSD(latestSuccess.WalletQuota)
		if config.AccountType == UpstreamAccountTypeOpenRouter && common.QuotaPerUnit > 0 {
			state.WalletBalanceUSD = roundUpstreamAccountAmount(float64(latestSuccess.WalletQuota) / common.QuotaPerUnit)
		}
		state.WalletQuotaUSD = state.WalletBalanceUSD
		state.WalletUsedTotalUSD = upstreamAccountQuotaToUSD(latestSuccess.WalletUsedQuota)
		state.WalletUsedQuotaUSD = state.WalletUsedTotalUSD
		subSummary := summarizeUpstreamAccountSubscriptions(parseUpstreamAccountRemoteSubscriptions(latestSuccess.SubscriptionStates))
		state.SubscriptionRemainingUSD = subSummary.RemainingUSD
		state.SubscriptionTotalQuotaUSD = subSummary.TotalUSD
		state.SubscriptionUsedQuotaUSD = subSummary.UsedUSD
		state.SubscriptionCount = subSummary.Count
		state.SubscriptionEarliestExpireAt = subSummary.EarliestExpireAt
		state.HasSubscriptionData = subSummary.HasData
		state.SubscriptionHasUnlimited = subSummary.HasUnlimited
		if count, err := countUpstreamAccountRemoteSuccessSnapshots(selectionSignature, batch.Id, latestSuccess.ConfigHash); err == nil {
			state.SnapshotCount = int(count)
			state.BaselineReady = count >= 2
		}
		if state.Status != upstreamAccountRemoteObserverStatusFailed {
			if state.BaselineReady {
				state.Status = upstreamAccountRemoteObserverStatusReady
			} else {
				state.Status = upstreamAccountRemoteObserverStatusNeedsBaseline
			}
		}
	}
	if latestAny == nil && latestSuccess == nil {
		state.Status = upstreamAccountRemoteObserverStatusNeedsBaseline
	}
	return state
}

func upstreamAccountRemoteSubscriptionDelta(prev UpstreamAccountSubscriptionSnapshot, curr UpstreamAccountSubscriptionSnapshot) (int64, bool) {
	if curr.AmountUsed >= prev.AmountUsed {
		return curr.AmountUsed - prev.AmountUsed, false
	}
	resetDetected := curr.LastResetTime > prev.LastResetTime ||
		(curr.NextResetTime > 0 && curr.NextResetTime != prev.NextResetTime) ||
		(curr.EndTime > 0 && curr.EndTime > prev.EndTime) ||
		(curr.AmountTotal > 0 && curr.AmountTotal != prev.AmountTotal)
	if resetDetected {
		return curr.AmountUsed, false
	}
	return 0, true
}

func upstreamAccountRemoteSnapshotDelta(prev UpstreamAccountSnapshot, curr UpstreamAccountSnapshot) (int64, []string) {
	warnings := make([]string, 0)
	totalDelta := int64(0)
	if curr.WalletUsedQuota >= prev.WalletUsedQuota {
		totalDelta += curr.WalletUsedQuota - prev.WalletUsedQuota
	} else {
		warnings = append(warnings, "远端钱包已用额度出现回退，当前时间段的钱包观测用量已按 0 处理")
	}
	return totalDelta, warnings
}

func upstreamAccountRemoteSnapshotDeltaForConfig(config UpstreamAccountRemoteConfig, prev UpstreamAccountSnapshot, curr UpstreamAccountSnapshot) (int64, []string) {
	config = normalizeUpstreamAccountRemoteConfig(config)
	if config.AccountType != UpstreamAccountTypeSub2API {
		return upstreamAccountRemoteSnapshotDelta(prev, curr)
	}
	if prev.WalletQuota >= curr.WalletQuota {
		return prev.WalletQuota - curr.WalletQuota, nil
	}
	return 0, []string{"远端钱包余额上升，可能因为充值或额度重置，当前时间段的钱包观测用量已按 0 处理"}
}

func uniqueUpstreamAccountWarnings(items []string) []string {
	if len(items) == 0 {
		return items
	}
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
