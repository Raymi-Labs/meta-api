package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

const descriptionTranslationURL = "https://translate.googleapis.com/translate_a/single"

var descriptionTaskOnce sync.Once
var descriptionHTTPClient = &http.Client{Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// Keep code, identifiers, URLs, variables, and numeric specifications opaque.
var descriptionProtected = regexp.MustCompile("(?s)```.*?```|`[^`]*`|https?://[^\\s<>]+|\\{\\{[^}]*\\}\\}|\\$\\{[^}]*\\}|\\b[A-Za-z][A-Za-z0-9]*[-_][A-Za-z0-9_.:/-]+\\b|\\b(?:OpenAI|ChatGPT|GPT|Claude|Anthropic|Gemini|DeepSeek|Qwen|Grok|Kimi|MiMo|MiniMax|Moonshot|API|SDK|JSON|Python|TypeScript)\\b|[0-9]+(?:[.,][0-9]+)*(?:[KMBkmb])?")
var descriptionMarker = regexp.MustCompile(`ZXV([0-9]{5})ZXV`)

type descriptionContentError string

func (e descriptionContentError) Error() string { return string(e) }

type descriptionPart struct {
	Text   string
	Tokens []string
}

// Chunk before the HTTP request, but never split a protected token. One durable
// worker step translates one chunk, so long descriptions cannot monopolize a lease.
func descriptionParts(source string) []descriptionPart {
	indices := descriptionProtected.FindAllStringIndex(source, -1)
	var parts []descriptionPart
	part := descriptionPart{}
	flush := func() {
		if part.Text != "" {
			parts = append(parts, part)
			part = descriptionPart{}
		}
	}
	plain := func(s string) {
		for len(s) > 0 {
			size := 0
			count := 0
			for i, r := range s {
				if count >= 600 {
					break
				}
				size = i + utf8.RuneLen(r)
				count++
			}
			if size < len(s) {
				boundary := strings.LastIndexAny(s[:size], " \n。！？.;:!?")
				if boundary > size/2 {
					_, width := utf8.DecodeRuneInString(s[boundary:])
					size = boundary + width
				}
			}
			chunk := s[:size]
			s = s[size:]
			if utf8.RuneCountInString(part.Text)+count > 700 {
				flush()
			}
			part.Text += chunk
		}
	}
	last := 0
	for _, span := range indices {
		plain(source[last:span[0]])
		if len(part.Tokens) >= 100 || utf8.RuneCountInString(part.Text) > 650 {
			flush()
		}
		part.Text += fmt.Sprintf("ZXV%05dZXV", len(part.Tokens))
		part.Tokens = append(part.Tokens, source[span[0]:span[1]])
		last = span[1]
	}
	plain(source[last:])
	flush()
	return parts
}
func restoreDescriptionPart(value string, part descriptionPart) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", descriptionContentError("empty translation")
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 4000 {
		return "", descriptionContentError("translation exceeds size limit")
	}
	seen := map[int]int{}
	for _, m := range descriptionMarker.FindAllStringSubmatch(value, -1) {
		index, _ := strconv.Atoi(m[1])
		seen[index]++
	}
	if len(seen) != len(part.Tokens) {
		return "", descriptionContentError("translation changed protected tokens")
	}
	for i := range part.Tokens {
		if seen[i] != 1 {
			return "", descriptionContentError("translation changed protected tokens")
		}
	}
	restored := descriptionMarker.ReplaceAllStringFunc(value, func(s string) string {
		i, _ := strconv.Atoi(descriptionMarker.FindStringSubmatch(s)[1])
		return part.Tokens[i]
	})
	if strings.Contains(restored, "ZXV") && !strings.Contains(part.Text, "ZXVZXV") {
		// Literal marker-like source text is unusual; refuse instead of leaking markers.
		return "", descriptionContentError("translation contains unresolved tokens")
	}
	leading := part.Text[:len(part.Text)-len(strings.TrimLeftFunc(part.Text, unicode.IsSpace))]
	trailing := part.Text[len(strings.TrimRightFunc(part.Text, unicode.IsSpace)):]
	return leading + strings.TrimSpace(restored) + trailing, nil
}

type descriptionTranslateError struct {
	status  int
	retryAt int64
}

func (e *descriptionTranslateError) Error() string {
	return fmt.Sprintf("description translation HTTP %d", e.status)
}
func translateDescriptionPart(ctx context.Context, part descriptionPart, locale string) (string, error) {
	// Code-only fragments do not need a third-party request.
	if strings.TrimSpace(descriptionMarker.ReplaceAllString(part.Text, "")) == "" {
		return restoreDescriptionPart(part.Text, part)
	}
	query := url.Values{"client": {"gtx"}, "sl": {"auto"}, "tl": {locale}, "dt": {"t"}, "q": {part.Text}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, descriptionTranslationURL+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	res, err := descriptionHTTPClient.Do(req)
	if err != nil {
		return "", errors.New("description translation unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		retry := int64(0)
		if seconds, err := strconv.ParseInt(res.Header.Get("Retry-After"), 10, 64); err == nil && seconds > 0 && seconds < 31536000 {
			retry = time.Now().Unix() + seconds
		} else if date, err := http.ParseTime(res.Header.Get("Retry-After")); err == nil {
			retry = date.Unix()
		}
		return "", &descriptionTranslateError{status: res.StatusCode, retryAt: retry}
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var envelope []interface{}
	if common.Unmarshal(body, &envelope) != nil || len(envelope) == 0 {
		return "", descriptionContentError("invalid translation response")
	}
	rows, ok := envelope[0].([]interface{})
	if !ok {
		return "", descriptionContentError("invalid translation segments")
	}
	var out strings.Builder
	for _, raw := range rows {
		row, ok := raw.([]interface{})
		if !ok || len(row) == 0 {
			return "", descriptionContentError("invalid translation segment")
		}
		s, ok := row[0].(string)
		if !ok {
			return "", descriptionContentError("invalid translation text")
		}
		out.WriteString(s)
	}
	if len(envelope) > 2 {
		detected, _ := envelope[2].(string)
		if strings.TrimSpace(out.String()) == strings.TrimSpace(part.Text) && detected != "" && !strings.EqualFold(detected, locale) && !(strings.HasPrefix(detected, "zh") && strings.HasPrefix(locale, "zh")) {
			return "", descriptionContentError("translation returned unchanged source language")
		}
	}
	return restoreDescriptionPart(out.String(), part)
}

// Returns one unpublished chunk. Ordering favors zh-CN then English and all
// remaining site languages, across all models, rather than starving the catalog.
func nextDescriptionTranslation(ctx context.Context, now int64) (*model.ModelDescriptionTranslation, []descriptionPart, error) {
	pricing := model.GetPricing()
	sources := map[string]string{}
	needed := map[string]bool{}
	for _, m := range pricing {
		source := model.CanonicalModelDescription(m.Description)
		if source == "" || utf8.RuneCountInString(source) > 16000 {
			continue
		}
		hash := model.ModelDescriptionHash(source)
		sources[hash] = source
		for _, locale := range model.ModelDescriptionLocales {
			if strings.TrimSpace(m.DescriptionTranslations[locale]) == "" {
				needed[hash+":"+locale] = true
			}
		}
	}
	hashes := make([]string, 0, len(sources))
	for hash := range sources {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	cache := map[string]model.ModelDescriptionTranslation{}
	for start := 0; start < len(hashes); start += 100 {
		var rows []model.ModelDescriptionTranslation
		if err := model.DB.WithContext(ctx).Where("source_hash IN ?", hashes[start:min(start+100, len(hashes))]).Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		for _, r := range rows {
			cache[r.CacheKey] = r
		}
	}
	locales := append([]string{"zh-CN", "en"}, model.ModelDescriptionLocales[2:]...)
	for _, locale := range locales {
		for _, hash := range hashes {
			key := hash + ":" + locale
			if !needed[key] {
				continue
			}
			row := cache[key]
			if row.Completed || row.NextAttemptAt > now {
				continue
			}
			row.CacheKey = key
			row.SourceHash = hash
			row.Locale = locale
			parts := descriptionParts(sources[hash])
			if row.Chunk >= len(parts) {
				continue
			}
			return &row, parts, nil
		}
	}
	return nil, nil, nil
}
func descriptionBackoff(failures int, limited bool) time.Duration {
	base := 5 * time.Minute
	capDelay := time.Hour
	if limited {
		base = 30 * time.Minute
		capDelay = 6 * time.Hour
	}
	return min(base*time.Duration(1<<min(max(failures-1, 0), 5)), capDelay)
}
func runDescriptionTranslationStep(ctx context.Context) error {
	now := time.Now().Unix()
	lease := uuid.NewString()
	state, claimed, err := model.ClaimModelDescriptionWorker(ctx, now, lease)
	if err != nil || !claimed {
		return err
	}
	next := now + 30
	failures := state.Failures
	defer func() {
		finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = model.FinishModelDescriptionWorker(finishCtx, lease, next, failures)
	}()
	row, parts, err := nextDescriptionTranslation(ctx, now)
	if err != nil {
		return err
	}
	if row == nil {
		return nil
	}
	value, err := translateDescriptionPart(ctx, parts[row.Chunk], row.Locale)
	if err == nil && len(row.Text)+len(value) > 64000 {
		// Stay below MySQL TEXT's byte limit on every supported database.
		err = descriptionContentError("complete translation exceeds storage limit")
	}
	next = time.Now().Unix() + 5
	if err != nil {
		row.Failures++
		limited := false
		var httpErr *descriptionTranslateError
		if errors.As(err, &httpErr) {
			limited = httpErr.status == 403 || httpErr.status == 429
		}
		failures++
		next = time.Now().Add(descriptionBackoff(failures, limited)).Unix()
		if httpErr != nil {
			next = max(next, httpErr.retryAt)
		}
		row.NextAttemptAt = next
		var contentErr descriptionContentError
		if errors.As(err, &contentErr) || (httpErr != nil && httpErr.status >= 400 && httpErr.status < 500 && !limited) {
			// A malformed translation must not starve every other model/language.
			row.NextAttemptAt = time.Now().Add(24 * time.Hour).Unix()
			next = time.Now().Unix() + 5
			failures = 0
		}
		if saveErr := model.DB.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(row).Error; saveErr != nil {
			return saveErr
		}
		return err
	}
	failures = 0
	row.Failures = 0
	row.NextAttemptAt = 0
	row.Text += value
	row.Chunk++
	row.Completed = row.Chunk == len(parts)
	if err := model.DB.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(row).Error; err != nil {
		return err
	}
	if row.Completed {
		model.RefreshPricing()
	}
	return nil
}
func StartModelDescriptionTranslationTask() {
	descriptionTaskOnce.Do(func() {
		if !common.IsMasterNode || !common.GetEnvOrDefaultBool("MODEL_DESCRIPTION_TRANSLATION_ENABLED", true) {
			return
		}
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				err := runDescriptionTranslationStep(ctx)
				cancel()
				if err != nil {
					common.SysError("model description translation deferred: " + err.Error())
				}
			}
		}()
	})
}
