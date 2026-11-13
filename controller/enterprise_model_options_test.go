package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseModelOptionsUsesActorAuthorization(t *testing.T) {
	ent, member := setupEnterprisePayerContract(t)
	require.NoError(t, model.DB.Model(member).Update("role", model.EnterpriseRoleOwner).Error)
	// Enterprise ownership must not expose platform-admin-only models.
	require.NoError(t, model.DB.Create(&model.ModelPermission{ModelName: imagePlaygroundAgentModel,
		NameRule: model.NameRuleExact, VisibilityScope: model.ModelPermissionScopeAdminOnly,
		CallScope: model.ModelPermissionScopeAdminOnly}).Error)
	model.InvalidateModelPermissionCache()
	query := func(userID int, path string, success bool) []string {
		t.Helper()
		router := gin.New()
		router.GET("/api/enterprises/:id/models", func(c *gin.Context) { c.Set("id", userID) },
			middleware.EnterpriseMemberAuth(model.EnterpriseRoleAdmin), GetEnterpriseModelOptions)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		var response struct {
			Success bool     `json:"success"`
			Data    []string `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		require.Equal(t, success, response.Success, w.Body.String())
		return response.Data
	}
	path := "/api/enterprises/11/models"
	require.Equal(t, []string{imagePlaygroundDefaultModel}, query(1, path, true))

	// Changing price identity and its special usable groups cannot change candidates.
	require.NoError(t, model.DB.Model(ent).Update("billing_group", "other-price").Error)
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Set("other-price", map[string]string{"-:default": "", "+:private": ""})
	require.Equal(t, []string{imagePlaygroundDefaultModel}, query(1, path, true))

	// Other members' additional access is intentionally not unioned into actor options.
	userID := 2
	require.NoError(t, model.DB.Create(&model.EnterpriseMember{EnterpriseId: ent.Id, UserId: 2, ActiveUserId: &userID,
		Role: model.EnterpriseRoleMember, Status: model.EnterpriseMemberStatusActive, ModelMode: model.EnterpriseModelModeInherit}).Error)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 2).Update("group", "private").Error)
	require.Equal(t, []string{imagePlaygroundDefaultModel}, query(1, path, true))

	// Candidates do change when the actor's actual authorization or platform role changes.
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1).Update("group", "private").Error)
	require.Equal(t, []string{imagePlaygroundDefaultModel, "private-model"}, query(1, path, true))
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1).Update("role", common.RoleAdminUser).Error)
	require.Equal(t, []string{imagePlaygroundAgentModel, imagePlaygroundDefaultModel, "private-model"}, query(1, path, true))

	// Configuration candidates must not disappear when the currently selected policy is empty.
	require.NoError(t, model.DB.Model(ent).Updates(map[string]any{"model_mode": model.EnterpriseModelModeCustom, "model_list": "[]"}).Error)
	require.Len(t, query(1, path, true), 3)
	query(2, path, false)                         // ordinary enterprise member
	query(1, "/api/enterprises/12/models", false) // another enterprise
}
