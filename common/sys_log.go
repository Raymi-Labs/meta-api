package common

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LogWriterMu protects the full write, not just the writer pointer. Rotation
// must hold its write lock while swapping writers and closing the previous file.
var LogWriterMu sync.RWMutex

// Keep these proxies stable: Gin middleware captures its writer at startup.
// Do not assign the proxies themselves to gin.DefaultWriter/DefaultErrorWriter.
var (
	LogWriter      io.Writer = currentLogWriter{}
	ErrorLogWriter io.Writer = currentLogWriter{errorStream: true}
)

type currentLogWriter struct {
	errorStream bool
}

func (w currentLogWriter) Write(p []byte) (int, error) {
	LogWriterMu.RLock()
	defer LogWriterMu.RUnlock()
	if w.errorStream {
		return gin.DefaultErrorWriter.Write(p)
	}
	return gin.DefaultWriter.Write(p)
}

func SysLog(s string) {
	t := time.Now()
	_, _ = fmt.Fprintf(LogWriter, "[SYS] %v | %s \n", t.Format("2006/01/02 - 15:04:05"), s)
}

func SysError(s string) {
	t := time.Now()
	_, _ = fmt.Fprintf(ErrorLogWriter, "[SYS] %v | %s \n", t.Format("2006/01/02 - 15:04:05"), s)
}

func FatalLog(v ...any) {
	t := time.Now()
	_, _ = fmt.Fprintf(ErrorLogWriter, "[FATAL] %v | %v \n", t.Format("2006/01/02 - 15:04:05"), v)
	os.Exit(1)
}

func LogStartupSuccess(startTime time.Time, port string) {

	duration := time.Since(startTime)
	durationMs := duration.Milliseconds()

	// Get network IPs
	networkIps := GetNetworkIps()

	// Print blank line for spacing
	fmt.Fprintf(LogWriter, "\n")

	// Print the main success message
	fmt.Fprintf(LogWriter, "  \033[32m%s %s\033[0m  ready in %d ms\n", SystemName, Version, durationMs)
	fmt.Fprintf(LogWriter, "\n")

	// Skip fancy startup message in container environments
	if !IsRunningInContainer() {
		// Print local URL
		fmt.Fprintf(LogWriter, "  ➜  \033[1mLocal:\033[0m   http://localhost:%s/\n", port)
	}

	// Print network URLs
	for _, ip := range networkIps {
		fmt.Fprintf(LogWriter, "  ➜  \033[1mNetwork:\033[0m http://%s:%s/\n", ip, port)
	}

	// Print blank line for spacing
	fmt.Fprintf(LogWriter, "\n")
}
