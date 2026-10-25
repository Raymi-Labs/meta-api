package grouphealth

import (
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestHealthReviewStreamTimingEvidence(t *testing.T) {
	start := time.Unix(1800000000, 0)
	for _, tc := range []struct {
		name         string
		clientStream bool
		observedAt   time.Time
		wantMeasured bool
	}{
		{name: "nonstream client with upstream SSE", observedAt: start.Add(5 * time.Second)},
		{name: "stream excludes later settlement", clientStream: true, observedAt: start.Add(5 * time.Second), wantMeasured: true},
		{name: "missing usage clock", clientStream: true},
		{name: "usage clock before output", clientStream: true, observedAt: start.Add(time.Second)},
		{name: "usage clock after request", clientStream: true, observedAt: start.Add(11 * time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := NewRequest("m", tc.clientStream)
			info := &relaycommon.RelayInfo{
				UsingGroup: "g", StartTime: start, IsStream: true,
				GroupHealthFirstOutputTime: start.Add(2 * time.Second),
				GroupHealthUsageObservedAt: tc.observedAt,
				GroupHealthTokenUsage:      &relaycommon.GroupHealthTokenUsage{InputTokens: 100, OutputTokens: 30},
			}
			request.Observe(info, true, start.Add(10*time.Second))
			sample := request.Samples(start)[0]
			require.NotNil(t, sample.TokenUsage, "real token totals remain available independently of stream timing")
			if tc.wantMeasured {
				require.NotNil(t, sample.GenerationMs)
				require.EqualValues(t, 3000, *sample.GenerationMs)
			} else {
				require.Nil(t, sample.GenerationMs)
			}
			if !tc.clientStream {
				require.Nil(t, sample.TTFTMs, "upstream protocol conversion cannot change client timing semantics")
			}
		})
	}
}
