package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestAutoDisablePreservesManualChannel(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, multi := range []bool{false, true} {
			t.Run(fmt.Sprintf("cache=%t/multi=%t", cached, multi), func(t *testing.T) {
				db := setupChannelBatchTestDB(t)
				oldCache := common.MemoryCacheEnabled
				common.MemoryCacheEnabled = cached
				t.Cleanup(func() { common.MemoryCacheEnabled = oldCache })
				ch := &Channel{Id: 1, Name: "manual", Key: "key", Status: common.ChannelStatusManuallyDisabled, Group: "default", Models: "example", ChannelInfo: ChannelInfo{IsMultiKey: multi, MultiKeySize: 1}}
				ch.SetOtherInfo(map[string]interface{}{"status_reason": "operator maintenance", "status_time": 123})
				require.NoError(t, db.Create(ch).Error)
				if cached {
					channelSyncLock.Lock()
					oldChannels := channelsIDM
					oldGroups := group2model2channels
					group2model2channels = map[string]map[string][]int{}
					copyChannel := *ch
					// Simulate the cross-node refresh delay.
					copyChannel.Status = common.ChannelStatusEnabled
					channelsIDM = map[int]*Channel{1: &copyChannel}
					channelSyncLock.Unlock()
					t.Cleanup(func() {
						channelSyncLock.Lock()
						channelsIDM = oldChannels
						group2model2channels = oldGroups
						channelSyncLock.Unlock()
					})
				}
				require.False(t, UpdateChannelStatus(1, "key", common.ChannelStatusAutoDisabled, "late upstream failure"))
				got, err := GetChannelById(1, true)
				require.NoError(t, err)
				require.Equal(t, common.ChannelStatusManuallyDisabled, got.Status)
				require.Equal(t, "operator maintenance", got.GetOtherInfo()["status_reason"])
				require.Empty(t, got.ChannelInfo.MultiKeyStatusList)
				if cached {
					cache, err := CacheGetChannel(1)
					require.NoError(t, err)
					require.Equal(t, common.ChannelStatusManuallyDisabled, cache.Status)
					require.Empty(t, cache.ChannelInfo.MultiKeyStatusList)
				}
			})
		}
	}
}
