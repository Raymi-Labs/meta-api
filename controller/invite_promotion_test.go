package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInvitePromotionRedirectAndScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	old := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = old; require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.InvitePromotionClick{}))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "inviter", AffCode: "invite1", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.User{Id: 2, Username: "disabled", AffCode: "disabled2", Status: common.UserStatusDisabled}).Error)
	router := gin.New()
	router.GET("/i/:code", InviteRedirect)
	router.GET("/self", func(c *gin.Context) { c.Set("id", 1) }, GetSelfInvitePromotion)
	router.GET("/admin/:id", GetAdminInvitePromotion)
	for _, test := range []struct {
		path, purpose string
		count         int64
	}{
		{"/i/invite1?src=banner", "", 1},
		{"/i/invite1?src=banner", "prefetch", 1},
		{"/i/invite1?src=invalid", "", 2},
		{"/i/unknown", "", 2},
		{"/i/disabled2", "", 2},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", test.path, nil)
		r.Header.Set("Sec-Purpose", test.purpose)
		router.ServeHTTP(w, r)
		require.Equal(t, http.StatusFound, w.Code)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		require.Contains(t, w.Header().Get("Location"), "/register?aff=")
		var total int64
		require.NoError(t, db.Model(&model.InvitePromotionClick{}).Select("COALESCE(SUM(clicks),0)").Scan(&total).Error)
		require.Equal(t, test.count, total)
	}
	// Changing a code does not reset statistics; old codes no longer accrue visits.
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 1).Update("aff_code", "updated1").Error)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/i/updated1?src=button", nil))
	require.Equal(t, 302, w.Code)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/i/invite1", nil))
	require.Equal(t, 302, w.Code)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/self?user_id=2", nil))
	var body struct {
		Success bool
		Data    model.InvitePromotionStats
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Equal(t, int64(3), body.Data.Clicks)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/admin/2", nil))
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.Zero(t, body.Data.Clicks)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/admin/invalid", nil))
	require.Equal(t, 400, w.Code)
	// Optional analytics storage failure must not block registration.
	require.NoError(t, db.Migrator().DropTable(&model.InvitePromotionClick{}))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/i/updated1", nil))
	require.Equal(t, 302, w.Code)
	require.Equal(t, "/register?aff=updated1", w.Header().Get("Location"))
}
