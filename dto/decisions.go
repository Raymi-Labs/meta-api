package dto

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// Decisions keeps the provider's structured state and criteria intact. It is
// deliberately not converted to messages or a generated text response.
type DecisionsRequest struct {
	Model     string                     `json:"model"`
	State     json.RawMessage            `json:"state"`
	Questions map[string]json.RawMessage `json:"questions"`
	Assets    json.RawMessage            `json:"assets,omitempty"`
	Provider  json.RawMessage            `json:"provider,omitempty"`
	SessionID string                     `json:"session_id,omitempty"`
	Trace     json.RawMessage            `json:"trace,omitempty"`
	User      string                     `json:"user,omitempty"`
	Stream    bool                       `json:"stream,omitempty"`
}

func (r *DecisionsRequest) IsStream(*gin.Context) bool { return false }
func (r *DecisionsRequest) SetModelName(name string)   { r.Model = name }
func (r *DecisionsRequest) GetTokenCountMeta() *types.TokenCountMeta {
	questions, _ := common.Marshal(r.Questions)
	return &types.TokenCountMeta{CombineText: string(r.State) + "\n" + string(questions)}
}

func (r *DecisionsRequest) Validate() error {
	if r == nil || strings.TrimSpace(r.Model) == "" {
		return fmt.Errorf("model is required")
	}
	if r.Stream {
		return fmt.Errorf("decisions does not support streaming")
	}
	state := strings.TrimSpace(string(r.State))
	if state == "" || (state[0] != '"' && state[0] != '{' && state[0] != '[') {
		return fmt.Errorf("state must be a string, object or array")
	}
	if len(r.Questions) == 0 {
		return fmt.Errorf("questions is required")
	}
	for name, raw := range r.Questions {
		var q struct {
			Type         string `json:"type"`
			Instructions string `json:"instructions"`
		}
		if name == "" || common.Unmarshal(raw, &q) != nil || strings.TrimSpace(q.Instructions) == "" {
			return fmt.Errorf("question %q requires instructions", name)
		}
		switch q.Type {
		case "noul", "choice", "score", "box":
		default:
			return fmt.Errorf("question %q has unsupported type", name)
		}
	}
	return nil
}
