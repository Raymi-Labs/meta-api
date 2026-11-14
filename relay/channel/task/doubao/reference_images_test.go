package doubao

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSeedancePhotoRequest(t *testing.T) {
	for _, tc := range []struct {
		name, body, upstream string
		roles                []string
		ratio                string
		invalid              bool
	}{
		{name: "image and images conflict", body: `{"image":"https://example.test/a.png","images":["https://example.test/b.png"]}`, invalid: true},
		{name: "unsupported audio does not silently vanish", body: `{"metadata":{"content":[{"type":"audio_url","audio_url":{"url":"https://example.test/a.mp3"},"role":"reference_audio"}]}}`, invalid: true},
		{name: "single reference", body: `{"image":"https://example.test/a.png"}`, roles: []string{"reference_image"}},
		{name: "two references stay references", body: `{"images":["https://example.test/a.png","https://example.test/b.jpg"],"aspect_ratio":"16:9"}`, roles: []string{"reference_image", "reference_image"}, ratio: "16:9"},
		{name: "input reference", body: fmt.Sprintf(`{"input_reference":%q}`, photoFixture(t, "png")), roles: []string{"reference_image"}},
		{name: "mapped international", upstream: "dreamina-seedance-2-5-260628", body: `{"images":["https://example.test/a.png"]}`, roles: []string{"reference_image"}},
		{name: "first frame", body: `{"mode":"first_frame","images":["https://example.test/a.png"]}`, roles: []string{"first_frame"}, ratio: "adaptive"},
		{name: "first last frame", body: `{"mode":"first_last_frame","images":["https://example.test/a.png","https://example.test/b.jpg"]}`, roles: []string{"first_frame", "last_frame"}, ratio: "adaptive"},
		{name: "explicit metadata frame", body: `{"metadata":{"content":[{"type":"image_url","image_url":{"url":"https://example.test/a.png"},"role":"first_frame"}]}}`, roles: []string{"first_frame"}, ratio: "adaptive"},
		{name: "frame ratio conflict", body: `{"mode":"first_frame","aspect_ratio":"16:9","image":"https://example.test/a.png"}`, invalid: true},
		{name: "reference mode empty", body: `{"mode":"reference"}`, invalid: true},
		{name: "text with photo", body: `{"mode":"text_to_video","image":"https://example.test/a.png"}`, invalid: true},
		{name: "frame count", body: `{"mode":"first_frame","images":["https://example.test/a.png","https://example.test/b.jpg"]}`, invalid: true},
		{name: "ambiguous sources", body: `{"image":"https://example.test/a.png","metadata":{"content":[]}}`, invalid: true},
		{name: "ambiguous reference", body: `{"image":"https://example.test/a.png","input_reference":"https://example.test/b.jpg"}`, invalid: true},
		{name: "local path", body: `{"images":["/tmp/photo.png"]}`, invalid: true},
		{name: "blob URL", body: `{"images":["blob:https://example.test/a"]}`, invalid: true},
		{name: "mixed roles", body: `{"metadata":{"content":[{"type":"image_url","image_url":{"url":"https://example.test/a.png"},"role":"first_frame"},{"type":"image_url","image_url":{"url":"https://example.test/b.png"},"role":"reference_image"}]}}`, invalid: true},
		{name: "lone last frame", body: `{"metadata":{"content":[{"type":"image_url","image_url":{"url":"https://example.test/a.png"},"role":"last_frame"}]}}`, invalid: true},
		{name: "legacy model unchanged", upstream: "doubao-seedance-1-0-pro-250528", body: `{"images":["https://example.test/a.png"]}`, roles: []string{""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := tc.upstream
			if upstream == "" {
				upstream = "doubao-seedance-2-5-260628"
			}
			var payload map[string]any
			require.NoError(t, common.Unmarshal([]byte(tc.body), &payload))
			payload["model"] = "public-alias"
			payload["prompt"] = "use this character"
			raw, err := common.Marshal(payload)
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(string(raw)))
			c.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(c)
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: upstream}}
			a := &TaskAdaptor{}
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			reader, err := a.BuildRequestBody(c, info)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			var body requestPayload
			require.NoError(t, common.Unmarshal(data, &body))
			require.Equal(t, upstream, body.Model)
			roles := []string(nil)
			for _, item := range body.Content {
				if item.Type == "image_url" {
					roles = append(roles, item.Role)
				}
			}
			require.Equal(t, tc.roles, roles)
			require.Equal(t, tc.ratio, body.Ratio)
		})
	}
}

func TestSeedanceReferenceCount(t *testing.T) {
	for _, n := range []int{1, 30, 31} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			req := &relaycommon.TaskSubmitReq{Images: make([]string, n)}
			for i := range req.Images {
				req.Images[i] = "https://example.test/image.png"
			}
			body := &requestPayload{Model: "doubao-seedance-2-5-260628"}
			err := normalizeSeedanceImages(req, body)
			if n > 30 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Len(t, body.Content, n)
			}
		})
	}
}

func photoFixture(t *testing.T, format string) string {
	t.Helper()
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	if format == "jpeg" {
		require.NoError(t, jpeg.Encode(&b, img, nil))
	} else {
		require.NoError(t, png.Encode(&b, img))
	}
	return "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestSeedancePhotoBytes(t *testing.T) {
	valid := photoFixture(t, "png")
	_, encoded, _ := strings.Cut(valid, ",")
	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	oversized := append([]byte(nil), raw...)
	binary.BigEndian.PutUint32(oversized[16:20], 8193)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	for _, tc := range []struct {
		name, value string
		budget      int64
		invalid     bool
	}{
		{"png", valid, 64 << 20, false},
		{"jpeg", photoFixture(t, "jpeg"), 64 << 20, false},
		{"remote remains remote", "https://example.invalid/photo.png", 64 << 20, false},
		{"invalid base64", "data:image/png;base64,%%%", 64 << 20, true},
		{"mime mismatch", strings.Replace(valid, "image/png", "image/jpeg", 1), 64 << 20, true},
		{"truncated", "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw[:len(raw)-15]), 64 << 20, true},
		{"dimension budget", "data:image/png;base64," + base64.StdEncoding.EncodeToString(oversized), 64 << 20, true},
		{"aggregate budget", valid, int64(len(raw) - 1), true},
		{"encoded size budget", "data:image/png;base64," + strings.Repeat("A", base64.StdEncoding.EncodedLen(20<<20)+1), 64 << 20, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSeedancePhoto(tc.value, &tc.budget)
			if tc.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
