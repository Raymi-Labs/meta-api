package model

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type CashReportMetric struct {
	Name          string   `json:"name"`
	Income        *float64 `json:"income"`
	Cost          *float64 `json:"cost"`
	Profit        *float64 `json:"profit"`
	Margin        *float64 `json:"margin"`
	KnownIncome   float64  `json:"known_income"`
	KnownCost     float64  `json:"known_cost"`
	Requests      int64    `json:"requests"`
	KnownRequests int64    `json:"known_requests"`
	Quota         int64    `json:"quota"`
	KnownQuota    int64    `json:"known_quota"`
	UsageUSD      float64  `json:"usage_usd"`
	KnownCostUSD  float64  `json:"known_cost_usd"`
	Status        string   `json:"status"`
}

type CashReportIssue struct {
	Code    string `json:"code"`
	Count   int64  `json:"count"`
	Message string `json:"message"`
	Action  string `json:"action"`
}

type CashReport struct {
	Config       CashAccountingConfig `json:"config"`
	AccountID    int                  `json:"account_id"`
	StartDate    string               `json:"start_date"`
	EndDate      string               `json:"end_date"`
	Currency     string               `json:"currency"`
	QuotaPerUnit float64              `json:"quota_per_unit"`
	Sync         CashAccountSync      `json:"sync"`
	Bindings     []CashAccountBinding `json:"bindings"`
	Summary      CashReportMetric     `json:"summary"`
	Days         []CashReportMetric   `json:"days"`
	Models       []CashReportMetric   `json:"models"`
	Channels     []CashReportMetric   `json:"channels"`
	Issues       []CashReportIssue    `json:"issues"`
	CashFlow     CashReportFlow       `json:"cash_flow"`
	GeneratedAt  int64                `json:"generated_at"`
}

type CashReportFlow struct {
	SiteConfirmedReceipts float64 `json:"site_confirmed_receipts"`
	SiteReceiptCount      int64   `json:"site_receipt_count"`
	PendingFundingLots    int64   `json:"pending_funding_lots"`
	AccountPayments       float64 `json:"account_payments"`
	AccountPaymentCount   int64   `json:"account_payment_count"`
}

type cashMetric struct {
	name                                                                            string
	income, cost, quota, knownQuota, requests, knownRequests, usage, knownCostUsage int64
	incomeUnknown, costUnknown                                                      bool
	estimate                                                                        bool
}

func (m *cashMetric) add(other *cashMetric) {
	m.income += other.income
	m.cost += other.cost
	m.quota += other.quota
	m.knownQuota += other.knownQuota
	m.requests += other.requests
	m.knownRequests += other.knownRequests
	m.usage += other.usage
	m.knownCostUsage += other.knownCostUsage
	m.incomeUnknown = m.incomeUnknown || other.incomeUnknown
	m.costUnknown = m.costUnknown || other.costUnknown
	m.estimate = m.estimate || other.estimate
}

func (m *cashMetric) view() CashReportMetric {
	result := CashReportMetric{Name: m.name, KnownIncome: cashAmount(m.income), KnownCost: cashAmount(m.cost), Requests: m.requests, KnownRequests: m.knownRequests, Quota: m.quota, KnownQuota: m.knownQuota, UsageUSD: float64(m.usage) / 1e9, KnownCostUSD: float64(m.knownCostUsage) / 1e9, Status: "verified"}
	if !m.incomeUnknown {
		v := cashAmount(m.income)
		result.Income = &v
	}
	if !m.costUnknown {
		v := cashAmount(m.cost)
		result.Cost = &v
	}
	if result.Income != nil && result.Cost != nil {
		v := cashAmount(m.income - m.cost)
		result.Profit = &v
		if m.income > 0 {
			margin := float64(m.income-m.cost) / float64(m.income) * 100
			result.Margin = &margin
		}
	}
	if m.incomeUnknown || m.costUnknown {
		result.Status = "pending"
	} else if m.estimate {
		result.Status = "estimated"
	}
	return result
}

func cashReportRange(start, end string, now int64) (time.Time, time.Time, error) {
	today, _ := time.Parse("2006-01-02", cashDay(now))
	if start == "" && end == "" {
		return today.AddDate(0, 0, -7), today, nil
	}
	s, err := time.Parse("2006-01-02", start)
	if err != nil {
		return s, s, errors.New("请选择有效的 UTC 日期范围")
	}
	e, err := time.Parse("2006-01-02", end)
	if err != nil {
		return s, e, errors.New("请选择有效的 UTC 日期范围")
	}
	e = e.AddDate(0, 0, 1)
	if !s.Before(e) || e.After(today) || e.Sub(s) > 93*24*time.Hour {
		return s, e, errors.New("收益范围最多九十三个完整 UTC 日，今天尚未结算")
	}
	return s, e, nil
}

type cashCostDay struct {
	totalUsage, selectedUsage, totalCost, expense, byokExpense int64
	known, byokConfirmed                                       bool
}

// Periodic weighted-average cost across the whole account. Consumption by
// other keys drains the same credit pool before the site's share is assigned.
func calculateCashCosts(start, end time.Time, entries []CashUpstreamEntry, activity []CashActivityDay, coverage map[string]bool, selected map[string]bool) (map[string]cashCostDay, error) {
	result := map[string]cashCostDay{}
	from := start
	latestOpening := ""
	for _, e := range entries {
		if e.Kind == "opening" && e.Date <= start.Format("2006-01-02") {
			d, _ := time.Parse("2006-01-02", e.Date)
			if e.Date > latestOpening {
				from, latestOpening = d, e.Date
			}
		}
	}
	if end.Sub(from) > 366*24*time.Hour {
		return nil, errors.New("购款核对跨度超过一年，请录入新的期初核对记录")
	}
	var boundCredits, boundCash, boundUsage, boundRequests int64
	for _, e := range entries {
		if e.CreditsNanos < 0 || e.CostCNYMicros < 0 || boundCredits > 1e17-e.CreditsNanos || boundCash > 1e17-e.CostCNYMicros {
			return nil, errors.New("账单合计超出范围")
		}
		boundCredits += e.CreditsNanos
		boundCash += e.CostCNYMicros
	}
	for _, a := range activity {
		if a.Requests < 0 || boundRequests > 1e15-a.Requests {
			return nil, errors.New("上游请求合计超出范围")
		}
		boundRequests += a.Requests
		if a.UsageNanos < 0 || a.BYOKNanos < 0 || boundUsage > 1e17-a.UsageNanos-a.BYOKNanos {
			return nil, errors.New("上游费用合计超出范围")
		}
		boundUsage += a.UsageNanos + a.BYOKNanos
	}
	byDate := map[string][]CashUpstreamEntry{}
	for _, e := range entries {
		byDate[e.Date] = append(byDate[e.Date], e)
	}
	usage := map[string]int64{}
	byokUsage := map[string]int64{}
	selectedUsage := map[string]int64{}
	for _, a := range activity {
		if a.KeyHash == "" {
			if usage[a.Date] > 1e18-a.UsageNanos {
				return nil, errors.New("上游费用合计超出范围")
			}
			usage[a.Date] += a.UsageNanos
		}
		if selected[a.KeyHash] {
			selectedUsage[a.Date] += a.UsageNanos
			byokUsage[a.Date] += a.BYOKNanos
		}
	}
	var credits, cash int64
	known := false
	for d := from; d.Before(end); d = d.AddDate(0, 0, 1) {
		day := d.Format("2006-01-02")
		cost := cashCostDay{totalUsage: usage[day], selectedUsage: selectedUsage[day]}
		refundUnresolved := false
		dayEntries := byDate[day]
		sort.SliceStable(dayEntries, func(i, j int) bool {
			if (dayEntries[i].Kind == "opening") != (dayEntries[j].Kind == "opening") {
				return dayEntries[i].Kind == "opening"
			}
			return dayEntries[i].ID < dayEntries[j].ID
		})
		for _, e := range dayEntries {
			switch e.Kind {
			case "opening":
				credits, cash, known = e.CreditsNanos, e.CostCNYMicros, true
			case "purchase":
				if credits > 1e18-e.CreditsNanos || cash > 1e18-e.CostCNYMicros {
					return nil, errors.New("购款余额合计超出范围")
				}
				credits += e.CreditsNanos
				cash += e.CostCNYMicros
			case "refund":
				if !known || e.CreditsNanos > credits || credits <= 0 {
					known, refundUnresolved = false, true
				} else {
					basis := cashMulDiv(cash, e.CreditsNanos, credits)
					// Retire the refunded credits' carrying basis. The difference
					// from actual net refund is an account expense (or recovery),
					// including fees even when all remaining credits are refunded.
					cost.expense += basis - e.CostCNYMicros
					credits -= e.CreditsNanos
					cash -= basis
				}
			case "expense":
				cost.expense += e.CostCNYMicros
			case "byok":
				cost.byokExpense += e.CostCNYMicros
				cost.byokConfirmed = true
			}
		}
		if !coverage[cashHash("", day)] {
			known = false
		}
		if cost.totalUsage > credits {
			known = false
		}
		if known && credits > 0 {
			cost.totalCost = cashMulDiv(cash, cost.totalUsage, credits)
			cash -= cost.totalCost
			credits -= cost.totalUsage
		}
		cost.known = (known || cost.totalUsage == 0) && !refundUnresolved
		if !coverage[cashHash("", day)] {
			cost.known = false
		}
		for hash := range selected {
			if !coverage[cashHash(hash, day)] {
				cost.known = false
			}
		}
		if cost.selectedUsage > cost.totalUsage {
			cost.known = false
		}
		if byokUsage[day] > 0 && !cost.byokConfirmed {
			cost.known = false
		}
		result[day] = cost
	}
	return result, nil
}

func GetCashReport(ctx context.Context, accountID int, startDate, endDate string) (*CashReport, error) {
	now := common.GetTimestamp()
	start, end, err := cashReportRange(startDate, endDate, now)
	if err != nil {
		return nil, err
	}
	if _, err = cashAccountForRead(accountID); err != nil {
		return nil, err
	}
	q := DB.WithContext(ctx)
	config, err := GetCashAccountingConfig()
	if err != nil {
		return nil, err
	}
	report := &CashReport{Config: config, AccountID: accountID, StartDate: start.Format("2006-01-02"), EndDate: end.AddDate(0, 0, -1).Format("2006-01-02"), Currency: "CNY", QuotaPerUnit: common.QuotaPerUnit, GeneratedAt: now,
		Bindings: []CashAccountBinding{}, Days: []CashReportMetric{}, Models: []CashReportMetric{}, Channels: []CashReportMetric{}, Issues: []CashReportIssue{}, Sync: CashAccountSync{AccountID: accountID}}
	if err = q.Where("account_id = ?", accountID).Find(&report.Bindings).Error; err != nil {
		return nil, err
	}
	if err = q.Where("account_id = ?", accountID).First(&report.Sync).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	issue := func(code string, count int64, message, action string) {
		if count > 0 {
			report.Issues = append(report.Issues, CashReportIssue{Code: code, Count: count, Message: message, Action: action})
		}
	}
	selected := map[string]bool{}
	hashes := []string{}
	var shared int64
	for _, b := range report.Bindings {
		selected[b.KeyHash] = true
		hashes = append(hashes, b.KeyHash)
		if !b.SiteOnly {
			shared++
		}
	}
	issue("not_enabled", boolCount(config.EnabledAt == 0), "现金收益尚未启用，现有余额和历史消费不会自动成为资金凭证", "enable")
	issue("no_binding", boolCount(len(hashes) == 0), "尚未关联此账户的本站调用密钥", "bindings")
	issue("shared_key", shared, "调用密钥可能包含站外使用，收入与成本范围尚未核实", "bindings")
	issue("sync_failed", boolCount(report.Sync.Error != ""), "财务同步失败，当前保留上次成功数据", "sync")
	var entries []CashUpstreamEntry
	if err = q.Where("account_id = ? AND voided_at = 0 AND date < ?", accountID, end.Format("2006-01-02")).Order("date asc, id asc").Limit(10001).Find(&entries).Error; err != nil {
		return nil, err
	}
	if len(entries) > 10000 {
		return nil, errors.New("账单数量超出单次核对范围")
	}
	loadFrom := start
	for _, e := range entries {
		if e.Kind == "opening" && e.Date <= report.StartDate {
			loadFrom, _ = time.Parse("2006-01-02", e.Date)
		}
	}
	if end.Sub(loadFrom) > 366*24*time.Hour {
		return nil, errors.New("请补充最近一年的期初核对记录")
	}
	var activity []CashActivityDay
	var coverRows []CashDayCoverage
	if err = q.Where("account_id = ? AND date >= ? AND date < ?", accountID, loadFrom.Format("2006-01-02"), end.Format("2006-01-02")).Limit(100001).Find(&activity).Error; err != nil {
		return nil, err
	}
	if len(activity) > 100000 {
		return nil, errors.New("日账明细过多，请缩小查询范围")
	}
	if err = q.Where("account_id = ? AND date >= ? AND date < ?", accountID, loadFrom.Format("2006-01-02"), end.Format("2006-01-02")).Find(&coverRows).Error; err != nil {
		return nil, err
	}
	coverage := map[string]bool{}
	for _, c := range coverRows {
		coverage[cashHash(c.KeyHash, c.Date)] = true
	}
	costDays, err := calculateCashCosts(start, end, entries, activity, coverage, selected)
	if err != nil {
		return nil, err
	}
	var usages []CashUsage
	if len(hashes) > 0 {
		if err = q.Where("key_hash IN ? AND occurred_at >= ? AND occurred_at < ?", hashes, start.Unix(), end.Unix()).Order("id asc").Limit(50001).Find(&usages).Error; err != nil {
			return nil, err
		}
	}
	if len(usages) > 50000 {
		return nil, errors.New("请求明细超过五万条，请缩小查询范围")
	}
	var totalQuota int64
	for _, u := range usages {
		if u.Quota < 0 || totalQuota > 1e17-u.Quota {
			return nil, errors.New("消费额度合计超出范围")
		}
		totalQuota += u.Quota
	}
	reservations := []string{}
	for _, u := range usages {
		if u.ReservationID != "" {
			reservations = append(reservations, u.ReservationID)
		}
	}
	allocByRequest := map[string][]CashFundingAllocation{}
	lotIDs := map[int64]bool{}
	for i := 0; i < len(reservations); i += 400 {
		var batch []CashFundingAllocation
		if err = q.Where("reservation_id IN ? AND quota > 0", reservations[i:min(i+400, len(reservations))]).Order("id asc").Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, a := range batch {
			allocByRequest[a.ReservationID] = append(allocByRequest[a.ReservationID], a)
			if a.LotID > 0 {
				lotIDs[a.LotID] = true
			}
		}
	}
	lots := map[int64]CashFundingLot{}
	receipts := map[int64]CashFundingReceipt{}
	riskRefs := map[string]bool{}
	ids := []int64{}
	for id := range lotIDs {
		ids = append(ids, id)
	}
	if len(ids) > 10000 {
		return nil, errors.New("资金批次过多，请缩小查询范围")
	}
	var totalBasis int64
	for i := 0; i < len(ids); i += 400 {
		var batch []CashFundingLot
		if err = q.Where("id IN ?", ids[i:min(i+400, len(ids))]).Find(&batch).Error; err != nil {
			return nil, err
		}
		receiptIDs := []int64{}
		refs := []string{}
		for _, l := range batch {
			lots[l.ID] = l
			if l.ReceiptID > 0 {
				receiptIDs = append(receiptIDs, l.ReceiptID)
			}
			if l.SourceType == "topup" {
				refs = append(refs, l.SourceRef)
			}
		}
		if len(receiptIDs) > 0 {
			var rs []CashFundingReceipt
			if err = q.Where("id IN ?", receiptIDs).Find(&rs).Error; err != nil {
				return nil, err
			}
			for _, r := range rs {
				if r.NetCNYMicros < 0 || totalBasis > 1e17-r.NetCNYMicros {
					return nil, errors.New("资金基础合计超出范围")
				}
				totalBasis += r.NetCNYMicros
				receipts[r.ID] = r
			}
		}
		if len(refs) > 0 {
			var risks []PaymentRiskCase
			if err = q.Select("trade_no").Where("trade_no IN ? AND status IN ?", refs, []string{PaymentRiskStatusOpen, PaymentRiskStatusReversed}).Find(&risks).Error; err != nil {
				return nil, err
			}
			for _, r := range risks {
				riskRefs[r.TradeNo] = true
			}
		}
	}
	// Exact cumulative rounding within each source lot keeps full allocations at
	// its net cash basis, regardless of report range or UI pagination.
	allocationMoney := map[int64]int64{}
	for i := 0; i < len(ids); i += 400 {
		var batch []CashFundingAllocation
		if err = q.Where("lot_id IN ? AND status = ? AND quota > 0", ids[i:min(i+400, len(ids))], "settled").Order("id asc").Limit(200001).Find(&batch).Error; err != nil {
			return nil, err
		}
		if len(batch) > 200000 {
			return nil, errors.New("资金批次分摊过多，请缩小查询范围")
		}
		used := map[int64]int64{}
		for _, a := range batch {
			l := lots[a.LotID]
			r, ok := receipts[l.ReceiptID]
			if !ok || l.TotalQuota <= 0 {
				continue
			}
			before := used[l.ID]
			after := before + a.Quota
			if after < before || after > l.TotalQuota {
				return nil, errors.New("资金分摊超过原批次额度，需要核对")
			}
			allocationMoney[a.ID] = cashMulDiv(r.NetCNYMicros, after, l.TotalQuota) - cashMulDiv(r.NetCNYMicros, before, l.TotalQuota)
			used[l.ID] = after
		}
	}
	days := map[string]*cashMetric{}
	models := map[string]*cashMetric{}
	channels := map[string]*cashMetric{}
	get := func(m map[string]*cashMetric, key string) *cashMetric {
		if m[key] == nil {
			m[key] = &cashMetric{name: key}
		}
		return m[key]
	}
	var unknownFunding int64
	for _, u := range usages {
		m := &cashMetric{quota: u.Quota, requests: 1}
		known := u.FundingSource == "wallet" && u.ReservationID != ""
		var allocated int64
		for _, a := range allocByRequest[u.ReservationID] {
			allocated += a.Quota
			l, exists := lots[a.LotID]
			if !exists || a.Status != "settled" || riskRefs[l.SourceRef] {
				known = false
				continue
			}
			if l.SourceType == "gift" {
				continue
			}
			value, ok := allocationMoney[a.ID]
			if !ok {
				known = false
			} else {
				m.income += value
			}
		}
		if allocated != u.Quota {
			known = false
		}
		if known {
			m.knownQuota = u.Quota
			m.knownRequests = 1
		} else {
			m.incomeUnknown = true
			unknownFunding++
		}
		get(days, cashDay(u.OccurredAt)).add(m)
		get(models, u.Model).add(m)
		get(channels, fmt.Sprint(u.ChannelID)).add(m)
	}
	issue("funding_unknown", unknownFunding, "部分消费缺少已核实的现金来源，套餐及未追溯的资金变动不会按零收入处理", "funding")
	issue("payment_risk", int64(len(riskRefs)), "存在支付风险或已回退的收款来源，需要核对现金退款", "funding")
	// Unmatched wallet settlements are a global completeness gap: their account
	// is unknown, so they conservatively prevent a false verified account total.
	var orphan int64
	if config.EnabledAt > 0 {
		if err = q.Model(&PulseWalletReservation{}).Where("status = ? AND updated_at >= ? AND updated_at < ?", "settled", max(start.Unix(), config.EnabledAt), end.Unix()).Where("NOT EXISTS (SELECT 1 FROM cash_usages WHERE cash_usages.reservation_id = pulse_wallet_reservations.id)").Count(&orphan).Error; err != nil {
			return nil, err
		}
	}
	issue("unmatched_settlement", orphan, "存在尚未关联渠道的已结算钱包请求，收入完整性待核对", "funding")
	activityByDay := map[string][]CashActivityDay{}
	officialRequests := map[string]int64{}
	officialModelRequests := map[string]int64{}
	for _, a := range activity {
		if selected[a.KeyHash] && a.Date >= report.StartDate {
			activityByDay[a.Date] = append(activityByDay[a.Date], a)
			officialRequests[a.Date] += a.Requests
			officialModelRequests[a.Model] += a.Requests
		}
	}
	var missingDays, unknownCosts, unmatchedDays int64
	summary := &cashMetric{name: "total"}
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		day := d.Format("2006-01-02")
		m := get(days, day)
		cost := costDays[day]
		complete := coverage[cashHash("", day)] && len(hashes) > 0
		for _, hash := range hashes {
			complete = complete && coverage[cashHash(hash, day)]
		}
		if !complete {
			missingDays++
			m.costUnknown = true
			m.incomeUnknown = true
		}
		if config.EnabledAt == 0 || config.EnabledAt > d.Unix() || shared > 0 || orphan > 0 {
			m.incomeUnknown = true
		}
		if officialRequests[day] != m.requests {
			m.incomeUnknown = true
			unmatchedDays++
		}
		rows := activityByDay[day]
		sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
		var prefix int64
		for _, a := range rows {
			item := &cashMetric{usage: a.UsageNanos}
			if cost.known && complete {
				item.cost = cashMulDiv(cost.totalCost, prefix+a.UsageNanos, cost.totalUsage) - cashMulDiv(cost.totalCost, prefix, cost.totalUsage)
				item.knownCostUsage = a.UsageNanos
			} else {
				item.costUnknown = true
			}
			prefix += a.UsageNanos
			m.add(item)
			modelMetric := get(models, a.Model)
			modelMetric.add(item)
			if a.Requests > 0 && modelMetric.requests == 0 {
				modelMetric.incomeUnknown = true
			}
		}
		m.cost += cost.expense + cost.byokExpense
		if cost.expense+cost.byokExpense != 0 {
			get(models, "直接费用 / BYOK").cost += cost.expense + cost.byokExpense
		}
		if !cost.known && complete {
			m.costUnknown = true
			unknownCosts++
		}
		if report.Sync.Error != "" {
			m.costUnknown = true
		}
		summary.add(m)
		report.Days = append(report.Days, m.view())
	}
	issue("missing_days", missingDays, "部分 UTC 日缺少完整上游日账，尚不能计算利润", "sync")
	issue("upstream_basis", unknownCosts, "缺少有效期初、购款或 BYOK 凭证，或 credits 余额无法闭合", "entries")
	issue("unmatched_usage", unmatchedDays, "本站与上游请求数量不一致，迟到日账、重试、失败或站外用量需要核对", "bindings")
	issue("capture_start", boolCount(config.EnabledAt == 0 || config.EnabledAt > start.Unix()), "所选范围包含首次采集之前的时间，历史收入保持待核实", "range")
	if len(hashes) == 0 {
		summary.costUnknown, summary.incomeUnknown = true, true
	}
	report.Summary = summary.view()
	for _, m := range models {
		if m.name != "直接费用 / BYOK" && officialModelRequests[m.name] != m.requests {
			m.incomeUnknown, m.costUnknown = true, true
		}
		if summary.incomeUnknown {
			m.incomeUnknown = true
		}
		if summary.costUnknown {
			m.costUnknown = true
		}
		report.Models = append(report.Models, m.view())
	}
	sort.Slice(report.Models, func(i, j int) bool { return report.Models[i].Name < report.Models[j].Name })
	// No fabricated channel cost split: the daily upstream API has no channel
	// dimension. Exact model costs remain visible; channels explicitly await it.
	var labels []Channel
	channelIDs := []int{}
	for _, u := range usages {
		channelIDs = append(channelIDs, u.ChannelID)
	}
	if len(channelIDs) > 0 {
		for i := 0; i < len(channelIDs); i += 400 {
			var part []Channel
			if err = q.Select("id", "name").Where("id IN ?", channelIDs[i:min(i+400, len(channelIDs))]).Find(&part).Error; err != nil {
				return nil, err
			}
			labels = append(labels, part...)
		}
	}
	names := map[string]string{}
	for _, c := range labels {
		names[fmt.Sprint(c.Id)] = c.Name
	}
	for id, m := range channels {
		m.costUnknown = true
		if summary.incomeUnknown {
			m.incomeUnknown = true
		}
		m.name = fmt.Sprintf("%s (#%s)", names[id], id)
		report.Channels = append(report.Channels, m.view())
	}
	sort.Slice(report.Channels, func(i, j int) bool { return report.Channels[i].Name < report.Channels[j].Name })
	// Current audited receipts restate their original received dates. Revisions
	// are not additional cash; opening cash basis is not a new customer receipt.
	var receiptRows []CashFundingReceipt
	if err = q.Model(&CashFundingReceipt{}).Select("cash_funding_receipts.*").
		Joins("JOIN cash_funding_lots ON cash_funding_lots.receipt_id = cash_funding_receipts.id").
		Where("cash_funding_lots.source_type = ? AND received_at >= ? AND received_at < ?", "topup", start.Unix(), end.Unix()).
		Limit(10001).Find(&receiptRows).Error; err != nil {
		return nil, err
	}
	if len(receiptRows) > 10000 {
		return nil, errors.New("到账记录过多，请缩小查询范围")
	}
	var netReceipts int64
	for _, r := range receiptRows {
		if r.NetCNYMicros < 0 || netReceipts > 1e18-r.NetCNYMicros {
			return nil, errors.New("到账金额合计超出范围")
		}
		netReceipts += r.NetCNYMicros
	}
	report.CashFlow.SiteConfirmedReceipts = cashAmount(netReceipts)
	report.CashFlow.SiteReceiptCount = int64(len(receiptRows))
	if err = q.Model(&CashFundingLot{}).Where("receipt_id = 0 AND source_type <> ?", "gift").Count(&report.CashFlow.PendingFundingLots).Error; err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Date < report.StartDate || e.Kind == "opening" {
			continue
		}
		amount := e.CostCNYMicros
		if e.Kind == "refund" {
			amount = -amount
		}
		report.CashFlow.AccountPayments += cashAmount(amount)
		report.CashFlow.AccountPaymentCount++
	}
	return report, nil
}

func boolCount(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
