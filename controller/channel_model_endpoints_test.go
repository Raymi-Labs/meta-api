package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func mockOfficialDiscovery(t *testing.T, response string, status int) *int {
	t.Helper()
	service.InitHttpClient()
	client := service.GetHttpClient()
	original := client.Transport
	calls := 0
	client.Transport = discoveryTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme == "https" {
			calls++
			require.Empty(t, r.Header.Get("Authorization"))
			require.Empty(t, r.Header.Get("X-Api-Key"))
			require.Empty(t, r.Header.Get("X-Goog-Api-Key"))
			require.Empty(t, r.Header.Get("X-Private-Tenant"))
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
		}
		return original.RoundTrip(r)
	})
	t.Cleanup(func() { client.Transport = original })
	return &calls
}

func TestDiscoveryEveryTypeUsesDraftAndSharedFallback(t *testing.T) {
	for channelType := range constant.ChannelTypeNames {
		if channelType == 0 || channelType == constant.ChannelTypeGemini {
			continue
		}
		t.Run(constant.GetChannelTypeName(channelType), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.True(t, strings.HasPrefix(r.URL.Path, "/draft/"))
				require.Equal(t, "tenant", r.Header.Get("X-Private-Tenant"))
				if channelType == constant.ChannelTypeAnthropic {
					require.Equal(t, "draft-key", r.Header.Get("X-Api-Key"))
				} else {
					require.Equal(t, "Bearer draft-key", r.Header.Get("Authorization"))
				}
				fmt.Fprint(w, `{"data":[{"id":"new-upstream-model"}]}`)
			}))
			defer server.Close()
			officialCalls := mockOfficialDiscovery(t, `{"data":[{"id":"official-model"}]}`, 200)
			_, result := previewModels(t, map[string]any{"type": channelType, "base_url": server.URL + "/draft", "key": "draft-key", "header_override": `{"X-Private-Tenant":"tenant"}`, "allow_reference_fallback": true})
			require.Equal(t, true, result["success"])
			require.Equal(t, "upstream", result["source"])
			require.Equal(t, []any{"new-upstream-model"}, result["data"])
			require.Equal(t, 1, calls)
			require.Zero(t, *officialCalls)
		})
	}
}

func TestDiscoveryAllOfficialFallbacksAreAnonymous(t *testing.T) {
	mockOfficialDiscovery(t, `{"data":[{"id":"latest-official"}]}`, 200)
	for channelType, base := range constant.ChannelBaseURLs {
		if !strings.HasPrefix(base, "https://") || channelType == constant.ChannelTypeGemini || channelType == constant.ChannelTypeCodex {
			continue
		}
		t.Run(constant.GetChannelTypeName(channelType), func(t *testing.T) {
			require.Equal(t, []string{"latest-official"}, fetchOfficialModelReference(context.Background(), channelType))
		})
	}
	// Account-specific/locally installed catalogs have no anonymous fixed official URL.
	for _, channelType := range []int{constant.ChannelTypeOllama, constant.ChannelTypeAzure, constant.ChannelTypeAws, constant.ChannelTypeVertexAi, constant.ChannelTypeCodex} {
		require.Empty(t, fetchOfficialModelReference(context.Background(), channelType))
	}
}

func TestDiscoveryNonGeminiReferenceAndStrictSync(t *testing.T) {
	calls := mockOfficialDiscovery(t, `{"data":[{"id":"official-latest"}]}`, 200)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "private-key", 401) }))
	defer server.Close()
	args := map[string]any{"type": constant.ChannelTypeOpenRouter, "base_url": server.URL, "key": "private-key", "header_override": `{"X-Private-Tenant":"secret"}`, "allow_reference_fallback": true}
	_, result := previewModels(t, args)
	require.Equal(t, "official_reference", result["source"])
	require.Equal(t, []any{"official-latest"}, result["data"])
	require.Equal(t, 1, *calls)
	args["allow_reference_fallback"] = false
	_, result = previewModels(t, args)
	require.Equal(t, false, result["success"])
	_, err := fetchChannelUpstreamModelIDs(&model.Channel{Type: 20, Key: "private-key", BaseURL: common.GetPointer(server.URL)})
	require.Error(t, err)
	require.Equal(t, 1, *calls)
}

func TestDiscoveryProviderPathsAndPagination(t *testing.T) {
	for _, tc := range []struct {
		kind       int
		base, path string
	}{
		{17, "https://proxy.example/v1/", "https://proxy.example/v1/models"},
		{17, "https://dashscope.aliyuncs.com", "https://dashscope.aliyuncs.com/compatible-mode/v1/models"},
		{26, "https://proxy.example/api/paas/v4", "https://proxy.example/api/paas/v4/models"},
		{45, "doubao-coding-plan", "https://ark.cn-beijing.volces.com/api/coding/v3/models"},
		{25, "kimi-coding-plan", "https://api.kimi.com/coding/v1/models"},
		{4, "https://proxy.example/v1", "https://proxy.example/api/tags"},
	} {
		endpoints, err := channelModelEndpoints(tc.kind, strings.TrimRight(tc.base, "/"))
		require.NoError(t, err)
		require.Equal(t, tc.path, endpoints[0])
	}
	for _, kind := range []int{constant.ChannelTypeAnthropic, constant.ChannelTypeCohere} {
		t.Run(constant.GetChannelTypeName(kind), func(t *testing.T) {
			count := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count++
				if kind == constant.ChannelTypeAnthropic {
					if count == 1 {
						fmt.Fprint(w, `{"data":[{"id":"first"}],"has_more":true,"last_id":"a+/&"}`)
					} else {
						require.Equal(t, "a+/&", r.URL.Query().Get("after_id"))
						fmt.Fprint(w, `{"data":[{"id":"second"}],"has_more":false}`)
					}
				} else {
					if count == 1 {
						fmt.Fprint(w, `{"models":[{"name":"first"}],"next_page_token":"a+/&"}`)
					} else {
						require.Equal(t, "a+/&", r.URL.Query().Get("page_token"))
						fmt.Fprint(w, `{"models":[{"name":"second"}]}`)
					}
				}
			}))
			defer server.Close()
			names, err := fetchChannelUpstreamModelIDs(&model.Channel{Type: kind, BaseURL: common.GetPointer(server.URL)})
			require.NoError(t, err)
			require.Equal(t, []string{"first", "second"}, names)
			require.Equal(t, 2, count)
		})
	}
}

func TestDiscoveryRejectsIncompleteCatalogsAndRedirects(t *testing.T) {
	for _, body := range []string{`{}`, `{"error":"denied"}`, `{"data":null}`, `{"data":[{}]}`, `{"data":[{"id":"one"}],"has_more":true}`, `{"data":[{"id":"one"}],"has_more":true,"last_id":"repeat"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		names, err := fetchChannelUpstreamModelIDs(&model.Channel{Type: 14, BaseURL: common.GetPointer(server.URL)})
		require.Error(t, err, body)
		require.Empty(t, names)
		server.Close()
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect must not receive credentials") }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer server.Close()
	_, err := fetchChannelUpstreamModelIDs(&model.Channel{Type: 1, Key: "secret", BaseURL: common.GetPointer(server.URL)})
	require.Error(t, err)
}

func TestDiscoveryBuiltinIncludesCloudAndTaskReferences(t *testing.T) {
	for _, kind := range []int{constant.ChannelTypeAzure, constant.ChannelTypeAws, constant.ChannelTypeVertexAi, constant.ChannelTypeMidjourney, constant.ChannelTypeSunoAPI, constant.ChannelTypeKling, constant.ChannelTypeVidu, constant.ChannelTypeDoubaoVideo, constant.ChannelTypeSora, constant.ChannelTypeCodex} {
		require.NotEmpty(t, builtinModelReference(kind), constant.GetChannelTypeName(kind))
	}
	require.Contains(t, builtinModelReference(constant.ChannelTypeSunoAPI), "suno_music")
	require.Contains(t, builtinModelReference(constant.ChannelTypeMidjourney), "mj_imagine")
	require.Empty(t, builtinModelReference(constant.ChannelTypeDify))
}

func TestDiscoveryOllamaUsesDraftProxyAndHeaders(t *testing.T) {
	calls := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "http://unreachable.invalid/api/tags", r.URL.String())
		require.Equal(t, "Bearer draft-key", r.Header.Get("Authorization"))
		require.Equal(t, "tenant", r.Header.Get("X-Tenant"))
		fmt.Fprint(w, `{"models":[{"name":"installed-model"}]}`)
	}))
	defer proxy.Close()
	_, result := previewModels(t, map[string]any{"type": constant.ChannelTypeOllama, "base_url": "http://unreachable.invalid/v1/", "key": "draft-key", "proxy": proxy.URL, "header_override": `{"X-Tenant":"tenant"}`})
	require.Equal(t, "upstream", result["source"])
	require.Equal(t, []any{"installed-model"}, result["data"])
	require.Equal(t, 1, calls)
}
