package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReasoningProfileAdminAndVisibleCatalog(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperData(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Model{}))
	decode := func(body string) modelMetaRequest {
		var r modelMetaRequest
		require.NoError(t, common.UnmarshalJsonStr(body, &r))
		return r
	}
	req := decode(`{"reasoning_profile":{"supportedLevels":["low","ultra"],"defaultLevel":"low"}}`)
	item, err := req.withDisplayMetadata(nil)
	require.NoError(t, err)
	item.ModelName, item.Status = "gpt-5.2", 1
	require.NoError(t, model.DB.Create(&item).Error)
	private := item
	private.Id = 0
	private.ModelName = "unlisted"
	require.NoError(t, model.DB.Create(&private).Error)
	readonly := decode(`{}`)
	require.Contains(t, readonly.omittedMetadataFields(), "reasoning_profile")
	preserved, err := readonly.withDisplayMetadata(&item)
	require.NoError(t, err)
	require.Equal(t, item.ReasoningProfile, preserved.ReasoningProfile)
	for _, body := range []string{`{"reasoning_profile":{"supportedLevels":["low"],"defaultLevel":"ultra"}}`, `{"name_rule":1,"reasoning_profile":{"supportedLevels":["high"],"defaultLevel":"high"}}`, `{"reasoning_profile":{"supportedLevels":["auto"],"defaultLevel":"auto"}}`, `{"reasoning_profile":{"supportedLevels":["low","low"],"defaultLevel":"low"}}`, `{"reasoning_profile":{}}`} {
		_, err = decode(body).withDisplayMetadata(&item)
		require.Error(t, err, body)
	}
	catalog := func(query string) map[string]any {
		res := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(res)
		ctx.Request = httptest.NewRequest("GET", "/api/user/models?"+query, nil)
		ctx.Set("id", 1)
		GetUserModels(ctx)
		var envelope map[string]any
		require.NoError(t, common.Unmarshal(res.Body.Bytes(), &envelope))
		require.Equal(t, true, envelope["success"])
		return envelope
	}
	legacy := catalog("group=default")
	require.NotContains(t, legacy, "reasoningProfiles")
	modern := catalog("group=default&include=reasoning_profiles")
	require.Equal(t, legacy["data"], modern["data"])
	require.Equal(t, map[string]any{"gpt-5.2": map[string]any{"defaultLevel": "low", "supportedLevels": []any{"low", "ultra"}}}, modern["reasoningProfiles"])
	require.Equal(t, map[string]any{}, catalog("group=forbidden&include=reasoning_profiles")["reasoningProfiles"])
	// Legacy editor: no reasoning field. A rule change must drop the exact profile,
	// even when returning to exact afterwards, without requiring a new admin UI.
	update := func(body string) {
		res := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(res)
		ctx.Request = httptest.NewRequest("PUT", "/api/models/", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		UpdateModelMeta(ctx)
		var v map[string]any
		require.NoError(t, common.Unmarshal(res.Body.Bytes(), &v))
		require.Equal(t, true, v["success"], v["message"])
	}
	wire, err := common.Marshal(map[string]any{"id": private.Id, "model_name": private.ModelName, "name_rule": 1, "status": 1})
	require.NoError(t, err)
	update(string(wire))
	var read model.Model
	require.NoError(t, model.DB.First(&read, private.Id).Error)
	require.Equal(t, 1, read.NameRule)
	require.Nil(t, read.ReasoningProfile)
	clear := decode(`{"reasoning_profile":null,"model_name":"gpt-5.2","status":1}`)
	cleared, err := clear.withDisplayMetadata(&item)
	require.NoError(t, err)
	require.Nil(t, cleared.ReasoningProfile)
	cleared.Id = item.Id
	require.NoError(t, cleared.UpdateOmittingMetadata(clear.omittedMetadataFields()))
	require.Equal(t, map[string]any{}, catalog("group=default&include=reasoning_profiles")["reasoningProfiles"])
}

func TestOfficialReasoningVisibleCatalogWithoutManualSetup(t *testing.T) {
	setupTokenModelHelperDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Model{}))
	got, err := model.VisibleReasoningProfiles([]string{"gpt-6-astra", "gpt-6-luna", "gpt-5.5", "gpt-unknown", "GPT-6-ASTRA"})
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max", "ultra"}, got["gpt-6-astra"].SupportedLevels)
	require.Len(t, got["gpt-6-luna"].SupportedLevels, 5)
	require.Len(t, got["gpt-5.5"].SupportedLevels, 4)
	limited := model.Model{ModelName: "gpt-6-astra", Status: 1, ReasoningProfile: &model.ReasoningProfile{SupportedLevels: []string{"medium"}, DefaultLevel: "medium"}}
	require.NoError(t, model.DB.Create(&limited).Error)
	got, err = model.VisibleReasoningProfiles([]string{"gpt-6-astra"})
	require.NoError(t, err)
	require.Equal(t, limited.ReasoningProfile, got["gpt-6-astra"])
	require.NoError(t, model.DB.Model(&limited).Update("reasoning_profile", nil).Error)
	got, err = model.VisibleReasoningProfiles([]string{"gpt-6-astra"})
	require.NoError(t, err)
	require.Len(t, got["gpt-6-astra"].SupportedLevels, 6)
	require.NoError(t, model.DB.Model(&limited).Update("capabilities", model.ModelCapabilities{"reasoning": false}).Error)
	got, err = model.VisibleReasoningProfiles([]string{"gpt-6-astra"})
	require.NoError(t, err)
	require.Equal(t, []string{}, got["gpt-6-astra"].SupportedLevels)
	// A community-owned bool must not override a manual discrete declaration.
	require.NoError(t, model.DB.Model(&limited).Updates(map[string]any{"model_capability_sync_state": `{"version":1,"source":"https://example.invalid","capabilities":{"reasoning":false}}`, "reasoning_profile": `{"supportedLevels":["medium"],"defaultLevel":"medium"}`}).Error)
	got, err = model.VisibleReasoningProfiles([]string{"gpt-6-astra"})
	require.NoError(t, err)
	require.Equal(t, []string{"medium"}, got["gpt-6-astra"].SupportedLevels)
}

func TestOfficialReasoningLastGoodSurvivesSourceOutageAndReplicaRead(t *testing.T) {
	setupTokenModelHelperDB(t)
	raw := []byte(`{"models":[{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"low"}]}]}`)
	require.NoError(t, model.ReplaceOfficialReasoningCatalog(model.CodexCatalog, raw))
	require.Error(t, model.ReplaceOfficialReasoningCatalog(model.CodexCatalog, []byte(`{}`)))
	got, err := model.VisibleReasoningProfiles([]string{"gpt-6-astra", "gpt-6-sol", "gemini-4-hydrogen"})
	require.NoError(t, err)
	require.Equal(t, []string{"low"}, got["gpt-6-astra"].SupportedLevels)
	require.Empty(t, got["gpt-6-astra"].DefaultLevel)
	require.Empty(t, got["gpt-6-sol"].SupportedLevels)
	require.NotEmpty(t, got["gemini-4-hydrogen"].SupportedLevels)
}

func TestOfficialReasoningClaudeLinkLossIsNotAWithdrawal(t *testing.T) {
	setupTokenModelHelperDB(t)
	const label = "Claude Fixture Four"
	const first = "[Claude Fixture Four](https://platform.claude.com/docs/en/models/non-derivable-id/overview)"
	const second = "[Claude Fixture Five](https://platform.claude.com/docs/en/models/other-id/overview)"
	const table = "### Effort levels\n| `low` | L | T |\n| `medium` | M | T |\n| `high` | H | T |\n| `max` | Available on Claude Fixture Five. | T |\n| `xhigh` | Available on Claude Fixture Five. | T |\n\n## Availability\n| Available | Claude API, other | "
	apply := func(names string) error {
		return model.ReplaceOfficialReasoningCatalog(model.ClaudeCatalog, []byte(table+names+" |"))
	}
	require.NoError(t, apply(first+", "+second))
	require.NoError(t, apply(label+", "+second)) // ID is retained using previously verified evidence, not name inference.
	require.Error(t, apply("Claude An Unlinked Rename, "+second))
	read := func() []string {
		p, err := model.VisibleReasoningProfiles([]string{"claude-non-derivable-id"})
		require.NoError(t, err)
		return p["claude-non-derivable-id"].SupportedLevels
	}
	require.Equal(t, []string{"low", "medium", "high"}, read())
	require.NoError(t, apply(second))
	require.Empty(t, read())
}
