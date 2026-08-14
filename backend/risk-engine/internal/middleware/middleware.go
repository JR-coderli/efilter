package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"risk-engine/internal/config"
	"risk-engine/internal/logger"
	"risk-engine/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	CtxAPIKey    = "api_key"
	CtxAPIKeyID  = "api_key_id"
	CtxRequestID = "request_id"
)

// bodyLogWriter captures the response body for access logging.
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyLogWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = uuid.New().String()
		}
		c.Set(CtxRequestID, rid)
		c.Header("X-Request-ID", rid)
		c.Next()
	}
}

func APIKeyAuth(cfg config.AppConfig) gin.HandlerFunc {
	valid := make(map[string]struct{})
	for _, k := range cfg.APIKeys {
		if k != "" {
			valid[k] = struct{}{}
		}
	}
	if cfg.APIKey != "" && len(valid) == 0 {
		valid[cfg.APIKey] = struct{}{}
	}

	return func(c *gin.Context) {
		key := extractAPIKey(c)
		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "missing api key"})
			return
		}

		if _, ok := valid[key]; !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid api key"})
			return
		}

		c.Set(CtxAPIKey, key)
		c.Set(CtxAPIKeyID, uint(0))
		c.Next()
	}
}

func extractAPIKey(c *gin.Context) string {
	if k := c.GetHeader("X-API-Key"); k != "" {
		return k
	}
	if k := c.Query("api_key"); k != "" {
		return k
	}
	// Some clients send "Authorization: Bearer <key>".
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func RateLimit(redis *redis.Client, cfg config.RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, exists := c.Get(CtxAPIKey)
		if !exists {
			c.Next()
			return
		}

		apiKey := key.(string)
		windowKey := fmt.Sprintf("rate:%s", apiKey)
		ctx := context.Background()

		current, err := redis.Incr(ctx, windowKey).Result()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "rate limit error"})
			return
		}
		if current == 1 {
			_ = redis.Expire(ctx, windowKey, time.Duration(cfg.Window)*time.Second)
		}

		if int(current) > cfg.MaxRequests {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 429, "message": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

func AccessLog(batcher *AccessLogBatcher, log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Capture response body.
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		// Capture request body (only for API endpoints to avoid large uploads).
		var reqBody string
		if c.Request.Body != nil && strings.HasPrefix(c.Request.URL.Path, "/api/") {
			bodyBytes, _ := io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			reqBody = string(bodyBytes)
			if len(reqBody) > 4096 {
				reqBody = reqBody[:4096]
			}
		}

		c.Next()
		duration := time.Since(start)

		fields := []zap.Field{
			zap.String("request_id", getString(c, CtxRequestID)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("response_time", duration),
		}

		// Values set by the handler to enrich the log.
		country := ""
		if v, ok := c.Get("log_country"); ok {
			country = toString(v)
			fields = append(fields, zap.String("country", country))
		}
		score := 0
		if v, ok := c.Get("log_risk_score"); ok {
			score = toInt(v)
			fields = append(fields, zap.Int("risk_score", score))
		}
		action := ""
		if v, ok := c.Get("log_action"); ok {
			action = toString(v)
		}
		ruleHit := ""
		if v, ok := c.Get("log_rule_hit"); ok {
			ruleHit = toString(v)
			fields = append(fields, zap.String("rule_hit", ruleHit))
		}

		domain := ""
		if v, ok := c.Get("log_domain"); ok {
			domain = toString(v)
		}
		pagePath := ""
		if v, ok := c.Get("log_page_path"); ok {
			pagePath = toString(v)
		}
		referer := ""
		if v, ok := c.Get("log_referer"); ok {
			referer = toString(v)
		}
		ua := c.Request.UserAgent()
		if v, ok := c.Get("log_user_agent"); ok {
			ua = toString(v)
		}
		acceptLang := ""
		if v, ok := c.Get("log_accept_language"); ok {
			acceptLang = toString(v)
		}

		logger.Info("access", fields...)

		// Persist to PostgreSQL asynchronously via batcher (only API requests).
		if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			return
		}

		respBody := blw.body.String()
		if len(respBody) > 4096 {
			respBody = respBody[:4096]
		}

		batcher.Add(models.AccessLog{
			RequestID:      getString(c, CtxRequestID),
			ClientIP:       c.ClientIP(),
			Method:         c.Request.Method,
			Path:           c.Request.URL.Path,
			Domain:         domain,
			PagePath:       pagePath,
			Referer:        referer,
			UserAgent:      ua,
			AcceptLanguage: acceptLang,
			Country:        country,
			RiskScore:      score,
			Action:         action,
			RuleHit:        ruleHit,
			RequestBody:    reqBody,
			ResponseBody:   respBody,
			StatusCode:     c.Writer.Status(),
			ResponseTime:   duration.Milliseconds(),
			CreatedAt:      time.Now(),
		})
	}
}

func getString(c *gin.Context, key string) string {
	v, ok := c.Get(key)
	if !ok {
		return ""
	}
	return toString(v)
}

func toString(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case fmt.Stringer:
		return s.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toInt(v interface{}) int {
	switch i := v.(type) {
	case int:
		return i
	case int64:
		return int(i)
	case float64:
		return int(i)
	case string:
		n, _ := strconv.Atoi(i)
		return n
	default:
		return 0
	}
}
