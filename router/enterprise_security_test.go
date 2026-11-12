package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type enterpriseAPI struct {
	t      *testing.T
	engine *gin.Engine
}

type enterpriseAPIResponse struct {
	Code    int
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func (r enterpriseAPIResponse) object() map[string]any {
	out, _ := r.Data.(map[string]any)
	return out
}

func setupEnterpriseAPI(t *testing.T, enabled bool) *enterpriseAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldSQLite, oldMySQL, oldPG, oldRedis := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL, common.RedisEnabled
	oldGlobal, oldCritical, oldEnabled := common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable, common.EnterpriseEnabled
	// 搜索限流按账号编号在进程内计数，而每个测试的账号编号都从 1 开始；放宽后重复运行才不会互相限流。
	oldSearchLimit := common.SearchRateLimitNum
	common.SearchRateLimitNum = 1 << 20
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	model.DB, model.LOG_DB = db, db
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL, common.RedisEnabled = true, false, false, false
	common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable, common.EnterpriseEnabled = false, false, enabled
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL, common.RedisEnabled = oldSQLite, oldMySQL, oldPG, oldRedis
		common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable, common.EnterpriseEnabled = oldGlobal, oldCritical, oldEnabled
		common.SearchRateLimitNum = oldSearchLimit
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	require.NoError(t, model.MigrateEnterpriseTables(db))

	engine := gin.New()
	engine.Use(sessions.Sessions("test", cookie.NewStore([]byte("local-test-session-key"))))
	// 测试用登录：请求头指定账号与平台角色，等价于已建立的会话。
	engine.Use(func(c *gin.Context) {
		if raw := c.GetHeader("X-Test-User"); raw != "" {
			id, _ := strconv.Atoi(raw)
			role, _ := strconv.Atoi(c.GetHeader("X-Test-Role"))
			session := sessions.Default(c)
			session.Set("username", "local-test-user-"+raw)
			session.Set("role", role)
			session.Set("id", id)
			session.Set("status", common.UserStatusEnabled)
		}
		c.Next()
	})
	SetApiRouter(engine)
	return &enterpriseAPI{t: t, engine: engine}
}

func (a *enterpriseAPI) user(name string) *model.User {
	a.t.Helper()
	user := &model.User{Username: name, AffCode: name, Email: name + "@example.com", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(a.t, model.DB.Create(user).Error)
	return user
}

// call 以指定账号发起请求；userID 为 0 表示未登录。
func (a *enterpriseAPI) call(userID, role int, method, path string, body any) enterpriseAPIResponse {
	a.t.Helper()
	payload := []byte("{}")
	if body != nil {
		var err error
		payload, err = common.Marshal(body)
		require.NoError(a.t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if userID > 0 {
		req.Header.Set("X-Test-User", strconv.Itoa(userID))
		req.Header.Set("X-Test-Role", strconv.Itoa(role))
		req.Header.Set("New-Api-User", strconv.Itoa(userID))
	}
	w := httptest.NewRecorder()
	a.engine.ServeHTTP(w, req)
	out := enterpriseAPIResponse{Code: w.Code}
	if w.Body.Len() > 0 {
		require.NoError(a.t, common.Unmarshal(w.Body.Bytes(), &out), "%s %s: %s", method, path, w.Body.String())
	}
	return out
}

func (a *enterpriseAPI) ok(userID, role int, method, path string, body any) enterpriseAPIResponse {
	a.t.Helper()
	resp := a.call(userID, role, method, path, body)
	require.True(a.t, resp.Success, "%s %s: %s", method, path, resp.Message)
	return resp
}

func (a *enterpriseAPI) denied(userID, role int, method, path string, body any, want error) {
	a.t.Helper()
	resp := a.call(userID, role, method, path, body)
	require.False(a.t, resp.Success, "%s %s must be rejected", method, path)
	if want != nil {
		require.Equal(a.t, want.Error(), resp.Message, "%s %s", method, path)
	}
}

// enterpriseRoutes 枚举已注册的全部企业接口，路径参数填入给定的企业 ID。
func (a *enterpriseAPI) enterpriseRoutes(enterpriseID int) [][2]string {
	var routes [][2]string
	for _, route := range a.engine.Routes() {
		if !strings.HasPrefix(route.Path, "/api/enterprise") {
			continue
		}
		path := strings.NewReplacer(":id", strconv.Itoa(enterpriseID), ":member_id", "1", ":invitation_id", "1",
			":request_id", "1", ":reservation_id", "1").Replace(route.Path)
		routes = append(routes, [2]string{route.Method, path})
	}
	require.GreaterOrEqual(a.t, len(routes), 30, "enterprise routes must be registered")
	return routes
}

func (a *enterpriseAPI) org(root *model.User, name string, owner *model.User) int {
	a.t.Helper()
	resp := a.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises",
		map[string]any{"name": name, "owner": owner.Username, "billing_group": "default"})
	return int(resp.object()["id"].(float64))
}

func (a *enterpriseAPI) join(enterpriseID int, inviter, invitee *model.User, role string, initial int64) int {
	a.t.Helper()
	base := "/api/enterprises/" + strconv.Itoa(enterpriseID)
	resp := a.ok(inviter.Id, common.RoleCommonUser, http.MethodPost, base+"/invitations",
		map[string]any{"email": invitee.Email, "role": role, "initial_quota": initial})
	link := resp.object()["link"].(string)
	token := link[strings.Index(link, "token=")+len("token="):]
	preview := a.ok(0, 0, http.MethodGet, "/api/enterprise/invitation?token="+token, nil)
	require.Equal(a.t, invitee.Email, preview.object()["email"])
	a.ok(invitee.Id, common.RoleCommonUser, http.MethodPost, "/api/enterprise/invitation/accept", map[string]any{"token": token})
	member, err := model.GetActiveEnterpriseMemberByUser(invitee.Id)
	require.NoError(a.t, err)
	require.NotNil(a.t, member)
	return member.Id
}

func TestEnterpriseRoutesClosedWhenDisabled(t *testing.T) {
	api := setupEnterpriseAPI(t, false)
	root := api.user("root")
	for _, route := range api.enterpriseRoutes(1) {
		// 开关关闭时，连超级管理员也不能使用任何企业接口。
		api.denied(root.Id, common.RoleRootUser, route[0], route[1], nil, model.ErrEnterpriseDisabled)
		api.denied(0, 0, route[0], route[1], nil, model.ErrEnterpriseDisabled)
	}
}

func TestEnterpriseRoutesRequireLogin(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	for _, route := range api.enterpriseRoutes(1) {
		if route[0] == http.MethodGet && strings.HasPrefix(route[1], "/api/enterprise/invitation") {
			// 邀请预览是唯一的公开接口，无效令牌只得到统一的失效提示。
			api.denied(0, 0, route[0], route[1]+"?token=bad", nil, model.ErrEnterpriseInvitation)
			continue
		}
		resp := api.call(0, 0, route[0], route[1], nil)
		require.False(t, resp.Success, "%s %s must require login", route[0], route[1])
		require.Equal(t, http.StatusUnauthorized, resp.Code, "%s %s", route[0], route[1])
	}
}

func TestEnterpriseRoutesIsolateTenantsAndRoles(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	root, platformAdmin := api.user("root"), api.user("platform-admin")
	ownerA, ownerB, staffA, outsider := api.user("owner-a"), api.user("owner-b"), api.user("staff-a"), api.user("outsider")
	entA, entB := api.org(root, "甲公司", ownerA), api.org(root, "乙公司", ownerB)
	api.join(entA, ownerA, staffA, model.EnterpriseRoleMember, 0)
	const user = common.RoleCommonUser

	for _, route := range api.enterpriseRoutes(entB) {
		switch {
		case strings.HasPrefix(route[1], "/api/enterprise/admin"):
			// 平台接口：企业所有者、普通成员、无关账号一律被拒。
			for _, who := range []*model.User{ownerA, staffA, outsider} {
				api.denied(who.Id, user, route[0], route[1], nil, nil)
			}
		case strings.HasPrefix(route[1], "/api/enterprises/"):
			// 另一家企业的后台：与访问不存在的企业得到同一个响应，不泄露企业是否存在。
			missing := strings.Replace(route[1], "/api/enterprises/"+strconv.Itoa(entB), "/api/enterprises/999999", 1)
			for _, who := range []*model.User{ownerA, staffA, outsider} {
				api.denied(who.Id, user, route[0], route[1], nil, model.ErrEnterpriseNotFound)
				api.denied(who.Id, user, route[0], missing, nil, model.ErrEnterpriseNotFound)
			}
			// 平台管理员不是企业成员，也不能走企业后台接口。
			api.denied(platformAdmin.Id, common.RoleAdminUser, route[0], route[1], nil, model.ErrEnterpriseNotFound)
		}
	}
	// 本企业的普通成员不能进入企业后台。
	for _, route := range api.enterpriseRoutes(entA) {
		if strings.HasPrefix(route[1], "/api/enterprises/") {
			api.denied(staffA.Id, user, route[0], route[1], nil, model.ErrEnterpriseForbidden)
		}
	}
	// 动钱的平台操作要求超级管理员。
	credit := "/api/enterprise/admin/enterprises/" + strconv.Itoa(entA) + "/credit"
	api.denied(platformAdmin.Id, common.RoleAdminUser, http.MethodPost, credit, map[string]any{"amount": 100, "voucher": "V1"}, nil)
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, credit, map[string]any{"amount": 1000, "voucher": "V1"})
	api.denied(root.Id, common.RoleRootUser, http.MethodPost, credit, map[string]any{"amount": 1000, "voucher": "V1"}, model.ErrEnterpriseDuplicateVoucher)

	// 路径里是自己的企业、对象却属于别的企业：按不存在处理。
	memberB, err := model.GetActiveEnterpriseMemberByUser(ownerB.Id)
	require.NoError(t, err)
	baseA := "/api/enterprises/" + strconv.Itoa(entA)
	foreign := baseA + "/members/" + strconv.Itoa(memberB.Id)
	api.denied(ownerA.Id, user, http.MethodGet, foreign, nil, model.ErrEnterpriseNotFound)
	api.denied(ownerA.Id, user, http.MethodPost, foreign+"/allocate", map[string]any{"amount": 10}, model.ErrEnterpriseNotFound)
	api.denied(ownerA.Id, user, http.MethodPost, foreign+"/disable", nil, model.ErrEnterpriseNotFound)
	api.denied(ownerA.Id, user, http.MethodPost, foreign+"/transfer-ownership", nil, model.ErrEnterpriseNotFound)
	balance, err := model.GetEnterpriseBalance(entB)
	require.NoError(t, err)
	require.Zero(t, balance.MemberAvailable)
}

func TestEnterpriseQuotaFlowOverAPI(t *testing.T) {
	api := setupEnterpriseAPI(t, true)
	root, owner, admin, staff := api.user("root"), api.user("owner"), api.user("admin"), api.user("staff")
	ent := api.org(root, "甲公司", owner)
	base := "/api/enterprises/" + strconv.Itoa(ent)
	const user = common.RoleCommonUser
	api.ok(root.Id, common.RoleRootUser, http.MethodPost, "/api/enterprise/admin/enterprises/"+strconv.Itoa(ent)+"/credit",
		map[string]any{"amount": 1000, "voucher": "BANK-001"})

	// 邀请时填写的初始额度在接受时到账；管理员邀请管理员被拒。
	adminMember := api.join(ent, owner, admin, model.EnterpriseRoleAdmin, 0)
	api.denied(admin.Id, user, http.MethodPost, base+"/invitations", map[string]any{"email": staff.Email, "role": model.EnterpriseRoleAdmin}, model.ErrEnterpriseForbidden)
	staffMember := api.join(ent, admin, staff, model.EnterpriseRoleMember, 200)
	self := api.ok(staff.Id, user, http.MethodGet, "/api/enterprise/self", nil).object()
	require.EqualValues(t, 200, self["member"].(map[string]any)["available"])
	require.Equal(t, false, self["manager"])
	require.NotContains(t, self, "unallocated")

	// 管理员给普通成员分配；不能给自己分配，由所有者操作。
	api.ok(admin.Id, user, http.MethodPost, base+"/members/"+strconv.Itoa(staffMember)+"/allocate", map[string]any{"amount": 100})
	api.denied(admin.Id, user, http.MethodPost, base+"/members/"+strconv.Itoa(adminMember)+"/allocate", map[string]any{"amount": 100}, model.ErrEnterpriseSelfOperation)
	api.ok(owner.Id, user, http.MethodPost, base+"/members/"+strconv.Itoa(adminMember)+"/allocate", map[string]any{"amount": 100})
	api.denied(owner.Id, user, http.MethodPost, base+"/members/"+strconv.Itoa(staffMember)+"/allocate", map[string]any{"amount": 5000}, model.ErrEnterpriseInsufficientUnallocated)

	// 申请：同时只能一个待审批；批准金额可改小；已处理的不能再批。
	created := api.ok(staff.Id, user, http.MethodPost, "/api/enterprise/self/requests", map[string]any{"amount": 300, "reason": "项目联调"}).object()
	requestID := strconv.Itoa(int(created["id"].(float64)))
	api.denied(staff.Id, user, http.MethodPost, "/api/enterprise/self/requests", map[string]any{"amount": 50}, model.ErrEnterpriseRequestPending)
	require.EqualValues(t, 1, api.ok(owner.Id, user, http.MethodGet, "/api/enterprise/self", nil).object()["pending_requests"])
	api.ok(admin.Id, user, http.MethodPost, base+"/quota-requests/"+requestID+"/approve", map[string]any{"amount": 250})
	api.denied(owner.Id, user, http.MethodPost, base+"/quota-requests/"+requestID+"/reject", nil, model.ErrEnterpriseRequestState)

	// 管理员自己的申请不能自批。
	own := api.ok(admin.Id, user, http.MethodPost, "/api/enterprise/self/requests", map[string]any{"amount": 40}).object()
	ownID := strconv.Itoa(int(own["id"].(float64)))
	api.denied(admin.Id, user, http.MethodPost, base+"/quota-requests/"+ownID+"/approve", nil, model.ErrEnterpriseSelfOperation)
	api.ok(owner.Id, user, http.MethodPost, base+"/quota-requests/"+ownID+"/approve", nil)

	// 设置只有所有者能改；所有者不能直接退出；停用后成员接口不再可用。
	settings := map[string]any{"name": "甲公司", "timezone": "Asia/Shanghai", "model_mode": model.EnterpriseModelModeInherit}
	api.denied(admin.Id, user, http.MethodPut, base+"/settings", settings, model.ErrEnterpriseForbidden)
	api.ok(owner.Id, user, http.MethodPut, base+"/settings", settings)
	api.denied(owner.Id, user, http.MethodPost, "/api/enterprise/self/leave", nil, model.ErrEnterpriseOwnerRequired)
	api.ok(admin.Id, user, http.MethodPost, base+"/members/"+strconv.Itoa(staffMember)+"/disable", map[string]any{"reason": "离职"})
	require.Nil(t, api.ok(staff.Id, user, http.MethodGet, "/api/enterprise/self", nil).Data)
	api.denied(staff.Id, user, http.MethodPost, "/api/enterprise/self/requests", map[string]any{"amount": 10}, model.ErrEnterpriseNotFound)

	// 全程守恒：入账 1000 = 未分配 + 成员可用（停用成员的 550 已并回未分配）。
	overview := api.ok(owner.Id, user, http.MethodGet, base+"/overview", nil).object()
	balance := overview["balance"].(map[string]any)
	require.Equal(t, true, balance["balanced"])
	require.EqualValues(t, 1000, balance["net_credited"])
	require.EqualValues(t, 860, balance["unallocated"])
	require.EqualValues(t, 140, balance["member_available"])
}
