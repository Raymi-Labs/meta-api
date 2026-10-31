package sora

import (
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// normalizeMuseRequest implements the supplier's JSON contract only for Muse.
// Remote image sizes remain the supplier's responsibility; never fetch them here.
func normalizeMuseRequest(body map[string]interface{}) error {
	prompt, ok := body["prompt"].(string)
	if !ok || strings.TrimSpace(prompt) == "" || utf8.RuneCountInString(prompt) > 4000 {
		return fmt.Errorf("Muse requires a prompt of 1-4000 characters")
	}
	seconds := 4
	explicit := false
	for _, field := range []string{"seconds", "duration"} {
		raw, exists := body[field]
		if !exists {
			continue
		}
		var n float64
		switch v := raw.(type) {
		case float64:
			n = v
		case string:
			parsed, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("Muse seconds must be an integer from 4 to 180")
			}
			n = float64(parsed)
		default:
			return fmt.Errorf("Muse seconds must be an integer from 4 to 180")
		}
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 4 || n > 180 || math.Trunc(n) != n {
			return fmt.Errorf("Muse seconds must be an integer from 4 to 180")
		}
		if explicit && seconds != int(n) {
			return fmt.Errorf("conflicting Muse duration and seconds")
		}
		seconds = int(n)
		explicit = true
	}
	body["seconds"] = strconv.Itoa(seconds)
	delete(body, "duration")
	size := "1280x720"
	if v, exists := body["size"]; exists {
		var valid bool
		size, valid = v.(string)
		if !valid {
			return fmt.Errorf("invalid Muse size")
		}
	}
	if size != "1280x720" && size != "720x1280" {
		return fmt.Errorf("Muse size must be 1280x720 or 720x1280")
	}
	body["size"] = size
	if r, exists := body["resolution"]; exists && r != "720p" {
		return fmt.Errorf("Muse resolution must be 720p")
	}
	if _, exists := body["image"]; exists {
		return fmt.Errorf("Muse uses input_reference or images, not image")
	}
	_, single := body["input_reference"]
	_, multiple := body["images"]
	if single && multiple {
		return fmt.Errorf("use either Muse input_reference or images, not both")
	}
	refs := []interface{}{}
	if single {
		refs = append(refs, body["input_reference"])
	}
	if multiple {
		var valid bool
		refs, valid = body["images"].([]interface{})
		if !valid || len(refs) < 1 || len(refs) > 4 {
			return fmt.Errorf("Muse images must contain 1-4 references")
		}
	}
	var total int64
	for _, raw := range refs {
		value, valid := raw.(string)
		if !valid || strings.TrimSpace(value) == "" {
			return fmt.Errorf("Muse references must be image URLs or data URIs")
		}
		if strings.HasPrefix(value, "data:") {
			header, encoded, found := strings.Cut(value, ",")
			if !found || (header != "data:image/png;base64" && header != "data:image/jpeg;base64" && header != "data:image/webp;base64") {
				return fmt.Errorf("Muse references support PNG, JPEG or WebP data URIs")
			}
			reader := base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(encoded))
			count, err := io.Copy(io.Discard, io.LimitReader(reader, 10_000_001))
			if err != nil || count == 0 || count > 10_000_000 {
				return fmt.Errorf("Muse reference is invalid or exceeds 10 MB")
			}
			total += count
			if total > 24_000_000 {
				return fmt.Errorf("Muse references exceed 24 MB total")
			}
		} else {
			u, err := url.Parse(value)
			if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
				return fmt.Errorf("Muse reference must be an HTTP(S) image URL or data URI")
			}
		}
	}
	return nil
}
