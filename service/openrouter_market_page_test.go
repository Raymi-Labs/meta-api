package service

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestMarketPageReadsExactPublicTextMeasurementAndWindows(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	id := "vendor/model:free"
	base := fmt.Sprintf(`{"queryKey":["model-page","endpointStats",{"variant":"free","perfWorkload":"text_generation","latencyMetric":"latency"}],"state":{"dataUpdatedAt":%d,"data":[{"id":"ep","model_variant_slug":"vendor/model:free","variant":"free","provider_slug":"p","status":0,"pricing":{"prompt":"0","completion":"0"},"stats":{"endpoint_id":"ep","latency_metric":"latency","p50_latency":0,"p50_throughput":71,"latency_request_count":4,"throughput_request_count":7,"window_minutes":30,"throughput_window_minutes":1440}}]}}`, now.UnixMilli())
	wrap := func(q string) []byte {
		line := `a:["$",null,null,{"state":{"queries":[` + q + `]}}]` + "\n"
		part1, err := common.Marshal([]any{1, line[:len(line)/2]})
		require.NoError(t, err)
		part2, err := common.Marshal([]any{1, line[len(line)/2:]})
		require.NoError(t, err)
		return []byte(fmt.Sprintf("<script>self.__next_f.push(%s)</script><script>self.__next_f.push(%s);</script>", part1, part2))
	}
	snap, err := parseMarketPage(wrap(base), id, now)
	require.NoError(t, err)
	r := marketSpeed(snap, id)
	require.NotNil(t, r)
	require.Zero(t, *r.TTFTMs)
	require.Equal(t, 71., *r.OutputTPS)
	require.Equal(t, 30, r.LatencyWindowMinutes)
	require.Equal(t, 1440, r.ThroughputWindowMinutes)
	for _, change := range [][2]string{{"vendor/model:free", "vendor/model"}, {"text_generation", "video_generation"}, {"endpointStats", "endpointLeaderboard"}, {`"endpoint_id":"ep"`, `"endpoint_id":"another"`}, {`"status":0`, `"is_private":true,"status":0`}, {`"variant":"free"`, `"variant":"standard"`}, {fmt.Sprint(now.UnixMilli()), fmt.Sprint(now.Add(-6 * time.Minute).UnixMilli())}} {
		_, err = parseMarketPage(wrap(strings.ReplaceAll(base, change[0], change[1])), id, now)
		require.Error(t, err, "%v", change)
	}
	snap, err = parseMarketPage(wrap(strings.ReplaceAll(base, `"latency_request_count":4`, `"latency_request_count":0`)), id, now)
	require.NoError(t, err)
	require.Nil(t, marketSpeed(snap, id).TTFTMs)
}
