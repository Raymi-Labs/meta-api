package service

import (
	"encoding/xml"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const officialFXURL = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"
const officialFXMaxAge = 7 * 24 * time.Hour

type OfficialPriceExchangeRate struct {
	USDCNY    float64 `json:"usd_cny"`
	Date      string  `json:"date"`
	Source    string  `json:"source"`
	SourceURL string  `json:"source_url"`
	FetchedAt int64   `json:"fetched_at"`
	Stale     bool    `json:"stale"`
}

func validOfficialFXDate(date string, now time.Time) bool {
	day, err := time.Parse("2006-01-02", date)
	return err == nil && !day.After(now.UTC()) && now.Sub(day) < officialFXMaxAge
}

func parseOfficialFX(body []byte, now time.Time) (OfficialPriceExchangeRate, error) {
	var doc struct {
		Cube struct {
			Days []struct {
				Date  string `xml:"time,attr"`
				Rates []struct {
					Currency string `xml:"currency,attr"`
					Rate     string `xml:"rate,attr"`
				} `xml:"Cube"`
			} `xml:"Cube"`
		} `xml:"Cube"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		return OfficialPriceExchangeRate{}, err
	}
	if len(doc.Cube.Days) != 1 || !validOfficialFXDate(doc.Cube.Days[0].Date, now) {
		return OfficialPriceExchangeRate{}, errors.New("invalid reference date")
	}
	rates := map[string]float64{}
	for _, entry := range doc.Cube.Days[0].Rates {
		if entry.Currency != "USD" && entry.Currency != "CNY" {
			continue
		}
		value, err := strconv.ParseFloat(entry.Rate, 64)
		if err != nil || value <= 0 || math.IsInf(value, 0) || math.IsNaN(value) || rates[entry.Currency] != 0 {
			return OfficialPriceExchangeRate{}, errors.New("invalid reference rate")
		}
		rates[entry.Currency] = value
	}
	rate := rates["CNY"] / rates["USD"]
	if rate <= 0 || math.IsInf(rate, 0) || math.IsNaN(rate) {
		return OfficialPriceExchangeRate{}, errors.New("missing currency")
	}
	return OfficialPriceExchangeRate{USDCNY: rate, Date: doc.Cube.Days[0].Date, Source: "ECB", SourceURL: officialFXURL, FetchedAt: now.Unix()}, nil
}

func fetchOfficialFX(now time.Time) (OfficialPriceExchangeRate, error) {
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect not allowed") }}
	response, err := client.Get(officialFXURL)
	if err != nil {
		return OfficialPriceExchangeRate{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return OfficialPriceExchangeRate{}, errors.New("reference feed unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(body) > 65536 {
		return OfficialPriceExchangeRate{}, errors.New("invalid reference feed size")
	}
	return parseOfficialFX(body, now)
}

type officialFXCache struct {
	mu          sync.Mutex
	quote       OfficialPriceExchangeRate
	nextAttempt time.Time
}

// One bounded refresh per process; payment/quota settings never enter this cache.
func (cache *officialFXCache) get(now time.Time, fetch func(time.Time) (OfficialPriceExchangeRate, error)) (OfficialPriceExchangeRate, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if !now.Before(cache.nextAttempt) {
		quote, err := fetch(now)
		cache.nextAttempt = now.Add(15 * time.Minute)
		if err == nil && quote.USDCNY > 0 && validOfficialFXDate(quote.Date, now) {
			cache.quote = quote
			cache.nextAttempt = now.Add(6 * time.Hour)
		} else {
			cache.quote.Stale = true
		}
	}
	return cache.quote, cache.quote.USDCNY > 0 && validOfficialFXDate(cache.quote.Date, now)
}

var officialPriceFX officialFXCache

func GetOfficialPriceExchangeRate() (OfficialPriceExchangeRate, bool) {
	return officialPriceFX.get(time.Now(), fetchOfficialFX)
}
