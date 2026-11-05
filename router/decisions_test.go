package router

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDecisionsRequiresToken(t *testing.T) {
	r := gin.New()
	SetRelayRouter(r)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/v1/decisions", strings.NewReader(`{"model":"typesafe/jev-1.14","state":"hi","questions":{}}`)))
	require.Equal(t, 401, w.Code)
}
