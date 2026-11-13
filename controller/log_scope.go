package controller

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

var logChannelIDsPattern = regexp.MustCompile(`^[0-9,\s，+-]+$`)

func parseAdminLogScope(c *gin.Context) (model.LogScope, error) {
	scope := model.LogScope{Vendor: strings.TrimSpace(c.Query("group_vendor"))}
	if len(scope.Vendor) > 128 {
		return scope, errors.New("厂商名称过长")
	}
	if raw := strings.TrimSpace(c.Query("channel_ids")); raw != "" {
		ids, err := parseLogChannelIDs(raw)
		if err != nil {
			return scope, err
		}
		scope.ChannelIDs = ids
	}
	keyword := strings.TrimSpace(c.Query("channel_keyword"))
	if keyword == "" {
		return scope, nil
	}
	var matched []int
	if logChannelIDsPattern.MatchString(keyword) {
		ids, err := parseLogChannelIDs(strings.ReplaceAll(keyword, "，", ","))
		if err != nil {
			return scope, err
		}
		matched = ids
	} else {
		if len(keyword) > 128 {
			return scope, errors.New("搜索关键词过长")
		}
		rows, err := model.FindLogChannels(keyword)
		if err != nil {
			return scope, err
		}
		if len(rows) > 200 {
			return scope, errors.New("匹配超过 200 个，请缩小关键词。")
		}
		// Keep a non-nil empty slice: no name matches must never mean all channels.
		matched = make([]int, 0, len(rows))
		for _, row := range rows {
			matched = append(matched, row.ID)
		}
	}
	if scope.ChannelIDs == nil {
		scope.ChannelIDs = matched
	} else {
		allowed := make(map[int]bool, len(matched))
		for _, id := range matched {
			allowed[id] = true
		}
		intersection := []int{}
		for _, id := range scope.ChannelIDs {
			if allowed[id] {
				intersection = append(intersection, id)
			}
		}
		scope.ChannelIDs = intersection
	}
	return scope, nil
}

// userLogPayerScope 把本人日志的付款方筛选换成令牌范围。日志库可能与主库分离，
// 所以先在主库取出本人由企业付款的令牌，再让日志库按令牌过滤。
func userLogPayerScope(c *gin.Context, userID int) (model.LogScope, error) {
	payer, err := parseTokenPayer(c.Query("payer"))
	if err != nil || payer == "" {
		return model.LogScope{}, err
	}
	ids, err := model.GetUserEnterpriseTokenIDs(userID)
	if err != nil {
		return model.LogScope{}, errors.New("查询日志失败")
	}
	if payer == model.TokenPayerEnterprise {
		// ids 为空时保持非 nil：没有企业令牌就是没有企业日志，不能退化成全部日志。
		return model.LogScope{TokenIDs: ids}, nil
	}
	return model.LogScope{ExcludeTokenIDs: ids}, nil
}

func parseLogChannelIDs(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	if len(parts) > 200 {
		return nil, errors.New("最多选择 200 个渠道")
	}
	ids := []int{}
	seen := map[int]bool{}
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			return nil, errors.New("渠道 ID 必须是正整数")
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, nil
}

func GetLogChannelOptions(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	if len(keyword) > 128 {
		common.ApiError(c, errors.New("搜索关键词过长"))
		return
	}
	rows, err := model.FindLogChannels(keyword)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	truncated := len(rows) > 200
	if truncated {
		rows = rows[:200]
	}
	common.ApiSuccess(c, gin.H{"items": rows, "truncated": truncated})
}

func GetLogGroupSummary(c *gin.Context) {
	scope, err := parseAdminLogScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	start, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	channel, _ := strconv.Atoi(c.Query("channel"))
	rows, err := model.GetLogGroupSummary(model.AdminLogQueryFilters{
		Scope: scope, StartTimestamp: start, EndTimestamp: end,
		ModelName: c.Query("model_name"), Username: c.Query("username"),
		TokenName: c.Query("token_name"), Channel: channel,
		Group: c.Query("group"), RequestID: c.Query("request_id"),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}
