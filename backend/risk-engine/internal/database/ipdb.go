package database

import (
	"fmt"
	"net"
	"strings"

	"risk-engine/internal/config"

	ip2location "github.com/ip2location/ip2location-go/v9"
	ip2proxy "github.com/ip2location/ip2proxy-go/v4"
)

// IPInfo is the unified result from both IP databases.
type IPInfo struct {
	IP            string  `json:"ip"`
	Country       string  `json:"country"`
	CountryLong   string  `json:"country_long"`
	Region        string  `json:"region"`
	City          string  `json:"city"`
	Latitude      float32 `json:"latitude"`
	Longitude     float32 `json:"longitude"`
	Timezone      string  `json:"timezone"`
	ISP           string  `json:"isp"`
	ASN           string  `json:"asn"`
	AS            string  `json:"as"`
	Domain        string  `json:"domain"`
	UsageType     string  `json:"usage_type"`
	IsProxy       bool    `json:"is_proxy"`
	ProxyType     string  `json:"proxy_type"`
	ProxySource   string  `json:"proxy_source"`
	IsVPN         bool    `json:"is_vpn"`
	IsDatacenter  bool    `json:"is_datacenter"`
	IsTor         bool    `json:"is_tor"`
	IsResidential bool    `json:"is_residential"`
	IsMobile      bool    `json:"is_mobile"`
	Threat        string  `json:"threat"`
	FraudScore    string  `json:"fraud_score"`
}

type IPDB struct {
	loc *ip2location.DB
	prx *ip2proxy.DB
	csv *IP2ProxyCSV
}

func NewIPDB(cfg config.IPDBConfig) (*IPDB, error) {
	db := &IPDB{}
	if cfg.IP2Location != "" {
		loc, err := ip2location.OpenDB(cfg.IP2Location)
		if err != nil {
			return nil, fmt.Errorf("open ip2location db failed: %w", err)
		}
		db.loc = loc
	}
	if cfg.IP2Proxy != "" {
		prx, err := ip2proxy.OpenDB(cfg.IP2Proxy)
		if err != nil {
			return nil, fmt.Errorf("open ip2proxy db failed: %w", err)
		}
		db.prx = prx
	}
	if cfg.IP2ProxyIPv6CSV != "" {
		csv, err := NewIP2ProxyCSV(cfg.IP2ProxyIPv6CSV)
		if err != nil {
			return nil, fmt.Errorf("open ip2proxy ipv6 csv failed: %w", err)
		}
		db.csv = csv
	}
	return db, nil
}

func (d *IPDB) Close() {
	if d.loc != nil {
		d.loc.Close()
	}
	if d.prx != nil {
		_ = d.prx.Close()
	}
	if d.csv != nil {
		d.csv.Close()
	}
}

func isIPv6(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() == nil
}

func normalizeIP(ip string) string {
	if host, _, err := net.SplitHostPort(ip); err == nil {
		return host
	}
	return ip
}

func (d *IPDB) Query(ip string) (*IPInfo, error) {
	ip = normalizeIP(ip)

	info := &IPInfo{IP: ip}

	var locErr, prxErr error
	if d.loc != nil {
		rec, err := d.loc.Get_all(ip)
		if err == nil {
			info.Country = clean(rec.Country_short)
			info.CountryLong = clean(rec.Country_long)
			info.Region = clean(rec.Region)
			info.City = clean(rec.City)
			info.Latitude = rec.Latitude
			info.Longitude = rec.Longitude
			info.Timezone = clean(rec.Timezone)
			info.ISP = clean(rec.Isp)
			info.ASN = clean(rec.Asn)
			info.AS = clean(rec.As)
			info.Domain = clean(rec.Domain)
			info.UsageType = clean(rec.Usagetype)
		} else {
			locErr = err
		}
	}

	if d.prx != nil {
		rec, err := d.prx.GetAll(ip)
		if err == nil {
			if info.Country == "" {
				info.Country = clean(rec.CountryShort)
			}
			if info.CountryLong == "" {
				info.CountryLong = clean(rec.CountryLong)
			}
			if info.Region == "" {
				info.Region = clean(rec.Region)
			}
			if info.City == "" {
				info.City = clean(rec.City)
			}
			if info.ISP == "" {
				info.ISP = clean(rec.Isp)
			}
			if info.ASN == "" {
				info.ASN = clean(rec.Asn)
			}
			// TEMP: disable IPv6 proxy detection from BIN to validate CSV fallback.
			if isIPv6(ip) {
				rec.IsProxy = 0
			}
			info.IsProxy = rec.IsProxy == 1
			if info.IsProxy {
				info.ProxySource = "BIN"
			}
			info.ProxyType = clean(rec.ProxyType)
			info.Domain = clean(rec.Domain)
			info.UsageType = clean(rec.UsageType)
			info.Threat = clean(rec.Threat)
			info.FraudScore = clean(rec.FraudScore)

			info.IsVPN = containsAny(rec.ProxyType, "VPN")
			info.IsTor = containsAny(rec.ProxyType, "TOR")
			info.IsDatacenter = containsAny(rec.ProxyType, "DCH", "DATACENTER", "HOSTING")
			info.IsResidential = containsAny(rec.UsageType, "RES", "RESIDENTIAL")
			info.IsMobile = containsAny(rec.UsageType, "MOB", "MOBILE") || containsAny(rec.ProxyType, "MOBILE")
		} else {
			prxErr = err
		}
	}

	// IPv6 fallback: the free IP2Proxy BIN is IPv4-only, so use the CSV
	// database for IPv6 proxy detection when the BIN did not detect one.
	if d.csv != nil && isIPv6(ip) && !info.IsProxy {
		isProxy, proxyType, country, err := d.csv.Query(ip)
		if err == nil && isProxy {
			info.IsProxy = true
			info.ProxySource = "CSV"
			info.ProxyType = clean(proxyType)
			if info.Country == "" && country != "" {
				info.Country = clean(country)
			}
			switch strings.ToUpper(proxyType) {
			case "VPN":
				info.IsVPN = true
			case "TOR":
				info.IsTor = true
			case "DCH", "DATACENTER", "HOSTING":
				info.IsDatacenter = true
			}
		}
	}

	if locErr != nil && prxErr != nil {
		return nil, fmt.Errorf("ip lookup failed: %v; %v", locErr, prxErr)
	}
	return info, nil
}

func clean(s string) string {
	upper := strings.ToUpper(s)
	if strings.Contains(upper, "UNAVAILABLE") ||
		strings.Contains(upper, "NOT SUPPORTED") ||
		strings.Contains(upper, "N/A") ||
		strings.Contains(upper, "MISSING") ||
		strings.Contains(upper, "INVALID") {
		return ""
	}
	return strings.TrimSpace(s)
}

func containsAny(s string, subs ...string) bool {
	upper := strings.ToUpper(strings.TrimSpace(s))
	for _, sub := range subs {
		if strings.EqualFold(strings.TrimSpace(sub), upper) {
			return true
		}
	}
	return false
}
