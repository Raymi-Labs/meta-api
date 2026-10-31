package logger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func useRotationTestLogger(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	setupLogLock.Lock()
	common.LogWriterMu.Lock()
	oldDir, oldOut, oldErr, oldFile := *common.LogDir, gin.DefaultWriter, gin.DefaultErrorWriter, currentLogFile
	oldCount := logCount.Swap(0)
	*common.LogDir, currentLogFile = dir, nil
	common.LogWriterMu.Unlock()
	setupLogLock.Unlock()
	t.Cleanup(func() {
		setupLogLock.Lock()
		defer setupLogLock.Unlock()
		common.LogWriterMu.Lock()
		defer common.LogWriterMu.Unlock()
		if currentLogFile != nil {
			require.NoError(t, currentLogFile.Close())
		}
		*common.LogDir, gin.DefaultWriter, gin.DefaultErrorWriter, currentLogFile = oldDir, oldOut, oldErr, oldFile
		logCount.Store(oldCount)
	})
	return dir
}

func TestSetupLoggerClosesOnlyOwnedFiles(t *testing.T) {
	useRotationTestLogger(t)
	external, err := os.CreateTemp(t.TempDir(), "external-sink")
	require.NoError(t, err)
	defer external.Close()
	common.LogWriterMu.Lock()
	gin.DefaultWriter, gin.DefaultErrorWriter = external, external
	common.LogWriterMu.Unlock()
	SetupLogger()
	first := currentLogFile
	require.NotNil(t, first)
	common.SysLog("first-file-only")
	*common.LogDir = t.TempDir()
	SetupLogger()
	second := currentLogFile
	require.NotSame(t, first, second)
	_, err = first.Stat()
	require.ErrorIs(t, err, os.ErrClosed)
	_, err = external.WriteString("caller-still-owns-this-file")
	require.NoError(t, err, "rotation must not close external sinks or stdout/stderr")
	LogInfo(context.Background(), "second-file-only")
	LogError(context.Background(), "second-error-only")
	common.SysError("second-system-only")
	firstBytes, err := os.ReadFile(first.Name())
	require.NoError(t, err)
	require.Contains(t, string(firstBytes), "first-file-only")
	require.NotContains(t, string(firstBytes), "second-")
	secondBytes, err := os.ReadFile(second.Name())
	require.NoError(t, err)
	for _, marker := range []string{"second-file-only", "second-error-only", "second-system-only"} {
		require.Contains(t, string(secondBytes), marker)
	}
}

func TestSetupLoggerConcurrentWritesAndRotation(t *testing.T) {
	dir := useRotationTestLogger(t)
	SetupLogger()
	var writers sync.WaitGroup
	start := make(chan struct{})
	for worker := 0; worker < 8; worker++ {
		writers.Add(1)
		go func(worker int) {
			defer writers.Done()
			<-start
			for i := 0; i < 32; i++ {
				message := fmt.Sprintf("rotation-record-%03d!", worker*32+i)
				switch i % 4 {
				case 0:
					common.SysLog(message)
				case 1:
					common.SysError(message)
				case 2:
					LogInfo(context.Background(), message)
				case 3:
					LogError(context.Background(), message)
				}
			}
		}(worker)
	}
	close(start)
	var retired []*os.File
	for i := 0; i < 16; i++ {
		retired = append(retired, currentLogFile)
		SetupLogger()
	}
	writers.Wait()
	for _, file := range retired {
		_, err := file.Stat()
		require.ErrorIs(t, err, os.ErrClosed)
	}
	files, err := filepath.Glob(filepath.Join(dir, "oneapi-*.log"))
	require.NoError(t, err)
	var content strings.Builder
	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		content.Write(data)
	}
	for i := 0; i < 256; i++ {
		require.Equal(t, 1, strings.Count(content.String(), fmt.Sprintf("rotation-record-%03d!", i)), "each completed log write must appear exactly once")
	}
}
