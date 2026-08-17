package database

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/oschwald/geoip2-golang"
)

// MaxMindPaths groups the optional GeoLite2 MMDB file paths.
type MaxMindPaths struct {
	Country string
	City    string
	ASN     string
}

// MaxMindInfo holds the geo/ASN data returned by MaxMind lookups.
type MaxMindInfo struct {
	Country string `json:"country"`
	City    string `json:"city"`
	ASN     string `json:"asn"`
	ISP     string `json:"isp"`
}

// MaxMindDB wraps one or more GeoLite2 MMDB readers.
type MaxMindDB struct {
	country *geoip2.Reader
	city    *geoip2.Reader
	asn     *geoip2.Reader
}

// NewMaxMindDB opens the requested MaxMind databases. Empty paths are skipped.
// If no databases are configured it returns a non-nil *MaxMindDB with no readers.
func NewMaxMindDB(paths MaxMindPaths) (*MaxMindDB, error) {
	db := &MaxMindDB{}
	opened := 0

	if paths.Country != "" {
		if _, err := os.Stat(paths.Country); err != nil {
			log.Printf("[maxmind] skipping country db: %v", err)
		} else {
			reader, err := geoip2.Open(paths.Country)
			if err != nil {
				return nil, fmt.Errorf("open maxmind country db failed: %w", err)
			}
			db.country = reader
			opened++
		}
	}
	if paths.City != "" {
		if _, err := os.Stat(paths.City); err != nil {
			log.Printf("[maxmind] skipping city db: %v", err)
		} else {
			reader, err := geoip2.Open(paths.City)
			if err != nil {
				_ = db.Close()
				return nil, fmt.Errorf("open maxmind city db failed: %w", err)
			}
			db.city = reader
			opened++
		}
	}
	if paths.ASN != "" {
		if _, err := os.Stat(paths.ASN); err != nil {
			log.Printf("[maxmind] skipping asn db: %v", err)
		} else {
			reader, err := geoip2.Open(paths.ASN)
			if err != nil {
				_ = db.Close()
				return nil, fmt.Errorf("open maxmind asn db failed: %w", err)
			}
			db.asn = reader
			opened++
		}
	}

	log.Printf("[maxmind] opened %d database(s)", opened)
	return db, nil
}

// Close releases all MaxMind readers.
func (m *MaxMindDB) Close() error {
	var firstErr error
	if m.country != nil {
		if err := m.country.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		m.country = nil
	}
	if m.city != nil {
		if err := m.city.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		m.city = nil
	}
	if m.asn != nil {
		if err := m.asn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		m.asn = nil
	}
	return firstErr
}

// Enabled reports whether any MaxMind database is loaded.
func (m *MaxMindDB) Enabled() bool {
	return m != nil && (m.country != nil || m.city != nil || m.asn != nil)
}

// Query looks up an IP across all loaded MaxMind databases.
func (m *MaxMindDB) Query(ip string) (*MaxMindInfo, error) {
	info := &MaxMindInfo{}
	if !m.Enabled() {
		return info, nil
	}

	parsed := net.ParseIP(normalizeIP(ip))
	if parsed == nil {
		return info, fmt.Errorf("invalid ip: %s", ip)
	}

	if m.country != nil {
		rec, err := m.country.Country(parsed)
		if err == nil {
			info.Country = clean(rec.Country.IsoCode)
		}
	}

	if m.city != nil {
		rec, err := m.city.City(parsed)
		if err == nil {
			info.City = clean(rec.City.Names["en"])
			if info.Country == "" {
				info.Country = clean(rec.Country.IsoCode)
			}
		}
	}

	if m.asn != nil {
		rec, err := m.asn.ASN(parsed)
		if err == nil {
			info.ASN = fmt.Sprintf("AS%d", rec.AutonomousSystemNumber)
			info.ISP = clean(rec.AutonomousSystemOrganization)
		}
	}

	return info, nil
}
