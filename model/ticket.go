package model

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const (
	TicketStatusOpen        = "open"
	TicketStatusInProgress  = "in_progress"
	TicketStatusWaitingUser = "waiting_user"
	TicketStatusResolved    = "resolved"
	TicketStatusClosed      = "closed"

	TicketCategoryBilling    = "billing"
	TicketCategoryAPI        = "api"
	TicketCategoryAccount    = "account"
	TicketCategoryInvoice    = "invoice"
	TicketCategoryEnterprise = "enterprise"
	TicketCategoryAppeal     = "appeal"
	TicketCategoryOther      = "other"

	TicketSenderUser   = "user"
	TicketSenderStaff  = "staff"
	TicketSenderSystem = "system"

	// 别名
	SenderRoleUser   = TicketSenderUser
	SenderRoleStaff  = TicketSenderStaff
	SenderRoleSystem = TicketSenderSystem
)

var (
	ErrTicketNotFound         = errors.New("工单不存在")
	ErrTicketUnauthorized     = errors.New("无权访问此工单")
	ErrTicketMaxOpen          = errors.New("您已有5个未结工单，请等待处理或关闭后再提交")
	ErrTicketInvalidStatus    = errors.New("工单状态无效")
	ErrTicketInvalidCategory  = errors.New("工单分类无效")
	ErrTicketClosed           = errors.New("工单已关闭，无法回复")
	ErrTicketInvalidRequestID = errors.New("请求ID格式无效")
	ErrTicketMessageTooLong   = errors.New("消息内容不能超过5000字符")
	ErrTicketSubjectTooLong   = errors.New("标题不能超过100字符")
	ErrTicketInvalidAssignee  = errors.New("指派人无效")
	ErrTicketMaxAppeal        = errors.New("您已有1个未结申诉工单")
)

var ticketRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

type Ticket struct {
	ID              int    `json:"id" gorm:"primaryKey"`
	UserID          int    `json:"user_id" gorm:"not null;index"`
	Subject         string `json:"subject" gorm:"size:100;not null"`
	Category        string `json:"category" gorm:"size:16;not null;index"`
	Status          string `json:"status" gorm:"size:16;not null;index"`
	Assignee        int    `json:"assignee" gorm:"index;default:0"`
	RequestID       string `json:"request_id" gorm:"size:64;index;default:''"`
	UserUnread      bool   `json:"user_unread" gorm:"not null;default:false;index"`
	StaffUnread     bool   `json:"staff_unread" gorm:"not null;default:false;index"`
	LastMessageAt   int64  `json:"last_message_at" gorm:"not null;index"`
	StatusUpdatedAt int64  `json:"status_updated_at" gorm:"not null"`
	CreatedAt       int64  `json:"created_at" gorm:"not null;index"`
	UpdatedAt       int64  `json:"updated_at" gorm:"not null"`
}

type TicketMessage struct {
	Attachments []TicketAttachment `json:"attachments,omitempty" gorm:"-"`
	ID          int                `json:"id" gorm:"primaryKey"`
	TicketID    int                `json:"ticket_id" gorm:"not null;index"`
	SenderID    int                `json:"sender_id" gorm:"not null;index"`
	SenderRole  string             `json:"sender_role" gorm:"size:16;not null"`
	Event       string             `json:"event" gorm:"size:64;default:''"`
	Body        string             `json:"body" gorm:"type:text"`
	IsInternal  bool               `json:"is_internal" gorm:"not null;default:false"`
	CreatedAt   int64              `json:"created_at" gorm:"not null;index"`
}

type TicketAttachment struct {
	ID           int    `json:"id" gorm:"primaryKey"`
	TicketID     int    `json:"ticket_id" gorm:"not null;index"`
	MessageID    *int   `json:"message_id" gorm:"index"` // 可选：关联到消息
	Filename     string `json:"filename" gorm:"size:255;not null"`
	MimeType     string `json:"mime_type" gorm:"size:64;not null"`
	Size         int    `json:"size" gorm:"not null"`
	StoragePath  string `json:"-" gorm:"type:text;not null"`
	UploadedByID int    `json:"uploaded_by_id" gorm:"not null;index"`
	CreatedAt    int64  `json:"created_at" gorm:"not null"`
}

func ticketDB(ctx context.Context) *gorm.DB {
	return DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).WithContext(ctx)
}

func ValidTicketStatus(status string) bool {
	switch status {
	case TicketStatusOpen, TicketStatusInProgress, TicketStatusWaitingUser, TicketStatusResolved, TicketStatusClosed:
		return true
	}
	return false
}

func ValidTicketCategory(category string) bool {
	switch category {
	case TicketCategoryBilling, TicketCategoryAPI, TicketCategoryAccount,
		TicketCategoryInvoice, TicketCategoryEnterprise, TicketCategoryAppeal, TicketCategoryOther:
		return true
	}
	return false
}

func ValidTicketRequestID(requestID string) bool {
	if requestID == "" {
		return true
	}
	return ticketRequestIDPattern.MatchString(requestID)
}

func invalidTicketBodyRune(r rune) bool {
	return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
}

func ValidTicketMessageBody(body string) bool {
	return strings.TrimSpace(body) != "" && len([]rune(body)) <= 5000
}

func ValidTicketSubject(subject string) bool {
	subjectRunes := []rune(subject)
	return len(subjectRunes) > 0 && len(subjectRunes) <= 100
}

func CreateTicket(ctx context.Context, userID int, subject, category, requestID, body string) (*Ticket, error) {
	subject = strings.TrimSpace(subject)
	body = strings.TrimSpace(body)
	if !ValidTicketSubject(subject) {
		return nil, ErrTicketSubjectTooLong
	}
	if !ValidTicketCategory(category) {
		return nil, ErrTicketInvalidCategory
	}
	if !ValidTicketRequestID(requestID) {
		return nil, ErrTicketInvalidRequestID
	}
	if !ValidTicketMessageBody(body) {
		return nil, ErrTicketMessageTooLong
	}
	if strings.IndexFunc(subject, unicode.IsControl) >= 0 || strings.IndexFunc(body, invalidTicketBodyRune) >= 0 {
		return nil, errors.New("标题或内容包含无效字符")
	}

	now := time.Now().Unix()
	ticket := &Ticket{
		UserID:          userID,
		Subject:         subject,
		Category:        category,
		Status:          TicketStatusOpen,
		RequestID:       requestID,
		UserUnread:      false,
		StaffUnread:     true,
		LastMessageAt:   now,
		StatusUpdatedAt: now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	err := ticketDB(ctx).Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "status").First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("用户不存在")
			}
			return err
		}
		// 非申诉工单要求用户状态正常
		if category != TicketCategoryAppeal && user.Status != 1 {
			return errors.New("账号已被封禁")
		}

		var openCount int64
		countQuery := tx.Model(&Ticket{}).Where("user_id = ? AND status IN ?", userID,
			[]string{TicketStatusOpen, TicketStatusInProgress, TicketStatusWaitingUser})
		if category == TicketCategoryAppeal {
			countQuery = tx.Model(&Ticket{}).Where("user_id = ? AND category = ? AND status <> ?", userID, TicketCategoryAppeal, TicketStatusClosed)
		}
		if err := countQuery.Count(&openCount).Error; err != nil {
			return err
		}
		if category == TicketCategoryAppeal {
			if openCount > 0 {
				return ErrTicketMaxAppeal
			}
		} else if openCount >= 5 {
			return ErrTicketMaxOpen
		}

		// Count only committed creations while holding the same user row lock.
		// Failed validation/writes never consume a successful-creation slot.
		duration, limit := int64(3600), int64(3)
		recent := tx.Model(&Ticket{}).Where("user_id = ?", userID)
		if category == TicketCategoryAppeal {
			duration, limit = 86400, 1
			recent = recent.Where("category = ?", TicketCategoryAppeal)
		} else {
			recent = recent.Where("category <> ?", TicketCategoryAppeal)
		}
		now = time.Now().Unix()
		var recentCount int64
		if err := recent.Where("created_at > ?", now-duration).Count(&recentCount).Error; err != nil {
			return err
		}
		if recentCount >= limit {
			return fmt.Errorf("操作过于频繁，请在 %d 秒后重试", duration)
		}
		ticket.CreatedAt, ticket.UpdatedAt, ticket.LastMessageAt, ticket.StatusUpdatedAt = now, now, now, now

		if err := tx.Create(ticket).Error; err != nil {
			return err
		}

		message := &TicketMessage{
			TicketID:   ticket.ID,
			SenderID:   userID,
			SenderRole: TicketSenderUser,
			Body:       body,
			IsInternal: false,
			CreatedAt:  now,
		}
		return tx.Create(message).Error
	})

	return ticket, err
}

type TicketFilter struct {
	UserID          int
	Status          string
	Category        string
	Keyword         string
	AssignedToMe    int
	Unassigned      bool
	StaffUnreadOnly bool
}

func ListTickets(ctx context.Context, filter TicketFilter, offset, limit int) ([]Ticket, int64, error) {
	query := ticketDB(ctx).Model(&Ticket{})

	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.Keyword != "" {
		pattern, err := sanitizeContainsLikePattern(filter.Keyword)
		if err != nil {
			return nil, 0, err
		}
		if pattern != "" {
			query = query.Where("subject LIKE ? ESCAPE '!'", pattern)
		}
	}
	if filter.AssignedToMe > 0 {
		query = query.Where("assignee = ?", filter.AssignedToMe)
	}
	if filter.Unassigned {
		query = query.Where("assignee = 0")
	}
	if filter.StaffUnreadOnly {
		query = query.Where("staff_unread = ?", true)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var tickets []Ticket
	err := query.Order("last_message_at DESC").Offset(offset).Limit(limit).Find(&tickets).Error
	return tickets, total, err
}

func GetTicket(ctx context.Context, ticketID int) (*Ticket, error) {
	var ticket Ticket
	err := ticketDB(ctx).First(&ticket, ticketID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTicketNotFound
	}
	return &ticket, err
}

func GetTicketMessages(ctx context.Context, ticketID int, includeInternal bool) ([]TicketMessage, error) {
	query := ticketDB(ctx).Where("ticket_id = ?", ticketID)
	if !includeInternal {
		query = query.Where("is_internal = ?", false)
	}
	var messages []TicketMessage
	err := query.Order("id ASC").Find(&messages).Error
	return messages, err
}

// ReadTicketMessages acknowledges only a successfully prepared snapshot. Writers
// lock the same ticket row, so activity after this transaction remains unread.
func ReadTicketMessages(ctx context.Context, ticket *Ticket, includeInternal bool) ([]TicketMessage, error) {
	var messages []TicketMessage
	var current Ticket
	err := ticketDB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, ticket.ID).Error; err != nil {
			return err
		}
		query := tx.Where("ticket_id = ?", current.ID)
		if !includeInternal {
			query = query.Where("is_internal = ?", false)
		}
		if err := query.Order("id ASC").Find(&messages).Error; err != nil {
			return err
		}
		// Attach only to messages in the authorized snapshot.
		var images []TicketAttachment
		if err := tx.Where("ticket_id = ? AND message_id IS NOT NULL", current.ID).Order("id ASC").Find(&images).Error; err != nil {
			return err
		}
		byMessage := make(map[int][]TicketAttachment)
		for _, image := range images {
			id := *image.MessageID
			byMessage[id] = append(byMessage[id], image)
		}
		for i := range messages {
			messages[i].Attachments = byMessage[messages[i].ID]
		}
		column := "user_unread"
		if includeInternal {
			column = "staff_unread"
		}
		if err := tx.Model(&Ticket{}).Where("id = ?", current.ID).Update(column, false).Error; err != nil {
			return err
		}
		if includeInternal {
			current.StaffUnread = false
		} else {
			current.UserUnread = false
		}
		return nil
	})
	if err == nil {
		*ticket = current
	}
	return messages, err
}

func GetTicketAttachments(ctx context.Context, ticketID int, includeInternal bool) ([]TicketAttachment, error) {
	query := ticketDB(ctx).Where("ticket_id = ?", ticketID)
	if !includeInternal {
		// 只返回非内部消息的附件，或未关联消息的附件
		query = query.Where("message_id IS NULL OR message_id IN (?)",
			ticketDB(ctx).Table("ticket_messages").Select("id").Where("ticket_id = ? AND is_internal = ?", ticketID, false))
	}
	var attachments []TicketAttachment
	err := query.Order("id ASC").Find(&attachments).Error
	return attachments, err
}

func CreateTicketMessage(ctx context.Context, ticketID, senderID int, senderRole, body string, isInternal bool) (*TicketMessage, error) {
	body = strings.TrimSpace(body)
	if !ValidTicketMessageBody(body) {
		return nil, ErrTicketMessageTooLong
	}
	if strings.IndexFunc(body, invalidTicketBodyRune) >= 0 {
		return nil, errors.New("消息内容包含无效字符")
	}

	now := time.Now().Unix()
	message := &TicketMessage{
		TicketID:   ticketID,
		SenderID:   senderID,
		SenderRole: senderRole,
		Body:       body,
		IsInternal: isInternal,
		CreatedAt:  now,
	}

	err := ticketDB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{
			"last_message_at": now,
			"updated_at":      now,
		}
		if senderRole == TicketSenderUser {
			updates["user_unread"] = false
			updates["staff_unread"] = true
		} else if !isInternal {
			updates["user_unread"] = true
			updates["staff_unread"] = false
		}
		return tx.Model(&Ticket{}).Where("id = ?", ticketID).Updates(updates).Error
	})

	return message, err
}

func CreateTicketEventMessage(ctx context.Context, ticketID int, event, body string) error {
	now := time.Now().Unix()
	message := &TicketMessage{
		TicketID:   ticketID,
		SenderID:   0,
		SenderRole: TicketSenderSystem,
		Event:      event,
		Body:       body,
		IsInternal: false,
		CreatedAt:  now,
	}
	return ticketDB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		return tx.Model(&Ticket{}).Where("id = ?", ticketID).Updates(map[string]interface{}{
			"last_message_at": now,
			"updated_at":      now,
		}).Error
	})
}

func CreateTicketAttachment(ctx context.Context, attachment *TicketAttachment) (*TicketAttachment, error) {
	now := time.Now().Unix()
	attachment.CreatedAt = now
	err := ticketDB(ctx).Transaction(func(tx *gorm.DB) error {
		var ticket Ticket
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&ticket, attachment.TicketID).Error; err != nil {
			return err
		}
		if ticket.Status == TicketStatusClosed {
			return ErrTicketClosed
		}
		internal := false
		if attachment.MessageID != nil {
			var message TicketMessage
			if err := tx.Where("id = ? AND ticket_id = ?", *attachment.MessageID, ticket.ID).First(&message).Error; err != nil {
				return err
			}
			internal = message.IsInternal
		}
		if err := insertTicketAttachment(tx, attachment); err != nil {
			return err
		}
		role := TicketSenderStaff
		if attachment.UploadedByID == ticket.UserID {
			role = TicketSenderUser
		}
		return updateTicketActivity(tx, &ticket, role, internal, "", now)
	})
	return attachment, err
}

func insertTicketAttachment(tx *gorm.DB, attachment *TicketAttachment) error {
	var count int64
	if err := tx.Model(&TicketAttachment{}).Where("ticket_id = ?", attachment.TicketID).Count(&count).Error; err != nil {
		return err
	}
	if count >= 10 {
		return errors.New("每个工单最多10个附件")
	}
	return tx.Create(attachment).Error
}

func GetTicketAttachment(ctx context.Context, attachmentID int) (*TicketAttachment, error) {
	var attachment TicketAttachment
	err := ticketDB(ctx).First(&attachment, attachmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTicketAttachmentNotFound
	}
	return &attachment, err
}

func TicketReplyAndTransition(ctx context.Context, ticketID, senderID int, senderRole, body string, isInternal bool, newStatus string, oldStatus string, image *TicketAttachment) error {
	body = strings.TrimSpace(body)
	if (body != "" || image == nil) && !ValidTicketMessageBody(body) {
		return ErrTicketMessageTooLong
	}
	if strings.IndexFunc(body, invalidTicketBodyRune) >= 0 {
		return errors.New("消息内容包含无效字符")
	}
	if newStatus != "" && !ValidTicketStatus(newStatus) {
		return ErrTicketInvalidStatus
	}

	now := time.Now().Unix()
	message := &TicketMessage{
		TicketID:   ticketID,
		SenderID:   senderID,
		SenderRole: senderRole,
		Body:       body,
		IsInternal: isInternal,
		CreatedAt:  now,
	}

	return ticketDB(ctx).Transaction(func(tx *gorm.DB) error {
		var ticket Ticket
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&ticket, ticketID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}
		if ticket.Status != oldStatus {
			return errors.New("工单状态已变更")
		}
		if ticket.Status == TicketStatusClosed {
			return ErrTicketClosed
		}

		if err := tx.Create(message).Error; err != nil {
			return err
		}

		if image != nil {
			image.TicketID = ticket.ID
			image.MessageID = &message.ID
			image.UploadedByID = senderID
			image.CreatedAt = now
			if err := insertTicketAttachment(tx, image); err != nil {
				return err
			}
		}

		return updateTicketActivity(tx, &ticket, senderRole, isInternal, newStatus, now)
	})
}

// updateTicketActivity requires the caller to hold the ticket row lock in tx.
// MySQL may report zero changed rows for same-second activity; that is not a
// conflict after the locked row's status has been checked.
func updateTicketActivity(tx *gorm.DB, ticket *Ticket, senderRole string, isInternal bool, newStatus string, now int64) error {
	updates := map[string]interface{}{
		"last_message_at": now,
		"updated_at":      now,
	}
	status := ticket.Status
	if !isInternal {
		if senderRole == TicketSenderUser {
			updates["user_unread"] = false
			updates["staff_unread"] = true
			if status == TicketStatusWaitingUser || status == TicketStatusResolved {
				status = TicketStatusOpen
			}
		} else {
			updates["user_unread"] = true
			updates["staff_unread"] = false
			status = newStatus
			if status == "" {
				status = TicketStatusWaitingUser
			}
		}
	}
	if status != ticket.Status {
		updates["status"] = status
		updates["status_updated_at"] = now
	}
	if err := tx.Model(&Ticket{}).Where("id = ? AND status = ?", ticket.ID, ticket.Status).Updates(updates).Error; err != nil {
		return err
	}
	if status != ticket.Status {
		return tx.Create(&TicketMessage{
			TicketID:   ticket.ID,
			SenderRole: TicketSenderSystem,
			Event:      "status_" + status,
			Body:       fmt.Sprintf("工单状态由 %s 变更为 %s", formatTicketStatus(ticket.Status), formatTicketStatus(status)),
			CreatedAt:  now,
		}).Error
	}
	return nil
}

func CloseTicket(ctx context.Context, ticketID, userID int, oldStatus string) error {
	now := time.Now().Unix()
	return ticketDB(ctx).Transaction(func(tx *gorm.DB) error {
		var ticket Ticket
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&ticket, ticketID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketNotFound
			}
			return err
		}
		if ticket.UserID != userID {
			return ErrTicketUnauthorized
		}
		if ticket.Status != oldStatus {
			return errors.New("工单状态已变更")
		}
		if ticket.Status == TicketStatusClosed {
			return nil
		}

		result := tx.Model(&Ticket{}).Where("id = ? AND status = ?", ticketID, oldStatus).Updates(map[string]interface{}{
			"status":            TicketStatusClosed,
			"status_updated_at": now,
			"updated_at":        now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("工单状态已变更")
		}

		eventMsg := &TicketMessage{
			TicketID:   ticketID,
			SenderID:   0,
			SenderRole: TicketSenderSystem,
			Event:      "status_closed",
			Body:       "用户关闭了工单",
			IsInternal: false,
			CreatedAt:  now,
		}
		return tx.Create(eventMsg).Error
	})
}

func UpdateTicketStatus(ctx context.Context, ticketID int, newStatus, oldStatus string) error {
	if !ValidTicketStatus(newStatus) {
		return ErrTicketInvalidStatus
	}

	now := time.Now().Unix()
	return ticketDB(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Ticket{}).Where("id = ? AND status = ?", ticketID, oldStatus).Updates(map[string]interface{}{
			"status":            newStatus,
			"status_updated_at": now,
			"updated_at":        now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var ticket Ticket
			if err := tx.First(&ticket, ticketID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrTicketNotFound
				}
				return err
			}
			if ticket.Status == newStatus {
				return nil
			}
			return errors.New("工单状态已变更")
		}

		eventBody := fmt.Sprintf("管理员将工单状态由 %s 变更为 %s", formatTicketStatus(oldStatus), formatTicketStatus(newStatus))
		eventMsg := &TicketMessage{
			TicketID:   ticketID,
			SenderID:   0,
			SenderRole: TicketSenderSystem,
			Event:      "status_" + newStatus,
			Body:       eventBody,
			IsInternal: false,
			CreatedAt:  now,
		}
		return tx.Create(eventMsg).Error
	})
}

func UpdateTicketCategory(ctx context.Context, ticketID int, category string) error {
	if !ValidTicketCategory(category) {
		return ErrTicketInvalidCategory
	}
	now := time.Now().Unix()
	result := ticketDB(ctx).Model(&Ticket{}).Where("id = ?", ticketID).Updates(map[string]interface{}{
		"category":   category,
		"updated_at": now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// MySQL reports changed rows, so an idempotent update may return zero.
		_, err := GetTicket(ctx, ticketID)
		return err
	}
	return nil
}

func UpdateTicketAssignee(ctx context.Context, ticketID, assignee int) error {
	if assignee < 0 {
		return ErrTicketInvalidAssignee
	}
	if assignee > 0 {
		var user User
		err := ticketDB(ctx).Select("id", "status", "role").First(&user, assignee).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketInvalidAssignee
			}
			return err
		}
		if user.Status != 1 || user.Role < 10 {
			return ErrTicketInvalidAssignee
		}
	}

	now := time.Now().Unix()
	result := ticketDB(ctx).Model(&Ticket{}).Where("id = ?", ticketID).Updates(map[string]interface{}{
		"assignee":   assignee,
		"updated_at": now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// MySQL reports changed rows, so an idempotent update may return zero.
		_, err := GetTicket(ctx, ticketID)
		return err
	}
	return nil
}

// AssignTicket 分配工单给管理员
func AssignTicket(ctx context.Context, ticketID, assigneeID int) error {
	return UpdateTicketAssignee(ctx, ticketID, assigneeID)
}

func ClearTicketUserUnread(ctx context.Context, ticketID, userID int) error {
	result := ticketDB(ctx).Model(&Ticket{}).Where("id = ? AND user_id = ? AND user_unread = ?", ticketID, userID, true).
		Update("user_unread", false)
	return result.Error
}

func ClearTicketStaffUnread(ctx context.Context, ticketID int) error {
	result := ticketDB(ctx).Model(&Ticket{}).Where("id = ? AND staff_unread = ?", ticketID, true).
		Update("staff_unread", false)
	return result.Error
}

func CountTicketUserUnread(ctx context.Context, userID int) (int64, error) {
	var count int64
	err := ticketDB(ctx).Model(&Ticket{}).Where("user_id = ? AND user_unread = ?", userID, true).Count(&count).Error
	return count, err
}

type StaffTicketCounts struct {
	UnreadCount  int64 `json:"count"`
	PendingCount int64 `json:"pending_count"`
}

func GetStaffTicketCounts(ctx context.Context) (StaffTicketCounts, error) {
	var counts StaffTicketCounts
	err := ticketDB(ctx).Model(&Ticket{}).
		Where("status IN ?", []string{TicketStatusOpen, TicketStatusInProgress}).
		Select("COUNT(CASE WHEN staff_unread = ? THEN 1 END) AS unread_count, COUNT(*) AS pending_count", true).
		Scan(&counts).Error
	return counts, err
}

func CountTicketStaffPending(ctx context.Context) (int64, error) {
	counts, err := GetStaffTicketCounts(ctx)
	return counts.UnreadCount, err
}

func ListTicketsForAutoTransition(ctx context.Context, status string, staleThreshold int64, limit int) ([]Ticket, error) {
	var tickets []Ticket
	err := ticketDB(ctx).Where("status = ? AND status_updated_at < ? AND last_message_at < ?", status, staleThreshold, staleThreshold).
		Order("id ASC").Limit(limit).Find(&tickets).Error
	return tickets, err
}

func AutoTransitionTicketStatus(ctx context.Context, ticketID int, fromStatus, toStatus string, staleThreshold int64) error {
	if !ValidTicketStatus(fromStatus) || !ValidTicketStatus(toStatus) {
		return ErrTicketInvalidStatus
	}

	now := time.Now().Unix()
	return ticketDB(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Ticket{}).Where("id = ? AND status = ? AND status_updated_at < ? AND last_message_at < ?", ticketID, fromStatus, staleThreshold, staleThreshold).Updates(map[string]interface{}{
			"status":            toStatus,
			"status_updated_at": now,
			"updated_at":        now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}

		eventBody := fmt.Sprintf("工单已自动由 %s 变更为 %s", formatTicketStatus(fromStatus), formatTicketStatus(toStatus))
		eventMsg := &TicketMessage{
			TicketID:   ticketID,
			SenderID:   0,
			SenderRole: TicketSenderSystem,
			Event:      "status_" + toStatus,
			Body:       eventBody,
			IsInternal: false,
			CreatedAt:  now,
		}
		return tx.Create(eventMsg).Error
	})
}

func GetTicketRelatedLogs(ctx context.Context, userID int, requestID string, limit int) ([]Log, error) {
	if requestID == "" || userID <= 0 {
		return []Log{}, nil
	}
	var logs []Log
	err := LOG_DB.WithContext(ctx).Where("user_id = ? AND request_id = ?", userID, requestID).
		Order("id DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

func formatTicketStatus(status string) string {
	switch status {
	case TicketStatusOpen:
		return "待处理"
	case TicketStatusInProgress:
		return "处理中"
	case TicketStatusWaitingUser:
		return "等待用户回复"
	case TicketStatusResolved:
		return "已解决"
	case TicketStatusClosed:
		return "已关闭"
	default:
		return status
	}
}

// Additional helper functions

var ErrTicketAttachmentNotFound = errors.New("附件不存在")

func GetTicketAttachmentCount(ctx context.Context, ticketID int) (int64, error) {
	var count int64
	err := ticketDB(ctx).Model(&TicketAttachment{}).Where("ticket_id = ?", ticketID).Count(&count).Error
	return count, err
}

func GetTicketAttachmentByID(ctx context.Context, attachmentID int) (*TicketAttachment, error) {
	var attachment TicketAttachment
	err := ticketDB(ctx).First(&attachment, attachmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTicketAttachmentNotFound
	}
	return &attachment, err
}

// DeleteTicketAttachment keeps authorization and closure checks under the same
// row lock used by uploads, replies and status transitions.
func DeleteTicketAttachment(ctx context.Context, attachmentID, actorID int, isAdmin bool) (*TicketAttachment, error) {
	attachment, err := GetTicketAttachment(ctx, attachmentID)
	if err != nil {
		return nil, err
	}
	err = ticketDB(ctx).Transaction(func(tx *gorm.DB) error {
		var ticket Ticket
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&ticket, attachment.TicketID).Error; err != nil {
			return err
		}
		if err := tx.First(attachment, attachmentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTicketAttachmentNotFound
			}
			return err
		}
		if !isAdmin {
			if ticket.UserID != actorID || attachment.UploadedByID != actorID {
				return ErrTicketUnauthorized
			}
			if attachment.MessageID != nil {
				var message TicketMessage
				if err := tx.Where("id = ? AND ticket_id = ? AND is_internal = ?", *attachment.MessageID, ticket.ID, false).First(&message).Error; err != nil {
					return ErrTicketUnauthorized
				}
			}
		}
		if ticket.Status == TicketStatusClosed {
			return ErrTicketClosed
		}
		return tx.Delete(attachment).Error
	})
	return attachment, err
}

// GetTicketByID 获取工单（别名）
func GetTicketByID(ctx context.Context, ticketID int) (*Ticket, error) {
	return GetTicket(ctx, ticketID)
}

// MarkTicketUserRead 标记工单用户已读
func MarkTicketUserRead(ctx context.Context, ticketID, userID int) error {
	return ClearTicketUserUnread(ctx, ticketID, userID)
}

// MarkTicketStaffRead 标记工单客服已读
func MarkTicketStaffRead(ctx context.Context, ticketID int) error {
	return ClearTicketStaffUnread(ctx, ticketID)
}

// GetUserUnreadTicketCount 获取用户未读工单数
func GetUserUnreadTicketCount(ctx context.Context, userID int) (int64, error) {
	return CountTicketUserUnread(ctx, userID)
}

// GetStaffUnreadTicketCount 获取客服待处理工单数
func GetStaffUnreadTicketCount(ctx context.Context) (int64, error) {
	return CountTicketStaffPending(ctx)
}

// GetUserTickets 获取用户工单列表
func GetUserTickets(ctx context.Context, userID int, offset, limit int) ([]Ticket, int64, error) {
	filter := TicketFilter{
		UserID: userID,
	}
	return ListTickets(ctx, filter, offset, limit)
}

// GetAllTickets 获取所有工单列表（管理端）
func GetAllTickets(ctx context.Context, offset, limit int) ([]Ticket, int64, error) {
	return ListTickets(ctx, TicketFilter{}, offset, limit)
}
