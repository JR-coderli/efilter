package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"risk-engine/internal/config"
	"risk-engine/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// getFrontendDir returns the absolute path to the frontend directory.
func getFrontendDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "../../frontend"
	}
	abs, err := filepath.Abs(filepath.Join(wd, "../../frontend"))
	if err != nil {
		return "../../frontend"
	}
	return filepath.ToSlash(abs)
}

func NewRouter(cfg *config.Config, db *gorm.DB, batcher *middleware.AccessLogBatcher, rdb *redis.Client, h *Handler, log *zap.Logger) *gin.Engine {
	if cfg.App.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.AccessLog(batcher, log))

	// Public health check.
	r.GET("/health", h.Health)

	// Dashboard frontend.
	frontendDir := getFrontendDir()
	r.GET("/static/*filepath", gin.WrapH(http.StripPrefix("/static/", http.FileServer(http.Dir(frontendDir)))))
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || path == "/health" || strings.HasPrefix(path, "/static/") {
			c.AbortWithStatusJSON(404, gin.H{"code": 404, "message": "not found"})
			return
		}
		c.File(filepath.Join(frontendDir, "index.html"))
	})

	// Authenticated API group.
	v1 := r.Group("/api/v1")
	v1.Use(middleware.APIKeyAuth(db))
	v1.Use(middleware.RateLimit(rdb, cfg.RateLimit))
	{
		v1.POST("/check", h.Check)
		v1.POST("/results", h.Results)
		v1.GET("/logs", h.Logs)
	}

	return r
}
