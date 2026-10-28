package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetDefaultVendorNameRecognizesMiMoBoundaries(t *testing.T) {
	for _, modelName := range []string{
		"mimo-v2.5",
		"MiMo-VL-7B",
		"xiaomi/mimo-v2.5-pro",
		"vendor.xiaomi-mimo",
	} {
		t.Run(modelName, func(t *testing.T) {
			require.Equal(t, defaultMiMoVendorName, getDefaultVendorName(modelName))
		})
	}
}

func TestGetDefaultVendorNameDoesNotMisclassifyMiMoSubstrings(t *testing.T) {
	for _, modelName := range []string{
		"mimosa",
		"notmimo-model",
		"xiaomish-model",
		"xiaomi/other-model",
		"qwen-tts",
		"",
	} {
		t.Run(modelName, func(t *testing.T) {
			require.NotEqual(t, defaultMiMoVendorName, getDefaultVendorName(modelName))
		})
	}
}

func TestInitDefaultVendorMappingFillsUnassignedMiMoMetadata(t *testing.T) {
	originalMeta := &Model{
		ModelName: "mimo-v2.5",
		VendorID:  0,
		Status:    1,
		NameRule:  NameRuleExact,
	}
	metaMap := map[string]*Model{"mimo-v2.5": originalMeta}
	vendorMap := map[int]*Vendor{
		7: {Id: 7, Name: defaultMiMoVendorName},
	}

	initDefaultVendorMapping(metaMap, vendorMap, []AbilityWithChannel{{
		Ability: Ability{Model: "mimo-v2.5"},
	}})

	require.Equal(t, 7, metaMap["mimo-v2.5"].VendorID)
	require.Zero(t, originalMeta.VendorID, "fallback must not mutate shared rule metadata")
}

func TestInitDefaultVendorMappingPreservesExplicitVendor(t *testing.T) {
	metaMap := map[string]*Model{
		"mimo-v2.5": {
			ModelName: "mimo-v2.5",
			VendorID:  99,
			Status:    1,
			NameRule:  NameRuleExact,
		},
	}
	vendorMap := map[int]*Vendor{
		7:  {Id: 7, Name: defaultMiMoVendorName},
		99: {Id: 99, Name: "Custom Vendor"},
	}

	initDefaultVendorMapping(metaMap, vendorMap, []AbilityWithChannel{{
		Ability: Ability{Model: "mimo-v2.5"},
	}})

	require.Equal(t, 99, metaMap["mimo-v2.5"].VendorID)
}

func TestInitDefaultVendorMappingLeavesUnknownMetadataUntouched(t *testing.T) {
	originalMeta := &Model{
		ModelName: "custom-unclassified",
		VendorID:  0,
		Status:    1,
		NameRule:  NameRuleExact,
	}
	metaMap := map[string]*Model{"custom-unclassified": originalMeta}
	vendorMap := map[int]*Vendor{
		7: {Id: 7, Name: "阿里巴巴"},
	}

	initDefaultVendorMapping(metaMap, vendorMap, []AbilityWithChannel{{
		Ability: Ability{Model: "custom-unclassified"},
	}})

	require.Same(t, originalMeta, metaMap["custom-unclassified"])
	require.Zero(t, metaMap["custom-unclassified"].VendorID)
}

func TestDefaultVendorNameRecognizesReviewAndMiniMax(t *testing.T) {
	for name, want := range map[string]string{
		"codex-auto-review": "OpenAI", " CODEX-AUTO-REVIEW ": "OpenAI",
		"minimax-m3.1-flash": "MiniMax", "MiniMaxAI/MiniMax-M3": "MiniMax",
		"notminimax-model": "", "minimaximum": "",
		"claude-minimax": "", "minimax/gpt-4o": "",
		"codex-other": "", "codex-auto-review-extra": "", "notcodex-auto-review": "",
	} {
		require.Equal(t, want, getDefaultVendorName(name), name)
	}
}

func TestDefaultVendorFallbackPreservesSharedMetadata(t *testing.T) {
	for _, rule := range []int{NameRuleExact, NameRulePrefix, NameRuleSuffix, NameRuleContains} {
		original := &Model{ModelName: "shared-rule", NameRule: rule, Status: 0,
			SyncOfficial: 0, ContextLength: 128000, Description: "curated", Tags: "retained"}
		names := []string{"gpt-6.1-sol", "claude-fable-5", "gemini-4-argon", "kimi-k3.1-pro", "qwen3.8-max", "deepseek-flash", "grok-4.7", "glm-5.3", "minimax-m3.1-flash", "codex-auto-review"}
		vendors := []string{"OpenAI", "Anthropic", "Google", "Moonshot", "阿里巴巴", "DeepSeek", "xAI", "智谱", "MiniMax", "OpenAI"}
		vendorMap := make(map[int]*Vendor)
		metaMap := make(map[string]*Model)
		var abilities []AbilityWithChannel
		for i, name := range names {
			vendorMap[i+1] = &Vendor{Id: i + 1, Name: vendors[i]}
			metaMap[name] = original
			abilities = append(abilities, AbilityWithChannel{Ability: Ability{Model: name}})
		}
		// Keep a single canonical OpenAI vendor; both names must resolve to it.
		delete(vendorMap, len(names))
		for pass := 0; pass < 2; pass++ {
			initDefaultVendorMapping(metaMap, vendorMap, abilities)
			for i, name := range names {
				resolved := metaMap[name]
				require.NotSame(t, original, resolved)
				require.Equal(t, vendors[i], vendorMap[resolved.VendorID].Name)
				copy := *resolved
				copy.VendorID = 0
				require.Equal(t, *original, copy, "only the display vendor may change")
			}
		}
		require.Zero(t, original.VendorID)
	}
}

func TestDefaultVendorFallbackPreservesUnresolvedExplicitVendor(t *testing.T) {
	original := &Model{ModelName: "gpt-6.1-sol", VendorID: 99}
	metaMap := map[string]*Model{original.ModelName: original}
	initDefaultVendorMapping(metaMap, map[int]*Vendor{1: {Id: 1, Name: "OpenAI"}},
		[]AbilityWithChannel{{Ability: Ability{Model: original.ModelName}}})
	require.Same(t, original, metaMap[original.ModelName])
	require.Equal(t, 99, original.VendorID, "do not replace an administrator's nonzero choice")
}

func TestGetOrCreateVendorBackfillsEmptyMiMoIconInMemory(t *testing.T) {
	vendor := &Vendor{Id: 7, Name: defaultMiMoVendorName}
	vendorMap := map[int]*Vendor{7: vendor}

	vendorID := getOrCreateVendor(defaultMiMoVendorName, vendorMap)

	require.Equal(t, 7, vendorID)
	require.Equal(t, "Xiaomi.color='#FF6900'", vendor.Icon)
}

func TestGetOrCreateVendorPreservesExplicitMiMoIcon(t *testing.T) {
	vendor := &Vendor{Id: 7, Name: defaultMiMoVendorName, Icon: "CustomIcon"}
	vendorMap := map[int]*Vendor{7: vendor}

	vendorID := getOrCreateVendor(defaultMiMoVendorName, vendorMap)

	require.Equal(t, 7, vendorID)
	require.Equal(t, "CustomIcon", vendor.Icon)
}

func TestGetOrCreateVendorReusesLegacyMiMoAliasInMemory(t *testing.T) {
	vendor := &Vendor{Id: 7, Name: legacyMiMoVendorName}
	vendorMap := map[int]*Vendor{7: vendor}

	vendorID := getOrCreateVendor(defaultMiMoVendorName, vendorMap)

	require.Equal(t, 7, vendorID)
	require.Equal(t, defaultMiMoVendorName, vendor.Name)
	require.Equal(t, "Xiaomi.color='#FF6900'", vendor.Icon)
}
