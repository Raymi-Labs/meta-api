package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"sync"
	"time"
)

var inviteStatementOnce sync.Once

func StartInviteStatementIssuingTask() {
	inviteStatementOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				now := common.GetTimestamp()
				if e := model.ReleaseInviteCarry(now); e != nil {
					common.SysError(e.Error())
				}
				if e := model.IssueInviteStatements(now); e != nil {
					common.SysError(e.Error())
				}

				model.NotifyInvitePayoutWorker()
				select {
				case <-ticker.C:
				case <-model.InviteStatementWakeup():
				}
			}
		}()
	})
}
