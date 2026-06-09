package observability

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const RequestIDHeader = "X-Request-ID"

var (
	httpServerDuration     metric.Float64Histogram
	httpServerDurationOnce sync.Once
)

func RequestMiddleware(cfg Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			reqID := c.Request().Header.Get(RequestIDHeader)
			if reqID == "" {
				reqID = ApiUtils.GenerateRequestID("e")
			}

			ctx := c.Request().Context()
			ctx = context.WithValue(ctx, ApiTypes.RequestIDKey, reqID)
			ctx = context.WithValue(ctx, ApiTypes.CallFlowKey, cfg.ServiceName)

			if cfg.Enabled {
				ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(c.Request().Header))
				spanName := c.Request().Method + " " + c.Path()
				var span trace.Span
				ctx, span = otel.Tracer(cfg.ServiceName).Start(ctx, spanName,
					trace.WithSpanKind(trace.SpanKindServer),
					trace.WithAttributes(requestAttributes(c, cfg, reqID)...),
				)
				defer span.End()
			}

			c.SetRequest(c.Request().WithContext(ctx))
			c.Set(string(ApiTypes.RequestIDKey), reqID)
			c.Response().Header().Set(RequestIDHeader, reqID)

			start := time.Now()
			err := next(c)
			status := c.Response().Status
			if status == 0 {
				status = http.StatusOK
			}
			if cfg.Enabled {
				attrs := append(requestAttributes(c, cfg, reqID), attribute.Int("http.response.status_code", status))
				if err != nil {
					trace.SpanFromContext(c.Request().Context()).RecordError(err)
					trace.SpanFromContext(c.Request().Context()).SetStatus(codes.Error, err.Error())
				}
				trace.SpanFromContext(c.Request().Context()).SetAttributes(attrs...)
				recordHTTPDuration(c.Request().Context(), time.Since(start), attrs)
			}
			if err != nil && c.Response().Committed {
				c.Response().Header().Set(http.CanonicalHeaderKey(RequestIDHeader), reqID)
			}
			return err
		}
	}
}

func requestAttributes(c echo.Context, cfg Config, reqID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("service.name", cfg.ServiceName),
		attribute.String("deployment.environment.name", cfg.Environment),
		attribute.String("http.request.method", c.Request().Method),
		attribute.String("url.path", c.Request().URL.Path),
		attribute.String("http.route", c.Path()),
		attribute.String("request.id", reqID),
	}
}

func recordHTTPDuration(ctx context.Context, duration time.Duration, attrs []attribute.KeyValue) {
	httpServerDurationOnce.Do(func() {
		histogram, err := otel.Meter("github.com/chendingplano/shared/go/api/observability").
			Float64Histogram("http.server.duration",
				metric.WithDescription("HTTP server request duration"),
				metric.WithUnit("s"))
		if err == nil {
			httpServerDuration = histogram
		}
	})
	if httpServerDuration != nil {
		httpServerDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
	}
}
