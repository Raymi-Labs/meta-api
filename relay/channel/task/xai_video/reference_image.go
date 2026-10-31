package xai_video

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

func referenceRequestError(err error) *dto.TaskError {
	return &dto.TaskError{Code: "invalid_request", Message: err.Error(), StatusCode: http.StatusBadRequest, LocalError: true, Error: err}
}

// normalizeReferenceImage never fetches media or drops conflicting inputs.
// The shared task DTO remains string-based for other video providers.
func normalizeReferenceImage(payload map[string]any) (string, error) {
	if _, exists := payload["images"]; exists {
		return "", fmt.Errorf("xAI first-frame input uses image.url or input_reference, not images")
	}
	reference := ""
	for _, field := range []string{"image", "input_reference"} {
		raw, exists := payload[field]
		if !exists {
			continue
		}
		value, ok := raw.(string)
		if field == "image" && !ok {
			object, valid := raw.(map[string]any)
			if !valid || len(object) != 1 {
				return "", fmt.Errorf("image must be a URL string or an object containing only url")
			}
			value, ok = object["url"].(string)
		}
		if !ok || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("%s must contain a non-empty image URL or data URI", field)
		}
		value = strings.TrimSpace(value)
		if reference != "" && reference != value {
			return "", fmt.Errorf("conflicting image and input_reference")
		}
		if strings.HasPrefix(value, "data:") {
			header, encoded, found := strings.Cut(value, ",")
			if !found || encoded == "" || (header != "data:image/png;base64" && header != "data:image/jpeg;base64" && header != "data:image/webp;base64") {
				return "", fmt.Errorf("%s must contain PNG, JPEG or WebP base64 image data", field)
			}
			count, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(encoded)))
			if err != nil || count == 0 {
				return "", fmt.Errorf("%s contains invalid base64 image data", field)
			}
		} else {
			parsed, err := url.Parse(value)
			if err != nil || parsed == nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" || parsed.User != nil {
				return "", fmt.Errorf("%s must contain an HTTP(S) image URL or base64 image data URI", field)
			}
		}
		reference = value
	}
	if reference != "" {
		payload["image"] = map[string]any{"url": reference}
	}
	delete(payload, "input_reference")
	return reference, nil
}
