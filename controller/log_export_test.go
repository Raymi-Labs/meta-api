package controller

import (
	"context"
	"encoding/csv"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLogExportDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldLogDB, oldRedis, oldType := model.DB, model.LOG_DB, common.RedisEnabled, common.LogSqlType
	model.DB, model.LOG_DB, common.RedisEnabled, common.LogSqlType = db, db, false, common.DatabaseTypeSQLite
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.LogSqlType = oldDB, oldLogDB, oldRedis, oldType
		sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Channel{}))
	return db
}

func callLogExport(t *testing.T, userID int, admin bool, query string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?start_timestamp=100&end_timestamp=200"+query, nil)
	c.Set("id", userID)
	exportLogs(c, admin)
	return w
}

func TestLogExportListParityAndPrivacy(t *testing.T) {
	db := setupLogExportDB(t)
	for i := 0; i < 260; i++ {
		require.NoError(t, db.Create(&model.Log{UserId: 1, Username: "alice", CreatedAt: 100, Type: model.LogTypeConsume, ChannelId: 8, Group: "OpenAI · 优质", ModelName: "gpt-test", TokenName: "=HYPERLINK(\"evil\")", Quota: 25, RequestId: "req", Content: "管理员(ID:42)调整", Other: `{"admin_info":{"secret":"hidden"}}`}).Error)
	}
	require.NoError(t, db.Create(&model.Log{UserId: 2, Username: "bob", CreatedAt: 200, Type: model.LogTypeConsume, Group: "OpenAI · 优质", Content: "other user secret"}).Error)
	require.NoError(t, db.Create(&model.Log{UserId: 1, CreatedAt: 201, Type: model.LogTypeConsume, Content: "outside"}).Error)
	w := callLogExport(t, 1, false, "&username=bob&channel_ids=999&group_vendor=Claude")
	require.Equal(t, 200, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\ufeff"))).ReadAll()
	require.NoError(t, err)
	_, total, err := model.GetUserLogs(1, 0, 100, 200, "", "", 0, 10, "", "")
	require.NoError(t, err)
	require.Len(t, rows, int(total)+1)
	require.Equal(t, "'=HYPERLINK(\"evil\")", rows[1][2])
	require.NotContains(t, w.Body.String(), "other user secret")
	require.NotContains(t, w.Body.String(), "admin_info")
	require.NotContains(t, w.Body.String(), "hidden")
	require.NotContains(t, w.Body.String(), "ID:42")
	require.NotContains(t, w.Body.String(), "渠道ID")
	w = callLogExport(t, 9, true, "&username=1&channel_ids=8&group_vendor=OpenAI&type=2&model_name=gpt-test&request_id=req")
	require.Equal(t, 200, w.Code)
	rows, err = csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\ufeff"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 261)
	require.Contains(t, rows[0], "渠道ID")
	w = callLogExport(t, 9, true, "&channel_keyword=no-matching-channel")
	require.Equal(t, 200, w.Code)
	require.Equal(t, 1, strings.Count(w.Body.String(), "\n"))
}

func TestLogExportLimitsAndFailure(t *testing.T) {
	db := setupLogExportDB(t)
	require.Equal(t, 401, callLogExport(t, 0, false, "").Code)
	require.Equal(t, 400, callLogExport(t, 1, true, "&type=bad").Code)
	require.Equal(t, 400, callLogExport(t, 1, true, "&channel_ids=-1").Code)
	for _, query := range []string{"", "?start_timestamp=0&end_timestamp=1", "?start_timestamp=200&end_timestamp=100", "?start_timestamp=1&end_timestamp=9999999"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("id", 1)
		c.Request = httptest.NewRequest("GET", "/"+query, nil)
		ExportUserLogs(c)
		require.Equal(t, 400, w.Code)
	}
	batch := make([]model.Log, model.LogExportMaxRows+1)
	for i := range batch {
		batch[i] = model.Log{UserId: 1, CreatedAt: 100}
	}
	require.NoError(t, db.CreateInBatches(batch, 200).Error)
	w := callLogExport(t, 1, false, "")
	require.Equal(t, 400, w.Code)
	require.Contains(t, w.Body.String(), "10000")
	require.NotContains(t, w.Header().Get("Content-Type"), "text/csv")
	require.Empty(t, w.Header().Get("Content-Disposition"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, model.WalkExportLogs(ctx, model.AdminLogQueryFilters{}, 1, false, func(*model.Log) error { t.Fatal("canceled query wrote a row"); return nil }))
}

func TestLogExportCursorExcludesNewRows(t *testing.T) {
	db := setupLogExportDB(t)
	rows := make([]model.Log, 501)
	for i := range rows {
		rows[i] = model.Log{UserId: 1, CreatedAt: 100}
	}
	require.NoError(t, db.CreateInBatches(rows, 200).Error)
	count := 0
	require.NoError(t, model.WalkExportLogs(context.Background(), model.AdminLogQueryFilters{}, 1, false, func(*model.Log) error {
		count++
		if count == 1 {
			return db.Create(&model.Log{UserId: 1, CreatedAt: 100}).Error
		}
		return nil
	}))
	require.Equal(t, 501, count)
	for _, text := range []string{"=cmd", "+cmd", "-cmd", "@cmd", "  =cmd", "\tcmd", "\n=cmd", "\ufeff=cmd"} {
		require.True(t, strings.HasPrefix(logCSVText(text), "'"))
	}
	require.Equal(t, "普通文本,\"换行\"\n下一行", logCSVText("普通文本,\"换行\"\n下一行"))
}
