package service

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoTerminalResultRefundsOnceAcrossRealtimeAndPolling(t *testing.T) {
	for i, source := range []string{BillingSourceWallet, BillingSourceSubscription, BillingSourceToken} {
		t.Run(source, func(t *testing.T) {
			truncate(t)
			user, token, ch, sub := 7400+i, 8400+i, 9400+i, 10400+i
			seedUser(t, user, 200)
			seedToken(t, token, user, "terminal-"+source, 100)
			seedChannel(t, ch)
			seedSubscription(t, sub, user, 1000, 100)
			task := makeTask(user, ch, 50, token, source, sub)
			require.NoError(t, model.DB.Create(task).Error)
			stale := *task
			result := &relaycommon.TaskInfo{Status: model.TaskStatusFailure, Reason: "provider rejected", Progress: "100%"}
			require.NoError(t, ApplyVideoTaskResult(context.Background(), &mockAdaptor{}, task, result, []byte(`{"done":true}`)))
			require.NoError(t, ApplyVideoTaskResult(context.Background(), &mockAdaptor{}, task, result, nil))
			require.NoError(t, ApplyVideoTaskResult(context.Background(), &mockAdaptor{}, &stale, result, nil))
			wantWallet := 200
			if source == BillingSourceWallet {
				wantWallet += 50
			}
			require.Equal(t, wantWallet, getUserQuota(t, user))
			require.Equal(t, 150, getTokenRemainQuota(t, token))
			if source == BillingSourceSubscription {
				require.EqualValues(t, 50, getSubscriptionUsed(t, sub))
			}
			require.EqualValues(t, 1, countLogs(t))
		})
	}
}

func TestVideoTerminalResultSettlesOnceAndRetainsData(t *testing.T) {
	truncate(t)
	seedUser(t, 7410, 200)
	seedToken(t, 8410, 7410, "terminal-success", 100)
	seedChannel(t, 9410)
	task := makeTask(7410, 9410, 50, 8410, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	stale := *task
	result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, Url: "https://example.test/video", Progress: "100%"}
	adaptor := &mockAdaptor{adjustReturn: 30}
	require.NoError(t, ApplyVideoTaskResult(context.Background(), adaptor, task, result, []byte(`{"done":true}`)))
	require.NoError(t, ApplyVideoTaskResult(context.Background(), adaptor, task, result, nil))
	require.NoError(t, ApplyVideoTaskResult(context.Background(), adaptor, &stale, result, nil))
	require.Equal(t, 220, getUserQuota(t, 7410))
	require.Equal(t, 120, getTokenRemainQuota(t, 8410))
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, task.ID).Error)
	require.Equal(t, 30, saved.Quota)
	require.Equal(t, `{"done":true}`, string(saved.Data))
	require.Equal(t, "https://example.test/video", saved.GetResultURL())
	require.NotZero(t, saved.FinishTime)
}

func TestVideoTerminalResultPersistenceFailureDoesNotBill(t *testing.T) {
	truncate(t)
	seedUser(t, 7411, 200)
	seedToken(t, 8411, 7411, "terminal-db-failure", 100)
	task := makeTask(7411, 0, 50, 8411, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	const hook = "test:reject-task-terminal"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			tx.AddError(errors.New("injected persistence failure"))
		}
	}))
	t.Cleanup(func() { model.DB.Callback().Update().Remove(hook) })
	err := ApplyVideoTaskResult(context.Background(), &mockAdaptor{}, task, &relaycommon.TaskInfo{Status: model.TaskStatusFailure}, nil)
	require.Error(t, err)
	require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	require.Equal(t, 200, getUserQuota(t, 7411))
	require.Equal(t, 100, getTokenRemainQuota(t, 8411))
	require.Zero(t, countLogs(t))
}

func TestVideoTerminalResultSettlementKeepsFundingSourceAndFailedQuota(t *testing.T) {
	for i, source := range []string{BillingSourceSubscription, BillingSourceToken} {
		t.Run(source, func(t *testing.T) {
			truncate(t)
			user, token, sub := 7420+i, 8420+i, 10420+i
			seedUser(t, user, 200)
			seedToken(t, token, user, "success-"+source, 100)
			seedSubscription(t, sub, user, 1000, 100)
			task := makeTask(user, 0, 50, token, source, sub)
			require.NoError(t, model.DB.Create(task).Error)
			require.NoError(t, ApplyVideoTaskResult(context.Background(), &mockAdaptor{adjustReturn: 30}, task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}, nil))
			require.Equal(t, 200, getUserQuota(t, user))
			require.Equal(t, 120, getTokenRemainQuota(t, token))
			if source == BillingSourceSubscription {
				require.EqualValues(t, 80, getSubscriptionUsed(t, sub))
			}
			var saved model.Task
			require.NoError(t, model.DB.First(&saved, task.ID).Error)
			require.Equal(t, 30, saved.Quota)
		})
	}
	t.Run("funding-failure-retains-reserved-quota", func(t *testing.T) {
		truncate(t)
		seedUser(t, 7422, 200)
		seedToken(t, 8422, 7422, "success-funding-failure", 100)
		task := makeTask(7422, 0, 50, 8422, BillingSourceSubscription, 99999)
		require.NoError(t, model.DB.Create(task).Error)
		require.NoError(t, ApplyVideoTaskResult(context.Background(), &mockAdaptor{adjustReturn: 30}, task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}, nil))
		var saved model.Task
		require.NoError(t, model.DB.First(&saved, task.ID).Error)
		require.Equal(t, 50, saved.Quota)
		require.Equal(t, 100, getTokenRemainQuota(t, 8422))
		require.Zero(t, countLogs(t))
	})
}
