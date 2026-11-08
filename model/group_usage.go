package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// groupColumnSQL quotes the reserved `group` column. initCol normally sets
// commonGroupCol at startup; fall back for callers that run before it.
func groupColumnSQL() string {
	if commonGroupCol != "" {
		return commonGroupCol
	}
	if common.UsingPostgreSQL {
		return `"group"`
	}
	return "`group`"
}

type groupCountRow struct {
	GroupName string
	Total     int64
}

func groupCountsToMap(rows []groupCountRow) map[string]int64 {
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.GroupName] += row.Total
	}
	return counts
}

// CountUsersByGroup returns the number of (not deleted) users per user group.
func CountUsersByGroup() (map[string]int64, error) {
	col := groupColumnSQL()
	var rows []groupCountRow
	err := DB.Model(&User{}).
		Select(col + " AS group_name, COUNT(*) AS total").
		Group(col).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return groupCountsToMap(rows), nil
}

// CountSubscriptionPlansByUpgradeGroup returns how many subscription plans
// upgrade buyers to each user group.
func CountSubscriptionPlansByUpgradeGroup() (map[string]int64, error) {
	var rows []groupCountRow
	err := DB.Model(&SubscriptionPlan{}).
		Select("upgrade_group AS group_name, COUNT(*) AS total").
		Where("upgrade_group <> ?", "").
		Group("upgrade_group").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return groupCountsToMap(rows), nil
}

// CountEnabledTokensByGroup counts enabled tokens per channel group they
// reference: the routing config groups for multi-group tokens, otherwise the
// token group. Empty and auto groups are skipped. A token is counted once per
// distinct group.
func CountEnabledTokensByGroup() (map[string]int64, error) {
	col := groupColumnSQL()
	var tokens []struct {
		Group         string `gorm:"column:group_name"`
		RoutingConfig string `gorm:"column:routing_config"`
	}
	err := DB.Model(&Token{}).
		Select(col+" AS group_name, routing_config").
		Where("status = ?", common.TokenStatusEnabled).
		Scan(&tokens).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64)
	for _, token := range tokens {
		groups := []string{token.Group}
		if token.RoutingConfig != "" {
			// Invalid configs are rejected at request time; count the plain
			// group so the token still shows up somewhere.
			if config, parseErr := ParseTokenRoutingConfig(token.RoutingConfig); parseErr == nil && config != nil {
				groups = config.Groups
			}
		}
		seen := make(map[string]bool, len(groups))
		for _, group := range groups {
			group = strings.TrimSpace(group)
			if group == "" || group == "auto" || seen[group] {
				continue
			}
			seen[group] = true
			counts[group]++
		}
	}
	return counts, nil
}

// ReassignUserGroup moves every user in group from to group to and returns the
// affected user ids. Callers validate both names.
func ReassignUserGroup(from, to string) ([]int, error) {
	if from == "" || to == "" || from == to {
		return nil, errors.New("invalid user group reassignment")
	}
	col := groupColumnSQL()
	var ids []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Clauses(clause.Locking{Strength: "UPDATE"}).Where(col+" = ?", from).Order("id").Pluck("id", &ids).Error; err != nil {
			return err
		}
		const chunkSize = 500
		for start := 0; start < len(ids); start += chunkSize {
			end := min(start+chunkSize, len(ids))
			result := tx.Model(&User{}).Where("id IN ? AND "+col+" = ?", ids[start:end], from).Update("group", to)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != int64(end-start) {
				return errors.New("user group changed during reassignment; retry")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if err := userCacheInvalidator(id); err != nil {
			common.SysLog("failed to invalidate user cache after group reassignment: " + err.Error())
		}
	}
	return ids, nil
}
