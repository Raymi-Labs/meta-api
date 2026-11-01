package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEditorAutomaticallyLoadsOfficialDefaultsAndCachesAttempts(t *testing.T) {
	db := catalogControllerDB(t)
	t.Setenv("MODEL_METADATA_SYNC_ENABLED", "true")
	t.Setenv("MODEL_METADATA_ANTHROPIC_API_KEY", "")
	t.Setenv("MODEL_METADATA_GEMINI_API_KEY", "")
	t.Setenv("SYNC_UPSTREAM_BASE", "https://community.example")
	m := model.Model{ModelName: "gpt-editor", Status: 1, SyncOfficial: 1}
	require.NoError(t, m.Insert())
	modelMetadataAttempts.Lock()
	delete(modelMetadataAttempts.until, m.ModelName)
	modelMetadataAttempts.Unlock()
	original := getHTTPClient()
	client := *original
	httpClient = &client
	t.Cleanup(func() { httpClient = original })
	var calls atomic.Int64
	client.Transport = attributeTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		body := strings.ReplaceAll(officialAttributeFixture, "gpt-example", m.ModelName)
		body = strings.Replace(body, "<div>1,050,000", "<div>gpt-editor</div><p>Official model introduction for all users</p><div>1,050,000", 1)
		if r.URL.Host == "community.example" {
			body = `{"openai":{"id":"openai","models":{"gpt-editor":{"id":"gpt-editor","description":"Community description","family":"gpt","type":"text","limit":{"context":1000}}}}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})
	get := func() model.Model {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(m.Id)}}
		c.Request = httptest.NewRequest(http.MethodGet, "/api/models/"+strconv.Itoa(m.Id)+"?refresh_metadata=true", nil)
		GetModelMeta(c)
		var response struct {
			Success bool        `json:"success"`
			Data    model.Model `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		require.True(t, response.Success, w.Body.String())
		return response.Data
	}
	saved := get()
	require.Equal(t, "Official model introduction for all users", saved.Description)
	require.EqualValues(t, 1050000, saved.ContextLength)
	require.Positive(t, saved.VendorID)
	require.Equal(t, "text", saved.Tags)
	require.Equal(t, "official", saved.AttributeSources["description"].Kind)
	first := calls.Load()
	get()
	require.Equal(t, first, calls.Load(), "opening the editor again must not refetch every model")
	require.NoError(t, db.Model(&model.Model{}).Where("id = ?", m.Id).Update("description", "Manual introduction").Error)
	saved = get()
	require.Equal(t, "Manual introduction", saved.Description)
	// An omitted automatic field survives a full editor save of other settings.
	request := modelMetaRequest{Model: model.Model{Id: m.Id, ModelName: m.ModelName, Status: 1, SyncOfficial: 1, Icon: "Custom"}}
	updated, err := request.withDisplayMetadata(&saved)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.Model{}).Where("id = ?", m.Id).Update("context_length", 2000000).Error)
	require.NoError(t, updated.UpdateOmittingMetadata(request.omittedMetadataFields()))
	saved = get()
	require.EqualValues(t, 2000000, saved.ContextLength)
	require.Equal(t, "Manual introduction", saved.Description)
}
