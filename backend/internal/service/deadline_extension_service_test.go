package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/wishwall/wishwall/internal/constants"
	"github.com/wishwall/wishwall/internal/dto"
	"github.com/wishwall/wishwall/internal/model"
	"github.com/wishwall/wishwall/internal/repository"
	"github.com/wishwall/wishwall/internal/util"
)

func newExtensionWishes() (current time.Time, future time.Time, today string) {
	loc := extensionLocation
	now := time.Now().In(loc)
	current = truncateDay(now).AddDate(0, 0, 30)
	future = truncateDay(now).AddDate(0, 0, 60)
	today = truncateDay(now).Format("2006-01-02")
	return
}

func TestDeadlineExtensionService_Submit(t *testing.T) {
	t.Parallel()
	current, future, today := newExtensionWishes()

	tests := []struct {
		name            string
		userID          uint64
		pendingErr      error
		pendingExt      *model.DeadlineExtension
		wantErrCode     int
		newDeadline     string
		currentDeadline *time.Time
		wishStatus      string
		claimUserID     uint64
		createErr       error
	}{
		{name: "submit success", userID: 2, pendingErr: repository.ErrNotFound, wantErrCode: 0,
			newDeadline: future.Format("2006-01-02"), currentDeadline: &current, wishStatus: constants.WishStatusInProgress, claimUserID: 2},
		{name: "pending already exists", userID: 2, pendingExt: &model.DeadlineExtension{ID: 3, Status: constants.ExtensionStatusPending},
			wantErrCode: constants.CodeExtensionPending,
			newDeadline: future.Format("2006-01-02"), currentDeadline: &current, wishStatus: constants.WishStatusClaimed, claimUserID: 2},
		{name: "new date must be after current deadline", userID: 2, pendingErr: repository.ErrNotFound,
			wantErrCode: constants.CodeExtensionInvalidDate,
			newDeadline: today, currentDeadline: &current, wishStatus: constants.WishStatusClaimed, claimUserID: 2},
		{name: "new date beyond 90 days", userID: 2, pendingErr: repository.ErrNotFound,
			wantErrCode:     constants.CodeExtensionInvalidDate,
			newDeadline:     truncateDay(time.Now().In(extensionLocation)).AddDate(0, 0, 91).Format("2006-01-02"),
			currentDeadline: &current, wishStatus: constants.WishStatusClaimed, claimUserID: 2},
		{name: "only fulfiller can submit", userID: 3, pendingErr: repository.ErrNotFound,
			wantErrCode: constants.CodeClaimNotOwner,
			newDeadline: future.Format("2006-01-02"), currentDeadline: &current, wishStatus: constants.WishStatusClaimed, claimUserID: 2},
		{name: "concurrent unique conflict", userID: 2, pendingErr: repository.ErrNotFound, createErr: repository.ErrConflict,
			wantErrCode: constants.CodeExtensionPending,
			newDeadline: future.Format("2006-01-02"), currentDeadline: &current, wishStatus: constants.WishStatusClaimed, claimUserID: 2},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			wishRepo := &mockWishRepo{
				findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
					return &model.Wish{ID: id, UserID: 1, Status: tt.wishStatus, ExpectedDeadline: tt.currentDeadline}, nil
				},
				updateWithTxFn: func(tx *gorm.DB, w *model.Wish) error { return nil },
			}
			claimRepo := &mockClaimRepo{
				findByWishFn: func(wishID uint64) (*model.WishClaim, error) {
					return &model.WishClaim{ID: 9, WishID: wishID, UserID: tt.claimUserID}, nil
				},
			}
			extRepo := &mockExtensionRepo{
				findPendingForUpdFn: func(tx *gorm.DB, wishID uint64) (*model.DeadlineExtension, error) {
					if tt.pendingExt != nil {
						return tt.pendingExt, nil
					}
					return nil, tt.pendingErr
				},
				createWithTxFn: func(tx *gorm.DB, ext *model.DeadlineExtension) error {
					if tt.createErr != nil {
						return tt.createErr
					}
					ext.ID = 11
					return nil
				},
			}
			svc := NewDeadlineExtensionService(&mockTx{}, extRepo, wishRepo, claimRepo, &mockUserRepo{}, &mockAudit{}, testLogger())
			ext, err := svc.Submit(context.Background(), tt.userID, 5,
				dto.CreateExtensionRequest{NewDeadline: tt.newDeadline, Reason: "期末考试周，需要延后"}, "127.0.0.1", "req-ext")
			if tt.wantErrCode == 0 {
				if err != nil {
					t.Fatalf("submit should succeed, got %v", err)
				}
				if ext.Status != constants.ExtensionStatusPending || ext.WishID != 5 {
					t.Fatalf("unexpected ext: %+v", ext)
				}
				return
			}
			var appErr *util.AppError
			if !errors.As(err, &appErr) || appErr.Code != tt.wantErrCode {
				t.Fatalf("expected code %d, got %v", tt.wantErrCode, err)
			}
		})
	}
}

func TestDeadlineExtensionService_Review(t *testing.T) {
	t.Parallel()
	_, future, _ := newExtensionWishes()

	t.Run("approve success updates wish deadline atomically", func(t *testing.T) {
		t.Parallel()
		deadlineUpdated := false
		wishRepo := &mockWishRepo{
			findByIDFn: func(id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusInProgress}, nil
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1, Status: constants.WishStatusInProgress}, nil
			},
			updateWithTxFn: func(tx *gorm.DB, w *model.Wish) error {
				deadlineUpdated = true
				if !w.ExpectedDeadline.Equal(future) {
					t.Fatalf("deadline = %v, want %v", w.ExpectedDeadline, future)
				}
				return nil
			},
		}
		extRepo := &mockExtensionRepo{
			findByIDFn: func(id uint64) (*model.DeadlineExtension, error) {
				return &model.DeadlineExtension{ID: id, WishID: 5, Status: constants.ExtensionStatusPending, NewDeadline: future}, nil
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.DeadlineExtension, error) {
				return &model.DeadlineExtension{ID: id, WishID: 5, Status: constants.ExtensionStatusPending, NewDeadline: future}, nil
			},
			reviewWithTxFn: func(tx *gorm.DB, id uint64, status string, reviewerID uint64, note string, reviewedAt time.Time) (int64, error) {
				if status != constants.ExtensionStatusApproved {
					t.Fatalf("status = %s", status)
				}
				return 1, nil
			},
		}
		svc := NewDeadlineExtensionService(&mockTx{}, extRepo, wishRepo, &mockClaimRepo{}, &mockUserRepo{}, &mockAudit{}, testLogger())
		ext, err := svc.Approve(context.Background(), 1, 11, dto.ReviewExtensionRequest{Note: "同意"}, "127.0.0.1", "req-app")
		if err != nil {
			t.Fatalf("approve: %v", err)
		}
		if ext.Status != constants.ExtensionStatusApproved || !deadlineUpdated {
			t.Fatalf("approve did not update state: %+v updated=%v", ext, deadlineUpdated)
		}
	})

	t.Run("concurrent review loses: rows affected 0, no state change", func(t *testing.T) {
		t.Parallel()
		wishUpdated := false
		wishRepo := &mockWishRepo{
			findByIDFn: func(id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1}, nil
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1}, nil
			},
			updateWithTxFn: func(tx *gorm.DB, w *model.Wish) error { wishUpdated = true; return nil },
		}
		extRepo := &mockExtensionRepo{
			findByIDFn: func(id uint64) (*model.DeadlineExtension, error) {
				return &model.DeadlineExtension{ID: id, WishID: 5, Status: constants.ExtensionStatusPending}, nil
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.DeadlineExtension, error) {
				return &model.DeadlineExtension{ID: id, WishID: 5, Status: constants.ExtensionStatusPending}, nil
			},
			reviewWithTxFn: func(tx *gorm.DB, id uint64, status string, reviewerID uint64, note string, reviewedAt time.Time) (int64, error) {
				return 0, nil
			},
		}
		svc := NewDeadlineExtensionService(&mockTx{}, extRepo, wishRepo, &mockClaimRepo{}, &mockUserRepo{}, &mockAudit{}, testLogger())
		_, err := svc.Approve(context.Background(), 1, 11, dto.ReviewExtensionRequest{}, "127.0.0.1", "req-race")
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeExtensionReviewed {
			t.Fatalf("expected CodeExtensionReviewed, got %v", err)
		}
		if wishUpdated {
			t.Fatal("wish deadline must not change when concurrent review loses")
		}
	})

	t.Run("non owner cannot review", func(t *testing.T) {
		t.Parallel()
		wishRepo := &mockWishRepo{
			findByIDFn: func(id uint64) (*model.Wish, error) { return &model.Wish{ID: id, UserID: 1}, nil },
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1}, nil
			},
		}
		extRepo := &mockExtensionRepo{
			findByIDFn: func(id uint64) (*model.DeadlineExtension, error) {
				return &model.DeadlineExtension{ID: id, WishID: 5, Status: constants.ExtensionStatusPending}, nil
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.DeadlineExtension, error) {
				return &model.DeadlineExtension{ID: id, WishID: 5, Status: constants.ExtensionStatusPending}, nil
			},
		}
		svc := NewDeadlineExtensionService(&mockTx{}, extRepo, wishRepo, &mockClaimRepo{}, &mockUserRepo{}, &mockAudit{}, testLogger())
		_, err := svc.Reject(context.Background(), 2, 11, dto.ReviewExtensionRequest{}, "127.0.0.1", "req-forbid")
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeWishNotOwner {
			t.Fatalf("expected CodeWishNotOwner, got %v", err)
		}
	})

	t.Run("already reviewed rejected", func(t *testing.T) {
		t.Parallel()
		wishRepo := &mockWishRepo{
			findByIDFn: func(id uint64) (*model.Wish, error) { return &model.Wish{ID: id, UserID: 1}, nil },
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				return &model.Wish{ID: id, UserID: 1}, nil
			},
		}
		extRepo := &mockExtensionRepo{
			findByIDFn: func(id uint64) (*model.DeadlineExtension, error) {
				return &model.DeadlineExtension{ID: id, WishID: 5, Status: constants.ExtensionStatusApproved}, nil
			},
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.DeadlineExtension, error) {
				return &model.DeadlineExtension{ID: id, WishID: 5, Status: constants.ExtensionStatusApproved}, nil
			},
		}
		svc := NewDeadlineExtensionService(&mockTx{}, extRepo, wishRepo, &mockClaimRepo{}, &mockUserRepo{}, &mockAudit{}, testLogger())
		_, err := svc.Reject(context.Background(), 1, 11, dto.ReviewExtensionRequest{}, "127.0.0.1", "req-twice")
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeExtensionReviewed {
			t.Fatalf("expected CodeExtensionReviewed, got %v", err)
		}
	})
}
