package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

const AudioVoiceLimit = 20

var ErrAudioVoiceLimit = errors.New("最多保留20个音色，请先删除不再使用的音色")
var ErrAudioVoiceConflict = errors.New("Idempotency-Key 已用于不同的音色请求")

// Only public fields are serialized. Neither recordings nor provider credentials
// are stored here. A sellable token gets its own scope within the issuer account.
type AudioVoice struct {
	Version         int    `json:"-"`
	ID              string `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserID          int    `json:"-" gorm:"index:idx_audio_voice_owner"`
	OwnerTokenID    int    `json:"-" gorm:"index:idx_audio_voice_owner"`
	Name            string `json:"name" gorm:"type:varchar(64)"`
	Model           string `json:"model" gorm:"type:varchar(191)"`
	Status          string `json:"status" gorm:"type:varchar(32)"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
	ChannelID       int    `json:"-" gorm:"index"`
	TargetModel     string `json:"-" gorm:"type:varchar(191)"`
	CredentialHash  string `json:"-" gorm:"type:varchar(64)"`
	UpstreamID      string `json:"-" gorm:"type:varchar(191)"`
	Prefix          string `json:"-" gorm:"type:varchar(10);uniqueIndex"`
	IdempotencyHash string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	RequestHash     string `json:"-" gorm:"type:varchar(64)"`
}

func GetAudioVoice(id string, userID, tokenID int) (*AudioVoice, error) {
	var v AudioVoice
	err := DB.Where("id = ? AND user_id = ? AND owner_token_id = ?", id, userID, tokenID).First(&v).Error
	return &v, err
}

func ListAudioVoices(userID, tokenID, offset, limit int) ([]AudioVoice, error) {
	voices := make([]AudioVoice, 0)
	err := DB.Where("user_id = ? AND owner_token_id = ? AND status <> ?", userID, tokenID, "deleted").Order("created_at DESC, id DESC").Offset(offset).Limit(limit).Find(&voices).Error
	return voices, err
}

// The user-row write serializes reservations across instances and works on all
// three engines (including SQLite, where SELECT FOR UPDATE is ineffective).
func ReserveAudioVoice(v *AudioVoice) (existing *AudioVoice, err error) {
	err = DB.Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&User{}).Where("id = ?", v.UserID).UpdateColumn("id", gorm.Expr("id")).Error; e != nil {
			return e
		}
		var old AudioVoice
		e := tx.Where("idempotency_hash = ?", v.IdempotencyHash).First(&old).Error
		if e == nil {
			if old.RequestHash != v.RequestHash {
				return ErrAudioVoiceConflict
			}
			existing = &old
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		var count int64
		if e = tx.Model(&AudioVoice{}).Where("user_id = ? AND owner_token_id = ? AND status <> ?", v.UserID, v.OwnerTokenID, "deleted").Count(&count).Error; e != nil {
			return e
		}
		if count >= AudioVoiceLimit {
			return ErrAudioVoiceLimit
		}
		v.CreatedAt, v.UpdatedAt = time.Now().Unix(), time.Now().Unix()
		return tx.Create(v).Error
	})
	return
}

// Compare-and-swap prevents a late query/create response from resurrecting a
// voice while another request is deleting it.
func UpdateAudioVoice(v *AudioVoice, status, upstreamID string) error {
	result := DB.Model(&AudioVoice{}).Where("id = ? AND status = ? AND version = ?", v.ID, v.Status, v.Version).Updates(map[string]any{"status": status, "upstream_id": upstreamID, "updated_at": time.Now().Unix(), "version": v.Version + 1})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("音色状态已变化，请重新查询")
	}
	v.Status, v.UpstreamID, v.UpdatedAt = status, upstreamID, time.Now().Unix()
	v.Version++
	return nil
}
