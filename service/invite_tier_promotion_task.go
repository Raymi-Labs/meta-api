package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"sync"
	"time"
)

var invitePromotionOnce sync.Once

func StartInviteTierPromotionTask() {
	invitePromotionOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				if e := model.RefreshInviteQualifications(common.GetTimestamp()); e != nil {
					common.SysError(e.Error())
				}
				if e := model.MaintainInvitePromoters(common.GetTimestamp()); e != nil {
					common.SysError(e.Error())
				}
				<-ticker.C
			}
		}()
	})
}
