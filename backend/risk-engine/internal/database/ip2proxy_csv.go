package database

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"sort"
	"strings"
	"time"
)

// ip2ProxyCSVRecord represents one IP range from the IP2Proxy IPv6 CSV.
// from/to are stored as 16-byte big-endian IPv6 numbers so that byte
// comparison gives the same ordering as 128-bit integer comparison.
type ip2ProxyCSVRecord struct {
	from      [16]byte
	to        [16]byte
	proxyType string
	country   string
}

// IP2ProxyCSV loads the IP2Proxy IPv6 CSV into memory and answers proxy
// lookups via binary search. It is intended as an IPv6 fallback because the
// free LITE BIN database is IPv4-only.
type IP2ProxyCSV struct {
	records []ip2ProxyCSVRecord
}

// NewIP2ProxyCSV loads the CSV file at path into memory.
// Expected columns: ip_from, ip_to, proxy_type, country_code, country_name.
func NewIP2ProxyCSV(path string) (*IP2ProxyCSV, error) {
	start := time.Now()

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ip2proxy csv failed: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReader(f))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	r.ReuseRecord = false

	// String interning: the CSV contains very few unique proxy types and
	// country codes, so deduplicate them to keep memory usage down.
	pool := make(map[string]string)
	intern := func(s string) string {
		if v, ok := pool[s]; ok {
			return v
		}
		pool[s] = s
		return s
	}

	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read ip2proxy csv failed: %w", err)
	}

	records := make([]ip2ProxyCSVRecord, 0, len(rows))
	skipped := 0
	for _, row := range rows {
		if len(row) < 5 {
			skipped++
			continue
		}
		from, err := decimalToIPv6Bytes(strings.TrimSpace(row[0]))
		if err != nil {
			skipped++
			continue
		}
		to, err := decimalToIPv6Bytes(strings.TrimSpace(row[1]))
		if err != nil {
			skipped++
			continue
		}
		proxyType := intern(strings.ToUpper(strings.TrimSpace(row[2])))
		country := intern(strings.ToUpper(strings.TrimSpace(row[3])))
		records = append(records, ip2ProxyCSVRecord{
			from:      from,
			to:        to,
			proxyType: proxyType,
			country:   country,
		})
	}

	sort.Slice(records, func(i, j int) bool {
		return compareIPv6Bytes(records[i].from, records[j].from) < 0
	})

	log.Printf("[ip2proxy-csv] loaded %d records (%d skipped) in %v from %s",
		len(records), skipped, time.Since(start), path)

	return &IP2ProxyCSV{records: records}, nil
}

// Close releases the in-memory records.
func (c *IP2ProxyCSV) Close() {
	c.records = nil
}

// Query looks up an IP in the CSV and returns (isProxy, proxyType, country, error).
func (c *IP2ProxyCSV) Query(ip string) (bool, string, string, error) {
	ipBytes, err := parseIPToIPv6Bytes(ip)
	if err != nil {
		return false, "", "", err
	}

	idx := sort.Search(len(c.records), func(i int) bool {
		return compareIPv6Bytes(c.records[i].from, ipBytes) >= 0
	})

	// The matching interval, if any, ends at idx-1 because records[idx].from
	// is the first range start not less than the IP.
	if idx > 0 {
		rec := c.records[idx-1]
		if compareIPv6Bytes(rec.from, ipBytes) <= 0 && compareIPv6Bytes(ipBytes, rec.to) <= 0 {
			return true, rec.proxyType, rec.country, nil
		}
	}
	return false, "", "", nil
}

func decimalToIPv6Bytes(s string) ([16]byte, error) {
	var out [16]byte
	if s == "" {
		return out, fmt.Errorf("empty decimal ip number")
	}
	n := new(big.Int)
	if _, ok := n.SetString(s, 10); !ok {
		return out, fmt.Errorf("invalid decimal ip number: %s", s)
	}
	b := n.Bytes()
	if len(b) > 16 {
		return out, fmt.Errorf("decimal ip number too large: %s", s)
	}
	copy(out[16-len(b):], b)
	return out, nil
}

func parseIPToIPv6Bytes(ip string) ([16]byte, error) {
	var out [16]byte
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return out, fmt.Errorf("invalid ip: %s", ip)
	}
	if v4 := parsed.To4(); v4 != nil {
		// IPv4-mapped IPv6 ::ffff:x.x.x.x
		out[10] = 0xff
		out[11] = 0xff
		copy(out[12:], v4)
		return out, nil
	}
	copy(out[:], parsed.To16())
	return out, nil
}

func compareIPv6Bytes(a, b [16]byte) int {
	for i := 0; i < 16; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
