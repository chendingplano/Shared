package idmanager

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/lib/pq"
	gooselib "github.com/pressly/goose/v3"
)

// Set IDMANAGER_TEST_DSN to a disposable database named idmanager_test_*.
// These tests replace shared.id_mgr and the test database's Goose history.
func TestPostgres(t *testing.T) {
	dsn := os.Getenv("IDMANAGER_TEST_DSN")
	if dsn == "" {
		t.Skip("set IDMANAGER_TEST_DSN for isolated PostgreSQL integration tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var database string
	if err := db.QueryRow(`SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(database, "idmanager_test_") {
		t.Fatal("integration database name must start with idmanager_test_")
	}
	ctx := context.Background()
	data, err := os.ReadFile("../goose/shared_migrations/20261108000001_id_manager.sql")
	if err != nil {
		t.Fatal(err)
	}
	reset := func(t *testing.T, legacy bool) *gooselib.Provider {
		t.Helper()
		if _, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS shared; DROP TABLE IF EXISTS shared.id_mgr; DROP TABLE IF EXISTS public.goose_db_version`); err != nil {
			t.Fatal(err)
		}
		if legacy {
			_, err := db.Exec(`CREATE TABLE shared.id_mgr (
			 id_name VARCHAR(128) PRIMARY KEY, crt_value BIGINT NOT NULL, id_desc TEXT NOT NULL,
			 caller_loc VARCHAR(20) NOT NULL, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			 updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
			 INSERT INTO shared.id_mgr(id_name,crt_value,id_desc,caller_loc) VALUES ('activity_log_id',14000,'old activity','old-location')`)
			if err != nil {
				t.Fatal(err)
			}
		}
		p, err := gooselib.NewProvider(gooselib.DialectPostgres, db, fstest.MapFS{"20261108000001_id_manager.sql": {Data: data}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Up(ctx); err != nil {
			t.Fatal(err)
		}
		return p
	}
	manager := func(t *testing.T, pool *sql.DB, requester string) *Manager {
		t.Helper()
		m, err := New(pool, requester)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	provision := func(t *testing.T, m *Manager, seq Sequence) {
		t.Helper()
		if err := m.Provision(ctx, seq); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("migration fresh legacy and rollback", func(t *testing.T) {
		for _, legacy := range []bool{false, true} {
			p := reset(t, legacy)
			if _, err := p.Up(ctx); err != nil {
				t.Fatal(err)
			}
			// Exercise SQL idempotency too, not just Goose version tracking.
			up := strings.Split(string(data), "-- +goose Down")[0]
			if _, err := db.Exec(up); err != nil {
				t.Fatal(err)
			}
			m := manager(t, db, "migration-test")
			if id, err := m.Next(ctx, GlobalSequence); id != 1 || err != nil {
				t.Fatalf("global %d %v", id, err)
			}
			if legacy {
				provision(t, m, Sequence{Name: "activity_log_id", AllocateSize: 1000})
				if id, err := m.Next(ctx, "activity_log_id"); id != 15000 || err != nil {
					t.Fatalf("converted activity ID %d %v", id, err)
				}
				var creator string
				var size int
				if err := db.QueryRow(`SELECT creator,allocate_size FROM shared.id_mgr WHERE id_name='activity_log_id'`).Scan(&creator, &size); err != nil {
					t.Fatal(err)
				}
				if creator != "old-location" || size != 1000 {
					t.Fatalf("conversion: %q %d", creator, size)
				}
			}
			if _, err := p.Down(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := p.Up(ctx); err != nil {
				t.Fatal(err)
			}
			m = manager(t, db, "after-rollback")
			if id, err := m.Next(ctx, GlobalSequence); id <= 1 || err != nil {
				t.Fatalf("rollback reused counter: %d %v", id, err)
			}
		}
	})
	t.Run("constraints", func(t *testing.T) {
		reset(t, false)
		for _, change := range []string{"start_id=0", "max_id=0", "max_id=9223372036854775807", "allocate_size=0", "next_id=0", "max_id=10,next_id=12", "allow_reuse=true", "id_name=''"} {
			if _, err := db.Exec("UPDATE shared.id_mgr SET " + change + " WHERE id_name='global'"); err == nil {
				t.Fatalf("accepted %s", change)
			}
		}
	})
	t.Run("provisioning audit and exhaustion with live cache", func(t *testing.T) {
		reset(t, false)
		m := manager(t, db, "creator")
		seq := Sequence{Name: "small", Desc: "original", StartID: 3, MaxID: 7, AllocateSize: 5}
		provision(t, m, seq)
		seq.Desc = "ignored"
		seq.AllocateSize = 1
		provision(t, m, seq)
		for _, conflicting := range []Sequence{{Name: "small", StartID: 2, MaxID: 7}, {Name: "small", StartID: 3, MaxID: 8}, {Name: "small", StartID: 3, MaxID: 7, AllowReuse: true}} {
			if err := m.Provision(ctx, conflicting); !errors.Is(err, ErrProvisionConflict) {
				t.Fatal(err)
			}
		}
		if id, err := m.Next(ctx, "small"); id != 3 || err != nil {
			t.Fatalf("%d %v", id, err)
		}
		var next int64
		var desc, creator, requester string
		var size int
		if err := db.QueryRow(`SELECT next_id,id_desc,creator,last_requester,allocate_size FROM shared.id_mgr WHERE id_name='small'`).Scan(&next, &desc, &creator, &requester, &size); err != nil {
			t.Fatal(err)
		}
		if next != 8 || desc != "original" || creator != "creator" || requester != "creator" || size != 5 {
			t.Fatalf("audit %d %q %q %q %d", next, desc, creator, requester, size)
		}
		if r, err := m.Generate(ctx, "small", 5); r != (Range{}) || !errors.Is(err, ErrExhausted) {
			t.Fatalf("%+v %v", r, err)
		}
		if r, err := m.Generate(ctx, "small", 4); r != (Range{4, 4}) || err != nil {
			t.Fatalf("cached after exhaustion %+v %v", r, err)
		}
	})
	t.Run("independent connections and concurrent managers", func(t *testing.T) {
		reset(t, false)
		other, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer other.Close()
		managers := []*Manager{manager(t, db, "worker-one"), manager(t, other, "worker-two")}
		for _, name := range []string{"a", "b"} {
			provision(t, managers[0], Sequence{Name: name, AllocateSize: 17})
		}
		type result struct {
			name string
			r    Range
			err  error
		}
		results := make(chan result, 800)
		var wg sync.WaitGroup
		for worker := 0; worker < 8; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for i := 0; i < 100; i++ {
					name := []string{"a", "b"}[i%2]
					r, err := managers[worker%2].Generate(ctx, name, int64(i%5+1))
					results <- result{name, r, err}
				}
			}(worker)
		}
		wg.Wait()
		close(results)
		seen := map[string]map[int64]bool{"a": {}, "b": {}}
		for result := range results {
			if result.err != nil {
				t.Fatal(result.err)
			}
			for id := result.r.FirstID; id < result.r.FirstID+result.r.NumIDs; id++ {
				if seen[result.name][id] {
					t.Fatalf("duplicate %s/%d", result.name, id)
				}
				seen[result.name][id] = true
			}
		}
		if len(seen["a"]) != 1200 || len(seen["b"]) != 1200 {
			t.Fatalf("wrong ID counts %d %d", len(seen["a"]), len(seen["b"]))
		}
	})
	t.Run("restart gaps reuse and rejected oversized request", func(t *testing.T) {
		reset(t, false)
		m := manager(t, db, "before-restart")
		provision(t, m, Sequence{Name: "restart", AllocateSize: 5})
		if id, err := m.Next(ctx, "restart"); id != 1 || err != nil {
			t.Fatalf("%d %v", id, err)
		}
		m = manager(t, db, "after-restart")
		if id, err := m.Next(ctx, "restart"); id != 6 || err != nil {
			t.Fatalf("%d %v", id, err)
		}
		provision(t, m, Sequence{Name: "reuse", StartID: 5, MaxID: 9, AllocateSize: 3, AllowReuse: true})
		for _, want := range []Range{{5, 3}, {5, 3}, {8, 2}, {5, 5}} {
			if r, err := m.Generate(ctx, "reuse", want.NumIDs); r != want || err != nil {
				t.Fatalf("%+v want %+v: %v", r, want, err)
			}
		}
		if r, err := m.Generate(ctx, "reuse", 6); r != (Range{}) || !errors.Is(err, ErrInsufficientRange) {
			t.Fatalf("%+v %v", r, err)
		}
		if id, err := m.Next(ctx, "reuse"); id != 5 || err != nil {
			t.Fatalf("failed request advanced counter: %d %v", id, err)
		}
	})
	t.Run("update failure rolls back and preserves old cache", func(t *testing.T) {
		reset(t, false)
		m := manager(t, db, "rollback-test")
		provision(t, m, Sequence{Name: "rollback", AllocateSize: 5})
		if r, err := m.Generate(ctx, "rollback", 4); r != (Range{1, 4}) || err != nil {
			t.Fatalf("initial block %+v %v", r, err)
		}
		_, err := db.Exec(`CREATE OR REPLACE FUNCTION shared.reject_id_update() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'injected reservation failure'; END $$;
		CREATE TRIGGER reject_id_update BEFORE UPDATE ON shared.id_mgr FOR EACH ROW EXECUTE FUNCTION shared.reject_id_update()`)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			db.Exec(`DROP TRIGGER IF EXISTS reject_id_update ON shared.id_mgr; DROP FUNCTION IF EXISTS shared.reject_id_update()`)
		})
		if r, err := m.Generate(ctx, "rollback", 2); r != (Range{}) || !errors.Is(err, ErrDatabase) {
			t.Fatalf("failed refill %+v %v", r, err)
		}
		var next int64
		if err := db.QueryRow(`SELECT next_id FROM shared.id_mgr WHERE id_name='rollback'`).Scan(&next); err != nil {
			t.Fatal(err)
		}
		if next != 6 {
			t.Fatalf("rollback left next_id=%d", next)
		}
		if id, err := m.Next(ctx, "rollback"); id != 5 || err != nil {
			t.Fatalf("old cache %d %v", id, err)
		}
		if _, err := db.Exec(`DROP TRIGGER reject_id_update ON shared.id_mgr`); err != nil {
			t.Fatal(err)
		}
		if id, err := m.Next(ctx, "rollback"); id != 6 || err != nil {
			t.Fatalf("new reservation %d %v", id, err)
		}
	})
	t.Run("row lock cancellation rollback and independent names", func(t *testing.T) {
		reset(t, false)
		m := manager(t, db, "lock-test")
		for _, name := range []string{"locked", "free"} {
			provision(t, m, Sequence{Name: name})
		}
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`SELECT next_id FROM shared.id_mgr WHERE id_name='locked' FOR UPDATE`); err != nil {
			t.Fatal(err)
		}
		cancelCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			id, err := m.Next(cancelCtx, "locked")
			if id != 0 {
				err = fmt.Errorf("failure ID=%d", id)
			}
			done <- err
		}()
		if id, err := m.Next(ctx, "free"); id != 1 || err != nil {
			t.Fatalf("independent name %d %v", id, err)
		}
		if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		if id, err := m.Next(ctx, "locked"); id != 1 || err != nil {
			t.Fatalf("cancellation advanced row: %d %v", id, err)
		}
	})
}
