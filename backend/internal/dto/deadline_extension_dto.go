package dto

import (
	"github.com/wishwall/wishwall/internal/model"
)

// CreateExtensionRequest 提交延期申请入参（圆梦人，须在当前截止日前提交）。
// 日期为 yyyy-MM-dd 文本，由 service 按业务时区解析（新日期须晚于当前截止日且不超过提交日起 90 天）。
type CreateExtensionRequest struct {
	NewDeadline string `json:"new_deadline" binding:"required,datetime=2006-01-02"`
	Reason      string `json:"reason" binding:"required,min=2,max=500"`
}

// ReviewExtensionRequest 审核延期申请入参（仅心愿发布者）。
type ReviewExtensionRequest struct {
	Note string `json:"note" binding:"omitempty,max=500"`
}

// DeadlineExtensionResponse 延期申请返回结构（详情页展示申请、原因与处理结果）。
type DeadlineExtensionResponse struct {
	ID              uint64  `json:"id"`
	WishID          uint64  `json:"wish_id"`
	ClaimID         uint64  `json:"claim_id"`
	ApplicantID     uint64  `json:"applicant_id"`
	ApplicantName   string  `json:"applicant_name"`
	ReviewerID      uint64  `json:"reviewer_id"`
	CurrentDeadline *string `json:"current_deadline"`
	NewDeadline     string  `json:"new_deadline"`
	Reason          string  `json:"reason"`
	Status          string  `json:"status"`
	StatusText      string  `json:"status_text"`
	ReviewNote      string  `json:"review_note"`
	ReviewedAt      *string `json:"reviewed_at"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// ToDeadlineExtensionResponse 从模型构造返回结构。
func ToDeadlineExtensionResponse(e *model.DeadlineExtension, applicantName, statusText string) DeadlineExtensionResponse {
	resp := DeadlineExtensionResponse{
		ID:            e.ID,
		WishID:        e.WishID,
		ClaimID:       e.ClaimID,
		ApplicantID:   e.ApplicantID,
		ApplicantName: applicantName,
		ReviewerID:    e.ReviewerID,
		NewDeadline:   e.NewDeadline.Format("2006-01-02"),
		Reason:        e.Reason,
		Status:        e.Status,
		StatusText:    statusText,
		ReviewNote:    e.ReviewNote,
		CreatedAt:     e.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:     e.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	if e.CurrentDeadline != nil {
		s := e.CurrentDeadline.Format("2006-01-02")
		resp.CurrentDeadline = &s
	}
	if e.ReviewedAt != nil {
		s := e.ReviewedAt.Format("2006-01-02 15:04:05")
		resp.ReviewedAt = &s
	}
	return resp
}
