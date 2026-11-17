package ratio_setting

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNoDiscountModelsValidationAndExactMatch(t *testing.T) {
	old := NoDiscountModelsJSON()
	t.Cleanup(func() { require.NoError(t, UpdateNoDiscountModels(old)) })
	require.NoError(t, UpdateNoDiscountModels(`["gpt-luna"]`))
	require.True(t, IsNoDiscountModel("gpt-luna"))
	require.False(t, IsNoDiscountModel("gpt-luna-alias"))
	for _, raw := range []string{`null`, `{}`, `[null]`, `[1]`, `[""]`, `[" x"]`, `["x*"]`, `["x","x"]`} {
		require.Error(t, UpdateNoDiscountModels(raw), raw)
		require.True(t, IsNoDiscountModel("gpt-luna"))
	}
	require.NoError(t, UpdateNoDiscountModels(`[]`))
	require.False(t, IsNoDiscountModel("gpt-luna"))
}
