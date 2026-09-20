package repository

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/wishwall/wishwall/internal/model"
)

// WishExtensionRepository 心愿延期申请仓储接口。
type WishExtensionRepository interface {
	CreateWithTx(tx *gorm.DB, ext *model.WishExtension) error
	FindByID(id uint64) (*model.WishExtension, error)
	FindByIDForUpdate(tx *gorm.DB, id uint64) (*model.WishExtension, error)
	FindPendingByWishID(wishID uint64) (*model.WishExtension, error)
	UpdateWithTx(tx *gorm.DB, ext *model.WishExtension) error
	ListByWishID(wishID uint64) ([]model.WishExtension, error)
}

type wishExtensionRepository struct {
	db *gorm.DB
}

// NewWishExtensionRepository 构造延期申请仓储。
func NewWishExtensionRepository(db *gorm.DB) WishExtensionRepository {
	return &wishExtensionRepository{db: db}
}

func (r *wishExtensionRepository) CreateWithTx(tx *gorm.DB, ext *model.WishExtension) error {
	if err := tx.Create(ext).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create extension on wish %d with tx: %w", ext.WishID, ErrConflict)
		}
		return fmt.Errorf("create extension on wish %d with tx: %w", ext.WishID, err)
	}
	return nil
}

func (r *wishExtensionRepository) FindByID(id uint64) (*model.WishExtension, error) {
	var ext model.WishExtension
	if err := r.db.First(&ext, id).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find extension by id %d: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("find extension by id %d: %w", id, err)
	}
	return &ext, nil
}

// FindByIDForUpdate 行级锁读取，用于并发审核场景（只许成功一次）。
func (r *wishExtensionRepository) FindByIDForUpdate(tx *gorm.DB, id uint64) (*model.WishExtension, error) {
	var ext model.WishExtension
	if err := tx.Clauses(gormclauseLock()).First(&ext, id).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find extension by id %d for update: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("find extension by id %d for update: %w", id, err)
	}
	return &ext, nil
}

// FindPendingByWishID 查询心愿当前待审（pending）的延期申请。
func (r *wishExtensionRepository) FindPendingByWishID(wishID uint64) (*model.WishExtension, error) {
	var ext model.WishExtension
	if err := r.db.Where("wish_id = ? AND status = ?", wishID, "pending").First(&ext).Error; err != nil {
		if isRecordNotFound(err) {
			return nil, fmt.Errorf("find pending extension by wish %d: %w", wishID, ErrNotFound)
		}
		return nil, fmt.Errorf("find pending extension by wish %d: %w", wishID, err)
	}
	return &ext, nil
}

func (r *wishExtensionRepository) UpdateWithTx(tx *gorm.DB, ext *model.WishExtension) error {
	if err := tx.Save(ext).Error; err != nil {
		return fmt.Errorf("update extension %d with tx: %w", ext.ID, err)
	}
	return nil
}

// ListByWishID 心愿的延期申请列表（详情页回读复用）。
func (r *wishExtensionRepository) ListByWishID(wishID uint64) ([]model.WishExtension, error) {
	var exts []model.WishExtension
	if err := r.db.Where("wish_id = ?", wishID).Order("created_at DESC").Find(&exts).Error; err != nil {
		return nil, fmt.Errorf("list extensions by wish %d: %w", wishID, err)
	}
	return exts, nil
}
