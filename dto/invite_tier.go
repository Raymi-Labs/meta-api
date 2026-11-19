package dto

type InviteTierRequest struct {
	Tier         int    `json:"tier"`
	DepartmentId int    `json:"department_id"`
	ExpiresAt    int64  `json:"expires_at"`
	Frozen       bool   `json:"frozen"`
	Approve      bool   `json:"approve"`
	Amount       int64  `json:"amount"`
	Remark       string `json:"remark"`
	NewInviterId int    `json:"new_inviter_id"`
	Name         string `json:"name"`
	Document     string `json:"document"`
	Device       string `json:"device"`
}
type InviteTierStatus struct {
	UserId                 int     `json:"user_id"`
	Enabled                bool    `json:"enabled"`
	Tier                   int     `json:"tier"`
	FirstRate              float64 `json:"first_rate"`
	QualifiedCount         int64   `json:"qualified_count"`
	DirectCount            int64   `json:"direct_count"`
	NextTierRemaining      int64   `json:"next_tier_remaining"`
	MinimumWithdrawalCents int64   `json:"minimum_withdrawal_cents"`
	DepartmentId           int     `json:"department_id"`
	Frozen                 bool    `json:"frozen"`
	UpstreamCut            bool    `json:"upstream_cut"`
	IdentityStatus         string  `json:"identity_status"`
	RequireIdentity        bool    `json:"require_identity"`
	DebtAmount             int64   `json:"debt_amount"`
}
