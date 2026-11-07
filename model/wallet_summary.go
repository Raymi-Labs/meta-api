package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// WalletUsage is a fixed rolling window, never the dashboard's selected/admin scope.
// Nil net quota means unavailable, not zero consumption.
type WalletUsage struct {
	NetQuota       *int64 `json:"net_quota"`
	Status         string `json:"status"`
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
}

const walletUsageRowLimit = 250000

var walletUsageSlots = make(chan struct{}, 2)

func GetWalletUsage(ctx context.Context, userID int, now time.Time) (WalletUsage, error) {
	return scanWalletUsage(ctx, userID, now, walletUsageRowLimit)
}

func scanWalletUsage(ctx context.Context, userID int, now time.Time, limit int) (WalletUsage, error) {
	result := WalletUsage{Status: "unavailable", StartTimestamp: now.Unix() - 86400, EndTimestamp: now.Unix()}
	if userID <= 0 || limit <= 0 {
		return result, errors.New("invalid wallet scope")
	}
	if !common.LogConsumeEnabled {
		result.Status = "not_recorded"
		return result, nil
	}
	select {
	case walletUsageSlots <- struct{}{}:
		defer func() { <-walletUsageSlots }()
	default:
		return result, errors.New("wallet usage busy")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	rows, err := LOG_DB.WithContext(ctx).Table("logs").Select("type, quota").Where("user_id = ? AND type IN ? AND created_at >= ? AND created_at < ?", userID, []int{LogTypeConsume, LogTypeRefund}, result.StartTimestamp, result.EndTimestamp).Limit(limit + 1).Rows()
	if err != nil {
		return result, err
	}
	defer rows.Close()
	var total int64
	for n := 0; rows.Next(); n++ {
		if n >= limit {
			return result, errors.New("wallet usage limit exceeded")
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var row struct {
			Type  int
			Quota int64
		}
		if err := LOG_DB.ScanRows(rows, &row); err != nil {
			return result, err
		}
		if row.Quota < 0 || row.Quota > 1<<50 {
			return result, errors.New("invalid usage amount")
		}
		if row.Type == LogTypeRefund {
			total -= row.Quota
		} else {
			total += row.Quota
		}
		if total > 1<<53-1 || total < -(1<<53-1) {
			return result, errors.New("usage total exceeds safe range")
		}
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result.Status = "ok"
	result.NetQuota = &total
	return result, nil
}
