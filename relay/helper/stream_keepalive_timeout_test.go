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

func TestHeaderTimeoutHeartbeatEvidence(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
		for _, scenario := range []string{"ping", "write", "short", "write_error", "flush_error", "canceled", "unknown"} {
			t.Run(string(format)+"/"+scenario, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
				probe := &deliveryProbeWriter{ResponseWriter: c.Writer, writeReady: make(chan struct{}, 8)}
				c.Writer = probe
				if scenario == "short" {
					probe.short = true
				}
				if scenario == "write_error" {
					probe.writeErr = io.ErrClosedPipe
				}
				if scenario == "flush_error" {
					probe.flushErr = io.ErrClosedPipe
				}
				stop := StartStreamKeepAlive(c, time.Millisecond, format)
				defer stop()
				select {
				case <-probe.writeReady:
				case <-time.After(time.Second):
					t.Fatal("ping absent")
				}
				StopStreamKeepAlive(c)
				switch scenario {
				case "write":
					c.Writer.Write([]byte("data: content\n\n"))
				case "canceled":
					cancel()
				case "unknown":
					c.Writer = probe
				}
				require.Equal(t, scenario == "ping", CanRetryHeaderTimeoutBeforeOutput(c, format))
			})
		}
	}
}
