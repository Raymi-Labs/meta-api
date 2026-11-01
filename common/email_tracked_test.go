package common

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestQuotaNoticeSMTPOutcome(t *testing.T) {
	oldServer, oldPort, oldSSL, oldFrom := SMTPServer, SMTPPort, SMTPSSLEnabled, SMTPFrom
	t.Cleanup(func() { SMTPServer, SMTPPort, SMTPSSLEnabled, SMTPFrom = oldServer, oldPort, oldSSL, oldFrom })
	for _, outcome := range []string{"accepted", "rejected", "lost_reply"} {
		t.Run(outcome, func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			SMTPServer = "127.0.0.1"
			SMTPPort = listener.Addr().(*net.TCPAddr).Port
			SMTPSSLEnabled = false
			SMTPFrom = "sender@example.invalid"
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, e := listener.Accept()
				if e != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				tp := textproto.NewConn(conn)
				_ = tp.PrintfLine("220 fixture")
				for {
					line, e := tp.ReadLine()
					if e != nil {
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
						_ = tp.PrintfLine("250 fixture")
					case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
						_ = tp.PrintfLine("250 OK")
					case line == "DATA":
						_ = tp.PrintfLine("354 go")
						_, e = tp.ReadDotBytes()
						if e != nil {
							return
						}
						if outcome == "lost_reply" {
							return
						}
						if outcome == "rejected" {
							_ = tp.PrintfLine("550 rejected")
						} else {
							_ = tp.PrintfLine("250 queued")
						}
						return
					default:
						_ = tp.PrintfLine("500 unexpected")
					}
				}
			}()
			err = SendEmailTrackedWithSenderName("公告", "recipient@example.invalid", "fixture", "Meta-API团队")
			var unknown *SMTPOutcomeUnknown
			var rejected *textproto.Error
			switch outcome {
			case "accepted":
				require.NoError(t, err)
			case "rejected":
				require.True(t, errors.As(err, &rejected), fmt.Sprint(err))
				require.False(t, errors.As(err, &unknown))
			case "lost_reply":
				require.True(t, errors.As(err, &unknown), fmt.Sprint(err))
			}
			<-done
		})
	}
}
