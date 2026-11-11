package dto

import (
	"net/url"
	"regexp"
)

var videoSecondsPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)
var videoSizePattern = regexp.MustCompile(`^[0-9]+x[0-9]+$`)

// VideoContentPath is relative to the gateway origin, never to the provider.
func VideoContentPath(taskID string) string {
	return "/v1/videos/" + url.PathEscape(taskID) + "/content"
}

// PublicVideo returns an allowlisted view. Provider metadata, IDs, errors and
// unknown extension fields must never cross the public response boundary.
func PublicVideo(v *OpenAIVideo, taskID, model string) *OpenAIVideo {
	out := &OpenAIVideo{ID: taskID, TaskID: taskID, Object: "video", Model: model,
		Progress: v.Progress, CreatedAt: v.CreatedAt, CompletedAt: v.CompletedAt, ExpiresAt: v.ExpiresAt}
	switch v.Status {
	case VideoStatusUnknown, VideoStatusQueued, VideoStatusInProgress, VideoStatusCompleted, VideoStatusFailed:
		out.Status = v.Status
	default:
		out.Status = VideoStatusQueued
	}
	if out.Progress < 0 {
		out.Progress = 0
	}
	if out.Progress > 100 {
		out.Progress = 100
	}
	if videoSecondsPattern.MatchString(v.Seconds) {
		out.Seconds = v.Seconds
	}
	if videoSizePattern.MatchString(v.Size) {
		out.Size = v.Size
	}
	if out.Status == VideoStatusCompleted {
		out.SetMetadata("url", VideoContentPath(taskID))
	}
	if out.Status == VideoStatusFailed || v.Error != nil {
		out.Error = &OpenAIVideoError{Code: "video_generation_failed", Message: "Video generation failed; contact support with the task ID"}
	}
	return out
}
