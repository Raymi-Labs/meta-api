package service

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
)

var desktopFeedbackIdentifier = regexp.MustCompile(`^[a-zA-Z0-9._-]{0,64}$`)
var desktopFeedbackVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,31}$`)
var desktopFeedbackHeaders = regexp.MustCompile(`(?im)(?:\b(?:proxy-)?authorization|\bset-cookie|\bcookie|\bx-api-key)\s*[:=][^\r\n]+`)
var desktopFeedbackNamedSecret = regexp.MustCompile(`(?is)(?:\\?["'])?\b(?:api[_ -]?keys?|access[_ -]?tokens?|refresh[_ -]?tokens?|session[_ -]?(?:id|tokens?)|client[_ -]?secret|credentials?|password|passwd|secret|tokens?|auth|pwd)(?:\\?["'])?\s*[:=].*$`)
var desktopFeedbackAuth = regexp.MustCompile(`(?i)\b(?:Bearer|Basic|Token|ApiKey)\s+[^\s"',}\]]+`)
var desktopFeedbackSecret = regexp.MustCompile(`(?i)(?:sk-[a-z0-9._~+/=-]{6,}|github_pat_[a-z0-9_]{6,}|gh[pousr]_[a-z0-9_]{6,}|AIza[a-z0-9_-]{8,}|eyJ[a-z0-9_-]+\.[a-z0-9_-]+\.[a-z0-9_-]+)`)
var desktopFeedbackPath = regexp.MustCompile(`(?i)(?:\b[a-z]:[\\/]|/(?:Users|home|private|tmp|var)/|~/|\\\\)[^\r\n"'<>]+`)
var desktopFeedbackPrivateKey = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`)

func ValidDesktopFeedbackPlatform(value string) bool {
	return value == "windows" || value == "macos" || value == "linux" || value == "unknown"
}

// Bounded input is required before this defense-in-depth redaction. It is not a
// promise to identify arbitrary secrets in prose; the client also previews it.
func RedactDesktopFeedbackText(value string, limit int) string {
	// First preserve useful JSON fields while replacing the entire value of a
	// sensitive key, including arrays and nested objects. An invalid/truncated
	// JSON fragment falls through to conservative tail omission below.
	var structured interface{}
	if common.UnmarshalJsonStr(value, &structured) == nil {
		redactDesktopFeedbackObject(structured)
		if encoded, err := common.Marshal(structured); err == nil {
			value = string(encoded)
		}
	}
	value = desktopFeedbackPrivateKey.ReplaceAllString(value, "[REDACTED]")
	value = desktopFeedbackHeaders.ReplaceAllString(value, "[REDACTED]")
	value = desktopFeedbackNamedSecret.ReplaceAllString(value, "[REDACTED]")
	value = desktopFeedbackAuth.ReplaceAllString(value, "[REDACTED]")
	value = desktopFeedbackSecret.ReplaceAllString(value, "[REDACTED]")
	value = contentSafetyURLPattern.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			return "[REDACTED URL]"
		}
		parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
		parsed.ForceQuery = false
		return parsed.String()
	})
	value = contentSafetyEmailPattern.ReplaceAllString(value, "[REDACTED EMAIL]")
	value = desktopFeedbackPath.ReplaceAllString(value, "[REDACTED PATH]")
	return truncateContentSafetyText(strings.TrimSpace(value), limit)
}

func redactDesktopFeedbackObject(value interface{}) {
	switch item := value.(type) {
	case map[string]interface{}:
		for key, child := range item {
			normalized := strings.ToLower(strings.Map(func(r rune) rune {
				if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
					return r
				}
				return -1
			}, key))
			normalized = strings.TrimSuffix(normalized, "s")
			switch normalized {
			case "key", "apikey", "accesskey", "secretkey", "privatekey", "clientsecret", "token", "accesstoken", "refreshtoken", "sessiontoken", "sessionid", "authorization", "auth", "password", "passwd", "secret", "credential", "cookie", "pwd":
				item[key] = "[REDACTED]"
			default:
				redactDesktopFeedbackObject(child)
			}
		}
	case []interface{}:
		for _, child := range item {
			redactDesktopFeedbackObject(child)
		}
	}
}

func validDesktopFeedbackText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

func NormalizeDesktopFeedback(request dto.DesktopFeedbackRequest, userID int, now time.Time) (*model.DesktopFeedback, error) {
	invalid := errors.New("反馈内容无效，请检查描述、版本及错误信息长度")
	parsed, err := uuid.Parse(request.ReportID)
	if err != nil || parsed == uuid.Nil || !strings.EqualFold(parsed.String(), request.ReportID) || userID <= 0 {
		return nil, invalid
	}
	request.Description = strings.TrimSpace(request.Description)
	if !validDesktopFeedbackText(request.Description) || utf8.RuneCountInString(request.Description) < 1 || utf8.RuneCountInString(request.Description) > 4000 ||
		!desktopFeedbackVersion.MatchString(request.AppVersion) || !ValidDesktopFeedbackPlatform(request.Platform) ||
		!desktopFeedbackIdentifier.MatchString(request.Context.View) || !desktopFeedbackIdentifier.MatchString(request.Context.Tool) || len(request.Errors) > 10 {
		return nil, invalid
	}
	errorsToStore := make([]dto.DesktopFeedbackError, 0, len(request.Errors))
	for _, item := range request.Errors {
		at, err := time.Parse(time.RFC3339, item.OccurredAt)
		if err != nil || len(item.OccurredAt) > 40 || !desktopFeedbackIdentifier.MatchString(item.Source) || !validDesktopFeedbackText(item.Message) || utf8.RuneCountInString(item.Message) > 2000 {
			return nil, invalid
		}
		errorsToStore = append(errorsToStore, dto.DesktopFeedbackError{
			OccurredAt: at.UTC().Format(time.RFC3339), Source: item.Source, Message: RedactDesktopFeedbackText(item.Message, 2000),
		})
	}
	encoded, err := common.Marshal(errorsToStore)
	if err != nil {
		return nil, err
	}
	return &model.DesktopFeedback{
		UserID: userID, ReportID: parsed.String(), Description: RedactDesktopFeedbackText(request.Description, 4000),
		AppVersion: request.AppVersion, Platform: request.Platform, View: request.Context.View, Tool: request.Context.Tool,
		Errors: string(encoded), Status: model.DesktopFeedbackPending, CreatedAt: now.Unix(), UpdatedAt: now.Unix(),
	}, nil
}

func DesktopFeedbackDetail(row *model.DesktopFeedback) (*dto.DesktopFeedbackDetail, error) {
	items := make([]dto.DesktopFeedbackError, 0)
	if err := common.UnmarshalJsonStr(row.Errors, &items); err != nil {
		return nil, err
	}
	return &dto.DesktopFeedbackDetail{
		DesktopFeedbackReceipt: dto.DesktopFeedbackReceipt{ID: row.ID, Status: row.Status, CreatedAt: row.CreatedAt},
		DesktopFeedbackRequest: dto.DesktopFeedbackRequest{
			ReportID: row.ReportID, Description: row.Description, AppVersion: row.AppVersion, Platform: row.Platform,
			Context: dto.DesktopFeedbackContext{View: row.View, Tool: row.Tool}, Errors: items,
		},
		UserID: row.UserID, UpdatedAt: row.UpdatedAt,
	}, nil
}
