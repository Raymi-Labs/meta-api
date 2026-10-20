package service

import (
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
)

const upstreamRequestIDsKey = "upstream_response_request_ids"

type upstreamRequestID struct {
	ChannelID int    `json:"channel_id"`
	ID        string `json:"request_id"`
	Header    string `json:"request_id_header"`
}

func validUpstreamRequestID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 256 || strings.ContainsFunc(value, unicode.IsControl) {
		return ""
	}
	return value
}

// RecordUpstreamRequestID keeps a bounded history of received HTTP responses,
// not a "final response ID" that could become stale on a later transport error.
func RecordUpstreamRequestID(c *gin.Context, header http.Header) {
	if c == nil {
		return
	}
	id, source := getUpstreamRequestID(header)
	if id == "" {
		return
	}
	value, _ := c.Get(upstreamRequestIDsKey)
	previous, _ := value.([]upstreamRequestID)
	if len(previous) >= 5 {
		previous = previous[len(previous)-4:]
	}
	entries := append([]upstreamRequestID(nil), previous...)
	entries = append(entries, upstreamRequestID{ChannelID: c.GetInt("channel_id"), ID: id, Header: source})
	c.Set(upstreamRequestIDsKey, entries)
}

func AppendUpstreamRequestIDs(c *gin.Context, adminInfo map[string]interface{}) {
	if value, exists := c.Get(upstreamRequestIDsKey); exists {
		adminInfo["upstream_request_ids"] = value
	}
}
