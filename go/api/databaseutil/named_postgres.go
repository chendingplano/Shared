package databaseutil

import (
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	_ "github.com/lib/pq"
)

var namedPostgresPools = struct {
	sync.Mutex
	pools map[string]*sql.DB
}{pools: make(map[string]*sql.DB)}

// NamedPostgresPool returns a centrally managed pool for a named database on the configured PostgreSQL server.
// It opens connections lazily; callers must authorize database names before calling it.
func NamedPostgresPool(cfg ApiTypes.DatabaseConfig, name string) (*sql.DB, error) {
	if name == "" {
		return nil, fmt.Errorf("database name is required")
	}
	dsn := &url.URL{Scheme: "postgres", User: url.UserPassword(cfg.UserName, cfg.Password), Host: net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port)), Path: "/" + name}
	query := dsn.Query()
	query.Set("sslmode", "disable")
	dsn.RawQuery = query.Encode()
	key := dsn.String()
	namedPostgresPools.Lock()
	defer namedPostgresPools.Unlock()
	if db := namedPostgresPools.pools[key]; db != nil {
		return db, nil
	}
	db, err := sql.Open("postgres", key)
	if err != nil {
		return nil, err
	}
	if cfg.MaxConnections > 0 {
		db.SetMaxOpenConns(cfg.MaxConnections)
	}
	namedPostgresPools.pools[key] = db
	return db, nil
}

func closeNamedPostgresPools() {
	namedPostgresPools.Lock()
	defer namedPostgresPools.Unlock()
	for key, db := range namedPostgresPools.pools {
		_ = db.Close()
		delete(namedPostgresPools.pools, key)
	}
}
