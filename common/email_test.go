package common

import (
	"bytes"
	"mime"
	"net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldUseSMTPLoginAuthRespectsForceSwitch(t *testing.T) {
	originForce := SMTPForceAuthLogin
	originAccount := SMTPAccount
	originServer := SMTPServer
	t.Cleanup(func() {
		SMTPForceAuthLogin = originForce
		SMTPAccount = originAccount
		SMTPServer = originServer
	})

	SMTPForceAuthLogin = true
	SMTPAccount = "user@example.com"
	SMTPServer = "smtp.example.com"

	if !shouldUseSMTPLoginAuth() {
		t.Fatal("expected AUTH LOGIN when SMTPForceAuthLogin is enabled")
	}
}

func TestShouldUseSMTPLoginAuthDetectsKnownServers(t *testing.T) {
	originForce := SMTPForceAuthLogin
	originAccount := SMTPAccount
	originServer := SMTPServer
	t.Cleanup(func() {
		SMTPForceAuthLogin = originForce
		SMTPAccount = originAccount
		SMTPServer = originServer
	})

	SMTPForceAuthLogin = false
	SMTPAccount = "user@example.com"
	SMTPServer = "smtp.sendcloud.net"

	if !shouldUseSMTPLoginAuth() {
		t.Fatal("expected AUTH LOGIN for configured LOGIN-only SMTP server")
	}
}

func TestBuildEmailMessageWithAttachment(t *testing.T) {
	originFrom := SMTPFrom
	originAccount := SMTPAccount
	originServer := SMTPServer
	originSystemName := SystemName
	t.Cleanup(func() {
		SMTPFrom = originFrom
		SMTPAccount = originAccount
		SMTPServer = originServer
		SystemName = originSystemName
	})

	SMTPFrom = "noreply@example.com"
	SMTPAccount = "noreply@example.com"
	SMTPServer = "smtp.example.com"
	SystemName = "NAN"

	message, err := buildEmailMessage("发票已开具", "user@example.com", "<p>ok</p>", []EmailAttachment{{
		Filename:    "invoice.pdf",
		ContentType: "application/pdf",
		Data:        []byte("%PDF-1.4\n"),
	}})
	if err != nil {
		t.Fatalf("buildEmailMessage() error = %v", err)
	}
	raw := string(message)
	for _, want := range []string{
		"Content-Type: multipart/mixed;",
		"Content-Type: text/html; charset=UTF-8",
		"Content-Type: application/pdf; name=invoice.pdf",
		"Content-Disposition: attachment; filename=invoice.pdf",
		"JVBERi0xLjQK",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("message missing %q:\n%s", want, raw)
		}
	}
}

func TestEmailSenderNameIsPerMessage(t *testing.T) {
	oldFrom, oldServer, oldName := SMTPFrom, SMTPServer, SystemName
	t.Cleanup(func() { SMTPFrom, SMTPServer, SystemName = oldFrom, oldServer, oldName })
	SMTPFrom, SMTPServer, SystemName = "notifications@meta-api.com", "smtp.example.invalid", "元衡 API"
	branded, err := buildEmailMessageWithSenderName("[Meta-API团队] 新平台公告", "user@example.invalid", "<p>维护</p>", nil, "Meta-API团队")
	require.NoError(t, err)
	normal, err := buildEmailMessage("验证码", "user@example.invalid", "<p>code</p>", nil)
	require.NoError(t, err)
	for i, raw := range [][]byte{branded, normal} {
		message, err := mail.ReadMessage(bytes.NewReader(raw))
		require.NoError(t, err)
		from, err := mail.ParseAddress(message.Header.Get("From"))
		require.NoError(t, err)
		require.Equal(t, []string{"Meta-API团队", "元衡 API"}[i], from.Name)
		require.Equal(t, "notifications@meta-api.com", from.Address)
		require.Contains(t, message.Header.Get("Message-ID"), "@meta-api.com>")
		subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
		require.NoError(t, err)
		require.Equal(t, []string{"[Meta-API团队] 新平台公告", "验证码"}[i], subject)
	}
	require.Equal(t, "元衡 API", SystemName)
}
