package api

import (
	"net/http"
	"time"

	"risk-engine/internal/middleware"
	"risk-engine/internal/models"
	"risk-engine/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	riskService *service.RiskService
}

func NewHandler(riskService *service.RiskService) *Handler {
	return &Handler{riskService: riskService}
}

type checkRequest struct {
	IP         string `json:"ip" binding:"required,ip"`
	UserAgent  string `json:"user_agent"`
	CookieID   string `json:"cookie_id"`
	Campaign   string `json:"campaign"`
}

type response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (h *Handler) Check(c *gin.Context) {
	var req checkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response{Code: 400, Message: err.Error()})
		return
	}

	rid, _ := c.Get(middleware.CtxRequestID)
	result, err := h.riskService.Check(c.Request.Context(), service.CheckRequest{
		IP:        req.IP,
		UserAgent: req.UserAgent,
		CookieID:  req.CookieID,
		Campaign:  req.Campaign,
		RequestID: rid.(string),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, response{Code: 500, Message: err.Error()})
		return
	}

	c.Set("log_country", result.Country)
	c.Set("log_risk_score", result.RiskScore)
	c.Set("log_action", result.Action)
	c.Set("log_rule_hit", result.RuleHit)

	c.JSON(http.StatusOK, response{Code: 0, Message: "ok", Data: result})
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, response{Code: 0, Message: "ok", Data: gin.H{"status": "up"}})
}

// ResultsRequest matches the x-www-form-urlencoded fields sent by the PHP file.
type ResultsRequest struct {
	IP             string `form:"ip" binding:"required"`
	Country        string `form:"country"`
	Domain         string `form:"domain"`
	PagePath       string `form:"path"`
	Referer        string `form:"referer"`
	UserAgent      string `form:"user_agent"`
	AcceptLanguage string `form:"accept_language"`
}

// Results returns {"result": true/false} for the PHP front-end filter.
func (h *Handler) Results(c *gin.Context) {
	var req ResultsRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"result": true})
		return
	}

	result, err := h.riskService.Filter(c.Request.Context(), req.IP, req.Country)
	if err != nil {
		// Fail open: if the service errors, allow the visit.
		c.JSON(http.StatusOK, gin.H{"result": true})
		return
	}

	c.Set("log_country", result.Country)
	c.Set("log_risk_score", result.RiskScore)
	c.Set("log_action", result.Action)
	c.Set("log_rule_hit", result.RuleHit)

	c.Set("log_domain", req.Domain)
	c.Set("log_page_path", req.PagePath)
	c.Set("log_referer", req.Referer)
	if req.UserAgent != "" {
		c.Set("log_user_agent", req.UserAgent)
	}
	c.Set("log_accept_language", req.AcceptLanguage)

	c.JSON(http.StatusOK, gin.H{"result": result.Result})
}

// LogsQuery is the query parameters for fetching access logs.
type LogsQuery struct {
	Limit       int    `form:"limit,default=100"`
	Offset      int    `form:"offset"`
	IP          string `form:"ip"`
	Keyword     string `form:"keyword"`
	Path        string `form:"path"`
	ExcludePath string `form:"exclude_path"`
}

// logStats holds aggregate counts for the dashboard cards.
type logStats struct {
	Total     int64 `json:"total"`
	Allow     int64 `json:"allow"`
	Deny      int64 `json:"deny"`
	HourCount int64 `json:"hour_count"`
}

// Logs returns recent access logs for the dashboard.
func (h *Handler) Logs(c *gin.Context) {
	var q LogsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		q.Limit = 100
	}
	if q.Limit <= 0 || q.Limit > 1000 {
		q.Limit = 100
	}

	baseQuery := func() *gorm.DB {
		query := h.riskService.DB().WithContext(c.Request.Context()).Model(&models.AccessLog{})
		if q.IP != "" {
			query = query.Where("client_ip = ?", q.IP)
		}
		if q.Path != "" {
			query = query.Where("path = ?", q.Path)
		}
		if q.ExcludePath != "" {
			query = query.Where("path != ?", q.ExcludePath)
		}
		return query
	}

	stats, err := computeLogStats(baseQuery)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response{Code: 500, Message: err.Error()})
		return
	}

	var total int64
	if err := baseQuery().Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, response{Code: 500, Message: err.Error()})
		return
	}

	// Keyword search is applied only to the log list, not the top-level stats.
	listQuery := baseQuery().Order("created_at DESC")
	if q.Keyword != "" {
		pattern := "%" + q.Keyword + "%"
		listQuery = listQuery.Where(
			"client_ip ILIKE ? OR country ILIKE ? OR rule_hit ILIKE ? OR path ILIKE ? OR action ILIKE ? OR domain ILIKE ? OR page_path ILIKE ? OR referer ILIKE ? OR user_agent ILIKE ? OR accept_language ILIKE ? OR request_body ILIKE ? OR response_body ILIKE ?",
			pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern,
		)
	}

	var logs []models.AccessLog
	if err := listQuery.Limit(q.Limit).Offset(q.Offset).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, response{Code: 500, Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, response{
		Code:    0,
		Message: "ok",
		Data: gin.H{
			"total": total,
			"stats": stats,
			"logs":  logs,
		},
	})
}

func computeLogStats(baseQuery func() *gorm.DB) (logStats, error) {
	var stats logStats
	var total int64
	if err := baseQuery().Count(&total).Error; err != nil {
		return stats, err
	}
	stats.Total = total

	var allow int64
	if err := baseQuery().Where("action IN ?", []string{"allow", "safe"}).Count(&allow).Error; err != nil {
		return stats, err
	}
	stats.Allow = allow

	var deny int64
	if err := baseQuery().Where("action IN ?", []string{"block", "review"}).Count(&deny).Error; err != nil {
		return stats, err
	}
	stats.Deny = deny

	hourAgo := time.Now().Add(-1 * time.Hour)
	var hourCount int64
	if err := baseQuery().Where("created_at > ?", hourAgo).Count(&hourCount).Error; err != nil {
		return stats, err
	}
	stats.HourCount = hourCount

	return stats, nil
}
