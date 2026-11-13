package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseInvitationRejectsSelfGrantAndLegacyReplay(t *testing.T) {
	enterpriseTestDB(t, "sqlite")
	ent, _, owner := enterpriseTestOrg(t)
	_, err := CreditEnterprise(1, ent.Id, 100000, "handoff-credit", "")
	require.NoError(t, err)
	admin, user := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleAdmin, 0)
	require.NoError(t, SetEnterpriseMemberModels(EnterpriseActor{UserId: owner.Id}, ent.Id, admin.Id, EnterpriseModelModeCustom, []string{}))
	_, _, err = CreateEnterpriseInvitation(EnterpriseActor{UserId: user.Id}, ent.Id, user.Email, EnterpriseRoleMember, 1000)
	require.ErrorIs(t, err, ErrEnterpriseSelfOperation)
	// Simulate an outstanding invitation written by an earlier version.
	token, err := DesktopLoginToken()
	require.NoError(t, err)
	old := EnterpriseInvitation{Digest: enterpriseInvitationDigest(token), EnterpriseId: ent.Id, Email: user.Email, Role: EnterpriseRoleMember, InitialQuota: 1000, InviterId: user.Id, Status: EnterpriseInvitationPending, ExpiresAt: time.Now().Unix() + 3600, CreatedAt: time.Now().Unix()}
	require.NoError(t, DB.Create(&old).Error)
	require.NoError(t, LeaveEnterprise(user.Id, ent.Id))
	_, err = AcceptEnterpriseInvitation(token, user.Id)
	require.Error(t, err)
	active, err := GetActiveEnterpriseMemberByUser(user.Id)
	require.NoError(t, err)
	require.Nil(t, active)
	require.EqualValues(t, 100000, enterpriseTestBalanced(t, ent.Id).Unallocated)
}

func TestEnterpriseInvitationRequiresCurrentInviterAuthority(t *testing.T) {
	for _, change := range []string{"leave", "disable", "demote", "account_disabled"} {
		t.Run(change, func(t *testing.T) {
			enterpriseTestDB(t, "sqlite")
			ent, _, owner := enterpriseTestOrg(t)
			admin, user := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleAdmin, 0)
			target := enterpriseTestUser(t, 10)
			invite, _, err := CreateEnterpriseInvitation(EnterpriseActor{UserId: user.Id}, ent.Id, target.Email, EnterpriseRoleMember, 0)
			require.NoError(t, err)
			switch change {
			case "leave":
				require.NoError(t, LeaveEnterprise(user.Id, ent.Id))
			case "disable":
				require.NoError(t, DisableEnterpriseMember(EnterpriseActor{UserId: owner.Id}, ent.Id, admin.Id, "test"))
			case "demote":
				require.NoError(t, SetEnterpriseMemberRole(EnterpriseActor{UserId: owner.Id}, ent.Id, admin.Id, EnterpriseRoleMember))
			case "account_disabled":
				require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("status", common.UserStatusDisabled).Error)
			}
			_, err = AcceptEnterpriseInvitation(invite, target.Id)
			require.Error(t, err)
			active, err := GetActiveEnterpriseMemberByUser(target.Id)
			require.NoError(t, err)
			require.Nil(t, active)
		})
	}
}

func TestEnterpriseInvitationGenerationAndOwnershipChanges(t *testing.T) {
	enterpriseTestDB(t, "sqlite")
	ent, _, owner := enterpriseTestOrg(t)
	admin, user := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleAdmin, 0)
	target := enterpriseTestUser(t, 0)
	token, inv, err := CreateEnterpriseInvitation(EnterpriseActor{UserId: user.Id}, ent.Id, target.Email, EnterpriseRoleMember, 0)
	require.NoError(t, err)
	require.Equal(t, admin.Id, inv.InviterMemberId)
	require.NoError(t, SetEnterpriseMemberRole(EnterpriseActor{UserId: owner.Id}, ent.Id, admin.Id, EnterpriseRoleMember))
	require.NoError(t, SetEnterpriseMemberRole(EnterpriseActor{UserId: owner.Id}, ent.Id, admin.Id, EnterpriseRoleAdmin))
	_, err = AcceptEnterpriseInvitation(token, target.Id)
	require.Error(t, err, "restoring role must not revive revoked invitation")
	// Old pending records without generation cannot silently acquire today's privileges.
	legacy, legacyRow, err := CreateEnterpriseInvitation(EnterpriseActor{UserId: owner.Id}, ent.Id, target.Email, EnterpriseRoleMember, 0)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&EnterpriseInvitation{}).Where("id = ?", legacyRow.Id).Update("inviter_member_id", 0).Error)
	_, err = AcceptEnterpriseInvitation(legacy, target.Id)
	require.ErrorIs(t, err, ErrEnterpriseInvitation)
	ownerInvite, _, err := CreateEnterpriseInvitation(EnterpriseActor{UserId: owner.Id}, ent.Id, target.Email, EnterpriseRoleAdmin, 0)
	require.NoError(t, err)
	require.NoError(t, TransferEnterpriseOwnership(EnterpriseActor{UserId: owner.Id}, ent.Id, admin.Id))
	_, err = AcceptEnterpriseInvitation(ownerInvite, target.Id)
	require.Error(t, err)
	fresh, _, err := CreateEnterpriseInvitation(EnterpriseActor{UserId: user.Id}, ent.Id, target.Email, EnterpriseRoleAdmin, 0)
	require.NoError(t, err)
	accepted, err := AcceptEnterpriseInvitation(fresh, target.Id)
	require.NoError(t, err)
	require.Equal(t, EnterpriseRoleAdmin, accepted.Member.Role)
}
