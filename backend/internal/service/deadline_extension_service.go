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

// DeadlineExtensionService 心愿延期申请服务：提交/批准/驳回/查询。
// 所有写操作在事务中执行，并发审核通过行锁 + 条件更新保证只能成功一次。
type DeadlineExtensionService interface {
	Submit(ctx context.Context, userID, wishID uint64, req dto.CreateExtensionRequest, ip, requestID string) (*model.DeadlineExtension, error)
	Approve(ctx context.Context, userID, extensionID uint64, req dto.ReviewExtensionRequest, ip, requestID string) (*model.DeadlineExtension, error)
	Reject(ctx context.Context, userID, extensionID uint64, req dto.ReviewExtensionRequest, ip, requestID string) (*model.DeadlineExtension, error)
	ListByWish(userID, wishID uint64, q dto.PageQuery) (*dto.PageResult, error)
	GetByID(extensionID uint64) (*model.DeadlineExtension, error)
}

type deadlineExtensionService struct {
	tx        repository.TxManager
	extension repository.DeadlineExtensionRepository
	wish      repository.WishRepository
	claim     repository.WishClaimRepository
	user      repository.UserRepository
	audit     AuditService
	logger    *slog.Logger
}

// NewDeadlineExtensionService 构造延期申请服务。
func NewDeadlineExtensionService(
	tx repository.TxManager,
	extension repository.DeadlineExtensionRepository,
	wish repository.WishRepository,
	claim repository.WishClaimRepository,
	user repository.UserRepository,
	audit AuditService,
	logger *slog.Logger,
) DeadlineExtensionService {
	return &deadlineExtensionService{tx: tx, extension: extension, wish: wish, claim: claim, user: user, audit: audit, logger: logger}
}

// extensionLocation 业务时区：与数据库 DSN 的 TimeZone=Asia/Shanghai 保持一致。
var extensionLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}()

// Submit 圆梦人提交延期申请。
func (s *deadlineExtensionService) Submit(ctx context.Context, userID, wishID uint64, req dto.CreateExtensionRequest, ip, requestID string) (*model.DeadlineExtension, error) {
	newDeadline, err := time.ParseInLocation("2006-01-02", req.NewDeadline, extensionLocation)
	if err != nil {
		return nil, util.NewAppError(constants.CodeValidationFailed, constants.MsgParamInvalid+"：new_deadline 日期格式非法", err)
	}

	var created *model.DeadlineExtension
	err = s.tx.Transaction(func(tx *gorm.DB) error {
		// 锁定心愿行：串行化同一心愿上的申请提交与审核。
		wish, ferr := s.wish.FindByIDForUpdate(tx, wishID)
		if ferr != nil {
			if errors.Is(ferr, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeWishNotFound, constants.MsgWishNotFound, ferr)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, ferr)
		}
		// 只有该心愿的圆梦人（认领者）才能提交。
		claim, cerr := s.claim.FindByWishID(wishID)
		if cerr != nil {
			if errors.Is(cerr, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeClaimNotFound, constants.MsgExtensionNotAllowed+"：心愿尚未被认领", cerr)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, cerr)
		}
		if claim.UserID != userID {
			return util.NewAppError(constants.CodeClaimNotOwner, constants.MsgExtensionOnlyFulfiller, errors.New("extension applicant is not claim owner"))
		}
		if wish.Status == constants.WishStatusCompleted {
			return util.NewAppError(constants.CodeExtensionNotAllowed, constants.MsgExtensionNotAllowed+"：心愿已完成", errors.New("wish already completed"))
		}
		if wish.ExpectedDeadline == nil {
			return util.NewAppError(constants.CodeExtensionNotAllowed, constants.MsgExtensionNoDeadline, errors.New("wish has no deadline"))
		}
		// 同一心愿只能有一条待审申请（锁定该心愿的待审行，部分唯一索引兜底）。
		pending, perr := s.extension.FindPendingByWishIDForUpdate(tx, wishID)
		if perr != nil && !errors.Is(perr, repository.ErrNotFound) {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, perr)
		}
		if pending != nil {
			return util.NewAppError(constants.CodeExtensionPending, constants.MsgExtensionPending, errors.New("pending extension exists"))
		}
		today := time.Now().In(extensionLocation)
		currentDay := truncateDay(wish.ExpectedDeadline.In(extensionLocation))
		newDay := truncateDay(newDeadline)
		todayDay := truncateDay(today)
		maxDay := todayDay.AddDate(0, 0, constants.ExtensionMaxDays)
		// 须在截止前提交；新日期须晚于当前截止日且不超过提交日起 90 天。
		if currentDay.Before(todayDay) {
			return util.NewAppError(constants.CodeExtensionNotAllowed, constants.MsgExtensionBeforeDeadline, errors.New("current deadline already passed"))
		}
		if !newDay.After(currentDay) || newDay.After(maxDay) {
			return util.NewAppError(constants.CodeExtensionInvalidDate, constants.MsgExtensionInvalidDate, errors.New("new deadline out of allowed range"))
		}

		current := *wish.ExpectedDeadline
		ext := &model.DeadlineExtension{
			WishID:          wishID,
			ClaimID:         claim.ID,
			ApplicantID:     userID,
			CurrentDeadline: &current,
			NewDeadline:     newDay,
			Reason:          req.Reason,
			Status:          constants.ExtensionStatusPending,
		}
		if cerr := s.extension.CreateWithTx(tx, ext); cerr != nil {
			if errors.Is(cerr, repository.ErrConflict) {
				return util.NewAppError(constants.CodeExtensionPending, constants.MsgExtensionPending, cerr)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, cerr)
		}
		created = ext
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogExtensionSubmitted, "extension_id", created.ID, "wish_id", wishID, "user_id", userID, "new_deadline", created.NewDeadline.Format("2006-01-02"))
	_ = s.audit.Record(&model.AuditLog{
		UserID: userID, Action: "submit_extension", EntityType: "deadline_extension", EntityID: u64str(created.ID),
		Detail: "提交延期申请，期望新截止日 " + created.NewDeadline.Format("2006-01-02") + "，原因：" + util.TruncateString(req.Reason, 50),
		IP:     ip, RequestID: requestID,
	})
	return created, nil
}

// Approve 心愿发布者批准延期：申请状态与心愿截止时间在同一事务内同时更新。
func (s *deadlineExtensionService) Approve(ctx context.Context, userID, extensionID uint64, req dto.ReviewExtensionRequest, ip, requestID string) (*model.DeadlineExtension, error) {
	return s.review(ctx, userID, extensionID, req, true, ip, requestID)
}

// Reject 心愿发布者驳回延期：驳回后圆梦人可重新提交。
func (s *deadlineExtensionService) Reject(ctx context.Context, userID, extensionID uint64, req dto.ReviewExtensionRequest, ip, requestID string) (*model.DeadlineExtension, error) {
	return s.review(ctx, userID, extensionID, req, false, ip, requestID)
}

func (s *deadlineExtensionService) review(ctx context.Context, userID, extensionID uint64, req dto.ReviewExtensionRequest, approve bool, ip, requestID string) (*model.DeadlineExtension, error) {
	targetStatus := constants.ExtensionStatusRejected
	action := "reject_extension"
	logTpl := constants.LogExtensionRejected
	successDetail := "驳回延期申请"
	if approve {
		targetStatus = constants.ExtensionStatusApproved
		action = "approve_extension"
		logTpl = constants.LogExtensionApproved
		successDetail = "批准延期申请，心愿截止时间同步更新"
	}

	var reviewed *model.DeadlineExtension
	err := s.tx.Transaction(func(tx *gorm.DB) error {
		// 先非锁定读取申请拿到 wish_id，再统一按“心愿行 -> 申请行”顺序加锁，
		// 与 Submit 的加锁顺序一致，避免并发提交/审核交叉等待死锁。
		ext0, ferr := s.extension.FindByID(extensionID)
		if ferr != nil {
			if errors.Is(ferr, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeExtensionNotFound, constants.MsgExtensionNotFound, ferr)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, ferr)
		}
		wish, werr := s.wish.FindByIDForUpdate(tx, ext0.WishID)
		if werr != nil {
			if errors.Is(werr, repository.ErrNotFound) {
				return util.NewAppError(constants.CodeWishNotFound, constants.MsgWishNotFound, werr)
			}
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, werr)
		}
		ext, lerr := s.extension.FindByIDForUpdate(tx, extensionID)
		if lerr != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, lerr)
		}
		// 只有心愿发布者能批准或驳回。
		if wish.UserID != userID {
			return util.NewAppError(constants.CodeWishNotOwner, constants.MsgExtensionOnlyOwner, errors.New("extension reviewer is not wish owner"))
		}
		// 并发审核只能成功一次：已处理（含另一事务抢先批准/驳回）直接失败，不得改动状态。
		if ext.Status != constants.ExtensionStatusPending {
			return util.NewAppError(constants.CodeExtensionReviewed, constants.MsgExtensionReviewed, errors.New("extension already reviewed: "+ext.Status))
		}
		if approve && wish.Status == constants.WishStatusCompleted {
			return util.NewAppError(constants.CodeExtensionNotAllowed, constants.MsgExtensionNotAllowed+"：心愿已完成", errors.New("wish already completed"))
		}

		now := time.Now()
		rows, rerr := s.extension.ReviewWithTx(tx, extensionID, targetStatus, userID, req.Note, now)
		if rerr != nil {
			return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, rerr)
		}
		if rows != 1 {
			// 条件更新未命中：并发对端已处理，返回冲突，心愿截止时间不被修改。
			s.logger.Warn(constants.LogExtensionConflict, "extension_id", extensionID, "reviewer_id", userID)
			return util.NewAppError(constants.CodeExtensionReviewed, constants.MsgExtensionReviewed, errors.New("concurrent review lost"))
		}
		if approve {
			// 与申请状态在同一事务内同时更新，任一失败整体回滚。
			wish.ExpectedDeadline = &ext.NewDeadline
			if uerr := s.wish.UpdateWithTx(tx, wish); uerr != nil {
				return util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, uerr)
			}
		}
		ext.Status = targetStatus
		ext.ReviewerID = userID
		ext.ReviewNote = req.Note
		ext.ReviewedAt = &now
		reviewed = ext
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(logTpl, "extension_id", extensionID, "wish_id", reviewed.WishID, "reviewer_id", userID)
	_ = s.audit.Record(&model.AuditLog{
		UserID: userID, Action: action, EntityType: "deadline_extension", EntityID: u64str(extensionID),
		Detail: successDetail, IP: ip, RequestID: requestID,
	})
	return reviewed, nil
}

// assertNoPendingLocked 已移除：待审校验在 Submit 事务内通过 FindPendingByWishIDForUpdate 内联完成。

func truncateDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func (s *deadlineExtensionService) ListByWish(userID, wishID uint64, q dto.PageQuery) (*dto.PageResult, error) {
	page, size := q.Normalize()
	if _, err := s.wish.FindByID(wishID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeWishNotFound, constants.MsgWishNotFound, err)
		}
		return nil, util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
	}
	total, err := s.extension.CountByWishID(wishID)
	if err != nil {
		return nil, util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
	}
	exts, err := s.extension.ListByWishID(wishID, (page-1)*size, size)
	if err != nil {
		return nil, util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
	}
	items := make([]dto.DeadlineExtensionResponse, 0, len(exts))
	for i := range exts {
		items = append(items, s.toResponse(&exts[i]))
	}
	return &dto.PageResult{Items: items, Total: total, Page: page, PageSize: size}, nil
}

func (s *deadlineExtensionService) GetByID(extensionID uint64) (*model.DeadlineExtension, error) {
	ext, err := s.extension.FindByID(extensionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeExtensionNotFound, constants.MsgExtensionNotFound, err)
		}
		return nil, util.NewAppError(constants.CodeInternalError, constants.MsgInternalError, err)
	}
	return ext, nil
}

// toResponse 组装申请人昵称与状态文本（详情页与列表复用）。
func (s *deadlineExtensionService) toResponse(e *model.DeadlineExtension) dto.DeadlineExtensionResponse {
	name := ""
	if u, err := s.user.FindByID(e.ApplicantID); err == nil {
		name = u.Nickname
	}
	return dto.ToDeadlineExtensionResponse(e, name, constants.ExtensionStatusText(e.Status))
}
