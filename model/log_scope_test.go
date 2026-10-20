package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type logScopeTestDialect struct {
	gorm.Dialector
	name string
}

func (d logScopeTestDialect) Name() string { return d.name }

func TestLogScopeSQLDialects(t *testing.T) {
	base := setupLogQueryTestDB(t)
	for _, tc := range []struct{ dialect, function, quoted string }{
		{"sqlite", "INSTR(", "`group`"},
		{"mysql", "SUBSTRING_INDEX(", "`group`"},
		{"postgres", "SPLIT_PART(", `"group"`},
	} {
		t.Run(tc.dialect, func(t *testing.T) {
			oldType := common.LogSqlType
			common.LogSqlType = tc.dialect
			defer func() { common.LogSqlType = oldType }()
			db := base.Session(&gorm.Session{DryRun: true, NewDB: true})
			config := *db.Config
			config.Dialector = logScopeTestDialect{Dialector: db.Dialector, name: tc.dialect}
			db.Config = &config
			var rows []Log
			tx := applyLogScope(db.Model(&Log{}), LogScope{Vendor: "OpenAI", ChannelIDs: []int{1, 2}}).Find(&rows)
			sql := tx.Statement.SQL.String()
			if !strings.Contains(sql, tc.function) || !strings.Contains(sql, tc.quoted) {
				t.Fatalf("unexpected dialect SQL: %s", sql)
			}
			if strings.Contains(sql, "openai") {
				t.Fatalf("vendor must be parameterized: %s", sql)
			}
		})
	}
}

func TestLogScopeVendorAndChannels(t *testing.T) {
	db := setupLogQueryTestDB(t)
	now := time.Now().Unix()
	groups := []string{"OpenAI · 企业专属", "OpenAI·优质", "Claude · 优质", "default", "企业专属", "gemini", "Gemini · 优质", "Deepseek · 优质", "xiaomi · 优质", "Custom · 优质", "Custom", "Inception · 标准", "Mercury · 优质", " inception labs ", "InceptionLabs·企业专属"}
	for i, group := range groups {
		for _, channel := range []int{1, 2} {
			if err := db.Create(&Log{Type: LogTypeConsume, Group: group, ChannelId: channel,
				CreatedAt: now - 2, Quota: (i + 1) * 10, PromptTokens: 100, CompletionTokens: 10,
				Other: `{"cache_tokens":50}`, Username: "scope-user"}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	cases := []struct {
		vendor string
		count  int64
	}{
		{"OpenAI", 2}, {"Claude", 1}, {"__other__", 3}, {"Gemini", 2},
		{"DeepSeek", 1}, {"MiMo", 1}, {"Custom", 1}, {"absent", 0},
		{"Inception", 4}, {"Mercury", 4}, {"inception labs", 4}, {"inceptionlabs", 4},
		{"OpenAI' OR 1=1 --", 0},
	}
	for _, tc := range cases {
		t.Run(tc.vendor, func(t *testing.T) {
			scope := LogScope{Vendor: tc.vendor, ChannelIDs: []int{2}}
			filters := AdminLogQueryFilters{Scope: scope, StartTimestamp: now - 10, EndTimestamp: now}
			tx, err := applyAdminLogFilters(db.Model(&Log{}), filters, true)
			if err != nil {
				t.Fatal(err)
			}
			var count int64
			if err := tx.Count(&count).Error; err != nil || count != tc.count {
				t.Fatalf("count=%d want=%d err=%v", count, tc.count, err)
			}
			rows, err := GetLogGroupSummary(filters)
			if err != nil {
				t.Fatal(err)
			}
			var summaryCount, summaryQuota int64
			for _, row := range rows {
				summaryCount += row.RequestCount
				summaryQuota += row.Quota
			}
			stat, err := SumUsedQuota(LogTypeUnknown, now-10, now, "", "", "", 0, "", "", true, scope)
			if err != nil || summaryCount != tc.count || int64(stat.Quota) != summaryQuota || int64(stat.Rpm) != tc.count {
				t.Fatalf("summary/stat mismatch: rows=%+v stat=%+v err=%v", rows, stat, err)
			}
			if tc.count > 0 && stat.CacheGlobalRate != 0.5 {
				t.Fatalf("cache not scoped: %+v", stat)
			}
		})
	}
	// Empty matches must never silently turn into all channels.
	var count int64
	applyLogScope(db.Model(&Log{}), LogScope{ChannelIDs: []int{}}).Count(&count)
	if count != 0 {
		t.Fatal("empty channel scope matched logs")
	}
	// An explicitly conflicting group intersects rather than overrides vendor.
	tx, err := applyAdminLogFilters(db.Model(&Log{}), AdminLogQueryFilters{
		Scope: LogScope{Vendor: "OpenAI"}, Group: "Claude · 优质",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	tx.Count(&count)
	if count != 0 {
		t.Fatal("group escaped vendor scope")
	}
	// Old time ranges cannot show present-minute RPM.
	stat, err := SumUsedQuota(0, now-1000, now-500, "", "", "", 0, "", "", true, LogScope{Vendor: "OpenAI"})
	if err != nil || stat.Rpm != 0 || stat.Quota != 0 {
		t.Fatalf("time window escaped: %+v %v", stat, err)
	}
}

func TestLogChannelOptionsUsePrimaryDBAndNoSecrets(t *testing.T) {
	logDB := setupLogQueryTestDB(t)
	primary, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	oldDB, oldCache := DB, common.MemoryCacheEnabled
	DB, common.MemoryCacheEnabled = primary, false
	t.Cleanup(func() { DB, common.MemoryCacheEnabled = oldDB, oldCache })
	if err := primary.AutoMigrate(&Channel{}); err != nil {
		t.Fatal(err)
	}
	for _, channel := range []Channel{
		{Id: 1, Name: "muze-企业", Key: "must-not-leak", Status: 1},
		{Id: 2, Name: "MUZE-独享", Key: "must-not-leak", Status: 2},
		{Id: 3, Name: "other", Key: "must-not-leak", Status: 1},
	} {
		if err := primary.Create(&channel).Error; err != nil {
			t.Fatal(err)
		}
	}
	options, err := FindLogChannels("muze")
	if err != nil || len(options) != 2 {
		t.Fatalf("disabled or case-insensitive match missing: %+v %v", options, err)
	}
	options, err = FindLogChannels("%_")
	if err != nil || len(options) != 0 {
		t.Fatalf("wildcards were not escaped: %+v %v", options, err)
	}
	now := time.Now().Unix()
	for _, channelID := range []int{1, 2, 999} {
		logDB.Create(&Log{Type: LogTypeConsume, ChannelId: channelID, Group: "OpenAI · 已删除分组",
			CreatedAt: now, Quota: 10, Username: "scope-user"})
	}
	logDB.Create(&Log{Type: LogTypeError, ChannelId: 1, Group: "OpenAI · 已删除分组", CreatedAt: now})
	scope := LogScope{Vendor: "OpenAI", ChannelIDs: []int{1, 2, 999}}
	logs, total, err := GetAllLogs(LogTypeConsume, now-10, now+1, "", "", "", 0, 1, 0, "", "", scope)
	if err != nil || total != 3 || len(logs) != 1 {
		t.Fatalf("scoped listing failed: total=%d logs=%d %v", total, len(logs), err)
	}
	filters := AdminLogQueryFilters{Scope: scope, StartTimestamp: now - 10, EndTimestamp: now + 1}
	rows, err := GetLogGroupSummary(filters)
	if err != nil || len(rows) != 1 || rows[0].RequestCount != 3 || rows[0].Quota != 30 {
		t.Fatalf("summary should include history/deleted channels, not errors or only first page: %+v %v", rows, err)
	}
	filters.LogType = LogTypeConsume
	users, _, err := GetTopUsers(filters, 10, "desc", "desc")
	if err != nil || len(users) != 1 || users[0].RequestCount != 3 {
		t.Fatalf("top users scope mismatch: %+v %v", users, err)
	}
}
