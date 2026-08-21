package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"risk-engine/internal/api"
	"risk-engine/internal/config"
	"risk-engine/internal/database"
	"risk-engine/internal/logger"
	"risk-engine/internal/middleware"
	"risk-engine/internal/models"
	"risk-engine/internal/service"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config error: %v\n", err)
		os.Exit(1)
	}

	if err := logger.Init(cfg.Log.Level, cfg.Log.Path); err != nil {
		fmt.Fprintf(os.Stderr, "init logger error: %v\n", err)
		os.Exit(1)
	}

	// Database is optional: if it fails, the service still starts so that
	// /api/v1/results and /api/v1/check can return IP risk results. Access
	// logs and dashboard will be unavailable until PostgreSQL is healthy.
	var db *gorm.DB
	db, err = database.NewGORM(cfg.Database)
	if err != nil {
		logger.Warn("connect database failed, continuing without access logs", zap.Error(err))
	} else {
		if err := database.AutoMigrate(db); err != nil {
			logger.Warn("auto migrate failed, continuing without access logs", zap.Error(err))
			db = nil
		}
	}

	rdb, err := database.NewRedis(cfg.Redis)
	if err != nil {
		logger.Fatal("connect redis failed", zap.Error(err))
	}

	ipdb, err := database.NewIPDB(cfg.IPDB)
	if err != nil {
		logger.Fatal("open ip database failed", zap.Error(err))
	}
	defer ipdb.Close()

	if db != nil {
		if err := seedDefaultData(db); err != nil {
			logger.Warn("seed default data failed", zap.Error(err))
		}

		// Start background cleanup for access logs (retain 24 hours).
		go startAccessLogCleanup(db)
	}

	// Start batched access log writer (flush every minute). A nil db is safe
	// and simply drops records, keeping the core APIs available.
	accessLogBatcher := middleware.NewAccessLogBatcher(db, logger.L(), time.Minute)
	defer accessLogBatcher.Stop()

	riskService := service.NewRiskService(db, rdb, ipdb)
	handler := api.NewHandler(riskService)
	router := api.NewRouter(cfg, db, accessLogBatcher, rdb, handler, logger.L())

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.App.Port),
		Handler: router,
	}

	go func() {
		logger.Info("server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}
	logger.Info("server stopped")
}

func seedDefaultData(db *gorm.DB) error {
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count == 0 {
		user := models.User{
			Username: "admin",
			Password: "",
			Status:   1,
		}
		if err := db.Create(&user).Error; err != nil {
			return err
		}
	}

	db.Model(&models.APIKey{}).Count(&count)
	if count == 0 {
		key := models.APIKey{
			UserID:    1,
			ApiKey:    "risk-engine-dev-key-2026",
			RateLimit: 100,
			Status:    1,
		}
		if err := db.Create(&key).Error; err != nil {
			return err
		}
	}

	// Remove legacy default rules that duplicate the built-in scoring in
	// service.calculateScore. Keeping them caused rule_hit to show duplicated
	// hits such as "proxy,proxy".
	db.Where("name IN ? AND condition IN ?",
		[]string{"vpn", "proxy", "datacenter", "tor"},
		[]string{"is_vpn == true", "is_proxy == true", "is_datacenter == true", "is_tor == true"},
	).Delete(&models.RiskRule{})

	// Built-in scoring rules are now hard-coded in service.calculateScore;
	// do not seed duplicates.

	return nil
}

func startAccessLogCleanup(db *gorm.DB) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-24 * time.Hour)
		result := db.Where("created_at < ?", cutoff).Delete(&models.AccessLog{})
		if result.Error != nil {
			logger.Error("access log cleanup failed", zap.Error(result.Error))
		} else {
			logger.Info("access log cleanup completed", zap.Int64("rows", result.RowsAffected))
		}
	}
}
