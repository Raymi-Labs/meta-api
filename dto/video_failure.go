package dto

import "strings"

// PublicVideoFailure returns only fixed gateway messages, never provider prose.
func PublicVideoFailure(code string) *OpenAIVideoError {
	messages := map[string]string{
		"video_input_rejected":     "Input content did not pass review; check the prompt and photos",
		"video_output_rejected":    "Generated content did not pass review",
		"video_invalid_input":      "Video parameters or photo format were rejected",
		"video_access_unavailable": "This channel is not currently authorized or available; contact support with the task ID",
		"video_cancelled":          "The upstream video task was cancelled",
		"video_expired":            "The upstream video task expired",
	}
	if message, ok := messages[code]; ok {
		return &OpenAIVideoError{Code: code, Message: message}
	}
	return &OpenAIVideoError{Code: "video_generation_failed", Message: "Video generation failed; contact support with the task ID"}
}

// SeedanceVideoFailure classifies documented codes without parsing or echoing
// sensitive provider messages. Unknown codes retain the generic fallback.
func SeedanceVideoFailure(status, code string) *OpenAIVideoError {
	if status == "cancelled" {
		return PublicVideoFailure("video_cancelled")
	}
	if status == "expired" {
		return PublicVideoFailure("video_expired")
	}
	if status != "failed" {
		return nil
	}
	root, _, _ := strings.Cut(code, ".")
	category := ""
	switch root {
	case "SensitiveContentDetected", "InputTextSensitiveContentDetected", "InputImageSensitiveContentDetected", "InputVideoSensitiveContentDetected", "InputAudioSensitiveContentDetected", "InputTextRiskDetection", "InputImageRiskDetection":
		category = "video_input_rejected"
	case "OutputTextSensitiveContentDetected", "OutputImageSensitiveContentDetected", "OutputVideoSensitiveContentDetected", "OutputAudioSensitiveContentDetected", "OutputTextRiskDetection", "OutputImageRiskDetection":
		category = "video_output_rejected"
	case "InvalidParameter", "MissingParameter", "InvalidImageURL":
		category = "video_invalid_input"
	case "AuthenticationError", "InvalidAccountStatus", "InvalidEndpoint":
		category = "video_access_unavailable"
	}
	switch code {
	case "OperationDenied.PermissionDenied", "OperationDenied.ServiceNotOpen", "OperationDenied.ServiceOverdue":
		category = "video_access_unavailable"
	}
	return PublicVideoFailure(category)
}
