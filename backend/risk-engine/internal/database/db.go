package database

import (
	"fmt"

	"risk-engine/internal/config"
	"risk-engine/internal/models"

	sqlite "github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewGORM(cfg config.DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.Driver {
	case "postgres", "postgresql":
		dialector = postgres.Open(cfg.DSN)
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open database failed: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}

	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.APIKey{},
		&models.RiskRule{},
		&models.IPBlacklist{},
		&models.IPWhitelist{},
		&models.AccessLog{},
	)
}
