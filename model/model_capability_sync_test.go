package model

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func capabilitySyncTestDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	var driver gorm.Dialector
	switch dialect {
	case "mysql":
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("disposable TEST_MYSQL_DSN is not configured")
		}
		driver = mysql.Open(dsn)
	case "postgres":
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("disposable TEST_POSTGRES_DSN is not configured")
		}
		driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
	default:
		driver = sqlite.Open(filepath.Join(t.TempDir(), "capabilities.db"))
	}
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	namespace := fmt.Sprintf("capability_sync_%d", time.Now().UnixNano())
	namespaceDB := db
	if dialect == "mysql" {
		require.NoError(t, db.Exec("CREATE DATABASE ?", clause.Table{Name: namespace}).Error)
		t.Cleanup(func() { require.NoError(t, namespaceDB.Exec("DROP DATABASE ?", clause.Table{Name: namespace}).Error) })
		require.NoError(t, db.Exec("USE ?", clause.Table{Name: namespace}).Error)
	} else if dialect == "postgres" {
		require.NoError(t, db.Exec("CREATE SCHEMA ?", clause.Table{Name: namespace}).Error)
		t.Cleanup(func() {
			require.NoError(t, namespaceDB.Exec("DROP SCHEMA ? CASCADE", clause.Table{Name: namespace}).Error)
		})
		require.NoError(t, db.Exec("SET search_path TO ?", clause.Table{Name: namespace}).Error)
	}
	// Bind the disposable namespace in the connection configuration, not only
	// in session SQL, so concurrent regression tests use independent connections.
	var concurrentDriver gorm.Dialector
	if dialect == "mysql" {
		config, err := mysqldriver.ParseDSN(os.Getenv("TEST_MYSQL_DSN"))
		require.NoError(t, err)
		config.DBName = namespace
		concurrentDriver = mysql.Open(config.FormatDSN())
	} else if dialect == "postgres" {
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			parsed, err := url.Parse(dsn)
			require.NoError(t, err)
			query := parsed.Query()
			query.Set("search_path", namespace)
			parsed.RawQuery = query.Encode()
			dsn = parsed.String()
		} else {
			dsn += " search_path=" + namespace
		}
		concurrentDriver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
	}
	if concurrentDriver != nil {
		separateDB, err := gorm.Open(concurrentDriver, &gorm.Config{})
		require.NoError(t, err)
		pool, err := separateDB.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = pool.Close() })
		db = separateDB
	}
	require.NoError(t, db.AutoMigrate(&Model{}, &Ability{}))
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous })
	return db
}

func capabilityEntry(contextLength int64) ModelCapabilityMetadata {
	return ModelCapabilityMetadata{ContextLength: contextLength,
		InputModalities: ModelModalities{"image", "text", "text"}, OutputModalities: ModelModalities{"text"}, Provider: "vendor"}
}

func readCapabilityModel(t *testing.T, id int) Model {
	t.Helper()
	var saved Model
	require.NoError(t, DB.First(&saved, id).Error)
	return saved
}

func TestModelCapabilitySyncOwnershipAndIdempotence(t *testing.T) {
	db := capabilitySyncTestDB(t, "sqlite")
	m := Model{FAQ: ModelFAQ{{Question: "Manual question", Answer: "Manual answer"}}, ModelName: "example", Status: 1, SyncOfficial: 1, ContextLength: 12000,
		OutputModalities: ModelModalities{"audio"}, Description: "curated", DisplayName: "Custom name"}
	require.NoError(t, db.Create(&m).Error)
	entries := map[string]ModelCapabilityMetadata{m.ModelName: capabilityEntry(128000)}
	result, err := ApplyModelCapabilityMetadata(context.Background(), entries, "source-a")
	require.NoError(t, err)
	require.Equal(t, ModelCapabilitySyncResult{Matched: 1, Updated: 1}, result)
	saved := readCapabilityModel(t, m.Id)
	require.EqualValues(t, 12000, saved.ContextLength)
	require.Equal(t, ModelModalities{"audio"}, saved.OutputModalities)
	require.Equal(t, ModelModalities{"text", "image"}, saved.InputModalities)
	require.Equal(t, m.Description, saved.Description)
	require.Equal(t, m.FAQ, saved.FAQ)
	require.Equal(t, m.DisplayName, saved.DisplayName)
	state := readModelCapabilityState(saved.ModelCapabilitySyncState)
	require.Nil(t, state.ContextLength)
	require.Empty(t, state.OutputModalities)
	require.Equal(t, saved.InputModalities, state.InputModalities)
	encoded, err := common.Marshal(saved)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "source-a")
	require.NotContains(t, string(encoded), "model_capability_sync_state")
	result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source-a")
	require.NoError(t, err)
	require.Equal(t, ModelCapabilitySyncResult{Matched: 1, Skipped: 1}, result)
	require.Equal(t, saved.ModelCapabilitySyncState, readCapabilityModel(t, m.Id).ModelCapabilitySyncState)

	entry := entries[m.ModelName]
	entry.InputModalities = ModelModalities{"text", "image", "video"}
	entry.Provider = "new-vendor"
	entries[m.ModelName] = entry
	result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source-b")
	require.NoError(t, err)
	require.Equal(t, 1, result.Updated)
	saved = readCapabilityModel(t, m.Id)
	require.Equal(t, entry.InputModalities, saved.InputModalities)
	state = readModelCapabilityState(saved.ModelCapabilitySyncState)
	require.Equal(t, "source-b", state.Source)
	require.Equal(t, "new-vendor", state.Provider)
	// A manual edit releases only that field, even with no other update to apply.
	saved.InputModalities = ModelModalities{"file"}
	require.NoError(t, saved.Update())
	result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source-b")
	require.NoError(t, err)
	require.Equal(t, 1, result.Updated)
	saved = readCapabilityModel(t, m.Id)
	require.Equal(t, ModelModalities{"file"}, saved.InputModalities)
	require.Empty(t, readModelCapabilityState(saved.ModelCapabilitySyncState).InputModalities)
	result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source-b")
	require.NoError(t, err)
	require.Equal(t, 0, result.Updated)
}

func TestModelCapabilitySyncEligibilityAndMissingModels(t *testing.T) {
	db := capabilitySyncTestDB(t, "sqlite")
	for _, m := range []Model{
		{ModelName: "disabled", SyncOfficial: 0, Status: 1},
		{ModelName: "prefix", SyncOfficial: 1, NameRule: NameRulePrefix, Status: 1},
		{ModelName: "contains", SyncOfficial: 1, NameRule: NameRuleContains, Status: 1},
		{ModelName: "suffix", SyncOfficial: 1, NameRule: NameRuleSuffix, Status: 1},
		{ModelName: "base-custom", SyncOfficial: 1, Status: 1},
		{ModelName: "CASE", SyncOfficial: 1, Status: 1},
	} {
		copy := m
		require.NoError(t, copy.Insert())
	}
	require.NoError(t, db.Create(&[]Ability{
		{Group: "a", Model: "fresh", ChannelId: 1, Enabled: true},
		{Group: "b", Model: "fresh", ChannelId: 2, Enabled: true},
		{Group: "a", Model: "not-enabled", ChannelId: 1, Enabled: false},
		{Group: "a", Model: "invalid", ChannelId: 1, Enabled: true},
	}).Error)
	entries := map[string]ModelCapabilityMetadata{}
	for _, name := range []string{"disabled", "prefix", "contains", "suffix", "base", "case", "fresh", "not-enabled", "not-configured"} {
		entries[name] = capabilityEntry(128000)
	}
	entries["invalid"] = ModelCapabilityMetadata{ContextLength: -1, InputModalities: ModelModalities{"unsupported"}}
	result, err := ApplyModelCapabilityMetadata(context.Background(), entries, "source")
	require.NoError(t, err)
	require.Equal(t, ModelCapabilitySyncResult{Matched: 2, Created: 1, Skipped: 5}, result)
	var rows []Model
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 7)
	for _, m := range rows {
		if m.ModelName == "fresh" {
			require.EqualValues(t, 128000, m.ContextLength)
			require.Equal(t, 1, m.Status)
			require.Equal(t, 1, m.SyncOfficial)
			require.Equal(t, NameRuleExact, m.NameRule)
			require.Equal(t, "source", readModelCapabilityState(m.ModelCapabilitySyncState).Source)
		} else {
			require.Zero(t, m.ContextLength)
			require.Empty(t, m.ModelCapabilitySyncState)
		}
	}
	var abilities []Ability
	require.NoError(t, db.Find(&abilities).Error)
	require.Len(t, abilities, 4)
}

func TestModelCapabilitySyncInvalidAndMissingFields(t *testing.T) {
	db := capabilitySyncTestDB(t, "sqlite")
	m := Model{ModelName: "test", Status: 1, SyncOfficial: 1}
	require.NoError(t, db.Create(&m).Error)
	entries := map[string]ModelCapabilityMetadata{"test": capabilityEntry(64000)}
	_, err := ApplyModelCapabilityMetadata(context.Background(), entries, "source")
	require.NoError(t, err)
	saved := readCapabilityModel(t, m.Id)
	for _, invalid := range []ModelCapabilityMetadata{
		{}, {ContextLength: -1}, {ContextLength: 9007199254740992},
		{InputModalities: ModelModalities{"text", "invalid"}, OutputModalities: ModelModalities{""}},
	} {
		entries["test"] = invalid
		result, err := ApplyModelCapabilityMetadata(context.Background(), entries, "source")
		require.NoError(t, err)
		require.Zero(t, result.Updated)
		require.Equal(t, saved.ModelCapabilitySyncState, readCapabilityModel(t, m.Id).ModelCapabilitySyncState)
	}
	entries["test"] = ModelCapabilityMetadata{ContextLength: -1, InputModalities: ModelModalities{"audio"}}
	result, err := ApplyModelCapabilityMetadata(context.Background(), entries, "source")
	require.NoError(t, err)
	require.Equal(t, 1, result.Updated)
	saved = readCapabilityModel(t, m.Id)
	require.EqualValues(t, 64000, saved.ContextLength)
	require.Equal(t, ModelModalities{"audio"}, saved.InputModalities)
	require.Equal(t, ModelModalities{"text"}, saved.OutputModalities)
	// Corrupt or unsupported state cannot confer ownership over non-empty data.
	for _, raw := range []string{`invalid`, `{"version":99,"source":"x","context_length":64000}`, `{"version":1,"source":"x","context_length":-1}`} {
		require.NoError(t, db.Model(&m).Update("model_capability_sync_state", raw).Error)
		entries["test"] = capabilityEntry(256000)
		result, err := ApplyModelCapabilityMetadata(context.Background(), entries, "source")
		require.NoError(t, err)
		require.Zero(t, result.Updated)
		require.EqualValues(t, 64000, readCapabilityModel(t, m.Id).ContextLength)
	}
}

func TestModelCapabilitySyncPreservesRulesAndTombstones(t *testing.T) {
	db := capabilitySyncTestDB(t, "sqlite")
	for _, m := range []Model{
		{ModelName: "prefix-", NameRule: NameRulePrefix, Status: 0, SyncOfficial: 1},
		{ModelName: "-suffix", NameRule: NameRuleSuffix, Status: 1, SyncOfficial: 0},
		{ModelName: "middle", NameRule: NameRuleContains, Status: 1, SyncOfficial: 1},
		{ModelName: "deleted", NameRule: NameRuleExact, Status: 1, SyncOfficial: 1},
		{ModelName: "removed-", NameRule: NameRulePrefix, Status: 1, SyncOfficial: 1},
	} {
		copy := m
		require.NoError(t, copy.Insert())
		if m.ModelName == "deleted" || m.ModelName == "removed-" {
			require.NoError(t, copy.Delete())
		}
	}
	entries := map[string]ModelCapabilityMetadata{}
	for _, name := range []string{"prefix-child", "child-suffix", "has-middle-name", "deleted", "removed-child"} {
		entries[name] = capabilityEntry(128000)
		require.NoError(t, db.Create(&Ability{Model: name, Group: "test", ChannelId: 1, Enabled: true}).Error)
	}
	result, err := ApplyModelCapabilityMetadata(context.Background(), entries, "source")
	require.NoError(t, err)
	require.Zero(t, result.Created)
	require.Zero(t, result.Updated)
	var count int64
	require.NoError(t, db.Unscoped().Model(&Model{}).Count(&count).Error)
	require.EqualValues(t, 5, count)
}

func TestModelCapabilitySyncConcurrentCreation(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := capabilitySyncTestDB(t, dialect)
			pool, err := db.DB()
			require.NoError(t, err)
			pool.SetMaxOpenConns(4)
			for _, manual := range []bool{false, true} {
				name := fmt.Sprintf("concurrent-%v", manual)
				require.NoError(t, db.Create(&Ability{Model: name, Group: "test", ChannelId: 1, Enabled: true}).Error)
				start := make(chan struct{})
				var workers sync.WaitGroup
				var errors [2]error
				for i := 0; i < 2; i++ {
					workers.Add(1)
					go func(index int) {
						defer workers.Done()
						<-start
						if manual && index == 1 {
							m := Model{ModelName: name, ContextLength: 42000, Status: 1, SyncOfficial: 1}
							errors[index] = m.Insert()
						} else {
							_, errors[index] = ApplyModelCapabilityMetadata(context.Background(), map[string]ModelCapabilityMetadata{name: capabilityEntry(128000)}, "source")
						}
					}(i)
				}
				close(start)
				workers.Wait()
				var count int64
				require.NoError(t, db.Model(&Model{}).Where("model_name = ?", name).Count(&count).Error)
				require.EqualValues(t, 1, count, "automatic and explicit creation must share a lock")
				for index, err := range errors {
					if err == nil {
						continue
					}
					if dialect == "sqlite" && strings.Contains(err.Error(), "SQLITE_BUSY") {
						continue
					}
					if manual && index == 1 {
						require.Contains(t, err.Error(), "模型名称已存在")
					} else {
						require.NoError(t, err)
					}
				}
			}
		})
	}
}

func TestModelCapabilitySyncDatabaseCompatibility(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := capabilitySyncTestDB(t, dialect)
			m := Model{ModelName: "legacy", Status: 1, SyncOfficial: 1}
			require.NoError(t, db.Create(&m).Error)
			// Existing installations can have NULL TEXT columns after migration.
			require.NoError(t, db.Model(&m).Updates(map[string]interface{}{"input_modalities": nil, "output_modalities": "", "model_capability_sync_state": nil}).Error)
			entries := map[string]ModelCapabilityMetadata{"legacy": capabilityEntry(128000)}
			result, err := ApplyModelCapabilityMetadata(context.Background(), entries, "source")
			require.NoError(t, err)
			require.Equal(t, 1, result.Updated)
			entry := capabilityEntry(256000)
			entries["legacy"] = entry
			result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source")
			require.NoError(t, err)
			require.Equal(t, 1, result.Updated)
			require.EqualValues(t, 256000, readCapabilityModel(t, m.Id).ContextLength)
			var snapshot modelCapabilitySnapshot
			require.NoError(t, db.Model(&Model{}).Select("*, official_pricing AS capability_pricing_snapshot, release_date AS capability_release_snapshot, context_length AS capability_context_snapshot, max_output_tokens AS capability_max_output_snapshot, capabilities AS capability_flags_snapshot, input_modalities AS capability_input_snapshot, output_modalities AS capability_output_snapshot, model_capability_sync_state AS capability_state_snapshot").First(&snapshot, m.Id).Error)
			require.NoError(t, db.Model(&m).Update("context_length", 42000).Error)
			changed, _, err := applyModelCapabilitySnapshot(context.Background(), snapshot, capabilityEntry(512000), "source")
			require.NoError(t, err)
			require.False(t, changed, "a concurrent manual save must win")
			require.EqualValues(t, 42000, readCapabilityModel(t, m.Id).ContextLength)
			// Exact model identity is case-sensitive even on MySQL's default collation.
			require.NoError(t, db.Model(&m).Update("context_length", snapshot.ContextLength).Error)
			require.NoError(t, db.Model(&m).Update("model_name", "LEGACY").Error)
			changed, _, err = applyModelCapabilitySnapshot(context.Background(), snapshot, capabilityEntry(512000), "source")
			require.NoError(t, err)
			require.False(t, changed)
			require.NoError(t, db.Model(&m).Updates(map[string]interface{}{"model_name": "legacy", "context_length": 42000}).Error)
			result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source")
			require.NoError(t, err)
			require.Equal(t, 1, result.Updated, "release context ownership")
			require.Nil(t, readModelCapabilityState(readCapabilityModel(t, m.Id).ModelCapabilitySyncState).ContextLength)
			require.NoError(t, db.Model(&m).Updates(map[string]interface{}{"sync_official": 0, "context_length": 0}).Error)
			result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source")
			require.NoError(t, err)
			require.Zero(t, result.Updated)
			require.Zero(t, readCapabilityModel(t, m.Id).ContextLength)
		})
	}
}

func TestModelReleaseDateSyncOwnershipAndDatabaseCompatibility(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := capabilitySyncTestDB(t, dialect)
			m := Model{ModelName: "dated", Status: 1, SyncOfficial: 1}
			require.NoError(t, db.Create(&m).Error)
			// Preserve SQL NULL from old schemas in optimistic WHERE conditions.
			require.NoError(t, db.Model(&m).Update("release_date", nil).Error)
			entry := ModelCapabilityMetadata{ReleaseDate: "2025-01", Provider: "native"}
			entries := map[string]ModelCapabilityMetadata{m.ModelName: entry}
			result, err := ApplyModelCapabilityMetadata(context.Background(), entries, "source")
			require.NoError(t, err)
			require.Equal(t, 1, result.Updated)
			saved := readCapabilityModel(t, m.Id)
			require.Equal(t, "2025-01", saved.ReleaseDate)
			require.Equal(t, "2025-01", *readModelCapabilityState(saved.ModelCapabilitySyncState).ReleaseDate)
			result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source")
			require.NoError(t, err)
			require.Zero(t, result.Updated)
			entry.ReleaseDate = "2026-10-24"
			entries[m.ModelName] = entry
			result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source")
			require.NoError(t, err)
			require.Equal(t, 1, result.Updated)
			saved = readCapabilityModel(t, m.Id)
			for _, invalid := range []string{"", "invalid", "2025-02-29"} {
				entry.ReleaseDate = invalid
				entries[m.ModelName] = entry
				result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source")
				require.NoError(t, err)
				require.Zero(t, result.Updated)
				require.Equal(t, saved.ModelCapabilitySyncState, readCapabilityModel(t, m.Id).ModelCapabilitySyncState)
			}
			var snapshot modelCapabilitySnapshot
			require.NoError(t, db.Model(&Model{}).Select("*, official_pricing AS capability_pricing_snapshot, release_date AS capability_release_snapshot, context_length AS capability_context_snapshot, max_output_tokens AS capability_max_output_snapshot, capabilities AS capability_flags_snapshot, input_modalities AS capability_input_snapshot, output_modalities AS capability_output_snapshot, model_capability_sync_state AS capability_state_snapshot").First(&snapshot, m.Id).Error)
			require.NoError(t, db.Model(&m).Update("release_date", "2026-10-25").Error)
			entry.ReleaseDate = "2026-10-26"
			changed, _, err := applyModelCapabilitySnapshot(context.Background(), snapshot, entry, "source")
			require.NoError(t, err)
			require.False(t, changed, "a concurrent manual date edit wins")
			entries[m.ModelName] = entry
			result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source")
			require.NoError(t, err)
			require.Equal(t, 1, result.Updated, "release manual date ownership")
			saved = readCapabilityModel(t, m.Id)
			require.Equal(t, "2026-10-25", saved.ReleaseDate)
			require.Nil(t, readModelCapabilityState(saved.ModelCapabilitySyncState).ReleaseDate)
			result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source")
			require.NoError(t, err)
			require.Zero(t, result.Updated)
			// Clearing permits automatic fill; disabling keeps it empty.
			require.NoError(t, db.Model(&m).Update("release_date", "").Error)
			result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source")
			require.NoError(t, err)
			require.Equal(t, 1, result.Updated)
			require.Equal(t, "2026-10-26", readCapabilityModel(t, m.Id).ReleaseDate)
			require.NoError(t, db.Model(&m).Updates(map[string]interface{}{"release_date": "", "sync_official": 0}).Error)
			result, err = ApplyModelCapabilityMetadata(context.Background(), entries, "source")
			require.NoError(t, err)
			require.Zero(t, result.Updated)
			require.Empty(t, readCapabilityModel(t, m.Id).ReleaseDate)
			// Valid date alone is sufficient to create enabled missing metadata.
			require.NoError(t, db.Create(&Ability{Model: "date-only", Group: "test", ChannelId: 1, Enabled: true}).Error)
			result, err = ApplyModelCapabilityMetadata(context.Background(), map[string]ModelCapabilityMetadata{"date-only": entry}, "source")
			require.NoError(t, err)
			require.Equal(t, 1, result.Created)
			var fresh Model
			require.NoError(t, db.Where("model_name = ?", "date-only").First(&fresh).Error)
			require.Equal(t, "2026-10-26", fresh.ReleaseDate)
			require.Zero(t, fresh.ContextLength)
		})
	}
}

func TestModelReleaseDateSyncPreservesManualAndUpgradesOldState(t *testing.T) {
	db := capabilitySyncTestDB(t, "sqlite")
	for _, initial := range []string{"", "2025-01"} {
		m := Model{ModelName: "model-" + initial, ReleaseDate: initial, ContextLength: 128000,
			Status: 1, SyncOfficial: 1, ModelCapabilitySyncState: `{"version":1,"source":"source","context_length":128000}`}
		require.NoError(t, db.Create(&m).Error)
		entry := ModelCapabilityMetadata{ReleaseDate: "2026-10-24", ContextLength: 256000}
		_, err := ApplyModelCapabilityMetadata(context.Background(), map[string]ModelCapabilityMetadata{m.ModelName: entry}, "source")
		require.NoError(t, err)
		saved := readCapabilityModel(t, m.Id)
		require.EqualValues(t, 256000, saved.ContextLength, "old state still owns context")
		if initial == "" {
			require.Equal(t, "2026-10-24", saved.ReleaseDate)
		} else {
			require.Equal(t, initial, saved.ReleaseDate, "pre-existing manual dates are retained")
			require.Nil(t, readModelCapabilityState(saved.ModelCapabilitySyncState).ReleaseDate)
		}
	}
}
