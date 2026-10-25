package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestChannelReasoningSettingsValidation(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Setting: common.GetPointer(`{"reasoning_adaptation":{"mode":"auto","rules":[{"model":"glm-5.3-flash","protocol":"chat","mapping":{"high":"high","max":"max"}}]}}`)}
	require.NoError(t, validateChannel(channel, false))
	setting := channel.GetSetting()
	require.Equal(t, "auto", setting.ReasoningAdaptation.Mode)
	require.Equal(t, "max", setting.ReasoningAdaptation.Rules[0].Mapping["max"])
	channel.Setting = common.GetPointer(`{"reasoning_adaptation":{"mode":"unknown"}}`)
	err := validateChannel(channel, false)
	require.ErrorContains(t, err, "reasoning_adaptation")
	require.NotContains(t, err.Error(), "客户端限制")
	channel.Setting = common.GetPointer(`{}`)
	require.NoError(t, validateChannel(channel, false))
	require.Empty(t, channel.GetSetting().ReasoningAdaptation.Mode)
}
