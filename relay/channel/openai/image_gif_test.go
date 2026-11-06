package openai

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGIFImageConversionAndResponse(t *testing.T) {
	var request dto.ImageRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"gpt-image-2.6-sunburst","prompt":"cat","output_format":"gif","frame_count":9,"frame_duration_ms":200,"size":"512x512"}`), &request))
	for _, mode := range []int{relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
		c.Request.Header.Set("Content-Type", "application/json")
		info := &relaycommon.RelayInfo{RelayMode: mode, RelayFormat: types.RelayFormatOpenAIImage}
		copied, err := common.DeepCopy(&request)
		require.NoError(t, err)
		converted, err := (&Adaptor{}).ConvertImageRequest(c, info, *copied)
		require.NoError(t, err)
		b, err := common.Marshal(converted)
		require.NoError(t, err)
		require.Equal(t, int64(9), gjson.GetBytes(b, "frame_count").Int())
		require.Equal(t, int64(200), gjson.GetBytes(b, "frame_duration_ms").Int())
		palette := color.Palette{color.Black, color.White}
		a := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		z := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
		z.SetColorIndex(0, 0, 1)
		var buf bytes.Buffer
		require.NoError(t, gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{a, z}, Delay: []int{20, 20}, LoopCount: 0}))
		body := `{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(buf.Bytes()) + `"}],"output_format":"gif"}`
		resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
		_, apiErr := OpenaiHandlerWithUsage(c, info, resp)
		require.Nil(t, apiErr)
		require.Equal(t, body, w.Body.String(), "GIF bytes must not be decoded or flattened by relay")
		require.Equal(t, 1, *info.ImageResponseCount, "frames are not separate billable output images")
	}
}
