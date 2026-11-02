package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateOptionRejectsInvalidSpecialGroupRatios(t *testing.T) {
	for _, key := range []string{"GroupGroupRatio", "group_ratio_setting.group_group_ratio"} {
		for _, bad := range []string{`null`, `[]`, `{"vip":{"paid":null}}`, `{"vip":{"paid":-0.1}}`, `{"vip":`} {
			t.Run(key+bad, func(t *testing.T) {
				body, err := common.Marshal(map[string]string{"key": key, "value": bad})
				require.NoError(t, err)
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(string(body)))
				UpdateOption(c)
				require.Equal(t, http.StatusOK, w.Code)
				var response struct {
					Success bool   `json:"success"`
					Message string `json:"message"`
				}
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
				require.False(t, response.Success)
				require.NotEmpty(t, response.Message)
			})
		}
	}
}
