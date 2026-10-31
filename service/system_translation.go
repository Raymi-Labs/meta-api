package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// English language names are the Qwen MT contract, not UI locale identifiers.
var systemTranslationLanguages = map[string]string{
	"en": "English", "zh-CN": "Chinese", "zh-TW": "Traditional Chinese",
	"ja": "Japanese", "ko": "Korean", "de": "German", "es": "Spanish",
	"fr": "French", "pt": "Portuguese", "ru": "Russian", "tr": "Turkish",
	"pl": "Polish", "nl": "Dutch", "it": "Italian", "vi": "Vietnamese",
	"uk": "Ukrainian", "bg": "Bulgarian",
}

// Loopback credentials must never follow environment proxies or redirects.
var systemTranslationClient = func() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Transport: transport, Timeout: 50 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}()

type SystemTranslationHTTPError struct {
	Status  int
	RetryAt int64
}

func (e *SystemTranslationHTTPError) Error() string {
	return fmt.Sprintf("system translation HTTP %d", e.Status)
}

type SystemTranslationContentError string

func (e SystemTranslationContentError) Error() string { return string(e) }

// TranslateSystemText reuses the ordinary authenticated relay, routing, quotas,
// and usage logs. Callers must cache results; this function does not retry.
func TranslateSystemText(ctx context.Context, text, locale string) (string, error) {
	target, ok := systemTranslationLanguages[locale]
	if !ok {
		return "", SystemTranslationContentError("unsupported translation locale")
	}
	token, err := model.ValidateSystemTranslationToken(ctx, model.SystemTranslationTokenID(), 0)
	if err != nil {
		return "", err
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n > 65535 {
		return "", errors.New("invalid local relay port")
	}
	payload := map[string]any{
		"model":               model.SystemTranslationModel,
		"messages":            []map[string]string{{"role": "user", "content": text}},
		"translation_options": map[string]string{"source_lang": "auto", "target_lang": target},
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return "", errors.New("cannot encode translation request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(n)+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", errors.New("cannot create translation request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-"+token.Key)
	req.Header.Set("User-Agent", "Meta-API-System-Translation")
	res, err := systemTranslationClient.Do(req)
	if err != nil {
		return "", errors.New("system translation relay unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		retry := int64(0)
		if seconds, err := strconv.ParseInt(res.Header.Get("Retry-After"), 10, 64); err == nil && seconds > 0 && seconds < 31536000 {
			retry = time.Now().Unix() + seconds
		} else if date, err := http.ParseTime(res.Header.Get("Retry-After")); err == nil {
			retry = date.Unix()
		}
		return "", &SystemTranslationHTTPError{Status: res.StatusCode, RetryAt: retry}
	}
	body, err = io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil {
		return "", errors.New("cannot read translation response")
	}
	if len(body) > 1<<20 {
		return "", SystemTranslationContentError("translation response exceeds size limit")
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if common.Unmarshal(body, &result) != nil || len(result.Choices) != 1 || result.Choices[0].FinishReason != "stop" {
		return "", SystemTranslationContentError("invalid or incomplete translation response")
	}
	value := result.Choices[0].Message.Content
	if strings.TrimSpace(value) == "" || !utf8.ValidString(value) {
		return "", SystemTranslationContentError("empty or invalid translation text")
	}
	return value, nil
}
