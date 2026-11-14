package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func PreviewChannelParamOverride(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	var req struct {
		Body     string                                  `json:"body"`
		Override string                                  `json:"override"`
		Context  relaycommon.ParamOverridePreviewContext `json:"context"`
	}
	if err := common.DecodeJsonUseNumberStrict(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid preview request (maximum 256 KiB)"})
		return
	}
	var override map[string]interface{}
	if common.GetJsonType([]byte(req.Override)) != "object" || common.Unmarshal([]byte(req.Override), &override) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "override must be a JSON object"})
		return
	}
	result, err := relaycommon.PreviewParamOverride(req.Body, override, req.Context)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
