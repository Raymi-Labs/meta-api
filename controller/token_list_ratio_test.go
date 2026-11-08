package controller

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupTokenListRatios(t *testing.T) {
	t.Helper()
	groups := ratio_setting.GroupRatio2JSONString()
	specialRatios := ratio_setting.GroupGroupRatio2JSONString()
	usable := setting.UserUsableGroups2JSONString()
	specialUsable := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(specialRatios))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(usable))
		ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReplaceAll(specialUsable)
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"standard":0.45,"special":0.9,"free":0.5,"zero":0,"private":2,"negative":-1,"auto":7}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"special":0.18,"free":0}}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"","standard":"","special":"","free":"","zero":"","negative":"","auto":""}`))
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReplaceAll(nil)
}

func TestTokenListGroupRatioResolution(t *testing.T) {
	setupTokenControllerTestDB(t)
	setupTokenListRatios(t)
	for _, tc := range []struct {
		name, group, status string
		ratio               float64
	}{
		{"ordinary", "standard", "fixed", 0.45},
		{"special override", "special", "fixed", 0.18},
		{"zero override is valid", "free", "fixed", 0},
		{"zero base is valid", "zero", "fixed", 0},
		{"inherit user group", "", "fixed", 1},
		{"auto never uses its configured number", "auto", "auto", 0},
		{"removed group", "removed", "unavailable", 0},
		{"unauthorized group", "private", "unavailable", 0},
		{"invalid negative ratio", "negative", "unavailable", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := &model.Token{UserId: 1, Group: tc.group, Key: "synthetic-token-secret"}
			item := buildTokenListItems(1, []*model.Token{token})[0]
			require.Equal(t, tc.status, item.GroupRatioStatus)
			if tc.status == "fixed" {
				require.NotNil(t, item.GroupRatio)
				require.Equal(t, tc.ratio, *item.GroupRatio)
				effectiveGroup := tc.group
				if effectiveGroup == "" {
					effectiveGroup = "default"
				}
				require.Equal(t, service.GetUserGroupRatio("default", effectiveGroup), *item.GroupRatio)
			} else {
				require.Nil(t, item.GroupRatio)
			}
			require.Equal(t, tc.group, item.Group, "display must not rewrite token group")
			require.Equal(t, token.GetMaskedKey(), item.Key)
			require.Equal(t, "synthetic-token-secret", token.Key, "source token must not be mutated")
			raw, err := common.Marshal(item)
			require.NoError(t, err)
			require.NotContains(t, string(raw), token.Key)
		})
	}
}

func TestTokenListGroupRatioFailureAndRefresh(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	setupTokenListRatios(t)
	token := &model.Token{UserId: 1, Group: "", Key: "synthetic-refresh-secret"}

	// A current user-group change is reflected without persisting a token field.
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Update("group", "special").Error)
	require.Equal(t, 0.9, *buildTokenListItems(1, []*model.Token{token})[0].GroupRatio)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Update("group", "auto").Error)
	require.Equal(t, "auto", buildTokenListItems(1, []*model.Token{token})[0].GroupRatioStatus)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Update("group", "default").Error)
	token.Group = "standard"
	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -0.1} {
		ratio_setting.GetGroupRatioSetting().GroupRatio.Set("standard", invalid)
		item := buildTokenListItems(1, []*model.Token{token})[0]
		require.Equal(t, "unavailable", item.GroupRatioStatus)
		require.Nil(t, item.GroupRatio)
		_, err := common.Marshal(item)
		require.NoError(t, err, "invalid settings must not break list JSON")
	}
	ratio_setting.GetGroupRatioSetting().GroupRatio.Set("standard", 0.25)
	require.Equal(t, 0.25, *buildTokenListItems(1, []*model.Token{token})[0].GroupRatio)

	// Revoking access must not disclose the group's numeric ratio.
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":""}`))
	require.Equal(t, "unavailable", buildTokenListItems(1, []*model.Token{token})[0].GroupRatioStatus)
	require.Equal(t, "unavailable", buildTokenListItems(9999, []*model.Token{token})[0].GroupRatioStatus)
	require.Equal(t, "unavailable", buildTokenListItems(1, []*model.Token{nil})[0].GroupRatioStatus)
	require.Empty(t, buildTokenListItems(1, nil))

	// Group lookup failure degrades only the display, not the token list.
	require.NoError(t, db.Migrator().DropTable(&model.User{}))
	item := buildTokenListItems(1, []*model.Token{token})[0]
	require.Equal(t, "unavailable", item.GroupRatioStatus)
	require.Nil(t, item.GroupRatio)
	require.Equal(t, token.GetMaskedKey(), item.Key)
}

func TestTokenListGroupRatioListAndSearch(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	setupTokenListRatios(t)
	for i, group := range []string{"standard", "special", "free", "", "auto", "private"} {
		token := seedToken(t, db, 1, fmt.Sprintf("ratio-token-%d", i), fmt.Sprintf("synthetic-key-%d-abcdef", i))
		require.NoError(t, db.Model(token).Update("group", group).Error)
	}
	seedToken(t, db, 2, "ratio-token-other-user", "synthetic-other-user-key")

	for _, endpoint := range []struct {
		name, path string
		handler    gin.HandlerFunc
	}{
		{"list", "/api/token/?", GetAllTokens},
		{"search", "/api/token/search?keyword=ratio-token%25&", SearchTokens},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			seen := make(map[int]bool)
			for page := 1; page <= 3; page++ {
				ctx, recorder := newAuthenticatedContext(t, http.MethodGet, fmt.Sprintf("%sp=%d&size=2", endpoint.path, page), nil, 1)
				endpoint.handler(ctx)
				res := decodeAPIResponse(t, recorder)
				require.True(t, res.Success, res.Message)
				var result struct {
					Items []tokenListItem `json:"items"`
					Total int             `json:"total"`
				}
				require.NoError(t, common.Unmarshal(res.Data, &result))
				require.Equal(t, 6, result.Total)
				require.Len(t, result.Items, 2)
				for _, item := range result.Items {
					require.Equal(t, 1, item.UserId)
					require.False(t, seen[item.Id], "pagination must not duplicate a token")
					seen[item.Id] = true
					require.False(t, strings.Contains(item.Key, "synthetic-key"))
					expected := buildTokenListItems(1, []*model.Token{item.Token})[0]
					require.Equal(t, expected.GroupRatioStatus, item.GroupRatioStatus)
					require.Equal(t, expected.GroupRatio, item.GroupRatio)
				}
			}
			ctx, recorder := newAuthenticatedContext(t, http.MethodGet, endpoint.path+"group=special&p=1&size=10", nil, 1)
			endpoint.handler(ctx)
			res := decodeAPIResponse(t, recorder)
			var result struct {
				Items []tokenListItem `json:"items"`
			}
			require.True(t, res.Success)
			require.NoError(t, common.Unmarshal(res.Data, &result))
			require.Len(t, result.Items, 1)
			require.Equal(t, 0.18, *result.Items[0].GroupRatio)
			require.Equal(t, "special", result.Items[0].Group)
		})
	}
}

func TestTokenListIdentityGroupUsesAuthorizedDefaultRatio(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	setupTokenListRatios(t)
	old := setting.UserGroups2JSONString()
	t.Cleanup(func() { require.NoError(t, setting.UpdateUserGroupsByJSONString(old)) })
	require.NoError(t, setting.UpdateUserGroupsByJSONString(`{"default":"","identity":""}`))
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Update("group", "identity").Error)
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"identity":{"default":0.25}}`))
	token := &model.Token{UserId: 1, Group: "", Key: "fixture"}
	item := buildTokenListItems(1, []*model.Token{token})[0]
	require.Equal(t, "fixed", item.GroupRatioStatus)
	require.NotNil(t, item.GroupRatio)
	require.Equal(t, 0.25, *item.GroupRatio)
	require.Empty(t, item.Group)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{}`))
	item = buildTokenListItems(1, []*model.Token{token})[0]
	require.Equal(t, "unavailable", item.GroupRatioStatus)
	require.Nil(t, item.GroupRatio)
}
