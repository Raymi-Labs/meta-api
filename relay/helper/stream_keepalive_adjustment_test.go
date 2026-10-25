package helper

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamKeepAliveAdjustmentEvidence(t *testing.T) {
	for _, scenario := range []string{"uncommitted", "ping", "restart", "write", "empty_write", "write_string", "header", "header_now", "flush", "untracked_write", "write_error", "write_error_unknown_writer", "write_error_unknown_writer_restart", "short_write", "flush_error", "canceled", "unknown_writer", "other_protocol", "changed_protocol"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
			probe := &deliveryProbeWriter{ResponseWriter: c.Writer, writeReady: make(chan struct{}, 8)}
			c.Writer = probe
			switch scenario {
			case "write_error", "write_error_unknown_writer", "write_error_unknown_writer_restart":
				probe.writeErr = io.ErrClosedPipe
			case "short_write":
				probe.short = true
			case "flush_error":
				probe.flushErr = io.ErrClosedPipe
			}
			format := types.RelayFormat(types.RelayFormatClaude)
			if scenario == "other_protocol" || scenario == "changed_protocol" {
				format = types.RelayFormatOpenAI
			}
			interval := time.Millisecond
			if scenario == "uncommitted" {
				interval = time.Hour
			}
			stop := StartStreamKeepAlive(c, interval, format)
			defer stop()
			if scenario != "uncommitted" {
				select {
				case <-probe.writeReady:
				case <-time.After(time.Second):
					t.Fatal("ping not attempted")
				}
				StopStreamKeepAlive(c)
			}
			switch scenario {
			case "write":
				_, _ = c.Writer.Write([]byte("event: content_block_delta\n\n"))
			case "empty_write":
				_, _ = c.Writer.Write(nil)
			case "write_string":
				_, _ = c.Writer.WriteString("event: content_block_start\n\n")
			case "header":
				c.Writer.WriteHeader(http.StatusOK)
			case "header_now":
				c.Writer.WriteHeaderNow()
			case "flush":
				c.Writer.Flush()
			case "untracked_write":
				_, _ = probe.Write([]byte("business data"))
			case "canceled":
				cancel()
			case "unknown_writer", "write_error_unknown_writer", "write_error_unknown_writer_restart":
				c.Writer = probe
				if scenario != "unknown_writer" {
					require.False(t, probe.Written(), "zero-byte failed ping must remain uncommitted")
				}
			}
			if scenario == "changed_protocol" {
				format = types.RelayFormatClaude
			}
			// All negative evidence must survive wrapper replacement, too.
			if scenario != "uncommitted" && scenario != "unknown_writer" && scenario != "write_error_unknown_writer" && scenario != "canceled" {
				stopAgain := StartStreamKeepAlive(c, interval, format)
				defer stopAgain()
				if scenario == "restart" {
					select {
					case <-probe.writeReady:
					case <-time.After(time.Second):
						t.Fatal("replacement ping not attempted")
					}
				}
			}
			want := scenario == "uncommitted" || scenario == "ping" || scenario == "restart"
			require.Equal(t, want, CanAdjustClaudeRequestBeforeOutput(c))
			if scenario == "write_error" || scenario == "write_error_unknown_writer" || scenario == "write_error_unknown_writer_restart" || scenario == "short_write" {
				require.Zero(t, probe.flushes.Load())
			}
		})
	}
}

func TestPingDataRejectsShortWrite(t *testing.T) {
	for _, ping := range []func(*gin.Context) error{PingData, ClaudePingData} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		probe := &deliveryProbeWriter{ResponseWriter: c.Writer, short: true}
		c.Writer = probe
		require.ErrorIs(t, ping(c), io.ErrShortWrite)
		require.Zero(t, probe.flushes.Load())
	}
}
