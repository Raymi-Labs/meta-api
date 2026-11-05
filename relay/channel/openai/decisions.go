package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func decisionsRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.ChannelType != constant.ChannelTypeOpenRouter {
		return "", fmt.Errorf("decisions requires an OpenRouter channel")
	}
	// OpenRouter's configured base is normally https://openrouter.ai/api.
	base := strings.TrimRight(info.ChannelBaseUrl, "/")
	base = strings.TrimSuffix(base, "/v1")
	return base + "/alpha/decisions", nil
}

func validateDecisionsBody(info *relaycommon.RelayInfo, body io.Reader) (io.Reader, error) {
	if info.ChannelType != constant.ChannelTypeOpenRouter {
		return nil, fmt.Errorf("decisions requires an OpenRouter channel")
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	var request *dto.DecisionsRequest
	if err = common.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	if err = request.Validate(); err != nil {
		return nil, err
	}
	if request.Model != info.UpstreamModelName {
		return nil, fmt.Errorf("decisions must use the mapped upstream model")
	}
	return bytes.NewReader(raw), nil
}

func decisionsResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	bad := func(err error) (any, *types.NewAPIError) {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	if resp == nil || resp.Body == nil {
		return bad(fmt.Errorf("missing decisions response"))
	}
	defer resp.Body.Close()
	const limit = 32 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return bad(err)
	}
	if len(raw) > limit {
		return bad(fmt.Errorf("decisions response exceeds limit"))
	}
	var response struct {
		Answers map[string]json.RawMessage `json:"answers"`
		Model   string                     `json:"model"`
		Error   json.RawMessage            `json:"error"`
		Usage   *struct {
			Input  *int `json:"input_tokens"`
			Output *int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err = common.Unmarshal(raw, &response); err != nil {
		return bad(fmt.Errorf("invalid decisions response: %w", err))
	}
	if len(response.Answers) == 0 || response.Model == "" || (len(response.Error) > 0 && string(response.Error) != "null") {
		return bad(fmt.Errorf("missing decisions answers or upstream error"))
	}
	for _, rawAnswer := range response.Answers {
		var answer map[string]json.RawMessage
		if common.Unmarshal(rawAnswer, &answer) != nil {
			return bad(fmt.Errorf("invalid decisions answer"))
		}
		var kind string
		if common.Unmarshal(answer["type"], &kind) != nil || len(answer[kind]) == 0 || string(answer[kind]) == "null" {
			return bad(fmt.Errorf("missing typed decisions answer"))
		}
	}
	u := response.Usage
	if u == nil || u.Input == nil || u.Output == nil || *u.Input < 0 || *u.Output < 0 || *u.Input > 1<<30 || *u.Output > 1<<30 {
		return bad(fmt.Errorf("missing or invalid decisions token usage"))
	}
	usage := &dto.Usage{PromptTokens: *u.Input, CompletionTokens: *u.Output, TotalTokens: *u.Input + *u.Output}
	info.SetFirstResponseTime()
	c.Data(http.StatusOK, "application/json", raw)
	return usage, nil
}
