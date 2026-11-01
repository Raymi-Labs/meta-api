package dto

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
)

// Keep the provider's wire evidence separate from creation counts synthesized
// by other protocol adapters. OpenAI prompt totals already include these writes.
func (d *InputTokenDetails) UnmarshalJSON(data []byte) error {
	type wire InputTokenDetails
	var decoded wire
	if err := common.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.CacheWriteTokens != nil {
		if *decoded.CacheWriteTokens < 0 {
			return errors.New("negative cache_write_tokens")
		}
		decoded.CachedCreationTokens = *decoded.CacheWriteTokens
	}
	*d = InputTokenDetails(decoded)
	return nil
}

// Optional modality counts are subsets of cached_tokens, not extra tokens.
// Pointers distinguish missing data from an authoritative explicit zero.
type CachedTokenDetails struct {
	TextTokens  *int `json:"text_tokens,omitempty"`
	ImageTokens *int `json:"image_tokens,omitempty"`
	AudioTokens *int `json:"audio_tokens,omitempty"`
}

func cloneTokenCount(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (d *CachedTokenDetails) Clone() *CachedTokenDetails {
	if d == nil {
		return nil
	}
	return &CachedTokenDetails{TextTokens: cloneTokenCount(d.TextTokens), ImageTokens: cloneTokenCount(d.ImageTokens), AudioTokens: cloneTokenCount(d.AudioTokens)}
}

func (d InputTokenDetails) Clone() InputTokenDetails {
	d.CachedTokensDetails = d.CachedTokensDetails.Clone()
	d.CacheWriteTokens = cloneTokenCount(d.CacheWriteTokens)
	return d
}

// Add aggregates distinct requests (e.g. automatic continuation). It must not
// be used for cumulative usage snapshots from successive events of one request.
func (d *InputTokenDetails) Add(incoming InputTokenDetails) {
	if incoming.CacheWriteTokens != nil {
		total := *incoming.CacheWriteTokens
		if d.CacheWriteTokens != nil {
			total += *d.CacheWriteTokens
		}
		d.CacheWriteTokens = &total
	} else {
		d.CacheWriteTokens = cloneTokenCount(d.CacheWriteTokens)
	}
	d.CachedTokens += incoming.CachedTokens
	d.CachedCreationTokens += incoming.CachedCreationTokens
	d.TextTokens += incoming.TextTokens
	d.ImageTokens += incoming.ImageTokens
	d.AudioTokens += incoming.AudioTokens
	d.CachedTokensDetails = d.CachedTokensDetails.Clone()
	if incoming.CachedTokensDetails == nil {
		return
	}
	if d.CachedTokensDetails == nil {
		d.CachedTokensDetails = &CachedTokenDetails{}
	}
	add := func(dst **int, src *int) {
		if src == nil {
			return
		}
		value := *src
		if *dst != nil {
			value += **dst
		}
		*dst = &value
	}
	add(&d.CachedTokensDetails.TextTokens, incoming.CachedTokensDetails.TextTokens)
	add(&d.CachedTokensDetails.ImageTokens, incoming.CachedTokensDetails.ImageTokens)
	add(&d.CachedTokensDetails.AudioTokens, incoming.CachedTokensDetails.AudioTokens)
}
