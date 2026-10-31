package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTokenQuotaInvocationRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{"POST", "/v1/messages", true}, {"POST", "/v1/responses", true}, {"POST", "/v1/images/generations", true}, {"POST", "/v1/audio/speech", true},
		{"POST", "/v1beta/models/gemini:generateContent", true}, {"POST", "/v1beta/models/gemini:countTokens", false}, {"POST", "/v1/messages/count_tokens", false},
		{"GET", "/v1/models", false}, {"GET", "/v1/videos/task", false}, {"POST", "/suno/fetch", false}, {"POST", "/mj/task/list-by-condition", false}, {"POST", "/mj/notify", false},
		{"POST", "/mj/submit/imagine", true}, {"POST", "/suno/submit/music", true}, {"POST", "/mj/submit/upload-discord-images", false},
		{"POST", "/v1/videos", true}, {"POST", "/v1/videos/id/remix", true}, {"POST", "/kling/v1/videos/text2video", true},
		{"POST", "/jimeng/?Action=CVSync2AsyncGetResult", false}, {"POST", "/jimeng/?Action=CVSync2AsyncSubmitTask", true},
		{"GET", "/v1/realtime", true}, {"GET", "/v1/audio/speech/stream", true}, {"POST", "/v1/audio/voices", false}, {"POST", "/v1/files", false},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tc.method, tc.path, nil)
			require.Equal(t, tc.want, IsTokenQuotaInvocation(c))
		})
	}
}
