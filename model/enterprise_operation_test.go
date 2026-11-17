package model

import (
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func operationRoot(t *testing.T) EnterpriseActor {
	u := enterpriseTestUser(t, 0)
	require.NoError(t, DB.Model(u).Update("role", common.RoleRootUser).Error)
	return EnterpriseActor{UserId: u.Id, Platform: true}
}

func TestEnterpriseOperationReplay(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, ownerMember, owner := enterpriseTestOrg(t)
			root := operationRoot(t)
			actor := EnterpriseActor{UserId: owner.Id}
			creditKey := common.GetUUID()
			first, err := CreditEnterpriseWithKey(root.UserId, ent.Id, 10000, "receipt-"+common.GetUUID(), "deposit", creditKey)
			require.NoError(t, err)
			require.NotEmpty(t, first.OperationID)
			// Same independent intent with different keys must still execute twice.
			key := common.GetUUID()
			a, err := AdjustEnterpriseWithKey(root.UserId, ent.Id, 300, "correction", key)
			require.NoError(t, err)
			b, err := AdjustEnterpriseWithKey(root.UserId, ent.Id, 300, "correction", key)
			require.NoError(t, err)
			require.True(t, b.Replayed)
			require.Equal(t, a.OperationID, b.OperationID)
			_, err = AdjustEnterpriseWithKey(root.UserId, ent.Id, 301, "correction", key)
			require.ErrorIs(t, err, ErrEnterpriseOperationConflict)
			_, err = AdjustEnterpriseWithKey(root.UserId, ent.Id, 300, "another reason", key)
			require.ErrorIs(t, err, ErrEnterpriseOperationConflict)
			_, err = AdjustEnterpriseWithKey(root.UserId, ent.Id, 300, "correction", common.GetUUID())
			require.NoError(t, err)
			require.EqualValues(t, 10600, enterpriseTestBalanced(t, ent.Id).NetCredited)
			allocKey := common.GetUUID()
			a, err = AllocateEnterpriseQuotaWithKey(actor, ent.Id, ownerMember.Id, 299, "self budget", allocKey)
			require.NoError(t, err)
			b, err = AllocateEnterpriseQuotaWithKey(actor, ent.Id, ownerMember.Id, 299, "self budget", allocKey)
			require.NoError(t, err)
			require.Equal(t, a.OperationID, b.OperationID)
			require.EqualValues(t, 299, b.MemberAvailableAfter)
			// A later balance does not invalidate replay or result retrieval.
			require.NoError(t, AllocateEnterpriseQuota(actor, ent.Id, ownerMember.Id, 1, ""))
			got, err := GetEnterpriseOperation(actor, ent.Id, allocKey)
			require.NoError(t, err)
			require.EqualValues(t, 299, got.MemberAvailableAfter)
			snapshot := int64(299)
			_, err = ReclaimEnterpriseQuotaSnapshot(actor, ent.Id, ownerMember.Id, 299, "all", common.GetUUID(), &snapshot)
			require.ErrorIs(t, err, ErrEnterpriseBalanceChanged)
			require.EqualValues(t, 300, enterpriseTestBalanced(t, ent.Id).MemberAvailable)
			snapshot = 300
			reclaimKey := common.GetUUID()
			a, err = ReclaimEnterpriseQuotaSnapshot(actor, ent.Id, ownerMember.Id, 300, "all", reclaimKey, &snapshot)
			require.NoError(t, err)
			b, err = ReclaimEnterpriseQuotaSnapshot(actor, ent.Id, ownerMember.Id, 300, "all", reclaimKey, &snapshot)
			require.NoError(t, err)
			require.Equal(t, a.OperationID, b.OperationID)
			require.Zero(t, enterpriseTestBalanced(t, ent.Id).MemberAvailable)
			other, _, _ := enterpriseTestOrg(t)
			_, err = AdjustEnterpriseWithKey(root.UserId, other.Id, 300, "correction", key)
			require.ErrorIs(t, err, ErrEnterpriseOperationConflict)
			require.Zero(t, enterpriseTestBalanced(t, other.Id).NetCredited)
			// Current authority applies before a replay is exposed.
			require.NoError(t, DB.Model(&User{}).Where("id = ?", root.UserId).Update("role", common.RoleCommonUser).Error)
			_, err = AdjustEnterpriseWithKey(root.UserId, ent.Id, 300, "correction", key)
			require.ErrorIs(t, err, ErrEnterpriseForbidden)
			_, err = GetEnterpriseOperation(root, ent.Id, key)
			require.ErrorIs(t, err, ErrEnterpriseForbidden)
			require.NoError(t, DB.Model(&User{}).Where("id = ?", root.UserId).Update("role", common.RoleRootUser).Error)
			require.NoError(t, DB.Model(&Enterprise{}).Where("id = ?", ent.Id).Update("status", EnterpriseStatusArchived).Error)
			got, err = AdjustEnterpriseWithKey(root.UserId, ent.Id, 300, "correction", key)
			require.NoError(t, err)
			require.True(t, got.Replayed)
			_, err = AdjustEnterpriseWithKey(root.UserId, ent.Id, 1, "new", common.GetUUID())
			require.ErrorIs(t, err, ErrEnterpriseArchived)
		})
	}
}

func TestEnterpriseOperationConcurrentAndAtomic(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, member, owner := enterpriseTestOrg(t)
			root := operationRoot(t)
			_, err := CreditEnterprise(root.UserId, ent.Id, 10000, common.GetUUID(), "")
			require.NoError(t, err)
			key := common.GetUUID()
			actor := EnterpriseActor{UserId: owner.Id}
			var wg sync.WaitGroup
			results := make(chan *EnterpriseOperationResult, 20)
			errs := make(chan error, 20)
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					r, e := AllocateEnterpriseQuotaWithKey(actor, ent.Id, member.Id, 299, "budget", key)
					results <- r
					errs <- e
				}()
			}
			wg.Wait()
			close(results)
			close(errs)
			for e := range errs {
				require.NoError(t, e)
			}
			ids := map[string]bool{}
			original := 0
			for r := range results {
				require.NotNil(t, r)
				ids[r.OperationID] = true
				if !r.Replayed {
					original++
				}
			}
			require.Len(t, ids, 1)
			require.Equal(t, 1, original)
			require.EqualValues(t, 299, enterpriseTestBalanced(t, ent.Id).MemberAvailable)
			// Inject failures at audit creation and final receipt update: neither money
			// nor the unique key may survive the rollback.
			for _, stage := range []string{"audit", "receipt"} {
				t.Run(stage, func(t *testing.T) {
					fail := errors.New("injected financial write failure")
					name := "enterprise-op-" + stage
					if stage == "audit" {
						require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
							if tx.Statement.Table == "enterprise_audit_events" {
								tx.AddError(fail)
							}
						}))
						defer DB.Callback().Create().Remove(name)
					} else {
						require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
							if tx.Statement.Table == "enterprise_operations" {
								tx.AddError(fail)
							}
						}))
						defer DB.Callback().Update().Remove(name)
					}
					failKey := common.GetUUID()
					_, e := AllocateEnterpriseQuotaWithKey(actor, ent.Id, member.Id, 1, "atomic", failKey)
					require.ErrorIs(t, e, fail)
					require.EqualValues(t, 299, enterpriseTestBalanced(t, ent.Id).MemberAvailable)
					got, e := GetEnterpriseOperation(actor, ent.Id, failKey)
					require.NoError(t, e)
					require.Nil(t, got)
				})
			}
		})
	}
}

func TestEnterpriseOperationReservationAtomic(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, member, owner := enterpriseTestOrg(t)
			root := operationRoot(t)
			_, err := CreditEnterprise(root.UserId, ent.Id, 1000, common.GetUUID(), "")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(EnterpriseActor{UserId: owner.Id}, ent.Id, member.Id, 1000, ""))
			r, err := AdjustEnterpriseReservation(enterpriseTestSubject(member, 0), common.GetUUID(), 299, EnterpriseReservationReserved, true)
			require.NoError(t, err)
			require.NoError(t, DB.Model(&r).Update("status", EnterpriseReservationReview).Error)
			fail := errors.New("audit unavailable")
			require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("resolve-audit", func(tx *gorm.DB) {
				if tx.Statement.Table == "enterprise_audit_events" {
					tx.AddError(fail)
				}
			}))
			key := common.GetUUID()
			_, err = ResolveEnterpriseReservationWithKey(root.UserId, r.Id, true, 1, "verified", key)
			require.ErrorIs(t, err, fail)
			require.NoError(t, DB.Callback().Create().Remove("resolve-audit"))
			bal := enterpriseTestBalanced(t, ent.Id)
			require.EqualValues(t, 299, bal.MemberReserved)
			require.Zero(t, bal.Consumed)
			a, err := ResolveEnterpriseReservationWithKey(root.UserId, r.Id, true, 1, "verified", key)
			require.NoError(t, err)
			b, err := ResolveEnterpriseReservationWithKey(root.UserId, r.Id, true, 1, "verified", key)
			require.NoError(t, err)
			require.Equal(t, a.OperationID, b.OperationID)
			bal = enterpriseTestBalanced(t, ent.Id)
			require.Zero(t, bal.MemberReserved)
			require.EqualValues(t, 1, bal.Consumed)
			var count int64
			require.NoError(t, DB.Model(&EnterpriseAuditEvent{}).Where("enterprise_id = ? AND action = ?", ent.Id, "reservation.resolve").Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}

func TestEnterpriseOperationOverflowAndAuthority(t *testing.T) {
	enterpriseTestDB(t, "sqlite")
	ent, ownerMember, owner := enterpriseTestOrg(t)
	root := operationRoot(t)
	_, err := enterpriseCheckedAdd(math.MaxInt64, 1)
	require.ErrorIs(t, err, ErrEnterpriseAmountOverflow)
	_, err = enterpriseCheckedAdd(math.MinInt64, -1)
	require.ErrorIs(t, err, ErrEnterpriseAmountOverflow)
	require.NoError(t, DB.Model(ent).Updates(map[string]any{"net_credited": int64(math.MaxInt64), "unallocated": int64(math.MaxInt64)}).Error)
	_, err = CreditEnterpriseWithKey(root.UserId, ent.Id, 1, common.GetUUID(), "", common.GetUUID())
	require.ErrorIs(t, err, ErrEnterpriseAmountOverflow)
	_, err = CreditEnterpriseWithKey(root.UserId, ent.Id, 1, common.GetUUID(), "", "bad")
	require.ErrorIs(t, err, ErrEnterpriseOperationInvalid)
	// An owner can act, but a former generation and a disabled account cannot replay.
	key := common.GetUUID()
	actor := EnterpriseActor{UserId: owner.Id}
	_, err = AllocateEnterpriseQuotaWithKey(actor, ent.Id, ownerMember.Id, 1, "", key)
	require.NoError(t, err)
	require.NoError(t, DB.Model(owner).Update("status", common.UserStatusDisabled).Error)
	_, err = GetEnterpriseOperation(actor, ent.Id, key)
	require.ErrorIs(t, err, ErrEnterpriseForbidden)
	require.NoError(t, DB.Model(owner).Update("status", common.UserStatusEnabled).Error)
	require.NoError(t, DB.Model(&EnterpriseOperation{}).Where("actor_id = ?", owner.Id).Update("actor_member_id", ownerMember.Id+999).Error)
	_, err = GetEnterpriseOperation(actor, ent.Id, key)
	require.ErrorIs(t, err, ErrEnterpriseForbidden)
	// Invalid audit serialization is an error rather than an empty audit snapshot.
	require.Error(t, DB.Transaction(func(tx *gorm.DB) error {
		return appendEnterpriseAuditTx(tx, ent.Id, actor, nil, "test", "enterprise", ent.Id, make(chan int), nil, "")
	}))
}

func TestEnterpriseOperationEveryMutationConcurrent(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, member, owner := enterpriseTestOrg(t)
			root := operationRoot(t)
			_, err := CreditEnterprise(root.UserId, ent.Id, 10000, common.GetUUID(), "")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(EnterpriseActor{UserId: owner.Id}, ent.Id, member.Id, 1000, ""))
			for _, action := range []string{"credit", "adjust", "reclaim", "resolve"} {
				t.Run(action, func(t *testing.T) {
					key, voucher := common.GetUUID(), common.GetUUID()
					before := enterpriseTestBalanced(t, ent.Id)
					var reservation EnterpriseReservation
					if action == "resolve" {
						reservation, err = AdjustEnterpriseReservation(enterpriseTestSubject(member, 0), common.GetUUID(), 1, EnterpriseReservationReserved, true)
						require.NoError(t, err)
						require.NoError(t, DB.Model(&reservation).Update("status", EnterpriseReservationReview).Error)
					}
					var wg sync.WaitGroup
					errs := make(chan error, 20)
					ids := make(chan string, 20)
					for i := 0; i < 20; i++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							var result *EnterpriseOperationResult
							var e error
							switch action {
							case "credit":
								result, e = CreditEnterpriseWithKey(root.UserId, ent.Id, 1, voucher, "", key)
							case "adjust":
								result, e = AdjustEnterpriseWithKey(root.UserId, ent.Id, -1, "correct", key)
							case "reclaim":
								result, e = ReclaimEnterpriseQuotaWithKey(EnterpriseActor{UserId: owner.Id}, ent.Id, member.Id, 1, "", key)
							case "resolve":
								result, e = ResolveEnterpriseReservationWithKey(root.UserId, reservation.Id, false, 0, "verified", key)
							}
							errs <- e
							if result != nil {
								ids <- result.OperationID
							}
						}()
					}
					wg.Wait()
					close(errs)
					close(ids)
					for e := range errs {
						require.NoError(t, e)
					}
					seen := map[string]bool{}
					for id := range ids {
						seen[id] = true
					}
					require.Len(t, seen, 1)
					after := enterpriseTestBalanced(t, ent.Id)
					switch action {
					case "credit":
						require.Equal(t, before.NetCredited+1, after.NetCredited)
					case "adjust":
						require.Equal(t, before.NetCredited-1, after.NetCredited)
					case "reclaim":
						require.Equal(t, before.MemberAvailable-1, after.MemberAvailable)
					case "resolve":
						require.Equal(t, before.MemberAvailable, after.MemberAvailable)
						require.Zero(t, after.MemberReserved)
					}
				})
			}
		})
	}
}

func TestEnterpriseOperationRejoinedAdminCannotReplay(t *testing.T) {
	enterpriseTestDB(t, "sqlite")
	ent, _, owner := enterpriseTestOrg(t)
	admin, user := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleAdmin, 0)
	member, _ := enterpriseTestJoin(t, ent, owner.Id, EnterpriseRoleMember, 0)
	_, err := CreditEnterprise(1, ent.Id, 1000, common.GetUUID(), "")
	require.NoError(t, err)
	key := common.GetUUID()
	actor := EnterpriseActor{UserId: user.Id}
	_, err = AllocateEnterpriseQuotaWithKey(actor, ent.Id, member.Id, 1, "", key)
	require.NoError(t, err)
	require.NoError(t, DisableEnterpriseMember(EnterpriseActor{UserId: owner.Id}, ent.Id, admin.Id, ""))
	invite, _, err := CreateEnterpriseInvitation(EnterpriseActor{UserId: owner.Id}, ent.Id, user.Email, EnterpriseRoleAdmin, 0)
	require.NoError(t, err)
	joined, err := AcceptEnterpriseInvitation(invite, user.Id)
	require.NoError(t, err)
	require.NotEqual(t, admin.Id, joined.Member.Id)
	_, err = AllocateEnterpriseQuotaWithKey(actor, ent.Id, member.Id, 1, "", key)
	require.ErrorIs(t, err, ErrEnterpriseForbidden)
	_, err = GetEnterpriseOperation(actor, ent.Id, key)
	require.ErrorIs(t, err, ErrEnterpriseForbidden)
	require.EqualValues(t, 1, enterpriseTestBalanced(t, ent.Id).MemberAvailable)
}

func TestEnterpriseOperationMigrationsKeepReceipts(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, _, _ := enterpriseTestOrg(t)
			root := operationRoot(t)
			key := common.GetUUID()
			first, err := AdjustEnterpriseWithKey(root.UserId, ent.Id, 1, "original", key)
			require.NoError(t, err)
			for i := 0; i < 2; i++ {
				require.NoError(t, MigrateEnterpriseTables(DB))
			}
			after, err := GetEnterpriseOperation(root, ent.Id, key)
			require.NoError(t, err)
			require.Equal(t, first.OperationID, after.OperationID)
			require.EqualValues(t, 1, after.UnallocatedAfter)
		})
	}
}

func TestEnterpriseOperationManualAutomaticRace(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			ent, member, owner := enterpriseTestOrg(t)
			root := operationRoot(t)
			_, err := CreditEnterprise(1, ent.Id, 1000, common.GetUUID(), "")
			require.NoError(t, err)
			require.NoError(t, AllocateEnterpriseQuota(EnterpriseActor{UserId: owner.Id}, ent.Id, member.Id, 1000, ""))
			sub := enterpriseTestSubject(member, 0)
			for i := 0; i < 6; i++ {
				request := common.GetUUID()
				r, e := AdjustEnterpriseReservation(sub, request, 10, EnterpriseReservationReserved, true)
				require.NoError(t, e)
				require.NoError(t, DB.Model(&r).Update("status", EnterpriseReservationReview).Error)
				start := make(chan struct{})
				errs := make(chan error, 2)
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					defer wg.Done()
					<-start
					_, e := ResolveEnterpriseReservationWithKey(root.UserId, r.Id, true, 1, "verified", common.GetUUID())
					errs <- e
				}()
				go func() {
					defer wg.Done()
					<-start
					_, e := AdjustEnterpriseReservation(sub, request, 1, EnterpriseReservationSettled, false)
					errs <- e
				}()
				close(start)
				wg.Wait()
				close(errs)
				for e := range errs {
					if e != nil {
						require.ErrorIs(t, e, ErrEnterpriseFinalized)
					}
				}
				b := enterpriseTestBalanced(t, ent.Id)
				require.EqualValues(t, i+1, b.Consumed)
				require.Zero(t, b.MemberReserved)
			}
		})
	}
}
