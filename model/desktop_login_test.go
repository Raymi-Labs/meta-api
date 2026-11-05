package model

import (
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Only disposable, explicitly configured databases; never production DSNs.
func TestDesktopLoginDatabaseMatrix(t *testing.T) {
	for _, kind := range []string{"mysql", "postgres"} {
		t.Run(kind, func(t *testing.T) {
			key := "TEST_MYSQL_DSN"
			if kind == "postgres" {
				key = "TEST_POSTGRES_DSN"
			}
			dsn := os.Getenv(key)
			if dsn == "" {
				t.Skip("disposable database DSN not configured")
			}
			var driver gorm.Dialector = mysql.Open(dsn)
			if kind == "postgres" {
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sql, err := db.DB()
			require.NoError(t, err)
			old := DB
			DB = db
			t.Cleanup(func() { DB = old; _ = sql.Close() })
			TestDesktopLoginBindingExpiryAndSingleUse(t)
		})
	}
}

func TestDesktopLoginBindingExpiryAndSingleUse(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&DesktopLoginGrant{}))
	verifier, err := DesktopLoginToken()
	require.NoError(t, err)
	id, g, err := NewDesktopLogin(DesktopLoginChallenge(verifier), "github")
	require.NoError(t, err)
	require.NotEqual(t, id, g.Digest)
	wrong, _ := DesktopLoginToken()
	_, err = DesktopLoginWithProof(id, wrong)
	require.Error(t, err)
	require.Error(t, ApproveDesktopLogin(id, 42, "wrong", false))
	require.NoError(t, ApproveDesktopLogin(id, 42, g.DisplayCode, false))
	require.Error(t, ApproveDesktopLogin(id, 43, g.DisplayCode, false))
	proof, err := DesktopLoginWithProof(id, verifier)
	require.NoError(t, err)
	require.Equal(t, 42, proof.UserID)
	var n atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ConsumeDesktopLogin(proof) == nil {
				n.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, n.Load())
	require.Error(t, ConsumeDesktopLogin(proof))
	id, g, err = NewDesktopLogin(DesktopLoginChallenge(verifier), "google")
	require.NoError(t, err)
	require.NoError(t, CancelDesktopLogin(id, verifier))
	require.Error(t, ApproveDesktopLogin(id, 42, g.DisplayCode, false))
	id, g, err = NewDesktopLogin(DesktopLoginChallenge(verifier), "github")
	require.NoError(t, err)
	require.NoError(t, DB.Model(g).Update("expires_at", time.Now().Unix()-1).Error)
	_, err = DesktopLoginWithProof(id, verifier)
	require.Error(t, err)
	_, _, err = NewDesktopLogin("invalid", "github")
	require.Error(t, err)
	_, _, err = NewDesktopLogin(DesktopLoginChallenge(verifier), "attacker")
	require.Error(t, err)
}
