package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm/clause"
)

// RouteOutage holds observed routing failures, not a request or billing ledger.
// CAS revisions and notification leases work identically with all three DBs.
type RouteOutage struct {
	ID                       string `gorm:"type:varchar(64);primaryKey"`
	GroupName                string `gorm:"type:varchar(64);not null"`
	ModelName                string `gorm:"type:varchar(255);not null"`
	Active                   bool   `gorm:"not null;default:false"`
	Notified                 bool   `gorm:"not null;default:false"`
	Generation               int64  `gorm:"not null;default:0"`
	FailureCount             int64  `gorm:"not null;default:0"`
	LastNotifiedFailureCount int64  `gorm:"not null;default:0"`
	FirstFailureAt           int64  `gorm:"not null;default:0"`
	LastFailureAt            int64  `gorm:"not null;default:0;index"`
	TokenSamples             string `gorm:"type:text"`
	ChannelCount             int    `gorm:"not null;default:0"`
	RecoveredAt              int64  `gorm:"not null;default:0"`
	NextCheckAt              int64  `gorm:"not null;default:0;index"`
	NextNotifyAt             int64  `gorm:"not null;default:0;index"`
	LeaseToken               string `gorm:"type:varchar(64);not null;default:''"`
	LeaseUntil               int64  `gorm:"not null;default:0"`
	LastDelivery             string `gorm:"type:varchar(32);not null;default:''"`
	Version                  int64  `gorm:"not null;default:0"`
}

type RouteOutageBatch struct {
	Group, Model           string
	Count, FirstAt, LastAt int64
	TokenIDs               []int
}

func RouteOutageID(group, modelName string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s%s", len(group), group, modelName))))
}

var errRouteOutageConflict = errors.New("route outage changed concurrently")

func mutateRouteOutage(id string, apply func(*RouteOutage) bool) error {
	for range 16 {
		var row RouteOutage
		if err := DB.First(&row, "id = ?", id).Error; err != nil {
			return err
		}
		version := row.Version
		if !apply(&row) {
			return nil
		}
		row.Version++
		result := DB.Model(&RouteOutage{}).Where("id = ? AND version = ?", id, version).Select("*").Omit("id").Updates(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			return nil
		}
	}
	return errRouteOutageConflict
}

func RecordRouteOutage(batch RouteOutageBatch, channelCount int, now int64) error {
	if batch.Count <= 0 || batch.Group == "" || batch.Group == "auto" || batch.Model == "" || len(batch.Group) > 256 || len([]rune(batch.Group)) > 64 || len([]rune(batch.Model)) > 255 || channelCount <= 0 {
		return nil
	}
	id := RouteOutageID(batch.Group, batch.Model)
	seed := RouteOutage{ID: id, GroupName: batch.Group, ModelName: batch.Model}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
		return err
	}
	return mutateRouteOutage(id, func(row *RouteOutage) bool {
		if !row.Active {
			row.Generation++
			row.Active, row.Notified = true, false
			row.FirstFailureAt, row.FailureCount, row.RecoveredAt, row.LastNotifiedFailureCount = batch.FirstAt, 0, 0, 0
			row.NextNotifyAt = now
			row.LeaseToken, row.LeaseUntil, row.LastDelivery, row.TokenSamples = "", 0, "", ""
		}
		row.FailureCount += batch.Count
		row.LastFailureAt = max(row.LastFailureAt, batch.LastAt)
		row.FirstFailureAt = min(row.FirstFailureAt, batch.FirstAt)
		row.ChannelCount, row.NextCheckAt = channelCount, now+time.Minute.Milliseconds()
		var samples []int
		_ = common.UnmarshalJsonStr(row.TokenSamples, &samples)
		for _, token := range batch.TokenIDs {
			if token <= 0 {
				continue
			}
			found := false
			for _, existing := range samples {
				if existing == token {
					found = true
					break
				}
			}
			if !found && len(samples) < 8 {
				samples = append(samples, token)
			}
		}
		encoded, _ := common.Marshal(samples)
		row.TokenSamples = string(encoded)
		return true
	})
}

// Resolving means a configured route is selectable, not an upstream health test.
// Removing all configuration retires the incident without a false recovery.
func ResolveRouteOutage(id string, now int64, configured bool) error {
	return mutateRouteOutage(id, func(row *RouteOutage) bool {
		if !row.Active {
			return false
		}
		row.Active, row.RecoveredAt = false, now
		row.NextNotifyAt, row.NextCheckAt = now, now+time.Minute.Milliseconds()
		if !configured {
			row.Notified = false
			row.LeaseToken, row.LeaseUntil = "", 0
		}
		return true
	})
}

func DeferRouteOutageCheck(id string, now int64) error {
	return mutateRouteOutage(id, func(row *RouteOutage) bool {
		row.NextCheckAt = now + time.Minute.Milliseconds()
		return true
	})
}

func DeferRouteOutageNotice(id string, now int64) error {
	return mutateRouteOutage(id, func(row *RouteOutage) bool {
		row.NextNotifyAt = now + time.Minute.Milliseconds()
		return true
	})
}

func RetireRouteOutage(id string) error {
	return mutateRouteOutage(id, func(row *RouteOutage) bool {
		row.Active, row.Notified = false, false
		row.LeaseToken, row.LeaseUntil = "", 0
		return true
	})
}

func ListRouteOutageChecks(now int64) ([]RouteOutage, error) {
	var rows []RouteOutage
	err := DB.Where("active = ? AND next_check_at <= ?", true, now).Order("next_check_at asc").Limit(100).Find(&rows).Error
	return rows, err
}

func ListRouteOutageNotices(now int64) ([]RouteOutage, error) {
	var rows []RouteOutage
	err := DB.Where("((active = ? AND (notified = ? OR failure_count > last_notified_failure_count)) OR (active = ? AND notified = ?)) AND next_notify_at <= ? AND lease_until <= ?", true, false, false, true, now, now).Order("next_notify_at asc").Limit(10).Find(&rows).Error
	return rows, err
}

func ClaimRouteOutageNotice(id string, now int64, token string) (*RouteOutage, error) {
	var claimed *RouteOutage
	err := mutateRouteOutage(id, func(row *RouteOutage) bool {
		claimed = nil
		if (!row.Active && !row.Notified) || (row.Active && row.Notified && row.FailureCount <= row.LastNotifiedFailureCount) || row.NextNotifyAt > now || row.LeaseUntil > now {
			return false
		}
		row.LeaseToken, row.LeaseUntil = token, now+2*time.Minute.Milliseconds()
		copy := *row
		claimed = &copy
		return true
	})
	return claimed, err
}

func FinishRouteOutageNotice(claim *RouteOutage, now int64, delivery string) error {
	if claim == nil {
		return nil
	}
	return mutateRouteOutage(claim.ID, func(row *RouteOutage) bool {
		if row.LeaseToken != claim.LeaseToken || row.Generation != claim.Generation {
			return false
		}
		row.LeaseToken, row.LeaseUntil, row.LastDelivery = "", 0, delivery
		submitted := delivery == QuotaNoticeSubmitted || delivery == QuotaNoticeUnknown
		if !submitted {
			row.NextNotifyAt = now + 5*time.Minute.Milliseconds()
			return true
		}
		if claim.Active {
			row.Notified = true
			row.LastNotifiedFailureCount = max(row.LastNotifiedFailureCount, claim.FailureCount)
			row.NextNotifyAt = now + 30*time.Minute.Milliseconds()
			if !row.Active {
				row.NextNotifyAt = now
			}
		} else {
			row.Notified = false
			row.NextNotifyAt = 0
		}
		return true
	})
}

func CleanupRouteOutages(now int64) error {
	var ids []string
	if err := DB.Model(&RouteOutage{}).Where("last_failure_at < ? AND lease_until <= ?", now-7*24*time.Hour.Milliseconds(), now).Order("last_failure_at asc").Limit(500).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return DB.Where("id IN ? AND last_failure_at < ? AND lease_until <= ?", ids, now-7*24*time.Hour.Milliseconds(), now).Delete(&RouteOutage{}).Error
}

// Bounded indexed lookup deliberately ignores token/client/user restrictions.
// An oversized configuration is unknown, rather than proof of an outage.
func ConfiguredRouteChannels(group, modelName string) ([]*Channel, string, bool, error) {
	names := []string{modelName}
	if normalized := ratio_setting.FormatMatchingModelName(modelName); normalized != "" && normalized != modelName {
		names = append(names, normalized)
	}
	var abilities []Ability
	if err := DB.Where(commonGroupCol+" = ? AND model IN ?", group, names).Limit(1025).Find(&abilities).Error; err != nil {
		return nil, "", false, err
	}
	if len(abilities) > 1024 {
		return nil, "", false, nil
	}
	if len(abilities) == 0 {
		return nil, "", true, nil
	}
	canonical := modelName
	exact := false
	for _, ability := range abilities {
		if ability.Model == modelName {
			exact = true
			break
		}
	}
	if !exact {
		canonical = abilities[0].Model
	}
	enabled := map[int]bool{}
	ids := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		if _, seen := enabled[ability.ChannelId]; !seen {
			ids = append(ids, ability.ChannelId)
		}
		enabled[ability.ChannelId] = enabled[ability.ChannelId] || ability.Enabled
	}
	var channels []*Channel
	if err := DB.Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return nil, "", false, err
	}
	for _, channel := range channels {
		if !enabled[channel.Id] {
			channel.Status = common.ChannelStatusManuallyDisabled
		}
	}
	if len(channels) != len(ids) {
		return nil, "", false, nil
	}
	return channels, canonical, true, nil
}
