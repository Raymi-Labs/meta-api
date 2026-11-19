package router

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ticketAPI struct {
	t      *testing.T
	engine *gin.Engine
}

type ticketAPIResponse struct {
	Code    int
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func (r ticketAPIResponse) object() map[string]any {
	out, _ := r.Data.(map[string]any)
	return out
}

func setupTicketAPI(t *testing.T) *ticketAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldSQLite, oldMySQL, oldPG, oldRedis := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL, common.RedisEnabled
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()

	// 确保附件目录存在
	os.MkdirAll(controller.TicketAttachmentStorageDir, 0755)

	model.DB, model.LOG_DB = db, db
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL, common.RedisEnabled = true, false, false, false

	// 重置内存限流器以避免测试间相互影响
	common.ResetTicketRateLimiter()

	t.Cleanup(func() {
		// Drain notifications before replacing shared database pointers.
		service.WaitTicketNotifications()
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL, common.RedisEnabled = oldSQLite, oldMySQL, oldPG, oldRedis
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		_ = sqlDB.Close()
		// 清理附件目录（测试结束时）
		_ = os.RemoveAll(controller.TicketAttachmentStorageDir)
	})

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Option{}))
	require.NoError(t, db.AutoMigrate(&model.Ticket{}, &model.TicketMessage{}, &model.TicketAttachment{}))

	engine := gin.New()
	engine.Use(sessions.Sessions("test", cookie.NewStore([]byte("local-test-session-key"))))
	// 测试用登录：请求头指定账号与角色
	engine.Use(func(c *gin.Context) {
		if raw := c.GetHeader("X-Test-User"); raw != "" {
			id, _ := strconv.Atoi(raw)
			role, _ := strconv.Atoi(c.GetHeader("X-Test-Role"))
			session := sessions.Default(c)
			session.Set("username", "test-user-"+raw)
			session.Set("role", role)
			session.Set("id", id)
			session.Set("status", common.UserStatusEnabled)
		}
		// 申诉会话
		if raw := c.GetHeader("X-Test-Appeal-User"); raw != "" {
			appealUserID, _ := strconv.Atoi(raw)
			session := sessions.Default(c)
			session.Set("appeal_user_id", appealUserID)
			session.Set("appeal_expires_at", time.Now().Add(2*time.Hour).Unix())
		}
		c.Next()
	})
	SetApiRouter(engine)
	return &ticketAPI{t: t, engine: engine}
}

func (a *ticketAPI) user(name string, status int) *model.User {
	a.t.Helper()
	user := &model.User{
		Username: name,
		AffCode:  name,
		Email:    name + "@example.com",
		Status:   status,
		Group:    "default",
		Role:     common.RoleCommonUser,
	}
	if strings.Contains(name, "admin") {
		user.Role = common.RoleAdminUser
	}
	require.NoError(a.t, model.DB.Create(user).Error)
	return user
}

// uniqueUser 创建带时间戳的唯一用户，避免限流影响
func (a *ticketAPI) uniqueUser(prefix string, status int) *model.User {
	name := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	return a.user(name, status)
}

func (a *ticketAPI) call(userID, role int, method, path string, body any) ticketAPIResponse {
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
	out := ticketAPIResponse{Code: w.Code}
	if w.Body.Len() > 0 {
		require.NoError(a.t, common.Unmarshal(w.Body.Bytes(), &out), "%s %s: %s", method, path, w.Body.String())
	}
	return out
}

func (a *ticketAPI) callAppeal(appealUserID int, method, path string, body any) ticketAPIResponse {
	a.t.Helper()
	payload := []byte("{}")
	if body != nil {
		var err error
		payload, err = common.Marshal(body)
		require.NoError(a.t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Appeal-User", strconv.Itoa(appealUserID))
	req.Header.Set("New-Api-User", strconv.Itoa(appealUserID))
	w := httptest.NewRecorder()
	a.engine.ServeHTTP(w, req)
	out := ticketAPIResponse{Code: w.Code}
	if w.Body.Len() > 0 {
		require.NoError(a.t, common.Unmarshal(w.Body.Bytes(), &out), "%s %s: %s", method, path, w.Body.String())
	}
	return out
}

func (a *ticketAPI) ok(userID, role int, method, path string, body any) ticketAPIResponse {
	a.t.Helper()
	resp := a.call(userID, role, method, path, body)
	require.True(a.t, resp.Success, "%s %s: %s", method, path, resp.Message)
	return resp
}

func (a *ticketAPI) denied(userID, role int, method, path string, body any) {
	a.t.Helper()
	resp := a.call(userID, role, method, path, body)
	require.False(a.t, resp.Success, "%s %s must be rejected", method, path)
}

// TestTicketCreateAndList 测试工单创建和列表
func TestTicketCreateAndList(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)

	// 创建工单
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject":  "测试工单",
		"category": model.TicketCategoryBilling,
		"body":     "这是一个测试工单",
	})
	ticketID := int(resp.object()["id"].(float64))
	require.Greater(t, ticketID, 0)

	// 获取工单列表
	listResp := api.ok(user.Id, common.RoleCommonUser, http.MethodGet, "/api/ticket", nil)
	tickets := listResp.object()["tickets"].([]any)
	require.Len(t, tickets, 1)
	ticket := tickets[0].(map[string]any)
	require.Equal(t, "测试工单", ticket["subject"])
	require.Equal(t, model.TicketCategoryBilling, ticket["category"])
	require.Equal(t, model.TicketStatusOpen, ticket["status"])
}

// TestTicketDetail 测试工单详情查询
func TestTicketDetail(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)

	// 创建工单
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject":  "详情测试",
		"category": model.TicketCategoryAPI,
		"body":     "工单内容",
	})
	ticketID := int(resp.object()["id"].(float64))

	// 获取详情
	detailResp := api.ok(user.Id, common.RoleCommonUser, http.MethodGet, fmt.Sprintf("/api/ticket/%d", ticketID), nil)
	data := detailResp.object()
	ticket := data["ticket"].(map[string]any)
	messages := data["messages"].([]any)

	require.Equal(t, "详情测试", ticket["subject"])
	require.Len(t, messages, 1) // 首条消息
	require.Equal(t, "工单内容", messages[0].(map[string]any)["body"])
}

// TestTicketReply 测试工单回复
func TestTicketReply(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)
	admin := api.uniqueUser("admin1", common.UserStatusEnabled)

	// 创建工单
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject":  "回复测试",
		"category": model.TicketCategoryBilling,
		"body":     "初始消息",
	})
	ticketID := int(resp.object()["id"].(float64))

	// 用户回复
	api.ok(user.Id, common.RoleCommonUser, http.MethodPost, fmt.Sprintf("/api/ticket/%d/reply", ticketID), map[string]any{
		"body": "用户回复1",
	})

	// 管理员回复
	api.ok(admin.Id, common.RoleAdminUser, http.MethodPost, fmt.Sprintf("/api/admin/ticket/%d/reply", ticketID), map[string]any{
		"body":        "管理员回复1",
		"is_internal": false,
	})

	// 查看消息（管理员回复会产生状态转换的系统消息，所以是4条）
	detailResp := api.ok(user.Id, common.RoleCommonUser, http.MethodGet, fmt.Sprintf("/api/ticket/%d", ticketID), nil)
	messages := detailResp.object()["messages"].([]any)
	require.GreaterOrEqual(t, len(messages), 3) // 至少3条：初始+用户回复+管理员回复
}

// TestTicketStatusTransition 测试工单状态转换
func TestTicketStatusTransition(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)
	admin := api.uniqueUser("admin1", common.UserStatusEnabled)

	// 创建工单
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject":  "状态测试",
		"category": model.TicketCategoryAPI,
		"body":     "测试状态转换",
	})
	ticketID := int(resp.object()["id"].(float64))

	// 管理员回复（应转为waiting_user）
	api.ok(admin.Id, common.RoleAdminUser, http.MethodPost, fmt.Sprintf("/api/admin/ticket/%d/reply", ticketID), map[string]any{
		"body": "请提供更多信息",
	})

	// 检查状态
	detailResp := api.ok(admin.Id, common.RoleAdminUser, http.MethodGet, fmt.Sprintf("/api/admin/ticket/%d", ticketID), nil)
	ticket := detailResp.object()["ticket"].(map[string]any)
	require.Equal(t, model.TicketStatusWaitingUser, ticket["status"])

	// 用户回复（应转回open）
	api.ok(user.Id, common.RoleCommonUser, http.MethodPost, fmt.Sprintf("/api/ticket/%d/reply", ticketID), map[string]any{
		"body": "已提供信息",
	})

	// 再次检查状态
	detailResp = api.ok(user.Id, common.RoleCommonUser, http.MethodGet, fmt.Sprintf("/api/ticket/%d", ticketID), nil)
	ticket = detailResp.object()["ticket"].(map[string]any)
	require.Equal(t, model.TicketStatusOpen, ticket["status"])
}

// TestTicketClose 测试关闭工单
func TestTicketClose(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)

	// 创建工单
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject":  "关闭测试",
		"category": model.TicketCategoryBilling,
		"body":     "将被关闭",
	})
	ticketID := int(resp.object()["id"].(float64))

	// 关闭工单
	api.ok(user.Id, common.RoleCommonUser, http.MethodPost, fmt.Sprintf("/api/ticket/%d/close", ticketID), nil)

	// 检查状态
	detailResp := api.ok(user.Id, common.RoleCommonUser, http.MethodGet, fmt.Sprintf("/api/ticket/%d", ticketID), nil)
	ticket := detailResp.object()["ticket"].(map[string]any)
	require.Equal(t, model.TicketStatusClosed, ticket["status"])

	// 尝试回复已关闭工单（应失败）
	api.denied(user.Id, common.RoleCommonUser, http.MethodPost, fmt.Sprintf("/api/ticket/%d/reply", ticketID), map[string]any{
		"body": "不应成功",
	})
}

// TestAppealTicketCreate 测试申诉工单创建
func TestAppealTicketCreate(t *testing.T) {
	api := setupTicketAPI(t)
	bannedUser := api.uniqueUser("banned", common.UserStatusDisabled)

	// 申诉用户创建工单
	resp := api.callAppeal(bannedUser.Id, http.MethodPost, "/api/appeal/tickets", map[string]any{
		"subject":  "账号申诉",
		"category": model.TicketCategoryAppeal,
		"body":     "我的账号被误封，请求解封",
	})
	require.True(t, resp.Success, "appeal ticket creation failed: %s", resp.Message)
	ticketID := int(resp.object()["id"].(float64))
	require.Greater(t, ticketID, 0)

	// 验证工单分类为申诉
	ticket, err := model.GetTicketByID(nil, ticketID)
	require.NoError(t, err)
	require.Equal(t, model.TicketCategoryAppeal, ticket.Category)
	require.Equal(t, bannedUser.Id, ticket.UserID)
}

// TestTicketPermission 测试工单权限隔离
func TestTicketPermission(t *testing.T) {
	api := setupTicketAPI(t)
	alice := api.uniqueUser("alice", common.UserStatusEnabled)
	bob := api.uniqueUser("bob", common.UserStatusEnabled)

	// Alice创建工单
	resp := api.ok(alice.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject":  "Alice的工单",
		"category": model.TicketCategoryBilling,
		"body":     "私密内容",
	})
	ticketID := int(resp.object()["id"].(float64))

	// Bob尝试访问Alice的工单（应失败）
	api.denied(bob.Id, common.RoleCommonUser, http.MethodGet, fmt.Sprintf("/api/ticket/%d", ticketID), nil)
	api.denied(bob.Id, common.RoleCommonUser, http.MethodPost, fmt.Sprintf("/api/ticket/%d/reply", ticketID), map[string]any{
		"body": "非法访问",
	})
}

// TestAdminTicketList 测试管理员工单列表
func TestAdminTicketList(t *testing.T) {
	api := setupTicketAPI(t)
	user1 := api.uniqueUser("user1", common.UserStatusEnabled)
	user2 := api.uniqueUser("user2", common.UserStatusEnabled)
	admin := api.uniqueUser("admin1", common.UserStatusEnabled)

	// 创建多个工单
	api.ok(user1.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject": "工单1", "category": model.TicketCategoryBilling, "body": "内容1",
	})
	api.ok(user2.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject": "工单2", "category": model.TicketCategoryAPI, "body": "内容2",
	})

	// 管理员查看全部工单
	listResp := api.ok(admin.Id, common.RoleAdminUser, http.MethodGet, "/api/admin/ticket", nil)
	tickets := listResp.object()["tickets"].([]any)
	require.Len(t, tickets, 2)
}

// TestAdminAssignTicket 测试工单分配
func TestAdminAssignTicket(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)
	admin := api.uniqueUser("admin1", common.UserStatusEnabled)

	// 创建工单
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject": "分配测试", "category": model.TicketCategoryAPI, "body": "需要分配",
	})
	ticketID := int(resp.object()["id"].(float64))

	// 分配给管理员
	api.ok(admin.Id, common.RoleAdminUser, http.MethodPut, fmt.Sprintf("/api/admin/ticket/%d/assign", ticketID), map[string]any{
		"assignee_id": admin.Id,
	})

	// 验证分配成功
	ticket, err := model.GetTicketByID(nil, ticketID)
	require.NoError(t, err)
	require.Equal(t, admin.Id, ticket.Assignee)
	// 指派和状态是独立操作；指派不得覆盖并发状态变化。
	require.Equal(t, model.TicketStatusOpen, ticket.Status)
}

// TestAdminUpdateStatus 测试管理员更新状态
func TestAdminUpdateStatus(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)
	admin := api.uniqueUser("admin1", common.UserStatusEnabled)

	// 创建工单
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject": "状态更新测试", "category": model.TicketCategoryBilling, "body": "测试",
	})
	ticketID := int(resp.object()["id"].(float64))

	// 管理员更新为resolved
	api.ok(admin.Id, common.RoleAdminUser, http.MethodPut, fmt.Sprintf("/api/admin/ticket/%d/status", ticketID), map[string]any{
		"status": model.TicketStatusResolved,
	})

	// 验证状态
	ticket, err := model.GetTicketByID(nil, ticketID)
	require.NoError(t, err)
	require.Equal(t, model.TicketStatusResolved, ticket.Status)
}

// TestInternalNote 测试内部备注
func TestInternalNote(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)
	admin := api.uniqueUser("admin1", common.UserStatusEnabled)

	// 创建工单
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject": "内部备注测试", "category": model.TicketCategoryAPI, "body": "测试",
	})
	ticketID := int(resp.object()["id"].(float64))

	// 管理员添加内部备注
	api.ok(admin.Id, common.RoleAdminUser, http.MethodPost, fmt.Sprintf("/api/admin/ticket/%d/reply", ticketID), map[string]any{
		"body":        "这是内部备注，用户不可见",
		"is_internal": true,
	})

	// 用户查看（不应看到内部备注）
	userResp := api.ok(user.Id, common.RoleCommonUser, http.MethodGet, fmt.Sprintf("/api/ticket/%d", ticketID), nil)
	userMessages := userResp.object()["messages"].([]any)
	require.Len(t, userMessages, 1) // 只有初始消息

	// 管理员查看（应看到内部备注）
	adminResp := api.ok(admin.Id, common.RoleAdminUser, http.MethodGet, fmt.Sprintf("/api/admin/ticket/%d", ticketID), nil)
	adminMessages := adminResp.object()["messages"].([]any)
	require.Len(t, adminMessages, 2) // 初始消息+内部备注
}

// TestUnreadCount 测试未读计数
func TestUnreadCount(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)
	admin := api.uniqueUser("admin1", common.UserStatusEnabled)

	// 创建工单
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject": "未读测试", "category": model.TicketCategoryBilling, "body": "测试",
	})
	ticketID := int(resp.object()["id"].(float64))

	// 管理员回复（用户应有未读）
	api.ok(admin.Id, common.RoleAdminUser, http.MethodPost, fmt.Sprintf("/api/admin/ticket/%d/reply", ticketID), map[string]any{
		"body": "管理员回复",
	})

	// 检查用户未读数
	countResp := api.ok(user.Id, common.RoleCommonUser, http.MethodGet, "/api/ticket/unread", nil)
	count := int(countResp.object()["count"].(float64))
	require.Equal(t, 1, count)

	// 用户查看工单（应标记为已读）
	api.ok(user.Id, common.RoleCommonUser, http.MethodGet, fmt.Sprintf("/api/ticket/%d", ticketID), nil)

	// 再次检查未读数（应为0）
	countResp = api.ok(user.Id, common.RoleCommonUser, http.MethodGet, "/api/ticket/unread", nil)
	count = int(countResp.object()["count"].(float64))
	require.Equal(t, 0, count)
}

// TestTicketAttachmentUpload 测试附件上传
func TestTicketAttachmentUpload(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)

	// 创建工单
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject": "附件测试", "category": model.TicketCategoryAPI, "body": "测试附件",
	})
	ticketID := int(resp.object()["id"].(float64))

	// 准备上传文件（PNG）
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("ticket_id", strconv.Itoa(ticketID))

	// 创建带Content-Type的part
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="file"; filename="test.png"`}
	h["Content-Type"] = []string{"image/png"}
	part, err := writer.CreatePart(h)
	require.NoError(t, err)

	// 写入PNG magic bytes
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	part.Write(pngHeader)
	part.Write([]byte("fake-png-content"))
	writer.Close()

	// 上传附件
	req := httptest.NewRequest(http.MethodPost, "/api/ticket/attachment/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Test-User", strconv.Itoa(user.Id))
	req.Header.Set("X-Test-Role", strconv.Itoa(common.RoleCommonUser))
	req.Header.Set("New-Api-User", strconv.Itoa(user.Id))
	w := httptest.NewRecorder()
	api.engine.ServeHTTP(w, req)

	var uploadResp ticketAPIResponse
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &uploadResp))
	require.True(t, uploadResp.Success, "upload failed: %s", uploadResp.Message)

	attachmentID := int(uploadResp.object()["id"].(float64))
	require.Greater(t, attachmentID, 0)

	// 验证附件记录
	attachment, err := model.GetTicketAttachmentByID(nil, attachmentID)
	require.NoError(t, err)
	require.Equal(t, ticketID, attachment.TicketID)
	require.Equal(t, "test.png", attachment.Filename)
	require.Equal(t, user.Id, attachment.UploadedByID)
}

// TestTicketAttachmentDownload 测试附件下载
func TestTicketAttachmentDownload(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)

	// 创建工单
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject": "下载测试", "category": model.TicketCategoryAPI, "body": "测试",
	})
	ticketID := int(resp.object()["id"].(float64))

	// 创建附件记录
	testContent := []byte("test-file-content")
	tmpFile := filepath.Join(controller.TicketAttachmentStorageDir, "test_download.txt")
	os.MkdirAll(controller.TicketAttachmentStorageDir, 0755)
	require.NoError(t, os.WriteFile(tmpFile, testContent, 0644))

	attachment := &model.TicketAttachment{
		TicketID:     ticketID,
		Filename:     "download.txt",
		MimeType:     "text/plain",
		Size:         len(testContent),
		StoragePath:  tmpFile,
		UploadedByID: user.Id,
	}
	attachment, err := model.CreateTicketAttachment(nil, attachment)
	require.NoError(t, err)

	// 下载附件
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/ticket/attachment/%d/download", attachment.ID), nil)
	req.Header.Set("X-Test-User", strconv.Itoa(user.Id))
	req.Header.Set("X-Test-Role", strconv.Itoa(common.RoleCommonUser))
	req.Header.Set("New-Api-User", strconv.Itoa(user.Id))
	w := httptest.NewRecorder()
	api.engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, testContent, w.Body.Bytes())
	require.Contains(t, w.Header().Get("Content-Disposition"), "download.txt")
}

// TestTicketAttachmentDelete 测试附件删除
func TestTicketAttachmentDelete(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)

	// 创建工单和附件
	resp := api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject": "删除测试", "category": model.TicketCategoryAPI, "body": "测试",
	})
	ticketID := int(resp.object()["id"].(float64))

	tmpFile := filepath.Join(controller.TicketAttachmentStorageDir, "test_delete.txt")
	os.MkdirAll(controller.TicketAttachmentStorageDir, 0755)
	require.NoError(t, os.WriteFile(tmpFile, []byte("delete-me"), 0644))

	attachment := &model.TicketAttachment{
		TicketID:     ticketID,
		Filename:     "delete.txt",
		MimeType:     "text/plain",
		Size:         9,
		StoragePath:  tmpFile,
		UploadedByID: user.Id,
	}
	attachment, err := model.CreateTicketAttachment(nil, attachment)
	require.NoError(t, err)

	// 删除附件
	api.ok(user.Id, common.RoleCommonUser, http.MethodDelete, fmt.Sprintf("/api/ticket/attachment/%d", attachment.ID), nil)

	// 验证已删除
	_, err = model.GetTicketAttachmentByID(nil, attachment.ID)
	require.Error(t, err)
	require.ErrorIs(t, err, model.ErrTicketAttachmentNotFound)

	// 文件也应被删除
	_, err = os.Stat(tmpFile)
	require.True(t, os.IsNotExist(err))
}

// TestTicketAttachmentPermission 测试附件权限
func TestTicketAttachmentPermission(t *testing.T) {
	api := setupTicketAPI(t)
	alice := api.uniqueUser("alice", common.UserStatusEnabled)
	bob := api.uniqueUser("bob", common.UserStatusEnabled)

	// Alice创建工单和附件
	resp := api.ok(alice.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject": "权限测试", "category": model.TicketCategoryAPI, "body": "测试",
	})
	ticketID := int(resp.object()["id"].(float64))

	tmpFile := filepath.Join(controller.TicketAttachmentStorageDir, "test_perm.txt")
	os.MkdirAll(controller.TicketAttachmentStorageDir, 0755)
	require.NoError(t, os.WriteFile(tmpFile, []byte("secret"), 0644))

	attachment := &model.TicketAttachment{
		TicketID:     ticketID,
		Filename:     "secret.txt",
		MimeType:     "text/plain",
		Size:         6,
		StoragePath:  tmpFile,
		UploadedByID: alice.Id,
	}
	attachment, err := model.CreateTicketAttachment(nil, attachment)
	require.NoError(t, err)

	// Bob尝试下载Alice的附件（应失败）
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/ticket/attachment/%d/download", attachment.ID), nil)
	req.Header.Set("X-Test-User", strconv.Itoa(bob.Id))
	req.Header.Set("X-Test-Role", strconv.Itoa(common.RoleCommonUser))
	req.Header.Set("New-Api-User", strconv.Itoa(bob.Id))
	w := httptest.NewRecorder()
	api.engine.ServeHTTP(w, req)

	var downloadResp ticketAPIResponse
	if w.Body.Len() > 0 {
		common.Unmarshal(w.Body.Bytes(), &downloadResp)
	}
	require.False(t, downloadResp.Success || w.Code == http.StatusOK, "Bob should not access Alice's attachment")

	// Bob尝试删除Alice的附件（应失败）
	api.denied(bob.Id, common.RoleCommonUser, http.MethodDelete, fmt.Sprintf("/api/ticket/attachment/%d", attachment.ID), nil)
}

// TestTicketRateLimit 测试限流（需要Redis才能真正生效，这里仅测试调用链路）
func TestTicketRateLimit(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)

	// 创建多个工单（无Redis时不会真正限流，但验证接口可调用）
	for i := 0; i < 2; i++ {
		resp := api.call(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
			"subject":  fmt.Sprintf("限流测试%d", i),
			"category": model.TicketCategoryBilling,
			"body":     "测试",
		})
		// 前几个应该成功（无Redis时全部成功）
		if i < 3 {
			require.True(t, resp.Success, "ticket %d should succeed", i)
		}
	}
}

// TestTicketMaxOpenConstraint 测试最大开放工单数限制
func TestTicketMaxOpenConstraint(t *testing.T) {
	api := setupTicketAPI(t)
	user := api.uniqueUser("alice", common.UserStatusEnabled)

	// 创建5个工单（限制数量）
	for i := 0; i < 5; i++ {
		api.ok(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
			"subject":  fmt.Sprintf("工单%d", i),
			"category": model.TicketCategoryBilling,
			"body":     "测试",
		})
		// 重置限流器避免触发限流
		common.ResetTicketRateLimiter()
	}

	// 再创建一个应该失败（超过最大开放数）
	resp := api.call(user.Id, common.RoleCommonUser, http.MethodPost, "/api/ticket", map[string]any{
		"subject":  "超限工单",
		"category": model.TicketCategoryBilling,
		"body":     "应失败",
	})
	require.False(t, resp.Success)
	require.Contains(t, resp.Message, "过多未关闭的工单")
}

// TestTicketAppealMaxConstraint 测试申诉工单唯一性
func TestTicketAppealMaxConstraint(t *testing.T) {
	api := setupTicketAPI(t)
	bannedUser := api.uniqueUser("banned", common.UserStatusDisabled)

	// 创建第一个申诉工单
	resp := api.callAppeal(bannedUser.Id, http.MethodPost, "/api/appeal/tickets", map[string]any{
		"subject":  "第一次申诉",
		"category": model.TicketCategoryAppeal,
		"body":     "请求解封",
	})
	require.True(t, resp.Success, "first appeal ticket creation failed: %s", resp.Message)

	// 重置限流器避免触发限流
	common.ResetTicketRateLimiter()

	// 尝试创建第二个申诉工单（应失败）
	resp = api.callAppeal(bannedUser.Id, http.MethodPost, "/api/appeal/tickets", map[string]any{
		"subject":  "第二次申诉",
		"category": model.TicketCategoryAppeal,
		"body":     "再次请求",
	})
	require.False(t, resp.Success, "second appeal should fail")
	require.Contains(t, resp.Message, "未关闭的申诉工单")
}
