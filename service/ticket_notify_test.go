package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type ticketNotifyTransport func(*http.Request) (*http.Response, error)

func (f ticketNotifyTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTicketNotificationUsesCurrentRecipientPermissions(t *testing.T) {
	setupNotificationLimitTest(t, 100, 60)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldClient, oldRedis := model.DB, httpClient, common.RedisEnabled
	fetch := system_setting.GetFetchSetting()
	oldProtection := fetch.EnableSSRFProtection
	model.DB, common.RedisEnabled, fetch.EnableSSRFProtection = db, false, false
	t.Cleanup(func() {
		WaitTicketNotifications()
		model.DB, httpClient, common.RedisEnabled = oldDB, oldClient, oldRedis
		fetch.EnableSSRFProtection = oldProtection
		sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Ticket{}))
	create := func(name string, role int) *model.User {
		setting, err := common.Marshal(dto.UserSetting{NotifyType: dto.NotifyTypeWebhook, WebhookUrl: "http://127.0.0.1:1/" + name})
		require.NoError(t, err)
		u := &model.User{Username: name, AffCode: name, Status: 1, Role: role, Setting: string(setting)}
		require.NoError(t, db.Create(u).Error)
		return u
	}
	root, staff, replacement := create("root", 100), create("staff", 10), create("replacement", 10)
	ticket := &model.Ticket{UserID: 999, Subject: "private title", Category: "api", Status: "open", Assignee: staff.Id}
	require.NoError(t, db.Create(ticket).Error)
	var recipients []string
	httpClient = &http.Client{Transport: ticketNotifyTransport(func(r *http.Request) (*http.Response, error) {
		recipients = append(recipients, r.URL.Path)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	notify := func(want string) {
		t.Helper()
		recipients = nil
		NotifyTicketUserReply(ticket)
		WaitTicketNotifications()
		if want == "" {
			require.Empty(t, recipients)
		} else {
			require.Equal(t, []string{want}, recipients)
		}
	}
	notify("/staff")
	require.NoError(t, db.Model(staff).Update("role", 1).Error)
	notify("/root")
	require.NoError(t, db.Model(staff).Updates(map[string]any{"role": 10, "status": 2}).Error)
	notify("/root")
	// The queued ticket snapshot still points at staff; use the persisted assignment.
	require.NoError(t, model.AssignTicket(context.Background(), ticket.ID, replacement.Id))
	notify("/replacement")
	require.NoError(t, db.Model(replacement).Update("role", 1).Error)
	require.NoError(t, db.Model(root).Update("status", 2).Error)
	notify("")
}
