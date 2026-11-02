package emojilib

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func TestLiveCatalogue(t *testing.T) {
	if os.Getenv("EMOJI_TEST_DATABASE") != "1" {
		t.Skip("set EMOJI_TEST_DATABASE=1 for the seeded integration database")
	}
	db, err := sql.Open("postgres", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for _, query := range []string{"庆祝", "celebration"} {
		page, err := Search(ctx, db, query, "", 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range page.Items {
			if e.Unicode == "🎉" {
				found = true
			}
		}
		if !found {
			t.Errorf("%q did not find party popper in top 10", query)
		}
	}
	mixed, err := Search(ctx, db, "celebration 🎉", "", 10, 0)
	if err != nil || len(mixed.Items) == 0 {
		t.Fatalf("mixed query returned no results: %v", err)
	}
	page, err := Search(ctx, db, "👩‍❤️‍👩", "", 30, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].Unicode != "👩‍❤️‍👩" {
		t.Fatalf("exact sequence: %+v %v", page, err)
	}
	seen := map[string]bool{}
	offset := 0
	for {
		p, err := Search(ctx, db, "", "people-body", 200, offset)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range p.Items {
			if seen[e.ID] {
				t.Fatalf("duplicate %s", e.ID)
			}
			seen[e.ID] = true
		}
		if p.NextOffset == nil {
			break
		}
		offset = *p.NextOffset
	}
	if len(seen) <= 200 {
		t.Fatalf("pagination test needs >200 entries, got %d", len(seen))
	}
	svg, hash, err := SVG(ctx, db, "1f389")
	if err != nil || svg == "" || len(hash) != 64 {
		t.Fatalf("SVG: %q %v", hash, err)
	}
}
