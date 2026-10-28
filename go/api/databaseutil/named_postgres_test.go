package databaseutil

import (
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"testing"
)

func TestNamedPostgresPoolsReuseAndClose(t *testing.T) {
	cfg := ApiTypes.DatabaseConfig{Host: "localhost", Port: 5432, UserName: "test", Password: "secret", MaxConnections: 3}
	first, err := NamedPostgresPool(cfg, "example")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NamedPostgresPool(cfg, "example")
	if err != nil || first != second {
		t.Fatalf("pool not reused: %v", err)
	}
	closeNamedPostgresPools()
	third, err := NamedPostgresPool(cfg, "example")
	if err != nil || third == first {
		t.Fatalf("pool not recreated: %v", err)
	}
	closeNamedPostgresPools()
}
