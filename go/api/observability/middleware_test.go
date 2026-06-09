package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestRequestMiddlewareAddsRequestID(t *testing.T) {
	e := echo.New()
	e.Use(RequestMiddleware(Config{Enabled: false, ServiceName: "chenweb", Environment: "test"}))
	e.GET("/api/v1/health", func(c echo.Context) error {
		reqID, _ := c.Request().Context().Value(ApiTypes.RequestIDKey).(string)
		if reqID == "" {
			t.Fatalf("request context is missing request ID")
		}
		return c.JSON(http.StatusOK, map[string]string{"req_id": reqID})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Request-ID"); got == "" {
		t.Fatalf("response is missing X-Request-ID")
	}
}

func TestRequestMiddlewarePreservesIncomingRequestID(t *testing.T) {
	e := echo.New()
	e.Use(RequestMiddleware(Config{Enabled: false, ServiceName: "chenweb", Environment: "test"}))
	e.GET("/api/v1/health", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("X-Request-ID", "req-from-client")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "req-from-client" {
		t.Fatalf("X-Request-ID = %q, want req-from-client", got)
	}
}
