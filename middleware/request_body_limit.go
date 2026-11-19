package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func AnonymousRequestBodyLimit() gin.HandlerFunc {
	return requestBodyLimit(common.GetAnonymousRequestBodyLimitBytes)
}

// TicketRequestBodyLimit bounds the entire JSON envelope, including unknown
// fields and trailing data. Multipart uploads have their own 5 MiB limit.
func TicketRequestBodyLimit() gin.HandlerFunc {
	return requestBodyLimit(func() int64 { return 64 * 1024 })
}

func requestBodyLimit(limit func() int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		maxBytes := limit()
		if maxBytes <= 0 || c.Request.Body == nil {
			c.Next()
			return
		}

		originalBody := c.Request.Body
		if c.Request.ContentLength > maxBytes {
			_ = originalBody.Close()
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		limitedBody, err := readAnonymousRequestBody(originalBody, maxBytes)
		_ = originalBody.Close()
		if err != nil {
			if common.IsRequestBodyTooLargeError(err) {
				c.AbortWithStatus(http.StatusRequestEntityTooLarge)
				return
			}
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}

		c.Request.Body = io.NopCloser(bytes.NewReader(limitedBody))
		c.Request.ContentLength = int64(len(limitedBody))
		c.Next()
	}
}

func readAnonymousRequestBody(body io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, common.ErrRequestBodyTooLarge
	}
	return data, nil
}
