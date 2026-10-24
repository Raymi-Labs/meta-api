package model

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var commonGroupCol string
var commonKeyCol string
var commonTrueVal string
var commonFalseVal string

var logKeyCol string
var logGroupCol string

func init() {
	initCol()
}

func initCol() {
	// init common column names
	if common.UsingPostgreSQL {
		commonGroupCol = `"group"`
		commonKeyCol = `"key"`
		commonTrueVal = "true"
		commonFalseVal = "false"
	} else {
		commonGroupCol = "`group`"
		commonKeyCol = "`key`"
		commonTrueVal = "1"
		commonFalseVal = "0"
	}
	if os.Getenv("LOG_SQL_DSN") != "" {
		switch common.LogSqlType {
		case common.DatabaseTypePostgreSQL:
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		default:
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	} else {
		// LOG_SQL_DSN 为空时，日志数据库与主数据库相同
		if common.UsingPostgreSQL {
			logGroupCol = `"group"`
			logKeyCol = `"key"`
		} else {
			logGroupCol = commonGroupCol
			logKeyCol = commonKeyCol
		}
	}
	// log sql type and database type
	//common.SysLog("Using Log SQL Type: " + common.LogSqlType)
}

var DB *gorm.DB

var LOG_DB *gorm.DB

func createRootAccountIfNeed() error {
	var user User
	//if user.Status != common.UserStatusEnabled {
	if err := DB.First(&user).Error; err != nil {
		common.SysLog("no user exists, create a root user for you: username is root, password is 123456")
		hashedPassword, err := common.Password2Hash("123456")
		if err != nil {
			return err
		}
		rootUser := User{
			Username:    "root",
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		DB.Create(&rootUser)
	}
	return nil
}

func CheckSetup() {
	setup := GetSetup()
	if setup == nil {
		// No setup record exists, check if we have a root user
		if RootUserExists() {
			common.SysLog("system is not initialized, but root user exists")
			// Create setup record
			newSetup := Setup{
				Version:       common.Version,
				InitializedAt: time.Now().Unix(),
			}
			err := DB.Create(&newSetup).Error
			if err != nil {
				common.SysLog("failed to create setup record: " + err.Error())
			}
			constant.Setup = true
		} else {
			common.SysLog("system is not initialized and no root user exists")
			constant.Setup = false
		}
	} else {
		// Setup record exists, system is initialized
		common.SysLog("system is already initialized at: " + time.Unix(setup.InitializedAt, 0).String())
		constant.Setup = true
	}
}

func chooseDB(envName string, isLog bool) (*gorm.DB, error) {
	defer func() {
		initCol()
	}()
	dsn := os.Getenv(envName)
	if dsn != "" {
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			// Use PostgreSQL
			common.SysLog("using PostgreSQL as database")
			if !isLog {
				common.UsingPostgreSQL = true
			} else {
				common.LogSqlType = common.DatabaseTypePostgreSQL
			}
			return gorm.Open(postgresMigrationDialector{postgres.Dialector{Config: &postgres.Config{
				DSN:                  dsn,
				PreferSimpleProtocol: true, // disables implicit prepared statement usage
			}}}, &gorm.Config{
				PrepareStmt: false, // named statements are unsafe with transaction-pooling proxies
			})
		}
		if strings.HasPrefix(dsn, "local") {
			common.SysLog("SQL_DSN not set, using SQLite as database")
			if !isLog {
				common.UsingSQLite = true
			} else {
				common.LogSqlType = common.DatabaseTypeSQLite
			}
			dsn, err := common.SQLiteDSN(common.SQLitePath)
			if err != nil {
				return nil, err
			}
			return gorm.Open(sqlite.Open(dsn), &gorm.Config{
				PrepareStmt: true, // precompile SQL
			})
		}
		// Use MySQL
		common.SysLog("using MySQL as database")
		// check parseTime
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		if !isLog {
			common.UsingMySQL = true
		} else {
			common.LogSqlType = common.DatabaseTypeMySQL
		}
		return gorm.Open(mysqlMigrationDialector{mysql.Dialector{Config: &mysql.Config{DSN: dsn}}}, &gorm.Config{
			PrepareStmt: true, // precompile SQL
		})
	}
	// Use SQLite
	common.SysLog("SQL_DSN not set, using SQLite as database")
	common.UsingSQLite = true
	sqliteDSN, err := common.SQLiteDSN(common.SQLitePath)
	if err != nil {
		return nil, err
	}
	return gorm.Open(sqlite.Open(sqliteDSN), &gorm.Config{
		PrepareStmt: true, // precompile SQL
	})
}

func InitDB() (err error) {
	db, err := chooseDB("SQL_DSN", false)
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		DB = db
		// MySQL charset/collation startup check: ensure Chinese-capable charset
		if common.UsingMySQL {
			if err := checkMySQLChineseSupport(DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		if common.UsingMySQL {
			//_, _ = sqlDB.Exec("ALTER TABLE channels MODIFY model_mapping TEXT;") // TODO: delete this line when most users have upgraded
		}
		common.SysLog("database migration started")
		err = migrateDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func InitLogDB() (err error) {
	if os.Getenv("LOG_SQL_DSN") == "" {
		LOG_DB = DB
		return
	}
	db, err := chooseDB("LOG_SQL_DSN", true)
	if err == nil {
		if common.DebugEnabled {
			db = db.Debug()
		}
		LOG_DB = db
		// If log DB is MySQL, also ensure Chinese-capable charset
		if common.LogSqlType == common.DatabaseTypeMySQL {
			if err := checkMySQLChineseSupport(LOG_DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := LOG_DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		common.SysLog("database migration started")
		err = migrateLOGDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func migrateDB() error {
	if err := migratePrefillGroupUniqueness(DB); err != nil {
		return err
	}
	// Migrate price_amount column from float/double to decimal for existing tables
	migrateSubscriptionPlanPriceAmount()
	err := DB.AutoMigrate(
		&Channel{},
		&Token{},
		&ChannelTagPolicy{},
		&ChannelQuotaUsage{},
		&QuotaNoticeAccount{},
		&QuotaNoticeSignal{},
		&QuotaNotice{},
		&User{},
		&PasskeyCredential{},
		&VerificationProof{},
		&Option{},
		&Redemption{},
		&RedemptionReviewCase{},
		&WalletTransferLock{},
		&SellableTokenProduct{},
		&SellableTokenOrder{},
		&SellableTokenIssuance{},
		&Ability{},
		&Log{},
		&PulseBenefitLogReceipt{},
		&Midjourney{},
		&TopUp{},
		&PaymentRiskCase{},
		&BenefitChangeRecord{},
		&PulseBenefitReceipt{},
		&PulseBenefitQuotaCounter{},
		&PulseFundingLedger{},
		&PulseWalletReservation{},
		&BenefitRollbackOperation{},
		&QuotaData{},
		&UserBalanceSnapshot{},
		&Task{},
		&Model{},
		&ModelPermission{},
		&Vendor{},
		&UpstreamAccount{},
		&UpstreamAccountSnapshot{},
		&PerfMetric{},
		&GroupHealthMetric{},
		&PrefillGroup{},
		&Setup{},
		&TwoFA{},
		&TwoFABackupCode{},
		&Checkin{},
		&SubscriptionOrder{},
		&SubscriptionIssuance{},
		&UserSubscription{},
		&SubscriptionPreConsumeRecord{},
		&InviteCommissionLedger{},
		&InvitePromotionClick{},
		&InviteCommissionDailyCapState{},
		&AffWithdrawal{},
		&InvoiceRequest{},
		&InvoiceRequestItem{},
		&InvoiceRequestProductItem{},
		&CustomOAuthProvider{},
		&UserOAuthBinding{},
		&CRSSite{},
		&CRSAccountSnapshot{},
		&CPASite{},
		&IPBlacklist{},
		&ContentSafetyViolation{},
		&ContentSafetyReviewCase{},
		&ContentSafetyEvidence{},
		&ContentSafetyNotification{},
	)
	if err != nil {
		return err
	}
	if common.UsingSQLite {
		if err := ensureTokenColumnsSQLite(); err != nil {
			return err
		}
		if err := ensureRedemptionColumnsSQLite(); err != nil {
			return err
		}
		if err := ensureSubscriptionPlanTableSQLite(); err != nil {
			return err
		}
		if err := ensureSubscriptionOrderColumnsSQLite(); err != nil {
			return err
		}
		if err := ensureSellableTokenOrderColumnsSQLite(); err != nil {
			return err
		}
		if err := ensureUpstreamAccountColumnsSQLite(); err != nil {
			return err
		}
	} else {
		if err := DB.AutoMigrate(&SubscriptionPlan{}); err != nil {
			return err
		}
	}
	if err := NormalizeMiMoVendorMetadata(); err != nil {
		return fmt.Errorf("failed to normalize Xiaomi MiMo vendor metadata: %w", err)
	}
	if err := MigrateWalletRedemptionTransferPolicy(); err != nil {
		return fmt.Errorf("failed to migrate wallet redemption transfer policy: %w", err)
	}
	return nil
}

func migrateDBFast() error {
	if err := migratePrefillGroupUniqueness(DB); err != nil {
		return err
	}
	var wg sync.WaitGroup

	migrations := []struct {
		model interface{}
		name  string
	}{
		{&Channel{}, "Channel"},
		{&Token{}, "Token"},
		{&ChannelTagPolicy{}, "ChannelTagPolicy"},
		{&ChannelQuotaUsage{}, "ChannelQuotaUsage"},
		{&QuotaNoticeAccount{}, "QuotaNoticeAccount"},
		{&QuotaNoticeSignal{}, "QuotaNoticeSignal"},
		{&QuotaNotice{}, "QuotaNotice"},
		{&User{}, "User"},
		{&PasskeyCredential{}, "PasskeyCredential"},
		{&VerificationProof{}, "VerificationProof"},
		{&Option{}, "Option"},
		{&Redemption{}, "Redemption"},
		{&RedemptionReviewCase{}, "RedemptionReviewCase"},
		{&WalletTransferLock{}, "WalletTransferLock"},
		{&SellableTokenProduct{}, "SellableTokenProduct"},
		{&SellableTokenOrder{}, "SellableTokenOrder"},
		{&SellableTokenIssuance{}, "SellableTokenIssuance"},
		{&Ability{}, "Ability"},
		{&Log{}, "Log"},
		{&PulseBenefitLogReceipt{}, "PulseBenefitLogReceipt"},
		{&Midjourney{}, "Midjourney"},
		{&TopUp{}, "TopUp"},
		{&PaymentRiskCase{}, "PaymentRiskCase"},
		{&BenefitChangeRecord{}, "BenefitChangeRecord"},
		{&PulseBenefitReceipt{}, "PulseBenefitReceipt"},
		{&PulseBenefitQuotaCounter{}, "PulseBenefitQuotaCounter"},
		{&PulseFundingLedger{}, "PulseFundingLedger"},
		{&PulseWalletReservation{}, "PulseWalletReservation"},
		{&BenefitRollbackOperation{}, "BenefitRollbackOperation"},
		{&QuotaData{}, "QuotaData"},
		{&UserBalanceSnapshot{}, "UserBalanceSnapshot"},
		{&Task{}, "Task"},
		{&Model{}, "Model"},
		{&ModelPermission{}, "ModelPermission"},
		{&Vendor{}, "Vendor"},
		{&UpstreamAccount{}, "UpstreamAccount"},
		{&UpstreamAccountSnapshot{}, "UpstreamAccountSnapshot"},
		{&PerfMetric{}, "PerfMetric"},
		{&GroupHealthMetric{}, "GroupHealthMetric"},
		{&PrefillGroup{}, "PrefillGroup"},
		{&Setup{}, "Setup"},
		{&TwoFA{}, "TwoFA"},
		{&TwoFABackupCode{}, "TwoFABackupCode"},
		{&Checkin{}, "Checkin"},
		{&SubscriptionOrder{}, "SubscriptionOrder"},
		{&SubscriptionIssuance{}, "SubscriptionIssuance"},
		{&UserSubscription{}, "UserSubscription"},
		{&SubscriptionPreConsumeRecord{}, "SubscriptionPreConsumeRecord"},
		{&InviteCommissionLedger{}, "InviteCommissionLedger"},
		{&InvitePromotionClick{}, "InvitePromotionClick"},
		{&InviteCommissionDailyCapState{}, "InviteCommissionDailyCapState"},
		{&AffWithdrawal{}, "AffWithdrawal"},
		{&InvoiceRequest{}, "InvoiceRequest"},
		{&InvoiceRequestItem{}, "InvoiceRequestItem"},
		{&InvoiceRequestProductItem{}, "InvoiceRequestProductItem"},
		{&CustomOAuthProvider{}, "CustomOAuthProvider"},
		{&UserOAuthBinding{}, "UserOAuthBinding"},
		{&CRSSite{}, "CRSSite"},
		{&CRSAccountSnapshot{}, "CRSAccountSnapshot"},
		{&CPASite{}, "CPASite"},
		{&IPBlacklist{}, "IPBlacklist"},
		{&ContentSafetyViolation{}, "ContentSafetyViolation"},
		{&ContentSafetyReviewCase{}, "ContentSafetyReviewCase"},
	}
	// 动态计算migration数量，确保errChan缓冲区足够大
	errChan := make(chan error, len(migrations))

	for _, m := range migrations {
		wg.Add(1)
		go func(model interface{}, name string) {
			defer wg.Done()
			if err := DB.AutoMigrate(model); err != nil {
				errChan <- fmt.Errorf("failed to migrate %s: %v", name, err)
			}
		}(m.model, m.name)
	}

	// Wait for all migrations to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	if common.UsingSQLite {
		if err := ensureTokenColumnsSQLite(); err != nil {
			return err
		}
		if err := ensureRedemptionColumnsSQLite(); err != nil {
			return err
		}
		if err := ensureSubscriptionPlanTableSQLite(); err != nil {
			return err
		}
		if err := ensureSubscriptionOrderColumnsSQLite(); err != nil {
			return err
		}
		if err := ensureSellableTokenOrderColumnsSQLite(); err != nil {
			return err
		}
		if err := ensureUpstreamAccountColumnsSQLite(); err != nil {
			return err
		}
	} else {
		if err := DB.AutoMigrate(&SubscriptionPlan{}); err != nil {
			return err
		}
	}
	if err := NormalizeMiMoVendorMetadata(); err != nil {
		return fmt.Errorf("failed to normalize Xiaomi MiMo vendor metadata: %w", err)
	}
	if err := MigrateWalletRedemptionTransferPolicy(); err != nil {
		return fmt.Errorf("failed to migrate wallet redemption transfer policy: %w", err)
	}
	common.SysLog("database migrated")
	return nil
}

func migrateLOGDB() error {
	var err error
	if err = LOG_DB.AutoMigrate(&Log{}, &PulseBenefitLogReceipt{}); err != nil {
		return err
	}
	return nil
}

type sqliteColumnDef struct {
	Name string
	DDL  string
}

func ensureSubscriptionPlanTableSQLite() error {
	if !common.UsingSQLite {
		return nil
	}
	tableName := "subscription_plans"
	if !DB.Migrator().HasTable(tableName) {
		createSQL := `CREATE TABLE ` + "`" + tableName + "`" + ` (
` + "`id`" + ` integer,
` + "`title`" + ` varchar(128) NOT NULL,
` + "`subtitle`" + ` varchar(255) DEFAULT '',
` + "`price_amount`" + ` decimal(10,6) NOT NULL,
` + "`currency`" + ` varchar(8) NOT NULL DEFAULT 'USD',
` + "`duration_unit`" + ` varchar(16) NOT NULL DEFAULT 'month',
` + "`duration_value`" + ` integer NOT NULL DEFAULT 1,
` + "`custom_seconds`" + ` bigint NOT NULL DEFAULT 0,
` + "`enabled`" + ` numeric DEFAULT 1,
` + "`sort_order`" + ` integer DEFAULT 0,
` + "`stripe_price_id`" + ` varchar(128) DEFAULT '',
` + "`creem_product_id`" + ` varchar(128) DEFAULT '',
` + "`max_purchase_per_user`" + ` integer DEFAULT 0,
` + "`max_stack_per_user`" + ` integer DEFAULT 0,
` + "`purchase_quantity_min`" + ` integer DEFAULT 1,
` + "`purchase_quantity_max`" + ` integer DEFAULT 12,
` + "`upgrade_group`" + ` varchar(64) DEFAULT '',
` + "`total_amount`" + ` bigint NOT NULL DEFAULT 0,
` + "`quota_reset_period`" + ` varchar(16) DEFAULT 'never',
` + "`quota_reset_custom_seconds`" + ` bigint DEFAULT 0,
` + "`created_at`" + ` bigint,
` + "`updated_at`" + ` bigint,
PRIMARY KEY (` + "`id`" + `)
)`
		return DB.Exec(createSQL).Error
	}
	var cols []struct {
		Name string `gorm:"column:name"`
	}
	if err := DB.Raw("PRAGMA table_info(`" + tableName + "`)").Scan(&cols).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		existing[c.Name] = struct{}{}
	}
	required := []sqliteColumnDef{
		{Name: "title", DDL: "`title` varchar(128) NOT NULL"},
		{Name: "subtitle", DDL: "`subtitle` varchar(255) DEFAULT ''"},
		{Name: "price_amount", DDL: "`price_amount` decimal(10,6) NOT NULL"},
		{Name: "currency", DDL: "`currency` varchar(8) NOT NULL DEFAULT 'USD'"},
		{Name: "duration_unit", DDL: "`duration_unit` varchar(16) NOT NULL DEFAULT 'month'"},
		{Name: "duration_value", DDL: "`duration_value` integer NOT NULL DEFAULT 1"},
		{Name: "custom_seconds", DDL: "`custom_seconds` bigint NOT NULL DEFAULT 0"},
		{Name: "enabled", DDL: "`enabled` numeric DEFAULT 1"},
		{Name: "sort_order", DDL: "`sort_order` integer DEFAULT 0"},
		{Name: "stripe_price_id", DDL: "`stripe_price_id` varchar(128) DEFAULT ''"},
		{Name: "creem_product_id", DDL: "`creem_product_id` varchar(128) DEFAULT ''"},
		{Name: "max_purchase_per_user", DDL: "`max_purchase_per_user` integer DEFAULT 0"},
		{Name: "max_stack_per_user", DDL: "`max_stack_per_user` integer DEFAULT 0"},
		{Name: "purchase_quantity_min", DDL: "`purchase_quantity_min` integer DEFAULT 1"},
		{Name: "purchase_quantity_max", DDL: "`purchase_quantity_max` integer DEFAULT 12"},
		{Name: "upgrade_group", DDL: "`upgrade_group` varchar(64) DEFAULT ''"},
		{Name: "total_amount", DDL: "`total_amount` bigint NOT NULL DEFAULT 0"},
		{Name: "quota_reset_period", DDL: "`quota_reset_period` varchar(16) DEFAULT 'never'"},
		{Name: "quota_reset_custom_seconds", DDL: "`quota_reset_custom_seconds` bigint DEFAULT 0"},
		{Name: "created_at", DDL: "`created_at` bigint"},
		{Name: "updated_at", DDL: "`updated_at` bigint"},
	}
	for _, col := range required {
		if _, ok := existing[col.Name]; ok {
			continue
		}
		if err := DB.Exec("ALTER TABLE `" + tableName + "` ADD COLUMN " + col.DDL).Error; err != nil {
			return err
		}
	}
	return nil
}

// ensureSubscriptionOrderColumnsSQLite 为 SQLite 旧库补齐新增字段。
func ensureSubscriptionOrderColumnsSQLite() error {
	if !common.UsingSQLite {
		return nil
	}
	tableName := "subscription_orders"
	if !DB.Migrator().HasTable(tableName) {
		return nil
	}
	var cols []struct {
		Name string `gorm:"column:name"`
	}
	if err := DB.Raw("PRAGMA table_info(`" + tableName + "`)").Scan(&cols).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		existing[c.Name] = struct{}{}
	}
	required := []sqliteColumnDef{
		{Name: "purchase_mode", DDL: "`purchase_mode` varchar(16) NOT NULL DEFAULT 'stack'"},
		{Name: "renew_target_subscription_id", DDL: "`renew_target_subscription_id` integer DEFAULT 0"},
		{Name: "purchase_quantity", DDL: "`purchase_quantity` integer NOT NULL DEFAULT 1"},
	}
	for _, col := range required {
		if _, ok := existing[col.Name]; ok {
			continue
		}
		if err := DB.Exec("ALTER TABLE `" + tableName + "` ADD COLUMN " + col.DDL).Error; err != nil {
			return err
		}
	}
	return nil
}

// ensureSellableTokenOrderColumnsSQLite 为 SQLite 旧库补齐可售令牌订单字段。
func ensureSellableTokenOrderColumnsSQLite() error {
	if !common.UsingSQLite {
		return nil
	}
	tableName := "sellable_token_orders"
	if !DB.Migrator().HasTable(tableName) {
		return nil
	}
	var cols []struct {
		Name string `gorm:"column:name"`
	}
	if err := DB.Raw("PRAGMA table_info(`" + tableName + "`)").Scan(&cols).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		existing[c.Name] = struct{}{}
	}
	required := []sqliteColumnDef{
		{Name: "trade_no", DDL: "`trade_no` varchar(255) DEFAULT ''"},
	}
	for _, col := range required {
		if _, ok := existing[col.Name]; ok {
			continue
		}
		if err := DB.Exec("ALTER TABLE `" + tableName + "` ADD COLUMN " + col.DDL).Error; err != nil {
			return err
		}
	}
	return nil
}

// ensureTokenColumnsSQLite 为 SQLite 旧库补齐令牌套餐字段。
func ensureTokenColumnsSQLite() error {
	if !common.UsingSQLite {
		return nil
	}
	tableName := "tokens"
	if !DB.Migrator().HasTable(tableName) {
		return nil
	}
	var cols []struct {
		Name string `gorm:"column:name"`
	}
	if err := DB.Raw("PRAGMA table_info(`" + tableName + "`)").Scan(&cols).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		existing[c.Name] = struct{}{}
	}
	required := []sqliteColumnDef{
		{Name: "source_type", DDL: "`source_type` varchar(32) DEFAULT ''"},
		{Name: "billing_mode", DDL: "`billing_mode` varchar(32) DEFAULT ''"},
		{Name: "sellable_token_product_id", DDL: "`sellable_token_product_id` integer DEFAULT 0"},
		{Name: "sellable_token_issuance_id", DDL: "`sellable_token_issuance_id` integer DEFAULT 0"},
		{Name: "channel_limits_enabled", DDL: "`channel_limits_enabled` numeric DEFAULT 0"},
		{Name: "channel_limits", DDL: "`channel_limits` text DEFAULT ''"},
		{Name: "max_concurrency", DDL: "`max_concurrency` integer DEFAULT 0"},
		{Name: "window_request_limit", DDL: "`window_request_limit` integer DEFAULT 0"},
		{Name: "window_seconds", DDL: "`window_seconds` bigint DEFAULT 0"},
		{Name: "package_enabled", DDL: "`package_enabled` numeric DEFAULT 0"},
		{Name: "package_limit_quota", DDL: "`package_limit_quota` integer DEFAULT 0"},
		{Name: "package_period", DDL: "`package_period` varchar(16) DEFAULT 'none'"},
		{Name: "package_custom_seconds", DDL: "`package_custom_seconds` bigint DEFAULT 0"},
		{Name: "package_used_quota", DDL: "`package_used_quota` integer DEFAULT 0"},
		{Name: "package_next_reset_time", DDL: "`package_next_reset_time` bigint DEFAULT 0"},
		{Name: "package_period_mode", DDL: "`package_period_mode` varchar(16) DEFAULT 'relative'"},
	}
	for _, col := range required {
		if _, ok := existing[col.Name]; ok {
			continue
		}
		if err := DB.Exec("ALTER TABLE `" + tableName + "` ADD COLUMN " + col.DDL).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureRedemptionColumnsSQLite() error {
	if !common.UsingSQLite {
		return nil
	}
	tableName := "redemptions"
	if !DB.Migrator().HasTable(tableName) {
		return nil
	}
	var cols []struct {
		Name string `gorm:"column:name"`
	}
	if err := DB.Raw("PRAGMA table_info(`" + tableName + "`)").Scan(&cols).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		existing[c.Name] = struct{}{}
	}
	required := []sqliteColumnDef{
		{Name: "benefit_type", DDL: "`benefit_type` varchar(32) NOT NULL DEFAULT 'quota'"},
		{Name: "sellable_token_product_id", DDL: "`sellable_token_product_id` integer DEFAULT 0"},
		{Name: "funding_source", DDL: "`funding_source` varchar(16) NOT NULL DEFAULT 'admin'"},
		{Name: "transferable_quota", DDL: "`transferable_quota` integer NOT NULL DEFAULT 0"},
		{Name: "create_request_id", DDL: "`create_request_id` varchar(64)"},
	}
	for _, col := range required {
		if _, ok := existing[col.Name]; ok {
			continue
		}
		if err := DB.Exec("ALTER TABLE `" + tableName + "` ADD COLUMN " + col.DDL).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureUpstreamAccountColumnsSQLite() error {
	if !common.UsingSQLite {
		return nil
	}
	tableName := "profit_board_upstream_accounts"
	if !DB.Migrator().HasTable(tableName) {
		return nil
	}
	var cols []struct {
		Name string `gorm:"column:name"`
	}
	if err := DB.Raw("PRAGMA table_info(`" + tableName + "`)").Scan(&cols).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		existing[c.Name] = struct{}{}
	}
	required := []sqliteColumnDef{
		{Name: "resource_display_mode", DDL: "`resource_display_mode` varchar(24) DEFAULT 'both'"},
		{Name: "email", DDL: "`email` varchar(255)"},
		{Name: "password_encrypted", DDL: "`password_encrypted` text"},
	}
	for _, col := range required {
		if _, ok := existing[col.Name]; ok {
			continue
		}
		if err := DB.Exec("ALTER TABLE `" + tableName + "` ADD COLUMN " + col.DDL).Error; err != nil {
			return err
		}
	}
	return nil
}

// migrateSubscriptionPlanPriceAmount migrates price_amount column from float/double to decimal(10,6)
// This is safe to run multiple times - it checks the column type first
func migrateSubscriptionPlanPriceAmount() {
	// SQLite doesn't support ALTER COLUMN, and its type affinity handles this automatically
	// Skip early to avoid GORM parsing the existing table DDL which may cause issues
	if common.UsingSQLite {
		return
	}

	tableName := "subscription_plans"
	columnName := "price_amount"

	// Check if table exists first
	if !DB.Migrator().HasTable(tableName) {
		return
	}

	// Check if column exists
	if !DB.Migrator().HasColumn(&SubscriptionPlan{}, columnName) {
		return
	}

	var alterSQL string
	if common.UsingPostgreSQL {
		// PostgreSQL: Check if already decimal/numeric
		var dataType string
		DB.Raw(`SELECT data_type FROM information_schema.columns 
			WHERE table_name = ? AND column_name = ?`, tableName, columnName).Scan(&dataType)
		if dataType == "numeric" {
			return // Already decimal/numeric
		}
		alterSQL = fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE decimal(10,6) USING %s::decimal(10,6)`,
			tableName, columnName, columnName)
	} else if common.UsingMySQL {
		// MySQL: Check if already decimal
		var columnType string
		DB.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns 
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&columnType)
		if strings.HasPrefix(strings.ToLower(columnType), "decimal") {
			return // Already decimal
		}
		alterSQL = fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s decimal(10,6) NOT NULL DEFAULT 0",
			tableName, columnName)
	} else {
		return
	}

	if alterSQL != "" {
		if err := DB.Exec(alterSQL).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to migrate %s.%s to decimal: %v", tableName, columnName, err))
		} else {
			common.SysLog(fmt.Sprintf("Successfully migrated %s.%s to decimal(10,6)", tableName, columnName))
		}
	}
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	return err
}

func CloseDB() error {
	if LOG_DB != DB {
		err := closeDB(LOG_DB)
		if err != nil {
			return err
		}
	}
	return closeDB(DB)
}

// checkMySQLChineseSupport ensures the MySQL connection and current schema
// default charset/collation can store Chinese characters. It allows common
// Chinese-capable charsets (utf8mb4, utf8, gbk, big5, gb18030) and panics otherwise.
func checkMySQLChineseSupport(db *gorm.DB) error {
	// 仅检测：当前库默认字符集/排序规则 + 各表的排序规则（隐含字符集）

	// Read current schema defaults
	var schemaCharset, schemaCollation string
	err := db.Raw("SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = DATABASE()").Row().Scan(&schemaCharset, &schemaCollation)
	if err != nil {
		return fmt.Errorf("读取当前库默认字符集/排序规则失败 / Failed to read schema default charset/collation: %v", err)
	}

	toLower := func(s string) string { return strings.ToLower(s) }
	// Allowed charsets that can store Chinese text
	allowedCharsets := map[string]string{
		"utf8mb4": "utf8mb4_",
		"utf8":    "utf8_",
		"gbk":     "gbk_",
		"big5":    "big5_",
		"gb18030": "gb18030_",
	}
	isChineseCapable := func(cs, cl string) bool {
		csLower := toLower(cs)
		clLower := toLower(cl)
		if prefix, ok := allowedCharsets[csLower]; ok {
			if clLower == "" {
				return true
			}
			return strings.HasPrefix(clLower, prefix)
		}
		// 如果仅提供了排序规则，尝试按排序规则前缀判断
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(clLower, prefix) {
				return true
			}
		}
		return false
	}

	// 1) 当前库默认值必须支持中文
	if !isChineseCapable(schemaCharset, schemaCollation) {
		return fmt.Errorf("当前库默认字符集/排序规则不支持中文：schema(%s/%s)。请将库设置为 utf8mb4/utf8/gbk/big5/gb18030 / Schema default charset/collation is not Chinese-capable: schema(%s/%s). Please set to utf8mb4/utf8/gbk/big5/gb18030",
			schemaCharset, schemaCollation, schemaCharset, schemaCollation)
	}

	// 2) 所有物理表的排序规则（隐含字符集）必须支持中文
	type tableInfo struct {
		Name      string
		Collation *string
	}
	var tables []tableInfo
	if err := db.Raw("SELECT TABLE_NAME, TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'").Scan(&tables).Error; err != nil {
		return fmt.Errorf("读取表排序规则失败 / Failed to read table collations: %v", err)
	}

	var badTables []string
	for _, t := range tables {
		// NULL 或空表示继承库默认设置，已在上面校验库默认，视为通过
		if t.Collation == nil || *t.Collation == "" {
			continue
		}
		cl := *t.Collation
		// 仅凭排序规则判断是否中文可用
		ok := false
		lower := strings.ToLower(cl)
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(lower, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			badTables = append(badTables, fmt.Sprintf("%s(%s)", t.Name, cl))
		}
	}

	if len(badTables) > 0 {
		// 限制输出数量以避免日志过长
		maxShow := 20
		shown := badTables
		if len(shown) > maxShow {
			shown = shown[:maxShow]
		}
		return fmt.Errorf(
			"存在不支持中文的表，请修复其排序规则/字符集。示例（最多展示 %d 项）：%v / Found tables not Chinese-capable. Please fix their collation/charset. Examples (showing up to %d): %v",
			maxShow, shown, maxShow, shown,
		)
	}
	return nil
}

var (
	lastPingTime time.Time
	pingMutex    sync.Mutex
)

func PingDB() error {
	pingMutex.Lock()
	defer pingMutex.Unlock()

	if time.Since(lastPingTime) < time.Second*10 {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Error getting sql.DB from GORM: %v", err)
		return err
	}

	err = sqlDB.Ping()
	if err != nil {
		log.Printf("Error pinging DB: %v", err)
		return err
	}

	lastPingTime = time.Now()
	common.SysLog("Database pinged successfully")
	return nil
}
