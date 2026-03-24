package ipdb

import (
	"fmt"
	"net"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/oschwald/maxminddb-golang"
)

// reader wraps the open MMDB file. Protected by mu so it can be hot-swapped
// after a sync without restarting the server.
var (
	mu     sync.RWMutex
	reader *maxminddb.Reader
)

// openDB opens (or re-opens) the MMDB file at path.
// Any previously opened reader is closed first.
func openDB(path string) error {
	mu.Lock()
	defer mu.Unlock()

	r, err := maxminddb.Open(path)
	if err != nil {
		return fmt.Errorf("ipdb: failed to open MMDB file (SHD_IPD_220): %w", err)
	}

	if reader != nil {
		_ = reader.Close()
	}
	reader = r
	return nil
}

// closeDB closes the MMDB reader. Called on shutdown.
func closeDB() {
	mu.Lock()
	defer mu.Unlock()
	if reader != nil {
		_ = reader.Close()
		reader = nil
	}
}

// lookupFromMMDB performs a raw lookup against the open MMDB reader.
func lookupFromMMDB(ip string) (*IPRecord, error) {
	mu.RLock()
	r := reader
	mu.RUnlock()

	if r == nil {
		return nil, fmt.Errorf("ipdb: MMDB not loaded yet (SHD_IPD_248)")
	}

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil, fmt.Errorf("ipdb: invalid IP address (SHD_IPD_253): %s", ip)
	}

	var rec mmdbRecord
	if err := r.Lookup(parsed, &rec); err != nil {
		return nil, fmt.Errorf("ipdb: MMDB lookup failed (SHD_IPD_258): %w", err)
	}

	return &IPRecord{
		IP:            ip,
		ASNNumber:     rec.ASN.Number,
		ASNOrg:        rec.ASN.Organization,
		CountryName:   rec.Country.Name,
		CountryISO:    rec.Country.ISOCode,
		ContinentName: rec.Continent.Name,
		ContinentCode: rec.Continent.Code,
	}, nil
}

// LookupIP returns geolocation data for ip.
// Results are cached in PostgreSQL for cacheTTLDays days.
func LookupIP(logger ApiTypes.JimoLogger, ip string) (*IPRecord, error) {
	if err := ValidateIP(ip); err != nil {
		return nil, err
	}

	db := ApiTypes.SharedDBHandle
	if db != nil {
		cached, err := getCachedRecord(db, ip, svc.cacheTTLDays)
		if err != nil {
			logger.Warn("ipdb: cache read error, falling back to MMDB", "error", err, "ip", ip)
		} else if cached != nil {
			logger.Info("ipdb: cache hit", "ip", ip)
			return cached, nil
		}
	}

	rec, err := lookupFromMMDB(ip)
	if err != nil {
		return nil, err
	}

	if db != nil {
		if err := upsertCachedRecord(db, rec); err != nil {
			logger.Warn("ipdb: cache write error", "error", err, "ip", ip)
		}
	}

	return rec, nil
}
