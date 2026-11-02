package service

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type quotaHealthFunding struct{ err error }

func (f *quotaHealthFunding) Source() string       { return BillingSourceSubscription }
func (f *quotaHealthFunding) PreConsume(int) error { return f.err }
func (f *quotaHealthFunding) Settle(int) error     { return nil }
func (f *quotaHealthFunding) Refund() error        { return nil }

func TestQuotaHealthFundingRequiresTypedRejection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		want  types.FailureClass
	}{
		{"subscription", fmt.Errorf("wrapped: %w", &types.QuotaRejection{Source: "subscription", Message: "no active subscription"}), types.FailureUserRequest},
		{"database with quota words", errors.New("database timeout: subscription quota insufficient"), types.FailureService},
		{"database", errors.New("database unavailable"), types.FailureService},
		{"wallet atomic rejection", model.ErrInsufficientImageQuota, types.FailureUserRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			// Zero token reservation isolates funding classification without DB side effects.
			s := &BillingSession{relayInfo: &relaycommon.RelayInfo{ForcePreConsume: true}, funding: &quotaHealthFunding{tc.cause}}
			e := s.preConsume(c, 0)
			require.NotNil(t, e)
			require.Equal(t, tc.want, types.ClassifyFailure(e))
		})
	}
}

func TestQuotaHealthAdditionalReserveDatabaseFailure(t *testing.T) {
	truncate(t)
	for _, funding := range []FundingSource{
		&WalletFunding{userId: 987654, requireAvailableQuota: true},
		&SubscriptionFunding{subscriptionId: 987654},
	} {
		s := &BillingSession{funding: funding}
		// Missing authoritative resources are infrastructure/data failures, not quota evidence.
		err := s.reserveFunding(10)
		var apiErr *types.NewAPIError
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, types.FailureService, types.ClassifyFailure(apiErr))
	}
}

func TestQuotaHealthLogCategoryDoesNotGuessFromCode(t *testing.T) {
	for _, code := range []types.ErrorCode{types.ErrorCodeInsufficientUserQuota, types.ErrorCodePreConsumeTokenQuotaFailed} {
		local := types.NewErrorWithStatusCode(&types.QuotaRejection{Source: "token", Message: "fixture"}, code, 403)
		category, _, _ := describeRequestFailure(local, false)
		require.Equal(t, "quota", category)
		unknown := types.NewErrorWithStatusCode(errors.New("quota insufficient"), code, 403)
		category, _, _ = describeRequestFailure(unknown, false)
		require.NotEqual(t, "quota", category)
		category, _, _ = describeRequestFailure(local, true)
		require.NotEqual(t, "quota", category)
	}
}

func TestQuotaHealthRollbackFailureRemainsServiceFailure(t *testing.T) {
	truncate(t)
	seedUser(t, 986001, 100)
	seedToken(t, 986002, 986001, "quota-health-rollback", 100)
	// Fail the second token update (rollback), after a successful debit.
	updates := 0
	name := "test:quota-health-rollback"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(name, func(db *gorm.DB) {
		if db.Statement.Table == "tokens" {
			updates++
			if updates == 2 {
				db.AddError(errors.New("rollback database unavailable"))
			}
		}
	}))
	t.Cleanup(func() { model.DB.Callback().Update().Remove(name) })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	s := &BillingSession{relayInfo: &relaycommon.RelayInfo{UserId: 986001, TokenId: 986002, TokenKey: "quota-health-rollback", ForcePreConsume: true}, funding: &quotaHealthFunding{&types.QuotaRejection{Source: "subscription", Message: "no active subscription"}}}
	err := s.preConsume(c, 10)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeUpdateDataError, err.GetErrorCode())
	require.Equal(t, types.FailureService, types.ClassifyFailure(err))
	require.Equal(t, 2, updates)
}
