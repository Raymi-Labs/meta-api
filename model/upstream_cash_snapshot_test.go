package model

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func waitCashSnapshots(t *testing.T, cache *cashReportCache) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(cache.slots) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("background snapshot did not finish")
}
func TestCashSnapshotReadNeverCalculatesAndPrewarms(t *testing.T) {
	var calls atomic.Int32
	cache := newCashReportCache(func(_ context.Context, id int, start, end string) (*CashReport, error) {
		calls.Add(1)
		return &CashReport{AccountID: id, StartDate: start, EndDate: end, GeneratedAt: 123}, nil
	})
	a := UpstreamAccount{Id: 1, AccountType: UpstreamAccountTypeOpenRouter}
	now := time.Now()
	end := now.UTC().Truncate(24 * time.Hour)
	startText, endText := end.AddDate(0, 0, -7).Format("2006-01-02"), end.AddDate(0, 0, -1).Format("2006-01-02")
	for i := 0; i < 20; i++ {
		if !cache.read(a, startText, endText).Refreshing {
			t.Fatal("cold state must be pending")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("read calculated a report")
	}
	cache.dispatch([]UpstreamAccount{a}, now)
	waitCashSnapshots(t, cache)
	if calls.Load() != 2 {
		t.Fatalf("must prewarm both 7 and 30 days: %d", calls.Load())
	}
	if r := cache.read(a, startText, endText); r.GeneratedAt != 123 || r.Refreshing {
		t.Fatalf("warm result: %+v", r)
	}
	cache.dispatch([]UpstreamAccount{a}, now.Add(time.Minute))
	waitCashSnapshots(t, cache)
	if calls.Load() != 2 {
		t.Fatal("fresh entries recalculated")
	}
}
func TestCashSnapshotFailureRetentionAndIsolation(t *testing.T) {
	cache := newCashReportCache(func(context.Context, int, string, string) (*CashReport, error) {
		return nil, errors.New("synthetic failure")
	})
	a := UpstreamAccount{Id: 1, AccountType: UpstreamAccountTypeOpenRouter, AccessTokenEncrypted: "old"}
	cache.read(a, "2026-11-01", "2026-11-07")
	key := cashSnapshotKey{1, "2026-11-01", "2026-11-07"}
	entry := cache.entries[key]
	entry.data = &CashReport{GeneratedAt: 123}
	cache.slots <- struct{}{}
	cache.refresh(key, entry)
	r := cache.read(a, key.start, key.end)
	if r.GeneratedAt != 123 || r.RefreshError == "" {
		t.Fatal("failure lost old result or error")
	}
	if cache.read(a, "2026-11-02", key.end).GeneratedAt != 0 {
		t.Fatal("range data leaked")
	}
	other := a
	other.Id = 2
	if cache.read(other, key.start, key.end).GeneratedAt != 0 {
		t.Fatal("account data leaked")
	}
	a.AccessTokenEncrypted = "new"
	if cache.read(a, key.start, key.end).GeneratedAt != 0 {
		t.Fatal("credential change reused old result")
	}
	cache.slots <- struct{}{}
	cache.refresh(key, entry)
	if cache.read(a, key.start, key.end).RefreshError != "" {
		t.Fatal("old job overwrote new identity")
	}
}
func TestCashSnapshotCapacityCleanupAndConcurrency(t *testing.T) {
	release := make(chan struct{})
	cache := newCashReportCache(func(context.Context, int, string, string) (*CashReport, error) {
		<-release
		return &CashReport{GeneratedAt: 1}, nil
	})
	a := UpstreamAccount{Id: 1, AccountType: UpstreamAccountTypeOpenRouter}
	for i := 0; i < 200; i++ {
		cache.read(a, fmt.Sprint(i), "end")
	}
	if len(cache.entries) != cashSnapshotLimit {
		t.Fatal("cache capacity not bounded")
	}
	cache.dispatch([]UpstreamAccount{a}, time.Now())
	if len(cache.slots) != 2 {
		t.Fatal("expected two bounded workers")
	}
	cache.dispatch(nil, time.Now())
	cache.mu.Lock()
	n := len(cache.entries)
	cache.mu.Unlock()
	if n != 0 {
		t.Fatal("deleted accounts retained")
	}
	close(release)
	waitCashSnapshots(t, cache)
	if len(cache.entries) != 0 {
		t.Fatal("deleted entries resurrected")
	}
}
func TestCashSnapshotEndpointValidatesBeforeCache(t *testing.T) {
	db := setupUpstreamAccountTestDB(t)
	account := UpstreamAccount{AccountType: UpstreamAccountTypeOpenRouter, Enabled: true}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	original := cashReportSnapshots
	cashReportSnapshots = newCashReportCache(func(context.Context, int, string, string) (*CashReport, error) {
		t.Error("HTTP read calculated")
		return nil, nil
	})
	t.Cleanup(func() { cashReportSnapshots = original })
	if _, err := GetCashReportSnapshot(account.Id, "invalid", "invalid"); err == nil {
		t.Fatal("invalid range accepted")
	}
	if r, err := GetCashReportSnapshot(account.Id, "", ""); err != nil || !r.Refreshing {
		t.Fatalf("cold endpoint: %+v %v", r, err)
	}
	if err := db.Delete(&account).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := GetCashReportSnapshot(account.Id, "", ""); err == nil {
		t.Fatal("deleted account cache exposed")
	}
}
