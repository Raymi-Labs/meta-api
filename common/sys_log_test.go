package common

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type logWriteFunc func([]byte) (int, error)

func (f logWriteFunc) Write(p []byte) (int, error) { return f(p) }

func TestCurrentLogWriterPropagatesResults(t *testing.T) {
	LogWriterMu.Lock()
	oldOut, oldErr := gin.DefaultWriter, gin.DefaultErrorWriter
	LogWriterMu.Unlock()
	t.Cleanup(func() {
		LogWriterMu.Lock()
		defer LogWriterMu.Unlock()
		gin.DefaultWriter, gin.DefaultErrorWriter = oldOut, oldErr
	})
	failure := errors.New("fixture write failure")
	for _, tc := range []struct {
		name string
		n    int
		err  error
	}{
		{"complete", 4, nil}, {"short", 2, nil}, {"failure", 1, failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := logWriteFunc(func(p []byte) (int, error) { require.Equal(t, "test", string(p)); return tc.n, tc.err })
			LogWriterMu.Lock()
			gin.DefaultWriter, gin.DefaultErrorWriter = sink, sink
			LogWriterMu.Unlock()
			for _, proxy := range []io.Writer{LogWriter, ErrorLogWriter} {
				n, err := proxy.Write([]byte("test"))
				require.Equal(t, tc.n, n)
				require.Equal(t, tc.err, err)
			}
		})
	}
}

func TestCurrentLogWriterHoldsLockUntilWriteCompletes(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	LogWriterMu.Lock()
	oldOut := gin.DefaultWriter
	gin.DefaultWriter = logWriteFunc(func(p []byte) (int, error) { close(entered); <-release; return len(p), nil })
	LogWriterMu.Unlock()
	t.Cleanup(func() { LogWriterMu.Lock(); defer LogWriterMu.Unlock(); gin.DefaultWriter = oldOut })
	go func() { _, _ = LogWriter.Write([]byte("in-flight")); close(done) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("write did not start")
	}
	// The sink is in Write now: a rotation must be unable to take the write lock.
	acquired := LogWriterMu.TryLock()
	if acquired {
		LogWriterMu.Unlock()
	}
	close(release)
	<-done
	require.False(t, acquired, "rotation must wait before closing a file still being written")
	require.True(t, LogWriterMu.TryLock())
	LogWriterMu.Unlock()
}
