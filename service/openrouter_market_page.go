package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"golang.org/x/net/html"
)

// The model page publishes typed endpointStats as hydration data even when the
// anonymous endpoints API omits percentiles. Parse JSON records only: never
// execute scripts or use similarly named benchmark/leaderboard statistics.
type marketPageQuery struct {
	Key   []json.RawMessage `json:"queryKey"`
	State struct {
		Updated int64           `json:"dataUpdatedAt"`
		Rows    json.RawMessage `json:"data"`
	} `json:"state"`
}
type marketPageEndpoint struct {
	ID          string                 `json:"id"`
	Slug        string                 `json:"model_variant_slug"`
	Variant     string                 `json:"variant"`
	Tag         string                 `json:"provider_slug"`
	Status      *int                   `json:"status"`
	ServiceTier *string                `json:"derived_service_tier"`
	Disabled    bool                   `json:"is_disabled"`
	Hidden      bool                   `json:"is_hidden"`
	Private     bool                   `json:"is_private"`
	BYOK        bool                   `json:"is_byok_only"`
	Pricing     map[string]interface{} `json:"pricing"`
	Stats       struct {
		Endpoint        string   `json:"endpoint_id"`
		Metric          string   `json:"latency_metric"`
		Latency         *float64 `json:"p50_latency"`
		Throughput      *float64 `json:"p50_throughput"`
		LatencyCount    int      `json:"latency_request_count"`
		ThroughputCount int      `json:"throughput_request_count"`
		Window          int      `json:"window_minutes"`
		SpeedWindow     int      `json:"throughput_window_minutes"`
	} `json:"stats"`
}

func pageQuerySpeed(q marketPageQuery, id string, now time.Time) performanceSnapshot {
	var opts struct {
		Variant  string `json:"variant"`
		Workload string `json:"perfWorkload"`
		Metric   string `json:"latencyMetric"`
		Effort   string `json:"reasoningEffort"`
	}
	_, variant, found := strings.Cut(id, ":")
	if !found {
		variant = "standard"
	}
	ts := time.UnixMilli(q.State.Updated)
	var label, kind string
	if len(q.Key) != 3 || common.Unmarshal(q.Key[0], &label) != nil || label != "model-page" || common.Unmarshal(q.Key[1], &kind) != nil || kind != "endpointStats" || common.Unmarshal(q.Key[2], &opts) != nil || opts.Workload != "text_generation" || opts.Metric != "latency" || opts.Variant != variant || (opts.Effort != "" && opts.Effort != "$undefined") || ts.After(now) || now.Sub(ts) >= performanceTTL {
		return performanceSnapshot{}
	}
	var rows []marketPageEndpoint
	if common.Unmarshal(q.State.Rows, &rows) != nil || len(rows) > 256 {
		return performanceSnapshot{}
	}
	snap := performanceSnapshot{FetchedAt: ts}
	for _, r := range rows {
		s := r.Stats
		if r.Slug != id || r.Variant != variant || r.ID == "" || s.Endpoint != r.ID || r.Private || r.Disabled || r.Hidden || r.BYOK || r.Status == nil || *r.Status != 0 {
			continue
		}
		e := performanceEndpoint{Tag: r.Tag, Status: r.Status, ServiceTier: r.ServiceTier, Pricing: r.Pricing}
		if s.Metric == "latency" && (s.Window == 30 || s.Window == 1440) && s.LatencyCount > 0 {
			e.LatencyWindowMinutes = s.Window
			e.Latency = &performancePercentile{s.Latency}
		}
		if (s.SpeedWindow == 30 || s.SpeedWindow == 1440) && s.ThroughputCount > 0 {
			e.ThroughputWindowMinutes = s.SpeedWindow
			e.Throughput = &performancePercentile{s.Throughput}
		}
		snap.Endpoints = append(snap.Endpoints, e)
	}
	return snap
}
func fetchMarketPage(ctx context.Context, client *http.Client, id string) (performanceSnapshot, error) {
	if !performanceModelID.MatchString(id) {
		return performanceSnapshot{}, errors.New("invalid public model")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/"+id, nil)
	if err != nil {
		return performanceSnapshot{}, err
	}
	req.Header.Set("Accept", "text/html")
	if client == nil {
		client = http.DefaultClient
	}
	cc := *client
	cc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	r, err := cc.Do(req)
	if err != nil {
		return performanceSnapshot{}, err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return performanceSnapshot{}, errors.New("public model page unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, (3<<20)+1))
	if err != nil || len(raw) > 3<<20 {
		return performanceSnapshot{}, errors.New("invalid public model page size")
	}
	return parseMarketPage(raw, id, time.Now())
}
func parseMarketPage(raw []byte, id string, now time.Time) (performanceSnapshot, error) {
	z := html.NewTokenizer(bytes.NewReader(raw))
	var payload strings.Builder
	for tt := z.Next(); tt != html.ErrorToken; tt = z.Next() {
		if tt != html.StartTagToken {
			continue
		}
		tag, _ := z.TagName()
		if string(tag) != "script" || z.Next() != html.TextToken {
			continue
		}
		s := strings.TrimSpace(string(z.Text()))
		s = strings.TrimSuffix(s, ";")
		data, ok := strings.CutPrefix(s, "self.__next_f.push(")
		if !ok || !strings.HasSuffix(data, ")") {
			continue
		}
		var pair []json.RawMessage
		if common.UnmarshalJsonStr(strings.TrimSuffix(data, ")"), &pair) != nil || len(pair) != 2 || string(pair[0]) != "1" {
			continue
		}
		var chunk string
		if common.Unmarshal(pair[1], &chunk) == nil {
			payload.WriteString(chunk)
		}
	}
	for _, line := range strings.Split(payload.String(), "\n") {
		if !strings.Contains(line, `"endpointStats"`) {
			continue
		}
		_, record, ok := strings.Cut(line, ":")
		var node []json.RawMessage
		if !ok || common.UnmarshalJsonStr(record, &node) != nil || len(node) != 4 {
			continue
		}
		var props struct {
			State struct {
				Queries []marketPageQuery `json:"queries"`
			} `json:"state"`
		}
		if common.Unmarshal(node[3], &props) != nil {
			continue
		}
		for _, q := range props.State.Queries {
			if snapshot := pageQuerySpeed(q, id, now); len(snapshot.Endpoints) > 0 {
				return snapshot, nil
			}
		}
	}
	return performanceSnapshot{}, errors.New("no fresh public text performance")
}
