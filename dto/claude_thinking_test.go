package dto

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestThinkingRoundTripAndRetryIsolation(t *testing.T) {
	for _, binding := range []string{`{"prefix_mismatch_behavior":"drop_block"}`, `null`} {
		body := `{"type":"adaptive","display":"summarized","block_binding":` + binding + `,"future":{"nested":[1,false]},"future_null":null}`
		var original Thinking
		require.NoError(t, common.UnmarshalJsonStr(body, &original))
		cloned, err := common.DeepCopy(&original)
		require.NoError(t, err)
		encoded, err := common.Marshal(cloned)
		require.NoError(t, err)
		require.JSONEq(t, body, string(encoded))
		cloned.BlockBinding[0] = ' '
		cloned.Extra["future"][0] = ' '
		cloned.Extra["new"] = json.RawMessage(`true`)
		require.Equal(t, binding, string(original.BlockBinding))
		require.JSONEq(t, `{"nested":[1,false]}`, string(original.Extra["future"]))
		require.NotContains(t, original.Extra, "new")
		require.NoError(t, common.UnmarshalJsonStr(`{"type":"disabled"}`, &original))
		require.Empty(t, original.Extra)
		require.Empty(t, original.BlockBinding)
	}
}

func TestThinkingPreserveBlockBinding(t *testing.T) {
	original := &Thinking{Type: "adaptive", BlockBinding: json.RawMessage(`{"keep":true}`)}
	for _, mode := range []string{"adaptive", "enabled", "disabled"} {
		current := &Thinking{Type: mode}
		current.PreserveBlockBinding(original)
		if mode == "adaptive" {
			require.JSONEq(t, string(original.BlockBinding), string(current.BlockBinding))
			current.BlockBinding[0] = ' '
			require.Equal(t, byte('{'), original.BlockBinding[0])
		} else {
			require.Empty(t, current.BlockBinding)
		}
	}
	current := &Thinking{Type: "adaptive", BlockBinding: json.RawMessage(`null`)}
	current.PreserveBlockBinding(original)
	require.Equal(t, "null", string(current.BlockBinding))
	(*Thinking)(nil).PreserveBlockBinding(original)
}
