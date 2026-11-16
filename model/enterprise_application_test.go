package model

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnterpriseApplications(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			user := enterpriseTestUser(t, 0)
			for _, input := range []struct{ kind, name, typ, contact string }{
				{"opening", " ", "qq", "123"}, {"opening", strings.Repeat("企", 65), "qq", "123"},
				{"custom", "", "invalid", "123"}, {"custom", "", "qq", " "}, {"custom", "", "x", strings.Repeat("a", 201)},
			} {
				_, err := CreateEnterpriseApplication(user.Id, input.kind, input.name, input.typ, input.contact)
				require.ErrorIs(t, err, ErrEnterpriseInvalidInput)
			}
			app, err := CreateEnterpriseApplication(user.Id, "opening", " 企业 ", "wechat", " contact ")
			require.NoError(t, err)
			require.Equal(t, "企业", app.Name)
			require.Equal(t, "contact", app.Contact)
			_, err = CreateEnterpriseApplication(user.Id, "opening", "重复", "qq", "123")
			require.ErrorIs(t, err, ErrEnterpriseRequestPending)
			custom, err := CreateEnterpriseApplication(user.Id, "custom", "ignored", "telegram", "@account")
			require.NoError(t, err)
			require.Empty(t, custom.Name)
			_, err = ReviewEnterpriseApplication(1, custom.Id, "approve", "default", "")
			require.ErrorIs(t, err, ErrEnterpriseInvalidInput)
			_, err = ReviewEnterpriseApplication(1, app.Id, "contact", "", "")
			require.ErrorIs(t, err, ErrEnterpriseInvalidInput)
			_, err = ReviewEnterpriseApplication(1, app.Id, "approve", "nonexistent-group", "")
			require.Error(t, err)
			var unchanged EnterpriseApplication
			require.NoError(t, DB.First(&unchanged, app.Id).Error)
			require.Equal(t, "pending", unchanged.Status)
			var n int64
			require.NoError(t, DB.Model(&Enterprise{}).Where("owner_user_id = ?", user.Id).Count(&n).Error)
			require.Zero(t, n)
			approved, err := ReviewEnterpriseApplication(1, app.Id, "approve", "default", "")
			require.NoError(t, err)
			member, err := GetActiveEnterpriseMemberByUser(user.Id)
			require.NoError(t, err)
			require.Equal(t, approved.EnterpriseId, member.EnterpriseId)
			require.Equal(t, EnterpriseRoleOwner, member.Role)
			require.Zero(t, enterpriseTestBalanced(t, approved.EnterpriseId).Unallocated)
			_, err = ReviewEnterpriseApplication(1, app.Id, "approve", "default", "")
			require.ErrorIs(t, err, ErrEnterpriseFinalized)
			_, err = CreateEnterpriseApplication(user.Id, "opening", "another", "qq", "123")
			require.ErrorIs(t, err, ErrEnterpriseAlreadyMember)
			_, err = ReviewEnterpriseApplication(1, custom.Id, "reject", "", "")
			require.ErrorIs(t, err, ErrEnterpriseInvalidInput)
			_, err = ReviewEnterpriseApplication(1, custom.Id, "reject", "", "请核实联系方式")
			require.NoError(t, err)
			custom, err = CreateEnterpriseApplication(user.Id, "custom", "", "x", "@account")
			require.NoError(t, err)
			_, err = ReviewEnterpriseApplication(1, custom.Id, "contact", "", "")
			require.NoError(t, err)
			items, total, err := ListEnterpriseApplications(user.Id, "contacted", "custom", 0, 1)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, items, 1)
			other := enterpriseTestUser(t, 0)
			items, total, err = ListEnterpriseApplications(other.Id, "", "", 0, 20)
			require.NoError(t, err)
			require.Zero(t, total)
			require.Empty(t, items)
			_, _, err = ListEnterpriseApplications(0, "unknown", "", 0, 20)
			require.ErrorIs(t, err, ErrEnterpriseInvalidInput)
		})
	}
}

func TestEnterpriseApplicationConcurrentReview(t *testing.T) {
	for _, dialect := range enterpriseTestDialects {
		t.Run(dialect, func(t *testing.T) {
			enterpriseTestDB(t, dialect)
			user := enterpriseTestUser(t, 0)
			app, err := CreateEnterpriseApplication(user.Id, "opening", "并发审核", "email", "a@example.com")
			require.NoError(t, err)
			var wg sync.WaitGroup
			results := make(chan error, 2)
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := ReviewEnterpriseApplication(1, app.Id, "approve", "default", "")
					results <- err
				}()
			}
			wg.Wait()
			close(results)
			success := 0
			for err := range results {
				if err == nil {
					success++
				} else {
					require.ErrorIs(t, err, ErrEnterpriseFinalized)
				}
			}
			require.Equal(t, 1, success)
			var n int64
			require.NoError(t, DB.Model(&Enterprise{}).Where("owner_user_id = ?", user.Id).Count(&n).Error)
			require.EqualValues(t, 1, n)
		})
	}
}
