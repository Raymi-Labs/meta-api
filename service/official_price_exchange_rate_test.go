package service

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"
)

func TestOfficialFXParse(t *testing.T) {
	now := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	body := func(date, usd, cny string) []byte {
		return []byte(fmt.Sprintf(`<Envelope xmlns="http://www.ecb.int/vocabulary/2002-08-01/eurofxref"><Cube><Cube time="%s"><Cube currency="USD" rate="%s"/><Cube currency="CNY" rate="%s"/></Cube></Cube></Envelope>`, date, usd, cny))
	}
	q, err := parseOfficialFX(body("2026-10-30", "1.1172", "7.4326"), now)
	if err != nil || math.Abs(q.USDCNY-7.4326/1.1172) > 1e-12 || q.Date != "2026-10-30" || q.Source != "ECB" {
		t.Fatalf("%+v %v", q, err)
	}
	for _, b := range [][]byte{body("2026-10-24", "1", "7"), body("2026-11-01", "1", "7"), body("invalid", "1", "7"), body("2026-10-30", "0", "7"), body("2026-10-30", "NaN", "7"), body("2026-10-30", "1", "Inf"), []byte(`<Envelope/>`), []byte(`not xml`), []byte(`<Envelope><Cube><Cube time="2026-10-30"><Cube currency="USD" rate="1"/><Cube currency="CNY" rate="7"/><Cube currency="CNY" rate="8"/></Cube></Cube></Envelope>`)} {
		if _, err := parseOfficialFX(b, now); err == nil {
			t.Fatalf("accepted %s", b)
		}
	}
}
func TestOfficialFXCache(t *testing.T) {
	now := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	cache := officialFXCache{}
	calls := 0
	fetch := func(at time.Time) (OfficialPriceExchangeRate, error) {
		calls++
		return OfficialPriceExchangeRate{USDCNY: 7, Date: "2026-10-30", FetchedAt: at.Unix()}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := cache.get(now, fetch); !ok {
				t.Error("missing quote")
			}
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatal(calls)
	}
	failure := func(time.Time) (OfficialPriceExchangeRate, error) {
		calls++
		return OfficialPriceExchangeRate{}, errors.New("offline")
	}
	q, ok := cache.get(now.Add(6*time.Hour), failure)
	if !ok || !q.Stale || q.USDCNY != 7 || calls != 2 {
		t.Fatalf("%+v %v %d", q, ok, calls)
	}
	cache.get(now.Add(6*time.Hour+time.Minute), failure)
	if calls != 2 {
		t.Fatal("failure backoff missing")
	}
	if _, ok := cache.get(now.Add(7*24*time.Hour), failure); ok {
		t.Fatal("expired quote")
	}
	empty := officialFXCache{}
	if _, ok := empty.get(now, failure); ok {
		t.Fatal("invented quote")
	}
}
