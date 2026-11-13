package relay

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func mjRelayDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	sql.SetMaxOpenConns(1)
	oldDB, oldLogs := model.DB, model.LOG_DB
	oldRedis, oldBatch, oldLog, oldExport := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.DataExportEnabled
	oldThreshold := common.QuotaRemindThreshold
	oldSQLite, oldMySQL, oldPG := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.DataExportEnabled = false, false, false, false
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = true, false, false
	common.QuotaRemindThreshold = 0
	require.NoError(t, db.AutoMigrate(&model.Midjourney{}, &model.Enterprise{}, &model.EnterpriseMember{}, &model.EnterpriseReservation{}, &model.EnterpriseTokenBudget{}, &model.Token{}, &model.User{}, &model.Channel{}))
	t.Cleanup(func() {
		service.GetHttpClient().CloseIdleConnections()
		model.DB, model.LOG_DB = oldDB, oldLogs
		common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.DataExportEnabled = oldRedis, oldBatch, oldLog, oldExport
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = oldSQLite, oldMySQL, oldPG
		common.QuotaRemindThreshold = oldThreshold
		require.NoError(t, sql.Close())
	})
	service.InitHttpClient()
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "mj-fixture", Quota: 0}).Error)
	require.NoError(t, db.Create(&model.Enterprise{Id: 2, Status: model.EnterpriseStatusActive}).Error)
	require.NoError(t, db.Create(&model.EnterpriseMember{Id: 3, EnterpriseId: 2, UserId: 1, Role: model.EnterpriseRoleOwner, Status: model.EnterpriseMemberStatusActive, Available: 1000}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 4, UserId: 1, Key: "test-only-key", EnterpriseId: 2, MembershipId: 3, UnlimitedQuota: true, RemainQuota: 7}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 5, Status: common.ChannelStatusEnabled}).Error)
}

func mjRelayRequest(body, url string, quota int) (*gin.Context, *relaycommon.RelayInfo, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set("base_url", url)
	common.SetContextKey(c, constant.ContextKeyEnterpriseId, 2)
	common.SetContextKey(c, constant.ContextKeyEnterpriseMemberId, 3)
	common.SetContextKey(c, constant.ContextKeyChannelId, 5)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-upstream-key")
	id := common.GetUUID()
	c.Set(common.RequestIdKey, id)
	info := &relaycommon.RelayInfo{UserId: 1, TokenId: 4, TokenKey: "test-only-key", TokenUnlimited: true, EnterpriseId: 2, EnterpriseMemberId: 3, RequestId: id,
		OriginModelName: "mj_imagine", UsingGroup: "default", UserGroup: "default", RelayMode: relayconstant.RelayModeMidjourneyImagine,
		StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 5}, PriceData: types.PriceData{Quota: quota, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}}
	return c, info, recorder
}

func mjAssertRelayBalance(t *testing.T, available, reserved, consumed int64) {
	t.Helper()
	var member model.EnterpriseMember
	require.NoError(t, model.DB.First(&member, 3).Error)
	require.Equal(t, available, member.Available)
	require.Equal(t, reserved, member.Reserved)
	require.Equal(t, consumed, member.Consumed)
	var token model.Token
	require.NoError(t, model.DB.First(&token, 4).Error)
	require.EqualValues(t, consumed, token.UsedQuota)
	require.Equal(t, 7, token.RemainQuota)
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	require.Zero(t, user.Quota)
}

func TestEnterpriseMJRelayPreconsumeAndProtocol(t *testing.T) {
	cases := []struct {
		name, action, body string
		charge             bool
		cost               int64
	}{
		{"imagine", constant.MjActionImagine, `{"code":1,"result":"new-task"}`, true, 100},
		{"swap", constant.MjActionSwapFace, `{"code":1,"result":"new-task"}`, true, 100},
		{"queue", constant.MjActionVideo, `{"code":22,"result":"new-task"}`, true, 100},
		{"existing", constant.MjActionUpscale, `{"code":21,"result":"new-task","properties":{"status":"SUCCESS"}}`, true, 100},
		{"inpaint-dialog", constant.MjActionInPaint, `{"code":1,"result":"new-task"}`, false, 0},
		{"custom-zoom-dialog", constant.MjActionCustomZoom, `{"code":1,"result":"new-task"}`, false, 0},
		{"upload", constant.MjActionUpload, `{"code":1,"result":["https://example.invalid/a.png"]}`, true, 100},
		{"rejected", constant.MjActionBlend, `{"code":23,"description":"queue full"}`, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mjRelayDB(t)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var task model.Midjourney
				if err := model.DB.First(&task).Error; err != nil {
					t.Error(err)
				}
				if task.PollingKey != "test-upstream-key" || task.PollingBaseURL == "" {
					t.Error("missing submission credential snapshot")
				}
				if task.BillingRequestId == "" || task.EnterpriseBillingState != model.MJEnterpriseUnknown {
					t.Error("ownership was not persisted before upstream")
				}
				var member model.EnterpriseMember
				if err := model.DB.First(&member, 3).Error; err != nil {
					t.Error(err)
				}
				expected := int64(100)
				if !tc.charge {
					expected = 0
				}
				if member.Reserved != expected {
					t.Errorf("upstream called before reservation: %d", member.Reserved)
				}
				if r.Header.Get("mj-api-secret") != "test-upstream-key" {
					t.Error("wrong channel credential")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c, info, recorder := mjRelayRequest(`{"prompt":"fixture"}`, server.URL, 100)
			require.Nil(t, relayEnterpriseMJSubmit(c, info, tc.action, "fixture", tc.charge))
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, http.StatusOK, recorder.Code)
			if tc.name == "upload" {
				require.JSONEq(t, tc.body, recorder.Body.String())
			}
			mjAssertRelayBalance(t, 1000-tc.cost, 0, tc.cost)
			_, err := model.ReconcileEnterpriseMJ(10)
			require.NoError(t, err)
			mjAssertRelayBalance(t, 1000-tc.cost, 0, tc.cost)
		})
	}
}

func TestEnterpriseMJRelayUnknownOutcomeAndOwnerCap(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"invalid-body", "not json", 200}, {"empty-body", "", 200}, {"server-error", `{"code":4,"description":"unknown"}`, 502}, {"missing-result", `{"code":1}`, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			mjRelayDB(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(test.status); fmt.Fprint(w, test.body) }))
			defer server.Close()
			c, info, _ := mjRelayRequest(`{}`, server.URL, 100)
			require.NotNil(t, relayEnterpriseMJSubmit(c, info, constant.MjActionImagine, "", true))
			n, err := model.ReconcileEnterpriseMJ(10)
			require.NoError(t, err)
			require.Zero(t, n)
			mjAssertRelayBalance(t, 900, 100, 0)
		})
	}
	t.Run("owner-token-cap", func(t *testing.T) {
		mjRelayDB(t)
		require.NoError(t, model.DB.Create(&model.EnterpriseTokenBudget{TokenId: 4, EnterpriseId: 2, MemberId: 3, LimitQuota: 50}).Error)
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
		defer server.Close()
		c, info, _ := mjRelayRequest(`{}`, server.URL, 100)
		require.NotNil(t, relayEnterpriseMJSubmit(c, info, constant.MjActionImagine, "", true))
		require.Zero(t, calls.Load(), "owner and unlimited token still obey enterprise token budget")
		mjAssertRelayBalance(t, 1000, 0, 0)
	})
}

func TestEnterpriseMJRelayQueriesActionsImagesAndNotify(t *testing.T) {
	fetch := system_setting.GetFetchSetting()
	saved := *fetch
	t.Cleanup(func() { *fetch = saved })
	fetch.AllowPrivateIp = true
	fetch.AllowedPorts = []string{"1-65535"}

	mjRelayDB(t)
	var images atomic.Int32
	image := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { images.Add(1); fmt.Fprint(w, "image") }))
	defer image.Close()
	personal := &model.Midjourney{UserId: 1, MjId: "personal", ImageUrl: image.URL, Status: "SUCCESS", Progress: "100%", Buttons: `[{"customId":"MJ::JOB::upsample::1::personal"}]`}
	company := &model.Midjourney{UserId: 1, MjId: "company", EnterpriseId: 2, EnterpriseMemberId: 3, BillingSource: service.BillingSourceEnterprise, ImageUrl: image.URL, Status: "IN_PROGRESS", Progress: "50%", Buttons: `[{"customId":"MJ::JOB::upsample::1::company"}]`}
	require.NoError(t, personal.Insert())
	require.NoError(t, company.Insert())
	for _, enterprise := range []bool{false, true} {
		t.Run(fmt.Sprintf("enterprise-%v", enterprise), func(t *testing.T) {
			forbidden := "company"
			own := "personal"
			if enterprise {
				forbidden, own = own, forbidden
			}
			context := func(body string) (*gin.Context, *relaycommon.RelayInfo, *httptest.ResponseRecorder) {
				c, info, w := mjRelayRequest(body, "", 100)
				if !enterprise {
					common.SetContextKey(c, constant.ContextKeyEnterpriseId, 0)
					common.SetContextKey(c, constant.ContextKeyEnterpriseMemberId, 0)
					info.EnterpriseId, info.EnterpriseMemberId = 0, 0
				}
				return c, info, w
			}
			c, _, _ := context(`{}`)
			c.Params = gin.Params{{Key: "id", Value: forbidden}}
			require.NotNil(t, RelayMidjourneyTask(c, relayconstant.RelayModeMidjourneyTaskFetch))
			require.NotNil(t, RelayMidjourneyTaskImageSeed(c))
			c, _, w := context(`{"ids":["personal","company"]}`)
			require.Nil(t, RelayMidjourneyTask(c, relayconstant.RelayModeMidjourneyTaskFetchByCondition))
			var tasks []dto.MidjourneyDto
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &tasks))
			require.Len(t, tasks, 1)
			require.Equal(t, own, tasks[0].MjId)
			c, _, w = context(`{}`)
			c.Params = gin.Params{{Key: "id", Value: forbidden}}
			RelayMidjourneyImage(c)
			require.Equal(t, 400, w.Code)
			c, _, w = context(`{}`)
			c.Params = gin.Params{{Key: "id", Value: own}}
			RelayMidjourneyImage(c)
			require.Equal(t, 200, w.Code)
			for _, mode := range []int{relayconstant.RelayModeMidjourneyChange, relayconstant.RelayModeMidjourneyModal, relayconstant.RelayModeMidjourneyVideo, relayconstant.RelayModeMidjourneyAction, relayconstant.RelayModeMidjourneySimpleChange} {
				body := fmt.Sprintf(`{"taskId":%q,"action":"UPSCALE","index":1,"customId":%q,"content":%q}`, forbidden, "MJ::JOB::upsample::1::"+forbidden, forbidden+" U1")
				c, info, _ := context(body)
				info.RelayMode = mode
				require.NotNil(t, RelayMidjourneySubmit(c, info), "cross-payer action must fail before HTTP")
			}
			c, info, _ := context(fmt.Sprintf(`{"customId":%q}`, "MJ::JOB::upsample::1::"+forbidden))
			info.RelayMode = relayconstant.RelayModeMidjourneyAction
			require.NotNil(t, RelayMidjourneySubmit(c, info))
			c, _, _ = context(fmt.Sprintf(`{"id":%q,"status":"FAILURE","progress":"100%%"}`, forbidden))
			require.NotNil(t, RelayMidjourneyNotify(c))
		})
	}
	// Even a valid enterprise token cannot fabricate an upstream failure/refund.
	c, _, _ := mjRelayRequest(`{"id":"company","status":"FAILURE","progress":"100%","imageUrl":"http://127.0.0.1"}`, "", 100)
	require.Nil(t, RelayMidjourneyNotify(c))
	var unchanged model.Midjourney
	require.NoError(t, model.DB.First(&unchanged, company.Id).Error)
	require.Equal(t, "IN_PROGRESS", unchanged.Status)
	require.Equal(t, image.URL, unchanged.ImageUrl)
	require.EqualValues(t, 2, images.Load())
	// Keep the test's HTTP request body explicitly closed, too.
	if c.Request.Body != nil {
		_, _ = io.Copy(io.Discard, c.Request.Body)
		require.NoError(t, c.Request.Body.Close())
	}
}

func TestEnterpriseMJRelayAcceptedSettlementFailureRecovers(t *testing.T) {
	mjRelayDB(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"code":1,"result":"accepted-before-crash"}`)
	}))
	defer server.Close()
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("mj-test-settlement-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "enterprise_reservations" {
			if updates, ok := tx.Statement.Dest.(map[string]interface{}); ok && updates["status"] == model.EnterpriseReservationSettled {
				tx.AddError(fmt.Errorf("simulated settlement outage"))
			}
		}
	}))
	c, info, _ := mjRelayRequest(`{}`, server.URL, 100)
	result := relayEnterpriseMJSubmit(c, info, constant.MjActionImagine, "", true)
	require.NotNil(t, result)
	require.Equal(t, "midjourney_settlement_pending", result.Description)
	require.NoError(t, model.DB.Callback().Update().Remove("mj-test-settlement-failure"))
	var task model.Midjourney
	require.NoError(t, model.DB.First(&task).Error)
	require.Equal(t, "accepted-before-crash", task.MjId)
	require.Equal(t, model.MJEnterpriseAccepted, task.EnterpriseBillingState)
	require.Equal(t, 100, task.EnterpriseInitialQuota)
	mjAssertRelayBalance(t, 900, 100, 0)
	n, err := model.ReconcileEnterpriseMJ(10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	mjAssertRelayBalance(t, 900, 0, 100)
	require.EqualValues(t, 1, calls.Load(), "recovery must not resubmit the upstream job")
}

func TestEnterpriseMJOriginResolutionKeepsPersonalActions(t *testing.T) {
	mjRelayDB(t)
	buttons := `[{"customId":"MJ::JOB::upsample::1::fixture"},{"customId":"MJ::CustomZoom::fixture"}]`
	personal := &model.Midjourney{UserId: 1, MjId: "fixture", Buttons: buttons}
	require.NoError(t, personal.Insert())
	for _, custom := range []string{"MJ::JOB::upsample::1::fixture", "MJ::CustomZoom::fixture"} {
		c, _, _ := mjRelayRequest(`{}`, "", 0)
		common.SetContextKey(c, constant.ContextKeyEnterpriseId, 0)
		common.SetContextKey(c, constant.ContextKeyEnterpriseMemberId, 0)
		request := &dto.MidjourneyRequest{CustomId: custom}
		task, err := ResolveMidjourneyOriginTask(c, relayconstant.RelayModeMidjourneyAction, request)
		require.Nil(t, err)
		require.NotNil(t, task)
		require.Equal(t, personal.Id, task.Id)
		require.Equal(t, "fixture", request.TaskId)
	}
	for _, custom := range []string{"", "MJ", "MJ::JOB", "MJ::JOB::upsample"} {
		c, _, _ := mjRelayRequest(`{}`, "", 0)
		_, err := ResolveMidjourneyOriginTask(c, relayconstant.RelayModeMidjourneyAction, &dto.MidjourneyRequest{CustomId: custom})
		require.NotNil(t, err)
	}
}

func TestEnterpriseMJOriginChannelRetainsSelectedKey(t *testing.T) {
	c, _, _ := mjRelayRequest(`{}`, "", 0)
	channel := &model.Channel{Id: 5, Key: "first-test-key\nsecond-test-key", Type: constant.ChannelTypeMidjourney}
	require.Nil(t, prepareMJOriginChannel(c, channel, "mj_imagine"))
	require.Equal(t, "test-upstream-key", common.GetContextKeyString(c, constant.ContextKeyChannelKey), "fixed-channel admission already selected a single key")
}

func TestEnterpriseMJRejectsAmbiguousTaskReferences(t *testing.T) {
	for _, body := range []string{
		`{"taskId":"company","TaskId":"personal"}`,
		`{"customId":"company-button","CUSTOMID":"personal-button"}`,
		`{"content":"company U1","Content":"personal U1"}`,
	} {
		c, _, _ := mjRelayRequest(body, "", 0)
		require.False(t, unambiguousMJReferences(c))
	}
	c, _, _ := mjRelayRequest(`{"taskId":"personal","customId":"owned-button","index":1}`, "", 0)
	require.True(t, unambiguousMJReferences(c))
	for _, content := range []string{"fixture ", "fixture u", "fixture v", "fixture", " U1"} {
		require.Nil(t, parseMJSimpleChange(content))
	}
	require.Equal(t, "fixture", parseMJSimpleChange("fixture U1").TaskId)
	require.Equal(t, "REROLL", parseMJSimpleChange("fixture R").Action)
}

func TestEnterpriseMJPollingUsesPersistedKeyAndAllLocalRows(t *testing.T) {
	mjRelayDB(t)
	var wrongEndpoint atomic.Int32
	wrong := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { wrongEndpoint.Add(1) }))
	defer wrong.Close()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/mj/task/list-by-condition" {
			t.Error("wrong poll path")
		}
		var payload struct {
			IDs []string `json:"ids"`
		}
		data, _ := io.ReadAll(r.Body)
		if err := common.Unmarshal(data, &payload); err != nil {
			t.Error(err)
		}
		if len(payload.IDs) != 1 || payload.IDs[0] != "same-id" {
			t.Error("cross-batch task IDs")
		}
		status := "SUCCESS"
		switch r.Header.Get("mj-api-secret") {
		case "snapshot-key-a":
		case "snapshot-key-b":
			status = "FAILURE"
		case "personal-key":
			status = "IN_PROGRESS"
		default:
			t.Error("poll did not use the submitted single key")
			w.WriteHeader(401)
			return
		}
		fmt.Fprintf(w, `[{"id":"same-id","status":%q,"progress":"100%%"},{"id":"unrequested","status":"FAILURE"}]`, status)
	}))
	defer server.Close()
	channel := &model.Channel{Id: 5, Key: "rotated-one\nrotated-two", BaseURL: &wrong.URL, Status: common.ChannelStatusEnabled}
	records := []*model.Midjourney{
		{Id: 101, UserId: 1, EnterpriseId: 2, EnterpriseMemberId: 3, BillingSource: model.EnterpriseQuotaSource, ChannelId: 5, MjId: "same-id", PollingKey: "snapshot-key-a", PollingBaseURL: server.URL},
		{Id: 102, UserId: 1, EnterpriseId: 2, EnterpriseMemberId: 3, BillingSource: model.EnterpriseQuotaSource, ChannelId: 5, MjId: "same-id", PollingKey: "snapshot-key-a", PollingBaseURL: server.URL},
		{Id: 103, UserId: 1, EnterpriseId: 2, EnterpriseMemberId: 3, BillingSource: model.EnterpriseQuotaSource, ChannelId: 5, MjId: "same-id", PollingKey: "snapshot-key-b", PollingBaseURL: server.URL},
		{Id: 104, UserId: 99, EnterpriseId: 2, EnterpriseMemberId: 98, BillingSource: model.EnterpriseQuotaSource, ChannelId: 5, MjId: "same-id", PollingKey: "snapshot-key-a", PollingBaseURL: server.URL},
		{Id: 105, UserId: 1, ChannelId: 5, MjId: "same-id", PollingKey: "personal-key", PollingBaseURL: server.URL},
	}
	batches, err := model.BuildMJPollingBatches(records)
	require.NoError(t, err)
	require.Len(t, batches, 4)
	results := map[int]string{}
	for _, batch := range batches {
		items, attempted, err := FetchMidjourneyPollingBatch(context.Background(), channel, batch)
		require.NoError(t, err)
		require.True(t, attempted)
		require.Len(t, items, 1)
		for _, item := range items {
			for _, task := range batch.TasksByID[item.MjId] {
				results[task.Id] = item.Status
			}
		}
	}
	require.Equal(t, map[int]string{101: "SUCCESS", 102: "SUCCESS", 103: "FAILURE", 104: "SUCCESS", 105: "IN_PROGRESS"}, results)
	require.EqualValues(t, 4, requests.Load())
	require.Zero(t, wrongEndpoint.Load())
}

func TestEnterpriseMJPollingDoesNotLeakSnapshotOnRedirectOrError(t *testing.T) {
	mjRelayDB(t)
	var received atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1) }))
	defer target.Close()
	for _, mode := range []string{"redirect", "malformed", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "redirect":
					http.Redirect(w, r, target.URL, http.StatusFound)
				case "malformed":
					fmt.Fprint(w, "provider echoed snapshot-key-secret")
				case "duplicate":
					fmt.Fprint(w, `[{"id":"same-id","status":"SUCCESS"},{"id":"same-id","status":"FAILURE"}]`)
				}
			}))
			defer server.Close()
			batches, err := model.BuildMJPollingBatches([]*model.Midjourney{{UserId: 1, EnterpriseId: 2, EnterpriseMemberId: 3, BillingSource: model.EnterpriseQuotaSource, ChannelId: 5, MjId: "same-id", PollingKey: "snapshot-key-secret", PollingBaseURL: server.URL}})
			require.NoError(t, err)
			items, attempted, err := FetchMidjourneyPollingBatch(context.Background(), &model.Channel{Id: 5}, batches[0])
			require.Error(t, err)
			require.True(t, attempted)
			require.Empty(t, items)
			require.NotContains(t, err.Error(), "snapshot-key-secret")
		})
	}
	require.Zero(t, received.Load(), "stored credential must not follow redirect")
}

func TestEnterpriseMJImageAppliesFetchPolicyBeforeRequest(t *testing.T) {
	mjRelayDB(t)
	fetch := system_setting.GetFetchSetting()
	saved := *fetch
	t.Cleanup(func() { *fetch = saved })
	fetch.EnableSSRFProtection = true
	fetch.AllowPrivateIp = false
	fetch.AllowedPorts = []string{"1-65535"}
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer server.Close()
	task := &model.Midjourney{UserId: 1, MjId: "blocked-image", ImageUrl: server.URL, EnterpriseId: 2, EnterpriseMemberId: 3, BillingSource: service.BillingSourceEnterprise}
	require.NoError(t, task.Insert())
	c, _, w := mjRelayRequest("{}", "", 0)
	c.Params = gin.Params{{Key: "id", Value: task.MjId}}
	RelayMidjourneyImage(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "image_url_not_allowed")
	require.Zero(t, hits.Load())
}
