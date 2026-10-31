package model

import (
	"crypto/sha256"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Optional DSNs must point to disposable local databases. The test creates and
// removes only its unique user and voice rows; it also exercises repeat migration.
func TestAudioVoiceDatabaseConcurrency(t *testing.T) {
	engines := []struct {
		name, dsn string
		open      func(string) gorm.Dialector
	}{
		{"sqlite", "file:" + t.Name() + "?mode=memory&cache=shared", func(s string) gorm.Dialector { return sqlite.Open(s) }},
		{"mysql", os.Getenv("VOICE_TEST_MYSQL_DSN"), func(s string) gorm.Dialector { return mysql.Open(s) }},
		{"postgres", os.Getenv("VOICE_TEST_POSTGRES_DSN"), func(s string) gorm.Dialector { return postgres.Open(s) }},
	}
	for _, engine := range engines {
		t.Run(engine.name, func(t *testing.T) {
			if engine.dsn == "" {
				t.Skip("disposable database DSN not configured")
			}
			db, err := gorm.Open(engine.open(engine.dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			defer sqlDB.Close()
			if engine.name == "sqlite" {
				sqlDB.SetMaxOpenConns(1)
			} else {
				sqlDB.SetMaxOpenConns(8)
			}
			oldDB := DB
			DB = db
			defer func() { DB = oldDB }()
			require.NoError(t, db.AutoMigrate(&User{}, &AudioVoice{}))
			require.NoError(t, db.AutoMigrate(&User{}, &AudioVoice{}))
			user := User{Username: "voice-" + engine.name, AffCode: "vc" + engine.name}
			require.NoError(t, db.Create(&user).Error)
			defer db.Unscoped().Delete(&user)
			defer db.Where("user_id = ?", user.Id).Delete(&AudioVoice{})
			makeVoice := func(n int) *AudioVoice {
				value := fmt.Sprintf("%s/%d/%d", engine.name, user.Id, n)
				return &AudioVoice{ID: value, UserID: user.Id, Model: "fixture", Status: "creating", Prefix: fmt.Sprintf("mv%08x", n), RequestHash: "same-request", IdempotencyHash: fmt.Sprintf("%x", sha256.Sum256([]byte(value)))}
			}
			var wg sync.WaitGroup
			var created atomic.Int32
			for range 8 {
				wg.Go(func() {
					old, err := ReserveAudioVoice(makeVoice(0))
					if err != nil {
						t.Error(err)
					} else if old == nil {
						created.Add(1)
					}
				})
			}
			wg.Wait()
			require.EqualValues(t, 1, created.Load())
			conflict := makeVoice(0)
			conflict.RequestHash = "different"
			_, err = ReserveAudioVoice(conflict)
			require.ErrorIs(t, err, ErrAudioVoiceConflict)
			created.Store(1)
			for n := 1; n < 32; n++ {
				wg.Go(func() {
					old, err := ReserveAudioVoice(makeVoice(n))
					if err == nil && old == nil {
						created.Add(1)
					} else if err != ErrAudioVoiceLimit {
						t.Error(err)
					}
				})
			}
			wg.Wait()
			require.EqualValues(t, AudioVoiceLimit, created.Load())
			v, err := GetAudioVoice(makeVoice(0).ID, user.Id, 0)
			require.NoError(t, err)
			stale := *v
			require.NoError(t, UpdateAudioVoice(v, "ready", "provider"))
			require.NoError(t, UpdateAudioVoice(v, "ready", "provider"))
			require.Error(t, UpdateAudioVoice(&stale, "processing", "provider"), "stale response must not overwrite new state")
			require.NoError(t, UpdateAudioVoice(v, "deleted", "provider"))
			_, err = ReserveAudioVoice(makeVoice(100))
			require.NoError(t, err)
			old, err := ReserveAudioVoice(makeVoice(0))
			require.NoError(t, err)
			require.Equal(t, "deleted", old.Status, "old idempotency key never recreates deleted voice")
			_, err = GetAudioVoice(v.ID, user.Id+1, 0)
			require.Error(t, err)
			_, err = GetAudioVoice(v.ID, user.Id, 99)
			require.Error(t, err)
		})
	}
}
