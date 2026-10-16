package model

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

const LogExportMaxRows = 10000

func applyUserLogFilters(tx *gorm.DB, userID int, filters AdminLogQueryFilters) (*gorm.DB, error) {
	// Only fields accepted by the self-service list are allowed here.
	return applyAdminLogFilters(tx.Where("logs.user_id = ?", userID), AdminLogQueryFilters{
		LogType: filters.LogType, StartTimestamp: filters.StartTimestamp, EndTimestamp: filters.EndTimestamp,
		ModelName: filters.ModelName, TokenName: filters.TokenName, Group: filters.Group, RequestID: filters.RequestID,
	}, false)
}

// WalkExportLogs shares list filters and redaction. ID cursors avoid offset drift
// when new logs arrive; records added after the first batch are excluded.
func WalkExportLogs(ctx context.Context, filters AdminLogQueryFilters, userID int, admin bool, write func(*Log) error) error {
	if !admin && userID <= 0 {
		return errors.New("无效的用户")
	}
	var cursor, count int
	for {
		tx := LOG_DB.WithContext(ctx).Model(&Log{})
		var err error
		if admin {
			tx, err = applyAdminLogFilters(tx, filters, true)
		} else {
			tx, err = applyUserLogFilters(tx, userID, filters)
		}
		if err != nil {
			return err
		}
		if cursor > 0 {
			tx = tx.Where("logs.id < ?", cursor)
		}
		var rows []*Log
		if err := tx.Order("logs.id desc").Limit(250).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		cursor = rows[len(rows)-1].Id
		if count+len(rows) > LogExportMaxRows {
			return errors.New("导出超过 10000 条，请缩小筛选范围")
		}
		if !admin {
			formatUserLogs(rows, count)
		}
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := write(row); err != nil {
				return err
			}
		}
		count += len(rows)
		if len(rows) < 250 {
			return nil
		}
	}
}
