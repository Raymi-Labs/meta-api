package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	grouphealth "github.com/QuantumNous/new-api/pkg/group_health"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type healthTestResponseWriter struct {
	*httptest.ResponseRecorder
	writeErr   error
	shortWrite bool
	flushErr   error
}

func (w *healthTestResponseWriter) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.shortWrite && len(data) > 0 {
		return w.ResponseRecorder.Write(data[:len(data)-1])
	}
	return w.ResponseRecorder.Write(data)
}

func (w *healthTestResponseWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *healthTestResponseWriter) FlushError() error {
	w.ResponseRecorder.Flush()
	return w.flushErr
}

func TestGroupHealthWriterRequiresActualSuccessfulWrite(t *testing.T) {
	for _, tc := range []struct {
		name        string
		writeErr    error
		shortWrite  bool
		writeString bool
	}{
		{name: "write error", writeErr: io.ErrClosedPipe},
		{name: "short write", shortWrite: true},
		{name: "write string error", writeErr: io.ErrClosedPipe, writeString: true},
		{name: "short write string", shortWrite: true, writeString: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := &healthTestResponseWriter{ResponseRecorder: httptest.NewRecorder(), writeErr: tc.writeErr, shortWrite: tc.shortWrite}
			c, _ := gin.CreateTestContext(base)
			writer := &groupHealthWriter{ResponseWriter: c.Writer}
			require.False(t, writer.delivered(), "a nil relay error without a body is not success")
			var n int
			var err error
			if tc.writeString {
				n, err = writer.WriteString("body")
			} else {
				n, err = writer.Write([]byte("body"))
			}
			require.Equal(t, tc.writeErr, err, "the wrapper preserves the relay's return value")
			if tc.shortWrite {
				require.Equal(t, 3, n)
			}
			require.False(t, writer.delivered())
		})
	}
}

func TestGroupHealthWriterPreservesFlushThroughGinAndRequestID(t *testing.T) {
	base := &healthTestResponseWriter{ResponseRecorder: httptest.NewRecorder(), flushErr: io.ErrClosedPipe}
	r := gin.New()
	r.Use(middleware.RequestId())
	r.GET("/", func(c *gin.Context) {
		writer := &groupHealthWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		c.Header(common.RequestIdKey, "upstream-request-id")
		writer.Flush()
		require.False(t, writer.failed.Load(), "Gin's error-discarding Flush is preserved rather than bypassed")
		require.Equal(t, c.GetString(common.RequestIdKey), base.Header().Get(common.RequestIdKey))
		require.True(t, base.Flushed)
	})
	r.ServeHTTP(base, httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestGroupHealthWriterSuccessSurvivesLaterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	writer := &groupHealthWriter{ResponseWriter: c.Writer}
	_, err := writer.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	require.NoError(t, err)
	cancel()
	require.ErrorIs(t, c.Request.Context().Err(), context.Canceled)
	health := grouphealth.NewRequest("model", false)
	health.ObserveResult(&relaycommon.RelayInfo{UsingGroup: "group"}, nil, writer.delivered(), time.Now())
	require.Equal(t, grouphealth.OutcomeSuccess, health.Samples(time.Now())[0].Outcome)
	writer.beginAttempt()
	require.False(t, writer.delivered(), "previous attempts cannot supply delivery evidence")
}

func TestGroupHealthWriterFailureWithStreamErrorRemainsUnattributed(t *testing.T) {
	base := &healthTestResponseWriter{ResponseRecorder: httptest.NewRecorder(), writeErr: io.ErrClosedPipe}
	c, _ := gin.CreateTestContext(base)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	writer := &groupHealthWriter{ResponseWriter: c.Writer}
	_, err := writer.Write([]byte("data: chunk\n\n"))
	require.ErrorIs(t, err, io.ErrClosedPipe)
	require.NoError(t, c.Request.Context().Err())
	status := relaycommon.NewStreamStatus()
	status.RecordError("downstream write failed")
	health := grouphealth.NewRequest("model", true)
	health.ObserveResult(&relaycommon.RelayInfo{UsingGroup: "group", StreamStatus: status}, nil, writer.delivered(), time.Now())
	require.Equal(t, grouphealth.OutcomeUnknownFailure, health.Samples(time.Now())[0].Outcome)
	base.writeErr = nil
	writer.beginAttempt()
	_, err = writer.Write([]byte("retry"))
	require.NoError(t, err)
	require.False(t, writer.delivered(), "a failed downstream response cannot be repaired by retry accounting")
}

func TestGroupHealthWriterPreservesFlushError(t *testing.T) {
	errFlush := errors.New("flush failed")
	base := &healthTestResponseWriter{ResponseRecorder: httptest.NewRecorder(), flushErr: errFlush}
	c, _ := gin.CreateTestContext(base)
	writer := &groupHealthWriter{ResponseWriter: &healthErrorAwareGinWriter{ResponseWriter: c.Writer, flushErr: errFlush}}
	_, err := writer.WriteString("body")
	require.NoError(t, err)
	require.True(t, writer.delivered())
	require.ErrorIs(t, writer.FlushError(), errFlush)
	require.False(t, writer.delivered())
	require.Same(t, writer.ResponseWriter, writer.Unwrap())
}

type healthErrorAwareGinWriter struct {
	gin.ResponseWriter
	flushErr error
}

func (w *healthErrorAwareGinWriter) FlushError() error {
	w.ResponseWriter.Flush()
	return w.flushErr
}

type healthBufferedWriter struct {
	gin.ResponseWriter
	pending []byte
	flushes int
}

func (w *healthBufferedWriter) Write(data []byte) (int, error) {
	w.pending = append(w.pending, data...)
	return len(data), nil
}

func (w *healthBufferedWriter) Flush() {
	w.flushes++
	_, _ = w.ResponseWriter.Write(w.pending)
	w.pending = nil
	w.ResponseWriter.Flush()
}

func (w *healthBufferedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestGroupHealthWriterDoesNotBypassBufferingMiddleware(t *testing.T) {
	base := &healthTestResponseWriter{ResponseRecorder: httptest.NewRecorder(), flushErr: io.ErrClosedPipe}
	c, _ := gin.CreateTestContext(base)
	buffered := &healthBufferedWriter{ResponseWriter: c.Writer}
	writer := &groupHealthWriter{ResponseWriter: buffered}
	_, err := writer.Write([]byte("buffered body"))
	require.NoError(t, err)
	require.Empty(t, base.Body.String())
	writer.Flush()
	require.Equal(t, "buffered body", base.Body.String())
	require.Equal(t, 1, buffered.flushes)
	require.True(t, base.Flushed)
	require.False(t, writer.failed.Load(), "errors swallowed inside another middleware remain unobservable")
}
