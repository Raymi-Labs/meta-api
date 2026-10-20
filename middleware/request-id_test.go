package middleware_test

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestRequestIDProtectsHTTPCommitPaths(t *testing.T) {
	for _, mode := range []string{"copy", "write", "string", "header_now", "empty", "flush", "recovery"} {
		t.Run(mode, func(t *testing.T) {
			r := gin.New()
			r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) { c.AbortWithStatus(500) }), middleware.RequestId())
			r.GET("/", func(c *gin.Context) {
				id := c.GetString(common.RequestIdKey)
				require.NotEmpty(t, id)
				require.Equal(t, id, c.Request.Context().Value(common.RequestIdKey))
				c.Header("X-Test-Local-ID", id)
				c.Header("X-Test-Preserved", "value")
				// Include Add and non-canonical map keys used by proxy adapters.
				c.Writer.Header().Add(common.RequestIdKey, "upstream-add")
				c.Writer.Header()["x-oneapi-request-id"] = []string{"upstream-lower"}
				c.Writer.Header()["X-ONEAPI-REQUEST-ID"] = []string{"upstream-upper"}
				switch mode {
				case "copy":
					service.IOCopyBytesGracefully(c, &http.Response{StatusCode: 201, Header: http.Header{common.RequestIdKey: {"upstream-copy"}}}, []byte("payload"))
				case "write":
					c.Status(201)
					_, err := c.Writer.Write([]byte("payload"))
					require.NoError(t, err)
				case "string":
					c.Status(201)
					_, err := c.Writer.WriteString("payload")
					require.NoError(t, err)
				case "header_now":
					c.Status(201)
					c.Writer.Header().Set(common.RequestIdKey, "upstream-after-status")
					c.Writer.WriteHeaderNow()
				case "empty":
					c.Status(204)
					c.Writer.Header().Set(common.RequestIdKey, "upstream-empty")
				case "flush":
					require.NoError(t, http.NewResponseController(c.Writer).Flush())
				case "recovery":
					panic("fixture")
				}
			})
			server := httptest.NewServer(r)
			defer server.Close()
			resp, err := server.Client().Get(server.URL)
			require.NoError(t, err)
			defer resp.Body.Close()
			data, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			id := resp.Header.Get("X-Test-Local-ID")
			require.Equal(t, []string{id}, resp.Header.Values(common.RequestIdKey))
			require.Equal(t, "value", resp.Header.Get("X-Test-Preserved"))
			switch mode {
			case "copy", "write", "string":
				require.Equal(t, 201, resp.StatusCode)
				require.Equal(t, "payload", string(data))
			case "header_now":
				require.Equal(t, 201, resp.StatusCode)
			case "empty":
				require.Equal(t, 204, resp.StatusCode)
			case "recovery":
				require.Equal(t, 500, resp.StatusCode)
			default:
				require.Equal(t, 200, resp.StatusCode)
			}
		})
	}
}

func TestRequestIDPreservesStreamingAndResponseController(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestId())
	release := make(chan struct{})
	r.GET("/", func(c *gin.Context) {
		c.Header("X-Test-Local-ID", c.GetString(common.RequestIdKey))
		c.Header(common.RequestIdKey, "upstream-stream")
		c.Header("Content-Type", "text/event-stream")
		controller := http.NewResponseController(c.Writer)
		require.NoError(t, controller.SetWriteDeadline(time.Now().Add(5*time.Second)))
		_, _ = c.Writer.WriteString("data: first\n\n")
		require.NoError(t, controller.Flush())
		<-release
		_, _ = c.Writer.WriteString("data: second\n\n")
	})
	server := httptest.NewServer(r)
	defer server.Close()
	// Release even on failure so the fixture server cannot hang on Close.
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	client := server.Client()
	client.Timeout = 5 * time.Second
	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, []string{resp.Header.Get("X-Test-Local-ID")}, resp.Header.Values(common.RequestIdKey))
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "data: first\n", line, "first frame must arrive before handler completes")
	close(release)
	rest, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "\ndata: second\n\n", string(rest))
}

func TestRequestIDPreservesWebSocketUpgrade(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestId())
	r.GET("/", func(c *gin.Context) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		require.NoError(t, err)
		defer conn.Close()
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("ok")))
	})
	server := httptest.NewServer(r)
	defer server.Close()
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	require.Equal(t, 101, resp.StatusCode)
	kind, data, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, kind)
	require.Equal(t, "ok", string(data))
}
