package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestAliTTSModelIsRequiredBeforeUpgrade(t *testing.T) {
	for _, query := range []string{"", "?model=voice-alias"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/v1/audio/speech/stream"+query, nil)
		request, selectChannel, err := getModelRequest(c)
		if query == "" {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.True(t, selectChannel)
			require.Equal(t, "voice-alias", request.Model)
		}
	}
}
