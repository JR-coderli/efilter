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

	db, err := database.NewGORM(cfg.Database)
	if err != nil {
		logger.Fatal("connect database failed", zap.Error(err))
	}
	if err := database.AutoMigrate(db); err != nil {
		logger.Fatal("auto migrate failed", zap.Error(err))
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

	if err := seedDefaultData(db); err != nil {
		logger.Fatal("seed default data failed", zap.Error(err))
	}

	// Start background cleanup for access logs (retain 24 hours).
	go startAccessLogCleanup(db)

	// Start batched access log writer (flush every minute).
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

	// Seed built-in risk rules so the API returns meaningful scores out of the box.
	var ruleCount int64
	db.Model(&models.RiskRule{}).Count(&ruleCount)
	if ruleCount == 0 {
		rules := []models.RiskRule{
			{Name: "vpn", Condition: "is_vpn == true", Score: 30, Action: "review", Status: 1},
			{Name: "proxy", Condition: "is_proxy == true", Score: 40, Action: "review", Status: 1},
			{Name: "datacenter", Condition: "is_datacenter == true", Score: 30, Action: "review", Status: 1},
			{Name: "tor", Condition: "is_tor == true", Score: 80, Action: "block", Status: 1},
		}
		if err := db.Create(&rules).Error; err != nil {
			return err
		}
	}

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
