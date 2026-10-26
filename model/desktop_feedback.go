package model

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const (
	DesktopFeedbackPending    = "pending"
	DesktopFeedbackInProgress = "in_progress"
	DesktopFeedbackResolved   = "resolved"
)

type DesktopFeedback struct {
	ID          int64  `gorm:"primaryKey"`
	UserID      int    `gorm:"not null;uniqueIndex:idx_desktop_feedback_owner_report;index"`
	ReportID    string `gorm:"size:36;not null;uniqueIndex:idx_desktop_feedback_owner_report"`
	Description string `gorm:"type:text;not null"`
	AppVersion  string `gorm:"size:32;not null;index"`
	Platform    string `gorm:"size:16;not null;index"`
	View        string `gorm:"size:64;not null"`
	Tool        string `gorm:"size:64;not null"`
	Errors      string `gorm:"size:131072;not null"`
	Status      string `gorm:"size:16;not null;index"`
	CreatedAt   int64  `gorm:"not null;index"`
	UpdatedAt   int64  `gorm:"not null"`
	UpdatedBy   int    `gorm:"not null;default:0"`
}

func ValidDesktopFeedbackStatus(status string) bool {
	return status == DesktopFeedbackPending || status == DesktopFeedbackInProgress || status == DesktopFeedbackResolved
}

func desktopFeedbackDB(ctx context.Context) *gorm.DB {
	// Even redacted prose can contain private information. Keep bound INSERT
	// values out of GORM error/slow-query logs as well as controller logs.
	return DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).WithContext(ctx)
}

// The unique pair is the idempotency boundary. A retry never overwrites the original report.
func CreateDesktopFeedback(ctx context.Context, row *DesktopFeedback) (*DesktopFeedback, error) {
	if err := desktopFeedbackDB(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "report_id"}}, DoNothing: true,
	}).Create(row).Error; err != nil {
		return nil, err
	}
	var stored DesktopFeedback
	err := desktopFeedbackDB(ctx).Where("user_id = ? AND report_id = ?", row.UserID, row.ReportID).First(&stored).Error
	return &stored, err
}

type DesktopFeedbackFilter struct {
	Status     string
	Platform   string
	AppVersion string
	UserID     int
}

func desktopFeedbackQuery(ctx context.Context, filter DesktopFeedbackFilter) *gorm.DB {
	query := desktopFeedbackDB(ctx).Model(&DesktopFeedback{})
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Platform != "" {
		query = query.Where("platform = ?", filter.Platform)
	}
	if filter.AppVersion != "" {
		query = query.Where("app_version = ?", filter.AppVersion)
	}
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	return query
}

func ListDesktopFeedback(ctx context.Context, filter DesktopFeedbackFilter, offset, limit int) ([]DesktopFeedback, int64, error) {
	var total int64
	rows := make([]DesktopFeedback, 0)
	query := desktopFeedbackQuery(ctx, filter)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}

func GetDesktopFeedback(ctx context.Context, id int64) (*DesktopFeedback, error) {
	var row DesktopFeedback
	err := desktopFeedbackDB(ctx).First(&row, id).Error
	return &row, err
}

func UpdateDesktopFeedbackStatus(ctx context.Context, id int64, status string, actor int) (*DesktopFeedback, error) {
	if !ValidDesktopFeedbackStatus(status) {
		return nil, gorm.ErrInvalidData
	}
	result := desktopFeedbackDB(ctx).Model(&DesktopFeedback{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status": status, "updated_at": time.Now().Unix(), "updated_by": actor,
	})
	if result.Error != nil {
		return nil, result.Error
	}
	// MySQL may report no changed rows for a repeated update; read by ID to distinguish absence.
	return GetDesktopFeedback(ctx, id)
}
