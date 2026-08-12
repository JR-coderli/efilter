package main

import (
	"encoding/csv"
	"fmt"
	"math/big"
	"net"
	"os"
	"sort"
	"strings"

	ip2proxy "github.com/ip2location/ip2proxy-go/v4"
)

type ip6Record struct {
	from      [16]byte
	to        [16]byte
	proxyType string
	country   string
}

func main() {
	binPath := "../../binfiles/IP2PROXY-LITE-PX2.BIN/IP2PROXY-LITE-PX2.BIN"
	csvPath := "../../binfiles/IP2PROXY-LITE-PX2.IPV6.CSV/IP2PROXY-LITE-PX2.IPV6.CSV"

	binDB, err := ip2proxy.OpenDB(binPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open BIN failed: %v\n", err)
		os.Exit(1)
	}
	defer binDB.Close()
	fmt.Println("BIN opened")

	records := loadCSV(csvPath)
	fmt.Printf("CSV loaded %d records\n", len(records))

	found := 0
	for i, r := range records {
		probe := midIP6(r.from, r.to)
		ip := net.IP(probe[:]).String()

		// Skip IPv4-mapped IPv6 ::ffff:x.x.x.x
		if isIPv4Mapped(probe) {
			continue
		}

		binRec, err := binDB.GetAll(ip)
		binHit := err == nil && binRec.IsProxy == 1

		if !binHit {
			fmt.Printf("CSV-only hit #%d (CSV row %d): %s proxy_type=%s\n", found+1, i, ip, r.proxyType)
			fmt.Printf("  curl -s -X POST http://127.0.0.1:8080/api/v1/check -H 'X-API-Key: risk-engine-dev-key-2026' -H 'Content-Type: application/json' -d '{\"ip\":\"%s\"}'\n", ip)
			found++
			if found >= 10 {
				break
			}
		}
	}

	if found == 0 {
		fmt.Println("No CSV-only IPv6 hits found; BIN covers all CSV IPv6 ranges in this sample.")
	}
}

func loadCSV(path string) []ip6Record {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		panic(err)
	}

	var records []ip6Record
	for _, row := range rows {
		if len(row) < 5 {
			continue
		}
		from, err := decimalToIP6(strings.Trim(row[0], "\""))
		if err != nil {
			continue
		}
		to, err := decimalToIP6(strings.Trim(row[1], "\""))
		if err != nil {
			continue
		}
		records = append(records, ip6Record{
			from:      from,
			to:        to,
			proxyType: strings.ToUpper(strings.Trim(row[2], "\"")),
			country:   strings.ToUpper(strings.Trim(row[3], "\"")),
		})
	}

	sort.Slice(records, func(i, j int) bool {
		return compareIP6(records[i].from, records[j].from) < 0
	})
	return records
}

func decimalToIP6(s string) ([16]byte, error) {
	var out [16]byte
	n := new(big.Int)
	if _, ok := n.SetString(s, 10); !ok {
		return out, fmt.Errorf("invalid decimal")
	}
	b := n.Bytes()
	if len(b) > 16 {
		return out, fmt.Errorf("too large")
	}
	copy(out[16-len(b):], b)
	return out, nil
}

func compareIP6(a, b [16]byte) int {
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

func midIP6(from, to [16]byte) [16]byte {
	var out [16]byte
	carry := 0
	for i := 15; i >= 0; i-- {
		sum := int(from[i]) + int(to[i]) + carry
		out[i] = byte(sum / 2)
		carry = (sum % 2) * 256
	}
	return out
}

func isIPv4Mapped(b [16]byte) bool {
	return b[0] == 0 && b[1] == 0 && b[2] == 0 && b[3] == 0 &&
		b[4] == 0 && b[5] == 0 && b[6] == 0 && b[7] == 0 &&
		b[8] == 0 && b[9] == 0 && b[10] == 0xff && b[11] == 0xff
}
