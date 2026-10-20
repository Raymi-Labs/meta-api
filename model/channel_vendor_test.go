package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestChannelMatchesVendor_MiMoBoundaryRules(t *testing.T) {
	tests := []struct {
		name         string
		models       string
		modelMapping string
		setting      string
		match        bool
	}{
		{name: "canonical prefix", models: "mimo-v2.5", match: true},
		{name: "xiaomi namespace", models: "xiaomi/mimo-v2.5-pro", match: true},
		{name: "mapped alias", models: "tts-alias", modelMapping: `{"tts-alias":"mimo-v2.5-tts"}`, match: true},
		{name: "mixed model set remains protocol", models: "gpt-4o,vendor.xiaomi-mimo,claude-3", match: false},
		{name: "explicit MiMo overrides model inference", models: "gpt-4o", setting: ChannelVendorMiMo, match: true},
		{name: "protocol override disables inference", models: "mimo-v2.5", setting: ChannelVendorProtocol, match: false},
		{name: "mimosa is unrelated", models: "mimosa", match: false},
		{name: "embedded mimo is unrelated", models: "notmimo-model", match: false},
		{name: "xiaomi namespace without mimo is unrelated", models: "xiaomi/other-model", match: false},
		{name: "empty model set", models: "", match: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &Channel{Models: test.models, Type: 1}
			if test.modelMapping != "" {
				channel.ModelMapping = &test.modelMapping
			}
			if test.setting != "" {
				channel.ChannelVendor = &test.setting
			}
			require.Equal(t, test.match, ChannelMatchesVendor(channel, ChannelVendorMiMo))
		})
	}
}

func TestCountAndFilterChannelsByVendor_DeduplicatesMultiModelChannels(t *testing.T) {
	channels := []*Channel{
		{Id: 1, Type: 1, Models: "mimo-v2.5,mimo-v2.5-pro"},
		{Id: 2, Type: 1, Models: "mimosa,gpt-4o"},
		{Id: 3, Type: 2, Models: "xiaomi/mimo-v2.5"},
		nil,
	}

	counts := CountChannelVendors(channels)
	require.EqualValues(t, 3, counts[ChannelVendorAll])
	require.EqualValues(t, 2, counts[ChannelVendorMiMo])

	filtered := FilterChannelsByVendor(channels, "小米 MiMo")
	require.Len(t, filtered, 2)
	require.Equal(t, []int{1, 3}, []int{filtered[0].Id, filtered[1].Id})

	typeCounts := CountChannelTypes(filtered)
	require.EqualValues(t, 1, typeCounts[1])
	require.EqualValues(t, 1, typeCounts[2])
}

func TestChannelCategoriesSeparateVendorFromProtocol(t *testing.T) {
	protocolSetting := ChannelVendorProtocol
	explicitMiMo := ChannelVendorMiMo
	channels := []*Channel{
		{Id: 1, Type: constant.ChannelTypeOpenAI, Models: "mimo-v2.5"},
		{Id: 2, Type: constant.ChannelTypeOpenAI, Models: "gpt-4o"},
		{Id: 3, Type: constant.ChannelTypeOpenAI, Models: "mimo-v2.5", ChannelVendor: &protocolSetting},
		{Id: 4, Type: constant.ChannelTypeAnthropic, Models: "claude-opus-4-6", ChannelVendor: &explicitMiMo},
	}

	counts := CountChannelCategories(channels)
	require.EqualValues(t, 4, counts[ChannelCategoryAll])
	require.EqualValues(t, 1, counts[ChannelCategoryTypePrefix+"1"])
	require.EqualValues(t, 1, counts["vendor:openai"])
	require.EqualValues(t, 2, counts[ChannelCategoryVendorPrefix+ChannelVendorMiMo])

	openAICategory, err := ParseChannelCategory("type:1")
	require.NoError(t, err)
	filtered := FilterChannelsByCategory(channels, openAICategory)
	require.Len(t, filtered, 1)
	require.Equal(t, 3, filtered[0].Id)

	mimoCategory, err := ParseChannelCategory("vendor:mimo")
	require.NoError(t, err)
	filtered = FilterChannelsByCategory(channels, mimoCategory)
	require.Equal(t, []int{1, 4}, []int{filtered[0].Id, filtered[1].Id})

	_, err = ParseChannelCategory("type:invalid")
	require.Error(t, err)
	_, err = ParseChannelCategory("vendor:unknown")
	require.Error(t, err)
}

func TestDisplayVendorInference(t *testing.T) {
	tests := []struct{ models, mapping, want string }{
		{"deepseek-ai/DeepSeek-V4.1-Flash", "", "deepseek"},
		{"gpt-4o,o3-mini", "", "openai"},
		{"anthropic/claude-sonnet-4", "", "anthropic"},
		{"gemini-2.5-pro", "", "google"},
		{"Qwen/Qwen3-32B", "", "qwen"},
		{"moonshotai/Kimi-K2", "", "moonshot"},
		{"z-ai/GLM-4.5", "", "zhipu"},
		{"grok-4", "", "xai"},
		{"MiniMaxAI/MiniMax-M2", "", "minimax"},
		{"mistralai/Mistral-Small", "", "mistral"},
		{"mercury-3,mercury-2.5,mercury-edit-2", "", "inception"},
		{"inception/mercury-3", "", "inception"},
		{"vendor:Mercury-Voice", "", "inception"},
		{"alias", `{"alias":"inception/mercury-3"}`, "inception"},
		{"mercury-3,gpt-4o", "", ""},
		{"mercury-3,unknown", "", ""},
		{"mercury-claude-distill", "", ""},
		{"notmercury-3", "", ""},
		{"mercurial", "", ""},
		{"inception/unknown", "", ""},
		{"alias", `{"alias":"deepseek-ai/DeepSeek-V4.1-Flash"}`, "deepseek"},
		{"deepseek-chat,claude-sonnet-4", "", ""},
		{"deepseek-chat,unknown", "", ""},
		{"notdeepseek-model", "", ""},
		{"deepseek-ai/DeepSeek-R1-Distill-Qwen-32B", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.models, func(t *testing.T) {
			c := &Channel{Type: constant.ChannelTypeCustom, Models: tt.models, ModelMapping: &tt.mapping}
			require.Equal(t, tt.want, InferChannelVendor(c))
			require.Equal(t, constant.ChannelTypeCustom, c.Type)
		})
	}
	for vendor := range channelVendorSegments {
		t.Run("explicit/"+vendor, func(t *testing.T) {
			c := &Channel{Type: constant.ChannelTypeCustom, Models: "unknown,mixed", ChannelVendor: &vendor}
			category, err := ParseChannelCategory("vendor:" + vendor)
			require.NoError(t, err)
			require.Equal(t, vendor, ResolveChannelVendor(c))
			require.Len(t, FilterChannelsByCategory([]*Channel{c}, category), 1)
			require.EqualValues(t, 1, CountChannelCategories([]*Channel{c})[category.Key])
			require.EqualValues(t, 1, CountChannelVendors([]*Channel{c})[vendor])
		})
	}
}
