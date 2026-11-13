package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type playgroundPayerRequest struct {
	Payer string `json:"payer"`
	Group string `json:"group"`
}

// readPlaygroundPayer preserves the full request for Distribute and Relay. Empty
// bodies are accepted by the image-session endpoint; an absent payer is personal.
func readPlaygroundPayer(c *gin.Context) (playgroundPayerRequest, error) {
	var request playgroundPayerRequest
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return request, err
	}
	body, err := storage.Bytes()
	if err != nil {
		return request, err
	}
	if len(bytes.TrimSpace(body)) > 0 {
		if err := common.Unmarshal(body, &request); err != nil {
			return request, err
		}
	}
	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		return request, err
	}
	c.Request.Body = io.NopCloser(storage)
	request.Payer = strings.ToLower(strings.TrimSpace(request.Payer))
	request.Group = strings.TrimSpace(request.Group)
	switch request.Payer {
	case "", model.TokenPayerPersonal:
		request.Payer = model.TokenPayerPersonal
	case model.TokenPayerEnterprise:
	default:
		return request, fmt.Errorf("参数 payer 仅支持 personal 或 enterprise")
	}
	return request, nil
}

func resolvePlaygroundPayer(userID int, payer string) (*EnterpriseTokenScope, error) {
	if userID <= 0 {
		return nil, model.ErrEnterpriseNotFound
	}
	if payer == model.TokenPayerPersonal {
		return nil, nil
	}
	ent, member, err := model.GetEnterprisePayer(userID)
	if err != nil {
		return nil, err
	}
	if member.UserId != userID || member.EnterpriseId != ent.Id || member.Status != model.EnterpriseMemberStatusActive {
		return nil, model.ErrEnterpriseMemberInactive
	}
	return &EnterpriseTokenScope{Enterprise: ent, Member: member}, nil
}

// ResolvePlaygroundPayer is shared by the text and image experience contracts.
// Enterprise and membership IDs always come from the logged-in account, never JSON.
func ResolvePlaygroundPayer(c *gin.Context) (*EnterpriseTokenScope, error) {
	request, err := readPlaygroundPayer(c)
	if err != nil {
		return nil, err
	}
	return resolvePlaygroundPayer(c.GetInt("id"), request.Payer)
}

// PlaygroundPayer must run after UserAuth and before Distribute. It establishes
// authorization, pricing identity and model restrictions before any channel lookup.
func PlaygroundPayer() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer common.CleanupBodyStorage(c)
		if c.GetBool("use_access_token") {
			abortWithOpenAiMessage(c, http.StatusForbidden, "暂不支持使用 access token", types.ErrorCodeAccessDenied)
			return
		}
		request, err := readPlaygroundPayer(c)
		if err != nil {
			if common.IsRequestBodyTooLargeError(err) {
				abortWithRequestBodyTooLarge(c)
			} else {
				abortWithOpenAiMessage(c, http.StatusBadRequest, err.Error(), types.ErrorCodeInvalidRequest)
			}
			return
		}
		userID := c.GetInt("id")
		if userID <= 0 {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, "未登录", types.ErrorCodeAccessDenied)
			return
		}
		user, err := model.GetUserCache(userID)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "无法读取用户信息", types.ErrorCodeQueryDataError)
			return
		}
		if user.Status != common.UserStatusEnabled {
			abortWithOpenAiMessage(c, http.StatusForbidden, "用户已被封禁", types.ErrorCodeAccessDenied)
			return
		}
		scope, err := resolvePlaygroundPayer(userID, request.Payer)
		if err != nil {
			abortEnterpriseToken(c, err)
			return
		}
		user.WriteContext(c)
		// A personal request never inherits a previously selected enterprise payer.
		for _, key := range []constant.ContextKey{constant.ContextKeyEnterpriseId, constant.ContextKeyEnterpriseMemberId,
			constant.ContextKeyEnterpriseRole, constant.ContextKeyEnterpriseBillingGroup} {
			common.DeleteContextKey(c, key)
		}
		if scope != nil {
			scope.WriteContext(c)
		}
		group := service.ResolveTokenGroup(user.Group, request.Group)
		if !service.GroupInUserUsableGroups(user.Group, group) {
			abortWithOpenAiMessage(c, http.StatusForbidden, "无权访问该分组", types.ErrorCodeAccessDenied)
			return
		}
		token := &model.Token{UserId: userID, Name: "playground-" + group, Group: group, UnlimitedQuota: true}
		if scope != nil {
			token.EnterpriseId, token.MembershipId = scope.Enterprise.Id, scope.Member.Id
		}
		if err := SetupContextForToken(c, token); err != nil {
			return
		}
		if scope != nil {
			scope.ApplyModelPolicy(c)
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
		common.SetContextKey(c, constant.ContextKeyPlaygroundPayerPrepared, true)
		c.Next()
	}
}
