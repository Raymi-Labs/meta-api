package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"sync"
	"time"
)

var invitePoolOnce sync.Once

func StartInvitePoolAdjustmentTask() {
	invitePoolOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				if e := model.AdjustOutstandingInvitePools(common.GetTimestamp()); e != nil {
					common.SysError(e.Error())
				}
				<-ticker.C
			}
		}()
	})
}
