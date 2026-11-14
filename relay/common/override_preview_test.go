package common

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestPreviewParamOverrideMatchesRuntime(t *testing.T) {
	var override map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(`{"operations":[
		{"mode":"set","path":"temperature","value":0.5,"conditions":[{"path":"upstream_model","mode":"full","value":"mapped"}]},
		{"mode":"set","path":"skip","value":true,"conditions":[{"path":"model","mode":"full","value":"other"}]},
		{"mode":"copy_header","from":"x-example","to":"x-preview"}
	]}`), &override))
	body := `{"model":"mapped","opaque":9007199254740993}`
	sample := ParamOverridePreviewContext{OriginalModel: "original", UpstreamModel: "mapped", RequestHeaders: map[string]string{"X-Example": "sample"}}
	preview, err := PreviewParamOverride(body, override, sample)
	require.NoError(t, err)
	require.Empty(t, preview.Error)
	require.Contains(t, preview.Body, `9007199254740993`)
	require.Contains(t, preview.Body, `"temperature":0.5`)
	require.NotContains(t, preview.Body, `"skip"`)
	require.Len(t, preview.Steps, 3)
	require.True(t, preview.Steps[0].ConditionsMatched)
	require.False(t, preview.Steps[1].ConditionsMatched)
	require.Equal(t, "sample", preview.HeaderOverride["x-preview"])
	info := &RelayInfo{ChannelMeta: &ChannelMeta{UpstreamModelName: "mapped", ParamOverride: override}, OriginModelName: "original", RequestHeaders: sample.RequestHeaders}
	actual, err := ApplyParamOverrideWithRelayInfo([]byte(body), info)
	require.NoError(t, err)
	require.Equal(t, string(actual), preview.Body)
	require.Equal(t, info.RuntimeHeadersOverride, preview.HeaderOverride)
	require.Equal(t, map[string]string{"X-Example": "sample"}, sample.RequestHeaders)
}

func TestPreviewParamOverrideFailuresAndLegacy(t *testing.T) {
	preview, err := PreviewParamOverride(`{"model":"old"}`, map[string]interface{}{"model": "new"}, ParamOverridePreviewContext{})
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"new"}`, preview.Body)
	var override map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(`{"operations":[{"mode":"return_error","value":{"message":"blocked","status_code":422,"skip_retry":true}},{"mode":"set","path":"should_not_run","value":true}]}`), &override))
	preview, err = PreviewParamOverride(`{}`, override, ParamOverridePreviewContext{})
	require.NoError(t, err)
	require.NotNil(t, preview.Rejection)
	require.Equal(t, 422, preview.Rejection.StatusCode)
	require.Len(t, preview.Steps, 1)
	require.Empty(t, preview.Body)
	_, err = PreviewParamOverride(`[]`, nil, ParamOverridePreviewContext{})
	require.Error(t, err)
	_, err = PreviewParamOverride(`{}`, map[string]interface{}{"operations": "invalid"}, ParamOverridePreviewContext{})
	require.Error(t, err)
	override = map[string]interface{}{"operations": []map[string]interface{}{{"mode": "regex_replace", "path": "model", "from": "[", "to": "x"}}}
	preview, err = PreviewParamOverride(`{"model":"x"}`, override, ParamOverridePreviewContext{})
	require.NoError(t, err)
	require.NotEmpty(t, preview.Error)
	require.Nil(t, preview.Rejection)
}

func TestPreviewParamOverrideLimits(t *testing.T) {
	operations := make([]map[string]interface{}, 101)
	for i := range operations {
		operations[i] = map[string]interface{}{"mode": "copy", "from": "source", "to": fmt.Sprintf("copy_%d", i)}
	}
	_, err := PreviewParamOverride(`{}`, map[string]interface{}{"operations": operations}, ParamOverridePreviewContext{})
	require.ErrorContains(t, err, "100 operations")
	body := `{"source":"` + strings.Repeat("a", 64*1024) + `"}`
	preview, err := PreviewParamOverride(body, map[string]interface{}{"operations": operations[:20]}, ParamOverridePreviewContext{})
	require.NoError(t, err)
	require.Contains(t, preview.Error, "1 MiB")
	require.Less(t, len(preview.Steps), 20)
}
