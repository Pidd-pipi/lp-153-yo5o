package database

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/wishwall/wishwall/internal/config"
	"github.com/wishwall/wishwall/internal/model"
)

// Open 建立 PostgreSQL 连接并完成迁移。
func Open(cfg *config.Config, logger *slog.Logger) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode,
	)
	gormCfg := &gorm.Config{}
	if cfg.AppEnv != "development" {
		gormCfg.Logger = gormlogger.Default.LogMode(gormlogger.Warn)
	}
	db, err := gorm.Open(postgres.Open(dsn), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := db.AutoMigrate(
		&model.User{},
		&model.Wish{},
		&model.WishClaim{},
		&model.DeadlineExtension{},
		&model.Blessing{},
		&model.TimeCapsule{},
		&model.Badge{},
		&model.AuditLog{},
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	// 同一心愿只允许一条待审延期申请：部分唯一索引兜底并发提交。
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_extensions_pending_wish
		ON deadline_extensions (wish_id) WHERE status = 'pending'
	`).Error; err != nil {
		return nil, fmt.Errorf("create pending extension unique index: %w", err)
	}
	logger.Info("database connected and migrated")
	return db, nil
}
