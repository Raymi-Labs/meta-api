package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func invoicePDFContext(id, role int, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(id)}}
	ctx.Set("role", role)
	return ctx, recorder
}

func TestInvoicePDFControllerRendersWithoutApprovingOrSending(t *testing.T) {
	request := setupInvoiceAttachmentTest(t)
	original := renderInvoicePDF
	t.Cleanup(func() { renderInvoicePDF = original })
	calls := 0
	renderInvoicePDF = func(ctx context.Context, html string) ([]byte, error) {
		calls++
		require.Equal(t, "<h1>invoice</h1>", html)
		return []byte("%PDF-1.4 generated invoice"), nil
	}
	for _, status := range []string{model.InvoiceStatusPending, model.InvoiceStatusInvoiced} {
		require.NoError(t, model.DB.Model(request).Update("status", status).Error)
		for _, kind := range []string{"detail_bill", "service_confirmation"} {
			ctx, recorder := invoicePDFContext(request.Id, common.RoleAdminUser, fmt.Sprintf(`{"kind":%q,"html":"<h1>invoice</h1>"}`, kind))
			RenderInvoicePDF(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "application/pdf", recorder.Header().Get("Content-Type"))
			require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			require.Equal(t, "%PDF-1.4 generated invoice", recorder.Body.String())
			stored, err := model.GetInvoiceRequestDetail(request.Id)
			require.NoError(t, err)
			require.Equal(t, status, stored.Status)
			require.Zero(t, stored.ReviewedAt)
			require.Empty(t, stored.InvoiceSendStatus)
			require.Empty(t, stored.InvoiceEmailAttachments)
		}
	}
	require.Equal(t, 4, calls)
}

func TestInvoicePDFControllerRejectsUnauthorizedInvalidAndRejected(t *testing.T) {
	request := setupInvoiceAttachmentTest(t)
	original := renderInvoicePDF
	t.Cleanup(func() { renderInvoicePDF = original })
	renderInvoicePDF = func(ctx context.Context, html string) ([]byte, error) {
		t.Fatal("must not render invalid input")
		return nil, nil
	}
	valid := `{"kind":"detail_bill","html":"<h1>invoice</h1>"}`
	for _, role := range []int{0, common.RoleCommonUser} {
		ctx, recorder := invoicePDFContext(request.Id, role, valid)
		RenderInvoicePDF(ctx)
		require.Equal(t, http.StatusForbidden, recorder.Code)
	}
	for _, body := range []string{`{`, `{}`, `{"kind":"url","html":"https://example.test"}`, `{"kind":"detail_bill","html":""}`, strings.Repeat("x", service.MaxInvoiceHTMLSize+1)} {
		ctx, recorder := invoicePDFContext(request.Id, common.RoleAdminUser, body)
		RenderInvoicePDF(ctx)
		require.Contains(t, recorder.Body.String(), service.ErrInvoicePDFInput.Error())
	}
	for _, id := range []int{0, request.Id + 1} {
		ctx, recorder := invoicePDFContext(id, common.RoleAdminUser, valid)
		RenderInvoicePDF(ctx)
		require.NotContains(t, recorder.Header().Get("Content-Type"), "application/pdf")
	}
	require.NoError(t, model.DB.Model(request).Update("status", model.InvoiceStatusRejected).Error)
	ctx, recorder := invoicePDFContext(request.Id, common.RoleAdminUser, valid)
	RenderInvoicePDF(ctx)
	require.Contains(t, recorder.Body.String(), service.ErrInvoicePDFInput.Error())
}

func TestInvoicePDFControllerRenderFailureLeavesRequestUnchanged(t *testing.T) {
	request := setupInvoiceAttachmentTest(t)
	original := renderInvoicePDF
	t.Cleanup(func() { renderInvoicePDF = original })
	for _, result := range []struct {
		data []byte
		err  error
	}{{nil, service.ErrInvoicePDFRender}, {nil, service.ErrInvoicePDFBusy}, {[]byte("invalid"), nil}, {append([]byte("%PDF-"), make([]byte, service.MaxGeneratedInvoicePDFSize)...), nil}} {
		renderInvoicePDF = func(ctx context.Context, html string) ([]byte, error) { return result.data, result.err }
		ctx, recorder := invoicePDFContext(request.Id, common.RoleAdminUser, `{"kind":"detail_bill","html":"<p>invoice</p>"}`)
		RenderInvoicePDF(ctx)
		require.NotContains(t, recorder.Header().Get("Content-Type"), "application/pdf")
		stored, err := model.GetInvoiceRequestDetail(request.Id)
		require.NoError(t, err)
		require.Equal(t, model.InvoiceStatusPending, stored.Status)
		require.Empty(t, stored.InvoiceSendStatus)
		require.Empty(t, stored.InvoiceEmailAttachments)
	}
}
