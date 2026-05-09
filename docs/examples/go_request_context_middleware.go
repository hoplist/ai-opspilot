package logctx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	eventIDKey   contextKey = "event_id"
)

func Middleware(service string, bizLine string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = newID()
		}

		eventID := r.Header.Get("X-Event-Id")

		w.Header().Set("X-Request-Id", requestID)
		if eventID != "" {
			w.Header().Set("X-Event-Id", eventID)
		}

		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		ctx = context.WithValue(ctx, eventIDKey, eventID)

		logger := LoggerFromContext(ctx, service, bizLine)
		logger.Info("request_started", "method", r.Method, "path", r.URL.Path)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoggerFromContext(ctx context.Context, service string, bizLine string) *slog.Logger {
	fields := []any{
		"service", service,
		"biz_line", bizLine,
		"request_id", RequestIDFromContext(ctx),
		"event_id", EventIDFromContext(ctx),
	}

	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasTraceID() {
		fields = append(fields, "trace_id", spanCtx.TraceID().String())
	}
	if spanCtx.HasSpanID() {
		fields = append(fields, "span_id", spanCtx.SpanID().String())
	}

	return slog.Default().With(fields...)
}

func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func EventIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(eventIDKey).(string)
	return value
}

func newID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "req-fallback"
	}
	return hex.EncodeToString(buf[:])
}

// Usage:
//
// mux := http.NewServeMux()
// mux.Handle("/api", Middleware("devops-server", "observability", apiHandler))
//
// Inside handlers:
// logger := LoggerFromContext(r.Context(), "devops-server", "observability")
// logger.Error("get_branches_failed", "event_name", "bs_external_GetBranchesByIds")
