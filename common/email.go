package common

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"slices"
	"strconv"
	"strings"
	"time"
)

func generateMessageID() (string, error) {
	split := strings.Split(SMTPFrom, "@")
	if len(split) < 2 {
		return "", fmt.Errorf("invalid SMTP account")
	}
	domain := strings.Split(SMTPFrom, "@")[1]
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), GetRandomString(12), domain), nil
}

func shouldUseSMTPLoginAuth() bool {
	if SMTPForceAuthLogin {
		return true
	}
	return isOutlookServer(SMTPAccount) || slices.Contains(EmailLoginAuthServerList, SMTPServer)
}

func getSMTPAuth() smtp.Auth {
	if shouldUseSMTPLoginAuth() {
		return LoginAuth(SMTPAccount, SMTPToken)
	}
	return smtp.PlainAuth("", SMTPAccount, SMTPToken, SMTPServer)
}

type EmailAttachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

func SendEmail(subject string, receiver string, content string) error {
	message, err := buildEmailMessage(subject, receiver, content, nil)
	if err != nil {
		return err
	}
	return sendSMTPMessage(receiver, message)
}

func SendEmailWithAttachments(subject string, receiver string, content string, attachments []EmailAttachment) error {
	message, err := buildEmailMessage(subject, receiver, content, attachments)
	if err != nil {
		return err
	}
	return sendSMTPMessage(receiver, message)
}

func buildEmailMessage(subject string, receiver string, content string, attachments []EmailAttachment) ([]byte, error) {
	return buildEmailMessageWithSenderName(subject, receiver, content, attachments, SystemName)
}

func buildEmailMessageWithSenderName(subject string, receiver string, content string, attachments []EmailAttachment, senderName string) ([]byte, error) {
	if SMTPFrom == "" { // for compatibility
		SMTPFrom = SMTPAccount
	}
	id, err2 := generateMessageID()
	if err2 != nil {
		return nil, err2
	}
	if SMTPServer == "" && SMTPAccount == "" {
		return nil, fmt.Errorf("SMTP 服务器未配置")
	}
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	from := mail.Address{Name: senderName, Address: SMTPFrom}
	var encodedContent bytes.Buffer
	quotedPrintableWriter := quotedprintable.NewWriter(&encodedContent)
	if _, err := quotedPrintableWriter.Write([]byte(content)); err != nil {
		return nil, err
	}
	if err := quotedPrintableWriter.Close(); err != nil {
		return nil, err
	}
	if len(attachments) > 0 {
		boundary := fmt.Sprintf("mixed_%s", GetRandomString(12))
		var message bytes.Buffer
		message.WriteString(fmt.Sprintf("To: %s\r\n"+
			"From: %s\r\n"+
			"Subject: %s\r\n"+
			"Date: %s\r\n"+
			"Message-ID: %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: multipart/mixed; boundary=%q\r\n\r\n",
			receiver, from.String(), encodedSubject, time.Now().Format(time.RFC1123Z), id, boundary))
		message.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		message.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		message.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		message.WriteString(encodedContent.String())
		message.WriteString("\r\n")
		for _, attachment := range attachments {
			filename := strings.TrimSpace(attachment.Filename)
			if filename == "" {
				filename = "attachment"
			}
			contentType := strings.TrimSpace(attachment.ContentType)
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			contentTypeHeader := mime.FormatMediaType(contentType, map[string]string{"name": filename})
			if contentTypeHeader == "" {
				contentTypeHeader = contentType
			}
			disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
			message.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			message.WriteString(fmt.Sprintf("Content-Type: %s\r\n", contentTypeHeader))
			message.WriteString("Content-Transfer-Encoding: base64\r\n")
			message.WriteString(fmt.Sprintf("Content-Disposition: %s\r\n\r\n", disposition))
			message.WriteString(wrapBase64(base64.StdEncoding.EncodeToString(attachment.Data)))
			message.WriteString("\r\n")
		}
		message.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
		return message.Bytes(), nil
	}
	message := []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s\r\n"+
		"Subject: %s\r\n"+
		"Date: %s\r\n"+
		"Message-ID: %s\r\n"+ // 添加 Message-ID 头
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n%s\r\n",
		receiver, from.String(), encodedSubject, time.Now().Format(time.RFC1123Z), id, encodedContent.String()))
	return message, nil
}

func wrapBase64(data string) string {
	if len(data) == 0 {
		return ""
	}
	var builder strings.Builder
	for len(data) > 76 {
		builder.WriteString(data[:76])
		builder.WriteString("\r\n")
		data = data[76:]
	}
	builder.WriteString(data)
	builder.WriteString("\r\n")
	return builder.String()
}

// SMTP 发送的重试与超时参数。
// 背景：宿主机上游网络偶发 UDP 丢包，DNS 可能解析超时、或只返回 AAAA
// 而本机无 IPv6 出口（dial network is unreachable），导致用户收不到验证码。
// 策略：拨号限时 + 前两次强制 IPv4 + 网络类错误自动重试（最后一次回退双栈，
// 兼容仅 IPv6 的 SMTP 服务器）。
const (
	smtpMaxAttempts = 3
	smtpDialTimeout = 10 * time.Second
)

func sendSMTPMessage(receiver string, message []byte) error {
	var err error
	for attempt := 1; attempt <= smtpMaxAttempts; attempt++ {
		network := "tcp4"
		if attempt == smtpMaxAttempts {
			network = "tcp" // 最后一次回退双栈，兼容仅 IPv6 的服务器
		}
		err = sendSMTPMessageOnce(network, receiver, message)
		if err == nil {
			return nil
		}
		if !isRetryableSMTPError(err) || attempt == smtpMaxAttempts {
			break
		}
		SysLog(fmt.Sprintf("smtp send attempt %d/%d failed, retrying: %v", attempt, smtpMaxAttempts, err))
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	SysError(fmt.Sprintf("failed to send email to %s: %v", MaskEmail(receiver), err))
	return err
}

// isRetryableSMTPError 仅对网络/解析类瞬时错误重试；
// SMTP 协议层错误（认证失败、收件人被拒等）重试无意义且可能触发风控。
func isRetryableSMTPError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "unexpected EOF")
}

func sendSMTPMessageOnce(network string, receiver string, message []byte) error {
	return sendSMTPMessageAttempt(network, receiver, message, false)
}

func sendSMTPMessageAttempt(network string, receiver string, message []byte, tracked bool) error {
	auth := getSMTPAuth()
	addr := net.JoinHostPort(SMTPServer, strconv.Itoa(SMTPPort))
	dialer := &net.Dialer{Timeout: smtpDialTimeout}

	var client *smtp.Client
	if SMTPPort == 465 || SMTPSSLEnabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         SMTPServer,
		}
		conn, err := tls.DialWithDialer(dialer, network, addr, tlsConfig)
		if err != nil {
			return err
		}
		if tracked {
			_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
		}
		client, err = smtp.NewClient(conn, SMTPServer)
		if err != nil {
			conn.Close()
			return err
		}
	} else {
		conn, err := dialer.Dial(network, addr)
		if err != nil {
			return err
		}
		if tracked {
			_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
		}
		client, err = smtp.NewClient(conn, SMTPServer)
		if err != nil {
			conn.Close()
			return err
		}
		// 587 等明文端口：服务器支持时升级 STARTTLS（与 smtp.SendMail 行为一致）。
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err = client.StartTLS(&tls.Config{ServerName: SMTPServer}); err != nil {
				client.Close()
				return err
			}
		}
	}
	defer client.Close()

	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(SMTPFrom); err != nil {
		return err
	}
	for _, to := range strings.Split(receiver, ";") {
		if err := client.Rcpt(to); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(message); err != nil {
		if tracked {
			return &SMTPOutcomeUnknown{Err: err}
		}
		return err
	}
	if err = w.Close(); err != nil {
		var rejected *textproto.Error
		if tracked && !errors.As(err, &rejected) {
			return &SMTPOutcomeUnknown{Err: err}
		}
		return err
	}
	if tracked {
		return nil
	} // DATA accepted; QUIT is not a delivery acknowledgement.
	return client.Quit()
}

// SMTPOutcomeUnknown prevents retrying mail whose final DATA reply was lost.
type SMTPOutcomeUnknown struct{ Err error }

func (e *SMTPOutcomeUnknown) Error() string { return e.Err.Error() }
func (e *SMTPOutcomeUnknown) Unwrap() error { return e.Err }

// SendEmailTracked performs exactly one SMTP attempt. Durable callers own retry
// scheduling and distinguish SMTP acceptance from end-recipient delivery.
func SendEmailTracked(subject, receiver, content string) error {
	return SendEmailTrackedWithSenderName(subject, receiver, content, SystemName)
}

// SendEmailTrackedWithSenderName overrides only this message's display name,
// preserving the configured SMTP address and the single-attempt delivery contract.
func SendEmailTrackedWithSenderName(subject, receiver, content, senderName string) error {
	message, err := buildEmailMessageWithSenderName(subject, receiver, content, nil, senderName)
	if err != nil {
		return err
	}
	return sendSMTPMessageAttempt("tcp4", receiver, message, true)
}
