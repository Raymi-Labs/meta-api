package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func RequestId() func(c *gin.Context) {
	return func(c *gin.Context) {
		id := common.GetTimeString() + common.GetRandomString(8)
		c.Set(common.RequestIdKey, id)
		ctx := context.WithValue(c.Request.Context(), common.RequestIdKey, id)
		c.Request = c.Request.WithContext(ctx)
		c.Header(common.RequestIdKey, id)
		writer := &requestIDWriter{ResponseWriter: c.Writer, requestID: id}
		c.Writer = writer
		// Gin commits empty responses through its original writer after Next.
		defer writer.preserveRequestID()
		c.Next()
	}
}

// Upstream header copies must not replace the ID used by local logs. Protect
// the actual commit points, since restoring headers after Next is too late.
type requestIDWriter struct {
	gin.ResponseWriter
	requestID string
}

func (w *requestIDWriter) preserveRequestID() {
	if w.Written() {
		return
	}
	header := w.Header()
	for key := range header {
		if strings.EqualFold(key, common.RequestIdKey) {
			delete(header, key)
		}
	}
	header.Set(common.RequestIdKey, w.requestID)
}

func (w *requestIDWriter) WriteHeader(code int) {
	w.preserveRequestID()
	w.ResponseWriter.WriteHeader(code)
}

func (w *requestIDWriter) WriteHeaderNow() {
	w.preserveRequestID()
	w.ResponseWriter.WriteHeaderNow()
}

func (w *requestIDWriter) Write(data []byte) (int, error) {
	w.preserveRequestID()
	return w.ResponseWriter.Write(data)
}

func (w *requestIDWriter) WriteString(data string) (int, error) {
	w.preserveRequestID()
	return w.ResponseWriter.WriteString(data)
}

func (w *requestIDWriter) Flush() {
	w.preserveRequestID()
	w.ResponseWriter.Flush()
}

func (w *requestIDWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
