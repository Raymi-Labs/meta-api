package service

import (
	"regexp"
	"strings"
)

// Match explicit capability refusals, not range errors or incidental mentions.
// The caller must independently require an upstream invalid_request_error / 400.
var samplingRejectionPatterns = []*regexp.Regexp{
	regexp.MustCompile("(?i)^[`\"']?(temperature|top_p|top_k)[`\"']?(?: parameter)? is (?:deprecated|not supported|unsupported)\\b"),
	regexp.MustCompile("(?i)^unsupported parameter[: ]+[`\"']?(temperature|top_p|top_k)\\b"),
	regexp.MustCompile("(?i)^(?:[a-z0-9][a-z0-9._:/-]* )?does not support non-default [`\"']?(temperature|top_p|top_k)\\b"),
}
var samplingParameterMention = regexp.MustCompile(`(?i)\b(temperature|top_p|top_k)\b`)

func RejectedSamplingParameter(message string) string {
	message = strings.TrimSpace(message)
	for _, pattern := range samplingRejectionPatterns {
		match := pattern.FindStringSubmatch(message)
		if len(match) == 0 {
			continue
		}
		parameter := strings.ToLower(match[1])
		// Mixed-parameter constraints need an explicit client/operator decision.
		for _, mention := range samplingParameterMention.FindAllString(message, -1) {
			if !strings.EqualFold(mention, parameter) {
				return ""
			}
		}
		return parameter
	}
	return ""
}
