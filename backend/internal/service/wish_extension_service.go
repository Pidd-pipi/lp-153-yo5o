package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/wishwall/wishwall/internal/constants"
	"github.com/wishwall/wishwall/internal/dto"
	"github.com/wishwall/wishwall/internal/model"
	"github.com/wishwall/wishwall/internal/repository"
	"github.com/wishwall/wishwall/internal/util"
)

// WishExtensionService 心愿延期申请服务：提交（事务+行锁）、审核（事务+行锁）、列表回读。
type WishExtensionService interface {
	Submit(ctx context.Context, userID, wishID uint64, req dto.CreateExtensionRequest, ip, requestID string) (*model.WishExtension, error)
	Review(ctx context.Context, userID, extensionID uint64, req dto.ReviewExtensionRequest, ip, requestID string) (*model.WishExtension, error)
	ListByWish(wishID uint64) ([]dto.WishExtensionResponse, error)
}

type wishExtensionService struct {
	tx     repository.TxManager
	wish   repository.WishRepository
	claim  repository.WishClaimRepository
	ext    repository.WishExtensionRepository
	user   repository.UserRepository
	audit  AuditService
	logger *slog.Logger
}

// NewWishExtensionService 构造延期申请服务。
func NewWishExtensionService(
	tx repository.TxManager,
	wish repository.WishRepository,
	claim repository.WishClaimRepository,
	ext repository.WishExtensionRepository,
	user repository.UserRepository,
	audit AuditService,
	logger *slog.Logger,
) WishExtensionService {
	return &wishExtensionService{tx: tx, wish: wish, claim: claim, ext: ext, user: user, audit: audit, logger: logger}
}

// Submit 圆梦人提交延期申请：SELECT ... FOR UPDATE 锁定心愿行，保证同一心愿并发提交只产生一条待审申请。
// 新日期须晚于当前截止日，且不超过提交日起九十天。
func (s *wishExtensionService) Submit(ctx context.Context, userID, wishID uint64, req dto.CreateExtensionRequest, ip, requestID string) (*model.WishExtension, error) {
	var created *model.WishExtension
	err := s.tx.Transaction(func(tx *gorm.DB) error {
		wish, err := s.wish.FindByIDForUpdate(tx, wishID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeWishNotFound, constants.MsgWishNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if wish.Status != constants.WishStatusClaimed && wish.Status != constants.WishStatusInProgress {
			return util.NewAppError(constants.CodeWishStatusInvalid, "心愿未被认领或已完成，无法申请延期", errors.New("wish status not extendable"))
		}
		claim, err := s.claim.FindByWishID(wishID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeClaimNotFound, constants.MsgClaimNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if claim.UserID != userID {
			return util.NewAppError(constants.CodeExtensionNotFulfiller, constants.MsgExtensionNotFulfiller, errors.New("extension requester is not the fulfiller"))
		}
		if wish.ExpectedDeadline == nil {
			return util.NewAppError(constants.CodeExtensionInvalidDate, "心愿未设置期望完成时间，无法申请延期", errors.New("wish has no deadline"))
		}
		now := time.Now()
		if !now.Before(*wish.ExpectedDeadline) {
			return util.NewAppError(constants.CodeExtensionInvalidDate, "已超过心愿截止时间，无法申请延期", errors.New("wish deadline passed"))
		}
		if !req.NewDeadline.After(*wish.ExpectedDeadline) {
			return util.NewAppError(constants.CodeExtensionInvalidDate, "新的完成日期必须晚于当前截止时间", errors.New("new deadline not after current deadline"))
		}
		maxDeadline := now.Add(constants.MaxExtensionDays * 24 * time.Hour)
		if req.NewDeadline.After(maxDeadline) {
			return util.NewAppError(constants.CodeExtensionInvalidDate, "新的完成日期不能超过提交日起九十天", errors.New("new deadline beyond max extension days"))
		}
		if _, err := s.ext.FindPendingByWishID(wishID); err == nil {
			return util.NewAppError(constants.CodeExtensionPendingExists, constants.MsgExtensionPendingExists, errors.New("pending extension exists"))
		} else if !errors.Is(err, repository.ErrNotFound) {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		ext := &model.WishExtension{
			WishID:      wishID,
			ClaimID:     claim.ID,
			UserID:      userID,
			OldDeadline: wish.ExpectedDeadline,
			NewDeadline: req.NewDeadline,
			Reason:      req.Reason,
			Status:      constants.ExtensionStatusPending,
		}
		if err := s.ext.CreateWithTx(tx, ext); err != nil {
			if errors.Is(err, repository.ErrConflict) {
				return util.NewAppError(constants.CodeExtensionPendingExists, constants.MsgExtensionPendingExists, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		created = ext
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogExtensionSubmitted, "extension_id", created.ID, "wish_id", wishID, "user_id", userID)
	_ = s.audit.Record(&model.AuditLog{
		UserID: userID, Action: "submit_extension", EntityType: "wish_extension", EntityID: u64str(created.ID),
		Detail: "圆梦人提交延期申请，新截止 " + created.NewDeadline.Format("2006-01-02"), IP: ip, RequestID: requestID,
	})
	return created, nil
}

// Review 心愿发布者审核延期申请：行锁锁定申请与心愿，并发处理只能成功一次；
// 批准时申请状态与心愿截止时间在同一事务更新，任一步失败整体回滚，不改动状态。
func (s *wishExtensionService) Review(ctx context.Context, userID, extensionID uint64, req dto.ReviewExtensionRequest, ip, requestID string) (*model.WishExtension, error) {
	var reviewed *model.WishExtension
	err := s.tx.Transaction(func(tx *gorm.DB) error {
		ext, err := s.ext.FindByIDForUpdate(tx, extensionID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeExtensionNotFound, constants.MsgExtensionNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		wish, err := s.wish.FindByIDForUpdate(tx, ext.WishID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeWishNotFound, constants.MsgWishNotFound, err)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		if wish.UserID != userID {
			return util.NewAppError(constants.CodeWishNotOwner, constants.MsgWishNotOwner, errors.New("extension reviewer is not wish owner"))
		}
		if ext.Status != constants.ExtensionStatusPending {
			return util.NewAppError(constants.CodeExtensionAlreadyProcessed, constants.MsgExtensionAlreadyProcessed, errors.New("extension already processed"))
		}
		now := time.Now()
		ext.ReviewerID = userID
		ext.ReviewedAt = &now
		if req.Action == constants.ExtensionActionApprove {
			ext.Status = constants.ExtensionStatusApproved
			newDeadline := ext.NewDeadline
			wish.ExpectedDeadline = &newDeadline
			if err := s.wish.UpdateWithTx(tx, wish); err != nil {
				return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
			}
		} else {
			ext.Status = constants.ExtensionStatusRejected
		}
		if err := s.ext.UpdateWithTx(tx, ext); err != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
		}
		reviewed = ext
		return nil
	})
	if err != nil {
		return nil, err
	}
	if req.Action == constants.ExtensionActionApprove {
		s.logger.Info(constants.LogExtensionApproved, "extension_id", extensionID, "wish_id", reviewed.WishID, "reviewer_id", userID)
		_ = s.audit.Record(&model.AuditLog{
			UserID: userID, Action: "approve_extension", EntityType: "wish_extension", EntityID: u64str(extensionID),
			Detail: "批准延期申请，心愿截止时间更新为 " + reviewed.NewDeadline.Format("2006-01-02"), IP: ip, RequestID: requestID,
		})
	} else {
		s.logger.Info(constants.LogExtensionRejected, "extension_id", extensionID, "wish_id", reviewed.WishID, "reviewer_id", userID)
		_ = s.audit.Record(&model.AuditLog{
			UserID: userID, Action: "reject_extension", EntityType: "wish_extension", EntityID: u64str(extensionID),
			Detail: "驳回延期申请", IP: ip, RequestID: requestID,
		})
	}
	return reviewed, nil
}

// ListByWish 心愿的延期申请列表（心愿详情页回读复用）。
func (s *wishExtensionService) ListByWish(wishID uint64) ([]dto.WishExtensionResponse, error) {
	exts, err := s.ext.ListByWishID(wishID)
	if err != nil {
		return nil, util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
	}
	items := make([]dto.WishExtensionResponse, 0, len(exts))
	for i := range exts {
		fulfillerName := ""
		if u, uerr := s.user.FindByID(exts[i].UserID); uerr == nil {
			fulfillerName = u.Nickname
		}
		reviewerName := ""
		if exts[i].ReviewerID > 0 {
			if u, uerr := s.user.FindByID(exts[i].ReviewerID); uerr == nil {
				reviewerName = u.Nickname
			}
		}
		items = append(items, dto.ToWishExtensionResponse(&exts[i], fulfillerName, reviewerName))
	}
	return items, nil
}
