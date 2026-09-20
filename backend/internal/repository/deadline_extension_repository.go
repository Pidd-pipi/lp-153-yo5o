package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/wishwall/wishwall/internal/model"
)

// DeadlineExtensionRepository 心愿延期申请仓储接口。
type DeadlineExtensionRepository interface {
	CreateWithTx(tx *gorm.DB, ext *model.DeadlineExtension) error
	FindByID(id uint64) (*model.DeadlineExtension, error)
	FindByIDForUpdate(tx *gorm.DB, id uint64) (*model.DeadlineExtension, error)
	FindLatestByWishID(wishID uint64) (*model.DeadlineExtension, error)
	FindPendingByWishIDForUpdate(tx *gorm.DB, wishID uint64) (*model.DeadlineExtension, error)
	ListByWishID(wishID uint64, offset, limit int) ([]model.DeadlineExtension, error)
	CountByWishID(wishID uint64) (int64, error)
	UpdateWithTx(tx *gorm.DB, ext *model.DeadlineExtension) error
	// ReviewWithTx 条件审核：仅当申请仍为 pending 时更新成功（RowsAffected=1）。
	// 并发审核（批准/驳回）只能成功一次，失败方 RowsAffected=0 且不改任何状态。
	ReviewWithTx(tx *gorm.DB, id uint64, status string, reviewerID uint64, note string, reviewedAt time.Time) (int64, error)
}

type deadlineExtensionRepository struct {
	db *gorm.DB
}

// NewDeadlineExtensionRepository 构造延期申请仓储。
func NewDeadlineExtensionRepository(db *gorm.DB) DeadlineExtensionRepository {
	return &deadlineExtensionRepository{db: db}
}

func (r *deadlineExtensionRepository) CreateWithTx(tx *gorm.DB, ext *model.DeadlineExtension) error {
	if err := tx.Create(ext).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create pending extension on wish %d: %w", ext.WishID, ErrConflict)
		}
		return fmt.Errorf("create extension on wish %d: %w", ext.WishID, err)
	}
	return nil
}

func (r *deadlineExtensionRepository) FindByID(id uint64) (*model.DeadlineExtension, error) {
	var ext model.DeadlineExtension
	if err := r.db.First(&ext, id).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find extension by id %d: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("find extension by id %d: %w", id, err)
	}
	return &ext, nil
}

// FindByIDForUpdate 行级锁读取，用于并发审核场景。
func (r *deadlineExtensionRepository) FindByIDForUpdate(tx *gorm.DB, id uint64) (*model.DeadlineExtension, error) {
	var ext model.DeadlineExtension
	if err := tx.Clauses(gormclauseLock()).First(&ext, id).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find extension by id %d for update: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("find extension by id %d for update: %w", id, err)
	}
	return &ext, nil
}

// FindPendingByWishIDForUpdate 事务内锁定该心愿的待审申请；不存在返回 ErrNotFound。
func (r *deadlineExtensionRepository) FindPendingByWishIDForUpdate(tx *gorm.DB, wishID uint64) (*model.DeadlineExtension, error) {
	var ext model.DeadlineExtension
	if err := tx.Clauses(gormclauseLock()).
		Where("wish_id = ? AND status = ?", wishID, "pending").
		Order("id DESC").First(&ext).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find pending extension by wish %d: %w", wishID, ErrNotFound)
		}
		return nil, fmt.Errorf("find pending extension by wish %d: %w", wishID, err)
	}
	return &ext, nil
}

// FindLatestByWishID 取心愿最新一条延期申请（详情页展示申请、原因、处理结果复用）。
func (r *deadlineExtensionRepository) FindLatestByWishID(wishID uint64) (*model.DeadlineExtension, error) {
	var ext model.DeadlineExtension
	if err := r.db.Where("wish_id = ?", wishID).Order("created_at DESC, id DESC").First(&ext).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find latest extension by wish %d: %w", wishID, ErrNotFound)
		}
		return nil, fmt.Errorf("find latest extension by wish %d: %w", wishID, err)
	}
	return &ext, nil
}

func (r *deadlineExtensionRepository) ListByWishID(wishID uint64, offset, limit int) ([]model.DeadlineExtension, error) {
	var exts []model.DeadlineExtension
	if err := r.db.Where("wish_id = ?", wishID).Order("created_at DESC, id DESC").
		Offset(offset).Limit(limit).Find(&exts).Error; err != nil {
		return nil, fmt.Errorf("list extensions by wish %d: %w", wishID, err)
	}
	return exts, nil
}

func (r *deadlineExtensionRepository) CountByWishID(wishID uint64) (int64, error) {
	var total int64
	if err := r.db.Model(&model.DeadlineExtension{}).Where("wish_id = ?", wishID).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("count extensions by wish %d: %w", wishID, err)
	}
	return total, nil
}

func (r *deadlineExtensionRepository) UpdateWithTx(tx *gorm.DB, ext *model.DeadlineExtension) error {
	if err := tx.Save(ext).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("update extension %d: %w", ext.ID, ErrConflict)
		}
		return fmt.Errorf("update extension %d: %w", ext.ID, err)
	}
	return nil
}

func (r *deadlineExtensionRepository) ReviewWithTx(tx *gorm.DB, id uint64, status string, reviewerID uint64, note string, reviewedAt time.Time) (int64, error) {
	result := tx.Model(&model.DeadlineExtension{}).
		Where("id = ? AND status = ?", id, "pending").
		Updates(map[string]any{
			"status":      status,
			"reviewer_id": reviewerID,
			"review_note": note,
			"reviewed_at": reviewedAt,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("review extension %d: %w", id, result.Error)
	}
	return result.RowsAffected, nil
}
