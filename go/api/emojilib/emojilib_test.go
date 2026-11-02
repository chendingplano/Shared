package emojilib

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/emojibuild"
	"testing"
)

func TestTextSearchRequiresBM25(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	_, err = Search(context.Background(), db, "庆祝", "", 30, 0)
	if !errors.Is(err, ErrBM25Unavailable) {
		t.Fatalf("expected BM25 unavailable, got %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBounds(t *testing.T) {
	if l, o := bounds(201, -1); l != 200 || o != 0 {
		t.Fatalf("got %d %d", l, o)
	}
	if l, _ := bounds(0, 0); l != 30 {
		t.Fatalf("default limit: %d", l)
	}
}

func TestReadinessReportsMissingIndex(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	if err := Ready(context.Background(), db); !errors.Is(err, ErrBM25Unavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestImportRollsBackOnDatabaseFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO shared.emoji_library").WillReturnError(errors.New("write failed"))
	mock.ExpectRollback()
	e := emojibuild.Emoji{ID: "1f389", Unicode: "🎉", NameEN: "party popper", NameZH: "拉炮", CategoryID: "activities", CategoryEN: "activities", CategoryZH: "活动", KeywordsEN: "party", KeywordsZH: "庆祝", DescriptionEN: "party popper", DescriptionZH: "拉炮", SVG: `<svg/>`}
	if err := Import(context.Background(), db, []emojibuild.Emoji{e}, Source{"17.0.3", "17.0"}); err == nil {
		t.Fatal("expected import error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMixedSymbolQueryContinuesToBM25(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT unicode_id").WillReturnRows(sqlmock.NewRows([]string{"unicode_id", "unicode_value", "name_en", "name_zh", "category_id", "category_en", "category_zh", "keywords_en", "keywords_zh", "description_en", "description_zh", "version", "score"}))
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	_, err = Search(context.Background(), db, "party 🎉", "", 30, 0)
	if !errors.Is(err, ErrBM25Unavailable) {
		t.Fatalf("mixed query skipped BM25: %v", err)
	}
}
