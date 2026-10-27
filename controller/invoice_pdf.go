package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var renderInvoicePDF = func(ctx context.Context, html string) ([]byte, error) { return service.RenderInvoicePDF(ctx, html) }

// RenderInvoicePDF is mounted only under AdminAuth. A role check also prevents
// accidental reuse as a public renderer when routes are changed in the future.
func RenderInvoicePDF(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "无权进行此操作，权限不足"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiError(c, service.ErrInvoicePDFInput)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxInvoiceHTMLSize)
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.ApiError(c, service.ErrInvoicePDFInput)
		return
	}
	var payload struct {
		HTML string `json:"html"`
		Kind string `json:"kind"`
	}
	if err := common.Unmarshal(data, &payload); err != nil || (payload.Kind != "detail_bill" && payload.Kind != "service_confirmation") || len(payload.HTML) == 0 {
		common.ApiError(c, service.ErrInvoicePDFInput)
		return
	}
	request, err := model.GetInvoiceRequestDetail(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if request.Status != model.InvoiceStatusPending && request.Status != model.InvoiceStatusInvoiced {
		common.ApiError(c, service.ErrInvoicePDFInput)
		return
	}
	pdf, err := renderInvoicePDF(c.Request.Context(), payload.HTML)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(pdf) > service.MaxGeneratedInvoicePDFSize || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		common.ApiError(c, service.ErrInvoicePDFRender)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="invoice-%d-%s.pdf"`, id, payload.Kind))
	c.Data(http.StatusOK, "application/pdf", pdf)
}
