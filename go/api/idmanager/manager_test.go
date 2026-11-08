package idmanager

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func mockManager(t *testing.T) (*Manager, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	m, err := New(db, "test-worker")
	if err != nil {
		t.Fatal(err)
	}
	return m, mock
}

const readSQL = `SELECT next_id, start_id, max_id, allocate_size, allow_reuse FROM shared.id_mgr WHERE id_name = $1 FOR UPDATE`
const updateSQL = `UPDATE shared.id_mgr SET next_id = $1, last_requester = $2, updated_at = CURRENT_TIMESTAMP WHERE id_name = $3`

func expectRead(mock sqlmock.Sqlmock, next, start, max, size int64, reuse bool) {
	mock.ExpectBegin()
	mock.ExpectQuery(readSQL).WithArgs("test").WillReturnRows(sqlmock.NewRows([]string{"next_id", "start_id", "max_id", "allocate_size", "allow_reuse"}).AddRow(next, start, max, size, reuse))
}

func expectBlock(mock sqlmock.Sqlmock, next, start, max, size, after int64, reuse bool) {
	expectRead(mock, next, start, max, size, reuse)
	mock.ExpectExec(updateSQL).WithArgs(after, "test-worker", "test").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func TestValidation(t *testing.T) {
	if _, err := New(nil, "worker"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil DB: %v", err)
	}
	m, _ := mockManager(t)
	if _, err := New(m.db, " "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty requester: %v", err)
	}
	for _, name := range []string{"", "  ", strings.Repeat("x", 129), "bad\x00name", string([]byte{255})} {
		if r, err := m.Generate(context.Background(), name, 1); r != (Range{}) || !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("name %q: %+v %v", name, r, err)
		}
	}
	for _, count := range []int64{0, -1} {
		if r, err := m.Generate(context.Background(), "test", count); r != (Range{}) || !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("count %d: %+v %v", count, r, err)
		}
	}
	for _, seq := range []Sequence{{Name: "test", StartID: -1}, {Name: "test", MaxID: math.MaxInt64}, {Name: "test", StartID: 3, MaxID: 2}, {Name: "test", AllocateSize: -1}, {Name: "global", AllowReuse: true}, {Name: "activity_log_id", AllowReuse: true}, {Name: "llm_run_id", AllowReuse: true}} {
		if err := m.Provision(context.Background(), seq); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("sequence %+v: %v", seq, err)
		}
	}
}

func TestCacheAndAbandonedTail(t *testing.T) {
	m, mock := mockManager(t)
	expectBlock(mock, 1, 1, 100, 5, 6, false)
	if id, err := m.Next(context.Background(), "test"); id != 1 || err != nil {
		t.Fatalf("%d %v", id, err)
	}
	if r, err := m.Generate(context.Background(), "test", 3); r != (Range{2, 3}) || err != nil {
		t.Fatalf("%+v %v", r, err)
	}
	expectBlock(mock, 6, 1, 100, 5, 11, false)
	if r, err := m.Generate(context.Background(), "test", 2); r != (Range{6, 2}) || err != nil {
		t.Fatalf("%+v %v", r, err)
	}
	if r, err := m.Generate(context.Background(), "test", 3); r != (Range{8, 3}) || err != nil {
		t.Fatalf("%+v %v", r, err)
	}
	expectBlock(mock, 11, 1, 100, 5, 21, false)
	if r, err := m.Generate(context.Background(), "test", 10); r != (Range{11, 10}) || err != nil {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestFailedRefillPreservesCache(t *testing.T) {
	for _, stage := range []string{"begin", "read", "update", "commit"} {
		t.Run(stage, func(t *testing.T) {
			m, mock := mockManager(t)
			expectBlock(mock, 1, 1, 100, 5, 6, false)
			if _, err := m.Generate(context.Background(), "test", 4); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("injected failure")
			if stage == "begin" {
				mock.ExpectBegin().WillReturnError(failure)
			} else {
				if stage == "read" {
					mock.ExpectBegin()
					mock.ExpectQuery(readSQL).WithArgs("test").WillReturnError(failure)
				} else {
					expectRead(mock, 6, 1, 100, 5, false)
					exec := mock.ExpectExec(updateSQL).WithArgs(int64(11), "test-worker", "test")
					if stage == "update" {
						exec.WillReturnError(failure)
					} else {
						exec.WillReturnResult(sqlmock.NewResult(0, 1))
						mock.ExpectCommit().WillReturnError(failure)
					}
				}
				if stage != "commit" {
					mock.ExpectRollback()
				}
			}
			if r, err := m.Generate(context.Background(), "test", 2); r != (Range{}) || !errors.Is(err, ErrDatabase) || !errors.Is(err, failure) {
				t.Fatalf("%+v %v", r, err)
			}
			if id, err := m.Next(context.Background(), "test"); id != 5 || err != nil {
				t.Fatalf("old tail: %d %v", id, err)
			}
			// An uncertain commit may have advanced the DB. Never publish its block.
			expectBlock(mock, 11, 1, 100, 5, 16, false)
			if id, err := m.Next(context.Background(), "test"); id != 11 || err != nil {
				t.Fatalf("new reservation: %d %v", id, err)
			}
		})
	}
}

func TestBoundsAndReuse(t *testing.T) {
	for _, tc := range []struct {
		name                                        string
		next, start, max, size, count, after, first int64
		reuse                                       bool
		want                                        error
	}{
		{"final", math.MaxInt64 - 1, 1, math.MaxInt64 - 1, 5, 1, math.MaxInt64, math.MaxInt64 - 1, false, nil},
		{"exhausted", 11, 1, 10, 5, 1, 0, 0, false, ErrExhausted},
		{"insufficient", 9, 1, 10, 5, 3, 0, 0, false, ErrInsufficientRange},
		{"wrap tail", 9, 1, 10, 5, 3, 6, 1, true, nil},
		{"wrap exact", 9, 1, 10, 5, 2, 1, 9, true, nil},
		{"too large", 9, 1, 10, 5, 11, 0, 0, true, ErrInsufficientRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mock := mockManager(t)
			if tc.want == nil {
				expectBlock(mock, tc.next, tc.start, tc.max, tc.size, tc.after, tc.reuse)
			} else {
				expectRead(mock, tc.next, tc.start, tc.max, tc.size, tc.reuse)
				mock.ExpectRollback()
			}
			r, err := m.Generate(context.Background(), "test", tc.count)
			if tc.want != nil {
				if r != (Range{}) || !errors.Is(err, tc.want) {
					t.Fatalf("%+v %v", r, err)
				}
				return
			}
			if err != nil || r != (Range{tc.first, tc.count}) {
				t.Fatalf("%+v %v", r, err)
			}
		})
	}
}

func TestUnknownSequenceAndCancellation(t *testing.T) {
	m, mock := mockManager(t)
	mock.ExpectBegin()
	mock.ExpectQuery(readSQL).WithArgs("test").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	if id, err := m.Next(context.Background(), "test"); id != 0 || !errors.Is(err, ErrUnknownSequence) {
		t.Fatalf("%d %v", id, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if id, err := m.Next(ctx, "test"); id != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("%d %v", id, err)
	}
	// A blocked refill holds only this sequence's lock; a second caller can cancel.
	mock.ExpectBegin()
	mock.ExpectQuery(readSQL).WithArgs("test").WillDelayFor(100 * time.Millisecond).WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()
	done := make(chan error, 1)
	go func() { _, err := m.Next(context.Background(), "test"); done <- err }()
	// Wait for the lock to be held without relying on scheduler timing.
	state := m.sequence("test")
	deadline := time.After(time.Second)
	for len(state.lock) == 0 {
		select {
		case <-deadline:
			t.Fatal("refill did not acquire lock")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if id, err := m.Next(ctx, "test"); id != 0 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%d %v", id, err)
	}
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
