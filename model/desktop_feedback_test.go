package model

import (
	"bytes"
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDesktopFeedbackConcurrentIdempotencyAndLogging(t *testing.T) {
	var output bytes.Buffer
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.New(log.New(&output, "", 0), logger.Config{LogLevel: logger.Info})})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	old := DB
	DB = db
	t.Cleanup(func() { DB = old; require.NoError(t, conn.Close()) })
	require.NoError(t, db.AutoMigrate(&DesktopFeedback{}))
	output.Reset()
	ctx := context.Background()
	var group sync.WaitGroup
	results := make(chan error, 20)
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := CreateDesktopFeedback(ctx, &DesktopFeedback{UserID: 7, ReportID: "a7149591-4139-4c52-a0a2-1fa4ae45958b", Description: "private-description", Errors: "[]", Status: DesktopFeedbackPending})
			results <- err
		}()
	}
	group.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	rows, total, err := ListDesktopFeedback(ctx, DesktopFeedbackFilter{UserID: 7}, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	_, err = CreateDesktopFeedback(ctx, &DesktopFeedback{UserID: 8, ReportID: rows[0].ReportID, Description: "another-private-description", Errors: "[]", Status: DesktopFeedbackPending})
	require.NoError(t, err)
	require.Empty(t, output.String())
	require.NoError(t, db.Migrator().DropTable(&DesktopFeedback{}))
	output.Reset()
	_, err = CreateDesktopFeedback(ctx, &DesktopFeedback{UserID: 7, ReportID: "failure", Description: "must-not-log"})
	require.Error(t, err)
	require.Empty(t, output.String())
}

func TestDesktopFeedbackUpsertDialects(t *testing.T) {
	base, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	conn, err := base.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	for _, dialect := range []gorm.Dialector{
		mysql.New(mysql.Config{Conn: conn, SkipInitializeWithVersion: true}),
		postgres.New(postgres.Config{Conn: conn}),
	} {
		t.Run(dialect.Name(), func(t *testing.T) {
			db, err := gorm.Open(dialect, &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
			require.NoError(t, err)
			statementSchema := &gorm.Statement{DB: db}
			require.NoError(t, statementSchema.Parse(&DesktopFeedback{}))
			errorField := statementSchema.Schema.LookUpField("Errors")
			if dialect.Name() == "mysql" {
				require.Equal(t, "mediumtext", dialect.DataTypeOf(errorField))
			} else {
				require.Equal(t, "varchar(131072)", dialect.DataTypeOf(errorField))
			}
			var statement string
			require.NoError(t, db.Callback().Create().After("gorm:create").Register("capture-feedback", func(tx *gorm.DB) { statement = tx.Statement.SQL.String() }))
			old := DB
			DB = db
			defer func() { DB = old }()
			_, _ = CreateDesktopFeedback(context.Background(), &DesktopFeedback{UserID: 7, ReportID: "report", CreatedAt: time.Now().Unix()})
			if dialect.Name() == "mysql" {
				require.Contains(t, statement, "ON DUPLICATE KEY UPDATE")
			} else {
				require.Contains(t, statement, `ON CONFLICT ("user_id","report_id") DO NOTHING`)
			}
		})
	}
}
