package model

import "time"

// WishExtension 心愿延期申请实体。状态机：pending -> approved / rejected（驳回后可重新提交）。
// 同一心愿仅允许一条待审申请（wish_id 部分唯一索引兜底并发）。
type WishExtension struct {
	ID          uint64     `gorm:"primaryKey" json:"id"`
	WishID      uint64     `gorm:"index;uniqueIndex:uk_extension_pending_wish,where:status = 'pending';not null" json:"wish_id"`
	ClaimID     uint64     `gorm:"index;not null" json:"claim_id"`
	UserID      uint64     `gorm:"index;not null" json:"user_id"`
	OldDeadline *time.Time `json:"old_deadline"`
	NewDeadline time.Time  `gorm:"not null" json:"new_deadline"`
	Reason      string     `gorm:"type:text;not null" json:"reason"`
	Status      string     `gorm:"size:20;index;not null;default:pending" json:"status"`
	ReviewerID  uint64     `gorm:"not null;default:0" json:"reviewer_id"`
	ReviewedAt  *time.Time `json:"reviewed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
