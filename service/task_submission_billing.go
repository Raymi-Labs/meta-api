package service

import (
	"errors"
	"fmt"
	"net/http"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// PrepareTaskBilling reserves each attempt's price before the provider can
// accept a paid task. Keep the original session so failures refund all attempts.
func PrepareTaskBilling(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	info.ForcePreConsume = true
	if info.Billing == nil {
		if info.PriceData.FreeModel {
			return nil
		}
		if err := PreConsumeBilling(c, info.PriceData.Quota, info); err != nil {
			return err
		}
	}
	if session, ok := info.Billing.(*BillingSession); ok {
		if err := session.requireTaskReservation(); err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
	}
	if err := info.Billing.Reserve(info.PriceData.Quota); err != nil {
		var apiErr *types.NewAPIError
		if errors.As(err, &apiErr) {
			return apiErr
		}
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}
	return nil
}

// Async submissions cannot use the synchronous trust bypass or admit wallet
// debt. Reserve retains the existing lock, token rollback and subscription logic.
func (s *BillingSession) requireTaskReservation() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled || s.refunded || s.fundingSettled {
		return fmt.Errorf("task billing session is already finalized")
	}
	if wallet, ok := s.funding.(*WalletFunding); ok {
		wallet.requireAvailableQuota = true
	}
	s.trusted = false
	return nil
}
