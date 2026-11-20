package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestFilterVisibleModelsByRatioIncludesTieredOnlyModel(t *testing.T) {
	saved := billing_setting.TieredBundle{
		BillingMode: billing_setting.GetBillingModeCopy(),
		BillingExpr: billing_setting.GetBillingExprCopy(),
	}
	t.Cleanup(func() { billing_setting.ReplaceBundle(saved) })
	billing_setting.ReplaceBundle(billing_setting.TieredBundle{
		BillingMode: map[string]string{"tier-only-model": billing_setting.BillingModeTieredExpr},
		BillingExpr: map[string]string{"tier-only-model": `tier("base", p * 2 + c * 8)`},
	})

	models := filterVisibleModelsByRatio([]string{"tier-only-model", "missing-price-model"}, false)
	if len(models) != 1 || models[0] != "tier-only-model" {
		t.Fatalf("unexpected filtered models: %#v", models)
	}
}

func setupTokenModelHelperDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	model.InvalidateModelPermissionCache()
	common.RedisEnabled = false
	originSelfUseModeEnabled := operation_setting.SelfUseModeEnabled
	operation_setting.SelfUseModeEnabled = true
	t.Cleanup(func() {
		operation_setting.SelfUseModeEnabled = originSelfUseModeEnabled
		model.InvalidateModelPermissionCache()
	})

	if err := db.AutoMigrate(&model.User{}, &model.Token{}, &model.Ability{}, &model.Channel{}, &model.Model{}, &model.ModelPermission{}, &model.Vendor{}, &model.OfficialReasoningSnapshot{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	if err := setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"VIP","team":"团队"}`); err != nil {
		t.Fatalf("update user usable groups: %v", err)
	}
	if err := setting.UpdateAutoGroupsByJsonString(`["default","vip"]`); err != nil {
		t.Fatalf("update auto groups: %v", err)
	}
	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"team":1}`); err != nil {
		t.Fatalf("update group ratio: %v", err)
	}
}

func seedTokenModelHelperData(t *testing.T) {
	t.Helper()

	users := []model.User{
		{Id: 1, Username: "user1", Group: "default", Status: common.UserStatusEnabled, AffCode: "aff-user1"},
	}
	if err := model.DB.Create(&users).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}

	channels := []model.Channel{
		{Id: 1, Name: "default-openai", Key: "sk-test-1", Tag: common.GetPointer[string]("shared-openai")},
		{Id: 2, Name: "default-anthropic", Key: "sk-test-2", Tag: common.GetPointer[string]("shared-claude"), Type: constant.ChannelTypeAnthropic},
		{Id: 3, Name: "default-gemini", Key: "sk-test-3", Type: constant.ChannelTypeGemini},
	}
	if err := model.DB.Create(&channels).Error; err != nil {
		t.Fatalf("seed channels: %v", err)
	}

	abilities := []model.Ability{
		{Group: "default", Model: "gpt-5.2", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "gpt-5.2-codex", ChannelId: 1, Enabled: true},
		{Group: "vip", Model: "claude-sonnet-4-6", ChannelId: 2, Enabled: true},
		{Group: "vip", Model: "gpt-5.2", ChannelId: 1, Enabled: true},
		{Group: "team", Model: "gemini-2.5-pro", ChannelId: 3, Enabled: true},
	}
	if err := model.DB.Create(&abilities).Error; err != nil {
		t.Fatalf("seed abilities: %v", err)
	}
}

func seedTokenModelHelperUser(t *testing.T) {
	t.Helper()

	users := []model.User{
		{Id: 1, Username: "user1", Group: "default", Status: common.UserStatusEnabled, AffCode: "aff-user1"},
	}
	if err := model.DB.Create(&users).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}
}

func TestResolveRequestedTokenModels_DefaultGroupOnly(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperData(t)

	models, err := resolveRequestedTokenModels(1, "")
	if err != nil {
		t.Fatalf("resolve requested token models: %v", err)
	}
	if len(models) != 2 || models[0] != "gpt-5.2" || models[1] != "gpt-5.2-codex" {
		t.Fatalf("unexpected default group models: %#v", models)
	}
}

func TestResolveRequestedTokenModels_AutoGroupUnion(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperData(t)

	models, err := resolveRequestedTokenModels(1, "auto")
	if err != nil {
		t.Fatalf("resolve auto group models: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("unexpected auto group model count: %#v", models)
	}
	if models[0] != "claude-sonnet-4-6" || models[1] != "gpt-5.2" || models[2] != "gpt-5.2-codex" {
		t.Fatalf("unexpected auto group models: %#v", models)
	}
}

func TestResolveTokenAllowedModels_IntersectsModelLimits(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperData(t)

	token := &model.Token{
		Id:                 11,
		UserId:             1,
		Group:              "auto",
		ModelLimitsEnabled: true,
		ModelLimits:        "gpt-5.2,gemini-2.5-pro",
	}
	models, err := resolveTokenAllowedModels(1, token)
	if err != nil {
		t.Fatalf("resolve token allowed models: %v", err)
	}
	if len(models) != 1 || models[0] != "gpt-5.2" {
		t.Fatalf("unexpected token allowed models: %#v", models)
	}
}

func TestResolveRequestedTokenModels_FiltersVisibilityOnlyForLists(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperData(t)

	permission := model.ModelPermission{
		ModelName:       "gpt-5.2-codex",
		NameRule:        model.NameRuleExact,
		VisibilityScope: model.ModelPermissionScopeNone,
		CallScope:       model.ModelPermissionScopeAll,
	}
	if err := permission.Insert(); err != nil {
		t.Fatalf("insert model permission: %v", err)
	}

	models, err := resolveRequestedTokenModels(1, "")
	if err != nil {
		t.Fatalf("resolve requested token models: %v", err)
	}
	if len(models) != 1 || models[0] != "gpt-5.2" {
		t.Fatalf("unexpected visible models: %#v", models)
	}

	rawModels, err := resolveRequestedTokenModelsRaw(1, "")
	if err != nil {
		t.Fatalf("resolve raw token models: %v", err)
	}
	if len(rawModels) != 2 {
		t.Fatalf("raw models should not be visibility-filtered: %#v", rawModels)
	}
}

func TestBuildTokenModelOptions_IncludesSupportedEndpointTypes(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperData(t)

	token := &model.Token{
		Id:     11,
		UserId: 1,
		Group:  "auto",
	}
	options, err := buildTokenModelOptions(1, token, "")
	if err != nil {
		t.Fatalf("build token model options: %v", err)
	}
	if len(options) != 3 {
		t.Fatalf("unexpected options: %#v", options)
	}

	if options[0].Name != "claude-sonnet-4-6" {
		t.Fatalf("unexpected first option: %#v", options[0])
	}
	if len(options[0].SupportedEndpointTypes) != 2 ||
		options[0].SupportedEndpointTypes[0] != constant.EndpointTypeAnthropic ||
		options[0].SupportedEndpointTypes[1] != constant.EndpointTypeOpenAI {
		t.Fatalf("unexpected endpoints for claude model: %#v", options[0].SupportedEndpointTypes)
	}

	if options[1].Name != "gpt-5.2" {
		t.Fatalf("unexpected second option: %#v", options[1])
	}
	if len(options[1].SupportedEndpointTypes) != 2 ||
		options[1].SupportedEndpointTypes[0] != constant.EndpointTypeOpenAI ||
		options[1].SupportedEndpointTypes[1] != constant.EndpointTypeOpenAIResponse {
		t.Fatalf("unexpected endpoints for gpt-5.2: %#v", options[1].SupportedEndpointTypes)
	}

	if options[2].Name != "gpt-5.2-codex" {
		t.Fatalf("unexpected third option: %#v", options[2])
	}
	if len(options[2].SupportedEndpointTypes) != 1 || options[2].SupportedEndpointTypes[0] != constant.EndpointTypeOpenAIResponse {
		t.Fatalf("unexpected endpoints for codex model: %#v", options[2].SupportedEndpointTypes)
	}
}

func TestResolveTestRequestPath_ClaudeModelDefaultsToChatCompletions(t *testing.T) {
	requestPath, _ := resolveTestRequestPath(nil, "claude-sonnet-4-6", "")
	if requestPath != "/v1/chat/completions" {
		t.Fatalf("unexpected request path: %s", requestPath)
	}
}

func TestResolveTestRequestPath_AnthropicClaudeModelUsesMessages(t *testing.T) {
	requestPath, _ := resolveTestRequestPath(&model.Channel{Type: constant.ChannelTypeAnthropic}, "claude-sonnet-4-6", "")
	if requestPath != "/v1/messages" {
		t.Fatalf("unexpected request path: %s", requestPath)
	}
}

func TestResolveTestRequestPath_ImageGenerationModelUsesImagesEndpoint(t *testing.T) {
	requestPath, endpointType := resolveTestRequestPath(&model.Channel{Id: 1}, "gpt-image-2", "")
	if requestPath != "/v1/images/generations" {
		t.Fatalf("unexpected request path: %s", requestPath)
	}
	if endpointType != string(constant.EndpointTypeImageGeneration) {
		t.Fatalf("unexpected endpoint type: %s", endpointType)
	}
}

func TestResolveTestRequestPath_GrokVideoUsesVideosEndpoint(t *testing.T) {
	requestPath, endpointType := resolveTestRequestPath(&model.Channel{Type: constant.ChannelTypeXai}, "grok-imagine-video", "")
	if requestPath != "/v1/videos" {
		t.Fatalf("unexpected request path: %s", requestPath)
	}
	if endpointType != string(constant.EndpointTypeOpenAIVideo) {
		t.Fatalf("unexpected endpoint type: %s", endpointType)
	}
	request := buildTestRequest("grok-imagine-video", endpointType, &model.Channel{Type: constant.ChannelTypeXai}, false)
	if _, ok := request.(*dto.OpenAIVideoRequest); !ok {
		t.Fatalf("unexpected video test request type: %T", request)
	}
}

func TestNormalizeTokenModelSupportedEndpointTypes_ImageGenerationUsesCanonicalEndpoint(t *testing.T) {
	endpointTypes := normalizeTokenModelSupportedEndpointTypes("gpt-image-2", []constant.EndpointType{
		constant.EndpointTypeOpenAI,
		constant.EndpointTypeOpenAIResponse,
	})
	if len(endpointTypes) != 1 || endpointTypes[0] != constant.EndpointTypeImageGeneration {
		t.Fatalf("unexpected image endpoint types: %#v", endpointTypes)
	}
}

func TestDetectErrorFromTestResponseBody_IgnoresNullError(t *testing.T) {
	body := []byte(`{"id":"resp_123","status":"completed","error":null,"output":[]}`)
	if err := detectErrorFromTestResponseBody(body); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestDetectErrorFromTestResponseBody_ReturnsOpenAIErrorMessage(t *testing.T) {
	body := []byte(`{"error":{"message":"upstream boom","type":"invalid_request_error"}}`)
	err := detectErrorFromTestResponseBody(body)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "upstream boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectErrorMessageFromJSONBytes_PrefersTopLevelMessageOverGenericErrorString(t *testing.T) {
	body := []byte(`{"error":"Relay service error","message":"No available Claude accounts support the requested model: claude-opus-4-6"}`)
	message := detectErrorMessageFromJSONBytes(body)
	if message != "No available Claude accounts support the requested model: claude-opus-4-6" {
		t.Fatalf("unexpected message: %s", message)
	}
}

func TestResolveChannelTestFailure_PrefersUpstreamMessageAndStatusCode(t *testing.T) {
	body := []byte(`{"error":"Relay service error","message":"No available Claude accounts support the requested model: claude-opus-4-6"}`)
	result := testResult{
		localErr:     errors.New("bad response status code 503, message: Relay service error"),
		newAPIError:  types.NewOpenAIError(errors.New("bad response status code 503, message: Relay service error"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable),
		responseBody: body,
	}
	message, statusCode := resolveChannelTestFailure(result)
	if statusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status code: %d", statusCode)
	}
	if message != "No available Claude accounts support the requested model: claude-opus-4-6" {
		t.Fatalf("unexpected message: %s", message)
	}
}

func TestSelectTokenTestChannelByAbility_IgnoresChannelModelsMismatch(t *testing.T) {
	setupTokenModelHelperDB(t)

	users := []model.User{
		{Id: 1, Username: "user1", Group: "default", Status: common.UserStatusEnabled, AffCode: "aff-user1"},
	}
	if err := model.DB.Create(&users).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}

	channels := []model.Channel{
		{Id: 10, Name: "wrong-codex", Key: "sk-test-codex", Type: constant.ChannelTypeCodex, Group: "default", Models: "gpt-5.4"},
		{Id: 11, Name: "right-openai", Key: "sk-test-openai", Group: "default", Models: "gpt-5.4"},
	}
	if err := model.DB.Create(&channels).Error; err != nil {
		t.Fatalf("seed channels: %v", err)
	}

	abilities := []model.Ability{
		{Group: "default", Model: "gpt-5.4", ChannelId: 11, Enabled: true, Priority: common.GetPointer[int64](0), Weight: 100},
	}
	if err := model.DB.Create(&abilities).Error; err != nil {
		t.Fatalf("seed abilities: %v", err)
	}

	token := &model.Token{
		Id:     20,
		UserId: 1,
		Group:  "default",
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/token/test/20", nil)
	ctx.Request = req

	if err := prepareOwnedTokenContext(ctx, token); err != nil {
		t.Fatalf("prepare context: %v", err)
	}

	channel, selectedGroup, err := selectTokenTestChannelByAbility(ctx, "gpt-5.4")
	if err != nil {
		t.Fatalf("select channel: %v", err)
	}
	if channel == nil {
		t.Fatal("expected channel, got nil")
	}
	if channel.Id != 11 {
		t.Fatalf("expected ability-matched channel 11, got %d", channel.Id)
	}
	if selectedGroup != "default" {
		t.Fatalf("expected selected group default, got %s", selectedGroup)
	}
}

func TestBuildChannelStyleTestExecution_AnthropicUsesMessagesStream(t *testing.T) {
	execution := buildChannelStyleTestExecution(&model.Channel{Type: constant.ChannelTypeAnthropic}, "claude-sonnet-4-6", "", nil)
	if execution.requestPath != "/v1/messages" {
		t.Fatalf("unexpected request path: %s", execution.requestPath)
	}
	if !execution.isStream {
		t.Fatal("expected anthropic test to use stream mode")
	}
	req, ok := execution.request.(*dto.ClaudeRequest)
	if !ok {
		t.Fatalf("unexpected request type: %T", execution.request)
	}
	if !req.Stream {
		t.Fatal("expected claude request stream=true")
	}
}

func TestBuildChannelStyleTestExecution_ChatToResponsesUsesResponsesStream(t *testing.T) {
	channel := &model.Channel{Id: 1}
	channel.SetSetting(dto.ChannelSettings{
		ChatCompletionsToResponsesMode: dto.ChatCompletionsToResponsesModeEnabled,
	})

	execution := buildChannelStyleTestExecution(channel, "gpt-5", "", nil)
	if execution.requestPath != "/v1/responses" {
		t.Fatalf("unexpected request path: %s", execution.requestPath)
	}
	if execution.endpointType != string(constant.EndpointTypeOpenAIResponse) {
		t.Fatalf("unexpected endpoint type: %s", execution.endpointType)
	}
	if !execution.isStream {
		t.Fatal("expected responses test to use stream mode")
	}
	req, ok := execution.request.(*dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("unexpected request type: %T", execution.request)
	}
	if !req.Stream {
		t.Fatal("expected responses request stream=true")
	}
}

func TestBuildChannelStyleTestExecution_ImageGenerationUsesImageRequest(t *testing.T) {
	execution := buildChannelStyleTestExecution(&model.Channel{Id: 1}, "gpt-image-2", "", nil)
	if execution.requestPath != "/v1/images/generations" {
		t.Fatalf("unexpected request path: %s", execution.requestPath)
	}
	if execution.endpointType != string(constant.EndpointTypeImageGeneration) {
		t.Fatalf("unexpected endpoint type: %s", execution.endpointType)
	}
	if execution.relayFormat != types.RelayFormatOpenAIImage {
		t.Fatalf("unexpected relay format: %s", execution.relayFormat)
	}
	if execution.isStream {
		t.Fatal("image generation test should not use stream mode")
	}
	req, ok := execution.request.(*dto.ImageRequest)
	if !ok {
		t.Fatalf("unexpected request type: %T", execution.request)
	}
	if req.Model != "gpt-image-2" || req.Prompt == "" {
		t.Fatalf("unexpected image request: %#v", req)
	}
	if req.N == nil || *req.N != 1 || req.Size != "1024x1024" || req.Quality != "low" {
		t.Fatalf("unexpected image test parameters: %#v", req)
	}
}

func TestBuildChannelStyleTestExecution_OpenAIChatTestUsesStream(t *testing.T) {
	execution := buildChannelStyleTestExecution(&model.Channel{Id: 1}, "gpt-5.2", "", nil)
	if execution.requestPath != "/v1/chat/completions" {
		t.Fatalf("unexpected request path: %s", execution.requestPath)
	}
	if !execution.isStream {
		t.Fatal("expected openai chat test to use stream mode")
	}
	req, ok := execution.request.(*dto.GeneralOpenAIRequest)
	if !ok {
		t.Fatalf("unexpected request type: %T", execution.request)
	}
	if !req.Stream {
		t.Fatal("expected openai chat request stream=true")
	}
	if req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
		t.Fatal("expected openai chat stream options to include usage")
	}
}

func TestBuildChannelStyleTestExecution_GPT54ChatTestUsesStream(t *testing.T) {
	execution := buildChannelStyleTestExecution(&model.Channel{Id: 1}, "gpt-5.4", "", nil)
	if execution.requestPath != "/v1/chat/completions" {
		t.Fatalf("unexpected request path: %s", execution.requestPath)
	}
	if !execution.isStream {
		t.Fatal("expected gpt-5.4 chat test to use stream mode")
	}
}

func TestCreateInternalJSONContext_UsesStreamAcceptHeader(t *testing.T) {
	recorder := httptest.NewRecorder()
	parent, _ := gin.CreateTestContext(recorder)
	parent.Request = httptest.NewRequest(http.MethodPost, "/api/token/test/1", nil)

	ctx, _ := createInternalJSONContext(parent, http.MethodPost, "/v1/responses", nil, tokenTestAcceptHeader(true))
	if got := ctx.Request.Header.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("unexpected accept header: %s", got)
	}
}

func TestTokenTestRuntimeLimit_AppliesConcurrencyLimit(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperUser(t)

	token := &model.Token{
		Id:             101,
		UserId:         1,
		Group:          "default",
		MaxConcurrency: 1,
	}

	ctx1, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx1.Request = httptest.NewRequest(http.MethodPost, "/api/token/test/101", nil)
	if err := prepareOwnedTokenContext(ctx1, token); err != nil {
		t.Fatalf("prepare first context: %v", err)
	}
	release1, err := middleware.AcquireTokenRuntimeLimit(ctx1)
	if err != nil {
		t.Fatalf("acquire first runtime limit: %v", err)
	}
	defer release1()

	concurrency, err := middleware.QueryTokenConcurrency(token.Id)
	if err != nil {
		t.Fatalf("query concurrency: %v", err)
	}
	if concurrency != 1 {
		t.Fatalf("unexpected concurrency after first acquire: %d", concurrency)
	}

	ctx2, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx2.Request = httptest.NewRequest(http.MethodPost, "/api/token/test/101", nil)
	if err := prepareOwnedTokenContext(ctx2, token); err != nil {
		t.Fatalf("prepare second context: %v", err)
	}
	_, err = middleware.AcquireTokenRuntimeLimit(ctx2)
	if err == nil {
		t.Fatal("expected second acquire to hit concurrency limit")
	}
	runtimeErr, ok := err.(*middleware.TokenRuntimeLimitError)
	if !ok {
		t.Fatalf("expected runtime limit error, got %T", err)
	}
	if runtimeErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("unexpected status code: %d", runtimeErr.StatusCode)
	}
	if !strings.Contains(runtimeErr.Error(), "并发已达到上限 1") {
		t.Fatalf("unexpected error message: %v", runtimeErr)
	}
}

func TestTokenTestRuntimeLimit_AppliesWindowLimit(t *testing.T) {
	setupTokenModelHelperDB(t)
	seedTokenModelHelperUser(t)

	token := &model.Token{
		Id:                 102,
		UserId:             1,
		Group:              "default",
		WindowRequestLimit: 1,
		WindowSeconds:      60,
	}

	ctx1, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx1.Request = httptest.NewRequest(http.MethodPost, "/api/token/test/102", nil)
	if err := prepareOwnedTokenContext(ctx1, token); err != nil {
		t.Fatalf("prepare first context: %v", err)
	}
	release1, err := middleware.AcquireTokenRuntimeLimit(ctx1)
	if err != nil {
		t.Fatalf("acquire first runtime limit: %v", err)
	}
	release1()

	count, _, _, err := middleware.QueryTokenWindowStatus(token.Id, token.WindowSeconds)
	if err != nil {
		t.Fatalf("query window status: %v", err)
	}
	if count != 1 {
		t.Fatalf("unexpected window count after first acquire: %d", count)
	}

	ctx2, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx2.Request = httptest.NewRequest(http.MethodPost, "/api/token/test/102", nil)
	if err := prepareOwnedTokenContext(ctx2, token); err != nil {
		t.Fatalf("prepare second context: %v", err)
	}
	_, err = middleware.AcquireTokenRuntimeLimit(ctx2)
	if err == nil {
		t.Fatal("expected second acquire to hit window limit")
	}
	runtimeErr, ok := err.(*middleware.TokenRuntimeLimitError)
	if !ok {
		t.Fatalf("expected runtime limit error, got %T", err)
	}
	if runtimeErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("unexpected status code: %d", runtimeErr.StatusCode)
	}
	if !strings.Contains(runtimeErr.Error(), "60 秒内的请求数已达到上限 1") {
		t.Fatalf("unexpected error message: %v", runtimeErr)
	}
}
