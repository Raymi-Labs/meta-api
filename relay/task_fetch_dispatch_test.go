package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskFetchRejectsUnsupportedModeWithoutPanic(t *testing.T) {
	for _, mode := range []int{relayconstant.RelayModeUnknown, relayconstant.RelayModeVideoSubmit, -1} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_owned", nil)
		var taskErr *dto.TaskError
		require.NotPanics(t, func() { taskErr = RelayTaskFetch(c, mode) })
		require.NotNil(t, taskErr)
		require.Equal(t, "invalid_relay_mode", taskErr.Code)
		require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		require.False(t, c.Writer.Written())
		require.Empty(t, recorder.Body.String())
	}
}
