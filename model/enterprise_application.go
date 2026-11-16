package model

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PendingKey is nullable: a unique key allows one pending request per user/kind
// while preserving all reviewed records on SQLite, MySQL and PostgreSQL.
type EnterpriseApplication struct {
	Id           int     `json:"id" gorm:"primaryKey"`
	UserId       int     `json:"user_id" gorm:"not null;index"`
	Kind         string  `json:"kind" gorm:"type:varchar(16);not null"`
	Name         string  `json:"name" gorm:"type:varchar(64)"`
	ContactType  string  `json:"contact_type" gorm:"type:varchar(16);not null"`
	Contact      string  `json:"contact" gorm:"type:varchar(200);not null"`
	Status       string  `json:"status" gorm:"type:varchar(16);not null;index"`
	PendingKey   *string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	EnterpriseId int     `json:"enterprise_id"`
	ReviewerId   int     `json:"reviewer_id"`
	Reason       string  `json:"reason" gorm:"type:varchar(200)"`
	CreatedAt    int64   `json:"created_at"`
	ReviewedAt   int64   `json:"reviewed_at"`
}

func CreateEnterpriseApplication(userID int, kind, name, contactType, contact string) (*EnterpriseApplication, error) {
	name, contact = strings.TrimSpace(name), strings.TrimSpace(contact)
	if userID <= 0 || (kind != "opening" && kind != "custom") || (kind == "opening" && (name == "" || len([]rune(name)) > 64)) || contact == "" || len([]rune(contact)) > 200 || strings.IndexFunc(contact, unicode.IsControl) >= 0 {
		return nil, ErrEnterpriseInvalidInput
	}
	switch contactType {
	case "wechat", "qq", "telegram", "x", "email", "phone", "other":
	default:
		return nil, ErrEnterpriseInvalidInput
	}
	if kind == "custom" {
		name = ""
	}
	key := fmt.Sprintf("%d:%s", userID, kind)
	app := &EnterpriseApplication{UserId: userID, Kind: kind, Name: name, ContactType: contactType, Contact: contact, Status: "pending", PendingKey: &key, CreatedAt: time.Now().Unix()}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Select("id", "status").First(&user, userID).Error; err != nil {
			return err
		}
		if user.Status != common.UserStatusEnabled {
			return ErrEnterpriseForbidden
		}
		if kind == "opening" {
			var count int64
			if err := tx.Model(&EnterpriseMember{}).Where("active_user_id = ?", userID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrEnterpriseAlreadyMember
			}
		}
		if err := tx.Create(app).Error; err != nil {
			if isEnterpriseDuplicateKeyErr(err) {
				return ErrEnterpriseRequestPending
			}
			return err
		}
		return nil
	})
	return app, err
}

func ListEnterpriseApplications(userID int, status, kind string, offset, limit int) ([]*EnterpriseApplication, int64, error) {
	if status != "" && status != "pending" && status != "approved" && status != "rejected" && status != "contacted" {
		return nil, 0, ErrEnterpriseInvalidInput
	}
	if kind != "" && kind != "opening" && kind != "custom" {
		return nil, 0, ErrEnterpriseInvalidInput
	}
	query := DB.Model(&EnterpriseApplication{})
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*EnterpriseApplication, 0)
	err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func ReviewEnterpriseApplication(actorID, id int, action, group, reason string) (*EnterpriseApplication, error) {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > 200 || (action != "approve" && action != "reject" && action != "contact") || (action == "reject" && reason == "") {
		return nil, ErrEnterpriseInvalidInput
	}
	groupOptionsMutex.Lock()
	defer groupOptionsMutex.Unlock()
	var app EnterpriseApplication
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&app, id).Error; err != nil {
			return err
		}
		if app.Status != "pending" {
			return ErrEnterpriseFinalized
		}
		status := "rejected"
		if action == "approve" {
			if app.Kind != "opening" {
				return ErrEnterpriseInvalidInput
			}
			ent, err := createEnterpriseTx(tx, actorID, app.Name, group, "Asia/Shanghai", app.UserId)
			if err != nil {
				return err
			}
			app.EnterpriseId = ent.Id
			status = "approved"
		}
		if action == "contact" {
			if app.Kind != "custom" {
				return ErrEnterpriseInvalidInput
			}
			status = "contacted"
		}
		app.Status, app.PendingKey, app.ReviewerId, app.Reason, app.ReviewedAt = status, nil, actorID, reason, time.Now().Unix()
		result := tx.Model(&EnterpriseApplication{}).Where("id = ? AND status = ?", id, "pending").Updates(map[string]any{"status": status, "pending_key": nil, "enterprise_id": app.EnterpriseId, "reviewer_id": actorID, "reason": reason, "reviewed_at": app.ReviewedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrEnterpriseFinalized
		}
		return nil
	})
	return &app, err
}
