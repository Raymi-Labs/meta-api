package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
)

func TestEnterpriseUsageMatchesLedger(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, owner := enterpriseTestOrg(t)
			actor := EnterpriseActor{UserId: owner.Id}
			m1, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			m2, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			_, err := CreditEnterprise(1, ent.Id, 100000, "V-1", "")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(actor, ent.Id, m1.Id, 20000, ""))
			require.NoError(t, AllocateEnterpriseQuota(actor, ent.Id, m2.Id, 20000, ""))

			// 另一家企业的账单，任何查询都不应带出来。
			other, otherOwnerMember, _ := enterpriseTestOrg(t)
			_, err = CreditEnterprise(1, other.Id, 5000, "V-1", "")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(EnterpriseActor{Platform: true}, other.Id, otherOwnerMember.Id, 5000, ""))
			_, err = AdjustEnterpriseReservation(enterpriseTestSubject(otherOwnerMember, 0), "other-"+common.GetUUID()[:8], 4000, EnterpriseReservationSettled, false)
			require.NoError(t, err)

			rid := func(s string) string { return fmt.Sprintf("%s-%s", s, common.GetUUID()[:8]) }
			sub := func(m *EnterpriseMember, tokenID int, modelName, tokenName string) EnterpriseSubject {
				s := enterpriseTestSubject(m, tokenID)
				s.ModelName, s.TokenName = modelName, tokenName
				return s
			}
			settle := func(s EnterpriseSubject, id string, reserve, final int) {
				_, err := AdjustEnterpriseReservation(s, id, reserve, EnterpriseReservationReserved, true)
				require.NoError(t, err)
				// 结算时不带快照：受理时写入的模型与令牌名称必须保留。
				s.ModelName, s.TokenName = "", ""
				_, err = AdjustEnterpriseReservation(s, id, final, EnterpriseReservationSettled, false)
				require.NoError(t, err)
			}

			a := sub(m1, 11, "gpt-a", "甲令牌")
			r1 := rid("r1")
			settle(a, r1, 400, 300)
			r2 := rid("r2") // 失败退回：不产生费用
			_, err = AdjustEnterpriseReservation(a, r2, 500, EnterpriseReservationReserved, true)
			require.NoError(t, err)
			_, err = AdjustEnterpriseReservation(a, r2, 0, EnterpriseReservationRefunded, false)
			require.NoError(t, err)
			r3 := rid("r3") // 仍在进行
			_, err = AdjustEnterpriseReservation(sub(m1, 12, "gpt-b", "乙令牌"), r3, 700, EnterpriseReservationReserved, true)
			require.NoError(t, err)
			_, err = ApplyEnterpriseCharge(a, "vf:"+r1, EnterpriseChargeViolationFee, 100)
			require.NoError(t, err)
			_, err = ApplyEnterpriseCharge(a, "adj:"+r1, EnterpriseChargeTaskAdjust, -50)
			require.NoError(t, err)
			r4 := rid("r4")
			settle(sub(m2, 21, "gpt-b", "丙令牌"), r4, 1000, 1000)

			all := EnterpriseUsageFilter{EnterpriseId: ent.Id}
			summary, err := GetEnterpriseUsageSummary(all)
			require.NoError(t, err)
			// 净消费 = 已结算之和（含违规费与负数调整），与成员账本的已消费合计一致。
			require.EqualValues(t, 300+100-50+1000, summary.NetConsumed)
			require.EqualValues(t, enterpriseTestBalanced(t, ent.Id).Consumed, summary.NetConsumed)
			require.EqualValues(t, 2, summary.Requests)
			require.EqualValues(t, 700, summary.Pending)
			require.EqualValues(t, 1, summary.PendingCount)
			require.EqualValues(t, enterpriseTestBalanced(t, ent.Id).MemberReserved, summary.Pending)
			require.Len(t, summary.ByMember, 2)
			require.Equal(t, m2.Id, summary.ByMember[0].MemberId)
			require.EqualValues(t, 1000, summary.ByMember[0].NetConsumed)
			require.NotEmpty(t, summary.ByMember[0].Name)
			require.Equal(t, m1.Id, summary.ByMember[1].MemberId)
			require.EqualValues(t, 350, summary.ByMember[1].NetConsumed)
			require.EqualValues(t, 700, summary.ByMember[1].Pending)
			require.Len(t, summary.ByModel, 2)
			require.Equal(t, "gpt-b", summary.ByModel[0].Name)
			require.EqualValues(t, 1000, summary.ByModel[0].NetConsumed)
			require.Equal(t, "gpt-a", summary.ByModel[1].Name)
			require.EqualValues(t, 350, summary.ByModel[1].NetConsumed)

			// 列表默认不含已退回；分页拼起来与导出遍历逐行一致，合计与汇总一致。
			items, total, err := ListEnterpriseUsage(all, 0, 100)
			require.NoError(t, err)
			require.EqualValues(t, 5, total)
			var paged []int64
			for offset := 0; offset < int(total); offset += 2 {
				page, pageTotal, err := ListEnterpriseUsage(all, offset, 2)
				require.NoError(t, err)
				require.Equal(t, total, pageTotal)
				for _, row := range page {
					paged = append(paged, row.Id)
				}
			}
			var walked []int64
			var settled int64
			require.NoError(t, WalkEnterpriseUsage(context.Background(), all, func(row *EnterpriseReservation) error {
				walked = append(walked, row.Id)
				require.Equal(t, ent.Id, row.EnterpriseId)
				require.NotEmpty(t, row.MemberName)
				if row.Status == EnterpriseReservationSettled {
					settled += row.Quota
				}
				return nil
			}))
			require.Len(t, items, 5)
			require.Equal(t, paged, walked)
			require.Equal(t, summary.NetConsumed, settled)
			for _, row := range items {
				require.NotEqual(t, EnterpriseReservationRefunded, row.Status)
				require.NotEmpty(t, row.ModelName)
				require.NotEmpty(t, row.TokenName)
			}

			count := func(f EnterpriseUsageFilter) int64 {
				f.EnterpriseId = ent.Id
				_, total, err := ListEnterpriseUsage(f, 0, 100)
				require.NoError(t, err)
				return total
			}
			require.EqualValues(t, 6, count(EnterpriseUsageFilter{Status: EnterpriseUsageStatusAll}))
			require.EqualValues(t, 1, count(EnterpriseUsageFilter{Status: EnterpriseReservationRefunded}))
			require.EqualValues(t, 4, count(EnterpriseUsageFilter{MemberId: m1.Id}))
			require.EqualValues(t, 1, count(EnterpriseUsageFilter{MemberId: m2.Id}))
			require.EqualValues(t, 3, count(EnterpriseUsageFilter{TokenId: 11}))
			require.EqualValues(t, 2, count(EnterpriseUsageFilter{ModelName: "gpt-b"}))
			require.EqualValues(t, 1, count(EnterpriseUsageFilter{Kind: EnterpriseChargeViolationFee}))
			// 别的企业的成员 ID 放进本企业的筛选里：查不到任何东西。
			require.EqualValues(t, 0, count(EnterpriseUsageFilter{MemberId: otherOwnerMember.Id}))

			// 时间范围：把一笔挪到很久以前，范围筛选与汇总同步变化。
			require.NoError(t, DB.Model(&EnterpriseReservation{}).Where("request_id = ?", r4).Update("created_at", 1000).Error)
			require.EqualValues(t, 1, count(EnterpriseUsageFilter{Start: 500, End: 2000}))
			require.EqualValues(t, 4, count(EnterpriseUsageFilter{Start: 2001}))
			ranged, err := GetEnterpriseUsageSummary(EnterpriseUsageFilter{EnterpriseId: ent.Id, Start: 2001})
			require.NoError(t, err)
			require.EqualValues(t, 350, ranged.NetConsumed)
			require.Len(t, ranged.ByMember, 1)

			// 另一家企业只看到自己的一笔。
			foreign, total, err := ListEnterpriseUsage(EnterpriseUsageFilter{EnterpriseId: other.Id}, 0, 100)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.EqualValues(t, 4000, foreign[0].Quota)

			// 缺企业条件或条件非法：直接拒绝，不能退化成全量查询。
			_, _, err = ListEnterpriseUsage(EnterpriseUsageFilter{}, 0, 100)
			require.ErrorIs(t, err, ErrEnterpriseNotFound)
			_, err = GetEnterpriseUsageSummary(EnterpriseUsageFilter{})
			require.ErrorIs(t, err, ErrEnterpriseNotFound)
			require.ErrorIs(t, WalkEnterpriseUsage(context.Background(), EnterpriseUsageFilter{}, nil), ErrEnterpriseNotFound)
			for _, bad := range []EnterpriseUsageFilter{{Kind: "x"}, {Status: "x"}, {MemberId: -1}, {Start: 10, End: 5}} {
				bad.EnterpriseId = ent.Id
				_, _, err = ListEnterpriseUsage(bad, 0, 100)
				require.ErrorIs(t, err, ErrEnterpriseInvalidInput)
			}

			// 超长名称截断到列宽，不让预留写入失败。
			long := rid("long")
			_, err = AdjustEnterpriseReservation(sub(m2, 21, strings.Repeat("m", 300), strings.Repeat("名", 300)), long, 10, EnterpriseReservationReserved, true)
			require.NoError(t, err)
			var stored EnterpriseReservation
			require.NoError(t, DB.Where("request_id = ?", long).First(&stored).Error)
			require.Len(t, stored.ModelName, 191)
			require.Len(t, []rune(stored.TokenName), 191)

			// 总览排行按累计已消费降序。
			spenders, err := ListEnterpriseTopSpenders(ent.Id, 10)
			require.NoError(t, err)
			require.Len(t, spenders, 2)
			require.Equal(t, m2.Id, spenders[0].MemberId)
			require.EqualValues(t, 1000, spenders[0].Consumed)
			require.Equal(t, m1.Id, spenders[1].MemberId)
			require.NotEmpty(t, spenders[1].Name)
		})
	}
}

func TestEnterpriseUsageExportLimit(t *testing.T) {
	enterpriseTestDB(t, "sqlite")
	ent, member, _ := enterpriseTestOrg(t)
	rows := make([]EnterpriseReservation, 0, LogExportMaxRows+1)
	for i := 0; i <= LogExportMaxRows; i++ {
		rows = append(rows, EnterpriseReservation{RequestId: fmt.Sprintf("bulk-%d", i), EnterpriseId: ent.Id, MemberId: member.Id,
			UserId: member.UserId, Kind: EnterpriseChargeRequest, Quota: 1, Status: EnterpriseReservationSettled, CreatedAt: 100, UpdatedAt: 100})
	}
	require.NoError(t, DB.CreateInBatches(rows, 500).Error)
	filter := EnterpriseUsageFilter{EnterpriseId: ent.Id}
	written := 0
	err := WalkEnterpriseUsage(context.Background(), filter, func(*EnterpriseReservation) error { written++; return nil })
	require.ErrorIs(t, err, ErrEnterpriseUsageTooLarge)
	require.LessOrEqual(t, written, LogExportMaxRows)
	// 正好到上限可以导出。
	require.NoError(t, DB.Where("request_id = ?", "bulk-0").Delete(&EnterpriseReservation{}).Error)
	written = 0
	require.NoError(t, WalkEnterpriseUsage(context.Background(), filter, func(*EnterpriseReservation) error { written++; return nil }))
	require.Equal(t, LogExportMaxRows, written)
}

func TestEnterpriseTokenListAndRevoke(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, ownerMember, owner := enterpriseTestOrg(t)
			admin, adminUser := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleAdmin, 0)
			admin2, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleAdmin, 0)
			staff, staffUser := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			other, otherOwnerMember, otherOwner := enterpriseTestOrg(t)

			mint := func(m *EnterpriseMember, name string, limit int64) *Token {
				token := &Token{UserId: m.UserId, EnterpriseId: m.EnterpriseId, MembershipId: m.Id, Name: name,
					Key: common.GetUUID() + common.GetUUID()[:16], Status: common.TokenStatusEnabled, ExpiredTime: -1, Group: "default"}
				require.NoError(t, CreateEnterpriseToken(token, limit))
				return token
			}
			staffToken, staffToken2 := mint(staff, "员工令牌", 500), mint(staff, "员工令牌二", 0)
			adminToken, admin2Token := mint(admin, "管理员令牌", 0), mint(admin2, "另一管理员令牌", 0)
			ownerToken := mint(ownerMember, "所有者令牌", 0)
			foreignToken := mint(otherOwnerMember, "外企令牌", 0)
			personal := &Token{UserId: staffUser.Id, Name: "个人令牌", Key: common.GetUUID() + common.GetUUID()[:16],
				Status: common.TokenStatusEnabled, ExpiredTime: -1, Group: "default"}
			require.NoError(t, DB.Create(personal).Error)

			// 列表只有本企业的企业令牌，带掩码与上限，不含明文。
			items, total, err := ListEnterpriseTokens(ent.Id, 0, 0, 100)
			require.NoError(t, err)
			require.EqualValues(t, 5, total)
			for _, item := range items {
				require.NotEqual(t, foreignToken.Id, item.Id)
				require.NotEqual(t, personal.Id, item.Id)
				require.NotEmpty(t, item.MemberName)
				require.NotEmpty(t, item.MaskedKey)
				for _, token := range []*Token{staffToken, staffToken2, adminToken, admin2Token, ownerToken} {
					require.NotContains(t, item.MaskedKey, token.Key)
				}
				if item.Id == staffToken.Id {
					require.EqualValues(t, 500, item.LimitQuota)
					require.Equal(t, staff.Id, item.MemberId)
				}
			}
			_, total, err = ListEnterpriseTokens(ent.Id, staff.Id, 0, 100)
			require.NoError(t, err)
			require.EqualValues(t, 2, total)

			alive := func(token *Token) bool {
				var n int64
				require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Count(&n).Error)
				return n == 1
			}
			asAdmin, asOwner, asStaff := EnterpriseActor{UserId: adminUser.Id}, EnterpriseActor{UserId: owner.Id}, EnterpriseActor{UserId: staffUser.Id}

			// 普通成员不能撤销别人的令牌；别的企业、个人令牌一律按不存在处理。
			require.ErrorIs(t, RevokeEnterpriseToken(asStaff, ent.Id, adminToken.Id, ""), ErrEnterpriseForbidden)
			require.ErrorIs(t, RevokeEnterpriseToken(asAdmin, ent.Id, foreignToken.Id, ""), ErrEnterpriseNotFound)
			require.ErrorIs(t, RevokeEnterpriseToken(asAdmin, ent.Id, personal.Id, ""), ErrEnterpriseNotFound)
			require.ErrorIs(t, RevokeEnterpriseToken(asAdmin, other.Id, foreignToken.Id, ""), ErrEnterpriseNotFound)
			require.ErrorIs(t, RevokeEnterpriseToken(EnterpriseActor{UserId: otherOwner.Id}, ent.Id, staffToken.Id, ""), ErrEnterpriseNotFound)
			// 管理员不能撤销其他管理员或所有者的令牌。
			require.ErrorIs(t, RevokeEnterpriseToken(asAdmin, ent.Id, admin2Token.Id, ""), ErrEnterpriseForbidden)
			require.ErrorIs(t, RevokeEnterpriseToken(asAdmin, ent.Id, ownerToken.Id, ""), ErrEnterpriseForbidden)
			for _, token := range []*Token{staffToken, adminToken, admin2Token, ownerToken, foreignToken, personal} {
				require.True(t, alive(token), token.Name)
			}

			// 管理员撤销普通成员的令牌；撤销后鉴权失败，重复撤销按不存在处理，并留审计。
			require.NoError(t, RevokeEnterpriseToken(asAdmin, ent.Id, staffToken.Id, "离职交接"))
			require.False(t, alive(staffToken))
			require.True(t, alive(staffToken2))
			require.ErrorIs(t, RevokeEnterpriseToken(asAdmin, ent.Id, staffToken.Id, ""), ErrEnterpriseNotFound)
			require.NoError(t, RevokeEnterpriseToken(asOwner, ent.Id, admin2Token.Id, ""))
			require.NoError(t, RevokeEnterpriseToken(asAdmin, ent.Id, adminToken.Id, ""))
			var audits int64
			require.NoError(t, DB.Model(&EnterpriseAuditEvent{}).Where("enterprise_id = ? AND action = ?", ent.Id, "token.revoke").Count(&audits).Error)
			require.EqualValues(t, 3, audits)

			// 已删除的企业令牌仍算在本人的企业令牌集合里，历史日志才能归到企业一侧。
			ids, err := GetUserEnterpriseTokenIDs(staffUser.Id)
			require.NoError(t, err)
			require.ElementsMatch(t, []int{staffToken.Id, staffToken2.Id}, ids)
			ids, err = GetUserEnterpriseTokenIDs(otherOwner.Id)
			require.NoError(t, err)
			require.Equal(t, []int{foreignToken.Id}, ids)
		})
	}
}
