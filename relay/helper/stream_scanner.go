package helper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize    = 64 << 10
	DefaultMaxScannerBufferSize = 64 << 20
	DefaultPingInterval         = 10 * time.Second
	MaxPingInterval             = 15 * time.Second
	DefaultStreamingTimeout     = 300 * time.Second
)

func getScannerBufferSize() int {
	if constant.StreamScannerMaxBufferMB > 0 {
		return constant.StreamScannerMaxBufferMB << 20
	}
	return DefaultMaxScannerBufferSize
}

func NewStreamScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, InitialScannerBufferSize), getScannerBufferSize())
	return scanner
}

func NormalizeStreamPingInterval(interval time.Duration) time.Duration {
	if interval <= 0 {
		return DefaultPingInterval
	}
	if interval > MaxPingInterval {
		return MaxPingInterval
	}
	return interval
}

func GetStreamPingConfig(info *relaycommon.RelayInfo) (bool, time.Duration) {
	if info != nil && info.DisablePing {
		return false, 0
	}
	generalSettings := operation_setting.GetGeneralSetting()
	if generalSettings == nil || !generalSettings.PingIntervalEnabled {
		return false, 0
	}
	return true, NormalizeStreamPingInterval(time.Duration(generalSettings.PingIntervalSeconds) * time.Second)
}

type scannedStreamLine struct {
	text string
	err  error
}

// StreamScannerHandler owns all downstream writes on the caller goroutine.
// Its only worker reads the upstream; closing the body and joining that worker
// before returning prevents access to a Gin context after it is recycled.
func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string) bool) {
	if resp == nil || resp.Body == nil || dataHandler == nil {
		return
	}
	StopStreamKeepAlive(c)
	status := relaycommon.NewStreamStatus()
	if info != nil {
		info.StreamStatus = status
	}
	if info == nil || !info.SuppressStreamResponseHeaders {
		SetEventStreamHeaders(c)
	}

	streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second
	if streamingTimeout <= 0 {
		streamingTimeout = DefaultStreamingTimeout
	}
	idle := time.NewTimer(streamingTimeout)
	defer idle.Stop()

	var ping <-chan time.Time
	if enabled, interval := GetStreamPingConfig(info); enabled {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		ping = ticker.C
	}

	ctx, cancel := context.WithCancel(context.Background())
	lines := make(chan scannedStreamLine, 10)
	advance := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer close(lines)
		send := func(line scannedStreamLine) bool {
			select {
			case lines <- line:
				return true
			case <-ctx.Done():
				return false
			}
		}
		defer func() {
			if recovered := recover(); recovered != nil {
				send(scannedStreamLine{err: fmt.Errorf("scanner panic: %v", recovered)})
			}
		}()
		scanner := NewStreamScanner(resp.Body)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			if !send(scannedStreamLine{text: scanner.Text()}) {
				return
			}
			// Do not read a cancellation/error ahead of the event currently being
			// handled: its actual write and usage observation own that ordering.
			select {
			case <-advance:
			case <-ctx.Done():
				return
			}
		}
		// Cleanup closes the upstream body to unblock Read. Its resulting read
		// error is not a new upstream failure.
		if err := scanner.Err(); err != nil && ctx.Err() == nil && err != io.EOF {
			send(scannedStreamLine{err: err})
		}
	}()
	defer func() {
		cancel()
		_ = resp.Body.Close()
		<-readerDone
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			status.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("handler panic: %v", recovered))
			status.RecordError(fmt.Sprintf("handler panic: %v", recovered))
			logger.LogError(c, status.Summary())
		}
	}()

	acknowledge := false
	for {
		if acknowledge {
			advance <- struct{}{}
			acknowledge = false
		}
		select {
		case <-c.Request.Context().Done():
			status.SetEndReason(relaycommon.StreamEndReasonClientGone, c.Request.Context().Err())
			return
		case <-idle.C:
			status.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
			return
		case <-ping:
			var err error
			if info != nil && info.RelayFormat == types.RelayFormatClaude {
				err = ClaudePingData(c)
			} else {
				err = PingData(c)
			}
			if err != nil {
				status.SetEndReason(relaycommon.StreamEndReasonPingFail, err)
				return
			}
		case line, ok := <-lines:
			if !ok {
				status.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
				return
			}
			if line.err != nil {
				if c.Request.Context().Err() != nil || errors.Is(line.err, context.Canceled) {
					status.SetEndReason(relaycommon.StreamEndReasonClientGone, context.Canceled)
				} else {
					status.SetEndReason(relaycommon.StreamEndReasonScannerErr, line.err)
				}
				return
			}
			acknowledge = true
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(streamingTimeout)
			data := strings.TrimSpace(line.text)
			if strings.HasPrefix(data, "data:") {
				data = strings.TrimSpace(strings.TrimPrefix(data, "data:"))
			} else if data != "[DONE]" {
				continue
			}
			if data == "" {
				continue
			}
			if data == "[DONE]" {
				status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
				return
			}
			if info != nil {
				info.SetFirstResponseTime()
				info.ReceivedResponseCount++
			}
			if !dataHandler(data) {
				if status.HasDeliveredCompletion() {
					status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
				} else {
					status.SetEndReason(relaycommon.StreamEndReasonHandlerStop, nil)
				}
				return
			}
		}
	}
}
