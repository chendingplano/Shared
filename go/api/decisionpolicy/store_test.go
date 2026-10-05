package decisionpolicy

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

var policyCols = []string{"id", "name", "description", "current_version", "latest_version", "status",
	"created_by", "updated_by", "created_at", "updated_at", "deleted_at"}

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = db.Close()
	})
	return NewStore(db, nil), mock
}

func policyRow(id int64, name string, current, latest int, status string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(policyCols).AddRow(id, name, "", current, latest, status, "u", "u", now, now, nil)
}

func q(s string) string { return regexp.QuoteMeta(s) }

func TestValidation(t *testing.T) {
	s, _ := newMockStore(t) // no SQL may run
	ctx := context.Background()
	long := make([]byte, maxNameLen+1)
	for i := range long {
		long[i] = 'a'
	}
	for _, in := range []CreateInput{
		{Name: "", Content: "x"}, {Name: string(long), Content: "x"}, {Name: "-x", Content: "x"},
		{Name: "a/b", Content: "x"}, {Name: "ok", Content: " \t\n"},
	} {
		if _, err := s.Create(ctx, in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Create(%q) err = %v", in.Name, err)
		}
	}
	if _, err := s.Update(ctx, 1, UpdateInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty update err = %v", err)
	}
	if _, err := s.CreateVersion(ctx, 1, VersionInput{Content: ""}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty version err = %v", err)
	}
	if _, err := s.List(ctx, ListFilter{Offset: -1}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("negative offset err = %v", err)
	}
	for _, name := range []string{"a", "A1", "prompt_injection.v-2"} {
		if err := validateName(name); err != nil {
			t.Errorf("validateName(%q) = %v", name, err)
		}
	}
}

func TestWithSchemaRejectsUnsafeName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for unsafe schema name")
		}
	}()
	NewStore(nil, nil, WithSchema("x; DROP TABLE y"))
}

func TestCreateFlow(t *testing.T) {
	s, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectQuery(q("INSERT INTO shared.decision_policies")).
		WithArgs("pi", "desc", "u1").WillReturnRows(policyRow(7, "pi", 1, 1, StatusActive))
	mock.ExpectExec(q("INSERT INTO shared.decision_policy_versions")).
		WithArgs(int64(7), "text", "note", "u1").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	p, err := s.Create(context.Background(), CreateInput{Name: "pi", Description: "desc", Content: "text", Note: "note", Actor: "u1"})
	if err != nil || p.ID != 7 || p.CurrentVersion != 1 {
		t.Fatalf("Create = %+v, %v", p, err)
	}
}

func TestCreateNameTakenRollsBack(t *testing.T) {
	s, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectQuery(q("INSERT INTO shared.decision_policies")).WillReturnError(&pq.Error{Code: "23505"})
	mock.ExpectRollback()
	if _, err := s.Create(context.Background(), CreateInput{Name: "pi", Content: "x"}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateVersionFlow(t *testing.T) {
	for _, tc := range []struct {
		name        string
		makeCurrent *bool
		wantCurrent int
	}{{"default makes current", nil, 4}, {"staged", new(bool), 2}} {
		t.Run(tc.name, func(t *testing.T) {
			s, mock := newMockStore(t)
			mock.ExpectBegin()
			mock.ExpectQuery(q("FOR UPDATE")).WithArgs(int64(7)).WillReturnRows(policyRow(7, "pi", 2, 3, StatusActive))
			mock.ExpectQuery(q("INSERT INTO shared.decision_policy_versions")).
				WithArgs(int64(7), 4, "v4", "", "u", "pi").
				WillReturnRows(sqlmock.NewRows([]string{"id", "policy_id", "name", "version", "content", "note", "created_by", "created_at"}).
					AddRow(40, 7, "pi", 4, "v4", "", "u", time.Now()))
			mock.ExpectExec(q("UPDATE shared.decision_policies SET latest_version")).
				WithArgs(int64(7), 4, tc.wantCurrent, "u").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			v, err := s.CreateVersion(context.Background(), 7, VersionInput{Content: "v4", Actor: "u", MakeCurrent: tc.makeCurrent})
			if err != nil || v.Version != 4 {
				t.Fatalf("CreateVersion = %+v, %v", v, err)
			}
		})
	}
}

func TestMutationsOnDeletedPolicy(t *testing.T) {
	ctx := context.Background()
	desc := "d"
	calls := map[string]func(*Store) error{
		"update":      func(s *Store) error { _, err := s.Update(ctx, 7, UpdateInput{Description: &desc}); return err },
		"version":     func(s *Store) error { _, err := s.CreateVersion(ctx, 7, VersionInput{Content: "x"}); return err },
		"set_current": func(s *Store) error { _, err := s.SetCurrentVersion(ctx, 7, 1, ""); return err },
		"delete":      func(s *Store) error { return s.Delete(ctx, 7, "") },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			s, mock := newMockStore(t)
			mock.ExpectBegin()
			mock.ExpectQuery(q("FOR UPDATE")).WillReturnRows(policyRow(7, "pi", 1, 1, StatusDeleted))
			mock.ExpectRollback()
			if err := call(s); !errors.Is(err, ErrDeleted) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestSetCurrentUnknownVersion(t *testing.T) {
	s, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectQuery(q("FOR UPDATE")).WillReturnRows(policyRow(7, "pi", 1, 3, StatusActive))
	mock.ExpectQuery(q("SELECT 1 FROM shared.decision_policy_versions")).WithArgs(int64(7), 9).
		WillReturnRows(sqlmock.NewRows([]string{"one"}))
	mock.ExpectRollback()
	if _, err := s.SetCurrentVersion(context.Background(), 7, 9, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestQueryErrorMapping(t *testing.T) {
	s, mock := newMockStore(t)
	ctx := context.Background()
	mock.ExpectQuery(q("WHERE id = $1")).WillReturnRows(sqlmock.NewRows(policyCols))
	if _, err := s.Get(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("no rows err = %v", err)
	}
	mock.ExpectQuery(q("LOWER(name) = LOWER($1)")).WillReturnError(&pq.Error{Code: "42P01", Message: `relation "shared.decision_policies" does not exist`})
	if _, err := s.GetByName(ctx, "x"); !errors.Is(err, ErrTablesMissing) {
		t.Errorf("missing table err = %v", err)
	}
}

func TestListEscapesWildcards(t *testing.T) {
	s, mock := newMockStore(t)
	mock.ExpectQuery(q("LIKE LOWER($2)")).
		WithArgs(StatusActive, `a\_b\%%`, defaultListLimit, 0).
		WillReturnRows(policyRow(1, "a_b%x", 1, 1, StatusActive))
	list, err := s.List(context.Background(), ListFilter{NamePrefix: "a_b%"})
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %+v, %v", list, err)
	}
	mock.ExpectQuery(q("LIKE")).WithArgs(StatusActive, "%", maxListLimit, 5).WillReturnRows(sqlmock.NewRows(policyCols))
	if list, err := s.List(context.Background(), ListFilter{Limit: 5000, Offset: 5}); err != nil || list == nil {
		t.Fatalf("capped List = %+v, %v", list, err)
	}
}
