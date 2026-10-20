package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setVideoTestPrice(t *testing.T, name, expr string) {
	t.Helper()
	saved := billing_setting.TieredBundle{BillingMode: billing_setting.GetBillingModeCopy(), BillingExpr: billing_setting.GetBillingExprCopy()}
	t.Cleanup(func() { billing_setting.ReplaceBundle(saved) })
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{BillingMode: map[string]string{name: billing_setting.BillingModeTieredExpr}, BillingExpr: map[string]string{name: expr}})
}

func TestVideoChannelTestsNeverGenerateOrRotateKeys(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer upstream.Close()
	for _, tc := range []struct {
		name              string
		kind              int
		mapping, endpoint string
	}{
		{"grok-imagine-video", constant.ChannelTypeXai, "", ""},
		{"sora-2", constant.ChannelTypeOpenAI, "", "openai"},
		{"wan2.6-t2v", constant.ChannelTypeAli, "", ""},
		{"happyhorse-1.0-t2v", constant.ChannelTypeAli, "", ""},
		{"seedance-2.0", constant.ChannelTypeVolcEngine, "", ""},
		{"MiniMax-Hailuo-02", constant.ChannelTypeMiniMax, "", ""},
		{"custom", constant.ChannelTypeKling, "", ""},
		{"alias", constant.ChannelTypeXai, `{"alias":"nested","nested":"grok-imagine-video"}`, ""},
		{"private-id", constant.ChannelTypeOpenAI, "", "openai-video"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := &model.Channel{Id: 75, Type: tc.kind, Models: tc.name, Group: "default", Key: "secret-one\nsecret-two", BaseURL: &upstream.URL, ModelMapping: &tc.mapping, TestTime: 12, ResponseTime: 999, Status: common.ChannelStatusAutoDisabled, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyPollingIndex: 1}}
			before := *ch
			// user 0 deliberately cannot run a real test: preflight must return
			// before user/cache/billing/key selection access.
			for i := 0; i < 3; i++ {
				result := testChannel(ch, 0, tc.name, tc.endpoint, nil, nil)
				require.NotNil(t, result.preflight)
				require.Nil(t, result.localErr)
				require.False(t, result.preflight.GenerationVerified)
				encoded, err := common.Marshal(videoPreflightResponse(result.preflight))
				require.NoError(t, err)
				require.NotContains(t, string(encoded), "secret-")
			}
			require.Equal(t, before, *ch)
		})
	}
	require.Zero(t, calls.Load())
}

func TestVideoExecutionGuardRunsBeforePreconsume(t *testing.T) {
	ctx, recorder := gin.CreateTestContext(httptest.NewRecorder())
	_ = recorder
	result := executeChannelStyleTest(ctx, httptest.NewRecorder(), &model.Channel{Type: constant.ChannelTypeXai}, "grok-imagine-video", "openai", &dto.GeneralOpenAIRequest{Model: "grok-imagine-video"}, types.RelayFormatOpenAI, false, testExecutionOptions{enableBilling: true})
	require.Error(t, result.localErr)
	require.Nil(t, result.info, "must stop before constructing a billing session")
}

func TestVideoConfigurationDoesNotAcceptImplicitPrice(t *testing.T) {
	ch := &model.Channel{Type: constant.ChannelTypeXai, Models: "grok-imagine-video-unpriced-test", Group: "default", Key: "key", BaseURL: common.GetPointer("https://example.test")}
	r := inspectVideoConfiguration(ch, ch.Models)
	require.False(t, r.Passed)
	for _, check := range r.Checks {
		if check.Name == "explicit_task_price" {
			require.False(t, check.OK)
		}
	}
	ch.ModelMapping = common.GetPointer(`{"grok-imagine-video-unpriced-test":"alias","alias":"grok-imagine-video-unpriced-test"}`)
	r = inspectVideoConfiguration(ch, ch.Models)
	for _, check := range r.Checks {
		if check.Name == "model_mapping" {
			require.False(t, check.OK)
		}
	}
}

func TestVideoPriceReferenceReusesTaskExpressionAndGroup(t *testing.T) {
	setupTokenModelHelperDB(t)
	name := "grok-imagine-video-test"
	setVideoTestPrice(t, name, `param("resolution") == "720p" ? tier("720", param("duration") * 200000) : tier("480", param("duration") * 100000)`)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":2}`))
	ch := &model.Channel{Group: "default"}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/preflight", nil)
	for _, tc := range []struct {
		duration   int
		resolution string
		dollars    float64
	}{{5, "720p", 2}, {10, "720p", 4}, {5, "480p", 1}} {
		q, err := calculateVideoPriceReference(ctx, ch, name, "default", "default", map[string]any{"duration": tc.duration, "resolution": tc.resolution})
		require.NoError(t, err)
		require.Equal(t, tc.dollars, q.AmountUSD)
		require.Equal(t, int(tc.dollars*common.QuotaPerUnit), q.Quota)
	}
	_, err := calculateVideoPriceReference(ctx, ch, name, "other", "default", map[string]any{})
	require.Error(t, err)
	_, err = calculateVideoPriceReference(ctx, ch, "unconfigured", "default", "default", map[string]any{})
	require.Error(t, err)
	setVideoTestPrice(t, name, `tier("free", 0)`)
	q, err := calculateVideoPriceReference(ctx, ch, name, "default", "default", map[string]any{})
	require.NoError(t, err)
	require.Zero(t, q.Quota)
}

func TestVideoHistoryIsBoundedPrivateAndScoped(t *testing.T) {
	setupTokenModelHelperDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Task{}))
	for _, task := range []model.Task{
		{ChannelId: 1, UserId: 1, Status: model.TaskStatusSuccess, SubmitTime: 123, Properties: model.Properties{OriginModelName: "video-a", Input: "private prompt"}, PrivateData: model.TaskPrivateData{Key: "secret-key", ResultURL: "private-url"}},
		{ChannelId: 1, UserId: 2, Status: model.TaskStatusFailure, SubmitTime: 124, Properties: model.Properties{OriginModelName: "video-a"}, FailReason: "secret-error"},
		{ChannelId: 2, UserId: 1, Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "video-a"}},
		{ChannelId: 1, UserId: 1, Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "other"}},
	} {
		require.NoError(t, model.DB.Create(&task).Error)
	}
	items, err := loadVideoHistory(1, "video-a", 1)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, int64(123), items[0].SubmitTime)
	items, err = loadVideoHistory(1, "video-a", 0)
	require.NoError(t, err)
	require.Len(t, items, 2)
	encoded, err := common.Marshal(items)
	require.NoError(t, err)
	for _, forbidden := range []string{"secret", "private", "user_id", "task_id", "url", "prompt"} {
		require.NotContains(t, string(encoded), forbidden)
	}
}

func TestVideoPreflightHTTPDoesNotGenerateOrPersist(t *testing.T) {
	setupTokenModelHelperDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Task{}, &model.Log{}))
	seedTokenModelHelperData(t)
	name := "grok-imagine-video"
	setVideoTestPrice(t, name, `tier("video", param("duration") * 100000)`)
	ch := model.Channel{Id: 87, Type: constant.ChannelTypeXai, Key: "never-expose", Models: name, Group: "default"}
	require.NoError(t, model.DB.Create(&ch).Error)
	for _, body := range []string{`{"model":"grok-imagine-video"}`, `{"model":"grok-imagine-video","group":"default","parameters":{"duration":5}}`} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/87/video/preflight", strings.NewReader(body))
		c.Params = gin.Params{{Key: "id", Value: "87"}}
		c.Set("id", 1)
		VideoChannelPreflight(c)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), `"test_kind":"video_preflight"`)
		require.Contains(t, w.Body.String(), `"upstream_requests":0`)
		require.NotContains(t, w.Body.String(), "never-expose")
	}
	var count int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, model.DB.Model(&model.Log{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestVideoTokenTestUsesOwnedTokenWithoutGeneration(t *testing.T) {
	setupTokenModelHelperDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Task{}, &model.Log{}))
	seedTokenModelHelperData(t)
	savedCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = savedCache })
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer upstream.Close()
	name := "grok-imagine-video-owned-test"
	setVideoTestPrice(t, name, `tier("video", 100000)`)
	ch := model.Channel{Id: 88, Type: constant.ChannelTypeXai, Key: "secret", Models: name, Group: "default", Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL}
	require.NoError(t, model.DB.Create(&ch).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: name, ChannelId: 88, Enabled: true}).Error)
	token := model.Token{Id: 88, UserId: 1, Key: "owned-test-token", Status: common.TokenStatusEnabled, Group: "default", UnlimitedQuota: true, ExpiredTime: -1}
	require.NoError(t, model.DB.Create(&token).Error)
	for _, userID := range []int{2, 1} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/token/test/88", strings.NewReader(`{"model":"`+name+`"}`))
		c.Params = gin.Params{{Key: "id", Value: "88"}}
		c.Set("id", userID)
		TestToken(c)
		if userID == 1 {
			require.Contains(t, w.Body.String(), `"test_kind":"video_preflight"`)
		} else {
			require.NotContains(t, w.Body.String(), `"test_kind":"video_preflight"`)
		}
	}
	require.Zero(t, calls.Load())
	var count int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, model.DB.Model(&model.Log{}).Count(&count).Error)
	require.Zero(t, count)
}
