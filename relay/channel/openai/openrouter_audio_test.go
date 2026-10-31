package openai

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const audioPolicy = `{"provider":{"only":["google-ai-studio","google-vertex/global"],"max_price":{"prompt":0.5,"completion":6}}}`
const audioEndpoints = `{"data":{"endpoints":[{"tag":"google-ai-studio","service_tier":null,"pricing":{"prompt":"0.0000005","completion":"0.000006","request":"0"}},{"tag":"google-vertex/global","pricing":{"prompt":"0.0000005","completion":"0.000006"}}]}}`

func audioInfo(base string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter, ChannelBaseUrl: base, ApiKey: "test-only", UpstreamModelName: "google/gemini-3.8-flash-lite-tts"}}
}

func TestOpenRouterAudioPriceGate(t *testing.T) {
	for _, tc := range []struct {
		name, response, policy string
		status                 int
		fail                   bool
	}{
		{"all providers at cap", audioEndpoints, audioPolicy, 200, false},
		{"one expensive fallback", strings.Replace(audioEndpoints, `0.000006`, `0.000006001`, 1), audioPolicy, 200, true},
		{"unknown fallback", strings.Replace(audioEndpoints, `google-vertex/global`, `unreviewed`, 1), audioPolicy, 200, true},
		{"missing price", strings.Replace(audioEndpoints, `"prompt":"0.0000005",`, ``, 1), audioPolicy, 200, true},
		{"negative price", strings.Replace(audioEndpoints, `0.000006`, `-0.000006`, 1), audioPolicy, 200, true},
		{"additional charge", strings.Replace(audioEndpoints, `"request":"0"`, `"request":"0.01"`, 1), audioPolicy, 200, true},
		{"unknown extra charge", strings.Replace(audioEndpoints, `"request":"0"`, `"new_fee":"0.01"`, 1), audioPolicy, 200, true},
		{"unknown tier", strings.Replace(audioEndpoints, `"service_tier":null`, `"service_tier":"priority"`, 1), audioPolicy, 200, true},
		{"no endpoints", `{"data":{"endpoints":[]}}`, audioPolicy, 200, true},
		{"bad JSON", `<html>`, audioPolicy, 200, true},
		{"catalog unavailable", audioEndpoints, audioPolicy, 503, true},
		{"no policy", audioEndpoints, `{}`, 200, true},
		{"stricter caller zero", audioEndpoints, strings.Replace(audioPolicy, `"completion":6`, `"completion":0`, 1), 200, true},
		{"missing cap", audioEndpoints, strings.Replace(audioPolicy, `,"completion":6`, ``, 1), 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v1/models/google/gemini-3.8-flash-lite-tts/endpoints", r.URL.Path)
				require.Equal(t, "Bearer test-only", r.Header.Get("Authorization"))
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.response)
			}))
			defer server.Close()
			err := checkOpenRouterAudioPrices(context.Background(), audioInfo(server.URL), []byte(tc.policy))
			require.Equal(t, tc.fail, err != nil, "%v", err)
		})
	}
}

func TestOpenRouterAudioMetadataRejectsRedirectAndRechecks(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = io.WriteString(w, audioEndpoints)
		} else {
			w.Header().Set("Location", "https://example.invalid/credentials")
			w.WriteHeader(302)
		}
	}))
	defer server.Close()
	info := audioInfo(server.URL)
	require.NoError(t, checkOpenRouterAudioPrices(context.Background(), info, []byte(audioPolicy)))
	require.Error(t, checkOpenRouterAudioPrices(context.Background(), info, []byte(audioPolicy)))
	require.Equal(t, 2, calls)
}

func TestOpenRouterAudioConversionAndMappedMultipart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, audioEndpoints) }))
	defer server.Close()
	info := audioInfo(server.URL)
	require.NoError(t, common.UnmarshalJsonStr(`{"operations":[{"mode":"set","path":"provider.only","value":["google-ai-studio","google-vertex/global"]},{"mode":"set","path":"provider.max_price","value":{"prompt":0.5,"completion":6}}]}`, &info.ParamOverride))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/audio/speech", strings.NewReader(`{"model":"gemini-3.8-flash-lite-tts","input":"Hello","provider":{"zdr":true,"data_collection":"deny"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	out, err := (&Adaptor{}).ConvertAudioRequest(c, info, dto.AudioRequest{Model: info.UpstreamModelName, Input: "Hello", Voice: "Kore"})
	require.NoError(t, err)
	b, err := io.ReadAll(out)
	require.NoError(t, err)
	require.Equal(t, info.UpstreamModelName, gjson.GetBytes(b, "model").String())
	require.Equal(t, "pcm", gjson.GetBytes(b, "response_format").String())
	require.True(t, gjson.GetBytes(b, "provider.zdr").Bool())
	require.Equal(t, "deny", gjson.GetBytes(b, "provider.data_collection").String())
	require.False(t, gjson.GetBytes(b, "provider.max_price").Exists())
	require.False(t, gjson.GetBytes(b, "provider.only").Exists())
	for _, req := range []dto.AudioRequest{{Model: info.UpstreamModelName, ResponseFormat: "mp3"}, {Model: info.UpstreamModelName, StreamFormat: "sse"}, {Model: info.UpstreamModelName, Speed: 2}, {Model: "google/different"}} {
		_, err = convertOpenRouterGeminiAudio(c, info, req)
		require.Error(t, err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gemini-3.5-transcribe"))
	f, err := writer.CreateFormFile("file", "test.wav")
	require.NoError(t, err)
	_, _ = f.Write([]byte("RIFF-audio"))
	require.NoError(t, writer.Close())
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/audio/transcriptions", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	info.RelayMode = relayconstant.RelayModeAudioTranscription
	info.UpstreamModelName = "google/gemini-3.5-transcribe"
	out, err = (&Adaptor{}).ConvertAudioRequest(c, info, dto.AudioRequest{Model: info.UpstreamModelName})
	require.NoError(t, err)
	b, err = io.ReadAll(out)
	require.NoError(t, err)
	require.Equal(t, info.UpstreamModelName, gjson.GetBytes(b, "model").String())
	require.Equal(t, "wav", gjson.GetBytes(b, "input_audio.format").String())
	require.Equal(t, "UklGRi1hdWRpbw==", gjson.GetBytes(b, "input_audio.data").String())
	require.Equal(t, "application/json", c.Request.Header.Get("Content-Type"))
}

func TestOpenRouterSpeechActualUsage(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "gen-test", r.URL.Query().Get("id"))
		calls++
		if calls == 1 {
			w.WriteHeader(404)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":"gen-test","native_tokens_prompt":7,"native_tokens_completion":79}}`)
	}))
	defer server.Close()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/audio/speech", nil)
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"audio/pcm"}, "X-Generation-Id": []string{"gen-test"}}, Body: io.NopCloser(strings.NewReader("pcm bytes"))}
	u, e := openRouterGeminiTTSResponse(c, response, audioInfo(server.URL))
	require.Nil(t, e)
	require.Equal(t, 7, u.PromptTokensDetails.TextTokens)
	require.Equal(t, 79, u.CompletionTokenDetails.AudioTokens)
	require.Equal(t, 86, u.TotalTokens)
	require.Equal(t, "pcm bytes", recorder.Body.String())
	require.Equal(t, 2, calls)
	response = &http.Response{Header: http.Header{"Content-Type": []string{"audio/pcm"}}, Body: io.NopCloser(strings.NewReader("pcm"))}
	u, e = openRouterGeminiTTSResponse(c, response, audioInfo(server.URL))
	require.Nil(t, u)
	require.NotNil(t, e)
	require.True(t, types.IsSkipRetryError(e))
	require.Equal(t, 2, calls)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := openRouterSpeechUsage(ctx, audioInfo(server.URL), "gen-test")
	require.Error(t, err)
}

func TestOpenRouterTranscriptionActualUsageRequired(t *testing.T) {
	for _, tc := range []struct {
		body string
		fail bool
	}{
		{`{"text":"Hello","usage":{"input_tokens":83,"output_tokens":30,"total_tokens":113}}`, false},
		{`{"text":"","usage":{"input_tokens":83,"output_tokens":0}}`, false},
		{`{"text":"Hello","usage":{"seconds":1}}`, true},
		{`{"text":"Hello","usage":{"input_tokens":83}}`, true},
		{`{"text":"Hello","usage":{"input_tokens":-1,"output_tokens":3}}`, true},
		{`{"text":"Hello","usage":{"input_tokens":83,"output_tokens":-1}}`, true},
		{`{"text":"Hello"}`, true},
		{`{"usage":{"input_tokens":83,"output_tokens":3}}`, true},
	} {
		t.Run(tc.body, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/audio/transcriptions", nil)
			resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}}
			u, e := openRouterGeminiSTTResponse(c, resp, audioInfo(""))
			require.Equal(t, tc.fail, e != nil)
			if tc.fail {
				require.True(t, types.IsSkipRetryError(e))
				require.Empty(t, rec.Body.String())
			} else {
				require.Equal(t, 83, u.PromptTokensDetails.AudioTokens)
				require.Equal(t, u.CompletionTokens, u.CompletionTokenDetails.TextTokens)
				require.Equal(t, tc.body, rec.Body.String())
			}
		})
	}
}

func TestOpenRouterSpeechUsageDelayedAndCancelled(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 5 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":"gen-delayed","native_tokens_prompt":3,"native_tokens_completion":26}}`)
	}))
	defer server.Close()
	u, err := openRouterSpeechUsage(context.Background(), audioInfo(server.URL), "gen-delayed")
	require.NoError(t, err)
	require.Equal(t, 3, u.PromptTokensDetails.TextTokens)
	require.Equal(t, 26, u.CompletionTokenDetails.AudioTokens)
	require.Equal(t, 6, calls, "usage must remain queryable beyond the old four attempts")

	calls = 0
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	u, err = openRouterSpeechUsage(ctx, audioInfo(server.URL), "gen-delayed")
	require.Nil(t, u)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, calls, "cancel while waiting must stop further metadata requests")
}

func TestOpenRouterAudioScope(t *testing.T) {
	info := audioInfo("")
	require.True(t, isOpenRouterGeminiAudio(info))
	info.ChannelType = constant.ChannelTypeOpenAI
	require.False(t, isOpenRouterGeminiAudio(info))
	info.ChannelType = constant.ChannelTypeOpenRouter
	info.UpstreamModelName = "openai/tts-1"
	require.False(t, isOpenRouterGeminiAudio(info))
	info.UpstreamModelName = "google/gemini-3.8-flash"
	info.RelayMode = relayconstant.RelayModeChatCompletions
	require.False(t, isOpenRouterGeminiAudio(info))
}
