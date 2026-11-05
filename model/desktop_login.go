package model

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

// No session, verifier or provider token is stored here. Conditional updates
// make authorization and consumption single-use across all application nodes.
type DesktopLoginGrant struct {
	Digest      string `gorm:"primaryKey;type:varchar(64)"`
	Challenge   string `gorm:"type:varchar(43);not null"`
	Provider    string `gorm:"type:varchar(16);not null"`
	DisplayCode string `gorm:"type:varchar(8);not null"`
	Status      string `gorm:"type:varchar(16);not null"`
	UserID      int
	ExpiresAt   int64 `gorm:"index;not null"`
}

var ErrDesktopLogin = errors.New("desktop authorization unavailable")

func DesktopLoginToken() (string, error) {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data[:]), nil
}
func ValidDesktopLoginProof(s string) bool {
	if len(s) != 43 {
		return false
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == s
}
func DesktopLoginChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
func desktopLoginDigest(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:])
}
func NewDesktopLogin(challenge, provider string) (string, *DesktopLoginGrant, error) {
	if !ValidDesktopLoginProof(challenge) || (provider != "github" && provider != "google") {
		return "", nil, ErrDesktopLogin
	}
	id, err := DesktopLoginToken()
	if err != nil {
		return "", nil, err
	}
	var code [4]byte
	if _, err = rand.Read(code[:]); err != nil {
		return "", nil, err
	}
	now := time.Now().Unix()
	DB.Where("expires_at <= ?", now).Delete(&DesktopLoginGrant{})
	g := &DesktopLoginGrant{Digest: desktopLoginDigest(id), Challenge: challenge, Provider: provider, DisplayCode: hex.EncodeToString(code[:]), Status: "pending", ExpiresAt: now + 300}
	if err = DB.Create(g).Error; err != nil {
		return "", nil, err
	}
	return id, g, nil
}
func GetDesktopLogin(id string) (*DesktopLoginGrant, error) {
	if !ValidDesktopLoginProof(id) {
		return nil, ErrDesktopLogin
	}
	var g DesktopLoginGrant
	if err := DB.Where("digest = ? AND expires_at > ?", desktopLoginDigest(id), time.Now().Unix()).First(&g).Error; err != nil {
		return nil, ErrDesktopLogin
	}
	return &g, nil
}
func ApproveDesktopLogin(id string, userID int, code string, deny bool) error {
	g, err := GetDesktopLogin(id)
	if err != nil || userID <= 0 {
		return ErrDesktopLogin
	}
	if subtle.ConstantTimeCompare([]byte(g.DisplayCode), []byte(code)) != 1 {
		return ErrDesktopLogin
	}
	state := "approved"
	if deny {
		state = "denied"
	}
	res := DB.Model(&DesktopLoginGrant{}).Where("digest = ? AND status = ? AND expires_at > ?", g.Digest, "pending", time.Now().Unix()).Updates(map[string]interface{}{"status": state, "user_id": userID})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrDesktopLogin
	}
	return nil
}
func DesktopLoginWithProof(id, verifier string) (*DesktopLoginGrant, error) {
	if !ValidDesktopLoginProof(verifier) {
		return nil, ErrDesktopLogin
	}
	g, err := GetDesktopLogin(id)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(g.Challenge), []byte(DesktopLoginChallenge(verifier))) != 1 {
		return nil, ErrDesktopLogin
	}
	return g, nil
}
func ConsumeDesktopLogin(g *DesktopLoginGrant) error {
	res := DB.Model(&DesktopLoginGrant{}).Where("digest = ? AND challenge = ? AND status = ? AND user_id = ? AND expires_at > ?", g.Digest, g.Challenge, "approved", g.UserID, time.Now().Unix()).Update("status", "consumed")
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrDesktopLogin
	}
	return nil
}
func CancelDesktopLogin(id, verifier string) error {
	g, err := DesktopLoginWithProof(id, verifier)
	if err != nil {
		return err
	}
	return DB.Model(&DesktopLoginGrant{}).Where("digest = ? AND status IN ?", g.Digest, []string{"pending", "approved"}).Update("status", "denied").Error
}
