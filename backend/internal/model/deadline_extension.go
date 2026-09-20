package model

import "time"

// DeadlineExtension 心愿延期申请实体。
// 状态机：pending（待审） -> approved（批准）/ rejected（驳回）。
// 业务约束：同一心愿同时只允许一条 pending 申请（部分唯一索引 idx_ext_pending_wish 兜底并发）。
type DeadlineExtension struct {
	ID              uint64     `gorm:"primaryKey" json:"id"`
	WishID          uint64     `gorm:"index;not null" json:"wish_id"`
	ClaimID         uint64     `gorm:"index;not null" json:"claim_id"`
	ApplicantID     uint64     `gorm:"index;not null" json:"applicant_id"`
	ReviewerID      uint64     `gorm:"not null;default:0" json:"reviewer_id"`
	CurrentDeadline *time.Time `gorm:"column:current_deadline" json:"current_deadline"`
	NewDeadline     time.Time  `gorm:"column:new_deadline;not null;index" json:"new_deadline"`
	Reason          string     `gorm:"type:text;not null" json:"reason"`
	Status          string     `gorm:"size:20;index;not null;default:pending" json:"status"`
	ReviewNote      string     `gorm:"type:text;not null;default:''" json:"review_note"`
	ReviewedAt      *time.Time `json:"reviewed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
