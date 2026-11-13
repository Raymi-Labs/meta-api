package service

import (
	"context"
	"github.com/QuantumNous/new-api/constant"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseRealtimeAndPlaygroundFunding(t *testing.T) {
	for _, kind := range []string{"realtime", "playground", "task"} {
		t.Run(kind, func(t *testing.T) {
			f := newEnterpriseBilling(t, 10000, 0)
			c, info := f.request(1000, 0)
			switch kind {
			case "realtime":
				info.RelayFormat = types.RelayFormatOpenAIRealtime
			case "playground":
				info.IsPlayground = true
			case "task":
				info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
			}
			require.Nil(t, PreConsumeBilling(c, 1000, info))
			if kind == "realtime" {
				info.PriceData.ModelRatio = 1
				usage := &dto.RealtimeUsage{InputTokens: 600, TotalTokens: 600}
				usage.InputTokenDetails.TextTokens = 600
				require.NoError(t, PreWssConsumeQuota(c, info, usage))
				require.NoError(t, PreWssConsumeQuota(c, info, usage))
				f.ledger(8800, 1200, 0)
			}
			require.NoError(t, SettleBilling(c, info, 1500))
			require.NoError(t, SettleBilling(c, info, 1500))
			f.ledger(8500, 0, 1500)
			require.Equal(t, 1500, f.tokenRow().UsedQuota)
			f.personalUntouched()
		})
	}
}

func TestEnterpriseVideoCompletionAndReplay(t *testing.T) {
	f := newEnterpriseBilling(t, 10000, 0)
	c, info := f.request(500, 0)
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	require.Nil(t, PreConsumeBilling(c, 500, info))
	require.NoError(t, SettleBilling(c, info, 500))
	task := model.InitTask("fixture", info)
	task.Quota = 500
	task.EnterpriseInitialQuota = 500
	task.Status = model.TaskStatusInProgress
	task.PrivateData.TokenId = f.token.Id
	task.PrivateData.BillingSource = BillingSourceEnterprise
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: "fixture-model", PerCallBilling: true, GroupRatio: 0.4}
	require.NoError(t, task.Insert())
	result := &relaycommon.TaskInfo{Status: model.TaskStatusFailure, Reason: "fixture failure"}
	require.NoError(t, ApplyVideoTaskResult(context.Background(), nil, task, result, nil))
	f.ledger(10000, 0, 0)
	require.Zero(t, f.tokenRow().UsedQuota)
	require.NoError(t, ApplyVideoTaskResult(context.Background(), nil, task, result, nil))
	f.ledger(10000, 0, 0)
	f.personalUntouched()
}

func TestEnterpriseTaskInitialChargeRecovery(t *testing.T) {
	f := newEnterpriseBilling(t, 10000, 0)
	c, info := f.request(500, 0)
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	require.Nil(t, PreConsumeBilling(c, 500, info))
	task := model.InitTask("fixture", info)
	task.Quota = 400
	task.EnterpriseInitialQuota = 400
	task.EnterpriseBillingPending = true
	task.PrivateData.TokenId = f.token.Id
	task.PrivateData.BillingSource = BillingSourceEnterprise
	require.NoError(t, task.Insert())
	// Simulate process death after task persistence but before SettleBilling.
	_, err := model.ReconcileEnterpriseTasks(100)
	require.NoError(t, err)
	f.ledger(9600, 0, 400)
	require.Equal(t, 400, f.tokenRow().UsedQuota)
	_, err = model.ReconcileEnterpriseTasks(100)
	require.NoError(t, err)
	f.ledger(9600, 0, 400)
	f.personalUntouched()
}

func TestEnterpriseTaskScopeSeparatesPayers(t *testing.T) {
	f := newEnterpriseBilling(t, 10000, 0)
	c, info := f.request(500, 0)
	c.Set("id", info.UserId)
	task := &model.Task{UserId: info.UserId, EnterpriseId: info.EnterpriseId, EnterpriseMemberId: info.EnterpriseMemberId}
	require.False(t, TaskPayerAllowed(c, task))
	// Scope is authoritative request context, not task parameters.
	common.SetContextKey(c, constant.ContextKeyEnterpriseId, info.EnterpriseId)
	common.SetContextKey(c, constant.ContextKeyEnterpriseMemberId, info.EnterpriseMemberId)
	require.True(t, TaskPayerAllowed(c, task))
	require.False(t, TaskPayerAllowed(c, &model.Task{UserId: info.UserId}))
	// Explicit cases are covered through API-token auth integration as well.
	require.False(t, TaskPayerAllowed(c, &model.Task{UserId: info.UserId + 1}))
}

func TestEnterpriseTaskChannelPinsOriginalEndpoint(t *testing.T) {
	current, original := "https://new.example.invalid", "https://original.example.invalid"
	ch := &model.Channel{Id: 17, BaseURL: &current}
	task := &model.Task{EnterpriseId: 3, PrivateData: model.TaskPrivateData{Key: "saved-fixture", EnterpriseBaseURL: original}}
	pinned := EnterpriseTaskChannel(ch, task)
	require.Equal(t, original, pinned.GetBaseURL())
	require.Equal(t, current, ch.GetBaseURL())
	require.Same(t, ch, EnterpriseTaskChannel(ch, &model.Task{}))
}
