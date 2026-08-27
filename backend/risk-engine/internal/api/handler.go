package api

import (
	"net/http"
	"strings"
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
	c.Set("log_country_ip2location", result.CountryIP2Location)
	c.Set("log_country_maxmind", result.CountryMaxMind)
	c.Set("log_max_city", result.CityMaxMind)
	c.Set("log_max_asn", result.ASNMaxMind)
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
	c.Set("log_country_ip2location", result.CountryIP2Location)
	c.Set("log_country_maxmind", result.CountryMaxMind)
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
	Limit       int    `form:"limit,default=50"`
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
	if h.riskService.DB() == nil {
		c.JSON(http.StatusServiceUnavailable, response{Code: 503, Message: "database unavailable"})
		return
	}

	var q LogsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		q.Limit = 50
	}
	if q.Limit <= 0 || q.Limit > 1000 {
		q.Limit = 50
	}

	filters := logFilters{
		IP:          q.IP,
		Path:        q.Path,
		ExcludePath: q.ExcludePath,
	}

	baseQuery := h.riskService.DB().WithContext(c.Request.Context()).Model(&models.AccessLog{})
	baseQuery = applyLogFilters(baseQuery, filters)

	// Run stats aggregation and list query concurrently to reduce latency.
	statsCh := make(chan logStats, 1)
	statsErrCh := make(chan error, 1)
	go func() {
		stats, err := computeLogStats(h.riskService.DB(), filters)
		if err != nil {
			statsErrCh <- err
			return
		}
		statsCh <- stats
	}()

	// Keyword search is applied only to the log list, not the top-level stats.
	// Avoid matching large text columns (request_body/response_body/user_agent/accept_language)
	// to keep queries fast.
	listQuery := baseQuery.Order("created_at DESC")
	if q.Keyword != "" {
		pattern := "%" + q.Keyword + "%"
		listQuery = listQuery.Where(
			"client_ip ILIKE ? OR country ILIKE ? OR country_ip2location ILIKE ? OR country_maxmind ILIKE ? OR max_city ILIKE ? OR max_asn ILIKE ? OR rule_hit ILIKE ? OR path ILIKE ? OR action ILIKE ? OR domain ILIKE ? OR page_path ILIKE ? OR referer ILIKE ?",
			pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern,
		)
	}

	var logs []models.AccessLog
	logsErr := make(chan error, 1)
	go func() {
		logsErr <- listQuery.Limit(q.Limit).Offset(q.Offset).Find(&logs).Error
	}()

	var stats logStats
	select {
	case err := <-statsErrCh:
		c.JSON(http.StatusInternalServerError, response{Code: 500, Message: err.Error()})
		return
	case stats = <-statsCh:
	}

	if err := <-logsErr; err != nil {
		c.JSON(http.StatusInternalServerError, response{Code: 500, Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, response{
		Code:    0,
		Message: "ok",
		Data: gin.H{
			"total": stats.Total,
			"stats": stats,
			"logs":  logs,
		},
	})
}

type logFilters struct {
	IP          string
	Path        string
	ExcludePath string
}

func applyLogFilters(query *gorm.DB, f logFilters) *gorm.DB {
	if f.IP != "" {
		query = query.Where("client_ip = ?", f.IP)
	}
	if f.Path != "" {
		query = query.Where("path = ?", f.Path)
	}
	if f.ExcludePath != "" {
		query = query.Where("path != ?", f.ExcludePath)
	}
	return query
}

func buildLogFilterSQL(f logFilters) (string, []interface{}) {
	var clauses []string
	var args []interface{}
	if f.IP != "" {
		clauses = append(clauses, "client_ip = ?")
		args = append(args, f.IP)
	}
	if f.Path != "" {
		clauses = append(clauses, "path = ?")
		args = append(args, f.Path)
	}
	if f.ExcludePath != "" {
		clauses = append(clauses, "path != ?")
		args = append(args, f.ExcludePath)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func computeLogStats(db *gorm.DB, filters logFilters) (logStats, error) {
	var stats logStats
	hourAgo := time.Now().Add(-1 * time.Hour)

	var result struct {
		Total     int64
		Allow     int64
		Deny      int64
		HourCount int64
	}

	whereClause, whereArgs := buildLogFilterSQL(filters)

	sql := "SELECT " +
		"COUNT(*) AS total, " +
		"COUNT(*) FILTER (WHERE action IN ('allow', 'safe')) AS allow, " +
		"COUNT(*) FILTER (WHERE action IN ('block', 'review')) AS deny, " +
		"COUNT(*) FILTER (WHERE created_at > ?) AS hour_count " +
		"FROM access_logs " + whereClause

	args := []interface{}{hourAgo}
	args = append(args, whereArgs...)

	err := db.Raw(sql, args...).Scan(&result).Error
	if err != nil {
		return stats, err
	}

	stats.Total = result.Total
	stats.Allow = result.Allow
	stats.Deny = result.Deny
	stats.HourCount = result.HourCount
	return stats, nil
}
