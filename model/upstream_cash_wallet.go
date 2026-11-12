package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CashGrantSource struct{ Kind, Ref string }

func cashNewUnknownLotTx(tx *gorm.DB, state *CashWalletState, quota int64, reason string) error {
	if quota <= 0 {
		return nil
	}
	ref := common.GetUUID()
	return tx.Create(&CashFundingLot{UserID: state.UserID, SourceType: "opening", SourceRef: reason,
		SourceKey: cashHash("opening", ref), TotalQuota: quota, RemainingQuota: quota, Epoch: state.Epoch, CreatedAt: common.GetTimestamp()}).Error
}

// Caller already owns the same user row lock as the actual wallet mutation.
func cashWalletStateTx(tx *gorm.DB, userID int, balance int64, forceUnknown bool) (*CashWalletState, error) {
	var state CashWalletState
	err := tx.Where("user_id = ?", userID).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		state = CashWalletState{UserID: userID, Quota: balance, Epoch: 1}
		if err = tx.Create(&state).Error; err != nil {
			return nil, err
		}
		if err = cashNewUnknownLotTx(tx, &state, max(balance, 0), "期初余额来源待核实"); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if forceUnknown || state.Quota != balance {
		if err = tx.Model(&CashFundingLot{}).Where("user_id = ? AND remaining_quota > 0", userID).Update("remaining_quota", 0).Error; err != nil {
			return nil, err
		}
		if err = tx.Model(&CashFundingAllocation{}).Where("user_id = ? AND status = ?", userID, "reserved").Update("status", "unknown").Error; err != nil {
			return nil, err
		}
		state.Quota, state.Epoch = balance, state.Epoch+1
		if err = tx.Save(&state).Error; err != nil {
			return nil, err
		}
		if err = cashNewUnknownLotTx(tx, &state, max(balance, 0), "额度变动缺少对应资金来源"); err != nil {
			return nil, err
		}
	}
	return &state, nil
}

func cashInvalidateWalletTx(tx *gorm.DB, user *User) error {
	if cashAccountingEnabledAt.Load() == 0 {
		return nil
	}
	_, err := cashWalletStateTx(tx, user.Id, int64(user.Quota), true)
	return err
}

func cashGrantWalletTx(tx *gorm.DB, userID, quota int, sources []CashGrantSource) error {
	if cashAccountingEnabledAt.Load() == 0 || quota <= 0 {
		return nil
	}
	user, err := lockPulseFundingUserTx(tx, userID)
	if err != nil {
		return err
	}
	state, err := cashWalletStateTx(tx, userID, int64(user.Quota), false)
	if err != nil {
		return err
	}
	source := CashGrantSource{Kind: "unknown", Ref: common.GetUUID()}
	if len(sources) > 0 && sources[0].Ref != "" {
		source = sources[0]
	}
	if source.Kind != "topup" && source.Kind != "gift" && source.Kind != "unknown" {
		return errors.New("invalid cash grant source")
	}
	available := max(int64(user.Quota)+int64(quota), 0) - max(int64(user.Quota), 0)
	lot := CashFundingLot{UserID: userID, SourceType: source.Kind, SourceRef: source.Ref,
		SourceKey: cashHash(source.Kind, fmt.Sprint(userID), source.Ref), TotalQuota: int64(quota), RemainingQuota: available,
		Epoch: state.Epoch, CreatedAt: common.GetTimestamp()}
	if err = tx.Create(&lot).Error; err != nil {
		return err
	}
	state.Quota += int64(quota)
	return tx.Save(state).Error
}

// Adjust only the analytical allocation. Quota is still mutated exclusively by
// AdjustPulseWalletReservation. Both writes share its transaction and user lock.
func cashAdjustReservationTx(tx *gorm.DB, user *User, reservation *PulseWalletReservation, target int, finalState string) error {
	if cashAccountingEnabledAt.Load() == 0 {
		return nil
	}
	state, err := cashWalletStateTx(tx, user.Id, int64(user.Quota), false)
	if err != nil {
		return err
	}
	delta := int64(target) - reservation.Quota
	now := common.GetTimestamp()
	if delta > 0 {
		var lots []CashFundingLot
		if err = tx.Where("user_id = ? AND epoch = ? AND remaining_quota > 0", user.Id, state.Epoch).Order("id asc").Find(&lots).Error; err != nil {
			return err
		}
		remaining := delta
		for i := range lots {
			lot := &lots[i]
			take := min(remaining, lot.RemainingQuota)
			if take <= 0 {
				break
			}
			allocation := CashFundingAllocation{ReservationID: reservation.ID, UserID: user.Id, LotID: lot.ID, Quota: take, Epoch: state.Epoch, Status: "reserved", CreatedAt: now}
			if err = tx.Create(&allocation).Error; err != nil {
				return err
			}
			lot.RemainingQuota -= take
			if err = tx.Save(lot).Error; err != nil {
				return err
			}
			remaining -= take
		}
		if remaining > 0 {
			if err = tx.Create(&CashFundingAllocation{ReservationID: reservation.ID, UserID: user.Id, Quota: remaining, Epoch: state.Epoch, Status: "unknown", CreatedAt: now}).Error; err != nil {
				return err
			}
		}
	} else if delta < 0 {
		var allocations []CashFundingAllocation
		if err = tx.Where("reservation_id = ? AND quota > 0", reservation.ID).Order("id desc").Find(&allocations).Error; err != nil {
			return err
		}
		remaining := -delta
		for i := range allocations {
			allocation := &allocations[i]
			giveBack := min(remaining, allocation.Quota)
			if giveBack <= 0 {
				break
			}
			if allocation.LotID > 0 && allocation.Epoch == state.Epoch && allocation.Status == "reserved" {
				if err = tx.Model(&CashFundingLot{}).Where("id = ? AND epoch = ?", allocation.LotID, state.Epoch).Update("remaining_quota", gorm.Expr("remaining_quota + ?", giveBack)).Error; err != nil {
					return err
				}
			}
			allocation.Quota -= giveBack
			if err = tx.Save(allocation).Error; err != nil {
				return err
			}
			remaining -= giveBack
		}
	}
	// Requests reserved before enablement, overdrafts, and invalidated in-flight
	// refunds retain unknown basis. They may never restore an old paid receipt.
	var allocated, available int64
	if err = tx.Model(&CashFundingAllocation{}).Where("reservation_id = ?", reservation.ID).Select("COALESCE(SUM(quota), 0)").Scan(&allocated).Error; err != nil {
		return err
	}
	if allocated < int64(target) {
		if err = tx.Create(&CashFundingAllocation{ReservationID: reservation.ID, UserID: user.Id, Quota: int64(target) - allocated, Epoch: state.Epoch, Status: "unknown", CreatedAt: now}).Error; err != nil {
			return err
		}
	}
	state.Quota = int64(user.Quota) - delta
	if err = tx.Model(&CashFundingLot{}).Where("user_id = ? AND epoch = ?", user.Id, state.Epoch).Select("COALESCE(SUM(remaining_quota), 0)").Scan(&available).Error; err != nil {
		return err
	}
	wantAvailable := max(state.Quota, 0)
	if available < wantAvailable {
		if err = cashNewUnknownLotTx(tx, state, wantAvailable-available, "退还额度的原资金来源不可追溯"); err != nil {
			return err
		}
	} else if available > wantAvailable {
		// Negative wallets or an untracked mutation cannot certify residual basis.
		state, err = cashWalletStateTx(tx, user.Id, state.Quota, true)
		if err != nil {
			return err
		}
	}
	if finalState != "reserved" {
		q := tx.Model(&CashFundingAllocation{}).Where("reservation_id = ?", reservation.ID)
		if finalState == "settled" {
			q = q.Where("status = ?", "reserved")
		}
		if err = q.Update("status", finalState).Error; err != nil {
			return err
		}
	}
	return tx.Save(state).Error
}

func recordCashUsage(c *gin.Context, userID int, params RecordConsumeLogParams) {
	if cashAccountingEnabledAt.Load() == 0 {
		return
	}
	requestID := c.GetString(common.RequestIdKey)
	if userID <= 0 || requestID == "" || len(requestID) > 64 || params.ChannelId <= 0 {
		return
	}
	row := CashUsage{RequestID: requestID, UserID: userID, ChannelID: params.ChannelId, Model: params.ModelName,
		Quota: int64(params.Quota), OccurredAt: common.GetTimestamp(), FundingSource: "unknown"}
	if value, ok := params.Other["upstream_model_name"].(string); ok && value != "" {
		row.Model = value
	}
	if len(row.Model) > 255 {
		row.Model = ""
	}
	if value, ok := params.Other["billing_source"].(string); ok && (value == "wallet" || value == "subscription" || value == "token" || value == EnterpriseQuotaSource) {
		row.FundingSource = value
	}
	if common.GetContextKeyInt(c, constant.ContextKeyChannelType) == constant.ChannelTypeOpenRouter {
		row.KeyHash = cashKeyHash(common.GetContextKeyString(c, constant.ContextKeyChannelKey))
	}
	var reservation PulseWalletReservation
	err := DB.Where("request_id = ? AND user_id = ?", requestID, userID).First(&reservation).Error
	if err == nil && reservation.Status == "settled" && reservation.Quota == row.Quota {
		row.ReservationID, row.FundingSource = reservation.ID, "wallet"
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.SysError("cash usage funding lookup failed")
		return // an unmatched durable reservation keeps the reporting gap visible
	}
	if err = DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "request_id"}}, DoNothing: true}).Create(&row).Error; err != nil {
		common.SysError("cash usage metadata could not be persisted; cash reconciliation remains incomplete")
	}
}

type CashReceiptInput struct {
	RequestID   string `json:"request_id"`
	PreviousID  int64  `json:"previous_id"`
	Currency    string `json:"currency"`
	GrossAmount string `json:"gross_amount"`
	FeeAmount   string `json:"fee_amount"`
	FXRate      string `json:"fx_rate"`
	ReceivedAt  int64  `json:"received_at"`
	Evidence    string `json:"evidence"`
	Note        string `json:"note"`
}

func SaveCashFundingReceipt(lotID int64, actor int, input CashReceiptInput) (*CashFundingReceipt, error) {
	if !validWalletRedemptionRequestID(input.RequestID) || actor <= 0 || lotID <= 0 {
		return nil, errors.New("无效的资金凭证请求")
	}
	input.Currency, input.Evidence, input.Note = strings.ToUpper(strings.TrimSpace(input.Currency)), strings.TrimSpace(input.Evidence), strings.TrimSpace(input.Note)
	if input.Evidence == "" || len(input.Evidence) > 255 || len(input.Note) > 255 || input.ReceivedAt <= 0 || input.ReceivedAt > common.GetTimestamp() {
		return nil, errors.New("请填写有效的到账时间和凭证引用")
	}
	net, err := cashConvert(input.GrossAmount, input.FeeAmount, input.Currency, input.FXRate, true)
	if err != nil {
		return nil, err
	}
	payload, _ := common.Marshal(input)
	row := CashFundingReceipt{LotID: lotID, PreviousID: input.PreviousID, RequestKey: cashHash(fmt.Sprint(actor), input.RequestID), PayloadHash: cashHash(fmt.Sprint(lotID), string(payload)),
		Currency: input.Currency, GrossAmount: input.GrossAmount, FeeAmount: input.FeeAmount, FXRate: input.FXRate, NetCNYMicros: net, ReceivedAt: input.ReceivedAt,
		Evidence: input.Evidence, Note: input.Note, CreatedBy: actor, CreatedAt: common.GetTimestamp()}
	err = DB.Transaction(func(tx *gorm.DB) error {
		var lot CashFundingLot
		if err := tx.First(&lot, lotID).Error; err != nil {
			return err
		}
		if _, err := lockPulseFundingUserTx(tx, lot.UserID); err != nil {
			return err
		}
		var existing CashFundingReceipt
		if err := tx.Where("request_key = ?", row.RequestKey).First(&existing).Error; err == nil {
			if existing.PayloadHash != row.PayloadHash {
				return errors.New("重复请求的资金凭证内容不一致")
			}
			row = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.First(&lot, lotID).Error; err != nil {
			return err
		}
		if lot.SourceType == "gift" {
			return errors.New("无偿赠送批次不需要收款凭证")
		}
		if lot.ReceiptID != input.PreviousID {
			return errors.New("资金凭证已更新，请刷新后重试")
		}
		if lot.ReceiptID > 0 && input.Note == "" {
			return errors.New("更正资金凭证时必须填写原因")
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return tx.Model(&CashFundingLot{}).Where("id = ?", lotID).Update("receipt_id", row.ID).Error
	})
	return &row, err
}

type CashFundingLotView struct {
	CashFundingLot
	Receipt *CashFundingReceipt `json:"receipt"`
}

func ListCashFundingLots(offset, limit int, userID int, pending bool) ([]CashFundingLotView, int64, error) {
	q := DB.Model(&CashFundingLot{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if pending {
		q = q.Where("receipt_id = 0 AND source_type <> ?", "gift")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var lots []CashFundingLot
	if err := q.Order("id desc").Offset(max(offset, 0)).Limit(min(max(limit, 1), 100)).Find(&lots).Error; err != nil {
		return nil, 0, err
	}
	result := make([]CashFundingLotView, 0, len(lots))
	ids := make([]int64, 0, len(lots))
	for _, lot := range lots {
		if lot.ReceiptID > 0 {
			ids = append(ids, lot.ReceiptID)
		}
	}
	var receipts []CashFundingReceipt
	if len(ids) > 0 {
		if err := DB.Where("id IN ?", ids).Find(&receipts).Error; err != nil {
			return nil, 0, err
		}
	}
	byID := map[int64]*CashFundingReceipt{}
	for i := range receipts {
		byID[receipts[i].ID] = &receipts[i]
	}
	for _, lot := range lots {
		result = append(result, CashFundingLotView{CashFundingLot: lot, Receipt: byID[lot.ReceiptID]})
	}
	return result, total, nil
}

func ListCashFundingReceipts(lotID int64, offset, limit int) ([]CashFundingReceipt, int64, error) {
	q := DB.Model(&CashFundingReceipt{}).Where("lot_id = ?", lotID)
	var total int64
	rows := []CashFundingReceipt{}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("id desc").Offset(max(offset, 0)).Limit(min(max(limit, 1), 100)).Find(&rows).Error
	return rows, total, err
}
