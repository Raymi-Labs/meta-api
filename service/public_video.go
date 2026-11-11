package service

import (
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// VideoContentURL uses the administrator-configured public gateway origin.
// Never derive a public result from provider URLs or untrusted forwarded headers.
func VideoContentURL(taskID string) string {
	path := dto.VideoContentPath(taskID)
	// Reuse the established exact company-origin mapping for public video
	// links without changing ServerAddress or unrelated callback configuration.
	origin, err := url.Parse(common.CanonicalLoginOrigin(system_setting.ServerAddress))
	if err != nil || (origin.Scheme != "https" && origin.Scheme != "http") || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" {
		return path
	}
	return origin.Scheme + "://" + origin.Host + path
}

func PublicVideo(v *dto.OpenAIVideo, taskID, model string) *dto.OpenAIVideo {
	out := dto.PublicVideo(v, taskID, model)
	if out.Status == dto.VideoStatusCompleted {
		out.SetMetadata("url", VideoContentURL(taskID))
	}
	return out
}
