package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	InvoiceStatusPending  = "pending"
	InvoiceStatusInvoiced = "invoiced"
	InvoiceStatusRejected = "rejected"

	InvoiceSendStatusPending = "pending"
	InvoiceSendStatusSent    = "sent"
	InvoiceSendStatusFailed  = "failed"

	InvoiceTypeNormal  = "normal"
	InvoiceTypeSpecial = "special"

	InvoiceTitleTypePersonal = "personal"
	InvoiceTitleTypeCompany  = "company"

	InvoiceSourceTypeSystemOrder    = "system_order"
	InvoiceSourceTypeManualTransfer = "manual_transfer"

	InvoiceOrderTypeManualTransfer   = "manual_transfer"
	InvoicePaymentMethodBankTransfer = "bank_transfer"
	InvoiceManualTransferPayeeName   = "上海曜算智能科技有限公司"
	InvoiceManualTransferProductName = "AI API 调用服务"
	InvoiceManualTransferProductUnit = "项"

	minInvoiceMoney = int64(20) // CNY, per application before service fees.
	maxInvoiceMoney = int64(1_000_000_000_000)
)

var (
	ErrInvoiceRequestNotFound         = errors.New("发票申请不存在")
	ErrInvoiceRequestAlreadyReviewed  = errors.New("发票申请已审核")
	ErrInvoiceOrderUnavailable        = errors.New("订单不可申请发票")
	ErrInvoiceAmountBelowMinimum      = errors.New("每次发票申请金额合计最低为人民币 20 元")
	ErrInvoiceManualTransferOccupied  = errors.New("该银行转账已存在待审核或已开票申请")
	ErrInsufficientQuotaForInvoiceFee = errors.New("余额不足，无法支付发票手续费")
)

// InvoiceEmailAttachments retains the exact PDFs uploaded for later resends.
// Storage paths are private; clients only need filenames to offer reuse.
type InvoiceEmailAttachments struct {
	DetailBillFileName          string `json:"detail_bill_file_name" gorm:"type:varchar(255);not null;default:''"`
	DetailBillFilePath          string `json:"-" gorm:"type:text"`
	ServiceConfirmationFileName string `json:"service_confirmation_file_name" gorm:"type:varchar(255);not null;default:''"`
	ServiceConfirmationFilePath string `json:"-" gorm:"type:text"`
}

type InvoiceRequest struct {
	InvoiceEmailAttachments `gorm:"embedded"`
	Id                      int     `json:"id"`
	UserId                  int     `json:"user_id" gorm:"index;not null"`
	InvoiceType             string  `json:"invoice_type" gorm:"type:varchar(16);not null;default:'normal'"`
	TitleType               string  `json:"title_type" gorm:"type:varchar(16);not null;default:'personal'"`
	Title                   string  `json:"title" gorm:"type:varchar(128);not null;default:''"`
	TaxNumber               string  `json:"tax_number" gorm:"type:varchar(64);not null;default:''"`
	RegisteredAddress       string  `json:"registered_address" gorm:"type:varchar(255);not null;default:''"`
	RegisteredPhone         string  `json:"registered_phone" gorm:"type:varchar(64);not null;default:''"`
	BankName                string  `json:"bank_name" gorm:"type:varchar(128);not null;default:''"`
	BankAccount             string  `json:"bank_account" gorm:"type:varchar(128);not null;default:''"`
	Email                   string  `json:"email" gorm:"type:varchar(128);not null;default:''"`
	Phone                   string  `json:"phone" gorm:"type:varchar(32);not null;default:''"`
	Remark                  string  `json:"remark" gorm:"type:text"`
	NeedDetailBill          bool    `json:"need_detail_bill" gorm:"not null;default:true"`
	NeedServiceConfirmation bool    `json:"need_service_confirmation" gorm:"not null;default:false"`
	SourceType              string  `json:"source_type" gorm:"type:varchar(32);index;not null;default:'system_order'"`
	Status                  string  `json:"status" gorm:"type:varchar(16);index;not null;default:'pending'"`
	TotalMoney              float64 `json:"total_money" gorm:"type:decimal(20,6);not null;default:0"`
	TotalQuota              int64   `json:"total_quota" gorm:"type:bigint;not null;default:0"`
	ServiceFeeRate          float64 `json:"service_fee_rate" gorm:"type:decimal(10,6);not null;default:0"`
	ServiceFeeQuota         int64   `json:"service_fee_quota" gorm:"type:bigint;not null;default:0"`
	InvoiceNo               string  `json:"invoice_no" gorm:"type:varchar(128);not null;default:''"`
	InvoiceUrl              string  `json:"invoice_url" gorm:"type:text"`
	InvoiceFileName         string  `json:"invoice_file_name" gorm:"type:varchar(255);not null;default:''"`
	InvoiceFilePath         string  `json:"-" gorm:"type:text"`
	InvoiceSentTo           string  `json:"invoice_sent_to" gorm:"type:varchar(128);not null;default:''"`
	InvoiceSentAt           int64   `json:"invoice_sent_at" gorm:"index;not null;default:0"`
	InvoiceSendStatus       string  `json:"invoice_send_status" gorm:"type:varchar(16);index;not null;default:''"`
	InvoiceSendError        string  `json:"invoice_send_error" gorm:"type:text"`
	AdminRemark             string  `json:"admin_remark" gorm:"type:text"`
	ReviewerUserId          int     `json:"reviewer_user_id" gorm:"index;not null;default:0"`
	CreatedAt               int64   `json:"created_at" gorm:"index"`
	ReviewedAt              int64   `json:"reviewed_at" gorm:"index;not null;default:0"`

	Username            string                      `json:"username,omitempty" gorm:"column:username;->"`
	DisplayName         string                      `json:"display_name,omitempty" gorm:"column:display_name;->"`
	ReviewerUsername    string                      `json:"reviewer_username,omitempty" gorm:"column:reviewer_username;->"`
	ReviewerDisplayName string                      `json:"reviewer_display_name,omitempty" gorm:"column:reviewer_display_name;->"`
	Items               []InvoiceRequestItem        `json:"items" gorm:"foreignKey:InvoiceRequestId"`
	ProductItems        []InvoiceRequestProductItem `json:"product_items" gorm:"foreignKey:InvoiceRequestId"`
}

type InvoiceRequestItem struct {
	Id                int     `json:"id"`
	InvoiceRequestId  int     `json:"invoice_request_id" gorm:"index;not null"`
	UserId            int     `json:"user_id" gorm:"index;not null"`
	SourceType        string  `json:"source_type" gorm:"type:varchar(32);index;not null;default:'system_order'"`
	OrderType         string  `json:"order_type" gorm:"type:varchar(32);index;not null"`
	OrderId           int     `json:"order_id" gorm:"index;not null"`
	TradeNo           string  `json:"trade_no" gorm:"type:varchar(255);index;not null;default:''"`
	PaymentMethod     string  `json:"payment_method" gorm:"type:varchar(50);not null;default:''"`
	Amount            int64   `json:"amount" gorm:"type:bigint;not null;default:0"`
	Money             float64 `json:"money" gorm:"type:decimal(20,6);not null;default:0"`
	ProductName       string  `json:"product_name" gorm:"type:varchar(255);not null;default:''"`
	PayerName         string  `json:"payer_name" gorm:"type:varchar(128);not null;default:''"`
	PayeeName         string  `json:"payee_name" gorm:"type:varchar(128);not null;default:''"`
	TransferBankName  string  `json:"transfer_bank_name" gorm:"type:varchar(128);not null;default:''"`
	TransferRemark    string  `json:"transfer_remark" gorm:"type:varchar(500);not null;default:''"`
	ManualFingerprint *string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	CreateTime        int64   `json:"create_time" gorm:"index"`
	CompleteTime      int64   `json:"complete_time" gorm:"index"`
}

type InvoiceRequestProductItem struct {
	Id               int     `json:"id"`
	InvoiceRequestId int     `json:"invoice_request_id" gorm:"index;not null"`
	ProductName      string  `json:"product_name" gorm:"type:varchar(255);not null"`
	Specification    string  `json:"specification" gorm:"type:varchar(255);not null;default:''"`
	Unit             string  `json:"unit" gorm:"type:varchar(32);not null;default:''"`
	Quantity         float64 `json:"quantity" gorm:"type:decimal(20,6);not null;default:1"`
	UnitPrice        float64 `json:"unit_price" gorm:"type:decimal(20,6);not null;default:0"`
	Money            float64 `json:"money" gorm:"type:decimal(20,6);not null;default:0"`
	Quota            int64   `json:"quota" gorm:"type:bigint;not null;default:0"`
	ServiceStartAt   int64   `json:"service_start_at" gorm:"index;not null;default:0"`
	ServiceEndAt     int64   `json:"service_end_at" gorm:"index;not null;default:0"`
	Remark           string  `json:"remark" gorm:"type:varchar(500);not null;default:''"`
}

type InvoiceOrderRef struct {
	OrderType string `json:"order_type"`
	Id        int    `json:"id"`
}

type InvoiceManualTransactionInput struct {
	TradeNo          string  `json:"trade_no"`
	PayerName        string  `json:"payer_name"`
	PayeeName        string  `json:"payee_name"`
	TransferBankName string  `json:"transfer_bank_name"`
	Money            float64 `json:"money"`
	PaidAt           int64   `json:"paid_at"`
	Remark           string  `json:"remark"`
}

type InvoiceProductItemInput struct {
	ProductName    string  `json:"product_name"`
	Specification  string  `json:"specification"`
	Unit           string  `json:"unit"`
	Quantity       float64 `json:"quantity"`
	UnitPrice      float64 `json:"unit_price"`
	Money          float64 `json:"money"`
	Quota          int64   `json:"quota"`
	ServiceStartAt int64   `json:"service_start_at"`
	ServiceEndAt   int64   `json:"service_end_at"`
	Remark         string  `json:"remark"`
}

type CreateInvoiceRequestInput struct {
	InvoiceType             string
	TitleType               string
	Title                   string
	TaxNumber               string
	RegisteredAddress       string
	RegisteredPhone         string
	BankName                string
	BankAccount             string
	Email                   string
	Phone                   string
	Remark                  string
	NeedDetailBill          bool
	NeedServiceConfirmation bool
	Orders                  []InvoiceOrderRef
	ManualTransactions      []InvoiceManualTransactionInput
	ProductItems            []InvoiceProductItemInput
}

type InvoiceRequestSearchParams struct {
	Status   string
	Username string
}

type InvoiceReviewInput struct {
	InvoiceEmailAttachments
	InvoiceNo         string
	InvoiceUrl        string
	InvoiceFileName   string
	InvoiceFilePath   string
	InvoiceSentTo     string
	InvoiceSendStatus string
	AdminRemark       string
}

func GetEligibleInvoiceOrders(userID int, pageInfo *common.PageInfo) ([]*PaymentRecord, int64, error) {
	if userID <= 0 {
		return nil, 0, errors.New("用户不存在")
	}
	if pageInfo == nil {
		pageInfo = &common.PageInfo{Page: 1, PageSize: common.ItemsPerPage}
	}

	topups, err := listEligibleInvoiceTopUpRecords(DB, userID)
	if err != nil {
		return nil, 0, err
	}
	walletPurchases, err := listEligibleInvoiceSellableTokenRecords(DB, userID)
	if err != nil {
		return nil, 0, err
	}
	records := mergePaymentRecordPage(nil, topups, walletPurchases)
	total := int64(len(records))
	start := pageInfo.GetStartIdx()
	if start >= len(records) {
		return []*PaymentRecord{}, total, nil
	}
	end := pageInfo.GetEndIdx()
	if end > len(records) {
		end = len(records)
	}
	return records[start:end], total, nil
}

func CreateInvoiceRequest(userID int, input CreateInvoiceRequestInput) (*InvoiceRequest, error) {
	if userID <= 0 {
		return nil, errors.New("用户不存在")
	}
	input = normalizeCreateInvoiceInput(input)
	if err := validateCreateInvoiceInput(input); err != nil {
		return nil, err
	}

	sourceType := invoiceSourceTypeFromInput(input)
	var request InvoiceRequest
	err := DB.Transaction(func(tx *gorm.DB) error {
		items := make([]InvoiceRequestItem, 0, len(input.Orders)+len(input.ManualTransactions))
		seenOrders := make(map[string]struct{}, len(input.Orders))
		seenManualTransfers := make(map[string]struct{}, len(input.ManualTransactions))
		totalMoneyDecimal := decimal.Zero
		var totalQuota int64

		for _, ref := range input.Orders {
			orderType := normalizeInvoiceOrderType(ref.OrderType)
			if orderType == "" || ref.Id <= 0 {
				return errors.New("订单参数错误")
			}
			key := fmt.Sprintf("%s:%d", orderType, ref.Id)
			if _, ok := seenOrders[key]; ok {
				return errors.New("订单重复选择")
			}
			seenOrders[key] = struct{}{}

			item, err := buildInvoiceRequestItemTx(tx, userID, orderType, ref.Id)
			if err != nil {
				return err
			}
			item.SourceType = InvoiceSourceTypeSystemOrder
			items = append(items, *item)
			totalMoneyDecimal = totalMoneyDecimal.Add(decimal.NewFromFloat(item.Money))
			if item.Amount < 0 || (item.Amount > 0 && totalQuota > (1<<63-1)-item.Amount) {
				return errors.New("订单额度合计超出范围")
			}
			totalQuota += item.Amount
		}

		for _, transfer := range input.ManualTransactions {
			fingerprint := invoiceManualTransferFingerprint(transfer)
			if _, ok := seenManualTransfers[fingerprint]; ok {
				return errors.New("银行转账重复录入")
			}
			seenManualTransfers[fingerprint] = struct{}{}
			occupied, err := isInvoiceManualTransferOccupiedTx(tx, fingerprint)
			if err != nil {
				return err
			}
			if occupied {
				return ErrInvoiceManualTransferOccupied
			}
			fingerprintCopy := fingerprint
			item := InvoiceRequestItem{
				UserId:            userID,
				SourceType:        InvoiceSourceTypeManualTransfer,
				OrderType:         InvoiceOrderTypeManualTransfer,
				OrderId:           0,
				TradeNo:           transfer.TradeNo,
				PaymentMethod:     InvoicePaymentMethodBankTransfer,
				Money:             transfer.Money,
				ProductName:       InvoiceManualTransferProductName,
				PayerName:         transfer.PayerName,
				PayeeName:         transfer.PayeeName,
				TransferBankName:  transfer.TransferBankName,
				TransferRemark:    transfer.Remark,
				ManualFingerprint: &fingerprintCopy,
				CreateTime:        transfer.PaidAt,
				CompleteTime:      transfer.PaidAt,
			}
			items = append(items, item)
			totalMoneyDecimal = totalMoneyDecimal.Add(decimal.NewFromFloat(transfer.Money))
		}

		if totalMoneyDecimal.LessThan(decimal.NewFromInt(minInvoiceMoney)) {
			return ErrInvoiceAmountBelowMinimum
		}
		if totalMoneyDecimal.GreaterThan(decimal.NewFromInt(maxInvoiceMoney)) {
			return errors.New("开票金额合计不能超过 1000000000000 元")
		}
		totalMoney, _ := totalMoneyDecimal.Float64()
		productItems, productQuota, err := buildInvoiceProductItems(sourceType, totalMoney, totalQuota, items)
		if err != nil {
			return err
		}
		if sourceType == InvoiceSourceTypeManualTransfer {
			totalQuota = productQuota
		}

		// 计算发票手续费额度：开票金额(totalMoney)为人民币(payMoney 同源)，
		// 先按充值汇率 Price 换回美元，再 × QuotaPerUnit 得到额度，保证与充值口径一致。
		var feeRate float64
		var feeQuota int64
		if rate := common.InvoiceServiceFeeRate; rate > 0 && totalMoney > 0 {
			if price := operation_setting.Price; price > 0 {
				dFee := decimal.NewFromFloat(totalMoney).
					Mul(decimal.NewFromFloat(rate)).
					Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
					Div(decimal.NewFromFloat(price))
				if dFee.GreaterThan(decimal.NewFromInt(1<<63 - 1)) {
					return errors.New("发票手续费额度超出范围")
				}
				if q := dFee.IntPart(); q > 0 {
					feeRate = rate
					feeQuota = q
				}
			}
		}

		request = InvoiceRequest{
			UserId:                  userID,
			InvoiceType:             input.InvoiceType,
			TitleType:               input.TitleType,
			Title:                   input.Title,
			TaxNumber:               input.TaxNumber,
			RegisteredAddress:       input.RegisteredAddress,
			RegisteredPhone:         input.RegisteredPhone,
			BankName:                input.BankName,
			BankAccount:             input.BankAccount,
			Email:                   input.Email,
			Phone:                   input.Phone,
			Remark:                  input.Remark,
			NeedDetailBill:          input.NeedDetailBill,
			NeedServiceConfirmation: input.NeedServiceConfirmation,
			SourceType:              sourceType,
			Status:                  InvoiceStatusPending,
			TotalMoney:              totalMoney,
			TotalQuota:              totalQuota,
			ServiceFeeRate:          feeRate,
			ServiceFeeQuota:         feeQuota,
			CreatedAt:               common.GetTimestamp(),
		}
		if err := tx.Create(&request).Error; err != nil {
			return err
		}
		if !input.NeedDetailBill {
			// GORM 会跳过带 default 标签的 bool 零值，这里显式落库用户取消明细账单的选择。
			if err := tx.Model(&InvoiceRequest{}).Where("id = ?", request.Id).Update("need_detail_bill", false).Error; err != nil {
				return err
			}
			request.NeedDetailBill = false
		}
		for i := range items {
			items[i].InvoiceRequestId = request.Id
		}
		if err := tx.Create(&items).Error; err != nil {
			if sourceType == InvoiceSourceTypeManualTransfer && isInvoiceDuplicateKeyErr(err) {
				return ErrInvoiceManualTransferOccupied
			}
			return err
		}
		for i := range productItems {
			productItems[i].InvoiceRequestId = request.Id
		}
		if err := tx.Create(&productItems).Error; err != nil {
			return err
		}
		request.Items = items
		request.ProductItems = productItems

		// 申请发票时扣除手续费：事务内行锁校验余额并扣减，保证与申请创建原子。
		// 人工转账仅作为开票凭证，不会创建充值订单，也不会增加钱包额度。
		if feeQuota > 0 {
			var feeUser User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "quota").First(&feeUser, "id = ?", userID).Error; err != nil {
				return err
			}
			if int64(feeUser.Quota) < feeQuota {
				return ErrInsufficientQuotaForInvoiceFee
			}
			if err := InvalidatePulsePaidFundingTx(tx, userID, "invoice_fee"); err != nil {
				return err
			}
			if err := tx.Model(&User{}).Where("id = ?", userID).
				Update("quota", gorm.Expr("quota - ?", feeQuota)).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if request.ServiceFeeQuota > 0 {
		if cacheErr := syncUserCacheByID(userID); cacheErr != nil {
			common.SysLog("failed to sync user cache after invoice fee deduction: " + cacheErr.Error())
		}
		RecordLog(userID, LogTypeManage, fmt.Sprintf(
			"申请发票（ID %d）扣除手续费 %d 额度（费率 %.4f，开票金额 %.2f 元）",
			request.Id, request.ServiceFeeQuota, request.ServiceFeeRate, request.TotalMoney))
	}
	return &request, nil
}

func GetUserInvoiceRequestsByParams(userID int, params InvoiceRequestSearchParams, pageInfo *common.PageInfo) ([]*InvoiceRequest, int64, error) {
	if userID <= 0 {
		return nil, 0, errors.New("用户不存在")
	}
	if pageInfo == nil {
		pageInfo = &common.PageInfo{Page: 1, PageSize: common.ItemsPerPage}
	}

	base := applyInvoiceRequestSearch(DB.Model(&InvoiceRequest{}).Where("user_id = ?", userID), params, false)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var requests []*InvoiceRequest
	query := applyInvoiceRequestSearch(DB.Model(&InvoiceRequest{}).Where("user_id = ?", userID), params, false)
	if err := query.Preload("Items").Preload("ProductItems").Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&requests).Error; err != nil {
		return nil, 0, err
	}
	return requests, total, nil
}

func GetAllInvoiceRequestsByParams(params InvoiceRequestSearchParams, pageInfo *common.PageInfo) ([]*InvoiceRequest, int64, error) {
	if pageInfo == nil {
		pageInfo = &common.PageInfo{Page: 1, PageSize: common.ItemsPerPage}
	}

	countQuery := applyInvoiceRequestSearch(invoiceRequestListWithUser(DB), params, true)
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var requests []*InvoiceRequest
	dataQuery := applyInvoiceRequestSearch(invoiceRequestListWithUser(DB), params, true)
	if err := dataQuery.Preload("Items").Preload("ProductItems").Order("invoice_requests.id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&requests).Error; err != nil {
		return nil, 0, err
	}
	return requests, total, nil
}

func GetUserInvoiceRequestDetail(userID int, id int) (*InvoiceRequest, error) {
	if userID <= 0 || id <= 0 {
		return nil, ErrInvoiceRequestNotFound
	}

	var request InvoiceRequest
	err := invoiceRequestDetailWithUser(DB).
		Preload("Items").
		Preload("ProductItems").
		Where("invoice_requests.id = ? AND invoice_requests.user_id = ?", id, userID).
		First(&request).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceRequestNotFound
		}
		return nil, err
	}
	return &request, nil
}

func GetInvoiceRequestDetail(id int) (*InvoiceRequest, error) {
	if id <= 0 {
		return nil, ErrInvoiceRequestNotFound
	}

	var request InvoiceRequest
	err := invoiceRequestDetailWithUser(DB).
		Preload("Items").
		Preload("ProductItems").
		Where("invoice_requests.id = ?", id).
		First(&request).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceRequestNotFound
		}
		return nil, err
	}
	return &request, nil
}

func ApproveInvoiceRequest(id int, reviewerUserID int, input InvoiceReviewInput) (*InvoiceRequest, error) {
	input.InvoiceNo = strings.TrimSpace(input.InvoiceNo)
	input.InvoiceUrl = strings.TrimSpace(input.InvoiceUrl)
	input.InvoiceFileName = strings.TrimSpace(input.InvoiceFileName)
	input.InvoiceFilePath = strings.TrimSpace(input.InvoiceFilePath)
	input.InvoiceSentTo = common.NormalizeEmailAddress(input.InvoiceSentTo)
	input.InvoiceSendStatus = strings.TrimSpace(input.InvoiceSendStatus)
	input.AdminRemark = strings.TrimSpace(input.AdminRemark)
	if input.InvoiceNo == "" && input.InvoiceUrl == "" && input.InvoiceFilePath == "" {
		return nil, errors.New("发票号、发票链接或发票 PDF 不能为空")
	}
	return reviewInvoiceRequest(id, reviewerUserID, InvoiceStatusInvoiced, input)
}

func RejectInvoiceRequest(id int, reviewerUserID int, adminRemark string) (*InvoiceRequest, error) {
	remark := strings.TrimSpace(adminRemark)
	if remark == "" {
		return nil, errors.New("驳回原因不能为空")
	}
	return reviewInvoiceRequest(id, reviewerUserID, InvoiceStatusRejected, InvoiceReviewInput{AdminRemark: remark})
}

func normalizeCreateInvoiceInput(input CreateInvoiceRequestInput) CreateInvoiceRequestInput {
	input.InvoiceType = normalizeInvoiceType(input.InvoiceType)
	input.TitleType = normalizeInvoiceTitleType(input.TitleType)
	if input.InvoiceType == InvoiceTypeSpecial {
		input.TitleType = InvoiceTitleTypeCompany
	}
	input.Title = strings.TrimSpace(input.Title)
	input.TaxNumber = strings.TrimSpace(input.TaxNumber)
	input.RegisteredAddress = strings.TrimSpace(input.RegisteredAddress)
	input.RegisteredPhone = strings.TrimSpace(input.RegisteredPhone)
	input.BankName = strings.TrimSpace(input.BankName)
	input.BankAccount = strings.TrimSpace(input.BankAccount)
	input.Email = common.NormalizeEmailAddress(input.Email)
	input.Phone = strings.TrimSpace(input.Phone)
	input.Remark = strings.TrimSpace(input.Remark)
	for i := range input.ManualTransactions {
		input.ManualTransactions[i].TradeNo = strings.TrimSpace(input.ManualTransactions[i].TradeNo)
		input.ManualTransactions[i].PayerName = strings.TrimSpace(input.ManualTransactions[i].PayerName)
		input.ManualTransactions[i].PayeeName = strings.TrimSpace(input.ManualTransactions[i].PayeeName)
		if normalizeInvoiceFingerprintPart(input.ManualTransactions[i].PayeeName) == normalizeInvoiceFingerprintPart(InvoiceManualTransferPayeeName) {
			input.ManualTransactions[i].PayeeName = InvoiceManualTransferPayeeName
		}
		input.ManualTransactions[i].TransferBankName = strings.TrimSpace(input.ManualTransactions[i].TransferBankName)
		input.ManualTransactions[i].Remark = strings.TrimSpace(input.ManualTransactions[i].Remark)
	}
	if len(input.ManualTransactions) > 0 {
		// 人工凭证的核心产物就是交易明细与产品清单，服务端强制保留，不能由篡改请求关闭。
		input.NeedDetailBill = true
		input.NeedServiceConfirmation = true
		// 产品明细完全由转账快照派生，忽略旧客户端或篡改请求提交的自定义内容。
		input.ProductItems = nil
	}
	return input
}

func validateCreateInvoiceInput(input CreateInvoiceRequestInput) error {
	hasOrders := len(input.Orders) > 0
	hasManualTransfers := len(input.ManualTransactions) > 0
	if !hasOrders && !hasManualTransfers {
		return errors.New("请选择平台订单或填写银行转账")
	}
	if hasOrders && hasManualTransfers {
		return errors.New("平台订单和银行转账不能在同一申请中混用")
	}
	if hasOrders && len(input.ProductItems) > 0 {
		return errors.New("平台订单的产品明细由系统快照生成，不能自定义")
	}
	if len(input.Orders) > 100 {
		return errors.New("单次最多选择 100 笔订单")
	}
	if len(input.ManualTransactions) > 50 {
		return errors.New("单次最多填写 50 笔银行转账")
	}
	if input.Title == "" {
		if input.InvoiceType == InvoiceTypeSpecial {
			return errors.New("单位名称不能为空")
		}
		return errors.New("发票抬头不能为空")
	}
	if len([]rune(input.Title)) > 128 {
		return errors.New("发票抬头不能超过 128 个字符")
	}
	if input.InvoiceType == InvoiceTypeSpecial && input.TaxNumber == "" {
		return errors.New("专票需要填写税号")
	}
	if input.TitleType == InvoiceTitleTypeCompany && input.TaxNumber == "" {
		return errors.New("企业抬头需要填写税号")
	}
	if len([]rune(input.TaxNumber)) > 64 {
		return errors.New("税号不能超过 64 个字符")
	}
	if len([]rune(input.RegisteredAddress)) > 255 {
		return errors.New("注册地址不能超过 255 个字符")
	}
	if len([]rune(input.RegisteredPhone)) > 64 {
		return errors.New("注册电话不能超过 64 个字符")
	}
	if len([]rune(input.BankName)) > 128 {
		return errors.New("开户银行不能超过 128 个字符")
	}
	if len([]rune(input.BankAccount)) > 128 {
		return errors.New("银行账号不能超过 128 个字符")
	}
	if input.Email == "" {
		return errors.New("接收邮箱不能为空")
	}
	if len([]rune(input.Email)) > 128 {
		return errors.New("接收邮箱不能超过 128 个字符")
	}
	if err := common.ValidateEmailAddress(input.Email); err != nil {
		return errors.New("接收邮箱格式不正确")
	}
	if len([]rune(input.Phone)) > 32 {
		return errors.New("手机号不能超过 32 个字符")
	}
	if len([]rune(input.Remark)) > 1000 {
		return errors.New("备注不能超过 1000 个字符")
	}
	for index, transfer := range input.ManualTransactions {
		if err := validateInvoiceManualTransaction(transfer); err != nil {
			return fmt.Errorf("第 %d 笔银行转账：%w", index+1, err)
		}
	}
	return nil
}

func validateInvoiceManualTransaction(input InvoiceManualTransactionInput) error {
	if input.TradeNo == "" {
		return errors.New("银行流水号不能为空")
	}
	if len([]rune(input.TradeNo)) > 255 {
		return errors.New("银行流水号不能超过 255 个字符")
	}
	if normalizeInvoiceFingerprintPart(input.TradeNo) == "" {
		return errors.New("银行流水号必须包含字母或数字")
	}
	if input.PayerName == "" {
		return errors.New("付款方名称不能为空")
	}
	if len([]rune(input.PayerName)) > 128 {
		return errors.New("付款方名称不能超过 128 个字符")
	}
	if normalizeInvoiceFingerprintPart(input.PayerName) == "" {
		return errors.New("付款方名称必须包含文字或数字")
	}
	if input.PayeeName == "" {
		return errors.New("收款方名称不能为空")
	}
	if len([]rune(input.PayeeName)) > 128 {
		return errors.New("收款方名称不能超过 128 个字符")
	}
	if input.PayeeName != InvoiceManualTransferPayeeName {
		return fmt.Errorf("收款方必须为%s", InvoiceManualTransferPayeeName)
	}
	if input.TransferBankName == "" {
		return errors.New("付款银行不能为空")
	}
	if len([]rune(input.TransferBankName)) > 128 {
		return errors.New("付款银行不能超过 128 个字符")
	}
	if normalizeInvoiceFingerprintPart(input.TransferBankName) == "" {
		return errors.New("付款银行必须包含文字或数字")
	}
	if err := validateInvoiceMoney(input.Money, true); err != nil {
		return err
	}
	if input.PaidAt <= 0 {
		return errors.New("转账时间不能为空")
	}
	if input.PaidAt > common.GetTimestamp() {
		return errors.New("转账时间不能晚于当前时间")
	}
	if len([]rune(input.Remark)) > 500 {
		return errors.New("转账备注不能超过 500 个字符")
	}
	return nil
}

func validateInvoiceMoney(value float64, requirePositive bool) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(maxInvoiceMoney) {
		return errors.New("金额不合法")
	}
	if requirePositive && value <= 0 {
		return errors.New("金额必须大于 0")
	}
	amount := decimal.NewFromFloat(value)
	if !amount.Equal(amount.Round(2)) {
		return errors.New("金额最多保留 2 位小数")
	}
	return nil
}

func invoiceSourceTypeFromInput(input CreateInvoiceRequestInput) string {
	if len(input.ManualTransactions) > 0 {
		return InvoiceSourceTypeManualTransfer
	}
	return InvoiceSourceTypeSystemOrder
}

func invoiceManualTransferFingerprint(input InvoiceManualTransactionInput) string {
	canonical := strings.Join([]string{
		normalizeInvoiceFingerprintPart(input.TransferBankName),
		normalizeInvoiceFingerprintPart(input.TradeNo),
		normalizeInvoiceFingerprintPart(input.PayerName),
	}, "|")
	sum := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("%x", sum[:])
}

func normalizeInvoiceFingerprintPart(value string) string {
	value = strings.ToLower(norm.NFKC.String(strings.TrimSpace(value)))
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func buildInvoiceProductItems(sourceType string, totalMoney float64, totalQuota int64, transactionItems []InvoiceRequestItem) ([]InvoiceRequestProductItem, int64, error) {
	if sourceType == InvoiceSourceTypeManualTransfer {
		return []InvoiceRequestProductItem{{
			ProductName: InvoiceManualTransferProductName,
			Unit:        InvoiceManualTransferProductUnit,
			Quantity:    1,
			UnitPrice:   totalMoney,
			Money:       totalMoney,
		}}, 0, nil
	}

	name := "AI API 调用额度"
	for _, item := range transactionItems {
		if strings.TrimSpace(item.ProductName) != "" {
			name = strings.TrimSpace(item.ProductName)
			break
		}
	}
	return []InvoiceRequestProductItem{{
		ProductName: name,
		Unit:        "项",
		Quantity:    1,
		UnitPrice:   totalMoney,
		Money:       totalMoney,
		Quota:       totalQuota,
	}}, totalQuota, nil
}

func normalizeInvoiceType(invoiceType string) string {
	switch strings.ToLower(strings.TrimSpace(invoiceType)) {
	case InvoiceTypeSpecial:
		return InvoiceTypeSpecial
	default:
		return InvoiceTypeNormal
	}
}

func normalizeInvoiceTitleType(titleType string) string {
	switch strings.ToLower(strings.TrimSpace(titleType)) {
	case InvoiceTitleTypeCompany:
		return InvoiceTitleTypeCompany
	default:
		return InvoiceTitleTypePersonal
	}
}

func normalizeInvoiceOrderType(orderType string) string {
	switch strings.ToLower(strings.TrimSpace(orderType)) {
	case PaymentRecordTypeTopUp:
		return PaymentRecordTypeTopUp
	case PaymentOrderTypeSubscription:
		return PaymentOrderTypeSubscription
	case PaymentRecordTypeSellableTokenPurchase:
		return PaymentRecordTypeSellableTokenPurchase
	default:
		return ""
	}
}

func buildInvoiceRequestItemTx(tx *gorm.DB, userID int, orderType string, orderID int) (*InvoiceRequestItem, error) {
	occupied, err := isInvoiceOrderOccupiedTx(tx, orderType, orderID)
	if err != nil {
		return nil, err
	}
	if occupied {
		return nil, errors.New("订单已存在待审核或已开票申请")
	}

	switch orderType {
	case PaymentRecordTypeTopUp, PaymentOrderTypeSubscription:
		return buildTopUpInvoiceItemTx(tx, userID, orderType, orderID)
	case PaymentRecordTypeSellableTokenPurchase:
		return buildSellableTokenInvoiceItemTx(tx, userID, orderID)
	default:
		return nil, errors.New("订单类型不支持")
	}
}

func buildTopUpInvoiceItemTx(tx *gorm.DB, userID int, orderType string, orderID int) (*InvoiceRequestItem, error) {
	var topup TopUp
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&topup, "id = ? AND user_id = ?", orderID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceOrderUnavailable
		}
		return nil, err
	}
	if topup.Status != common.TopUpStatusSuccess {
		return nil, ErrInvoiceOrderUnavailable
	}
	actualOrderType := resolveTopUpPaymentOrderType(&topup)
	if actualOrderType != orderType {
		return nil, errors.New("订单类型不匹配")
	}
	paymentMoney, ok := topup.CNYPaymentAmount()
	if !ok {
		return nil, ErrInvoiceOrderUnavailable
	}
	return &InvoiceRequestItem{
		UserId:        userID,
		OrderType:     orderType,
		OrderId:       topup.Id,
		TradeNo:       topup.TradeNo,
		PaymentMethod: topup.PaymentMethod,
		Amount:        topup.Amount,
		Money:         paymentMoney,
		CreateTime:    topup.CreateTime,
		CompleteTime:  topup.CompleteTime,
	}, nil
}

func buildSellableTokenInvoiceItemTx(tx *gorm.DB, userID int, orderID int) (*InvoiceRequestItem, error) {
	var row sellableTokenPaymentRecordRow
	query := sellableTokenPaymentSelectQueryWithTx(tx, false).
		Where("sellable_token_orders.id = ? AND sellable_token_orders.user_id = ?", orderID, userID).
		Where("sellable_token_orders.status = ?", SellableTokenOrderStatusCompleted).
		Where("(sellable_token_issuances.status = ? OR sellable_token_issuances.id IS NULL)", SellableTokenIssuanceStatusIssued)
	if err := query.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceOrderUnavailable
		}
		return nil, err
	}
	return &InvoiceRequestItem{
		UserId:        userID,
		OrderType:     PaymentRecordTypeSellableTokenPurchase,
		OrderId:       row.Id,
		TradeNo:       formatSellableTokenPaymentTradeNo(row.TradeNo, row.UserId, row.Id),
		PaymentMethod: PaymentMethodWallet,
		Amount:        int64(row.PriceQuota),
		Money:         0,
		ProductName:   row.ProductName,
		CreateTime:    row.CreateTime,
		CompleteTime:  row.CompleteTime,
	}, nil
}

func isInvoiceOrderOccupiedTx(tx *gorm.DB, orderType string, orderID int) (bool, error) {
	if tx == nil {
		tx = DB
	}
	var count int64
	err := tx.Table("invoice_request_items").
		Joins("JOIN invoice_requests ON invoice_requests.id = invoice_request_items.invoice_request_id").
		Where("invoice_request_items.order_type = ? AND invoice_request_items.order_id = ?", orderType, orderID).
		Where("invoice_requests.status IN ?", invoiceOccupiedStatuses()).
		Count(&count).Error
	return count > 0, err
}

func isInvoiceManualTransferOccupiedTx(tx *gorm.DB, fingerprint string) (bool, error) {
	if tx == nil {
		tx = DB
	}
	if strings.TrimSpace(fingerprint) == "" {
		return false, errors.New("银行转账指纹不能为空")
	}
	var count int64
	err := tx.Table("invoice_request_items").
		Joins("JOIN invoice_requests ON invoice_requests.id = invoice_request_items.invoice_request_id").
		Where("invoice_request_items.manual_fingerprint = ?", fingerprint).
		Where("invoice_requests.status IN ?", invoiceOccupiedStatuses()).
		Count(&count).Error
	return count > 0, err
}

func isInvoiceDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "duplicate entry") ||
		strings.Contains(message, "duplicate key")
}

func listEligibleInvoiceTopUpRecords(tx *gorm.DB, userID int) ([]*PaymentRecord, error) {
	var topups []*TopUp
	excludeOrderTypes := []string{PaymentRecordTypeTopUp, PaymentOrderTypeSubscription}
	err := tx.Model(&TopUp{}).
		Where("user_id = ? AND status = ?", userID, common.TopUpStatusSuccess).
		Where(`NOT EXISTS (
			SELECT 1 FROM invoice_request_items
			JOIN invoice_requests ON invoice_requests.id = invoice_request_items.invoice_request_id
			WHERE invoice_request_items.order_type IN ?
			AND invoice_request_items.order_id = top_ups.id
			AND invoice_requests.status IN ?
		)`, excludeOrderTypes, invoiceOccupiedStatuses()).
		Order("create_time desc, id desc").
		Find(&topups).Error
	if err != nil {
		return nil, err
	}

	records := make([]*PaymentRecord, 0, len(topups))
	for _, topup := range topups {
		paymentMoney, ok := topup.CNYPaymentAmount()
		if !ok {
			continue
		}
		records = append(records, &PaymentRecord{
			Id:                 topup.Id,
			RecordType:         PaymentRecordTypeTopUp,
			UserId:             topup.UserId,
			TradeNo:            topup.TradeNo,
			PaymentMethod:      topup.PaymentMethod,
			Amount:             topup.Amount,
			Money:              paymentMoney,
			Currency:           "CNY",
			PaymentAmountKnown: true,
			Status:             topup.Status,
			CreateTime:         topup.CreateTime,
			CompleteTime:       topup.CompleteTime,
			OrderType:          resolveTopUpPaymentOrderType(topup),
		})
	}
	return records, nil
}

func listEligibleInvoiceSellableTokenRecords(tx *gorm.DB, userID int) ([]*PaymentRecord, error) {
	query := sellableTokenPaymentSelectQueryWithTx(tx, false).
		Where("sellable_token_orders.user_id = ?", userID).
		Where("sellable_token_orders.status = ?", SellableTokenOrderStatusCompleted).
		Where("(sellable_token_issuances.status = ? OR sellable_token_issuances.id IS NULL)", SellableTokenIssuanceStatusIssued).
		Where(`NOT EXISTS (
			SELECT 1 FROM invoice_request_items
			JOIN invoice_requests ON invoice_requests.id = invoice_request_items.invoice_request_id
			WHERE invoice_request_items.order_type = ?
			AND invoice_request_items.order_id = sellable_token_orders.id
			AND invoice_requests.status IN ?
		)`, PaymentRecordTypeSellableTokenPurchase, invoiceOccupiedStatuses()).
		Order("sellable_token_orders.create_time desc, sellable_token_orders.id desc")

	var rows []*sellableTokenPaymentRecordRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	records := make([]*PaymentRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, &PaymentRecord{
			Id:            row.Id,
			RecordType:    PaymentRecordTypeSellableTokenPurchase,
			UserId:        row.UserId,
			TradeNo:       formatSellableTokenPaymentTradeNo(row.TradeNo, row.UserId, row.Id),
			PaymentMethod: PaymentMethodWallet,
			Amount:        int64(row.PriceQuota),
			Money:         0,
			Status:        common.TopUpStatusSuccess,
			CreateTime:    row.CreateTime,
			CompleteTime:  row.CompleteTime,
			ProductId:     row.ProductId,
			ProductName:   row.ProductName,
			OrderType:     PaymentRecordTypeSellableTokenPurchase,
		})
	}
	return records, nil
}

func sellableTokenPaymentSelectQueryWithTx(tx *gorm.DB, includeUser bool) *gorm.DB {
	if tx == nil {
		tx = DB
	}
	selectClause := []string{
		"sellable_token_orders.id AS id",
		"sellable_token_orders.user_id AS user_id",
		"sellable_token_orders.product_id AS product_id",
		"sellable_token_products.name AS product_name",
		"sellable_token_orders.trade_no AS trade_no",
		"sellable_token_orders.price_quota AS price_quota",
		"sellable_token_orders.create_time AS create_time",
		"sellable_token_orders.complete_time AS complete_time",
		"sellable_token_issuances.status AS issuance_status",
	}
	query := tx.Table("sellable_token_orders").
		Joins("LEFT JOIN sellable_token_products ON sellable_token_products.id = sellable_token_orders.product_id").
		Joins("LEFT JOIN sellable_token_issuances ON sellable_token_issuances.source_id = sellable_token_orders.id AND sellable_token_issuances.source_type = ?", SellableTokenSourceTypeWallet)
	if includeUser {
		selectClause = append(selectClause, "users.username AS username", "users.display_name AS display_name")
		query = query.Joins("LEFT JOIN users ON users.id = sellable_token_orders.user_id")
	}
	return query.Select(strings.Join(selectClause, ", "))
}

func invoiceOccupiedStatuses() []string {
	return []string{InvoiceStatusPending, InvoiceStatusInvoiced}
}

func invoiceRequestListWithUser(tx *gorm.DB) *gorm.DB {
	return tx.Model(&InvoiceRequest{}).
		Select("invoice_requests.*, users.username AS username, users.display_name AS display_name").
		Joins("LEFT JOIN users ON users.id = invoice_requests.user_id")
}

func invoiceRequestDetailWithUser(tx *gorm.DB) *gorm.DB {
	return tx.Model(&InvoiceRequest{}).
		Select("invoice_requests.*, users.username AS username, users.display_name AS display_name, reviewers.username AS reviewer_username, reviewers.display_name AS reviewer_display_name").
		Joins("LEFT JOIN users ON users.id = invoice_requests.user_id").
		Joins("LEFT JOIN users AS reviewers ON reviewers.id = invoice_requests.reviewer_user_id")
}

func applyInvoiceRequestSearch(query *gorm.DB, params InvoiceRequestSearchParams, includeUsername bool) *gorm.DB {
	if params.Status != "" {
		query = query.Where("invoice_requests.status = ?", strings.TrimSpace(params.Status))
	}
	if includeUsername && strings.TrimSpace(params.Username) != "" {
		query = applyPaymentRecordUsernameFilter(query, params.Username, "users.id", "users.username")
	}
	return query
}

func reviewInvoiceRequest(id int, reviewerUserID int, targetStatus string, input InvoiceReviewInput) (*InvoiceRequest, error) {
	if id <= 0 {
		return nil, ErrInvoiceRequestNotFound
	}
	if reviewerUserID <= 0 {
		return nil, errors.New("审核人不存在")
	}
	if len([]rune(input.InvoiceNo)) > 128 {
		return nil, errors.New("发票号不能超过 128 个字符")
	}
	if len([]rune(input.InvoiceFileName)) > 255 {
		return nil, errors.New("发票文件名不能超过 255 个字符")
	}
	if len([]rune(input.InvoiceSentTo)) > 128 {
		return nil, errors.New("发票接收邮箱不能超过 128 个字符")
	}
	if input.InvoiceSentTo != "" {
		if err := common.ValidateEmailAddress(input.InvoiceSentTo); err != nil {
			return nil, errors.New("发票接收邮箱格式不正确")
		}
	}
	if input.InvoiceSendStatus != "" && !isValidInvoiceSendStatus(input.InvoiceSendStatus) {
		return nil, errors.New("邮件发送状态不合法")
	}
	if len([]rune(input.AdminRemark)) > 1000 {
		return nil, errors.New("审核备注不能超过 1000 个字符")
	}
	if targetStatus != InvoiceStatusInvoiced && targetStatus != InvoiceStatusRejected {
		return nil, errors.New("审核动作不合法")
	}

	var request InvoiceRequest
	now := common.GetTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Preload("Items").Preload("ProductItems").First(&request, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceRequestNotFound
			}
			return err
		}
		if request.Status != InvoiceStatusPending {
			return ErrInvoiceRequestAlreadyReviewed
		}
		if targetStatus == InvoiceStatusInvoiced {
			if input.InvoiceSentTo == "" {
				input.InvoiceSentTo = common.NormalizeEmailAddress(request.Email)
			}
			if len([]rune(input.InvoiceSentTo)) > 128 {
				return errors.New("发票接收邮箱不能超过 128 个字符")
			}
			if err := common.ValidateEmailAddress(input.InvoiceSentTo); err != nil {
				return errors.New("发票接收邮箱格式不正确")
			}
		}
		updates := map[string]interface{}{
			"status":           targetStatus,
			"reviewer_user_id": reviewerUserID,
			"admin_remark":     input.AdminRemark,
			"reviewed_at":      now,
		}
		if targetStatus == InvoiceStatusInvoiced {
			updates["invoice_no"] = input.InvoiceNo
			updates["invoice_url"] = input.InvoiceUrl
			updates["invoice_file_name"] = input.InvoiceFileName
			updates["invoice_file_path"] = input.InvoiceFilePath
			for key, value := range invoiceEmailAttachmentUpdates(input.InvoiceEmailAttachments) {
				updates[key] = value
			}
			updates["invoice_sent_to"] = input.InvoiceSentTo
			updates["invoice_send_status"] = input.InvoiceSendStatus
			updates["invoice_send_error"] = ""
		}
		result := tx.Model(&InvoiceRequest{}).Where("id = ? AND status = ?", id, InvoiceStatusPending).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrInvoiceRequestAlreadyReviewed
		}
		request.Status = targetStatus
		request.ReviewerUserId = reviewerUserID
		request.AdminRemark = input.AdminRemark
		request.ReviewedAt = now
		if targetStatus == InvoiceStatusInvoiced {
			request.InvoiceNo = input.InvoiceNo
			request.InvoiceUrl = input.InvoiceUrl
			request.InvoiceFileName = input.InvoiceFileName
			request.InvoiceFilePath = input.InvoiceFilePath
			request.InvoiceEmailAttachments = input.InvoiceEmailAttachments
			request.InvoiceSentTo = input.InvoiceSentTo
			request.InvoiceSendStatus = input.InvoiceSendStatus
			request.InvoiceSendError = ""
		}
		// 驳回时退还此前申请扣除的手续费（仅 pending→rejected 触发，RowsAffected 检查保证幂等）
		if targetStatus == InvoiceStatusRejected && request.ServiceFeeQuota > 0 {
			if err := tx.Model(&User{}).Where("id = ?", request.UserId).
				Update("quota", gorm.Expr("quota + ?", request.ServiceFeeQuota)).Error; err != nil {
				return err
			}
		}
		if targetStatus == InvoiceStatusRejected {
			// 驳回后释放人工转账唯一指纹，允许用户修正资料后重新申请。
			if err := tx.Model(&InvoiceRequestItem{}).
				Where("invoice_request_id = ? AND source_type = ?", request.Id, InvoiceSourceTypeManualTransfer).
				Update("manual_fingerprint", nil).Error; err != nil {
				return err
			}
			for i := range request.Items {
				request.Items[i].ManualFingerprint = nil
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if targetStatus == InvoiceStatusRejected && request.ServiceFeeQuota > 0 {
		if cacheErr := syncUserCacheByID(request.UserId); cacheErr != nil {
			common.SysLog("failed to sync user cache after invoice fee refund: " + cacheErr.Error())
		}
		RecordLog(request.UserId, LogTypeRefund, fmt.Sprintf(
			"发票申请（ID %d）被驳回，退还手续费 %d 额度", request.Id, request.ServiceFeeQuota))
	}
	return &request, nil
}

func invoiceEmailAttachmentUpdates(files InvoiceEmailAttachments) map[string]interface{} {
	updates := make(map[string]interface{})
	if files.DetailBillFilePath != "" {
		updates["detail_bill_file_name"] = files.DetailBillFileName
		updates["detail_bill_file_path"] = files.DetailBillFilePath
	}
	if files.ServiceConfirmationFilePath != "" {
		updates["service_confirmation_file_name"] = files.ServiceConfirmationFileName
		updates["service_confirmation_file_path"] = files.ServiceConfirmationFilePath
	}
	return updates
}

// UpdateInvoiceEmailAttachments stores uploads before sending, so an SMTP failure
// can be retried without uploading again. Unselected attachments are preserved.
func UpdateInvoiceEmailAttachments(id int, files InvoiceEmailAttachments) error {
	updates := invoiceEmailAttachmentUpdates(files)
	if len(updates) == 0 {
		return nil
	}
	result := DB.Model(&InvoiceRequest{}).
		Where("id = ? AND status = ?", id, InvoiceStatusInvoiced).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var matched int64
		if err := DB.Model(&InvoiceRequest{}).Where("id = ? AND status = ?", id, InvoiceStatusInvoiced).Count(&matched).Error; err != nil {
			return err
		}
		if matched == 0 {
			return ErrInvoiceRequestNotFound
		}
	}
	return nil
}

func UpdateInvoiceSendStatus(id int, status string, errorMessage string) (*InvoiceRequest, error) {
	return updateInvoiceSendStatus(id, "", status, errorMessage)
}

// UpdateInvoiceSendStatusWithRecipient 原子记录邮件结果和本次实际收件邮箱，
// 允许管理员修正历史错误邮箱后重发，同时保持发票审核状态不变。
func UpdateInvoiceSendStatusWithRecipient(id int, recipient string, status string, errorMessage string) (*InvoiceRequest, error) {
	return updateInvoiceSendStatus(id, recipient, status, errorMessage)
}

func updateInvoiceSendStatus(id int, recipient string, status string, errorMessage string) (*InvoiceRequest, error) {
	status = strings.TrimSpace(status)
	recipient = common.NormalizeEmailAddress(recipient)
	errorMessage = strings.TrimSpace(errorMessage)
	if id <= 0 {
		return nil, ErrInvoiceRequestNotFound
	}
	if !isValidInvoiceSendStatus(status) {
		return nil, errors.New("邮件发送状态不合法")
	}
	if recipient != "" {
		if len([]rune(recipient)) > 128 {
			return nil, errors.New("发票接收邮箱不能超过 128 个字符")
		}
		if err := common.ValidateEmailAddress(recipient); err != nil {
			return nil, errors.New("发票接收邮箱格式不正确")
		}
	}
	if len([]rune(errorMessage)) > 1000 {
		errorMessage = string([]rune(errorMessage)[:1000])
	}
	updates := map[string]interface{}{
		"invoice_send_status": status,
		"invoice_send_error":  errorMessage,
	}
	if recipient != "" {
		updates["invoice_sent_to"] = recipient
	}
	if status == InvoiceSendStatusSent {
		updates["invoice_sent_at"] = common.GetTimestamp()
	} else {
		updates["invoice_sent_at"] = int64(0)
	}
	result := DB.Model(&InvoiceRequest{}).
		Where("id = ? AND status = ?", id, InvoiceStatusInvoiced).
		Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		// MySQL 在提交值未变化时可能返回零影响行，需要区分幂等更新与记录不存在。
		var matched int64
		if err := DB.Model(&InvoiceRequest{}).
			Where("id = ? AND status = ?", id, InvoiceStatusInvoiced).
			Count(&matched).Error; err != nil {
			return nil, err
		}
		if matched == 0 {
			return nil, ErrInvoiceRequestNotFound
		}
	}
	return GetInvoiceRequestDetail(id)
}

func isValidInvoiceSendStatus(status string) bool {
	switch status {
	case InvoiceSendStatusPending, InvoiceSendStatusSent, InvoiceSendStatusFailed:
		return true
	default:
		return false
	}
}
