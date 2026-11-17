package helper

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamDeliveryObservationCountsOnlyFlushedEffectiveFrames(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		effective  bool
	}{
		{"role", `{"choices":[{"delta":{"role":"assistant"}}]}`, false},
		{"text", `{"choices":[{"delta":{"content":"hello"}}]}`, true},
		{"reasoning", `{"choices":[{"delta":{"reasoning_content":"thinking"}}]}`, true},
		{"tool_id", `{"choices":[{"delta":{"tool_calls":[{"id":"call_1"}]}}]}`, false},
		{"tool_arguments", `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{}"}}]}}]}`, true},
		{"responses_created", `{"type":"response.created"}`, false},
		{"responses_delta", `{"type":"response.output_text.delta","delta":"hello"}`, true},
		{"claude_ping", `{"type":"ping"}`, false},
		{"claude_thinking", `{"type":"content_block_delta","delta":{"thinking":"hello"}}`, true},
		{"done", `[DONE]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			o := common.NewStreamDeliveryObservation(time.Now())
			c.Set(common.StreamDeliveryObservationKey, o)
			require.NoError(t, PingData(c))
			require.NoError(t, StringData(c, tc.data))
			snapshot := o.Freeze(time.Now())
			require.True(t, snapshot.Observed)
			require.EqualValues(t, 1, snapshot.Heartbeats)
			require.Equal(t, tc.effective, snapshot.EffectiveFrames == 1)
			require.Equal(t, tc.effective, snapshot.FirstEffectiveMs != nil)
			require.Equal(t, tc.effective, snapshot.LastEffectiveAgoMs != nil)
			require.NoError(t, StringData(c, `{"choices":[{"delta":{"content":"late"}}]}`))
			require.Equal(t, snapshot, o.Freeze(time.Now()))
		})
	}
}

func TestStreamDeliveryObservationWriteFailureIsNotDelivery(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	o := common.NewStreamDeliveryObservation(time.Now())
	c.Set(common.StreamDeliveryObservationKey, o)
	c.Writer = &deliveryProbeWriter{ResponseWriter: c.Writer, flushErr: errors.New("flush failed")}
	require.Error(t, StringData(c, `{"choices":[{"delta":{"content":"secret"}}]}`))
	s := o.Freeze(time.Now())
	require.True(t, s.WriteFailed)
	require.Zero(t, s.EffectiveFrames)
	require.Nil(t, s.FirstEffectiveMs)
}

func TestStreamKeepAliveEarlyFirstFrameRealHTTP(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, types.RelayFormatClaude} {
		t.Run(string(format), func(t *testing.T) {
			released := make(chan struct{})
			finished := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(finished)
				c, _ := gin.CreateTestContext(w)
				c.Request = r
				stop := StartStreamKeepAlive(c, 1200*time.Second, format)
				defer stop()
				select {
				case <-released:
				case <-r.Context().Done():
					return
				}
				stop()
				if format == types.RelayFormatClaude {
					_ = StringData(c, `{"type":"message_stop"}`)
				} else {
					_ = StringData(c, `{"type":"response.completed"}`)
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "POST", server.URL, nil)
			start := time.Now()
			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Less(t, time.Since(start), 3*time.Second, "first frame must precede a short idle timeout even with legacy interval")
			reader := bufio.NewReader(resp.Body)
			line, err := reader.ReadString('\n')
			require.NoError(t, err)
			if format == types.RelayFormatClaude {
				require.Equal(t, "event: ping\n", line)
			} else {
				require.Equal(t, ": PING\n", line)
			}
			close(released)
			select {
			case <-finished:
			case <-ctx.Done():
				t.Fatal("heartbeat worker did not finish")
			}
		})
	}
}

func TestStreamDeliveryUnsupportedProtocolRemainsUnknown(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1beta/models/model:streamGenerateContent", nil)
	o := common.NewStreamDeliveryObservation(time.Now())
	c.Set(common.StreamDeliveryObservationKey, o)
	require.NoError(t, PingData(c))
	require.NoError(t, StringData(c, `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`))
	require.False(t, o.Freeze(time.Now()).Observed)
}
