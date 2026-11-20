package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOfficialCatalogPublicationIsAtomicAndCanNarrowOrRemoveExactModels(t *testing.T) {
	var cache officialProfiles
	apply := func(rows string) error {
		m, err := ProjectOfficialReasoningSource(CodexCatalog, []byte(`{"models":`+rows+`}`))
		if err != nil {
			return err
		}
		if err := retainWithdrawals(cache, m); err != nil {
			return err
		}
		cache = m
		return nil
	}
	first := `{"slug":"fixture","default_reasoning_level":"low","supported_reasoning_levels":[{"effort":"low"},{"effort":"ultra"}]}`
	other := `{"slug":"another","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"medium"}]}`
	require.NoError(t, apply(`[`+first+`,`+other+`]`))
	before := cache
	require.Len(t, before, 2)
	for _, rows := range []string{`[]`, `null`, `[{}]`, `[` + first + `,{"slug":"future","default_reasoning_level":"high","supported_reasoning_levels":[{"effort":"low"}]}]`, `[` + first + `,` + first + `]`} {
		require.Error(t, apply(rows))
		require.Equal(t, before, cache)
	}
	require.NoError(t, apply(`[{"slug":"fixture","default_reasoning_level":"low","supported_reasoning_levels":[{"effort":"low"},{"effort":"future"}]}]`))
	after := cache
	require.Len(t, after, 2)
	require.Equal(t, []string{"low"}, after["fixture"].SupportedLevels)
	require.NotContains(t, after, "Fixture")
	require.Empty(t, after["another"].SupportedLevels)
	require.Len(t, before["fixture"].SupportedLevels, 2)
}
