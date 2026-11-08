package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckUserChannelGroupConflictExemptsDefault(t *testing.T) {
	channelGroups := map[string]float64{"default": 1, "vip": 1, "OpenAI · 优质": 0.068}

	require.NoError(t, CheckUserChannelGroupConflict(map[string]string{"default": "", "svip": ""}, channelGroups))

	err := CheckUserChannelGroupConflict(map[string]string{"default": "", "vip": "", "OpenAI · 优质": "", "分销商": ""}, channelGroups)
	require.EqualError(t, err, "用户分组与渠道分组不能重名：OpenAI · 优质、vip")
}
