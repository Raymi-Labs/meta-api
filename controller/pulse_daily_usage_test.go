package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPulseYesterdayRejectsOverridesAndDisabledLogs(t *testing.T) {
	old := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = old })
	for _, tc := range []struct {
		url    string
		id     uint64
		status int
	}{{"/", 0, 401}, {"/?user_id=2", 1, 400}, {"/", 1, 503}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, tc.url, nil)
		c.Set("pulse_service_user_id", tc.id)
		PulseYesterdayUsage(c)
		require.Equal(t, tc.status, w.Code)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
}
