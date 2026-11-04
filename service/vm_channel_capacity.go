package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/google/uuid"
)

var ErrVMCapacityUnavailable = errors.New("VM channel capacity unavailable")

// VMCapacitySnapshot contains no credentials or account identifiers. A snapshot is not a reservation.
type VMCapacitySnapshot struct {
	SchemaVersion           int       `json:"schema_version"`
	Model                   string    `json:"model"`
	ObservedAt              time.Time `json:"observed_at"`
	MaxAgeMs                int       `json:"max_age_ms"`
	StateQuality            string    `json:"state_quality"`
	EffectiveAdmissionLimit int       `json:"effective_admission_limit"`
	AvailableNow            int       `json:"available_now"`
	KeyInflight             int       `json:"key_inflight"`
}

type vmCapacityEntry struct {
	Snapshot  VMCapacitySnapshot `json:"snapshot"`
	ExpiresAt time.Time          `json:"expires_at"`
	Failed    bool               `json:"failed"`
}

type VMCapacityStatus struct {
	Model    string              `json:"model"`
	State    string              `json:"state"`
	AgeMs    int64               `json:"age_ms"`
	Snapshot *VMCapacitySnapshot `json:"snapshot,omitempty"`
}

// PeekVMChannelCapacity is read-only: opening the admin page must not trigger upstream probes.
func PeekVMChannelCapacity(ctx context.Context, ch *model.Channel, requestedModel string) VMCapacityStatus {
	modelName := ResolveChannelUpstreamModel(ch, requestedModel)
	status := VMCapacityStatus{Model: modelName, State: "unknown"}
	resource, key := vmCapacityKeys(ch, modelName)
	var entry vmCapacityEntry
	if common.RedisEnabled {
		if common.RDB == nil {
			return status
		}
		ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		defer cancel()
		if n, err := common.RDB.Exists(ctx, resource+":busy").Result(); err != nil {
			return status
		} else if n > 0 {
			status.State = "busy"
			return status
		}
		raw, err := common.RDB.Get(ctx, key).Bytes()
		if err != nil || common.Unmarshal(raw, &entry) != nil {
			return status
		}
	} else {
		vmCapacityLocal.Lock()
		entry = vmCapacityLocal.entries[key]
		busy := time.Now().Before(vmCapacityLocal.nextQuery[resource+":busy"])
		vmCapacityLocal.Unlock()
		if busy {
			status.State = "busy"
			return status
		}
	}
	if entry.Failed {
		status.State = "unavailable"
		return status
	}
	if entry.Snapshot.ObservedAt.IsZero() {
		return status
	}
	status.AgeMs = max(0, time.Since(entry.Snapshot.ObservedAt).Milliseconds())
	status.State = "stale"
	if validVMCapacity(entry.Snapshot, time.Now()) && time.Now().Before(entry.ExpiresAt) {
		status.State = entry.Snapshot.StateQuality
		status.Snapshot = &entry.Snapshot
	}
	return status
}

var vmCapacityLocal = struct {
	sync.Mutex
	entries    map[string]vmCapacityEntry
	refreshing map[string]bool
	nextQuery  map[string]time.Time
}{entries: map[string]vmCapacityEntry{}, refreshing: map[string]bool{}, nextQuery: map[string]time.Time{}}

func VMCapacityEnabled(ch *model.Channel) bool {
	return ch != nil && ch.GetSetting().UpstreamCapacity.Enabled
}

func vmCapacityURL(ch *model.Channel, modelName string) (string, error) {
	u, err := url.Parse(strings.TrimRight(ch.GetBaseURL(), "/"))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", ErrVMCapacityUnavailable
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1") + "/v1/channel/capacity"
	u.RawPath = ""
	u.RawQuery = url.Values{"model": {modelName}}.Encode()
	return u.String(), nil
}

// Fingerprints invalidate cache on credential, proxy or mapping changes, without storing secrets in Redis keys.
func vmCapacityKeys(ch *model.Channel, modelName string) (string, string) {
	resource := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSuffix(strings.TrimRight(ch.GetBaseURL(), "/"), "/v1")+"\x00"+ch.Key)))
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s", ch.Id, resource, ch.GetSetting().Proxy, ch.GetModelMapping(), modelName))))
	return "channel:vm-capacity:" + resource, "channel:vm-capacity:snapshot:" + fingerprint
}

// ValidateVMCapacityChannel runs against resolved channel settings (including retained keys).
func ValidateVMCapacityChannel(ch *model.Channel) error {
	if !VMCapacityEnabled(ch) {
		return nil
	}
	if !operation_setting.GetChannelConcurrencySetting().Enabled {
		return errors.New("VM capacity requires channel concurrency control")
	}
	if ch.Type != constant.ChannelTypeAnthropic || ch.ChannelInfo.IsMultiKey || len(ch.GetKeys()) != 1 || strings.TrimSpace(ch.Key) == "" {
		return errors.New("VM capacity requires a single-key Claude channel")
	}
	if _, err := vmCapacityURL(ch, "claude"); err != nil {
		return errors.New("VM capacity requires a valid channel Base URL")
	}
	if model.DB != nil {
		channels, err := model.GetAllChannels(0, 0, true, false)
		if err != nil {
			return ErrVMCapacityUnavailable
		}
		resource, _ := vmCapacityKeys(ch, "")
		for _, other := range channels {
			if other.Id == ch.Id || other.Status != common.ChannelStatusEnabled {
				continue
			}
			otherResource, _ := vmCapacityKeys(other, "")
			if otherResource == resource {
				return errors.New("VM capacity requires one channel per upstream key")
			}
		}
	}
	return nil
}

func validVMCapacity(s VMCapacitySnapshot, now time.Time) bool {
	age := now.Sub(s.ObservedAt)
	return s.SchemaVersion == 1 && s.Model != "" && (s.StateQuality == "known" || s.StateQuality == "partial") &&
		s.MaxAgeMs > 0 && s.MaxAgeMs <= 1000 && age >= -200*time.Millisecond && age < time.Duration(s.MaxAgeMs)*time.Millisecond &&
		s.EffectiveAdmissionLimit >= 0 && s.AvailableNow >= 0 && s.AvailableNow <= s.EffectiveAdmissionLimit && s.KeyInflight >= 0
}

func fetchVMCapacity(ctx context.Context, ch *model.Channel, modelName string) (VMCapacitySnapshot, error) {
	var s VMCapacitySnapshot
	if err := ValidateVMCapacityChannel(ch); err != nil {
		return s, ErrVMCapacityUnavailable
	}
	endpoint, err := vmCapacityURL(ch, modelName)
	if err != nil {
		return s, ErrVMCapacityUnavailable
	}
	base, err := GetHttpClientWithProxy(ch.GetSetting().Proxy)
	if err != nil || base == nil {
		return s, ErrVMCapacityUnavailable
	}
	client := *base
	client.Timeout = 750 * time.Millisecond
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return s, ErrVMCapacityUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+ch.Key)
	resp, err := client.Do(req)
	if err != nil {
		return s, ErrVMCapacityUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return s, ErrVMCapacityUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32769))
	if err != nil || len(body) > 32768 {
		return s, ErrVMCapacityUnavailable
	}
	var fields map[string]interface{}
	if common.Unmarshal(body, &fields) != nil {
		return s, ErrVMCapacityUnavailable
	}
	for _, name := range []string{"schema_version", "model", "observed_at", "max_age_ms", "state_quality", "effective_admission_limit", "available_now", "key_inflight"} {
		if fields[name] == nil {
			return s, ErrVMCapacityUnavailable
		}
	}
	if common.Unmarshal(body, &s) != nil || !validVMCapacity(s, time.Now()) {
		return VMCapacitySnapshot{}, ErrVMCapacityUnavailable
	}
	// The VM may canonicalize a supported alias; admission remains scoped to the queried model.
	return s, nil
}

// GetVMChannelCapacity coalesces demand-driven refreshes. Redis mode never fails open to local state.
func GetVMChannelCapacity(ctx context.Context, ch *model.Channel, requestedModel string) (VMCapacitySnapshot, error) {
	modelName := ResolveChannelUpstreamModel(ch, requestedModel)
	resource, key := vmCapacityKeys(ch, modelName)
	now := time.Now()
	if common.RedisEnabled {
		if common.RDB == nil {
			return VMCapacitySnapshot{}, ErrVMCapacityUnavailable
		}
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if n, err := common.RDB.Exists(ctx, resource+":busy").Result(); err != nil || n > 0 {
			return VMCapacitySnapshot{}, ErrVMCapacityUnavailable
		}
		if raw, err := common.RDB.Get(ctx, key).Bytes(); err == nil {
			var entry vmCapacityEntry
			if common.Unmarshal(raw, &entry) == nil && now.Before(entry.ExpiresAt) {
				if entry.Failed {
					return entry.Snapshot, ErrVMCapacityUnavailable
				}
				if validVMCapacity(entry.Snapshot, now) {
					return entry.Snapshot, nil
				}
			}
		}
		token := uuid.NewString()
		locked, err := common.RDB.SetNX(ctx, key+":refresh", token, 2*time.Second).Result()
		if err != nil || !locked {
			return VMCapacitySnapshot{}, ErrVMCapacityUnavailable
		}
		defer func() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()
			common.RDB.Eval(releaseCtx, `if redis.call('GET',KEYS[1]) == ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`, []string{key + ":refresh"}, token)
		}()
		allowed, err := common.RDB.SetNX(ctx, resource+":rate", "1", 150*time.Millisecond).Result()
		if err != nil || !allowed {
			return VMCapacitySnapshot{}, ErrVMCapacityUnavailable
		}
		snapshot, fetchErr := fetchVMCapacity(ctx, ch, modelName)
		entry := makeVMCapacityEntry(snapshot, fetchErr)
		raw, _ := common.Marshal(entry)
		// Publish only while still owning the refresh lock; an expired owner cannot overwrite newer state.
		published, err := common.RDB.Eval(ctx, `if redis.call('GET',KEYS[1]) ~= ARGV[1] then return 0 end redis.call('PSETEX',KEYS[2],ARGV[2],ARGV[3]); return 1`, []string{key + ":refresh", key}, token, max(1, time.Until(entry.ExpiresAt).Milliseconds()), raw).Int()
		if err != nil || published != 1 {
			return VMCapacitySnapshot{}, ErrVMCapacityUnavailable
		}
		return snapshot, fetchErr
	}
	vmCapacityLocal.Lock()
	if now.Before(vmCapacityLocal.nextQuery[resource+":busy"]) {
		vmCapacityLocal.Unlock()
		return VMCapacitySnapshot{}, ErrVMCapacityUnavailable
	}
	if entry, ok := vmCapacityLocal.entries[key]; ok && now.Before(entry.ExpiresAt) {
		vmCapacityLocal.Unlock()
		if entry.Failed || !validVMCapacity(entry.Snapshot, now) {
			return entry.Snapshot, ErrVMCapacityUnavailable
		}
		return entry.Snapshot, nil
	}
	if vmCapacityLocal.refreshing[key] || now.Before(vmCapacityLocal.nextQuery[resource]) {
		vmCapacityLocal.Unlock()
		return VMCapacitySnapshot{}, ErrVMCapacityUnavailable
	}
	// Bound retained model/config generations. Expired entries have no admission authority.
	if len(vmCapacityLocal.entries) >= 1024 {
		for k, e := range vmCapacityLocal.entries {
			if !now.Before(e.ExpiresAt) {
				delete(vmCapacityLocal.entries, k)
			}
		}
		for k, t := range vmCapacityLocal.nextQuery {
			if !now.Before(t) {
				delete(vmCapacityLocal.nextQuery, k)
			}
		}
		if len(vmCapacityLocal.entries) >= 1024 {
			vmCapacityLocal.Unlock()
			return VMCapacitySnapshot{}, ErrVMCapacityUnavailable
		}
	}
	vmCapacityLocal.refreshing[key] = true
	vmCapacityLocal.nextQuery[resource] = now.Add(150 * time.Millisecond)
	vmCapacityLocal.Unlock()
	snapshot, err := fetchVMCapacity(ctx, ch, modelName)
	vmCapacityLocal.Lock()
	delete(vmCapacityLocal.refreshing, key)
	vmCapacityLocal.entries[key] = makeVMCapacityEntry(snapshot, err)
	vmCapacityLocal.Unlock()
	return snapshot, err
}

func makeVMCapacityEntry(s VMCapacitySnapshot, err error) vmCapacityEntry {
	if err != nil {
		return vmCapacityEntry{Failed: true, ExpiresAt: time.Now().Add(time.Second)}
	}
	return vmCapacityEntry{Snapshot: s, ExpiresAt: s.ObservedAt.Add(time.Duration(s.MaxAgeMs) * time.Millisecond)}
}

// HoldVMChannelCapacity absorbs a confirmed pre-execution refusal, not a provider fault.
func HoldVMChannelCapacity(ch *model.Channel, retryAfter time.Duration) {
	resource, _ := vmCapacityKeys(ch, "")
	delay := max(time.Second, min(5*time.Second, retryAfter))
	if common.RedisEnabled {
		if common.RDB != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			common.RDB.Eval(ctx, `if redis.call('PTTL',KEYS[1]) < tonumber(ARGV[1]) then redis.call('PSETEX',KEYS[1],ARGV[1],'1') end return 1`, []string{resource + ":busy"}, delay.Milliseconds())
		}
		return
	}
	vmCapacityLocal.Lock()
	defer vmCapacityLocal.Unlock()
	until := time.Now().Add(delay)
	if until.After(vmCapacityLocal.nextQuery[resource+":busy"]) {
		vmCapacityLocal.nextQuery[resource+":busy"] = until
	}
}
