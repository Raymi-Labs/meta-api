package model

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnterpriseOwnerReviewsOwnQuotaRequest(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, ownerMember, owner := enterpriseTestOrg(t)
			actor := EnterpriseActor{UserId: owner.Id}
			_, err := CreditEnterprise(1, ent.Id, 1000, "OWNER-APPROVAL", "")
			require.NoError(t, err)
			req, err := CreateEnterpriseQuotaRequest(owner.Id, ent.Id, 1200, "追加额度")
			require.NoError(t, err)
			_, err = ReviewEnterpriseQuotaRequest(actor, ent.Id, req.Id, true, 0, "")
			require.ErrorIs(t, err, ErrEnterpriseInsufficientUnallocated)
			pending, err := CountEnterprisePendingRequests(ent.Id)
			require.NoError(t, err)
			require.EqualValues(t, 1, pending)
			require.EqualValues(t, 1000, enterpriseTestBalanced(t, ent.Id).Unallocated)

			// 同一张历史待办允许Owner批准较小金额，并发/重复审批只划拨一次。
			var wg sync.WaitGroup
			results := make(chan error, 4)
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := ReviewEnterpriseQuotaRequest(actor, ent.Id, req.Id, true, 400, "本人预算")
					results <- err
				}()
			}
			wg.Wait()
			close(results)
			succeeded := 0
			for err := range results {
				if err == nil {
					succeeded++
				} else {
					require.ErrorIs(t, err, ErrEnterpriseRequestState)
				}
			}
			require.Equal(t, 1, succeeded)
			balance := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 600, balance.Unallocated)
			require.EqualValues(t, 400, balance.MemberAvailable)
			require.EqualValues(t, 1000, balance.NetCredited)
			var stored EnterpriseQuotaRequest
			require.NoError(t, DB.First(&stored, req.Id).Error)
			require.Equal(t, EnterpriseRequestApproved, stored.Status)
			require.Equal(t, owner.Id, stored.ReviewerId)
			require.Nil(t, stored.PendingMemberId)
			var audits int64
			require.NoError(t, DB.Model(&EnterpriseAuditEvent{}).Where("enterprise_id = ? AND actor_id = ? AND action = ?", ent.Id, owner.Id, "request.approved").Count(&audits).Error)
			require.EqualValues(t, 1, audits)

			rejected, err := CreateEnterpriseQuotaRequest(owner.Id, ent.Id, 100, "无需追加")
			require.NoError(t, err)
			_, err = ReviewEnterpriseQuotaRequest(actor, ent.Id, rejected.Id, false, 0, "取消本次预算")
			require.NoError(t, err)
			require.EqualValues(t, 600, enterpriseTestBalanced(t, ent.Id).Unallocated)

			// 转移所有权后按当前角色授权，旧Owner不得以申请时的身份自批。
			stale, err := CreateEnterpriseQuotaRequest(owner.Id, ent.Id, 100, "移交前申请")
			require.NoError(t, err)
			next, nextUser := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			require.NoError(t, TransferEnterpriseOwnership(actor, ent.Id, next.Id))
			_, err = ReviewEnterpriseQuotaRequest(actor, ent.Id, stale.Id, true, 0, "")
			require.ErrorIs(t, err, ErrEnterpriseSelfOperation)
			_, err = ReviewEnterpriseQuotaRequest(EnterpriseActor{UserId: nextUser.Id}, ent.Id, stale.Id, true, 0, "")
			require.NoError(t, err)
			member, err := GetEnterpriseMember(ent.Id, ownerMember.Id)
			require.NoError(t, err)
			require.EqualValues(t, 500, member.Available)
			enterpriseTestBalanced(t, ent.Id)
		})
	}
}
