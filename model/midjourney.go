package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"strings"
)

type Midjourney struct {
	// Like TaskPrivateData.Key, these are main-database-only transport credentials.
	// Keep the original endpoint with the key: a channel edit must not redirect it.
	PollingKey     string `json:"-" gorm:"type:text"`
	PollingBaseURL string `json:"-" gorm:"type:text"`

	// Empty payer columns identify historical personal tasks. Membership IDs identify
	// a membership generation; a later rejoin must not inherit old task authority.
	EnterpriseId               int     `json:"enterprise_id" gorm:"not null;default:0;index"`
	EnterpriseMemberId         int     `json:"-" gorm:"not null;default:0"`
	BillingSource              string  `json:"-" gorm:"type:varchar(24);not null;default:''"`
	BillingRequestId           string  `json:"-" gorm:"type:varchar(96);not null;default:'';index"`
	EnterpriseTaskKey          *string `json:"-" gorm:"type:varchar(96);uniqueIndex"`
	BillingTokenId             int     `json:"-" gorm:"not null;default:0"`
	EnterpriseInitialQuota     int     `json:"-" gorm:"not null;default:0"`
	EnterpriseBillingState     string  `json:"-" gorm:"type:varchar(24);not null;default:'';index"`
	EnterpriseBillingSettled   bool    `json:"-" gorm:"not null;default:false"`
	EnterpriseBillingFinalized bool    `json:"-" gorm:"not null;default:false"`

	Id          int    `json:"id"`
	Code        int    `json:"code"`
	UserId      int    `json:"user_id" gorm:"index"`
	Action      string `json:"action" gorm:"type:varchar(40);index"`
	MjId        string `json:"mj_id" gorm:"index"`
	Prompt      string `json:"prompt"`
	PromptEn    string `json:"prompt_en"`
	Description string `json:"description"`
	State       string `json:"state"`
	SubmitTime  int64  `json:"submit_time" gorm:"index"`
	StartTime   int64  `json:"start_time" gorm:"index"`
	FinishTime  int64  `json:"finish_time" gorm:"index"`
	ImageUrl    string `json:"image_url"`
	VideoUrl    string `json:"video_url"`
	VideoUrls   string `json:"video_urls"`
	Status      string `json:"status" gorm:"type:varchar(20);index"`
	Progress    string `json:"progress" gorm:"type:varchar(30);index"`
	FailReason  string `json:"fail_reason"`
	ChannelId   int    `json:"channel_id"`
	Quota       int    `json:"quota"`
	Buttons     string `json:"buttons"`
	Properties  string `json:"properties"`
}

// TaskQueryParams 用于包含所有搜索条件的结构体，可以根据需求添加更多字段
type TaskQueryParams struct {
	ChannelID      string
	MjID           string
	StartTimestamp string
	EndTimestamp   string
}

func GetAllUserTask(userId int, startIdx int, num int, queryParams TaskQueryParams) []*Midjourney {
	var tasks []*Midjourney
	var err error

	// 初始化查询构建器
	query := DB.Where("user_id = ?", userId)

	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		// 假设您已将前端传来的时间戳转换为数据库所需的时间格式，并处理了时间戳的验证和解析
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetAllTasks(startIdx int, num int, queryParams TaskQueryParams) []*Midjourney {
	var tasks []*Midjourney
	var err error

	// 初始化查询构建器
	query := DB

	// 添加过滤条件
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetAllUnFinishTasks() []*Midjourney {
	var tasks []*Midjourney
	var err error
	// get all tasks progress is not 100%
	err = DB.Where("(enterprise_id = 0 AND progress <> ?) OR (enterprise_id > 0 AND enterprise_billing_state = ? AND status NOT IN ?)", "100%", MJEnterpriseAccepted, []string{"SUCCESS", "FAILURE"}).Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

func GetByOnlyMJId(mjId string) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("mj_id = ?", mjId).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetByMJId(userId int, mjId string) *Midjourney {
	var mj *Midjourney
	var err error
	err = mjSubjectQuery(DB, MJTaskSubject{UserId: userId}).Where("mj_id = ?", mjId).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetByMJIds(userId int, mjIds []string) []*Midjourney {
	var mj []*Midjourney
	var err error
	err = mjSubjectQuery(DB, MJTaskSubject{UserId: userId}).Where("mj_id in (?)", mjIds).Find(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetMjByuId(id int) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("id = ?", id).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func UpdateProgress(id int, progress string) error {
	return DB.Model(&Midjourney{}).Where("id = ?", id).Update("progress", progress).Error
}

func (midjourney *Midjourney) Insert() error {
	var err error
	err = DB.Create(midjourney).Error
	return err
}

func (midjourney *Midjourney) Update() error {
	var err error
	err = DB.Save(midjourney).Error
	return err
}

// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Returns (true, nil) if this caller won the update, (false, nil) if
// another process already moved the task out of fromStatus.
// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Uses Model().Select("*").Updates() to avoid GORM Save()'s INSERT fallback.
func (midjourney *Midjourney) UpdateWithStatus(fromStatus string) (bool, error) {
	if midjourney.EnterpriseId > 0 {
		// Never save a stale billing snapshot over a settlement/reconciliation worker.
		// Terminal states are immutable; notification and polling cannot resurrect work.
		result := DB.Model(&Midjourney{}).Where("id = ? AND status = ? AND status NOT IN ?", midjourney.Id, fromStatus, []string{"SUCCESS", "FAILURE"}).Updates(midjourneyProgressUpdates(midjourney))
		return result.RowsAffected > 0, result.Error
	}

	result := DB.Model(midjourney).Where("status = ?", fromStatus).Select("*").Updates(midjourney)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func MjBulkUpdate(mjIds []string, params map[string]any) error {
	return DB.Model(&Midjourney{}).
		Where("mj_id in (?)", mjIds).
		Updates(params).Error
}

func MjBulkUpdateByTaskIds(taskIDs []int, params map[string]any) error {
	return DB.Model(&Midjourney{}).
		Where("id in (?)", taskIDs).
		Updates(params).Error
}

// CountAllTasks returns total midjourney tasks for admin query
func CountAllTasks(queryParams TaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Midjourney{})
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

// CountAllUserTask returns total midjourney tasks for user
func CountAllUserTask(userId int, queryParams TaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Midjourney{}).Where("user_id = ?", userId)
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

// MJTaskSubject is authorization scope, not a client-supplied filter. Token IDs
// deliberately are not part of task access: rotating a token keeps legitimate access.
type MJTaskSubject struct{ UserId, EnterpriseId, MemberId int }

func mjSubjectQuery(db *gorm.DB, sub MJTaskSubject) *gorm.DB {
	db = db.Where("user_id = ?", sub.UserId)
	if sub.UserId <= 0 || sub.EnterpriseId < 0 || sub.MemberId < 0 {
		return db.Where("1 = 0")
	}
	if sub.EnterpriseId == 0 {
		if sub.MemberId != 0 {
			return db.Where("1 = 0")
		}
		return db.Where("enterprise_id = 0 AND billing_source <> ?", EnterpriseQuotaSource)
	}
	if sub.MemberId == 0 {
		return db.Where("1 = 0")
	}
	return db.Where("enterprise_id = ? AND enterprise_member_id = ? AND billing_source = ?", sub.EnterpriseId, sub.MemberId, EnterpriseQuotaSource)
}

func GetMJTaskForSubject(sub MJTaskSubject, id string) *Midjourney {
	if id == "" {
		return nil
	}
	var tasks []Midjourney
	if err := mjSubjectQuery(DB, sub).Where("mj_id = ?", id).Order("id ASC").Find(&tasks).Error; err != nil || len(tasks) == 0 {
		return nil
	}
	// IDs can collide between upstream accounts. An ID-only API cannot select one
	// safely; refuse ambiguity rather than act under another account's credential.
	for _, task := range tasks[1:] {
		if task.ChannelId != tasks[0].ChannelId || task.PollingKey != tasks[0].PollingKey || task.PollingBaseURL != tasks[0].PollingBaseURL {
			return nil
		}
	}
	return &tasks[0]
}

func GetMJTasksForSubject(sub MJTaskSubject, ids []string) []*Midjourney {
	tasks := make([]*Midjourney, 0)
	if len(ids) == 0 {
		return tasks
	}
	if err := mjSubjectQuery(DB, sub).Where("mj_id IN ?", ids).Find(&tasks).Error; err != nil {
		return nil
	}
	return tasks
}

// Custom-ID-only actions must resolve to a button belonging to a scoped task.
// Never send an arbitrary upstream identifier under the channel credential.
func GetMJTaskForCustomID(sub MJTaskSubject, customID string) *Midjourney {
	if customID == "" {
		return nil
	}
	rows, err := mjSubjectQuery(DB.Model(&Midjourney{}), sub).Where("buttons <> ?", "").Order("id DESC").Rows()
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var task Midjourney
		if DB.ScanRows(rows, &task) != nil {
			return nil
		}
		if task.HasCustomID(customID) {
			return &task
		}
	}
	return nil
}

func (task *Midjourney) HasCustomID(id string) bool {
	var buttons []struct {
		CustomID string `json:"customId"`
	}
	if common.UnmarshalJsonStr(task.Buttons, &buttons) != nil {
		return false
	}
	for _, button := range buttons {
		if button.CustomID == id {
			return true
		}
	}
	return false
}

func midjourneyProgressUpdates(task *Midjourney) map[string]any {
	return map[string]any{"code": task.Code, "progress": task.Progress, "prompt_en": task.PromptEn, "state": task.State,
		"submit_time": task.SubmitTime, "start_time": task.StartTime, "finish_time": task.FinishTime,
		"image_url": task.ImageUrl, "video_url": task.VideoUrl, "video_urls": task.VideoUrls,
		"status": task.Status, "fail_reason": task.FailReason, "properties": task.Properties, "buttons": task.Buttons}
}

// Channel lookup failure is not evidence of a failed enterprise upstream task.
func MjBulkUpdatePersonalByChannel(channelID int, ids []string, updates map[string]any) error {
	return DB.Model(&Midjourney{}).Where("enterprise_id = 0 AND channel_id = ? AND mj_id IN ?", channelID, ids).Updates(updates).Error
}

// MJPollingBatch is one owner and one upstream credential namespace. Do not log
// this object: transport credentials are private and excluded from JSON.
type MJPollingBatch struct {
	ChannelID int                      `json:"-"`
	Subject   MJTaskSubject            `json:"-"`
	TaskIDs   []string                 `json:"-"`
	TasksByID map[string][]*Midjourney `json:"-"`
	key       string
	baseURL   string
}

// BuildMJPollingBatches never guesses enterprise credentials for historical or
// incomplete rows. All duplicates remain present, scoped to their owner/key.
func BuildMJPollingBatches(tasks []*Midjourney) ([]MJPollingBatch, error) {
	batches := make([]MJPollingBatch, 0)
	indexes := make(map[string]int)
	var failures []error
	for _, task := range tasks {
		if task == nil || task.MjId == "" {
			continue
		}
		if task.EnterpriseId > 0 && (task.BillingSource != EnterpriseQuotaSource || task.EnterpriseMemberId <= 0 || task.PollingKey == "" || task.PollingBaseURL == "") {
			failures = append(failures, fmt.Errorf("midjourney task %d: missing trusted polling snapshot", task.Id))
			continue
		}
		scope := fmt.Sprintf("%d:%d:%d:%d:%x", task.ChannelId, task.UserId, task.EnterpriseId, task.EnterpriseMemberId, sha256.Sum256([]byte(task.PollingBaseURL+"\x00"+task.PollingKey)))
		index, exists := indexes[scope]
		if !exists {
			index = len(batches)
			indexes[scope] = index
			batches = append(batches, MJPollingBatch{ChannelID: task.ChannelId, Subject: MJTaskSubject{UserId: task.UserId, EnterpriseId: task.EnterpriseId, MemberId: task.EnterpriseMemberId}, TasksByID: make(map[string][]*Midjourney), key: task.PollingKey, baseURL: task.PollingBaseURL})
		}
		batch := &batches[index]
		if _, exists := batch.TasksByID[task.MjId]; !exists {
			batch.TaskIDs = append(batch.TaskIDs, task.MjId)
		}
		batch.TasksByID[task.MjId] = append(batch.TasksByID[task.MjId], task)
	}
	return batches, errors.Join(failures...)
}

// PollingTransport preserves the historical personal fallback only. Enterprise
// tasks must use the single credential selected when their submission was sent.
func (batch MJPollingBatch) PollingTransport(channel *Channel) (string, string, error) {
	if channel == nil || channel.Id != batch.ChannelID {
		return "", "", errors.New("midjourney polling channel mismatch")
	}
	if batch.key != "" && batch.baseURL != "" {
		return batch.baseURL, batch.key, nil
	}
	if batch.Subject.EnterpriseId > 0 {
		return "", "", errors.New("missing enterprise midjourney polling snapshot")
	}
	return channel.GetBaseURL(), strings.TrimPrefix(channel.Key, "Bearer "), nil
}

// EnterpriseMJTaskIdentity is bounded for cross-database unique indexes and
// includes the upstream credential namespace, payer and owner, not just MJ ID.
func (task *Midjourney) EnterpriseMJTaskIdentity() string {
	raw := fmt.Sprintf("%d:%d:%d:%d:%s\x00%s\x00%s", task.ChannelId, task.UserId, task.EnterpriseId, task.EnterpriseMemberId, task.PollingBaseURL, task.PollingKey, task.MjId)
	return fmt.Sprintf("mj-v2:%x", sha256.Sum256([]byte(raw)))
}
