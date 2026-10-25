package model

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// InvitePromotionClick stores aggregate counts only; no visitor identifiers.
type InvitePromotionClick struct {
	UserID int    `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Day    string `gorm:"primaryKey;size:10" json:"day"`
	Source string `gorm:"primaryKey;size:16" json:"source"`
	Clicks int64  `gorm:"not null;default:0" json:"clicks"`
}

type InvitePromotionStats struct {
	From     string               `json:"from"`
	To       string               `json:"to"`
	Timezone string               `json:"timezone"`
	Clicks   int64                `json:"clicks"`
	Sources  map[string]int64     `json:"sources"`
	Daily    []InvitePromotionDay `json:"daily"`
}

type InvitePromotionDay struct {
	Day    string `json:"day"`
	Clicks int64  `json:"clicks"`
}

func NormalizeInvitePromotionSource(source string) string {
	switch source {
	case "text", "button", "banner":
		return source
	default:
		return "link"
	}
}

func RecordInvitePromotionClick(ctx context.Context, userID int, source string, now time.Time) error {
	row := InvitePromotionClick{UserID: userID, Day: now.UTC().Format("2006-01-02"), Source: NormalizeInvitePromotionSource(source), Clicks: 1}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "day"}, {Name: "source"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"clicks": gorm.Expr("? + ?", clause.Column{Table: clause.CurrentTable, Name: "clicks"}, 1)}),
	}).Create(&row).Error
}

func GetInvitePromotionStats(ctx context.Context, userID int, now time.Time) (*InvitePromotionStats, error) {
	today := now.UTC()
	start := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -29)
	stats := &InvitePromotionStats{From: start.Format("2006-01-02"), To: today.Format("2006-01-02"), Timezone: "UTC", Sources: map[string]int64{"link": 0, "text": 0, "button": 0, "banner": 0}, Daily: make([]InvitePromotionDay, 0, 30)}
	var rows []InvitePromotionClick
	if err := DB.WithContext(ctx).Where("user_id = ? AND day >= ? AND day <= ?", userID, stats.From, stats.To).Find(&rows).Error; err != nil {
		return nil, err
	}
	days := make(map[string]int64)
	for _, row := range rows {
		stats.Clicks += row.Clicks
		stats.Sources[row.Source] += row.Clicks
		days[row.Day] += row.Clicks
	}
	for day := start; !day.After(today); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		stats.Daily = append(stats.Daily, InvitePromotionDay{Day: key, Clicks: days[key]})
	}
	return stats, nil
}
