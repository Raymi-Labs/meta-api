package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestOutcomeDoesNotTreatHTTP200AsStreamSuccess(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	require.True(t, RequestSucceeded(c))
	c.Set(RequestOutcomeKey, false)
	require.False(t, RequestSucceeded(c))
	c.Set(RequestOutcomeKey, true)
	require.True(t, RequestSucceeded(c))
	MarkRequestFailure(c, types.NewError(errors.New("SSE failed"), types.ErrorCodeBadResponse))
	require.False(t, RequestSucceeded(c))
}

func TestRequestOutcomeConfirmedDeliverySurvivesOnlyLaterCancellation(t *testing.T) {
	for _, scenario := range []string{"no-output", "unconfirmed-success", "delivered", "write-failure", "http-failure"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(ctx)
			if scenario != "no-output" {
				c.Set(RequestOutcomeKey, true)
			}
			if scenario == "delivered" || scenario == "write-failure" || scenario == "http-failure" {
				c.Set(RequestDeliveredCompletionKey, true)
			}
			if scenario == "write-failure" {
				MarkRequestFailure(c, types.NewError(errors.New("write failed"), types.ErrorCodeBadResponseBody))
			}
			if scenario == "http-failure" {
				c.Status(502)
			}
			cancel()
			require.Equal(t, scenario == "delivered", RequestSucceeded(c))
		})
	}
}
