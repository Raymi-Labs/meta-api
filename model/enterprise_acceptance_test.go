package model

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEnterpriseIncrementalMigrationPreservesPersonalToken(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			// 独立临时表模拟升级前结构，不删除共享测试表或连接真实业务库。
			table := "enterprise_migration_" + common.GetUUID()[:12]
			fields := []reflect.StructField{}
			kind := reflect.TypeOf(Token{})
			for i := 0; i < kind.NumField(); i++ {
				field := kind.Field(i)
				if field.Name != "EnterpriseId" && field.Name != "MembershipId" {
					fields = append(fields, field)
				}
			}
			legacy := reflect.New(reflect.StructOf(fields)).Interface()
			require.NoError(t, DB.Table(table).AutoMigrate(legacy))
			t.Cleanup(func() { require.NoError(t, DB.Migrator().DropTable(table)) })
			require.NoError(t, DB.Table(table).Create(map[string]any{"id": 1, "user_id": 99, "key": "fixture-only", "name": "旧个人令牌", "remain_quota": 12345, "used_quota": 678, "unlimited_quota": false, "routing_config": "fixture-routing"}).Error)
			for i := 0; i < 2; i++ {
				require.NoError(t, DB.Table(table).AutoMigrate(&Token{}))
			}
			var got Token
			require.NoError(t, DB.Table(table).First(&got).Error)
			require.Zero(t, got.EnterpriseId)
			require.Zero(t, got.MembershipId)
			require.Equal(t, 12345, got.RemainQuota)
			require.Equal(t, 678, got.UsedQuota)
			require.Equal(t, "fixture-routing", got.RoutingConfig)
			require.Equal(t, "fixture-only", got.Key)

			inviteTable := "enterprise_inv_migration_" + common.GetUUID()[:12]
			fields = nil
			kind = reflect.TypeOf(EnterpriseInvitation{})
			for i := 0; i < kind.NumField(); i++ {
				if field := kind.Field(i); field.Name != "InviterMemberId" {
					fields = append(fields, field)
				}
			}
			require.NoError(t, DB.Table(inviteTable).AutoMigrate(reflect.New(reflect.StructOf(fields)).Interface()))
			t.Cleanup(func() { require.NoError(t, DB.Migrator().DropTable(inviteTable)) })
			inv := EnterpriseInvitation{Digest: common.GetUUID(), EnterpriseId: 7, Email: "fixture@example.invalid", Role: EnterpriseRoleMember, InviterId: 99, Status: EnterpriseInvitationPending, InitialQuota: 500}
			require.NoError(t, DB.Table(inviteTable).Omit("InviterMemberId").Create(&inv).Error)
			for i := 0; i < 2; i++ {
				require.NoError(t, DB.Table(inviteTable).AutoMigrate(&EnterpriseInvitation{}))
			}
			var migrated EnterpriseInvitation
			require.NoError(t, DB.Table(inviteTable).First(&migrated).Error)
			require.Zero(t, migrated.InviterMemberId, "旧邀请不能自动补签权限")
			require.Equal(t, inv.Digest, migrated.Digest)
			require.Equal(t, inv.InitialQuota, migrated.InitialQuota)
		})
	}
}

func TestEnterpriseTokenQuotaSearchUsesPayerBudget(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, member, user := enterpriseTestOrg(t)
			mint := func(limit int64) *Token {
				token := &Token{UserId: user.Id, EnterpriseId: ent.Id, MembershipId: member.Id, Name: "企业令牌", Key: common.GetUUID(), Group: "default"}
				require.NoError(t, CreateEnterpriseToken(token, limit))
				return token
			}
			limited, unlimited := mint(500), mint(0)
			require.NoError(t, DB.Model(&EnterpriseTokenBudget{}).Where("token_id = ?", limited.Id).Update("used_quota", 200).Error)
			require.NoError(t, DB.Model(&EnterpriseTokenBudget{}).Where("token_id = ?", unlimited.Id).Update("used_quota", 30).Error)
			personal := &Token{UserId: user.Id, Name: "个人令牌", Key: common.GetUUID(), RemainQuota: 400, UsedQuota: 100, Group: "default"}
			require.NoError(t, DB.Create(personal).Error)
			min, max, used := 500, 500, 150
			rows, total, err := SearchUserTokens(user.Id, "", "", 0, 10, "", &min, &max, nil, nil, "", "remain_quota", "desc", "")
			require.NoError(t, err)
			require.EqualValues(t, 2, total)
			require.Equal(t, []int{personal.Id, limited.Id}, []int{rows[0].Id, rows[1].Id})
			rows, total, err = SearchUserTokens(user.Id, "", "", 0, 10, "", &min, nil, &used, nil, "", "remain_quota", "asc", TokenPayerEnterprise)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Equal(t, limited.Id, rows[0].Id)
			rows, total, err = SearchUserTokens(user.Id, "", "", 0, 10, "", nil, nil, nil, nil, "", "remain_quota", "asc", "")
			require.NoError(t, err)
			require.EqualValues(t, 3, total)
			require.Equal(t, []int{unlimited.Id, limited.Id, personal.Id}, []int{rows[0].Id, rows[1].Id, rows[2].Id})
		})
	}
}

func TestEnterpriseOwnerRequiresUsableAccount(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, owner := enterpriseTestOrg(t)
			member, user := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
			actor := EnterpriseActor{UserId: owner.Id}
			require.ErrorIs(t, owner.Delete(), ErrEnterpriseOwnerRequired)
			require.ErrorIs(t, owner.HardDelete(), ErrEnterpriseOwnerRequired)
			require.NoError(t, DB.Model(user).Update("status", common.UserStatusDisabled).Error)
			require.ErrorIs(t, TransferEnterpriseOwnership(actor, ent.Id, member.Id), ErrEnterpriseOwnerAccount)
			require.NoError(t, DB.Model(user).Update("status", common.UserStatusEnabled).Error)
			require.NoError(t, user.Delete())
			require.ErrorIs(t, TransferEnterpriseOwnership(actor, ent.Id, member.Id), ErrEnterpriseOwnerAccount)
			stored, err := GetEnterpriseByID(ent.Id)
			require.NoError(t, err)
			require.Equal(t, owner.Id, stored.OwnerUserId)
		})
	}
}

func TestEnterpriseCreationSerializesWithOwnerDeletion(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			user := enterpriseTestUser(t, 999)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			original := DB
			DB = DB.WithContext(ctx)
			t.Cleanup(func() { DB = original })
			entered, proceed := make(chan struct{}), make(chan struct{})
			var release sync.Once
			defer release.Do(func() { close(proceed) })
			callback := "enterprise_owner_creation_fixture"
			require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "enterprises" {
					close(entered)
					select {
					case <-proceed:
					case <-ctx.Done():
						tx.AddError(ctx.Err())
					}
				}
			}))
			t.Cleanup(func() { require.NoError(t, original.Callback().Create().Remove(callback)) })
			created, deleted := make(chan error, 1), make(chan error, 1)
			go func() { _, err := CreateEnterprise(1, "并发开通", "default", "UTC", user.Id); created <- err }()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			go func() { deleted <- user.Delete() }()
			// 删除不能越过已读取Owner且尚未写企业的事务。
			select {
			case err := <-deleted:
				t.Errorf("删除在开通事务完成前返回: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			release.Do(func() { close(proceed) })
			require.NoError(t, <-created)
			if !t.Failed() {
				require.ErrorIs(t, <-deleted, ErrEnterpriseOwnerRequired)
			}
			var alive User
			require.NoError(t, DB.First(&alive, user.Id).Error)
		})
	}
}
