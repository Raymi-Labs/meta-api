package model

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/stretchr/testify/require"
)

var enterpriseTestDialects = []string{"sqlite", "mysql", "postgres"}

func enterpriseTestDB(t *testing.T, dialect string) {
	t.Helper()
	quotaNoticeTestDB(t, dialect)
	oldSQLite, oldMySQL, oldPG, oldRedis := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL, common.RedisEnabled
	common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL, common.RedisEnabled = dialect == "sqlite", dialect == "mysql", dialect == "postgres", false
	t.Cleanup(func() {
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL, common.RedisEnabled = oldSQLite, oldMySQL, oldPG, oldRedis
	})
	require.NoError(t, DB.AutoMigrate(&User{}, &Token{}))
	require.NoError(t, DB.AutoMigrate(enterpriseModels()...))
}

func enterpriseTestUser(t *testing.T, quota int) *User {
	t.Helper()
	id := common.GetUUID()
	user := &User{Username: "ent-" + id[:12], AffCode: id[:12], Email: "ent-" + id[:12] + "@example.com",
		Quota: quota, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, DB.Create(user).Error)
	return user
}

// enterpriseTestOrg 开通一家企业并返回企业与所有者成员。
func enterpriseTestOrg(t *testing.T) (*Enterprise, *EnterpriseMember, *User) {
	t.Helper()
	owner := enterpriseTestUser(t, 0)
	ent, err := CreateEnterprise(1, "测试企业"+common.GetUUID()[:6], "default", "", owner.Id)
	require.NoError(t, err)
	member, err := GetActiveEnterpriseMemberByUser(owner.Id)
	require.NoError(t, err)
	require.NotNil(t, member)
	return ent, member, owner
}

// enterpriseTestJoin 通过邀请让一个新账号加入企业。
func enterpriseTestJoin(t *testing.T, ent *Enterprise, inviter int, role string, initial int64) (*EnterpriseMember, *User) {
	t.Helper()
	user := enterpriseTestUser(t, 0)
	token, _, err := CreateEnterpriseInvitation(EnterpriseActor{UserId: inviter}, ent.Id, user.Email, role, initial)
	require.NoError(t, err)
	res, err := AcceptEnterpriseInvitation(token, user.Id)
	require.NoError(t, err)
	return res.Member, user
}

func enterpriseTestBalanced(t *testing.T, enterpriseID int) *EnterpriseBalance {
	t.Helper()
	bal, err := GetEnterpriseBalance(enterpriseID)
	require.NoError(t, err)
	require.True(t, bal.Balanced, "守恒式不成立: %+v", bal)
	return bal
}

func enterpriseTestSubject(m *EnterpriseMember, tokenID int) EnterpriseSubject {
	return EnterpriseSubject{EnterpriseId: m.EnterpriseId, MemberId: m.Id, UserId: m.UserId, TokenId: tokenID}
}

func TestEnterpriseMembershipRules(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, ownerMember, owner := enterpriseTestOrg(t)
			ownerActor := EnterpriseActor{UserId: owner.Id}

			_, err := CreateEnterprise(1, "无效分组", "no-such-group-"+common.GetUUID()[:6], "", owner.Id)
			require.Error(t, err)
			// 一个账号只能加入一家企业：已是所有者的账号不能再开通第二家。
			_, err = CreateEnterprise(1, "第二家", "default", "", owner.Id)
			require.ErrorIs(t, err, ErrEnterpriseAlreadyMember)

			admin, adminUser := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleAdmin, 0)
			member, memberUser := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			adminActor, memberActor := EnterpriseActor{UserId: adminUser.Id}, EnterpriseActor{UserId: memberUser.Id}

			// 普通成员不能邀请；管理员不能邀请管理员。
			_, _, err = CreateEnterpriseInvitation(memberActor, ent.Id, "x@example.com", EnterpriseRoleMember, 0)
			require.ErrorIs(t, err, ErrEnterpriseForbidden)
			_, _, err = CreateEnterpriseInvitation(adminActor, ent.Id, "x@example.com", EnterpriseRoleAdmin, 0)
			require.ErrorIs(t, err, ErrEnterpriseForbidden)
			_, _, err = CreateEnterpriseInvitation(ownerActor, ent.Id, "x@example.com", EnterpriseRoleOwner, 0)
			require.ErrorIs(t, err, ErrEnterpriseInvalidInput)

			// 管理员不能任免管理员、不能转移所有权、不能改企业设置。
			require.ErrorIs(t, SetEnterpriseMemberRole(adminActor, ent.Id, member.Id, EnterpriseRoleAdmin), ErrEnterpriseForbidden)
			require.ErrorIs(t, TransferEnterpriseOwnership(adminActor, ent.Id, admin.Id), ErrEnterpriseForbidden)
			require.ErrorIs(t, UpdateEnterpriseSettings(adminActor, ent.Id, EnterpriseSettingsInput{Name: "x", Timezone: "UTC", ModelMode: EnterpriseModelModeInherit}), ErrEnterpriseForbidden)

			// 所有者不能被降级、停用或直接退出。
			require.ErrorIs(t, SetEnterpriseMemberRole(ownerActor, ent.Id, ownerMember.Id, EnterpriseRoleMember), ErrEnterpriseOwnerRequired)
			require.ErrorIs(t, DisableEnterpriseMember(EnterpriseActor{UserId: 1, Platform: true}, ent.Id, ownerMember.Id, ""), ErrEnterpriseOwnerRequired)
			require.ErrorIs(t, LeaveEnterprise(owner.Id, ent.Id), ErrEnterpriseOwnerRequired)

			// 管理员不能停用另一名管理员或自己。
			admin2, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleAdmin, 0)
			require.ErrorIs(t, DisableEnterpriseMember(adminActor, ent.Id, admin2.Id, ""), ErrEnterpriseForbidden)
			require.ErrorIs(t, DisableEnterpriseMember(adminActor, ent.Id, admin.Id, ""), ErrEnterpriseSelfOperation)

			// 转移所有权后恰有一名所有者，原所有者变为管理员并可以退出。
			require.NoError(t, TransferEnterpriseOwnership(ownerActor, ent.Id, admin.Id))
			var owners int64
			require.NoError(t, DB.Model(&EnterpriseMember{}).Where("enterprise_id = ? AND status = ? AND role = ?", ent.Id, EnterpriseMemberStatusActive, EnterpriseRoleOwner).Count(&owners).Error)
			require.EqualValues(t, 1, owners)
			got, err := GetEnterpriseByID(ent.Id)
			require.NoError(t, err)
			require.Equal(t, adminUser.Id, got.OwnerUserId)
			require.ErrorIs(t, TransferEnterpriseOwnership(ownerActor, ent.Id, member.Id), ErrEnterpriseForbidden)
			require.NoError(t, LeaveEnterprise(owner.Id, ent.Id))
			gone, err := GetActiveEnterpriseMemberByUser(owner.Id)
			require.NoError(t, err)
			require.Nil(t, gone)
		})
	}
}

func TestEnterpriseCrossTenantIsolation(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			entA, _, ownerA := enterpriseTestOrg(t)
			entB, _, ownerB := enterpriseTestOrg(t)
			memberB, _ := enterpriseTestJoin(t, entB, ownerB.Id, EnterpriseRoleMember, 0)
			_, err := CreditEnterprise(1, entA.Id, 1000, "A-1", "")
			require.NoError(t, err)
			_, err = CreditEnterprise(1, entB.Id, 1000, "B-1", "")
			require.NoError(t, err)
			actorA := EnterpriseActor{UserId: ownerA.Id}

			// A 的所有者以 B 的企业 ID 操作：与企业不存在同一个错误。
			require.ErrorIs(t, AllocateEnterpriseQuota(actorA, entB.Id, memberB.Id, 10, ""), ErrEnterpriseNotFound)
			require.ErrorIs(t, DisableEnterpriseMember(actorA, entB.Id, memberB.Id, ""), ErrEnterpriseNotFound)
			_, _, err = CreateEnterpriseInvitation(actorA, entB.Id, "x@example.com", EnterpriseRoleMember, 0)
			require.ErrorIs(t, err, ErrEnterpriseNotFound)
			// A 的所有者用自己的企业 ID 加 B 的成员 ID：同样不可访问。
			require.ErrorIs(t, AllocateEnterpriseQuota(actorA, entA.Id, memberB.Id, 10, ""), ErrEnterpriseNotFound)
			require.ErrorIs(t, DisableEnterpriseMember(actorA, entA.Id, memberB.Id, ""), ErrEnterpriseNotFound)
			require.ErrorIs(t, SetEnterpriseMemberModels(actorA, entA.Id, memberB.Id, EnterpriseModelModeCustom, nil), ErrEnterpriseNotFound)
			_, err = GetEnterpriseMember(entA.Id, memberB.Id)
			require.ErrorIs(t, err, ErrEnterpriseNotFound)

			require.EqualValues(t, 1000, enterpriseTestBalanced(t, entA.Id).Unallocated)
			require.EqualValues(t, 1000, enterpriseTestBalanced(t, entB.Id).Unallocated)
		})
	}
}

func TestEnterpriseInvitationLifecycle(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, owner := enterpriseTestOrg(t)
			actor := EnterpriseActor{UserId: owner.Id}
			_, err := CreditEnterprise(1, ent.Id, 500, "V-1", "")
			require.NoError(t, err)

			user := enterpriseTestUser(t, 77)
			token, inv, err := CreateEnterpriseInvitation(actor, ent.Id, "  "+user.Email+" ", "", 300)
			require.NoError(t, err)
			require.NotContains(t, inv.Digest, token)
			// 未接受的邀请不占用预算。
			require.EqualValues(t, 500, enterpriseTestBalanced(t, ent.Id).Unallocated)

			// 邮箱不一致的账号不能接受，且不消费邀请。
			other := enterpriseTestUser(t, 0)
			_, err = AcceptEnterpriseInvitation(token, other.Id)
			require.ErrorIs(t, err, ErrEnterpriseInvitationMail)

			res, err := AcceptEnterpriseInvitation(token, user.Id)
			require.NoError(t, err)
			require.False(t, res.QuotaShortfall)
			require.EqualValues(t, 300, res.Member.Available)
			require.Equal(t, 1, res.Member.Generation)
			bal := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 200, bal.Unallocated)
			require.EqualValues(t, 300, bal.MemberAvailable)

			// 一次性：再次接受失败，额度只到账一次，个人钱包不变。
			_, err = AcceptEnterpriseInvitation(token, user.Id)
			require.ErrorIs(t, err, ErrEnterpriseInvitation)
			require.EqualValues(t, 200, enterpriseTestBalanced(t, ent.Id).Unallocated)
			var reloaded User
			require.NoError(t, DB.First(&reloaded, user.Id).Error)
			require.Equal(t, 77, reloaded.Quota)

			// 未分配不足：照常加入，额度为 0，并标记不足。
			user2 := enterpriseTestUser(t, 0)
			token2, _, err := CreateEnterpriseInvitation(actor, ent.Id, user2.Email, EnterpriseRoleMember, 9999)
			require.NoError(t, err)
			res2, err := AcceptEnterpriseInvitation(token2, user2.Id)
			require.NoError(t, err)
			require.True(t, res2.QuotaShortfall)
			require.EqualValues(t, 0, res2.Member.Available)
			require.EqualValues(t, 200, enterpriseTestBalanced(t, ent.Id).Unallocated)

			// 过期邀请不可用。
			user3 := enterpriseTestUser(t, 0)
			token3, inv3, err := CreateEnterpriseInvitation(actor, ent.Id, user3.Email, EnterpriseRoleMember, 0)
			require.NoError(t, err)
			require.NoError(t, DB.Model(&EnterpriseInvitation{}).Where("id = ?", inv3.Id).Update("expires_at", time.Now().Unix()-1).Error)
			_, err = PreviewEnterpriseInvitation(token3)
			require.ErrorIs(t, err, ErrEnterpriseInvitation)
			_, err = AcceptEnterpriseInvitation(token3, user3.Id)
			require.ErrorIs(t, err, ErrEnterpriseInvitation)

			// 撤销后的邀请不可用；重新邀请会作废同邮箱的旧邀请。
			token4, _, err := CreateEnterpriseInvitation(actor, ent.Id, user3.Email, EnterpriseRoleMember, 0)
			require.NoError(t, err)
			token5, inv5, err := CreateEnterpriseInvitation(actor, ent.Id, user3.Email, EnterpriseRoleMember, 0)
			require.NoError(t, err)
			_, err = AcceptEnterpriseInvitation(token4, user3.Id)
			require.ErrorIs(t, err, ErrEnterpriseInvitation)
			require.NoError(t, RevokeEnterpriseInvitation(actor, ent.Id, inv5.Id))
			_, err = AcceptEnterpriseInvitation(token5, user3.Id)
			require.ErrorIs(t, err, ErrEnterpriseInvitation)

			// 已加入企业的账号不能再接受其他企业的邀请。
			entB, _, ownerB := enterpriseTestOrg(t)
			tokenB, _, err := CreateEnterpriseInvitation(EnterpriseActor{UserId: ownerB.Id}, entB.Id, user.Email, EnterpriseRoleMember, 0)
			require.NoError(t, err)
			_, err = AcceptEnterpriseInvitation(tokenB, user.Id)
			require.ErrorIs(t, err, ErrEnterpriseAlreadyMember)
			// 失败的接受整体回滚，邀请仍可在离开原企业后使用。
			require.NoError(t, LeaveEnterprise(user.Id, ent.Id))
			resB, err := AcceptEnterpriseInvitation(tokenB, user.Id)
			require.NoError(t, err)
			require.Equal(t, entB.Id, resB.Member.EnterpriseId)
		})
	}
}

func TestEnterpriseInvitationConcurrentAccept(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, owner := enterpriseTestOrg(t)
			_, err := CreditEnterprise(1, ent.Id, 1000, "V-1", "")
			require.NoError(t, err)
			user := enterpriseTestUser(t, 0)
			token, _, err := CreateEnterpriseInvitation(EnterpriseActor{UserId: owner.Id}, ent.Id, user.Email, EnterpriseRoleMember, 100)
			require.NoError(t, err)

			var wg sync.WaitGroup
			results := make(chan error, 12)
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := AcceptEnterpriseInvitation(token, user.Id)
					results <- err
				}()
			}
			wg.Wait()
			close(results)
			ok := 0
			for err := range results {
				if err == nil {
					ok++
				}
			}
			require.Equal(t, 1, ok)
			bal := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 900, bal.Unallocated)
			require.EqualValues(t, 100, bal.MemberAvailable)
		})
	}
}

func TestEnterpriseRejoinStartsNewGeneration(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, owner := enterpriseTestOrg(t)
			actor := EnterpriseActor{UserId: owner.Id}
			_, err := CreditEnterprise(1, ent.Id, 1000, "V-1", "")
			require.NoError(t, err)
			member, user := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 400)

			token := &Token{UserId: user.Id, Key: common.GetUUID() + common.GetUUID()[:16], Name: "ent", Status: common.TokenStatusEnabled,
				UnlimitedQuota: true, ExpiredTime: -1, EnterpriseId: ent.Id, MembershipId: member.Id}
			require.NoError(t, DB.Create(token).Error)
			personal := &Token{UserId: user.Id, Key: common.GetUUID() + common.GetUUID()[:16], Name: "me", Status: common.TokenStatusEnabled,
				UnlimitedQuota: true, ExpiredTime: -1}
			require.NoError(t, DB.Create(personal).Error)

			require.NoError(t, DisableEnterpriseMember(actor, ent.Id, member.Id, "离职"))
			// 剩余额度并入未分配并记流水；企业令牌立即失效，个人令牌不受影响。
			bal := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 1000, bal.Unallocated)
			require.EqualValues(t, 0, bal.MemberAvailable)
			var sweeps int64
			require.NoError(t, DB.Model(&EnterpriseLedger{}).Where("enterprise_id = ? AND member_id = ? AND kind = ?", ent.Id, member.Id, EnterpriseLedgerMemberSweep).Count(&sweeps).Error)
			require.EqualValues(t, 1, sweeps)
			var reloaded, reloadedPersonal Token
			require.NoError(t, DB.First(&reloaded, token.Id).Error)
			require.Equal(t, common.TokenStatusDisabled, reloaded.Status)
			require.NoError(t, DB.First(&reloadedPersonal, personal.Id).Error)
			require.Equal(t, common.TokenStatusEnabled, reloadedPersonal.Status)
			// 旧成员关系不再通过令牌校验。
			_, old, err := GetEnterpriseTokenContext(ent.Id, member.Id, user.Id)
			require.NoError(t, err)
			require.Equal(t, EnterpriseMemberStatusDisabled, old.Status)
			require.ErrorIs(t, AllocateEnterpriseQuota(actor, ent.Id, member.Id, 10, ""), ErrEnterpriseNotFound)

			// 重新加入：新成员行、代次加一，旧令牌仍绑定旧成员行且保持停用。
			token2, _, err := CreateEnterpriseInvitation(actor, ent.Id, user.Email, EnterpriseRoleMember, 0)
			require.NoError(t, err)
			res, err := AcceptEnterpriseInvitation(token2, user.Id)
			require.NoError(t, err)
			require.NotEqual(t, member.Id, res.Member.Id)
			require.Equal(t, 2, res.Member.Generation)
			require.NoError(t, DB.First(&reloaded, token.Id).Error)
			require.Equal(t, common.TokenStatusDisabled, reloaded.Status)
			require.Equal(t, member.Id, reloaded.MembershipId)
		})
	}
}

func TestEnterpriseWalletRules(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, ownerMember, owner := enterpriseTestOrg(t)
			ownerActor := EnterpriseActor{UserId: owner.Id}
			admin, adminUser := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleAdmin, 0)
			member, memberUser := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			adminActor := EnterpriseActor{UserId: adminUser.Id}

			_, err := CreditEnterprise(1, ent.Id, 10000, "BANK-001", "首笔")
			require.NoError(t, err)
			// 同一企业内凭证号唯一；另一家企业可以用相同凭证号。
			_, err = CreditEnterprise(1, ent.Id, 10000, "BANK-001", "")
			require.ErrorIs(t, err, ErrEnterpriseDuplicateVoucher)
			entB, _, _ := enterpriseTestOrg(t)
			_, err = CreditEnterprise(1, entB.Id, 5, "BANK-001", "")
			require.NoError(t, err)
			_, err = CreditEnterprise(1, ent.Id, 0, "BANK-002", "")
			require.ErrorIs(t, err, ErrEnterpriseInvalidInput)
			_, err = CreditEnterprise(1, ent.Id, 10, " ", "")
			require.ErrorIs(t, err, ErrEnterpriseInvalidInput)

			// 方案示例：到账 10000，甲 2000、乙 1000，未分配 7000。
			require.NoError(t, AllocateEnterpriseQuota(adminActor, ent.Id, member.Id, 2000, ""))
			require.NoError(t, AllocateEnterpriseQuota(ownerActor, ent.Id, admin.Id, 1000, ""))
			bal := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 7000, bal.Unallocated)
			require.EqualValues(t, 3000, bal.MemberAvailable)

			// 管理员不能给自己或其他管理员、所有者分配；普通成员不能分配。
			require.ErrorIs(t, AllocateEnterpriseQuota(adminActor, ent.Id, admin.Id, 1, ""), ErrEnterpriseSelfOperation)
			require.ErrorIs(t, AllocateEnterpriseQuota(adminActor, ent.Id, ownerMember.Id, 1, ""), ErrEnterpriseForbidden)
			require.ErrorIs(t, AllocateEnterpriseQuota(EnterpriseActor{UserId: memberUser.Id}, ent.Id, member.Id, 1, ""), ErrEnterpriseForbidden)
			// 所有者自己的预算调整是直接管理操作，留痕。
			require.NoError(t, AllocateEnterpriseQuota(ownerActor, ent.Id, ownerMember.Id, 500, "自用"))
			var audits int64
			require.NoError(t, DB.Model(&EnterpriseAuditEvent{}).Where("enterprise_id = ? AND action = ? AND target_id = ? AND actor_id = ?", ent.Id, "budget.allocate", ownerMember.Id, owner.Id).Count(&audits).Error)
			require.EqualValues(t, 1, audits)

			// 不能超分配；回收仅限为正的部分。
			require.ErrorIs(t, AllocateEnterpriseQuota(adminActor, ent.Id, member.Id, 6501, ""), ErrEnterpriseInsufficientUnallocated)
			require.ErrorIs(t, ReclaimEnterpriseQuota(adminActor, ent.Id, member.Id, 2001, ""), ErrEnterpriseReclaimExceeds)
			require.NoError(t, ReclaimEnterpriseQuota(adminActor, ent.Id, member.Id, 500, ""))
			bal = enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 7000, bal.Unallocated)

			// 调账必须写原因；未分配为负时禁止分配，下次入账先抵。
			_, err = AdjustEnterprise(1, ent.Id, -100, " ")
			require.ErrorIs(t, err, ErrEnterpriseInvalidInput)
			_, err = AdjustEnterprise(1, ent.Id, -7100, "录错金额")
			require.NoError(t, err)
			require.EqualValues(t, -100, enterpriseTestBalanced(t, ent.Id).Unallocated)
			require.ErrorIs(t, AllocateEnterpriseQuota(adminActor, ent.Id, member.Id, 1, ""), ErrEnterpriseInsufficientUnallocated)
			_, err = CreditEnterprise(1, ent.Id, 150, "BANK-003", "")
			require.NoError(t, err)
			require.EqualValues(t, 50, enterpriseTestBalanced(t, ent.Id).Unallocated)

			// 冻结后不能分配，解冻后恢复。
			require.NoError(t, SetEnterpriseStatus(1, ent.Id, EnterpriseStatusFrozen, "欠费"))
			require.ErrorIs(t, AllocateEnterpriseQuota(adminActor, ent.Id, member.Id, 1, ""), ErrEnterpriseFrozen)
			require.NoError(t, SetEnterpriseStatus(1, ent.Id, EnterpriseStatusActive, ""))
			require.NoError(t, AllocateEnterpriseQuota(adminActor, ent.Id, member.Id, 1, ""))
			enterpriseTestBalanced(t, ent.Id)
		})
	}
}

func TestEnterpriseConcurrentAllocationConserves(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, owner := enterpriseTestOrg(t)
			actor := EnterpriseActor{UserId: owner.Id}
			a, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			b, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			_, err := CreditEnterprise(1, ent.Id, 1000, "V-1", "")
			require.NoError(t, err)

			// 30 笔各 50 的分配争抢 1000：恰好 20 笔成功，同时穿插回收与重复凭证入账。
			var wg sync.WaitGroup
			results := make(chan error, 30)
			credits := make(chan error, 10)
			for i := 0; i < 30; i++ {
				wg.Add(1)
				target := a.Id
				if i%2 == 1 {
					target = b.Id
				}
				go func() {
					defer wg.Done()
					results <- AllocateEnterpriseQuota(actor, ent.Id, target, 50, "")
				}()
			}
			wg.Wait()
			close(results)
			ok := 0
			for err := range results {
				if err == nil {
					ok++
				} else {
					require.ErrorIs(t, err, ErrEnterpriseInsufficientUnallocated)
				}
			}
			require.Equal(t, 20, ok)
			bal := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 0, bal.Unallocated)
			require.EqualValues(t, 1000, bal.MemberAvailable)

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := CreditEnterprise(1, ent.Id, 300, "V-DUP", "")
					credits <- err
				}()
			}
			wg.Wait()
			close(credits)
			ok = 0
			for err := range credits {
				if err == nil {
					ok++
				} else {
					require.ErrorIs(t, err, ErrEnterpriseDuplicateVoucher)
				}
			}
			require.Equal(t, 1, ok)
			bal = enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 300, bal.Unallocated)
			require.EqualValues(t, 1300, bal.NetCredited)
		})
	}
}

func TestEnterpriseReservationStateMachine(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, owner := enterpriseTestOrg(t)
			actor := EnterpriseActor{UserId: owner.Id}
			member, user := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			_, err := CreditEnterprise(1, ent.Id, 10000, "V-1", "")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(actor, ent.Id, member.Id, 2000, ""))
			sub := enterpriseTestSubject(member, 0)
			rid := func(s string) string { return fmt.Sprintf("%s-%s", s, common.GetUUID()[:8]) }

			// 预留 400 → 结算 300：多退少补，重复结算幂等。
			r1 := rid("r1")
			res, err := AdjustEnterpriseReservation(sub, r1, 400, EnterpriseReservationReserved, true)
			require.NoError(t, err)
			require.EqualValues(t, 2000, res.AvailableBefore)
			bal := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 1600, bal.MemberAvailable)
			require.EqualValues(t, 400, bal.MemberReserved)
			// 重试共用同一预留：相同目标值不重复占用。
			_, err = AdjustEnterpriseReservation(sub, r1, 400, EnterpriseReservationReserved, true)
			require.NoError(t, err)
			require.EqualValues(t, 400, enterpriseTestBalanced(t, ent.Id).MemberReserved)
			_, err = AdjustEnterpriseReservation(sub, r1, 300, EnterpriseReservationSettled, false)
			require.NoError(t, err)
			again, err := AdjustEnterpriseReservation(sub, r1, 300, EnterpriseReservationSettled, false)
			require.NoError(t, err)
			require.False(t, again.Changed)
			_, err = AdjustEnterpriseReservation(sub, r1, 0, EnterpriseReservationRefunded, false)
			require.ErrorIs(t, err, ErrEnterpriseFinalized)
			_, err = AdjustEnterpriseReservation(sub, r1, 999, EnterpriseReservationSettled, false)
			require.ErrorIs(t, err, ErrEnterpriseFinalized)
			bal = enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 1700, bal.MemberAvailable)
			require.EqualValues(t, 0, bal.MemberReserved)
			require.EqualValues(t, 300, bal.Consumed)

			// 退款释放全部占用，重复退款幂等。
			r2 := rid("r2")
			_, err = AdjustEnterpriseReservation(sub, r2, 500, EnterpriseReservationReserved, true)
			require.NoError(t, err)
			_, err = AdjustEnterpriseReservation(sub, r2, 0, EnterpriseReservationRefunded, false)
			require.NoError(t, err)
			_, err = AdjustEnterpriseReservation(sub, r2, 0, EnterpriseReservationRefunded, false)
			require.NoError(t, err)
			require.EqualValues(t, 1700, enterpriseTestBalanced(t, ent.Id).MemberAvailable)

			// 可用额度不足本次预留：拒绝且不留痕；有在途时标记为在途占用。
			r3, r4 := rid("r3"), rid("r4")
			_, err = AdjustEnterpriseReservation(sub, r3, 1500, EnterpriseReservationReserved, true)
			require.NoError(t, err)
			_, err = AdjustEnterpriseReservation(sub, r4, 300, EnterpriseReservationReserved, true)
			var rejection *types.QuotaRejection
			require.True(t, errors.As(err, &rejection))
			require.Equal(t, EnterpriseQuotaSource, rejection.Source)
			require.Equal(t, types.QuotaReasonInFlight, rejection.RejectionReason())
			var count int64
			require.NoError(t, DB.Model(&EnterpriseReservation{}).Where("request_id = ?", r4).Count(&count).Error)
			require.EqualValues(t, 0, count)

			// 结算超出预留：成员可用额度为负，守恒不变；为负后拒绝新请求（含免费请求）。
			_, err = AdjustEnterpriseReservation(sub, r3, 1900, EnterpriseReservationSettled, false)
			require.NoError(t, err)
			bal = enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, -200, bal.MemberAvailable)
			require.EqualValues(t, 2200, bal.Consumed)
			_, err = AdjustEnterpriseReservation(sub, rid("r5"), 1, EnterpriseReservationReserved, true)
			require.True(t, errors.As(err, &rejection))
			require.Equal(t, types.QuotaReasonExhausted, rejection.RejectionReason())
			_, err = AdjustEnterpriseReservation(sub, rid("r6"), 0, EnterpriseReservationReserved, true)
			require.True(t, errors.As(err, &rejection))

			// 补额先抵欠额：补 150 仍为负，再补 100 后恢复。
			require.NoError(t, AllocateEnterpriseQuota(actor, ent.Id, member.Id, 150, ""))
			_, err = AdjustEnterpriseReservation(sub, rid("r7"), 1, EnterpriseReservationReserved, true)
			require.Error(t, err)
			require.NoError(t, AllocateEnterpriseQuota(actor, ent.Id, member.Id, 100, ""))
			r8 := rid("r8")
			_, err = AdjustEnterpriseReservation(sub, r8, 50, EnterpriseReservationReserved, true)
			require.NoError(t, err)

			// 主体不一致的终结被拒绝。
			otherMember, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			_, err = AdjustEnterpriseReservation(enterpriseTestSubject(otherMember, 0), r8, 0, EnterpriseReservationRefunded, false)
			require.ErrorIs(t, err, ErrEnterpriseSubject)
			_, err = AdjustEnterpriseReservation(EnterpriseSubject{EnterpriseId: ent.Id, MemberId: member.Id, UserId: owner.Id}, rid("r9"), 1, EnterpriseReservationReserved, true)
			require.ErrorIs(t, err, ErrEnterpriseSubject)

			// 停用成员：新请求拒绝，在途仍按原企业结算，残余归并回未分配。
			require.NoError(t, DisableEnterpriseMember(actor, ent.Id, member.Id, ""))
			_, err = AdjustEnterpriseReservation(sub, rid("r10"), 1, EnterpriseReservationReserved, true)
			require.ErrorIs(t, err, ErrEnterpriseMemberInactive)
			_, err = AdjustEnterpriseReservation(sub, r8, 20, EnterpriseReservationSettled, false)
			require.NoError(t, err)
			bal = enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 30, bal.MemberAvailable)
			n, err := SweepInactiveEnterpriseResidues(100)
			require.NoError(t, err)
			require.Equal(t, 1, n)
			bal = enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 0, bal.MemberAvailable)
			require.EqualValues(t, 2220, bal.Consumed)
			require.EqualValues(t, 10000-2220, bal.Unallocated)

			// 全程不触碰个人钱包。
			var reloaded User
			require.NoError(t, DB.First(&reloaded, user.Id).Error)
			require.Equal(t, 0, reloaded.Quota)
		})
	}
}

func TestEnterpriseChatAdmissionAndTokenCap(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, owner := enterpriseTestOrg(t)
			actor := EnterpriseActor{UserId: owner.Id}
			member, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			_, err := CreditEnterprise(1, ent.Id, 100000, "V-1", "")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(actor, ent.Id, member.Id, 1000, ""))
			sub := enterpriseTestSubject(member, 4242)

			// 可用额度高于信任线：只占用有界首笔，而不是完整预估。
			r1 := "chat-" + common.GetUUID()[:8]
			res, err := ReserveEnterpriseChatQuota(sub, r1, 5000, 40, 100)
			require.NoError(t, err)
			require.EqualValues(t, 40, res.Quota)
			// 重复准入沿用原占用。
			res, err = ReserveEnterpriseChatQuota(sub, r1, 5000, 40, 100)
			require.NoError(t, err)
			require.EqualValues(t, 40, res.Quota)
			require.EqualValues(t, 40, enterpriseTestBalanced(t, ent.Id).MemberReserved)
			// 真实费用高于占用：结算后可用额度为 1000-700。
			_, err = AdjustEnterpriseReservation(sub, r1, 700, EnterpriseReservationSettled, false)
			require.NoError(t, err)
			require.EqualValues(t, 300, enterpriseTestBalanced(t, ent.Id).MemberAvailable)

			// 令牌上限记在企业账本，与成员预算同事务检查。
			require.NoError(t, SetEnterpriseTokenLimit(sub, 800))
			_, err = AdjustEnterpriseReservation(sub, "cap-"+common.GetUUID()[:8], 150, EnterpriseReservationReserved, true)
			var rejection *types.QuotaRejection
			require.True(t, errors.As(err, &rejection))
			require.ErrorIs(t, err, ErrEnterpriseTokenCap)
			r2 := "cap-" + common.GetUUID()[:8]
			_, err = AdjustEnterpriseReservation(sub, r2, 100, EnterpriseReservationReserved, true)
			require.NoError(t, err)
			budgets, err := GetEnterpriseTokenBudgets([]int{4242})
			require.NoError(t, err)
			require.EqualValues(t, 800, budgets[4242].UsedQuota)
			// 退款归还令牌已用量与成员预算。
			_, err = AdjustEnterpriseReservation(sub, r2, 0, EnterpriseReservationRefunded, false)
			require.NoError(t, err)
			budgets, err = GetEnterpriseTokenBudgets([]int{4242})
			require.NoError(t, err)
			require.EqualValues(t, 700, budgets[4242].UsedQuota)
			require.EqualValues(t, 300, enterpriseTestBalanced(t, ent.Id).MemberAvailable)

			// 事后调整按事件键幂等：违规费补扣、任务退款。
			fee := "fee-" + common.GetUUID()[:8]
			_, err = ApplyEnterpriseCharge(sub, fee, EnterpriseChargeViolationFee, 50)
			require.NoError(t, err)
			again, err := ApplyEnterpriseCharge(sub, fee, EnterpriseChargeViolationFee, 50)
			require.NoError(t, err)
			require.False(t, again.Changed)
			_, err = ApplyEnterpriseCharge(sub, fee, EnterpriseChargeViolationFee, 60)
			require.ErrorIs(t, err, ErrEnterpriseFinalized)
			_, err = ApplyEnterpriseCharge(sub, "refund-"+common.GetUUID()[:8], EnterpriseChargeTaskAdjust, -200)
			require.NoError(t, err)
			bal := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 450, bal.MemberAvailable)
			require.EqualValues(t, 550, bal.Consumed)
		})
	}
}

func TestEnterpriseConcurrentReservationsNeverOverspend(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, owner := enterpriseTestOrg(t)
			member, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			_, err := CreditEnterprise(1, ent.Id, 1000, "V-1", "")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(EnterpriseActor{UserId: owner.Id}, ent.Id, member.Id, 55, ""))
			sub := enterpriseTestSubject(member, 0)

			// 20 个并发请求各预留 10，成员只有 55：恰好 5 个成功。
			var wg sync.WaitGroup
			type outcome struct {
				id  string
				err error
			}
			results := make(chan outcome, 20)
			for i := 0; i < 20; i++ {
				wg.Add(1)
				id := fmt.Sprintf("conc-%d-%s", i, common.GetUUID()[:8])
				go func() {
					defer wg.Done()
					_, err := AdjustEnterpriseReservation(sub, id, 10, EnterpriseReservationReserved, true)
					results <- outcome{id, err}
				}()
			}
			wg.Wait()
			close(results)
			var admitted []string
			for r := range results {
				if r.err == nil {
					admitted = append(admitted, r.id)
					continue
				}
				var rejection *types.QuotaRejection
				require.True(t, errors.As(r.err, &rejection), "unexpected error: %v", r.err)
			}
			require.Len(t, admitted, 5)
			bal := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 5, bal.MemberAvailable)
			require.EqualValues(t, 50, bal.MemberReserved)

			// 并发重复终结同一批请求：每个请求只生效一次。
			for _, id := range admitted {
				for j := 0; j < 4; j++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, _ = AdjustEnterpriseReservation(sub, id, 7, EnterpriseReservationSettled, false)
					}()
				}
			}
			wg.Wait()
			bal = enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 20, bal.MemberAvailable)
			require.EqualValues(t, 0, bal.MemberReserved)
			require.EqualValues(t, 35, bal.Consumed)
		})
	}
}

func TestEnterpriseStaleReservationGoesToReview(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, owner := enterpriseTestOrg(t)
			member, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			_, err := CreditEnterprise(1, ent.Id, 1000, "V-1", "")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(EnterpriseActor{UserId: owner.Id}, ent.Id, member.Id, 500, ""))
			sub := enterpriseTestSubject(member, 0)
			stale, late, fresh := "stale-"+common.GetUUID()[:8], "late-"+common.GetUUID()[:8], "fresh-"+common.GetUUID()[:8]
			for _, id := range []string{stale, late, fresh} {
				_, err = AdjustEnterpriseReservation(sub, id, 100, EnterpriseReservationReserved, true)
				require.NoError(t, err)
			}
			require.NoError(t, DB.Model(&EnterpriseReservation{}).Where("request_id IN ?", []string{stale, late}).Update("updated_at", time.Now().Unix()-7200).Error)

			// 模拟进程崩溃：超时未终结的预留转待对账，不自动退款，新近的预留不受影响。
			n, err := MarkStaleEnterpriseReservations(time.Now().Unix()-3600, 100)
			require.NoError(t, err)
			require.EqualValues(t, 2, n)
			bal := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 300, bal.MemberReserved)
			require.EqualValues(t, 200, bal.MemberAvailable)
			items, total, err := ListEnterpriseReviewReservations(ent.Id, 0, 10)
			require.NoError(t, err)
			require.EqualValues(t, 2, total)
			require.Len(t, items, 2)

			// 迟到的真实结算仍然生效。
			_, err = AdjustEnterpriseReservation(sub, late, 60, EnterpriseReservationSettled, false)
			require.NoError(t, err)
			// 平台人工处理：必须写原因；全额释放后不可再处理。
			var row EnterpriseReservation
			require.NoError(t, DB.Where("request_id = ?", stale).First(&row).Error)
			require.ErrorIs(t, ResolveEnterpriseReservation(1, row.Id, false, 0, " "), ErrEnterpriseInvalidInput)
			require.NoError(t, ResolveEnterpriseReservation(1, row.Id, false, 0, "上游确认未执行"))
			require.ErrorIs(t, ResolveEnterpriseReservation(1, row.Id, true, 10, "重复处理"), ErrEnterpriseFinalized)
			bal = enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 100, bal.MemberReserved)
			require.EqualValues(t, 340, bal.MemberAvailable)
			require.EqualValues(t, 60, bal.Consumed)
		})
	}
}

func TestEnterpriseQuotaRequestApproval(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, owner := enterpriseTestOrg(t)
			ownerActor := EnterpriseActor{UserId: owner.Id}
			admin, adminUser := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleAdmin, 0)
			_, admin2User := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleAdmin, 0)
			member, memberUser := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			adminActor, admin2Actor := EnterpriseActor{UserId: adminUser.Id}, EnterpriseActor{UserId: admin2User.Id}
			_, err := CreditEnterprise(1, ent.Id, 7000, "V-1", "")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(adminActor, ent.Id, member.Id, 1500, ""))

			// 同时只能有一个待审批。
			req, err := CreateEnterpriseQuotaRequest(memberUser.Id, ent.Id, 800, "月底冲刺")
			require.NoError(t, err)
			_, err = CreateEnterpriseQuotaRequest(memberUser.Id, ent.Id, 100, "")
			require.ErrorIs(t, err, ErrEnterpriseRequestPending)
			pending, err := CountEnterprisePendingRequests(ent.Id)
			require.NoError(t, err)
			require.EqualValues(t, 1, pending)

			// 普通成员不能审批；批准金额不能超过申请金额。
			_, err = ReviewEnterpriseQuotaRequest(EnterpriseActor{UserId: memberUser.Id}, ent.Id, req.Id, true, 0, "")
			require.ErrorIs(t, err, ErrEnterpriseSelfOperation)
			_, err = ReviewEnterpriseQuotaRequest(adminActor, ent.Id, req.Id, true, 801, "")
			require.ErrorIs(t, err, ErrEnterpriseInvalidInput)

			// 两名管理员并发审批：只成功一次，企业资产不因审批增加。
			var wg sync.WaitGroup
			results := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				actor := adminActor
				if i%2 == 1 {
					actor = admin2Actor
				}
				go func() {
					defer wg.Done()
					_, err := ReviewEnterpriseQuotaRequest(actor, ent.Id, req.Id, true, 0, "同意")
					results <- err
				}()
			}
			wg.Wait()
			close(results)
			ok := 0
			for err := range results {
				if err == nil {
					ok++
				} else {
					require.ErrorIs(t, err, ErrEnterpriseRequestState)
				}
			}
			require.Equal(t, 1, ok)
			bal := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 2300, bal.MemberAvailable)
			require.EqualValues(t, 4700, bal.Unallocated)
			require.EqualValues(t, 7000, bal.NetCredited)

			// 批准金额可以改小；处理后可以再次申请。
			req2, err := CreateEnterpriseQuotaRequest(memberUser.Id, ent.Id, 1000, "")
			require.NoError(t, err)
			reviewed, err := ReviewEnterpriseQuotaRequest(adminActor, ent.Id, req2.Id, true, 400, "先批一部分")
			require.NoError(t, err)
			require.EqualValues(t, 400, reviewed.ApprovedAmount)
			require.EqualValues(t, 4300, enterpriseTestBalanced(t, ent.Id).Unallocated)

			// 管理员的申请不能自批，也不能由另一名管理员批，由所有者处理。
			adminReq, err := CreateEnterpriseQuotaRequest(adminUser.Id, ent.Id, 200, "")
			require.NoError(t, err)
			_, err = ReviewEnterpriseQuotaRequest(adminActor, ent.Id, adminReq.Id, true, 0, "")
			require.ErrorIs(t, err, ErrEnterpriseSelfOperation)
			_, err = ReviewEnterpriseQuotaRequest(admin2Actor, ent.Id, adminReq.Id, true, 0, "")
			require.ErrorIs(t, err, ErrEnterpriseForbidden)
			_, err = ReviewEnterpriseQuotaRequest(ownerActor, ent.Id, adminReq.Id, true, 0, "")
			require.NoError(t, err)
			got, err := GetEnterpriseMember(ent.Id, admin.Id)
			require.NoError(t, err)
			require.EqualValues(t, 200, got.Available)

			// 未分配不足：保持待审批并返回明确原因；之后可拒绝。
			req3, err := CreateEnterpriseQuotaRequest(memberUser.Id, ent.Id, 99999, "")
			require.NoError(t, err)
			_, err = ReviewEnterpriseQuotaRequest(adminActor, ent.Id, req3.Id, true, 0, "")
			require.ErrorIs(t, err, ErrEnterpriseInsufficientUnallocated)
			var still EnterpriseQuotaRequest
			require.NoError(t, DB.First(&still, req3.Id).Error)
			require.Equal(t, EnterpriseRequestPending, still.Status)
			_, err = ReviewEnterpriseQuotaRequest(adminActor, ent.Id, req3.Id, false, 0, "金额过大")
			require.NoError(t, err)

			// 撤销本人待办；过期待办不可批并让出槽位；跨企业不可见。
			req4, err := CreateEnterpriseQuotaRequest(memberUser.Id, ent.Id, 10, "")
			require.NoError(t, err)
			require.ErrorIs(t, CancelEnterpriseQuotaRequest(adminUser.Id, ent.Id, req4.Id), ErrEnterpriseRequestState)
			require.NoError(t, CancelEnterpriseQuotaRequest(memberUser.Id, ent.Id, req4.Id))
			req5, err := CreateEnterpriseQuotaRequest(memberUser.Id, ent.Id, 10, "")
			require.NoError(t, err)
			require.NoError(t, DB.Model(&EnterpriseQuotaRequest{}).Where("id = ?", req5.Id).Update("expires_at", time.Now().Unix()-1).Error)
			_, err = ReviewEnterpriseQuotaRequest(adminActor, ent.Id, req5.Id, true, 0, "")
			require.ErrorIs(t, err, ErrEnterpriseRequestState)
			req6, err := CreateEnterpriseQuotaRequest(memberUser.Id, ent.Id, 10, "")
			require.NoError(t, err)
			entB, _, ownerB := enterpriseTestOrg(t)
			_, err = ReviewEnterpriseQuotaRequest(EnterpriseActor{UserId: ownerB.Id}, entB.Id, req6.Id, true, 0, "")
			require.ErrorIs(t, err, ErrEnterpriseNotFound)

			// 成员停用使待办失效。
			require.NoError(t, DisableEnterpriseMember(adminActor, ent.Id, member.Id, ""))
			var lapsed EnterpriseQuotaRequest
			require.NoError(t, DB.First(&lapsed, req6.Id).Error)
			require.Equal(t, EnterpriseRequestExpired, lapsed.Status)
			enterpriseTestBalanced(t, ent.Id)
		})
	}
}
