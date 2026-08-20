package models

import (
	"time"
)

type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"size:64;not null;uniqueIndex" json:"username"`
	Password  string    `gorm:"size:255;not null" json:"-"`
	Status    int       `gorm:"default:1" json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type APIKey struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"not null;index" json:"user_id"`
	ApiKey    string    `gorm:"size:128;not null;uniqueIndex" json:"api_key"`
	RateLimit int       `gorm:"default:100" json:"rate_limit"`
	Status    int       `gorm:"default:1" json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type RiskRule struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Condition string    `gorm:"type:text;not null" json:"condition"`
	Score     int       `gorm:"not null" json:"score"`
	Action    string    `gorm:"size:32" json:"action"`
	Status    int       `gorm:"default:1" json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type IPBlacklist struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	IP        string    `gorm:"size:64;not null;uniqueIndex" json:"ip"`
	Reason    string    `gorm:"size:255" json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type IPWhitelist struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	IP        string    `gorm:"size:64;not null;uniqueIndex" json:"ip"`
	Remark    string    `gorm:"size:255" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
}

type AccessLog struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	RequestID      string    `gorm:"size:64;index" json:"request_id"`
	ClientIP       string    `gorm:"size:64;index" json:"client_ip"`
	Method         string    `gorm:"size:16" json:"method"`
	Path           string    `gorm:"size:255" json:"path"`
	Domain         string    `gorm:"size:255;index" json:"domain"`
	PagePath       string    `gorm:"size:512;index" json:"page_path"`
	Referer        string    `gorm:"type:text" json:"referer"`
	UserAgent      string    `gorm:"type:text" json:"user_agent"`
	AcceptLanguage string    `gorm:"type:text" json:"accept_language"`
	Country          string    `gorm:"size:8;index" json:"country"`
	CountryIP2Location string   `gorm:"size:8;index" json:"country_ip2location"`
	CountryMaxMind   string    `gorm:"size:8;index" json:"country_maxmind"`
	MaxCity          string    `gorm:"size:64;index" json:"max_city"`
	MaxASN           string    `gorm:"size:32;index" json:"max_asn"`
	RiskScore      int       `json:"risk_score"`
	Action         string    `gorm:"size:16" json:"action"`
	RuleHit        string    `gorm:"size:255" json:"rule_hit"`
	RequestBody    string    `gorm:"type:text" json:"request_body"`
	ResponseBody   string    `gorm:"type:text" json:"response_body"`
	StatusCode     int       `json:"status_code"`
	ResponseTime   int64     `gorm:"index" json:"response_time_ms"`
	CreatedAt      time.Time `gorm:"index" json:"created_at"`
}
