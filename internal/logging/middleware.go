package logging

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Flush() {
	if flusher, ok := sw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

type timingCtxKey struct{}

type ServerMetric struct {
	Name       string
	DurationMs float64
}

type ServerTimingTracker struct {
	mu      sync.Mutex
	metrics []ServerMetric
}

func (t *ServerTimingTracker) Record(name string, durationMs float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.metrics = append(t.metrics, ServerMetric{Name: name, DurationMs: roundMetric(durationMs)})
}

func (t *ServerTimingTracker) Metrics() []ServerMetric {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]ServerMetric, len(t.metrics))
	copy(out, t.metrics)
	return out
}

func TrackerFromContext(ctx context.Context) *ServerTimingTracker {
	if t, ok := ctx.Value(timingCtxKey{}).(*ServerTimingTracker); ok {
		return t
	}
	return nil
}

type TimingConfig struct {
	Route  string
	SlowMs float64
	Ctx    []any
}

func CorrelationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = GenerateID()
		}
		ctx := WithCorrelationID(r.Context(), reqID)
		w.Header().Set("X-Request-ID", reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start).String(),
			"correlation_id", CorrelationID(r.Context()),
		)
	})
}

func TimingMiddleware(next http.Handler) http.Handler {
	return TimingMiddlewareWithConfig(next, TimingConfig{
		SlowMs: 500,
	})
}

func TimingMiddlewareWithConfig(next http.Handler, cfg TimingConfig) http.Handler {
	slowMs := cfg.SlowMs
	if slowMs == 0 {
		slowMs = 500
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracker := &ServerTimingTracker{}
		ctx := context.WithValue(r.Context(), timingCtxKey{}, tracker)
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r.WithContext(ctx))

		totalMs := float64(time.Since(start).Nanoseconds()) / float64(time.Millisecond)
		metrics := tracker.Metrics()
		headerValue := formatServerTimingHeader(metrics, totalMs)
		w.Header().Set("Server-Timing", headerValue)

		if totalMs >= slowMs {
			route := cfg.Route
			if route == "" {
				route = r.Method + " " + r.URL.Path
			}
			args := []any{
				"route", route,
				"duration_ms", roundMetric(totalMs),
			}
			for i := 0; i+1 < len(cfg.Ctx); i += 2 {
				args = append(args, cfg.Ctx[i], cfg.Ctx[i+1])
			}
			args = append(args, "correlation_id", CorrelationID(r.Context()))
			slog.Warn("slow request", args...)
		}
	})
}

func Measure(ctx context.Context, name string, fn func() error) error {
	tracker := TrackerFromContext(ctx)
	if tracker == nil {
		return fn()
	}
	start := time.Now()
	err := fn()
	durationMs := float64(time.Since(start).Nanoseconds()) / float64(time.Millisecond)
	tracker.Record(name, durationMs)
	return err
}

func formatServerTimingHeader(metrics []ServerMetric, totalMs float64) string {
	parts := make([]string, 0, len(metrics)+1)
	for _, m := range metrics {
		parts = append(parts, fmt.Sprintf("%s;dur=%.1f", m.Name, m.DurationMs))
	}
	parts = append(parts, fmt.Sprintf("total;dur=%.1f", roundMetric(totalMs)))
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}

func roundMetric(v float64) float64 {
	return math.Round(v*10) / 10
}
