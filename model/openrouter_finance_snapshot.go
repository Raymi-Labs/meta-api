package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type openRouterFinanceEntry struct {
	identity   string
	data       *OpenRouterAccountFinance
	attempted  time.Time
	refreshing bool
}

type openRouterFinanceCache struct {
	mu      sync.Mutex
	entries map[int]*openRouterFinanceEntry
	slots   chan struct{}
	collect func(context.Context, UpstreamAccount) *OpenRouterAccountFinance
}

func newOpenRouterFinanceCache(collect func(context.Context, UpstreamAccount) *OpenRouterAccountFinance) *openRouterFinanceCache {
	return &openRouterFinanceCache{entries: make(map[int]*openRouterFinanceEntry), slots: make(chan struct{}, 2), collect: collect}
}

var openRouterFinanceSnapshots = newOpenRouterFinanceCache(collectOpenRouterAccountFinance)
var openRouterFinanceSnapshotOnce sync.Once

// Values published in entries are immutable. Return a shallow copy for response metadata.
func (cache *openRouterFinanceCache) read(account UpstreamAccount) *OpenRouterAccountFinance {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	identity := fmt.Sprintf("%x", sha256.Sum256([]byte(account.AccessTokenEncrypted+"\x00"+account.AccessToken)))
	entry := cache.entries[account.Id]
	if entry == nil || entry.identity != identity {
		entry = &openRouterFinanceEntry{identity: identity}
		cache.entries[account.Id] = entry
	}
	if !entry.refreshing && time.Since(entry.attempted) >= upstreamAccountSyncInterval {
		select {
		case cache.slots <- struct{}{}:
			entry.refreshing = true
			entry.attempted = time.Now()
			go cache.refresh(account, entry)
		default:
			// No queued goroutine per request; a later read or scheduler pass retries.
		}
	}
	result := &OpenRouterAccountFinance{}
	if entry.data != nil {
		*result = *entry.data
	}
	result.Refreshing = entry.refreshing || entry.data == nil
	return result
}

func (cache *openRouterFinanceCache) refresh(account UpstreamAccount, entry *openRouterFinanceEntry) {
	defer func() { <-cache.slots }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result := cache.collect(ctx, account)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	// A rotated key must never receive a result from an older collection.
	if cache.entries[account.Id] != entry {
		return
	}
	refreshError := strings.Trim(strings.Join([]string{result.CreditsError, result.ActivityError, result.SiteError}, "; "), "; ")
	if entry.data != nil && refreshError != "" && entry.data.StartDate == result.StartDate && entry.data.EndDate == result.EndDate {
		previous := entry.data
		retained := false
		if result.CreditsError != "" && previous.Credits != nil {
			result.Credits = previous.Credits
			retained = true
		}
		if result.ActivityError != "" && previous.Activity != nil {
			result.Activity = previous.Activity
			retained = true
		}
		if result.SiteError != "" && previous.SiteActivity != nil {
			result.SiteActivity = previous.SiteActivity
			result.SiteChannels = previous.SiteChannels
			retained = true
		}
		if retained {
			result.FetchedAt = previous.FetchedAt
			result.RefreshError = refreshError
		}
	}
	entry.data = result
	entry.refreshing = false
}

// Run on every serving node: snapshots are local and contain no plaintext credentials.
func StartOpenRouterFinanceSnapshotTask() {
	openRouterFinanceSnapshotOnce.Do(func() {
		go func() {
			warm := func() {
				accounts, err := listUpstreamAccounts()
				if err != nil {
					common.SysError("OpenRouter snapshot accounts unavailable")
					return
				}
				active := make(map[int]bool)
				for _, account := range accounts {
					if account.Enabled && account.AccountType == UpstreamAccountTypeOpenRouter {
						active[account.Id] = true
						openRouterFinanceSnapshots.read(account)
					}
				}
				openRouterFinanceSnapshots.mu.Lock()
				for id := range openRouterFinanceSnapshots.entries {
					if !active[id] {
						delete(openRouterFinanceSnapshots.entries, id)
					}
				}
				openRouterFinanceSnapshots.mu.Unlock()
			}
			warm()
			// Frequent dispatch lets accounts beyond the concurrency limit start promptly;
			// each account still collects at most once every five minutes.
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				warm()
			}
		}()
	})
}
