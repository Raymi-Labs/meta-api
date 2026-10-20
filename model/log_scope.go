package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// LogScope is display/query-only: never use these filters for channel routing.
// A nil ChannelIDs slice means unrestricted; an empty non-nil slice matches nothing.
type LogScope struct {
	Vendor     string
	ChannelIDs []int
}

// Keep aliases consistent with web/src/components/table/tokens/tokenGroupUtils.js.
var logVendorAliases = map[string]string{
	"openai": "openai", "claude": "claude", "anthropic": "anthropic",
	"gemini": "gemini", "google": "google", "grok": "grok", "xai": "xai",
	"kimi": "kimi", "moonshot": "moonshot", "deepseek": "deepseek",
	"mimo": "mimo", "xiaomi": "mimo", "qwen": "qwen", "alibaba": "qwen",
	"阿里巴巴": "qwen", "阿里通义千问": "qwen", "通义千问": "qwen",
	"glm": "glm", "zhipu": "glm", "智谱": "glm",
	"baidu": "baidu", "百度": "baidu", "spark": "spark", "讯飞": "spark",
	"hunyuan": "hunyuan", "腾讯": "hunyuan", "yi": "yi", "零一万物": "yi",
	"minimax": "minimax", "cohere": "cohere", "jina": "jina", "mistral": "mistral",
	"inception": "inception", "inception labs": "inception", "inceptionlabs": "inception", "mercury": "inception",
	"perplexity": "perplexity", "360": "360", "suno": "suno", "coze": "coze",
	"kling": "kling", "可灵": "kling", "jimeng": "jimeng", "即梦": "jimeng", "vidu": "vidu",
}

func applyLogScope(tx *gorm.DB, scopes ...LogScope) *gorm.DB {
	for _, scope := range scopes {
		if scope.ChannelIDs != nil {
			if len(scope.ChannelIDs) == 0 {
				tx = tx.Where("1 = 0")
			} else {
				tx = tx.Where("logs.channel_id IN ?", scope.ChannelIDs)
			}
		}
		vendor := strings.ToLower(strings.TrimSpace(scope.Vendor))
		if vendor == "" {
			continue
		}
		col := qualifiedLogGroupCol()
		pos := "INSTR(" + col + ", '·')"
		prefix := "SUBSTR(" + col + ", 1, " + pos + " - 1)"
		switch tx.Dialector.Name() {
		case "postgres":
			pos = "STRPOS(" + col + ", '·')"
			prefix = "SPLIT_PART(" + col + ", '·', 1)"
		case "mysql":
			pos = "LOCATE('·', " + col + ")"
			prefix = "SUBSTRING_INDEX(" + col + ", '·', 1)"
		}
		// Like the UI, a leading separator is not a scoped vendor.
		separated := pos + " > 1 AND TRIM(" + prefix + ") <> ''"
		if vendor == "__other__" {
			known := make([]string, 0, len(logVendorAliases))
			for alias := range logVendorAliases {
				known = append(known, alias)
			}
			tx = tx.Where("TRIM("+col+") <> ''").
				Where("NOT ("+separated+")").
				Where("LOWER(TRIM("+col+")) NOT IN ?", known)
			continue
		}
		canonical, known := logVendorAliases[vendor]
		if !known {
			canonical = vendor
		}
		aliases := []string{canonical}
		for alias, target := range logVendorAliases {
			if target == canonical && alias != canonical {
				aliases = append(aliases, alias)
			}
		}
		scoped := "(" + separated + ") AND LOWER(TRIM(" + prefix + ")) IN ?"
		if known {
			tx = tx.Where("("+scoped+") OR LOWER(TRIM("+col+")) IN ?", aliases, aliases)
		} else {
			tx = tx.Where(scoped, aliases)
		}
	}
	return tx
}

type LogChannelOption struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status int    `json:"status"`
}

// Query the primary DB separately: LOG_DB may be on another server.
func FindLogChannels(keyword string) ([]LogChannelOption, error) {
	pattern, err := sanitizeContainsLikePattern(keyword)
	if err != nil {
		return nil, err
	}
	if pattern == "" {
		return []LogChannelOption{}, nil
	}
	result := []LogChannelOption{}
	err = DB.Model(&Channel{}).Select("id, name, status").
		Where("LOWER(name) LIKE ? ESCAPE '!'", strings.ToLower(pattern)).
		Order("id ASC").Limit(201).Find(&result).Error
	return result, err
}

type LogGroupSummary struct {
	Group            string `json:"group" gorm:"column:group_name"`
	RequestCount     int64  `json:"request_count"`
	Quota            int64  `json:"quota"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
}

func GetLogGroupSummary(filters AdminLogQueryFilters) ([]LogGroupSummary, error) {
	// Explicitly summarize consumption records, not retry/error bookkeeping rows.
	filters.LogType = LogTypeConsume
	if filters.StartTimestamp <= 0 || filters.EndTimestamp < filters.StartTimestamp {
		return nil, errors.New("分组汇总需要有效的时间范围")
	}
	tx, err := applyAdminLogFilters(LOG_DB.Table("logs"), filters, true)
	if err != nil {
		return nil, err
	}
	col := qualifiedLogGroupCol()
	rows := []LogGroupSummary{}
	err = tx.Select(col + " AS group_name, COUNT(*) AS request_count, " +
		"COALESCE(SUM(quota), 0) AS quota, COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens, " +
		"COALESCE(SUM(completion_tokens), 0) AS completion_tokens").
		Group(col).Order("quota DESC").Order(col + " ASC").Limit(1001).Scan(&rows).Error
	if len(rows) > 1000 {
		return nil, errors.New("分组过多，请缩小查询范围")
	}
	return rows, err
}
