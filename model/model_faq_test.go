package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestModelFAQValidationAndStorage(t *testing.T) {
	f := ModelFAQ{{Question: " 第二个？ ", Answer: " 第一行\n第二行 "}, {Question: "第一个？", Answer: "<script>alert(1)</script>"}}
	require.NoError(t, f.Normalize())
	require.Equal(t, "第二个？", f[0].Question)
	require.Equal(t, "第一行\n第二行", f[0].Answer)
	value, err := f.Value()
	require.NoError(t, err)
	for _, value := range []interface{}{value, []byte(value.(string))} {
		var saved ModelFAQ
		require.NoError(t, saved.Scan(value))
		require.Equal(t, f, saved)
	}
	clone := f.Clone()
	clone[0].Question = "changed"
	require.Equal(t, "第二个？", f[0].Question)
	for _, value := range []interface{}{nil, "", []byte{}, "null", "[]"} {
		var empty ModelFAQ
		require.NoError(t, empty.Scan(value))
		encoded, err := common.Marshal(empty)
		require.NoError(t, err)
		require.Equal(t, "[]", string(encoded))
	}
	for _, value := range []interface{}{1, "{}", `[1]`, `[{"question":1}]`, `broken`} {
		var invalid ModelFAQ
		require.Error(t, invalid.Scan(value))
	}
	for _, f := range []ModelFAQ{
		{{Question: " ", Answer: "answer"}}, {{Question: "question", Answer: "\n"}},
		{{Question: strings.Repeat("问", 201), Answer: "answer"}},
		{{Question: "question", Answer: strings.Repeat("答", 2001)}},
		make(ModelFAQ, 21),
	} {
		require.Error(t, f.Normalize())
	}
	boundary := ModelFAQ{{Question: strings.Repeat("问", 200), Answer: strings.Repeat("答", 2000)}}
	require.NoError(t, boundary.Normalize())
	tooLarge := ModelFAQ{}
	for range 20 {
		tooLarge = append(tooLarge, boundary[0])
	}
	require.ErrorContains(t, tooLarge.Normalize(), "内容过长")
}
