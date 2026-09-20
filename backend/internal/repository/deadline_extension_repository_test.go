package repository

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestExtensionRepository_ReviewWithTx 条件审核更新：WHERE status='pending' 保证并发审核只有一方命中 1 行，
// 落败方 RowsAffected=0 且不产生任何状态变更（配合 service 层事务，心愿截止时间也不会被改）。
func TestExtensionRepository_ReviewWithTx(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		rows int64
	}{
		{name: "first review wins one row", rows: 1},
		{name: "concurrent review loses zero rows", rows: 0},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gdb, mock := newMockDB(t)
			mock.ExpectBegin()
			mock.ExpectExec(regexp.QuoteMeta(`UPDATE "deadline_extensions" SET "review_note"=$1,"reviewed_at"=$2,"reviewer_id"=$3,"status"=$4,"updated_at"=$5 WHERE id = $6 AND status = $7`)).
				WithArgs("同意", now, uint64(1), "approved", sqlmock.AnyArg(), uint64(11), "pending").
				WillReturnResult(sqlmock.NewResult(0, tt.rows))
			mock.ExpectCommit()

			repo := NewDeadlineExtensionRepository(gdb)
			affected, err := repo.ReviewWithTx(gdb, 11, "approved", 1, "同意", now)
			if err != nil {
				t.Fatalf("review with tx: %v", err)
			}
			if affected != tt.rows {
				t.Fatalf("rows affected = %d, want %d", affected, tt.rows)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations: %v", err)
			}
		})
	}
}

// TestExtensionRepository_FindLatestByWishID 详情页回读最新一条申请。
func TestExtensionRepository_FindLatestByWishID(t *testing.T) {
	t.Parallel()
	gdb, mock := newMockDB(t)
	rows := sqlmock.NewRows([]string{"id", "wish_id", "claim_id", "applicant_id", "reviewer_id",
		"current_deadline", "new_deadline", "reason", "status", "review_note", "reviewed_at", "created_at", "updated_at"}).
		AddRow(2, 5, 9, 2, 1, nil, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			"期末周", "approved", "", nil, now20260920(), now20260920())
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "deadline_extensions" WHERE wish_id = $1 ORDER BY created_at DESC, id DESC,"deadline_extensions"."id" LIMIT $2`)).
		WithArgs(uint64(5), 1).WillReturnRows(rows)

	repo := NewDeadlineExtensionRepository(gdb)
	ext, err := repo.FindLatestByWishID(5)
	if err != nil {
		t.Fatalf("find latest: %v", err)
	}
	if ext.ID != 2 || ext.Status != "approved" {
		t.Fatalf("unexpected ext: %+v", ext)
	}
}

func now20260920() time.Time { return time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC) }
