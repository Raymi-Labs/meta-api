package middleware

import (
	"math"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// EnterpriseTokenScope 是企业令牌在一次请求内的付款方上下文。
type EnterpriseTokenScope struct {
	Enterprise *model.Enterprise
	Member     *model.EnterpriseMember
}

// ResolveEnterpriseToken 个人令牌返回 nil；企业令牌校验成员关系与企业状态后返回付款方上下文。
func ResolveEnterpriseToken(token *model.Token) (*EnterpriseTokenScope, error) {
	if token == nil || token.EnterpriseId <= 0 {
		return nil, nil
	}
	ent, member, err := model.ValidateEnterpriseToken(token)
	if err != nil {
		return nil, err
	}
	return &EnterpriseTokenScope{Enterprise: ent, Member: member}, nil
}

// WriteContext 写入企业付款身份和预算，保留账号原有的授权分组。
// 必须在个人用户上下文写入之后、令牌上下文写入之前调用。
func (s *EnterpriseTokenScope) WriteContext(c *gin.Context) {
	available := s.Member.Available
	if available > math.MaxInt32 {
		available = math.MaxInt32
	}
	common.SetContextKey(c, constant.ContextKeyEnterpriseBillingGroup, s.Enterprise.BillingGroup)
	// 请求内展示与判断用的"可用额度"是成员预算，不是个人钱包。
	common.SetContextKey(c, constant.ContextKeyUserQuota, int(available))
	common.SetContextKey(c, constant.ContextKeyEnterpriseId, s.Enterprise.Id)
	common.SetContextKey(c, constant.ContextKeyEnterpriseMemberId, s.Member.Id)
	common.SetContextKey(c, constant.ContextKeyEnterpriseRole, s.Member.Role)
}

// ApplyModelPolicy 把企业与成员两级模型限制同令牌自身的模型限制取交集。
// 必须在令牌上下文写入之后调用。空集合表示全部禁止。
func (s *EnterpriseTokenScope) ApplyModelPolicy(c *gin.Context) {
	allowed, restricted := service.EnterpriseModelPolicy(s.Enterprise, s.Member)
	if !restricted {
		return
	}
	if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		current, _ := common.GetContextKeyType[map[string]bool](c, constant.ContextKeyTokenModelLimit)
		narrowed := make(map[string]bool, len(current))
		for name, enabled := range current {
			if enabled && (allowed[name] || allowed[ratio_setting.FormatMatchingModelName(name)]) {
				narrowed[name] = true
			}
		}
		allowed = narrowed
	}
	common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimit, allowed)
}

// abortEnterpriseToken 企业令牌被拒绝时的统一出口：业务原因原样返回，其余只给通用提示。
func abortEnterpriseToken(c *gin.Context, err error) {
	if model.IsEnterpriseUserError(err) {
		abortWithOpenAiMessage(c, http.StatusForbidden, err.Error(), types.ErrorCodeAccessDenied)
		return
	}
	logger.LogError(c, "enterprise token check failed: "+err.Error())
	abortWithOpenAiMessage(c, http.StatusInternalServerError, "企业令牌校验失败，请稍后重试")
}
