package model

import (
	"errors"
	"net"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PromoterIdentity struct {
	Id             int    `json:"id"`
	UserId         int    `json:"user_id" gorm:"uniqueIndex;not null"`
	Ciphertext     []byte `json:"-"`
	Nonce          []byte `json:"-"`
	SubjectHash    string `json:"-" gorm:"type:varchar(64);index"`
	MaskedName     string `json:"masked_name" gorm:"type:varchar(64)"`
	MaskedDocument string `json:"masked_document" gorm:"type:varchar(64)"`
	Status         string `json:"status" gorm:"type:varchar(20);index"`
	ReviewerId     int    `json:"reviewer_id"`
	Remark         string `json:"remark" gorm:"type:varchar(255)"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime"`
}
type InviteSubject struct {
	Hash   string `json:"-" gorm:"primaryKey;type:varchar(64)"`
	UserId int    `json:"user_id" gorm:"index"`
	Kind   string `json:"kind" gorm:"type:varchar(24)"`
}
type InviteRiskSignal struct {
	Id            int    `json:"id"`
	UserId        int    `json:"user_id" gorm:"uniqueIndex:idx_invite_signal"`
	Kind          string `json:"kind" gorm:"type:varchar(24);uniqueIndex:idx_invite_signal"`
	Hash          string `json:"-" gorm:"type:varchar(64)"`
	RelatedUserId int    `json:"related_user_id"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
}

func maskInviteValue(value string) string {
	r := []rune(value)
	if len(r) < 3 {
		return "***"
	}
	return string(r[:1]) + "***" + string(r[len(r)-1:])
}
func claimInviteSubjectTx(tx *gorm.DB, user int, kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	hash := common.GenerateHMAC("invite/" + kind + "/" + value)
	subject := InviteSubject{Hash: hash, UserId: user, Kind: kind}
	if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&subject).Error; e != nil {
		return e
	}
	if e := inviteLock(tx).First(&subject, "hash = ?", hash).Error; e != nil {
		return e
	}
	related := 0
	if subject.UserId != user {
		related = subject.UserId
	}
	signal := InviteRiskSignal{UserId: user, Kind: kind, Hash: hash, RelatedUserId: related}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "kind"}}, DoUpdates: clause.Assignments(map[string]any{"hash": hash, "related_user_id": related})}).Create(&signal).Error
}
func SavePromoterIdentity(user int, name, document string) error {
	name = strings.TrimSpace(name)
	document = strings.ToUpper(strings.TrimSpace(document))
	if !common.HasPersistentCryptoSecret() || len([]rune(name)) < 2 || len([]rune(name)) > 64 || len(document) < 6 || len(document) > 64 {
		return ErrInviteInvalid
	}
	raw, e := common.Marshal(map[string]string{"name": name, "document": document})
	if e != nil {
		return e
	}
	cipher, nonce, e := common.EncryptSensitiveData(raw, "invite-identity")
	if e != nil {
		return e
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, e := lockInviteUser(tx, user); e != nil {
			return e
		}
		var old PromoterIdentity
		e := tx.Where("user_id = ?", user).First(&old).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if old.Status == "approved" {
			return ErrInviteConflict
		}
		if e := claimInviteSubjectTx(tx, user, "identity", document); e != nil {
			return e
		}
		row := PromoterIdentity{UserId: user, Ciphertext: cipher, Nonce: nonce, SubjectHash: common.GenerateHMAC("invite/identity/" + document), MaskedName: maskInviteValue(name), MaskedDocument: maskInviteValue(document), Status: "pending"}
		if old.Id > 0 {
			row.Id = old.Id
			row.CreatedAt = old.CreatedAt
		}
		return tx.Save(&row).Error
	})
}
func SaveInviteDevice(user int, device string) error {
	if len(device) < 8 || len(device) > 256 || !common.HasPersistentCryptoSecret() {
		return ErrInviteInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, e := lockInviteUser(tx, user); e != nil {
			return e
		}
		return claimInviteSubjectTx(tx, user, "device", device)
	})
}
func InviteIdentityPlaintext(user int) (map[string]string, error) {
	var row PromoterIdentity
	if e := DB.Where("user_id = ?", user).First(&row).Error; e != nil {
		return nil, e
	}
	raw, e := common.DecryptSensitiveData(row.Ciphertext, row.Nonce, "invite-identity")
	if e != nil {
		return nil, e
	}
	var data map[string]string
	e = common.Unmarshal(raw, &data)
	return data, e
}
func invitePaymentRiskTx(tx *gorm.DB, u *User, trade string, cents int64) (string, error) {
	if common.HasPersistentCryptoSecret() && u.StripeCustomer != "" {
		if e := claimInviteSubjectTx(tx, u.Id, "payment", u.StripeCustomer); e != nil {
			return "", e
		}
	}
	var n int64
	if e := tx.Model(&InviteRiskSignal{}).Where("user_id = ? AND related_user_id > 0", u.Id).Count(&n).Error; e != nil {
		return "", e
	}
	reason := ""
	if n > 0 {
		reason = "invite_related_account"
	}
	// Registration clustering is a review signal, never an automatic freeze.
	if reason == "" && u.RegisterIP != "" {
		ip := net.ParseIP(u.RegisterIP)
		if ip != nil {
			var peers []User
			if e := tx.Select("id", "register_ip").Where("id <> ? AND created_at >= ? AND created_at <= ?", u.Id, u.CreatedAt-3*86400, u.CreatedAt+3*86400).Limit(1000).Find(&peers).Error; e != nil {
				return "", e
			}
			for _, peer := range peers {
				other := net.ParseIP(peer.RegisterIP)
				if other == nil {
					continue
				}
				bits := 64
				if ip.To4() != nil {
					bits = 24
				}
				total := 128
				if ip.To4() != nil {
					ip = ip.To4()
					other = other.To4()
					total = 32
				}
				if other == nil || !ip.Mask(net.CIDRMask(bits, total)).Equal(other.Mask(net.CIDRMask(bits, total))) {
					continue
				}
				var count int64
				if e := tx.Model(&InviteCommissionAllocation{}).Where("invitee_user_id = ? AND paid_cents = ?", peer.Id, cents).Count(&count).Error; e != nil {
					return "", e
				}
				if count > 0 {
					reason = "invite_registration_cluster"
					break
				}
			}
		}
	}
	if reason != "" {
		var existing int64
		if e := tx.Model(&PaymentRiskCase{}).Where("record_type = ? AND trade_no = ?", PaymentRiskRecordTypeTopUp, trade).Count(&existing).Error; e != nil {
			return "", e
		}
		if existing == 0 {
			if _, e := upsertPaymentRiskCaseTx(tx, PaymentRiskCaseUpsertInput{RecordType: PaymentRiskRecordTypeTopUp, TradeNo: trade, UserId: u.Id, Reason: reason, Source: "invite_tier", Currency: "CNY", ReceivedMoney: float64(cents) / 100}); e != nil {
				return "", e
			}
		}
	}
	if reason == "invite_registration_cluster" {
		return "", nil
	}
	return reason, nil
}
