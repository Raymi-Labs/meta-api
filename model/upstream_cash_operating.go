package model

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

// Operational usage is independent of cash receipts and cannot prove cash profit.
type CashOperations struct {
	LocalAvailable bool                      `json:"local_available"`
	Credits        *OpenRouterAccountCredits `json:"credits"`
	Usage          *float64                  `json:"usage"`
	CreditsError   string                    `json:"credits_error,omitempty"`
	UsageError     string                    `json:"usage_error,omitempty"`
	LocalError     string                    `json:"local_error,omitempty"`
}
type cashLogAggregate struct {
	ChannelID int
	ModelName string
	Day       int64
	Quota     int64
	Requests  int64
}

func queryCashLogAggregates(ctx context.Context, ids []int, start, end time.Time) ([]cashLogAggregate, error) {
	rows := []cashLogAggregate{}
	if len(ids) == 0 {
		return rows, nil
	}
	// Integer modulo and subtraction work on SQLite, MySQL and PostgreSQL.
	const day = "(created_at - (created_at % 86400))"
	err := LOG_DB.WithContext(ctx).Model(&Log{}).
		Select("channel_id, model_name, "+day+" AS day, SUM(quota) AS quota, COUNT(*) AS requests").
		Where("channel_id IN ? AND type = ? AND created_at >= ? AND created_at < ?", ids, LogTypeConsume, start.Unix(), end.Unix()).
		Group("channel_id, model_name, " + day).Scan(&rows).Error
	if err != nil {
		return nil, errors.New("本站消费日志统计失败")
	}
	return rows, nil
}
func cashOperatingReportFromSources(id int, start, end time.Time, finance *OpenRouterAccountFinance, channels []Channel, logs []cashLogAggregate, localErr error) *CashReport {
	report := &CashReport{AccountID: id, StartDate: start.Format("2006-01-02"), EndDate: end.AddDate(0, 0, -1).Format("2006-01-02"), GeneratedAt: common.GetTimestamp(), QuotaPerUnit: common.QuotaPerUnit,
		Days: []CashReportMetric{}, Models: []CashReportMetric{}, Channels: []CashReportMetric{}, Operations: &CashOperations{Credits: finance.Credits, CreditsError: finance.CreditsError, UsageError: finance.ActivityError}}
	op := report.Operations
	earliest := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -30)
	if start.Before(earliest) {
		op.UsageError = "上游消费仅提供最近30个完整UTC日"
	}
	if op.UsageError == "" && finance.Activity != nil {
		usage := 0.0
		for _, r := range finance.Activity {
			if r.Date >= report.StartDate && r.Date <= report.EndDate {
				usage += *r.Usage
			}
		}
		op.Usage = &usage
	}
	if localErr != nil {
		op.LocalError = localErr.Error()
		return report
	}
	op.LocalAvailable = true
	names := map[int]string{}
	for _, ch := range channels {
		names[ch.Id] = ch.Name
	}
	models, days, byChannel := map[string]*CashReportMetric{}, map[string]*CashReportMetric{}, map[string]*CashReportMetric{}
	add := func(m map[string]*CashReportMetric, key string, row cashLogAggregate) {
		if m[key] == nil {
			m[key] = &CashReportMetric{Name: key}
		}
		m[key].Quota += row.Quota
		m[key].Requests += row.Requests
	}
	for _, r := range logs {
		report.Summary.Quota += r.Quota
		report.Summary.Requests += r.Requests
		add(models, r.ModelName, r)
		add(days, time.Unix(r.Day, 0).UTC().Format("2006-01-02"), r)
		add(byChannel, fmt.Sprintf("%s (#%d)", names[r.ChannelID], r.ChannelID), r)
	}
	flatten := func(m map[string]*CashReportMetric) []CashReportMetric {
		result := []CashReportMetric{}
		for _, v := range m {
			result = append(result, *v)
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
		return result
	}
	report.Days, report.Models, report.Channels = flatten(days), flatten(models), flatten(byChannel)
	return report
}
func GetCashOperatingReport(ctx context.Context, id int, startDate, endDate string) (*CashReport, error) {
	start, end, err := cashReportRange(startDate, endDate, common.GetTimestamp())
	if err != nil {
		return nil, err
	}
	account, err := cashAccountForRead(id)
	if err != nil {
		return nil, err
	}
	if !account.Enabled {
		return nil, errors.New("OpenRouter account disabled")
	}
	return collectCashOperatingReport(ctx, account, start, end, newOpenRouterAccountClient())
}

func collectCashOperatingReport(ctx context.Context, account *UpstreamAccount, start, end time.Time, client *http.Client) (*CashReport, error) {
	var err error
	finance := fetchOpenRouterFinance(ctx, client, account.remoteObserverConfig())
	hashes, localErr := cashRemoteKeyHashes(ctx, client, account)
	var channels []Channel
	var rows []cashLogAggregate
	if localErr == nil {
		if err = DB.WithContext(ctx).Select("id", "name", "key").Where("type = ?", constant.ChannelTypeOpenRouter).Find(&channels).Error; err != nil {
			localErr = errors.New("本站渠道查询失败")
		}
	}
	if localErr == nil {
		ids := openRouterOwnedChannels(channels, hashes)
		if len(ids) == 0 {
			localErr = errors.New("未找到当前密钥归属此账户的本站渠道")
		} else {
			rows, localErr = queryCashLogAggregates(ctx, ids, start, end)
		}
	}
	return cashOperatingReportFromSources(account.Id, start, end, finance, channels, rows, localErr), nil
}

func retainCashOperatingValues(data, previous *CashReport) {
	if data.Operations == nil || previous == nil || previous.Operations == nil {
		return
	}
	op, old := data.Operations, previous.Operations
	retained := false
	if op.CreditsError != "" && old.Credits != nil {
		op.Credits = old.Credits
		retained = true
	}
	if op.UsageError != "" && old.Usage != nil {
		op.Usage = old.Usage
		retained = true
	}
	if op.LocalError != "" && old.LocalAvailable {
		op.LocalAvailable = true
		data.Summary, data.Days, data.Models, data.Channels = previous.Summary, previous.Days, previous.Models, previous.Channels
		retained = true
	}
	if retained {
		data.GeneratedAt = previous.GeneratedAt
	}
}
