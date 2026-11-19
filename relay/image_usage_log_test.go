package relay

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageUsageLogContent(t *testing.T) {
	for _, q := range []string{"low", "medium", "high", "auto", "standard", "hd"} {
		require.Equal(t, []string{"大小 1024x1024", "请求品质 " + q, "生成数量 1"}, imageUsageLogContent("1024x1024", q, 1))
	}
	for _, q := range []string{"", "unrecognized\nvalue"} {
		require.Equal(t, []string{"生成数量 2"}, imageUsageLogContent("", q, 2))
	}
}
