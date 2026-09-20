package dto

import (
	"time"

	"github.com/wishwall/wishwall/internal/model"
)

// CreateExtensionRequest 提交延期申请入参（新完成日期 + 延期原因）。
type CreateExtensionRequest struct {
	NewDeadline time.Time `json:"new_deadline" binding:"required"`
	Reason      string    `json:"reason" binding:"required,min=2,max=500"`
}

// ReviewExtensionRequest 审核延期申请入参（approve / reject）。
type ReviewExtensionRequest struct {
	Action string `json:"action" binding:"required,oneof=approve reject"`
}

// WishExtensionResponse 延期申请返回结构。
type WishExtensionResponse struct {
	ID            uint64  `json:"id"`
	WishID        uint64  `json:"wish_id"`
	ClaimID       uint64  `json:"claim_id"`
	UserID        uint64  `json:"user_id"`
	FulfillerName string  `json:"fulfiller_name"`
	OldDeadline   *string `json:"old_deadline"`
	NewDeadline   string  `json:"new_deadline"`
	Reason        string  `json:"reason"`
	Status        string  `json:"status"`
	ReviewerID    uint64  `json:"reviewer_id"`
	ReviewerName  string  `json:"reviewer_name"`
	ReviewedAt    *string `json:"reviewed_at"`
	CreatedAt     string  `json:"created_at"`
}

// ToWishExtensionResponse 从模型构造返回结构。
func ToWishExtensionResponse(e *model.WishExtension, fulfillerName, reviewerName string) WishExtensionResponse {
	resp := WishExtensionResponse{
		ID:            e.ID,
		WishID:        e.WishID,
		ClaimID:       e.ClaimID,
		UserID:        e.UserID,
		FulfillerName: fulfillerName,
		NewDeadline:   e.NewDeadline.Format("2006-01-02"),
		Reason:        e.Reason,
		Status:        e.Status,
		ReviewerID:    e.ReviewerID,
		ReviewerName:  reviewerName,
		CreatedAt:     e.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if e.OldDeadline != nil {
		s := e.OldDeadline.Format("2006-01-02")
		resp.OldDeadline = &s
	}
	if e.ReviewedAt != nil {
		s := e.ReviewedAt.Format("2006-01-02 15:04:05")
		resp.ReviewedAt = &s
	}
	return resp
}
