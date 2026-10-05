package decisionpolicy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// TestIntegrationLifecycle runs every spec scenario against real Postgres. It
// runs only when DECISIONPOLICY_TEST_DSN is set, creates its own throwaway
// schema from the real migration file, and drops it afterwards. It never reads
// TEST_DATABASE_URL.
func TestIntegrationLifecycle(t *testing.T) {
	dsn := os.Getenv("DECISIONPOLICY_TEST_DSN")
	if dsn == "" {
		t.Skip("DECISIONPOLICY_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	schema := fmt.Sprintf("decisionpolicy_test_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, migrationUpSQL(t, schema)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	s := NewStore(db, nil, WithSchema(schema))

	// Create; duplicate name (case-insensitive); invalid input.
	p, err := s.Create(ctx, CreateInput{Name: "prompt_injection", Description: "PI", Content: "v1 text", Note: "first", Actor: "u1"})
	if err != nil || p.CurrentVersion != 1 || p.LatestVersion != 1 || p.Status != StatusActive {
		t.Fatalf("create = %+v, %v", p, err)
	}
	if _, err := s.Create(ctx, CreateInput{Name: "Prompt_Injection", Content: "x"}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("duplicate create err = %v", err)
	}
	for _, in := range []CreateInput{{Name: "", Content: "x"}, {Name: "_bad", Content: "x"}, {Name: "a b", Content: "x"}, {Name: "ok", Content: "  \n"}} {
		if _, err := s.Create(ctx, in); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("create %+v err = %v", in, err)
		}
	}

	// New versions; staged version; content immutability.
	v2, err := s.CreateVersion(ctx, p.ID, VersionInput{Content: "v2 text", Actor: "u2"})
	if err != nil || v2.Version != 2 || v2.PolicyName != "prompt_injection" {
		t.Fatalf("v2 = %+v, %v", v2, err)
	}
	staged := false
	v3, err := s.CreateVersion(ctx, p.ID, VersionInput{Content: "v3 text", MakeCurrent: &staged})
	if err != nil || v3.Version != 3 {
		t.Fatalf("v3 = %+v, %v", v3, err)
	}
	if cur, _ := s.GetCurrent(ctx, "PROMPT_INJECTION"); cur == nil || cur.Version != 2 || cur.Content != "v2 text" {
		t.Fatalf("current after staged = %+v", cur)
	}
	if v1, _ := s.GetVersion(ctx, p.ID, 1); v1 == nil || v1.Content != "v1 text" || v1.Note != "first" {
		t.Fatalf("v1 = %+v", v1)
	}

	// Rollback, then numbers keep growing; unknown version.
	if p2, err := s.SetCurrentVersion(ctx, p.ID, 1, "u3"); err != nil || p2.CurrentVersion != 1 {
		t.Fatalf("rollback = %+v, %v", p2, err)
	}
	if cur, _ := s.GetCurrent(ctx, "prompt_injection"); cur.Content != "v1 text" {
		t.Fatalf("current after rollback = %+v", cur)
	}
	if v4, err := s.CreateVersion(ctx, p.ID, VersionInput{Content: "v4 text"}); err != nil || v4.Version != 4 {
		t.Fatalf("v4 = %+v, %v", v4, err)
	}
	if _, err := s.SetCurrentVersion(ctx, p.ID, 99, "u3"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown version err = %v", err)
	}

	// Concurrent writers get distinct consecutive numbers.
	const writers = 8
	var wg sync.WaitGroup
	got := make([]int, writers)
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := s.CreateVersion(ctx, p.ID, VersionInput{Content: fmt.Sprintf("c%d", i)})
			errs[i] = err
			if v != nil {
				got[i] = v.Version
			}
		}(i)
	}
	wg.Wait()
	sort.Ints(got)
	for i := range got {
		if errs[i] != nil || got[i] != 5+i {
			t.Fatalf("concurrent versions = %v, errs = %v", got, errs)
		}
	}
	if vs, err := s.ListVersions(ctx, p.ID); err != nil || len(vs) != 12 || vs[0].Version != 12 {
		t.Fatalf("list versions = %d, %v", len(vs), err)
	}

	// Metadata edit: rename, rename to taken name, no version created.
	other, err := s.Create(ctx, CreateInput{Name: "toxicity", Content: "tox"})
	if err != nil {
		t.Fatal(err)
	}
	newName, desc := "prompt-injection", "renamed"
	if p3, err := s.Update(ctx, p.ID, UpdateInput{Name: &newName, Description: &desc, Actor: "u4"}); err != nil ||
		p3.Name != newName || p3.LatestVersion != 12 || p3.UpdatedBy != "u4" {
		t.Fatalf("rename = %+v, %v", p3, err)
	}
	taken := "TOXICITY"
	if _, err := s.Update(ctx, p.ID, UpdateInput{Name: &taken}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("rename to taken err = %v", err)
	}
	if list, err := s.List(ctx, ListFilter{NamePrefix: "prompt"}); err != nil || len(list) != 1 || list[0].ID != p.ID {
		t.Fatalf("list prefix = %+v, %v", list, err)
	}
	if list, _ := s.List(ctx, ListFilter{NamePrefix: "prompt_"}); len(list) != 0 { // '_' is literal, not a wildcard
		t.Fatalf("list with literal underscore = %+v", list)
	}

	// Soft delete: hidden by name, readable by ID, name reusable, mutations rejected.
	if err := s.Delete(ctx, other.ID, "u5"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetByName(ctx, "toxicity"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted by name err = %v", err)
	}
	if _, err := s.GetCurrent(ctx, "toxicity"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get current deleted err = %v", err)
	}
	if d, err := s.Get(ctx, other.ID); err != nil || d.Status != StatusDeleted || d.DeletedAt == nil {
		t.Fatalf("get deleted by id = %+v, %v", d, err)
	}
	if v, err := s.GetVersion(ctx, other.ID, 1); err != nil || v.Content != "tox" {
		t.Fatalf("deleted version = %+v, %v", v, err)
	}
	if again, err := s.Create(ctx, CreateInput{Name: "toxicity", Content: "tox2"}); err != nil || again.ID == other.ID || again.CurrentVersion != 1 {
		t.Fatalf("recreate = %+v, %v", again, err)
	}
	if _, err := s.Update(ctx, other.ID, UpdateInput{Description: &desc}); !errors.Is(err, ErrDeleted) {
		t.Fatalf("update deleted err = %v", err)
	}
	if _, err := s.CreateVersion(ctx, other.ID, VersionInput{Content: "x"}); !errors.Is(err, ErrDeleted) {
		t.Fatalf("version deleted err = %v", err)
	}
	if _, err := s.SetCurrentVersion(ctx, other.ID, 1, ""); !errors.Is(err, ErrDeleted) {
		t.Fatalf("set current deleted err = %v", err)
	}
	if err := s.Delete(ctx, other.ID, ""); !errors.Is(err, ErrDeleted) {
		t.Fatalf("delete deleted err = %v", err)
	}
	if _, err := s.Get(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing id err = %v", err)
	}

	// Missing tables are reported by name.
	missing := NewStore(db, nil, WithSchema(schema+"_missing"))
	if _, err := missing.Get(ctx, 1); !errors.Is(err, ErrTablesMissing) || !strings.Contains(err.Error(), "decision_policies") {
		t.Fatalf("missing tables err = %v", err)
	}
}

// migrationUpSQL returns the Up section of the app migration with the shared
// schema replaced by schema, so the test exercises the real DDL.
func migrationUpSQL(t *testing.T, schema string) string {
	t.Helper()
	path := os.Getenv("DECISIONPOLICY_MIGRATION_FILE")
	if path == "" {
		path = filepath.Join("..", "..", "..", "..", "ChenWeb", "shared_migrations", "20261005000001_create_shared_decision_policies.sql")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration (set DECISIONPOLICY_MIGRATION_FILE): %v", err)
	}
	up, _, found := strings.Cut(string(raw), "-- +goose Down")
	if !found {
		t.Fatalf("migration %s has no Down marker", path)
	}
	up = strings.ReplaceAll(up, "CREATE SCHEMA IF NOT EXISTS shared;", "CREATE SCHEMA "+schema+";")
	return strings.ReplaceAll(up, "shared.", schema+".")
}
