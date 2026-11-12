package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAliyunModelAttributes(t *testing.T) {
	b, err := os.ReadFile("testdata/aliyun-model-attributes.html")
	require.NoError(t, err)
	a, err := parseAliyunModelAttributes(b, "wan3.0-video-prime", "https://help.aliyun.com/zh/model-studio/wan3-0-video-prime")
	require.NoError(t, err)
	require.Equal(t, model.ModelModalities{"text", "image", "video", "audio", "file"}, a.InputModalities)
	require.Equal(t, model.ModelModalities{"video"}, a.OutputModalities)
	require.Contains(t, a.Capabilities, "function_calling")
	require.False(t, a.Capabilities["function_calling"])
	require.Zero(t, a.ContextLength)
	_, err = parseAliyunModelAttributes(b, "wan3.0-video", "url")
	require.Error(t, err)
	a, err = parseAliyunModelAttributes([]byte(strings.ReplaceAll(string(b), "不支持", "待确认")), "wan3.0-video-prime", "url")
	require.NoError(t, err)
	require.NotContains(t, a.Capabilities, "function_calling")
}
func TestAliyunAttributeLiveCoverage(t *testing.T) {
	dir := os.Getenv("TEST_ALIYUN_ATTRIBUTES")
	if dir == "" {
		t.Skip("opt-in local downloaded documents")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.html"))
	require.NoError(t, err)
	results := map[string]model.ModelCapabilityMetadata{}
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".html")
		b, e := os.ReadFile(f)
		require.NoError(t, e)
		a, e := parseAliyunModelAttributes(b, id, "https://help.aliyun.com/zh/model-studio/"+strings.ReplaceAll(id, ".", "-"))
		if e == nil {
			results[id] = a
		} else {
			t.Log(id, e)
		}
	}
	b, err := common.Marshal(results)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report.json"), b, 0600))
	t.Logf("fetched=%d parsed=%d", len(files), len(results))
}

func TestAliyunChineseTwoColumnAttributes(t *testing.T) {
	b := []byte(`<h1>qwen-audio-3.1-realtime-flash</h1><table><tr><th>能力项</th><th>说明</th></tr><tr><td>输入模态</td><td>文本、音频</td></tr><tr><td>输出模态</td><td>文本、音频</td></tr><tr><td>Function Calling</td><td>支持</td></tr></table><table><tr><th>参数</th><th>上限（Token）</th></tr><tr><td>上下文长度</td><td>262,144</td></tr><tr><td>最大输出长度</td><td>16,384</td></tr></table>`)
	a, err := parseAliyunModelAttributes(b, "qwen-audio-3.1-realtime-flash", "url")
	require.NoError(t, err)
	require.Equal(t, model.ModelModalities{"text", "audio"}, a.InputModalities)
	require.True(t, a.Capabilities["function_calling"])
	require.EqualValues(t, 262144, a.ContextLength)
	require.EqualValues(t, 16384, a.MaxOutputTokens)
}
