package iconlib

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// liveDB opens the dev database from PG_* env vars and skips when the icon
// library is not seeded. The tests only read.
func liveDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("PG_HOST") == "" {
		t.Skip("PG_* not set")
	}
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("PG_HOST"), os.Getenv("PG_PORT"), os.Getenv("PG_USER_NAME"),
		os.Getenv("PG_PASSWORD"), os.Getenv("PG_DB_NAME")))
	if err != nil {
		t.Skip(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM shared.icon_library`).Scan(&n); err != nil || n == 0 {
		t.Skip("shared.icon_library missing or empty")
	}
	return db
}

func contains(icons []Icon, name string) bool {
	for _, ic := range icons {
		if ic.Name == name {
			return true
		}
	}
	return false
}

func TestLiveSearchKeywordAndChinese(t *testing.T) {
	db := liveDB(t)
	defer db.Close()
	for _, c := range []struct{ q, want string }{
		{"reload", "refresh-cw"},
		{"刷新", "refresh-cw"},
		{"wheelchair", "accessibility"},
	} {
		got, err := Search(context.Background(), db, c.q, "", 20)
		if err != nil {
			t.Fatalf("%s: %v", c.q, err)
		}
		if !contains(got, c.want) {
			t.Errorf("query %q: %s not in top 20", c.q, c.want)
		}
	}
}

func TestLiveGetSVG(t *testing.T) {
	db := liveDB(t)
	defer db.Close()
	svg, v, err := GetSVG(context.Background(), db, "refresh-cw")
	if err != nil || v < 1 || len(svg) < 20 {
		t.Fatalf("svg=%q v=%d err=%v", svg, v, err)
	}
	if _, _, err := GetSVG(context.Background(), db, "no-such-icon-xyz"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
