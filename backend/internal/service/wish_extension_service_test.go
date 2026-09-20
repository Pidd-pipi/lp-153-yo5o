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
	"github.com/wishwall/wishwall/internal/util"
)

func TestWishExtensionService_Submit(t *testing.T) {
	t.Parallel()
	day := 24 * time.Hour
	currentDeadline := time.Now().Add(10 * day)
	newDeadline := time.Now().Add(20 * day)

	baseWish := func() *model.Wish {
		d := currentDeadline
		return &model.Wish{ID: 5, UserID: 1, Status: constants.WishStatusClaimed, ExpectedDeadline: &d}
	}

	tests := []struct {
		name        string
		wish        *model.Wish
		claimUserID uint64
		pendingErr  error
		newDeadline time.Time
		wantErrCode int
	}{
		{
			name:        "submit success",
			wish:        baseWish(),
			claimUserID: 2,
			pendingErr:  notFoundErr(),
			newDeadline: newDeadline,
			wantErrCode: 0,
		},
		{
			name:        "pending extension exists",
			wish:        baseWish(),
			claimUserID: 2,
			pendingErr:  nil, // 已存在一条待审申请
			newDeadline: newDeadline,
			wantErrCode: constants.CodeExtensionPendingExists,
		},
		{
			name:        "new deadline not after current",
			wish:        baseWish(),
			claimUserID: 2,
			pendingErr:  notFoundErr(),
			newDeadline: time.Now().Add(5 * day),
			wantErrCode: constants.CodeExtensionInvalidDate,
		},
		{
			name:        "new deadline beyond ninety days",
			wish:        baseWish(),
			claimUserID: 2,
			pendingErr:  notFoundErr(),
			newDeadline: time.Now().Add(91 * day),
			wantErrCode: constants.CodeExtensionInvalidDate,
		},
		{
			name:        "requester is not fulfiller",
			wish:        baseWish(),
			claimUserID: 9,
			pendingErr:  notFoundErr(),
			newDeadline: newDeadline,
			wantErrCode: constants.CodeExtensionNotFulfiller,
		},
		{
			name: "wish completed not extendable",
			wish: func() *model.Wish {
				w := baseWish()
				w.Status = constants.WishStatusCompleted
				return w
			}(),
			claimUserID: 2,
			pendingErr:  notFoundErr(),
			newDeadline: newDeadline,
			wantErrCode: constants.CodeWishStatusInvalid,
		},
		{
			name: "wish without deadline",
			wish: func() *model.Wish {
				w := baseWish()
				w.ExpectedDeadline = nil
				return w
			}(),
			claimUserID: 2,
			pendingErr:  notFoundErr(),
			newDeadline: newDeadline,
			wantErrCode: constants.CodeExtensionInvalidDate,
		},
		{
			name: "wish deadline passed",
			wish: func() *model.Wish {
				w := baseWish()
				past := time.Now().Add(-day)
				w.ExpectedDeadline = &past
				return w
			}(),
			claimUserID: 2,
			pendingErr:  notFoundErr(),
			newDeadline: newDeadline,
			wantErrCode: constants.CodeExtensionInvalidDate,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			wishRepo := &mockWishRepo{
				findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) { return tt.wish, nil },
			}
			claimRepo := &mockClaimRepo{
				findByWishFn: func(wishID uint64) (*model.WishClaim, error) {
					return &model.WishClaim{ID: 7, WishID: wishID, UserID: tt.claimUserID}, nil
				},
			}
			extRepo := &mockExtensionRepo{
				findPendingByWishFn: func(wishID uint64) (*model.WishExtension, error) {
					if tt.pendingErr != nil {
						return nil, tt.pendingErr
					}
					return &model.WishExtension{ID: 3, WishID: wishID, Status: constants.ExtensionStatusPending}, nil
				},
				createWithTxFn: func(tx *gorm.DB, ext *model.WishExtension) error {
					ext.ID = 11
					return nil
				},
			}
			svc := NewWishExtensionService(&mockTx{}, wishRepo, claimRepo, extRepo, &mockUserRepo{}, &mockAudit{}, testLogger())
			ext, err := svc.Submit(context.Background(), 2, 5, dto.CreateExtensionRequest{
				NewDeadline: tt.newDeadline, Reason: "行程冲突，需要更多时间",
			}, "127.0.0.1", "req-ext-1")
			if tt.wantErrCode == 0 {
				if err != nil {
					t.Fatalf("submit should succeed, got %v", err)
				}
				if ext.Status != constants.ExtensionStatusPending {
					t.Fatalf("expected pending, got %s", ext.Status)
				}
				if ext.OldDeadline == nil || !ext.OldDeadline.Equal(currentDeadline) {
					t.Fatalf("expected old deadline %v, got %v", currentDeadline, ext.OldDeadline)
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

func TestWishExtensionService_Review(t *testing.T) {
	t.Parallel()
	day := 24 * time.Hour
	oldDeadline := time.Now().Add(10 * day)
	newDeadline := time.Now().Add(20 * day)

	newSvc := func(extStatus string, wishUserID uint64, captured **model.Wish) (WishExtensionService, *bool) {
		wishUpdated := false
		wishRepo := &mockWishRepo{
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.Wish, error) {
				d := oldDeadline
				return &model.Wish{ID: id, UserID: wishUserID, Status: constants.WishStatusClaimed, ExpectedDeadline: &d}, nil
			},
			updateWithTxFn: func(tx *gorm.DB, wish *model.Wish) error {
				wishUpdated = true
				*captured = wish
				return nil
			},
		}
		extRepo := &mockExtensionRepo{
			findByIDForUpdateFn: func(tx *gorm.DB, id uint64) (*model.WishExtension, error) {
				return &model.WishExtension{
					ID: id, WishID: 5, ClaimID: 7, UserID: 2,
					OldDeadline: &oldDeadline, NewDeadline: newDeadline,
					Reason: "需要更多时间", Status: extStatus,
				}, nil
			},
			updateWithTxFn: func(tx *gorm.DB, ext *model.WishExtension) error { return nil },
		}
		svc := NewWishExtensionService(&mockTx{}, wishRepo, &mockClaimRepo{}, extRepo, &mockUserRepo{}, &mockAudit{}, testLogger())
		return svc, &wishUpdated
	}

	t.Run("approve updates wish deadline in same tx", func(t *testing.T) {
		var captured *model.Wish
		svc, wishUpdated := newSvc(constants.ExtensionStatusPending, 1, &captured)
		ext, err := svc.Review(context.Background(), 1, 3, dto.ReviewExtensionRequest{Action: constants.ExtensionActionApprove}, "127.0.0.1", "req-ext-2")
		if err != nil {
			t.Fatalf("approve should succeed, got %v", err)
		}
		if ext.Status != constants.ExtensionStatusApproved {
			t.Fatalf("expected approved, got %s", ext.Status)
		}
		if ext.ReviewerID != 1 || ext.ReviewedAt == nil {
			t.Fatalf("expected reviewer recorded, got %+v", ext)
		}
		if !*wishUpdated {
			t.Fatal("expected wish deadline updated in same transaction")
		}
		if captured.ExpectedDeadline == nil || !captured.ExpectedDeadline.Equal(newDeadline) {
			t.Fatalf("expected wish deadline %v, got %v", newDeadline, captured.ExpectedDeadline)
		}
	})

	t.Run("reject keeps wish deadline untouched", func(t *testing.T) {
		var captured *model.Wish
		svc, wishUpdated := newSvc(constants.ExtensionStatusPending, 1, &captured)
		ext, err := svc.Review(context.Background(), 1, 3, dto.ReviewExtensionRequest{Action: constants.ExtensionActionReject}, "127.0.0.1", "req-ext-3")
		if err != nil {
			t.Fatalf("reject should succeed, got %v", err)
		}
		if ext.Status != constants.ExtensionStatusRejected {
			t.Fatalf("expected rejected, got %s", ext.Status)
		}
		if *wishUpdated {
			t.Fatal("reject must not change wish deadline")
		}
	})

	t.Run("only wish owner can review", func(t *testing.T) {
		var captured *model.Wish
		svc, _ := newSvc(constants.ExtensionStatusPending, 1, &captured)
		_, err := svc.Review(context.Background(), 2, 3, dto.ReviewExtensionRequest{Action: constants.ExtensionActionApprove}, "127.0.0.1", "req-ext-4")
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeWishNotOwner {
			t.Fatalf("expected code %d, got %v", constants.CodeWishNotOwner, err)
		}
	})

	t.Run("concurrent review only succeeds once", func(t *testing.T) {
		var captured *model.Wish
		svc, wishUpdated := newSvc(constants.ExtensionStatusApproved, 1, &captured)
		_, err := svc.Review(context.Background(), 1, 3, dto.ReviewExtensionRequest{Action: constants.ExtensionActionApprove}, "127.0.0.1", "req-ext-5")
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeExtensionAlreadyProcessed {
			t.Fatalf("expected code %d, got %v", constants.CodeExtensionAlreadyProcessed, err)
		}
		if *wishUpdated {
			t.Fatal("failed review must not change wish state")
		}
	})
}
