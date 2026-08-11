package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"risk-engine/internal/database"
	"risk-engine/internal/models"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// CheckRequest is the input for risk checking.
type CheckRequest struct {
	IP          string `json:"ip"`
	UserAgent   string `json:"user_agent"`
	CookieID    string `json:"cookie_id"`
	Campaign    string `json:"campaign"`
	RequestID   string `json:"-"`
}

// CheckResult matches the API contract in the development doc.
type CheckResult struct {
	IP            string `json:"ip"`
	Country       string `json:"country"`
	City          string `json:"city"`
	ISP           string `json:"isp"`
	ASN           string `json:"asn"`
	IsProxy       bool   `json:"is_proxy"`
	IsVPN         bool   `json:"is_vpn"`
	IsDatacenter  bool   `json:"is_datacenter"`
	RiskScore     int    `json:"risk_score"`
	Action        string `json:"action"`
	RuleHit       string `json:"rule_hit,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
}

type cachedRisk struct {
	Info      *database.IPInfo `json:"info"`
	Score     int              `json:"score"`
	Action    string           `json:"action"`
	RuleHit   string           `json:"rule_hit"`
	ExpiredAt time.Time        `json:"expired_at"`
}

type RiskService struct {
	db    *gorm.DB
	redis *redis.Client
	ipdb  *database.IPDB
}

func NewRiskService(db *gorm.DB, rdb *redis.Client, ipdb *database.IPDB) *RiskService {
	return &RiskService{db: db, redis: rdb, ipdb: ipdb}
}

func (s *RiskService) DB() *gorm.DB {
	return s.db
}

// FilterResult is the boolean result used by the PHP landing page.
type FilterResult struct {
	Result    bool   `json:"result"`
	Country   string `json:"-"`
	RiskScore int    `json:"-"`
	Action    string `json:"-"`
	RuleHit   string `json:"-"`
}

// Filter evaluates an IP for the PHP front-end.
// Logic: if no country from IP DB -> allow (true).
//        if targetCountry == "ALL" -> allow as long as not proxy.
//        if targetCountry list contains the IP country and not proxy -> allow.
//        otherwise -> block.
func (s *RiskService) Filter(ctx context.Context, ip, targetCountries string) (*FilterResult, error) {
	if ip == "" {
		return nil, fmt.Errorf("ip is required")
	}

	info, err := s.ipdb.Query(ip)
	if err != nil {
		return nil, fmt.Errorf("ip query failed: %w", err)
	}

	country := info.Country
	if country == "" {
		country = info.CountryLong
	}

	// Build a readable proxy hit summary, e.g. "proxy:yes(PUB)" or "proxy:no".
	proxyHit := "proxy:no"
	if info.IsProxy || info.IsVPN || info.IsTor || info.IsDatacenter {
		var details []string
		if info.IsVPN {
			details = append(details, "VPN")
		}
		if info.IsTor {
			details = append(details, "TOR")
		}
		if info.IsDatacenter {
			details = append(details, "DCH")
		}
		if info.ProxyType != "" && !strings.EqualFold(info.ProxyType, "VPN") && !strings.EqualFold(info.ProxyType, "TOR") && !strings.EqualFold(info.ProxyType, "DCH") {
			details = append(details, strings.ToUpper(info.ProxyType))
		}
		if len(details) == 0 {
			details = append(details, "YES")
		}
		proxyHit = "proxy:yes(" + strings.Join(details, "/") + ")"
	}

	// If proxy/VPN/Tor/datacenter -> block.
	if info.IsProxy || info.IsVPN || info.IsTor || info.IsDatacenter {
		score, action, ruleHit := s.calculateScore(ctx, info)
		return &FilterResult{
			Result:    false,
			Country:   country,
			RiskScore: score,
			Action:    action,
			RuleHit:   ruleHit + " & " + proxyHit,
		}, nil
	}

	// If we cannot determine the country, default allow.
	if country == "" {
		return &FilterResult{Result: true, Action: "allow", RuleHit: "unknown_country & " + proxyHit}, nil
	}

	// Empty targetCountries or ALL means any country is allowed.
	trimmedTargets := strings.TrimSpace(targetCountries)
	if trimmedTargets == "" || strings.EqualFold(trimmedTargets, "ALL") {
		return &FilterResult{Result: true, Country: country, Action: "allow", RuleHit: "all_countries_allowed & " + proxyHit}, nil
	}

	// Check target countries.
	targets := splitAndTrim(targetCountries, ",")
	for _, t := range targets {
		if strings.EqualFold(t, country) {
			return &FilterResult{
				Result:  true,
				Country: country,
				Action:  "allow",
				RuleHit: "country_match:" + strings.ToUpper(country) + " & " + proxyHit,
			}, nil
		}
	}

	return &FilterResult{
		Result:  false,
		Country: country,
		Action:  "block",
		RuleHit: "country_mismatch:" + strings.ToUpper(country) + "!=" + strings.ToUpper(trimmedTargets) + " & " + proxyHit,
	}, nil
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *RiskService) Check(ctx context.Context, req CheckRequest) (*CheckResult, error) {
	if req.IP == "" {
		return nil, fmt.Errorf("ip is required")
	}

	// 1. Try cache.
	cacheKey := "risk:ip:" + req.IP
	if cached, err := s.getCache(ctx, cacheKey); err == nil && cached != nil {
		return s.buildResult(req, cached.Info, cached.Score, cached.Action, cached.RuleHit), nil
	}

	// 2. Query IP databases.
	info, err := s.ipdb.Query(req.IP)
	if err != nil {
		return nil, fmt.Errorf("ip query failed: %w", err)
	}

	// 3. Score.
	score, action, ruleHit := s.calculateScore(ctx, info)

	// 4. Cache and return.
	return s.cacheAndReturn(ctx, cacheKey, info, score, action, ruleHit), nil
}

func (s *RiskService) buildResult(req CheckRequest, info *database.IPInfo, score int, action, ruleHit string) *CheckResult {
	country := info.Country
	if country == "" {
		country = info.CountryLong
	}
	return &CheckResult{
		IP:           info.IP,
		Country:      country,
		City:         info.City,
		ISP:          info.ISP,
		ASN:          info.ASN,
		IsProxy:      info.IsProxy,
		IsVPN:        info.IsVPN,
		IsDatacenter: info.IsDatacenter,
		RiskScore:    score,
		Action:       action,
		RuleHit:      ruleHit,
		RequestID:    req.RequestID,
	}
}

func (s *RiskService) cacheAndReturn(ctx context.Context, key string, info *database.IPInfo, score int, action, ruleHit string) *CheckResult {
	_ = s.setCache(ctx, key, info, score, action, ruleHit, 10*time.Minute)
	return s.buildResult(CheckRequest{IP: info.IP, RequestID: ""}, info, score, action, ruleHit)
}

func (s *RiskService) getCache(ctx context.Context, key string) (*cachedRisk, error) {
	data, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var cr cachedRisk
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, err
	}
	if time.Now().After(cr.ExpiredAt) {
		return nil, fmt.Errorf("cache expired")
	}
	return &cr, nil
}

func (s *RiskService) setCache(ctx context.Context, key string, info *database.IPInfo, score int, action, ruleHit string, ttl time.Duration) error {
	cr := cachedRisk{
		Info:      info,
		Score:     score,
		Action:    action,
		RuleHit:   ruleHit,
		ExpiredAt: time.Now().Add(ttl),
	}
	data, err := json.Marshal(cr)
	if err != nil {
		return err
	}
	return s.redis.Set(ctx, key, data, ttl).Err()
}

func (s *RiskService) calculateScore(ctx context.Context, info *database.IPInfo) (int, string, string) {
	score := 0
	var hits []string

	// Built-in rules aligned with the development doc.
	if info.IsVPN {
		score += 30
		hits = append(hits, "vpn")
	}
	if info.IsProxy {
		score += 40
		hits = append(hits, "proxy")
	}
	if info.IsDatacenter {
		score += 30
		hits = append(hits, "datacenter")
	}
	if info.IsTor {
		score += 80
		hits = append(hits, "tor")
	}

	// Dynamic rules from database.
	rules, _ := s.loadActiveRules(ctx)
	for _, rule := range rules {
		if matchRule(info, rule.Condition) {
			score += rule.Score
			hits = append(hits, rule.Name)
			if rule.Action != "" {
				// Action override from a matching rule is noted but final
				// action is still derived from the total score.
				_ = rule.Action
			}
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	action := scoreToAction(score)
	return score, action, strings.Join(hits, ",")
}

func scoreToAction(score int) string {
	switch {
	case score >= 0 && score < 30:
		return "safe"
	case score >= 30 && score < 70:
		return "review"
	default:
		return "block"
	}
}

func (s *RiskService) loadActiveRules(ctx context.Context) ([]models.RiskRule, error) {
	var rules []models.RiskRule
	if err := s.db.WithContext(ctx).Where("status = ?", 1).Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// matchRule supports a tiny subset of expressions, e.g.:
//   is_vpn == true
//   is_datacenter = 1
//   country == CN
//   proxy_type == VPN
func matchRule(info *database.IPInfo, cond string) bool {
	cond = strings.TrimSpace(cond)
	// Normalize separators.
	cond = strings.ReplaceAll(cond, "==", "=")
	parts := strings.SplitN(cond, "=", 2)
	if len(parts) != 2 {
		return false
	}
	field := strings.TrimSpace(strings.ToLower(parts[0]))
	value := strings.TrimSpace(parts[1])

	var actual string
	switch field {
	case "is_vpn", "vpn":
		actual = strconv.FormatBool(info.IsVPN)
	case "is_proxy", "proxy":
		actual = strconv.FormatBool(info.IsProxy)
	case "is_datacenter", "datacenter", "hosting":
		actual = strconv.FormatBool(info.IsDatacenter)
	case "is_tor", "tor":
		actual = strconv.FormatBool(info.IsTor)
	case "country":
		actual = info.Country
	case "isp":
		actual = info.ISP
	case "asn":
		actual = info.ASN
	case "usage_type":
		actual = info.UsageType
	case "proxy_type":
		actual = info.ProxyType
	default:
		return false
	}

	// Boolean match accepts true/1/yes.
	lowerVal := strings.ToLower(value)
	if lowerVal == "true" || lowerVal == "1" || lowerVal == "yes" {
		return strings.ToLower(actual) == "true" || actual == "1" || strings.ToLower(actual) == "yes"
	}
	if lowerVal == "false" || lowerVal == "0" || lowerVal == "no" {
		return strings.ToLower(actual) == "false" || actual == "0" || strings.ToLower(actual) == "no"
	}

	return strings.EqualFold(actual, value)
}
