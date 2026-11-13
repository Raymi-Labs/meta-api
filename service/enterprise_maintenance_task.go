package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	enterpriseMaintenanceInterval  = 5 * time.Minute
	enterpriseMaintenanceBatchSize = 500
	// 同步请求不会持续这么久。超过这个时长仍未终结，说明进程在结算前退出或结算失败。
	enterpriseReservationStaleDefault = 6 * 60 * 60

	NotifyTypeEnterpriseReview = "enterprise_reservation_review"
)

var (
	enterpriseMaintenanceOnce    sync.Once
	enterpriseMaintenanceRunning atomic.Bool
)

func enterpriseReservationStaleSeconds() int64 {
	seconds := common.GetEnvOrDefault("ENTERPRISE_RESERVATION_STALE_SECONDS", enterpriseReservationStaleDefault)
	if seconds < 600 {
		seconds = 600
	}
	return int64(seconds)
}

// StartEnterpriseMaintenanceTask 启动企业账本的回收任务，只在主节点运行。
func StartEnterpriseMaintenanceTask() {
	enterpriseMaintenanceOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			ticker := time.NewTicker(enterpriseMaintenanceInterval)
			defer ticker.Stop()
			runEnterpriseMaintenanceLoop(context.Background(), ticker.C, func() {
				runEnterpriseMaintenanceOnce(time.Now())
			})
		})
	})
}

func runEnterpriseMaintenanceLoop(ctx context.Context, ticks <-chan time.Time, run func()) {
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
		run()
	}
}

// runEnterpriseMaintenanceOnce 做三件事（第三件是企业余额低于提醒线时邮件提醒所有者与管理员）：
//  1. 超时未终结的预留转待对账并告警。额度继续计在成员在途里，不自动退款——
//     结果不明时退款等于白送，由平台核实后按真实费用结算或释放。
//  2. 已停用成员的残余额度（在途请求事后结算产生）并回企业未分配。
func runEnterpriseMaintenanceOnce(now time.Time) (review int64, swept int) {
	if !enterpriseMaintenanceRunning.CompareAndSwap(false, true) {
		return 0, 0
	}
	defer enterpriseMaintenanceRunning.Store(false)
	// Already accepted work remains payable even when new enterprise requests are disabled.
	if model.DB.Migrator().HasTable(&model.Task{}) {
		if _, err := model.ReconcileEnterpriseTasks(enterpriseMaintenanceBatchSize); err != nil {
			common.SysError(err.Error())
		}
	}
	if model.DB.Migrator().HasTable(&model.Midjourney{}) {
		if _, err := model.ReconcileEnterpriseMJ(enterpriseMaintenanceBatchSize); err != nil {
			common.SysError(err.Error())
		}
	}
	if !common.EnterpriseEnabled {
		return 0, 0
	}

	review, err := model.MarkStaleEnterpriseReservations(now.Unix()-enterpriseReservationStaleSeconds(), enterpriseMaintenanceBatchSize)
	if err != nil {
		common.SysError("enterprise reservation review sweep failed: " + err.Error())
	} else if review > 0 {
		common.SysLog(fmt.Sprintf("enterprise reservations moved to review: %d", review))
		notifyEnterpriseReview(review)
	}

	swept, err = model.SweepInactiveEnterpriseResidues(enterpriseMaintenanceBatchSize)
	if err != nil {
		common.SysError("enterprise member residue sweep failed: " + err.Error())
	} else if swept > 0 {
		common.SysLog(fmt.Sprintf("enterprise inactive member residues swept: %d", swept))
	}
	if notified := notifyEnterpriseLowBalances(enterpriseMaintenanceBatchSize); notified > 0 {
		common.SysLog(fmt.Sprintf("enterprise low balance reminders sent: %d", notified))
	}
	return review, swept
}

// 测试里替换，避免真的发通知。
var notifyEnterpriseReview = func(count int64) {
	NotifyRootUser(NotifyTypeEnterpriseReview, "企业额度有待对账的预留",
		fmt.Sprintf("有 %d 笔企业请求超时未结算，已转入待对账，额度仍被占用。请在企业管理中核实后按真实费用结算或释放。", count))
}
