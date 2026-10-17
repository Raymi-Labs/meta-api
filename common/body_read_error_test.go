package common

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type failingRequestBody struct {
	cause         error
	reads, closes int
}

func (b *failingRequestBody) Read([]byte) (int, error) { b.reads++; return 0, b.cause }
func (b *failingRequestBody) Close() error             { b.closes++; return nil }

func TestRequestBodyKeepsFirstFailure(t *testing.T) {
	for _, cause := range []error{io.ErrUnexpectedEOF, errors.New("client upload interrupted"), &http.MaxBytesError{Limit: 10}} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		body := &failingRequestBody{cause: cause}
		c.Request.Body = body
		_, first := GetRequestBody(c)
		require.Error(t, first)
		reads := body.reads
		for i := 0; i < 3; i++ {
			_, err := GetRequestBody(c)
			require.Same(t, first, err)
		}
		require.Equal(t, reads, body.reads)
		require.Equal(t, 1, body.closes)
		if IsRequestBodyTooLargeError(cause) {
			require.True(t, IsRequestBodyTooLargeError(first))
		} else {
			require.ErrorIs(t, first, cause)
		}
	}
}

func TestRequestBodySuccessfulReadsRemainReusable(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"test"}`))
	t.Cleanup(func() { CleanupBodyStorage(c) })
	for i := 0; i < 3; i++ {
		body, err := GetBodyStorage(c)
		require.NoError(t, err)
		got, err := io.ReadAll(body)
		require.NoError(t, err)
		require.Equal(t, `{"model":"test"}`, string(got))
	}
}
