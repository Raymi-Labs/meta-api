package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func CreateEnterpriseApplication(c *gin.Context) {
	var req struct {
		Kind        string `json:"kind"`
		Name        string `json:"name"`
		ContactType string `json:"contact_type"`
		Contact     string `json:"contact"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	app, err := model.CreateEnterpriseApplication(c.GetInt("id"), req.Kind, req.Name, req.ContactType, req.Contact)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, app)
}

func ListEnterpriseSelfApplications(c *gin.Context)  { listEnterpriseApplications(c, c.GetInt("id")) }
func AdminListEnterpriseApplications(c *gin.Context) { listEnterpriseApplications(c, 0) }
func listEnterpriseApplications(c *gin.Context, userID int) {
	page := common.GetPageQuery(c)
	items, total, err := model.ListEnterpriseApplications(userID, c.Query("status"), c.Query("kind"), page.GetStartIdx(), page.GetPageSize())
	enterprisePage(c, page, items, total, err)
}

func AdminReviewEnterpriseApplication(c *gin.Context) {
	id, ok := enterprisePathInt(c, "application_id")
	if !ok {
		return
	}
	var req struct {
		Action       string `json:"action"`
		BillingGroup string `json:"billing_group"`
		Reason       string `json:"reason"`
	}
	if !enterpriseBody(c, &req) {
		return
	}
	app, err := model.ReviewEnterpriseApplication(c.GetInt("id"), id, req.Action, req.BillingGroup, req.Reason)
	if err != nil {
		enterpriseApiError(c, err)
		return
	}
	common.ApiSuccess(c, app)
}
