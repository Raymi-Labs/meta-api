package dto

type DesktopFeedbackContext struct {
	View string `json:"view"`
	Tool string `json:"tool"`
}

type DesktopFeedbackError struct {
	OccurredAt string `json:"occurredAt"`
	Source     string `json:"source"`
	Message    string `json:"message"`
}

type DesktopFeedbackRequest struct {
	ReportID    string                 `json:"reportId"`
	Description string                 `json:"description"`
	AppVersion  string                 `json:"appVersion"`
	Platform    string                 `json:"platform"`
	Context     DesktopFeedbackContext `json:"context"`
	Errors      []DesktopFeedbackError `json:"errors"`
}

type DesktopFeedbackReceipt struct {
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"createdAt"`
}

type DesktopFeedbackDetail struct {
	DesktopFeedbackReceipt
	DesktopFeedbackRequest
	UserID    int   `json:"userId"`
	UpdatedAt int64 `json:"updatedAt"`
}
