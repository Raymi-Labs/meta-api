package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	inviteCommissionSettlementBatchSize = 300
)

var (
	inviteCommissionSettlementOnce    sync.Once
	inviteCommissionSettlementRunning atomic.Bool
)

func StartInviteCommissionSettlementTask() {
	inviteCommissionSettlementOnce.Do(func() {
		gopool.Go(func() {
			// Every instance can receive a payment; database transactions prevent
			// duplicate payouts. Wakeups coalesce and the sweep recovers lost signals.
			logger.LogInfo(context.Background(), "invite commission settlement task started: immediate + 30s recovery")
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			runInviteCommissionSettlementLoop(context.Background(), model.InviteCommissionSettlementWakeups(), ticker.C, runInviteCommissionSettlementOnce)
		})
	})
}

// Kept separate from process startup so recovery triggers can be tested without
// a permanent worker or a wall-clock sleep.
func runInviteCommissionSettlementLoop(ctx context.Context, wakeups <-chan struct{}, ticks <-chan time.Time, run func()) {
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-wakeups:
		case <-ticks:
		}
		run()
	}
}

func runInviteCommissionSettlementOnce() {
	// 防重入保护：避免慢查询或大积压导致多轮并发结算。
	if !inviteCommissionSettlementRunning.CompareAndSwap(false, true) {
		return
	}
	defer inviteCommissionSettlementRunning.Store(false)

	if !common.InviterCommissionEnabled {
		return
	}

	// 包含当天待办；旧pending保留原业务日及日限额，不重复发放已结算记录。
	targetDate := time.Now().Format("2006-01-02")

	totalSettled := 0
	totalSkipped := 0
	for {
		// 分批处理，避免单次任务过大导致长事务和高延迟。
		settled, skipped, processed, err := model.SettleInviteCommissionByBizDate(targetDate, inviteCommissionSettlementBatchSize)
		if err != nil {
			logger.LogWarn(context.Background(), fmt.Sprintf("invite commission settlement failed: %v", err))
			return
		}
		totalSettled += settled
		totalSkipped += skipped
		// 当前批次不足上限，说明本轮没有更多待处理记录。
		if processed < inviteCommissionSettlementBatchSize {
			break
		}
	}

	if common.DebugEnabled && (totalSettled > 0 || totalSkipped > 0) {
		logger.LogDebug(context.Background(), "invite commission settlement: settled=%d skipped=%d", totalSettled, totalSkipped)
	}
}
