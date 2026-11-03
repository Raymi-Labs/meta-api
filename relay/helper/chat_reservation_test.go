package helper

import (
	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestChatInitialReservationSeparatesMaximumOutput(t *testing.T) {
	old := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"chat-reserve-fixture":6600}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(old)) })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, tokens := range []int{128000, 1000000, 128} {
		info := &relaycommon.RelayInfo{OriginModelName: "chat-reserve-fixture", UsingGroup: "default", UserGroup: "default"}
		meta := &types.TokenCountMeta{MaxTokens: tokens}
		price, err := ModelPriceHelper(c, info, 100, meta)
		require.NoError(t, err)
		require.Equal(t, tokens, meta.MaxTokens)
		require.Equal(t, tokens, price.RequestedMaxTokens)
		require.LessOrEqual(t, price.InitialChatQuota, common.GetTrustQuota())
		require.LessOrEqual(t, price.InitialChatQuota, price.QuotaToPreConsume)
		require.Equal(t, min(tokens, 4096), price.InitialCompletionTokens)

	}
	for _, fileType := range []types.FileType{types.FileTypeAudio, types.FileTypeVideo} {
		info := &relaycommon.RelayInfo{OriginModelName: "chat-reserve-fixture", UsingGroup: "default", UserGroup: "default"}
		price, err := ModelPriceHelper(c, info, 100, &types.TokenCountMeta{MaxTokens: 128000, Files: []*types.FileMeta{{FileType: fileType}}})
		require.NoError(t, err)
		require.Zero(t, price.InitialChatQuota)
	}
}
