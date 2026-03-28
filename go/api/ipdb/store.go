package ipdb

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
	"github.com/google/uuid"
)

const (
	cacheTable   = "ipdb_lookup_cache"
	syncLogTable = "ipdb_sync_log"
)

// CreateTables creates all tables required by the ipdb service.
// This is called from sysdatastores.CreateSysTables.
func CreateTables(logger ApiTypes.JimoLogger) error {
	db := ApiTypes.SharedDBHandle
	if err := createCacheTable(logger, db); err != nil {
		return err
	}
	return createSyncLogTable(logger, db)
}

func createCacheTable(logger ApiTypes.JimoLogger, db *sql.DB) error {
	logger.Info("ipdb: creating table", "table", cacheTable)
	stmt := `CREATE TABLE IF NOT EXISTS ` + cacheTable + ` (
		ip              VARCHAR(45)  PRIMARY KEY,
		asn_number      BIGINT       NOT NULL DEFAULT 0,
		asn_org         VARCHAR(256) NOT NULL DEFAULT '',
		country_name    VARCHAR(128) NOT NULL DEFAULT '',
		country_iso     VARCHAR(10)  NOT NULL DEFAULT '',
		continent_name  VARCHAR(64)  NOT NULL DEFAULT '',
		continent_code  VARCHAR(10)  NOT NULL DEFAULT '',
		looked_up_at    TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	)`
	if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
		return fmt.Errorf("ipdb: failed creating %s (SHD_IPD_044): %w", cacheTable, err)
	}
	// Index for time-based expiry cleanup
	databaseutil.ExecuteStatement(db,
		`CREATE INDEX IF NOT EXISTS idx_ipdb_cache_looked_up_at ON `+cacheTable+` (looked_up_at)`)
	logger.Info("ipdb: table ready", "table", cacheTable)
	return nil
}

func createSyncLogTable(logger ApiTypes.JimoLogger, db *sql.DB) error {
	logger.Info("ipdb: creating table", "table", syncLogTable)
	stmt := `CREATE TABLE IF NOT EXISTS ` + syncLogTable + ` (
		id          VARCHAR(40)   PRIMARY KEY DEFAULT gen_random_uuid()::text,
		status      VARCHAR(20)   NOT NULL,
		file_size   BIGINT        NOT NULL DEFAULT 0,
		error_msg   TEXT          NOT NULL DEFAULT '',
		synced_at   TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	)`
	if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
		return fmt.Errorf("ipdb: failed creating %s (SHD_IPD_063): %w", syncLogTable, err)
	}
	databaseutil.ExecuteStatement(db,
		`CREATE INDEX IF NOT EXISTS idx_ipdb_sync_log_synced_at ON `+syncLogTable+` (synced_at DESC)`)
	logger.Info("ipdb: table ready", "table", syncLogTable)
	return nil
}

// getCachedRecord retrieves a cached record for ip, or returns nil if not found / expired.
func getCachedRecord(db *sql.DB, ip string, ttlDays int) (*IPRecord, error) {
	cutoff := time.Now().AddDate(0, 0, -ttlDays)
	row := db.QueryRow(
		`SELECT ip, asn_number, asn_org, country_name, country_iso,
		        continent_name, continent_code, looked_up_at
		   FROM `+cacheTable+`
		  WHERE ip = $1 AND looked_up_at > $2`,
		ip, cutoff)

	rec := &IPRecord{}
	err := row.Scan(
		&rec.IP, &rec.ASNNumber, &rec.ASNOrg,
		&rec.CountryName, &rec.CountryISO,
		&rec.ContinentName, &rec.ContinentCode, &rec.LookedUpAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("ipdb: cache scan failed (SHD_IPD_089): %w", err)
	}
	return rec, nil
}

// upsertCachedRecord writes (or refreshes) a cache entry.
func upsertCachedRecord(db *sql.DB, rec *IPRecord) error {
	_, err := db.Exec(
		`INSERT INTO `+cacheTable+`
		        (ip, asn_number, asn_org, country_name, country_iso,
		         continent_name, continent_code, looked_up_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,NOW(),NOW())
		 ON CONFLICT (ip) DO UPDATE SET
		        asn_number     = EXCLUDED.asn_number,
		        asn_org        = EXCLUDED.asn_org,
		        country_name   = EXCLUDED.country_name,
		        country_iso    = EXCLUDED.country_iso,
		        continent_name = EXCLUDED.continent_name,
		        continent_code = EXCLUDED.continent_code,
		        looked_up_at   = NOW()`,
		rec.IP, rec.ASNNumber, rec.ASNOrg,
		rec.CountryName, rec.CountryISO,
		rec.ContinentName, rec.ContinentCode)
	if err != nil {
		return fmt.Errorf("ipdb: cache upsert failed (SHD_IPD_113): %w", err)
	}
	return nil
}

// purgeStaleCacheEntries removes entries older than ttlDays.
func purgeStaleCacheEntries(db *sql.DB, ttlDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -ttlDays)
	res, err := db.Exec(`DELETE FROM `+cacheTable+` WHERE looked_up_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("ipdb: cache purge failed (SHD_IPD_122): %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// writeSyncLog records the result of a sync attempt.
func writeSyncLog(db *sql.DB, status string, fileSize int64, errMsg string) error {
	_, err := db.Exec(
		`INSERT INTO `+syncLogTable+` (id, status, file_size, error_msg, synced_at)
		 VALUES ($1, $2, $3, $4, NOW())`,
		uuid.New().String(), status, fileSize, errMsg)
	if err != nil {
		return fmt.Errorf("ipdb: sync log insert failed (SHD_IPD_135): %w", err)
	}
	return nil
}

// GetLastSyncStatus returns the most recent sync log entry.
func GetLastSyncStatus(db *sql.DB) (*SyncStatus, error) {
	row := db.QueryRow(
		`SELECT id, status, file_size, error_msg, synced_at
		   FROM ` + syncLogTable + `
		  ORDER BY synced_at DESC LIMIT 1`)

	s := &SyncStatus{}
	err := row.Scan(&s.ID, &s.Status, &s.FileSize, &s.ErrorMsg, &s.SyncedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("ipdb: sync status query failed (SHD_IPD_150): %w", err)
	}
	return s, nil
}

// ValidateIP returns an error if ip is not a valid IPv4 or IPv6 address.
func ValidateIP(ip string) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP address (SHD_IPD_158): %s", ip)
	}
	return nil
}
