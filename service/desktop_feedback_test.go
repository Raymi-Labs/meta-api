package service

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func validFeedback() dto.DesktopFeedbackRequest {
	return dto.DesktopFeedbackRequest{ReportID: "a7149591-4139-4c52-a0a2-1fa4ae45958b", Description: "模型切换失败", AppVersion: "0.1.108", Platform: "macos", Context: dto.DesktopFeedbackContext{View: "home", Tool: "dsh"}, Errors: []dto.DesktopFeedbackError{{OccurredAt: "2026-10-26T01:00:00Z", Source: "toast", Message: "error sending request"}}}
}

func TestDesktopFeedbackValidationAndRedaction(t *testing.T) {
	for name, mutate := range map[string]func(*dto.DesktopFeedbackRequest){
		"control":       func(r *dto.DesktopFeedbackRequest) { r.Description = "invalid\x00text" },
		"error-control": func(r *dto.DesktopFeedbackRequest) { r.Errors[0].Message = "bad\u0085text" },
		"empty":         func(r *dto.DesktopFeedbackRequest) { r.Description = "  " },
		"long":          func(r *dto.DesktopFeedbackRequest) { r.Description = strings.Repeat("中", 4001) },
		"bad-id":        func(r *dto.DesktopFeedbackRequest) { r.ReportID = "not-uuid" },
		"nil-id":        func(r *dto.DesktopFeedbackRequest) { r.ReportID = "00000000-0000-0000-0000-000000000000" },
		"version":       func(r *dto.DesktopFeedbackRequest) { r.AppVersion = "path/name" },
		"platform":      func(r *dto.DesktopFeedbackRequest) { r.Platform = "windows11" },
		"view":          func(r *dto.DesktopFeedbackRequest) { r.Context.View = "https://private-url" },
		"too-many":      func(r *dto.DesktopFeedbackRequest) { r.Errors = make([]dto.DesktopFeedbackError, 11) },
		"error-long":    func(r *dto.DesktopFeedbackRequest) { r.Errors[0].Message = strings.Repeat("中", 2001) },
		"error-time":    func(r *dto.DesktopFeedbackRequest) { r.Errors[0].OccurredAt = "yesterday" },
		"error-source":  func(r *dto.DesktopFeedbackRequest) { r.Errors[0].Source = "private/source" },
	} {
		t.Run(name, func(t *testing.T) {
			r := validFeedback()
			mutate(&r)
			_, err := NormalizeDesktopFeedback(r, 7, time.Now())
			require.Error(t, err)
		})
	}
	r := validFeedback()
	r.Description = strings.Repeat("中", 4000)
	r.Errors[0].Message = strings.Repeat("中", 2000)
	row, err := NormalizeDesktopFeedback(r, 7, time.Now())
	require.NoError(t, err)
	require.Equal(t, 7, row.UserID)
	_, err = NormalizeDesktopFeedback(r, 0, time.Now())
	require.Error(t, err)
	raw := "Authorization: Bearer auth-secret\n{\"tokens\":[\"nested-secret\"]}\npassword='password-secret'\nhttps://login:pass@example.com/api/models?group=group-secret#fragment-secret\n/Users/alice/project/name\nC:\\Users\\alice\\secret.txt\nalice@example.com\nsk-opaquecredential\n-----BEGIN PRIVATE KEY-----\nkeycontents\n-----END PRIVATE KEY-----"
	redacted := RedactDesktopFeedbackText(raw, 4000)
	for _, secret := range []string{"auth-secret", "nested-secret", "password-secret", "login:pass", "group-secret", "fragment-secret", "alice", "opaquecredential", "keycontents"} {
		require.NotContains(t, redacted, secret)
	}
	require.Equal(t, "https://example.com/api/models", RedactDesktopFeedbackText("https://login:pass@example.com/api/models?group=group-secret#fragment-secret", 2000))
}

func TestDesktopFeedbackMultilineSecrets(t *testing.T) {
	for _, raw := range []string{
		"{\"tokens\": [\n\"opaque-secret\"\n]}",
		"failure {\"tokens\": [\n\"opaque-secret\"\n]}",
		"password=\"\nopaque-secret\n\"",
		"{\"auth\":{\"nested\":\"opaque-secret\"}}",
	} {
		require.NotContains(t, RedactDesktopFeedbackText(raw, 4000), "opaque-secret")
	}
}
