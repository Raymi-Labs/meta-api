package helper

import (
	"bytes"
	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestChannelTestMissingGroupRatioKeepsBusinessDiagnostics(t *testing.T) {
	old, oldSpecial := ratio_setting.GroupRatio2JSONString(), ratio_setting.GroupGroupRatio2JSONString()
	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultWriter
	var log bytes.Buffer
	gin.DefaultWriter = &log
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = oldWriter
		common.LogWriterMu.Unlock()
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(old))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldSpecial))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"paid":0.4,"free":0}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"default":0.0001,"free":0}}`))
	for _, tc := range []struct {
		name, user, group string
		test              bool
		want              float64
		special, warn     bool
	}{
		{"test missing", "default", "default", true, 1, false, false},
		{"real missing", "default", "default", false, 1, false, true},
		{"configured test", "default", "paid", true, 0.4, false, false},
		{"zero test", "default", "free", true, 0, false, false},
		{"special test", "vip", "default", true, 0.0001, true, false},
		{"special zero", "vip", "free", true, 0, true, false},
		{"special real", "vip", "default", false, 0.0001, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log.Reset()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{IsChannelTest: tc.test, UserGroup: tc.user, UsingGroup: tc.group}
			got := HandleGroupRatio(c, info)
			require.Equal(t, tc.want, got.GroupRatio)
			require.Equal(t, tc.special, got.HasSpecialRatio)
			if tc.warn {
				require.Contains(t, log.String(), "group ratio not found: default")
			} else {
				require.NotContains(t, log.String(), "group ratio not found:")
			}
			require.False(t, ratio_setting.ContainsGroupRatio("default"), "tests must not restore catalog groups")
		})
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("auto_group", "paid")
	info := &relaycommon.RelayInfo{IsChannelTest: true, UserGroup: "default", UsingGroup: "default"}
	require.Equal(t, 0.4, HandleGroupRatio(c, info).GroupRatio)
	require.Equal(t, "paid", info.UsingGroup)
}
