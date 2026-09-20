package service

// 真实 PostgreSQL 集成测试：仅当设置环境变量 WISHWALL_TEST_DSN 时运行。
// 验证延期申请完整闭环：提交约束、发布者权限、批准的原子性、并发审核只成功一次、驳回后可重新提交、详情回读。
// 运行：WISHWALL_TEST_DSN="host=127.0.0.1 port=55432 user=wishwall_user dbname=wishwall_test sslmode=disable TimeZone=Asia/Shanghai" go test -run TestExtensionIntegration -v

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/wishwall/wishwall/internal/constants"
	"github.com/wishwall/wishwall/internal/dto"
	"github.com/wishwall/wishwall/internal/model"
	"github.com/wishwall/wishwall/internal/repository"
	"github.com/wishwall/wishwall/internal/util"
)

type extensionIntegrationEnv struct {
	db        *gorm.DB
	wish      repository.WishRepository
	claim     repository.WishClaimRepository
	extension repository.DeadlineExtensionRepository
	user      repository.UserRepository
	tx        repository.TxManager
	svc       DeadlineExtensionService
	wishSvc   WishService
}

func setupExtensionIntegration(t *testing.T) *extensionIntegrationEnv {
	t.Helper()
	dsn := os.Getenv("WISHWALL_TEST_DSN")
	if dsn == "" {
		t.Skip("skip real-db integration test: WISHWALL_TEST_DSN not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.Wish{}, &model.WishClaim{}, &model.DeadlineExtension{},
		&model.Blessing{}, &model.TimeCapsule{}, &model.Badge{}, &model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_extensions_pending_wish
		ON deadline_extensions (wish_id) WHERE status = 'pending'`).Error; err != nil {
		t.Fatalf("partial index: %v", err)
	}

	wishRepo := repository.NewWishRepository(db)
	claimRepo := repository.NewWishClaimRepository(db)
	extRepo := repository.NewDeadlineExtensionRepository(db)
	userRepo := repository.NewUserRepository(db)
	blessRepo := repository.NewBlessingRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)
	txManager := repository.NewTxManager(db)

	auditSvc := NewAuditService(auditRepo, userRepo, testLogger())
	extSvc := NewDeadlineExtensionService(txManager, extRepo, wishRepo, claimRepo, userRepo, auditSvc, testLogger())
	wshSvc := NewWishService(wishRepo, claimRepo, extRepo, blessRepo, userRepo, nil, auditSvc, testLogger())

	t.Cleanup(func() {
		_ = db.Exec("TRUNCATE deadline_extensions, wish_claims, blessings, wishes, users, audit_logs RESTART IDENTITY CASCADE").Error
	})
	return &extensionIntegrationEnv{
		db: db, wish: wishRepo, claim: claimRepo, extension: extRepo, user: userRepo,
		tx: txManager, svc: extSvc, wishSvc: wshSvc,
	}
}

func (e *extensionIntegrationEnv) createUser(t *testing.T, name string) *model.User {
	t.Helper()
	u := &model.User{Username: name, Email: name + "@it.local", PasswordHash: "x", Nickname: name, Role: "user", Status: "active"}
	if err := e.user.Create(u); err != nil {
		t.Fatalf("create user %s: %v", name, err)
	}
	return u
}

// createClaimedWish 创建一条已认领、截止日为 offsetDays 后的心愿。
func (e *extensionIntegrationEnv) createClaimedWish(t *testing.T, ownerID, fulfillerID uint64, offsetDays int) *model.Wish {
	t.Helper()
	deadline := time.Now().In(extensionLocation).AddDate(0, 0, offsetDays)
	w := &model.Wish{
		UserID: ownerID, Title: "集成测试心愿-" + t.Name(), Content: "延期申请闭环集成测试",
		Category: "other", Visibility: "public", Difficulty: "medium",
		ExpectedDeadline: &deadline, Status: constants.WishStatusClaimed,
	}
	if err := e.wish.Create(w); err != nil {
		t.Fatalf("create wish: %v", err)
	}
	c := &model.WishClaim{WishID: w.ID, UserID: fulfillerID, Status: constants.WishStatusClaimed}
	if err := e.claim.Create(c); err != nil {
		t.Fatalf("create claim: %v", err)
	}
	return w
}

func dayOffset(n int) string {
	return time.Now().In(extensionLocation).AddDate(0, 0, n).Format("2006-01-02")
}

func wantCode(t *testing.T, err error, code int) {
	t.Helper()
	var appErr *util.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("want code %d, got %v", code, err)
	}
	if appErr.Code != code {
		t.Fatalf("want code %d, got %d (%v)", code, appErr.Code, err)
	}
}

func TestExtensionIntegration(t *testing.T) {
	env := setupExtensionIntegration(t)
	ctx := context.Background()
	owner := env.createUser(t, "owner_it")
	fulfiller := env.createUser(t, "fulfiller_it")
	other := env.createUser(t, "other_it")

	t.Run("submit constraints and read back", func(t *testing.T) {
		w := env.createClaimedWish(t, owner.ID, fulfiller.ID, 30)

		// 非圆梦人不能提交。
		_, err := env.svc.Submit(ctx, other.ID, w.ID, dto.CreateExtensionRequest{NewDeadline: dayOffset(60), Reason: "理由正当"}, "ip", "r1")
		wantCode(t, err, constants.CodeClaimNotOwner)

		// 新日期不晚于当前截止日。
		_, err = env.svc.Submit(ctx, fulfiller.ID, w.ID, dto.CreateExtensionRequest{NewDeadline: dayOffset(10), Reason: "理由正当"}, "ip", "r1")
		wantCode(t, err, constants.CodeExtensionInvalidDate)

		// 新日期超过提交日起 90 天（当前截止 30 天，新 95 天）。
		_, err = env.svc.Submit(ctx, fulfiller.ID, w.ID, dto.CreateExtensionRequest{NewDeadline: dayOffset(95), Reason: "理由正当"}, "ip", "r1")
		wantCode(t, err, constants.CodeExtensionInvalidDate)

		// 合法提交。
		ext, err := env.svc.Submit(ctx, fulfiller.ID, w.ID, dto.CreateExtensionRequest{NewDeadline: dayOffset(60), Reason: "期末考试周冲突"}, "ip", "r1")
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if ext.Status != constants.ExtensionStatusPending {
			t.Fatalf("status = %s", ext.Status)
		}

		// 同一心愿只能有一条待审申请。
		_, err = env.svc.Submit(ctx, fulfiller.ID, w.ID, dto.CreateExtensionRequest{NewDeadline: dayOffset(70), Reason: "再延一次"}, "ip", "r1")
		wantCode(t, err, constants.CodeExtensionPending)

		// 非发布者不能审核。
		_, err = env.svc.Approve(ctx, fulfiller.ID, ext.ID, dto.ReviewExtensionRequest{}, "ip", "r1")
		wantCode(t, err, constants.CodeWishNotOwner)

		// 详情接口回读最新申请（刷新后仍可回读）。
		detail, err := env.wishSvc.GetByID(w.ID)
		if err != nil {
			t.Fatalf("detail: %v", err)
		}
		if detail.Extension == nil || detail.Extension.ID != ext.ID || detail.Extension.Status != "pending" {
			t.Fatalf("detail extension mismatch: %+v", detail.Extension)
		}
		if detail.Extension.ApplicantName != "fulfiller_it" {
			t.Fatalf("applicant name = %q", detail.Extension.ApplicantName)
		}
	})

	t.Run("approve updates extension and wish atomically", func(t *testing.T) {
		w := env.createClaimedWish(t, owner.ID, fulfiller.ID, 30)
		ext, err := env.svc.Submit(ctx, fulfiller.ID, w.ID, dto.CreateExtensionRequest{NewDeadline: dayOffset(60), Reason: "需要更多时间"}, "ip", "r2")
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if _, err := env.svc.Approve(ctx, owner.ID, ext.ID, dto.ReviewExtensionRequest{Note: "同意"}, "ip", "r2"); err != nil {
			t.Fatalf("approve: %v", err)
		}
		got, err := env.extension.FindByID(ext.ID)
		if err != nil {
			t.Fatalf("reload extension: %v", err)
		}
		if got.Status != constants.ExtensionStatusApproved || got.ReviewerID != owner.ID {
			t.Fatalf("extension = %+v", got)
		}
		wish2, err := env.wish.FindByID(w.ID)
		if err != nil {
			t.Fatalf("reload wish: %v", err)
		}
		wantDay := time.Now().In(extensionLocation).AddDate(0, 0, 60)
		wy, wm, wd := wish2.ExpectedDeadline.In(extensionLocation).Date()
		ty, tm, td := wantDay.Date()
		if wy != ty || wm != tm || wd != td {
			t.Fatalf("wish deadline = %v, want %v", wish2.ExpectedDeadline, wantDay)
		}
		// 已审核后不能重复处理。
		_, err = env.svc.Reject(ctx, owner.ID, ext.ID, dto.ReviewExtensionRequest{}, "ip", "r2")
		wantCode(t, err, constants.CodeExtensionReviewed)
	})

	t.Run("reject then resubmit", func(t *testing.T) {
		w := env.createClaimedWish(t, owner.ID, fulfiller.ID, 30)
		ext, err := env.svc.Submit(ctx, fulfiller.ID, w.ID, dto.CreateExtensionRequest{NewDeadline: dayOffset(45), Reason: "第一次理由不充分"}, "ip", "r3")
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if _, err := env.svc.Reject(ctx, owner.ID, ext.ID, dto.ReviewExtensionRequest{Note: "理由不充分"}, "ip", "r3"); err != nil {
			t.Fatalf("reject: %v", err)
		}
		// 驳回后可重新提交。
		ext2, err := env.svc.Submit(ctx, fulfiller.ID, w.ID, dto.CreateExtensionRequest{NewDeadline: dayOffset(50), Reason: "补充了详细安排"}, "ip", "r3")
		if err != nil {
			t.Fatalf("resubmit after reject: %v", err)
		}
		if ext2.ID == ext.ID {
			t.Fatal("resubmit should create a new extension row")
		}
		// 最新一条为待审申请。
		latest, err := env.extension.FindLatestByWishID(w.ID)
		if err != nil {
			t.Fatalf("latest: %v", err)
		}
		if latest.ID != ext2.ID || latest.Status != constants.ExtensionStatusPending {
			t.Fatalf("latest = %+v", latest)
		}
	})

	t.Run("concurrent approve and reject only one succeeds", func(t *testing.T) {
		w := env.createClaimedWish(t, owner.ID, fulfiller.ID, 30)
		ext, err := env.svc.Submit(ctx, fulfiller.ID, w.ID, dto.CreateExtensionRequest{NewDeadline: dayOffset(60), Reason: "并发审核测试"}, "ip", "r4")
		if err != nil {
			t.Fatalf("submit: %v", err)
		}

		var wg sync.WaitGroup
		var approveErr, rejectErr error
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, approveErr = env.svc.Approve(ctx, owner.ID, ext.ID, dto.ReviewExtensionRequest{}, "ip", "r4-app")
		}()
		go func() {
			defer wg.Done()
			<-start
			_, rejectErr = env.svc.Reject(ctx, owner.ID, ext.ID, dto.ReviewExtensionRequest{}, "ip", "r4-rej")
		}()
		close(start)
		wg.Wait()

		okCount := 0
		if approveErr == nil {
			okCount++
		}
		if rejectErr == nil {
			okCount++
		}
		if okCount != 1 {
			t.Fatalf("expected exactly one success, got approveErr=%v rejectErr=%v", approveErr, rejectErr)
		}
		winner := constants.ExtensionStatusApproved
		losing := rejectErr
		if approveErr != nil {
			winner = constants.ExtensionStatusRejected
			losing = approveErr
		}
		wantCode(t, losing, constants.CodeExtensionReviewed)

		got, err := env.extension.FindByID(ext.ID)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if got.Status != winner {
			t.Fatalf("final status = %s, want %s", got.Status, winner)
		}
		wish2, err := env.wish.FindByID(w.ID)
		if err != nil {
			t.Fatalf("reload wish: %v", err)
		}
		if winner == "approved" {
			wy, wm, wd := wish2.ExpectedDeadline.In(extensionLocation).Date()
			ty, tm, td := time.Now().In(extensionLocation).AddDate(0, 0, 60).Date()
			if wy != ty || wm != tm || wd != td {
				t.Fatalf("approve winner must update deadline, got %v", wish2.ExpectedDeadline)
			}
		} else {
			wy, wm, wd := wish2.ExpectedDeadline.In(extensionLocation).Date()
			ty, tm, td := time.Now().In(extensionLocation).AddDate(0, 0, 30).Date()
			if wy != ty || wm != tm || wd != td {
				t.Fatalf("reject winner must not move deadline, got %v", wish2.ExpectedDeadline)
			}
		}
		fmt.Printf("concurrent review winner=%s\n", winner)
	})

	t.Run("concurrent double submit only one pending survives", func(t *testing.T) {
		w := env.createClaimedWish(t, owner.ID, fulfiller.ID, 30)
		var wg sync.WaitGroup
		errs := make([]error, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-start
				_, errs[idx] = env.svc.Submit(ctx, fulfiller.ID, w.ID, dto.CreateExtensionRequest{NewDeadline: dayOffset(60), Reason: "并发提交"}, "ip", fmt.Sprintf("r5-%d", idx))
			}(i)
		}
		close(start)
		wg.Wait()
		pending, err := env.extension.CountByWishID(w.ID)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if pending != 1 {
			t.Fatalf("want exactly one extension row, got %d (errs %v %v)", pending, errs[0], errs[1])
		}
		successes := 0
		for _, e := range errs {
			if e == nil {
				successes++
			} else {
				wantCode(t, e, constants.CodeExtensionPending)
			}
		}
		if successes != 1 {
			t.Fatalf("want one success, got %d", successes)
		}
	})
}
