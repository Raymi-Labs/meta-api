package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSystemTranslationBindingAndSanitizedStatus(t *testing.T) {
	descriptionTestDB(t)
	invoke := func(id int, value string) string {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("id", id)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(`{"key":"ModelDescriptionTranslationTokenId","value":"`+value+`"}`))
		UpdateOption(c)
		return w.Body.String()
	}
	require.Contains(t, invoke(101, "100"), `"success":false`)
	require.Contains(t, invoke(100, "-1"), `"success":false`)
	require.Contains(t, invoke(100, "100"), `"success":true`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", 100)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/option/model-description-translation", nil)
	GetSystemTranslationSettings(c)
	require.Contains(t, w.Body.String(), `"state":"ready"`)
	require.Contains(t, w.Body.String(), `"name":"System translation"`)
	require.NotContains(t, w.Body.String(), "testsystemtranslation")
	require.NotContains(t, w.Body.String(), `"key"`)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 100).Update("model_limits_enabled", false).Error)
	require.Contains(t, invoke(100, "100"), `"success":false`)
	require.Contains(t, invoke(100, "0"), `"success":true`)
	require.Zero(t, model.SystemTranslationTokenID())
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 100).Update("role", common.RoleCommonUser).Error)
	require.Contains(t, invoke(100, "100"), `"success":false`)
}

func TestSystemTranslationRejectsRedirectAndIncompleteOutput(t *testing.T) {
	descriptionTestDB(t)
	redirected := 0
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected++ }))
	t.Cleanup(other.Close)
	cases := []string{"redirect", "truncated", "malformed", "oversize"}
	for _, scenario := range cases {
		t.Run(scenario, func(t *testing.T) {
			descriptionMockRelay(t, func(w http.ResponseWriter, r *http.Request) {
				switch scenario {
				case "redirect":
					http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
				case "truncated":
					_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`))
				case "malformed":
					_, _ = w.Write([]byte(`{"choices":[]}`))
				case "oversize":
					_, _ = w.Write([]byte(strings.Repeat("x", (1<<20)+1)))
				}
			})
			_, err := translateDescriptionPart(context.Background(), descriptionPart{Text: "Model description"}, "zh-CN")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "testsystemtranslation")
		})
	}
	require.Zero(t, redirected)
}
