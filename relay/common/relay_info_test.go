package common

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetFirstEffectiveOutputTimeCapturesInitialChannelOnce(t *testing.T) {
	info := &RelayInfo{ChannelMeta: &ChannelMeta{ChannelId: 10, ChannelTag: "tag-a"}}
	info.SetFirstEffectiveOutputTime()
	first := info.FirstEffectiveOutputTime
	require.False(t, info.GroupHealthFirstOutputTime.IsZero())

	info.ChannelMeta = &ChannelMeta{ChannelId: 20, ChannelTag: "tag-b"}
	time.Sleep(time.Millisecond)
	info.SetFirstEffectiveOutputTime()

	require.Equal(t, first, info.FirstEffectiveOutputTime)
	require.Equal(t, 10, info.FirstEffectiveOutputChannelId)
	require.Equal(t, "tag-a", info.FirstEffectiveOutputChannelTag)
}

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestTaskRelayInfoPreservesResolvedMode(t *testing.T) {
	cases := []struct {
		name, method, path string
		resolved, want     int
	}{
		{"video fetch", http.MethodGet, "/v1/videos/task_owned", relayconstant.RelayModeVideoFetchByID, relayconstant.RelayModeVideoFetchByID},
		{"legacy fetch", http.MethodGet, "/v1/video/generations/task_owned", relayconstant.RelayModeVideoFetchByID, relayconstant.RelayModeVideoFetchByID},
		{"video submit", http.MethodPost, "/v1/videos", relayconstant.RelayModeVideoSubmit, relayconstant.RelayModeVideoSubmit},
		{"legacy submit", http.MethodPost, "/v1/video/generations", relayconstant.RelayModeVideoSubmit, relayconstant.RelayModeVideoSubmit},
		{"remix", http.MethodPost, "/v1/videos/task_owned/remix", relayconstant.RelayModeVideoSubmit, relayconstant.RelayModeVideoSubmit},
		{"kling fetch", http.MethodGet, "/kling/v1/videos/text2video/task_owned", relayconstant.RelayModeVideoFetchByID, relayconstant.RelayModeVideoFetchByID},
		{"suno fetch", http.MethodGet, "/suno/fetch/task_owned", relayconstant.RelayModeSunoFetchByID, relayconstant.RelayModeSunoFetchByID},
		{"suno batch fetch", http.MethodPost, "/suno/fetch", relayconstant.RelayModeSunoFetch, relayconstant.RelayModeSunoFetch},
		{"path fallback", http.MethodPost, "/v1/videos", relayconstant.RelayModeUnknown, relayconstant.RelayModeVideoSubmit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tc.method, tc.path, nil)
			if tc.resolved != relayconstant.RelayModeUnknown {
				c.Set("relay_mode", tc.resolved)
			}
			info, err := GenRelayInfo(c, types.RelayFormatTask, nil, nil)
			require.NoError(t, err)
			require.Equal(t, tc.want, info.RelayMode)
		})
	}
}

func TestTaskModeOverrideDoesNotChangeTextRelay(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
	info, err := GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil)
	require.NoError(t, err)
	require.Equal(t, relayconstant.RelayModeChatCompletions, info.RelayMode)
}
