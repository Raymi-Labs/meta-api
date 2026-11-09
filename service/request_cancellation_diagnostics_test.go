package service

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCancellationTransportProgress(t *testing.T) {
	for _, stage := range []string{"unknown", "waiting_headers", "waiting_body", "receiving_body", "retry"} {
		t.Run(stage, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx)
			finish := BeginRequestCancellationObservation(c, time.Now().Add(-45*time.Second))
			defer finish()
			if stage != "unknown" {
				BeginCancellationHTTPAttempt(c)
			}
			if stage == "waiting_body" || stage == "receiving_body" || stage == "retry" {
				body := ObserveCancellationHTTPResponse(c, io.NopCloser(strings.NewReader("data: ping\n")))
				if stage != "waiting_body" {
					got, err := io.ReadAll(body)
					require.NoError(t, err)
					require.Equal(t, "data: ping\n", string(got))
				}
				require.NoError(t, body.Close())
			}
			if stage == "retry" {
				BeginCancellationHTTPAttempt(c)
			}
			cancel()
			finish()
			first := map[string]interface{}{}
			appendCancellationDiagnostics(c, types.NewClientCanceledError(), first, time.Minute)
			d := first["cancellation"].(CancellationDiagnostics)
			require.GreaterOrEqual(t, d.DurationMs, int64(45000))
			require.Less(t, d.DurationMs, int64(46000))
			want := stage
			if want == "retry" {
				want = "waiting_headers"
			}
			require.Equal(t, want, d.Phase)
			if stage == "unknown" {
				require.Nil(t, d.UpstreamResponseStarted)
			} else {
				require.NotNil(t, d.UpstreamResponseStarted)
				require.Equal(t, stage == "receiving_body", *d.UpstreamResponseStarted)
			}
			// A late read or retry must not overwrite the cancellation snapshot.
			body := ObserveCancellationHTTPResponse(c, io.NopCloser(strings.NewReader("late")))
			_, err := io.ReadAll(body)
			require.NoError(t, err)
			BeginCancellationHTTPAttempt(c)
			second := map[string]interface{}{}
			appendCancellationDiagnostics(c, types.NewClientCanceledError(), second, 2*time.Minute)
			require.Equal(t, first, second)
			finish() // idempotent cleanup; must not block
		})
	}
}

func TestCancellationDiagnosticsFinalLogAndFallback(t *testing.T) {
	c := failureLogContext(t)
	MarkRequestFailure(c, types.NewClientCanceledError())
	RecordFinalRequestFailure(c, 45*time.Second)
	var row model.Log
	require.NoError(t, model.LOG_DB.First(&row).Error)
	details, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	diagnostic := details["admin_info"].(map[string]interface{})["cancellation"].(map[string]interface{})
	require.EqualValues(t, 45000, diagnostic["duration_ms"])
	require.Nil(t, diagnostic["upstream_response_started"])
	require.Equal(t, "unknown", diagnostic["phase"])
	require.NotContains(t, details, "cancellation")
	require.Equal(t, "client_canceled", details["failure_class"])
	logs, _, err := model.GetUserLogs(c.GetInt("id"), model.LogTypeError, 0, 0, "", "", 0, 20, "", "")
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.NotContains(t, logs[0].Other, "cancellation")
	admin := map[string]interface{}{}
	appendCancellationDiagnostics(c, types.NewErrorWithStatusCode(io.EOF, types.ErrorCodeBadResponseBody, 502), admin, time.Minute)
	require.Empty(t, admin)
}

func TestCancellationObservationNormalFinish(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest("POST", "/", nil).WithContext(ctx)
	finish := BeginRequestCancellationObservation(c, time.Now())
	finish()
	finish()
	cancel()
	finish()
	o := cancellationObservation(c)
	o.mu.Lock()
	defer o.mu.Unlock()
	require.Nil(t, o.frozen, "normal completion unregisters the observer")
}
