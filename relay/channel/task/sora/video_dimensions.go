package sora

import (
	"fmt"
	"strconv"
	"strings"
)

// normalizeVideoDimensions only derives pixels when both axes are unambiguous.
// Unknown resolution tokens remain upstream parameters, never a guessed 720p size.
func normalizeVideoDimensions(body map[string]interface{}, defaultSize string) error {
	read := func(key string) (string, error) {
		raw, ok := body[key]
		if !ok {
			return "", nil
		}
		v, ok := raw.(string)
		if !ok || strings.TrimSpace(v) == "" {
			return "", fmt.Errorf("invalid video %s", key)
		}
		return v, nil
	}
	size, err := read("size")
	if err != nil {
		return err
	}
	ratio, err := read("aspect_ratio")
	if err != nil {
		return err
	}
	resolution, err := read("resolution")
	if err != nil {
		return err
	}
	var w, h, a, b uint64
	if size != "" {
		w, h, err = videoPair(size, "x")
		if err != nil {
			return err
		}
	}
	if ratio != "" {
		a, b, err = videoPair(ratio, ":")
		if err != nil {
			return err
		}
	}
	pixels, known := videoResolutionPixels(resolution)
	if size != "" {
		if ratio != "" && w*b != h*a {
			return fmt.Errorf("conflicting video size and aspect_ratio")
		}
		if known && min(w, h) != pixels {
			return fmt.Errorf("conflicting video size and resolution")
		}
		return nil
	}
	if resolution == "" && ratio == "" {
		body["size"] = defaultSize
		return nil
	}
	if ratio == "" {
		return nil
	} // no orientation was supplied
	if resolution == "" {
		dw, dh, _ := videoPair(defaultSize, "x")
		pixels = min(dw, dh)
		known = true
	}
	if !known {
		return nil
	}
	short := min(a, b)
	// Preserve explicit ratio without rounding it into a different shape.
	if pixels*a%short != 0 || pixels*b%short != 0 {
		return nil
	}
	w, h = pixels*a/short, pixels*b/short
	if w > 0xffffffff || h > 0xffffffff {
		return fmt.Errorf("video dimensions overflow")
	}
	body["size"] = fmt.Sprintf("%dx%d", w, h)
	return nil
}
func videoPair(raw, sep string) (uint64, uint64, error) {
	parts := strings.Split(raw, sep)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid video dimensions")
	}
	values := [2]uint64{}
	for i, p := range parts {
		if p == "" || p[0] == '0' {
			return 0, 0, fmt.Errorf("invalid video dimensions")
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return 0, 0, fmt.Errorf("invalid video dimensions")
			}
		}
		n, e := strconv.ParseUint(p, 10, 32)
		if e != nil || n == 0 {
			return 0, 0, fmt.Errorf("invalid video dimensions")
		}
		values[i] = n
	}
	return values[0], values[1], nil
}
func videoResolutionPixels(raw string) (uint64, bool) {
	if !strings.HasSuffix(raw, "p") {
		return 0, false
	}
	n, e := strconv.ParseUint(strings.TrimSuffix(raw, "p"), 10, 32)
	return n, e == nil && n > 0
}
