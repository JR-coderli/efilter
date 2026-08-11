package api

import (
	"net/http"

	"risk-engine/internal/middleware"
	"risk-engine/internal/models"
	"risk-engine/internal/service"

	"github.com/gin-gonic/gin"
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
	IP      string `form:"ip" binding:"required"`
	Country string `form:"country"`
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

	c.JSON(http.StatusOK, gin.H{"result": result.Result})
}

// LogsQuery is the query parameters for fetching access logs.
type LogsQuery struct {
	Limit  int    `form:"limit,default=100"`
	Offset int    `form:"offset"`
	IP     string `form:"ip"`
	Path   string `form:"path"`
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

	var logs []models.AccessLog
	dbQuery := h.riskService.DB().WithContext(c.Request.Context()).Order("created_at DESC")
	if q.IP != "" {
		dbQuery = dbQuery.Where("client_ip = ?", q.IP)
	}
	if q.Path != "" {
		dbQuery = dbQuery.Where("path = ?", q.Path)
	}

	var total int64
	if err := dbQuery.Model(&models.AccessLog{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, response{Code: 500, Message: err.Error()})
		return
	}

	if err := dbQuery.Limit(q.Limit).Offset(q.Offset).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, response{Code: 500, Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, response{
		Code:    0,
		Message: "ok",
		Data: gin.H{
			"total": total,
			"logs":  logs,
		},
	})
}
