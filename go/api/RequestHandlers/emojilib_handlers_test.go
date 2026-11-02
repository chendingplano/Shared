package RequestHandlers

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmojiSearchReturnsUnavailableWithoutBM25(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := ApiTypes.SharedDBHandle
	ApiTypes.SharedDBHandle = db
	defer func() { ApiTypes.SharedDBHandle = old }()
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/search?q=party", nil), rec)
	if err := HandleSearchEmojiLibrary(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestEmojiNoticeServesEmbeddedAttribution(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/notices/NOTICE.txt", nil), rec)
	ctx.SetParamNames("name")
	ctx.SetParamValues("NOTICE.txt")
	if err := HandleEmojiNotice(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Twemoji") {
		t.Fatalf("notice missing: %d %s", rec.Code, rec.Body.String())
	}
}
