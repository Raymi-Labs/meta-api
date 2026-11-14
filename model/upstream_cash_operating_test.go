package model

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestCashOperatingReportWithoutAnyCashTables(t *testing.T) {
	setupUpstreamAccountTestDB(t)
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Log{}))
	tables, err := DB.Migrator().GetTables()
	require.NoError(t, err)
	for _, table := range tables {
		if strings.HasPrefix(table, "cash_") {
			require.NoError(t, DB.Migrator().DropTable(table))
		}
	}
	require.False(t, DB.Migrator().HasTable(&CashAccountBinding{}))
	start := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -7)
	end := start.AddDate(0, 0, 7)
	channels := []Channel{{Id: 1, Name: "owned", Type: constant.ChannelTypeOpenRouter, Key: "own"}, {Id: 2, Name: "foreign", Type: constant.ChannelTypeOpenRouter, Key: "other"}, {Id: 3, Name: "mixed", Type: constant.ChannelTypeOpenRouter, Key: "own\nother"}}
	require.NoError(t, DB.Create(&channels).Error)
	logs := make([]Log, 50001)
	for i := range logs {
		logs[i] = Log{ChannelId: 1, Type: LogTypeConsume, ModelName: "local/model", CreatedAt: start.Unix(), Quota: 2}
	}
	require.NoError(t, DB.CreateInBatches(&logs, 100).Error)
	extra := []Log{{ChannelId: 1, Type: LogTypeConsume, CreatedAt: end.Unix(), Quota: 999}, {ChannelId: 2, Type: LogTypeConsume, CreatedAt: start.Unix(), Quota: 999}, {ChannelId: 3, Type: LogTypeConsume, CreatedAt: start.Unix(), Quota: 999}, {ChannelId: 1, Type: LogTypeError, CreatedAt: start.Unix(), Quota: 999}}
	require.NoError(t, DB.Create(&extra).Error)
	client := &http.Client{Transport: openRouterTestTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"data":{"total_credits":100,"total_usage":40}}`
		switch r.URL.Path {
		case "/api/v1/activity":
			body = fmt.Sprintf(`{"data":[{"date":%q,"model":"m","usage":5}]}`, start.Format("2006-01-02"))
		case "/api/v1/keys":
			body = fmt.Sprintf(`{"data":[{"hash":%q}]}`, cashKeyHash("own"))
			if r.URL.Query().Get("offset") != "0" {
				body = `{"data":[]}`
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	report, err := collectCashOperatingReport(context.Background(), &UpstreamAccount{Id: 36, AccountType: UpstreamAccountTypeOpenRouter, AccessToken: "test"}, start, end, client)
	require.NoError(t, err)
	require.Empty(t, report.Operations.LocalError)
	require.EqualValues(t, 100002, report.Summary.Quota)
	require.EqualValues(t, 50001, report.Summary.Requests)
	require.True(t, report.Operations.LocalAvailable)
	require.Len(t, report.Channels, 1)
	require.Equal(t, "owned (#1)", report.Channels[0].Name)
	require.Equal(t, "local/model", report.Models[0].Name)
	require.Equal(t, 5.0, *report.Operations.Usage)
	require.Equal(t, 100.0, *report.Operations.Credits.TotalCredits)
	require.Nil(t, report.Summary.Profit)
}
func TestCashOperatingPartialFailureAndRetention(t *testing.T) {
	start := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -7)
	end := start.AddDate(0, 0, 7)
	report := cashOperatingReportFromSources(1, start, end, &OpenRouterAccountFinance{CreditsError: "timeout", ActivityError: "timeout"}, nil, []cashLogAggregate{{Quota: 20, Requests: 2, Day: start.Unix()}}, nil)
	require.Nil(t, report.Operations.Usage)
	require.Nil(t, report.Operations.Credits)
	require.EqualValues(t, 20, report.Summary.Quota)
	failed := &CashReport{GeneratedAt: 999, Operations: &CashOperations{LocalError: "query failed"}}
	report.GeneratedAt = 123
	retainCashOperatingValues(failed, report)
	require.EqualValues(t, 20, failed.Summary.Quota)
	require.True(t, failed.Operations.LocalAvailable)
	require.EqualValues(t, 123, failed.GeneratedAt)
	require.Equal(t, "query failed", failed.Operations.LocalError)
}
