package service

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPublicVideoUsesConfiguredGatewayOnly(t *testing.T) {
	old := system_setting.ServerAddress
	t.Cleanup(func() { system_setting.ServerAddress = old })
	system_setting.ServerAddress = "https://gateway.example/"
	source := &dto.OpenAIVideo{ID: "provider-task", Model: "private-provider.example", Status: dto.VideoStatusCompleted, Seconds: "4", Size: "1280x720", Metadata: map[string]any{"url": "https://private-provider.example/secret", "nested": map[string]any{"key": "secret"}}}
	out := PublicVideo(source, "task_public", "public-model")
	require.Equal(t, "https://gateway.example/v1/videos/task_public/content", out.Metadata["url"])
	require.Len(t, out.Metadata, 1)
	require.Equal(t, "4", out.Seconds)
	require.Equal(t, "1280x720", out.Size)
	require.Contains(t, source.Metadata["url"], "private-provider")
	source.Seconds = "https://private-provider.example"
	source.Size = "secret"
	source.Status = dto.VideoStatusFailed
	out = PublicVideo(source, "task_public", "public-model")
	require.Empty(t, out.Metadata)
	require.Empty(t, out.Seconds)
	require.Empty(t, out.Size)
	require.Equal(t, "video_generation_failed", out.Error.Code)
	for _, alias := range []string{"https://meta-api.vip", "https://cn.meta-api.vip/", "https://nan.meta-api.vip", "https://meta-api.com"} {
		system_setting.ServerAddress = alias
		require.Equal(t, "https://meta-api.com/v1/videos/task_public/content", VideoContentURL("task_public"))
		require.Equal(t, alias, system_setting.ServerAddress, "do not rewrite global callbacks")
	}
	for _, other := range []string{"https://meta-api.vip.evil.test", "https://meta-api.com:444", "https://tenant.example"} {
		system_setting.ServerAddress = other
		require.Equal(t, other+"/v1/videos/task_public/content", VideoContentURL("task_public"))
	}
	for _, bad := range []string{"https://key@gateway.example", "https://gateway.example?key=secret", "https://gateway.example#secret", "not-a-url"} {
		system_setting.ServerAddress = bad
		require.Equal(t, "/v1/videos/task_public/content", VideoContentURL("task_public"))
	}
}
