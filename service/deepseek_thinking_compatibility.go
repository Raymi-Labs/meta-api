package service

import (
	"regexp"
	"strings"
)

// Match the specific provider refusal, optionally followed by its request ID.
// Callers must separately check the upstream status, structured error type and
// provider boundary before changing a request's thinking mode.
var deepSeekMissingThinkingHistoryPattern = regexp.MustCompile("^The `content\\[\\]\\.thinking` in the thinking mode must be passed back to the API\\.(?: \\(request_id: [A-Za-z0-9_-]+\\))?$")

func IsDeepSeekMissingThinkingHistory(message string) bool {
	return deepSeekMissingThinkingHistoryPattern.MatchString(strings.TrimSpace(message))
}
