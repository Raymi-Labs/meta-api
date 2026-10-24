package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskLocalErrorsNeverRetry(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		require.False(t, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: status, LocalError: true}, 3), "status %d", status)
	}
	require.True(t, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: http.StatusTooManyRequests}, 3))
}
