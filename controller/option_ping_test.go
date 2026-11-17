package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateOptionRejectsInvalidPingInterval(t *testing.T) {
	for _, value := range []string{`"1200"`, `0`, `-1`, `1.5`, `"not-a-number"`, `true`} {
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest("PUT", "/api/option/", strings.NewReader(`{"key":"general_setting.ping_interval_seconds","value":`+value+`}`))
		UpdateOption(c)
		require.Contains(t, r.Body.String(), `"success":false`)
		require.Contains(t, r.Body.String(), "between 1 and 5")
	}
}
