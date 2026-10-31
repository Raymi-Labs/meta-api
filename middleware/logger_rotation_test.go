package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccessAndRecoveryFollowCurrentLogWriter(t *testing.T) {
	common.LogWriterMu.RLock()
	oldOut, oldErr := gin.DefaultWriter, gin.DefaultErrorWriter
	common.LogWriterMu.RUnlock()
	dir := t.TempDir()
	newSink := func(name string) *os.File {
		file, err := os.Create(filepath.Join(dir, name))
		require.NoError(t, err)
		t.Cleanup(func() { _ = file.Close() })
		return file
	}
	firstOut, firstErr, nextOut, nextErr := newSink("first-out"), newSink("first-err"), newSink("next-out"), newSink("next-err")
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		defer common.LogWriterMu.Unlock()
		gin.DefaultWriter, gin.DefaultErrorWriter = oldOut, oldErr
	})
	common.LogWriterMu.Lock()
	gin.DefaultWriter, gin.DefaultErrorWriter = firstOut, firstErr
	common.LogWriterMu.Unlock()
	router := gin.New()
	router.Use(PanicRecover())
	SetUpLogger(router)
	router.GET("/before", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/after", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/panic", func(c *gin.Context) { panic("rotation-fixture") })
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	require.Equal(t, http.StatusNoContent, request("/before").Code)
	// A running router must not retain the previous file after rotation.
	common.LogWriterMu.Lock()
	gin.DefaultWriter, gin.DefaultErrorWriter = nextOut, nextErr
	closeOut, closeErr := firstOut.Close(), firstErr.Close()
	common.LogWriterMu.Unlock()
	require.NoError(t, closeOut)
	require.NoError(t, closeErr)
	require.Equal(t, http.StatusNoContent, request("/after").Code)
	panicked := request("/panic")
	require.Equal(t, http.StatusInternalServerError, panicked.Code)
	require.Contains(t, panicked.Body.String(), `"type":"new_api_panic"`)
	require.Contains(t, panicked.Body.String(), "Panic detected, error: rotation-fixture. Please submit a issue here: https://github.com/Calcium-Ion/new-api")
	read := func(file *os.File) string {
		data, err := os.ReadFile(file.Name())
		require.NoError(t, err)
		return string(data)
	}
	require.Contains(t, read(firstOut), "/before")
	require.NotContains(t, read(firstOut), "GET /after")
	require.Contains(t, read(nextOut), "GET /after")
	require.NotContains(t, read(firstErr), "rotation-fixture")
	require.Contains(t, read(nextErr), "[Recovery]")
	require.Contains(t, read(nextErr), "rotation-fixture")
}
