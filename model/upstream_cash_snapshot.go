package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const cashSnapshotLimit = 128

type cashSnapshotKey struct {
	account    int
	start, end string
}
type cashSnapshotEntry struct {
	identity           string
	data               *CashReport
	attempted, touched time.Time
	refreshing         bool
	err                string
}
type cashReportCache struct {
	mu      sync.Mutex
	entries map[cashSnapshotKey]*cashSnapshotEntry
	slots   chan struct{}
	collect func(context.Context, int, string, string) (*CashReport, error)
}

func newCashReportCache(collect func(context.Context, int, string, string) (*CashReport, error)) *cashReportCache {
	return &cashReportCache{entries: make(map[cashSnapshotKey]*cashSnapshotEntry), slots: make(chan struct{}, 2), collect: collect}
}

var cashReportSnapshots = newCashReportCache(GetCashReport)
var cashSnapshotOnce sync.Once

func cashSnapshotIdentity(a UpstreamAccount) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%t:%s", a.AccountType, a.Enabled, a.AccessTokenEncrypted))))
}

// Registering a range never starts a calculation in the HTTP request path.
func (cache *cashReportCache) read(a UpstreamAccount, start, end string) *CashReport {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	key := cashSnapshotKey{a.Id, start, end}
	identity := cashSnapshotIdentity(a)
	entry := cache.entries[key]
	if entry == nil || entry.identity != identity {
		if entry == nil && len(cache.entries) >= cashSnapshotLimit {
			var oldest cashSnapshotKey
			var oldestTime time.Time
			for k, e := range cache.entries {
				if !e.refreshing && (oldestTime.IsZero() || e.touched.Before(oldestTime)) {
					oldest, oldestTime = k, e.touched
				}
			}
			if !oldestTime.IsZero() {
				delete(cache.entries, oldest)
			}
		}
		entry = &cashSnapshotEntry{identity: identity}
		cache.entries[key] = entry
	}
	entry.touched = time.Now()
	result := &CashReport{AccountID: a.Id, StartDate: start, EndDate: end}
	if entry.data != nil {
		*result = *entry.data
	}
	result.Refreshing = entry.refreshing || (entry.data == nil && entry.err == "")
	result.RefreshError = entry.err
	return result
}
func GetCashReportSnapshot(id int, startDate, endDate string) (*CashReport, error) {
	start, end, err := cashReportRange(startDate, endDate, common.GetTimestamp())
	if err != nil {
		return nil, err
	}
	account, err := cashAccountForRead(id)
	if err != nil {
		return nil, err
	}
	return cashReportSnapshots.read(*account, start.Format("2006-01-02"), end.AddDate(0, 0, -1).Format("2006-01-02")), nil
}

func (cache *cashReportCache) dispatch(accounts []UpstreamAccount, now time.Time) {
	valid := make(map[int]string)
	for _, a := range accounts {
		if a.AccountType != UpstreamAccountTypeOpenRouter {
			continue
		}
		valid[a.Id] = cashSnapshotIdentity(a)
		end := now.UTC().Truncate(24 * time.Hour)
		for _, days := range []int{7, 30} {
			cache.read(a, end.AddDate(0, 0, -days).Format("2006-01-02"), end.AddDate(0, 0, -1).Format("2006-01-02"))
		}
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	keys := make([]cashSnapshotKey, 0, len(cache.entries))
	for key, entry := range cache.entries {
		if identity, ok := valid[key.account]; !ok || identity != entry.identity || now.Sub(entry.touched) > 30*time.Minute {
			delete(cache.entries, key)
			continue
		}
		if !entry.refreshing && now.Sub(entry.attempted) >= upstreamAccountSyncInterval {
			keys = append(keys, key)
		}
	}
	// Oldest attempt first prevents a slow account from starving custom ranges.
	sort.Slice(keys, func(i, j int) bool { return cache.entries[keys[i]].attempted.Before(cache.entries[keys[j]].attempted) })
	for _, key := range keys {
		select {
		case cache.slots <- struct{}{}:
			entry := cache.entries[key]
			entry.refreshing, entry.attempted = true, now
			go cache.refresh(key, entry)
		default:
			return
		}
	}
}
func (cache *cashReportCache) refresh(key cashSnapshotKey, entry *cashSnapshotEntry) {
	defer func() { <-cache.slots }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, err := cache.collect(ctx, key.account, key.start, key.end)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.entries[key] != entry {
		return
	}
	entry.refreshing = false
	if err != nil {
		entry.err = err.Error()
		return
	}
	entry.err, entry.data = "", data
}
func StartCashReportSnapshotTask() {
	cashSnapshotOnce.Do(func() {
		go func() {
			warm := func() {
				accounts, err := listUpstreamAccounts()
				if err != nil {
					common.SysError("cash report snapshot accounts unavailable")
					return
				}
				cashReportSnapshots.dispatch(accounts, time.Now())
			}
			warm()
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				warm()
			}
		}()
	})
}
