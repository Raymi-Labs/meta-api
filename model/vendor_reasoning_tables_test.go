package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOfficialReasoningSourceParsersRequireExactEvidence(t *testing.T) {
	tail := "\n\n### Explanation\n"
	header := "| Model | Default Thinking | Levels Supported |\n| --- | --- | --- |\n"
	table := header + "| gemini-new | On | low, high |\n| gemini-fast | On (low) | low, medium, high |" + tail
	g, e := ProjectOfficialReasoningSource(GeminiCatalog, []byte(table))
	require.NoError(t, e)
	require.Equal(t, "", g["gemini-new"].DefaultLevel)
	require.Equal(t, []string{"low", "high"}, g["gemini-new"].SupportedLevels)
	require.Equal(t, "low", g["gemini-fast"].DefaultLevel)
	for _, bad := range []string{strings.TrimSuffix(table, tail), strings.Replace(table, "gemini-fast", "gemini-new", 1), strings.Replace(table, "On (low)", "On (max)", 1), strings.Replace(table, "low, high", "low, low", 1)} {
		_, e := ProjectOfficialReasoningSource(GeminiCatalog, []byte(bad))
		require.Error(t, e)
	}
	doc := "### Effort levels\n| `low` | L | Test |\n| `medium` | M | Test |\n| `high` | H | Test |\n| `max` | Max. Available on Claude Test 5 and Claude Test 5.1. | Test |\n| `xhigh` | Extra. Available on Claude Test 5.1. | Test |\n\n## Availability\n| Available | Claude API, other | [Claude Test 5](https://platform.claude.com/docs/en/models/test-5/overview), [Claude Test 5.1](https://platform.claude.com/docs/en/models/test-5-1/overview) |"
	c, e := ProjectOfficialReasoningSource(ClaudeCatalog, []byte(doc))
	require.NoError(t, e)
	require.Len(t, c, 2)
	require.NotContains(t, c["claude-test-5"].SupportedLevels, "xhigh")
	_, badLink := ProjectOfficialReasoningSource(ClaudeCatalog, []byte(strings.Replace(doc, "https://platform.claude.com/docs/en/models/test-5/overview", "/en/models/test-5/overview", 1)))
	require.Error(t, badLink)
	require.Contains(t, c["claude-test-5-1"].SupportedLevels, "xhigh")
	require.Empty(t, c["claude-test-5"].DefaultLevel)
	_, e = ProjectOfficialReasoningSource(ClaudeCatalog, []byte(strings.Replace(doc, "Available on Claude Test 5.1.", "unsupported schema", 1)))
	require.Error(t, e)
	codex, e := ProjectOfficialReasoningSource(CodexCatalog, []byte(`{"models":[{"slug":"test","default_reasoning_level":"future","supported_reasoning_levels":[{"effort":"high"},{"effort":"future"}]}]}`))
	require.NoError(t, e)
	require.Equal(t, []string{"high"}, codex["test"].SupportedLevels)
	require.Empty(t, codex["test"].DefaultLevel)
}
