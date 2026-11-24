package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type chatBudgetDiagnosticKey struct{}

var budgetModelID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,191}$`)

type budgetBody struct {
	fields []string
	model  string
}

func inspectBudgetBody(body []byte) budgetBody {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return budgetBody{}
	}
	result := budgetBody{fields: []string{}}
	for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		if gjson.GetBytes(body, key).Exists() {
			result.fields = append(result.fields, key)
		}
	}
	model := gjson.GetBytes(body, "model")
	if model.Type == gjson.String && budgetModelID.MatchString(model.Str) &&
		!logSecretPattern.MatchString(model.Str) && !strings.Contains(model.Str, "://") {
		result.model = strings.Clone(model.Str)
	}
	return result
}

func inboundBudgetBody(c *gin.Context, storage common.BodyStorage) budgetBody {
	const cacheKey = "chat_inbound_budget_fields"
	if cached, exists := c.Get(cacheKey); exists {
		return cached.(budgetBody)
	}
	if storage == nil {
		return budgetBody{}
	}
	body, err := storage.Bytes()
	if err != nil {
		return budgetBody{}
	}
	fields := inspectBudgetBody(body)
	c.Set(cacheKey, fields)
	return fields
}

// RecordChatBudgetRejection binds the evidence to the rejected HTTP response. Retries
// and status mapping can then reuse the existing per-attempt and final error snapshots.
func RecordChatBudgetRejection(c *gin.Context, sent *http.Request, resp *http.Response, reader io.Reader, wire []byte) {
	if c.Request == nil || !strings.HasSuffix(c.Request.URL.Path, "/chat/completions") {
		return
	}
	actual := resp.Request
	if actual == nil {
		actual = sent
	}
	if actual.Response != nil || actual.Method != http.MethodPost || actual.URL == nil || actual.URL.String() != sent.URL.String() {
		return
	}
	protocol := ""
	switch {
	case strings.HasSuffix(actual.URL.Path, "/chat/completions"):
		protocol = "chat/completions"
	case strings.HasSuffix(actual.URL.Path, "/responses"):
		protocol = "responses"
	default:
		return
	}
	source, _ := c.Get(common.KeyBodyStorage)
	storage, _ := source.(common.BodyStorage)
	in := inboundBudgetBody(c, storage)
	pass := common.IsOriginalBodyReader(reader, storage)
	out := in
	if !pass {
		out = inspectBudgetBody(wire)
	}
	d := &types.ChatOutputBudget{
		Protocol: protocol, Model: out.model, Passthrough: pass,
		InboundFields: in.fields, OutboundFields: out.fields,
	}
	resp.Request = actual.WithContext(context.WithValue(actual.Context(), chatBudgetDiagnosticKey{}, d))
}

func isChatBudgetRejection(err *types.NewAPIError) bool {
	if err.Upstream == nil || err.Upstream.OutputBudget == nil || err.Upstream.OutputBudget.Protocol != "chat/completions" ||
		err.UpstreamStatusCode != http.StatusBadRequest || err.GetErrorCode() != "unsupported_parameter" {
		return false
	}
	up := err.ToOpenAIError()
	if up.Param != "" && up.Param != "max_tokens" {
		return false
	}
	message := strings.TrimSpace(strings.ToLower(up.Message))
	return strings.HasPrefix(message, "unsupported parameter: 'max_tokens' is not supported with this model. use 'max_completion_tokens' instead.")
}
