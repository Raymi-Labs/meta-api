package service

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestShouldRetryChannelError_RequestedModelUnavailableBadRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	err := types.WithOpenAIError(types.OpenAIError{
		Message: "run error: 400 No available OpenAI accounts support the requested model: gpt-5.4",
		Type:    "upstream_error",
		Code:    nil,
	}, 400)

	if !ShouldRetryChannelError(ctx, err, 1) {
		t.Fatalf("expected requested-model-unavailable 400 to be retryable")
	}
}

func TestShouldRetryChannelError_GenericProcessingBadRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	err := types.WithOpenAIError(types.OpenAIError{
		Message: "An error occurred while processing your request. You can retry your request, or contact us through\nour help center at help.openai.com if the error persists. Please include the request ID 780e874b-",
		Type:    "upstream_error",
		Code:    nil,
	}, 400)

	if !ShouldRetryChannelError(ctx, err, 1) {
		t.Fatalf("expected upstream processing 400 to be retryable")
	}
}

func TestShouldRetryChannelError_GenericBadRequestRemainsNonRetryable(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	err := types.WithOpenAIError(types.OpenAIError{
		Message: "Invalid request",
		Type:    "invalid_request_error",
		Code:    "invalid_request_error",
	}, 400)

	if ShouldRetryChannelError(ctx, err, 1) {
		t.Fatalf("did not expect generic 400 to be retryable")
	}
}

func TestShouldRetryChannelError_UpstreamOverloadUsesAlternateChannel(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	originalRanges := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = nil
	t.Cleanup(func() {
		operation_setting.AutomaticRetryStatusCodeRanges = originalRanges
	})

	err := types.WithOpenAIError(types.OpenAIError{
		Message: "Our servers are currently overloaded. Please try again later.",
		Type:    "service_unavailable_error",
		Code:    "server_is_overloaded",
	}, http.StatusServiceUnavailable)

	if !ShouldRetryChannelError(ctx, err, 1) {
		t.Fatal("expected provider overload to reroute independently of generic retry ranges")
	}
	if ShouldRetryChannelError(ctx, err, 0) {
		t.Fatal("provider overload must respect the retry budget")
	}

	ctx.Set("specific_channel_id", 1367)
	if ShouldRetryChannelError(ctx, err, 1) {
		t.Fatal("provider overload must not reroute an explicitly pinned channel")
	}
}

func TestShouldRetryChannelError_RequestTooLargeOverKnownLimitStopsRetry(t *testing.T) {
	originLimit := constant.ResponsesRequestBodyLimitMB
	constant.ResponsesRequestBodyLimitMB = 20
	t.Cleanup(func() { constant.ResponsesRequestBodyLimitMB = originLimit })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	err := types.NewOpenAIError(errors.New("bad response status code 413"), types.ErrorCodeBadResponseStatusCode, http.StatusRequestEntityTooLarge)
	err.Upstream = &types.UpstreamDiagnostics{RequestLength: 30 << 20}

	if ShouldRetryChannelError(ctx, err, 1) {
		t.Fatalf("did not expect oversized responses request to be retried")
	}
}

func TestShouldRetryChannelError_RequestTooLargeBelowKnownLimitCanReroute(t *testing.T) {
	originLimit := constant.ResponsesRequestBodyLimitMB
	constant.ResponsesRequestBodyLimitMB = 20
	t.Cleanup(func() { constant.ResponsesRequestBodyLimitMB = originLimit })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	err := types.NewOpenAIError(errors.New("bad response status code 413"), types.ErrorCodeBadResponseStatusCode, http.StatusRequestEntityTooLarge)
	err.Upstream = &types.UpstreamDiagnostics{RequestLength: 2 << 20}

	if !ShouldRetryChannelError(ctx, err, 1) {
		t.Fatalf("expected below-limit 413 to be allowed to reroute to another non-equivalent channel")
	}
}

func TestShouldRetryChannelError_CRSAccountPoolUnavailablePaymentRequired(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	err := types.WithOpenAIError(types.OpenAIError{
		Message: "No available accounts in group Pro",
		Type:    "upstream_error",
		Code:    nil,
	}, http.StatusPaymentRequired)

	if !ShouldRetryChannelError(ctx, err, 1) {
		t.Fatalf("expected CRS account pool 402 to be retryable")
	}
}

func TestIsQuotaRelatedErrorByCode(t *testing.T) {
	err := types.NewError(errors.New("insufficient"), types.ErrorCodeInsufficientUserQuota)
	if !IsQuotaRelatedError(err) {
		t.Fatalf("expected quota related error by error code")
	}
}

func TestIsQuotaRelatedErrorByOpenAIType(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "quota exceeded",
		Type:    "insufficient_quota",
		Code:    "insufficient_quota",
	}, 403)
	if !IsQuotaRelatedError(err) {
		t.Fatalf("expected quota related error by openai type/code")
	}
}

func TestIsQuotaRelatedErrorNegative(t *testing.T) {
	err := types.NewError(errors.New("upstream timeout"), types.ErrorCodeDoRequestFailed)
	if IsQuotaRelatedError(err) {
		t.Fatalf("did not expect timeout to be quota related")
	}
}

func TestIsChannelModelMismatchError_CodexUnsupportedModel(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "The 'gpt-5.4' model is not supported when using Codex with a ChatGPT account.",
		Type:    "bad_response_status_code",
		Code:    "bad_response_status_code",
	}, 400)
	if !IsChannelModelMismatchError(err) {
		t.Fatalf("expected codex unsupported model to be treated as channel mismatch")
	}
}

func TestIsChannelModelMismatchError_StreamRequired(t *testing.T) {
	err := types.NewOpenAIError(errors.New("bad response status code 400, message: Stream must be set to true"), types.ErrorCodeBadResponseStatusCode, 400)
	if !IsChannelModelMismatchError(err) {
		t.Fatalf("expected stream-required error to be treated as channel mismatch")
	}
}

func TestIsChannelModelMismatchError_RequestedModelUnavailable(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "No available OpenAI accounts support the requested model: gpt-5.4",
		Type:    "upstream_error",
		Code:    nil,
	}, 503)
	if IsChannelModelMismatchError(err) {
		t.Fatalf("did not expect requested-model-unavailable error to be treated as channel mismatch")
	}
}

func TestIsUpstreamModelTemporaryUnavailableError_Positive(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "No available Claude accounts support the requested model: claude-opus-4-6",
		Type:    "upstream_error",
		Code:    nil,
	}, 503)
	if !IsUpstreamModelTemporaryUnavailableError(err) {
		t.Fatalf("expected requested-model-unavailable error to be treated as temporary upstream model unavailability")
	}
}

func TestIsTemporaryUpstreamError_GenericProcessingBadRequest(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "An error occurred while processing your request. You can retry your request, or contact us through\nour help center at help.openai.com if the error persists. Please include the request ID 780e874b-",
		Type:    "upstream_error",
		Code:    nil,
	}, 400)

	if !IsTemporaryUpstreamError(err) {
		t.Fatalf("expected upstream processing 400 to be treated as temporary")
	}
}

func TestIsChannelModelMismatchError_InvalidRequestNegative(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "Invalid request",
		Type:    "<nil>",
		Code:    nil,
	}, 400)
	if IsChannelModelMismatchError(err) {
		t.Fatalf("did not expect generic invalid request to be treated as channel mismatch")
	}
}

func TestIsRetryableUpstreamQuotaError_LocalQuotaNegative(t *testing.T) {
	err := types.NewError(errors.New("用户额度不足"), types.ErrorCodeInsufficientUserQuota, types.ErrOptionWithSkipRetry())
	if IsRetryableUpstreamQuotaError(err) {
		t.Fatalf("did not expect local skip-retry quota error to be treated as retryable upstream quota")
	}
}

func TestIsRetryableUpstreamQuotaError_UpstreamQuotaPositive(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "用户额度不足",
		Type:    "insufficient_user_quota",
		Code:    "insufficient_user_quota",
	}, 403)
	if !IsRetryableUpstreamQuotaError(err) {
		t.Fatalf("expected upstream quota error to be retryable")
	}
}

func TestIsRetryableSharedUpstreamPoolError_RateLimitPositive(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "rate limit exceeded",
		Type:    "upstream_error",
		Code:    nil,
	}, 429)
	if !IsRetryableSharedUpstreamPoolError(err) {
		t.Fatalf("expected upstream rate limit to be treated as shared upstream pool error")
	}
}

func TestIsCRSMemoryPressureError_CompactMessage(t *testing.T) {
	err := types.NewOpenAIError(
		errors.New("unexpected status 503 Service Unavailable:CRSis under memory pressure，please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		503,
	)

	if !IsCRSMemoryPressureError(err) {
		t.Fatalf("expected compact CRS memory pressure message to be detected")
	}
	if !IsRetryableSharedUpstreamPoolError(err) {
		t.Fatalf("expected CRS memory pressure to be treated as shared upstream pool error")
	}
}

func TestCRSMemoryPressureShortCircuitTripsBaseURLOnly(t *testing.T) {
	resetCRSShortCircuitForTest()
	originRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = originRedisEnabled
		resetCRSShortCircuitForTest()
	})

	tag := "crs-site-a"
	otherTag := "crs-site-b"
	baseURL := "https://CRS.example.com/v1/"
	normalizedBaseURL := "https://crs.example.com/v1"
	otherBaseURL := "https://other.example.com/v1"
	channel := &model.Channel{Id: 10, Status: common.ChannelStatusEnabled, Tag: &tag, BaseURL: &baseURL}
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "CRS is under memory pressure, please retry later",
		Type:    "upstream_error",
		Code:    nil,
	}, 503)

	trips := ApplyCRSMemoryPressureShortCircuit(channel, err)
	if len(trips) != 1 || trips[0].Scope != "base_url" {
		t.Fatalf("expected only base_url trip, got %+v", trips)
	}
	if !IsChannelUnavailableForRequest(channel) {
		t.Fatalf("expected original channel to be short-circuited")
	}
	if IsChannelUnavailableForRequest(&model.Channel{Id: 11, Status: common.ChannelStatusEnabled, Tag: &tag, BaseURL: &otherBaseURL}) {
		t.Fatalf("did not expect same tag with another base_url to be short-circuited")
	}
	if !IsChannelUnavailableForRequest(&model.Channel{Id: 12, Status: common.ChannelStatusEnabled, Tag: &otherTag, BaseURL: &normalizedBaseURL}) {
		t.Fatalf("expected same base_url to be short-circuited")
	}
	if IsChannelUnavailableForRequest(&model.Channel{Id: 13, Status: common.ChannelStatusEnabled, Tag: &otherTag, BaseURL: &otherBaseURL}) {
		t.Fatalf("did not expect unrelated channel to be short-circuited")
	}
}

func TestApplyChannelFailureRetryExclusion_QuotaExcludesOnlyFailedChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	originDB := model.DB
	originLogDB := model.LOG_DB
	originMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.MemoryCacheEnabled = originMemoryCacheEnabled
	})

	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	tag := "shared-upstream"
	channels := []model.Channel{
		{Id: 1, Name: "primary", Status: common.ChannelStatusEnabled, Tag: &tag},
		{Id: 2, Name: "sibling", Status: common.ChannelStatusEnabled, Tag: &tag},
		{Id: 3, Name: "other-model", Status: common.ChannelStatusEnabled, Tag: &tag},
		{Id: 4, Name: "other-group", Status: common.ChannelStatusEnabled, Tag: &tag},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("seed channels: %v", err)
	}
	abilities := []model.Ability{
		{Group: "default", Model: "gpt-5.4", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "gpt-5.4", ChannelId: 2, Enabled: true},
		{Group: "default", Model: "gpt-4.1", ChannelId: 3, Enabled: true},
		{Group: "vip", Model: "gpt-5.4", ChannelId: 4, Enabled: true},
	}
	if err := db.Create(&abilities).Error; err != nil {
		t.Fatalf("seed abilities: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.4",
	}
	retryErr := types.WithOpenAIError(types.OpenAIError{
		Message: "insufficient quota",
		Type:    "insufficient_quota",
		Code:    "insufficient_quota",
	}, 429)

	ApplyChannelFailureRetryExclusion(param, &channels[0], retryErr)

	if len(param.ExcludeChannels) != 1 {
		t.Fatalf("expected only failed channel, got %v", param.ExcludeChannels)
	}
	if !slices.Contains(param.ExcludeChannels, 1) {
		t.Fatalf("unexpected excluded channels: %v", param.ExcludeChannels)
	}
	if slices.Contains(param.ExcludeChannels, 2) || slices.Contains(param.ExcludeChannels, 3) || slices.Contains(param.ExcludeChannels, 4) {
		t.Fatalf("unexpected sibling channels excluded: %v", param.ExcludeChannels)
	}
}

func TestApplyChannelFailureRetryExclusion_TemporaryModelUnavailableExcludesOnlyFailedChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	originDB := model.DB
	originLogDB := model.LOG_DB
	originMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.MemoryCacheEnabled = originMemoryCacheEnabled
	})

	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	tag := "shared-upstream"
	channels := []model.Channel{
		{Id: 7, Name: "primary", Status: common.ChannelStatusEnabled, Tag: &tag},
		{Id: 8, Name: "sibling", Status: common.ChannelStatusEnabled, Tag: &tag},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("seed channels: %v", err)
	}
	abilities := []model.Ability{
		{Group: "default", Model: "claude-opus-4-6", ChannelId: 7, Enabled: true},
		{Group: "default", Model: "claude-opus-4-6", ChannelId: 8, Enabled: true},
	}
	if err := db.Create(&abilities).Error; err != nil {
		t.Fatalf("seed abilities: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	param := &RetryParam{}
	param.Ctx = ctx
	param.TokenGroup = "default"
	param.ModelName = "claude-opus-4-6"
	channel := &channels[0]
	retryErr := types.WithOpenAIError(types.OpenAIError{
		Message: "No available Claude accounts support the requested model: claude-opus-4-6",
		Type:    "upstream_error",
		Code:    nil,
	}, 503)

	ApplyChannelFailureRetryExclusion(param, channel, retryErr)

	if len(param.ExcludeChannels) != 1 {
		t.Fatalf("expected only failed channel, got %v", param.ExcludeChannels)
	}
	if !slices.Contains(param.ExcludeChannels, 7) || slices.Contains(param.ExcludeChannels, 8) {
		t.Fatalf("unexpected excluded channels: %v", param.ExcludeChannels)
	}
}

func TestApplyChannelFailureRetryExclusion_CRSAccountPoolUnavailableExcludesOnlyFailedChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	originDB := model.DB
	originLogDB := model.LOG_DB
	originMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.MemoryCacheEnabled = originMemoryCacheEnabled
	})

	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	tagA := "crs-ccmx.meta-api.vip - pro"
	tagB := "crs-other.meta-api.vip - pro"
	baseURL := "http://192.3.179.236:3000/openai"
	channels := []model.Channel{
		{Id: 11, Name: "primary", Status: common.ChannelStatusEnabled, Tag: &tagA, BaseURL: &baseURL},
		{Id: 12, Name: "same-tag", Status: common.ChannelStatusEnabled, Tag: &tagA, BaseURL: &baseURL},
		{Id: 13, Name: "other-tag-same-base", Status: common.ChannelStatusEnabled, Tag: &tagB, BaseURL: &baseURL},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("seed channels: %v", err)
	}
	abilities := []model.Ability{
		{Group: "vip", Model: "gpt-5.5", ChannelId: 11, Enabled: true},
		{Group: "vip", Model: "gpt-5.5", ChannelId: 12, Enabled: true},
		{Group: "vip", Model: "gpt-5.5", ChannelId: 13, Enabled: true},
	}
	if err := db.Create(&abilities).Error; err != nil {
		t.Fatalf("seed abilities: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "vip")
	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "vip",
		ModelName:  "gpt-5.5",
	}
	retryErr := types.WithOpenAIError(types.OpenAIError{
		Message: "No available accounts in group Pro",
		Type:    "upstream_error",
		Code:    nil,
	}, http.StatusPaymentRequired)

	ApplyChannelFailureRetryExclusion(param, &channels[0], retryErr)

	if len(param.ExcludeChannels) != 1 {
		t.Fatalf("expected only failed channel, got %v", param.ExcludeChannels)
	}
	if !slices.Contains(param.ExcludeChannels, 11) {
		t.Fatalf("unexpected excluded channels: %v", param.ExcludeChannels)
	}
	if slices.Contains(param.ExcludeChannels, 12) || slices.Contains(param.ExcludeChannels, 13) {
		t.Fatalf("did not expect sibling channels to be excluded: %v", param.ExcludeChannels)
	}
}

func TestApplyChannelFailureRetryExclusion_ChannelMismatchExcludesOnlyFailedChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	originDB := model.DB
	originLogDB := model.LOG_DB
	originMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.MemoryCacheEnabled = originMemoryCacheEnabled
	})

	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	tag := "shared-crs"
	channels := []model.Channel{
		{Id: 21, Name: "primary", Status: common.ChannelStatusEnabled, Tag: &tag},
		{Id: 22, Name: "sibling", Status: common.ChannelStatusEnabled, Tag: &tag},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("seed channels: %v", err)
	}
	abilities := []model.Ability{
		{Group: "vip", Model: "gpt-5.5", ChannelId: 21, Enabled: true},
		{Group: "vip", Model: "gpt-5.5", ChannelId: 22, Enabled: true},
	}
	if err := db.Create(&abilities).Error; err != nil {
		t.Fatalf("seed abilities: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "vip")
	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "vip",
		ModelName:  "gpt-5.5",
	}
	retryErr := types.NewOpenAIError(errors.New("bad response status code 400, message: Stream must be set to true"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)

	ApplyChannelFailureRetryExclusion(param, &channels[0], retryErr)

	if len(param.ExcludeChannels) != 1 {
		t.Fatalf("expected only failed channel, got %v", param.ExcludeChannels)
	}
	if !slices.Contains(param.ExcludeChannels, 21) || slices.Contains(param.ExcludeChannels, 22) {
		t.Fatalf("unexpected excluded channels: %v", param.ExcludeChannels)
	}
}

func TestApplyChannelFailureRetryExclusion_RequestTooLargeWithoutBaseURLExcludesOnlyFailedChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	originDB := model.DB
	originLogDB := model.LOG_DB
	originMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.MemoryCacheEnabled = originMemoryCacheEnabled
	})

	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	tag := "shared-crs"
	channels := []model.Channel{
		{Id: 31, Name: "primary", Status: common.ChannelStatusEnabled, Tag: &tag},
		{Id: 32, Name: "sibling", Status: common.ChannelStatusEnabled, Tag: &tag},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("seed channels: %v", err)
	}
	abilities := []model.Ability{
		{Group: "vip", Model: "gpt-5.5", ChannelId: 31, Enabled: true},
		{Group: "vip", Model: "gpt-5.5", ChannelId: 32, Enabled: true},
	}
	if err := db.Create(&abilities).Error; err != nil {
		t.Fatalf("seed abilities: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "vip")
	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "vip",
		ModelName:  "gpt-5.5",
	}
	retryErr := types.NewOpenAIError(errors.New("bad response status code 413"), types.ErrorCodeBadResponseStatusCode, http.StatusRequestEntityTooLarge)

	ApplyChannelFailureRetryExclusion(param, &channels[0], retryErr)

	if len(param.ExcludeChannels) != 1 {
		t.Fatalf("expected only failed channel, got %v", param.ExcludeChannels)
	}
	if !slices.Contains(param.ExcludeChannels, 31) || slices.Contains(param.ExcludeChannels, 32) {
		t.Fatalf("unexpected excluded channels: %v", param.ExcludeChannels)
	}
}

func TestApplyChannelFailureRetryExclusion_RequestTooLargeUsesBaseURLGroup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	originDB := model.DB
	originLogDB := model.LOG_DB
	originMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.MemoryCacheEnabled = originMemoryCacheEnabled
	})

	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	tagA := "shared-crs-a"
	tagB := "shared-crs-b"
	baseURL := "https://CRS.example.com/openai/"
	sameBaseURL := "https://crs.example.com/openai"
	otherBaseURL := "https://other.example.com/openai"
	channels := []model.Channel{
		{Id: 41, Name: "primary", Status: common.ChannelStatusEnabled, Tag: &tagA, BaseURL: &baseURL},
		{Id: 42, Name: "same-base", Status: common.ChannelStatusEnabled, Tag: &tagB, BaseURL: &sameBaseURL},
		{Id: 43, Name: "other-base", Status: common.ChannelStatusEnabled, Tag: &tagB, BaseURL: &otherBaseURL},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("seed channels: %v", err)
	}
	abilities := []model.Ability{
		{Group: "vip", Model: "gpt-5.5", ChannelId: 41, Enabled: true},
		{Group: "vip", Model: "gpt-5.5", ChannelId: 42, Enabled: true},
		{Group: "vip", Model: "gpt-5.5", ChannelId: 43, Enabled: true},
	}
	if err := db.Create(&abilities).Error; err != nil {
		t.Fatalf("seed abilities: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "vip")
	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "vip",
		ModelName:  "gpt-5.5",
	}
	retryErr := types.NewOpenAIError(errors.New("bad response status code 413"), types.ErrorCodeBadResponseStatusCode, http.StatusRequestEntityTooLarge)

	ApplyChannelFailureRetryExclusion(param, &channels[0], retryErr)

	if !slices.Contains(param.ExcludeChannels, 41) || !slices.Contains(param.ExcludeChannels, 42) {
		t.Fatalf("expected same base_url channels to be excluded, got %v", param.ExcludeChannels)
	}
	if slices.Contains(param.ExcludeChannels, 43) {
		t.Fatalf("did not expect unrelated base_url channel to be excluded: %v", param.ExcludeChannels)
	}
}

func TestShouldDisableChannelIgnoresLocalErrors(t *testing.T) {
	previousAutoDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = previousAutoDisable })
	upstreamKeyErr := types.WithOpenAIError(types.OpenAIError{Message: "Incorrect API key provided", Type: "invalid_request_error", Code: "invalid_api_key"}, http.StatusUnauthorized)
	for _, tc := range []struct {
		name        string
		err         *types.NewAPIError
		source      string
		wantDisable bool
	}{
		{name: "upstream invalid key", err: upstreamKeyErr, source: "upstream", wantDisable: true},
		{name: "upstream keyword in a 403 response", err: types.NewOpenAIError(errors.New("Permission denied"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden), source: "upstream", wantDisable: true},
		{name: "upstream keyword in an adaptor-parsed body", err: types.NewError(errors.New("Your credit balance is too low"), types.ErrorCodeBadResponseBody), source: "upstream", wantDisable: true},
		{name: "channel key error", err: types.NewError(errors.New("invalid auth"), types.ErrorCodeChannelInvalidKey), source: "channel", wantDisable: true},
		{name: "channel model mapping error", err: types.NewError(errors.New("bad mapping"), types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry()), source: "channel", wantDisable: true},
		{name: "local 401 from request conversion", err: types.NewErrorWithStatusCode(errors.New("bad image"), types.ErrorCodeConvertRequestFailed, http.StatusUnauthorized), source: "local", wantDisable: false},
		{name: "local error carrying a disable keyword from client input", err: types.NewError(errors.New("fetch https://x.invalid/Permission denied failed"), types.ErrorCodeConvertRequestFailed), source: "local", wantDisable: false},
		{name: "override condition type mismatch", err: types.NewErrorWithStatusCode(errors.New("comparison failed for path temperature"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry()), source: "local", wantDisable: false},
		{name: "local wrapper around an upstream key error", err: types.NewErrorWithStatusCode(upstreamKeyErr, types.ErrorCodeInvalidRequest, http.StatusUnauthorized), source: "upstream", wantDisable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			assert.Equal(t, tc.source, RelayErrorSource(tc.err))
			assert.Equal(t, tc.wantDisable, ShouldDisableChannel(0, tc.err))
			if tc.source == "local" {
				assert.False(t, ShouldRetryChannelError(c, tc.err, 1))
			}
		})
	}
}
