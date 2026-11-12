// Package server holds the HTTP middleware Loom adds around the Substrate host: a request body
// cap and the OpenTelemetry HTTP metrics. Listening, timeouts, panic recovery, shutdown and
// access control belong to httphost and the auth guard.
package server

import (
	"net/http"

	otelprop "github.com/hollis-labs/libs/core/util/otel/propagation"
)

const DefaultMaxBodyBytes int64 = 2 << 20

type Config struct {
	MaxBodyBytes  int64
	Recorder      otelprop.HTTPMetricRecorder
	RouteResolver func(*http.Request) string
}

// Wrap caps request bodies and, when a recorder is set, records OTel HTTP metrics.
func Wrap(handler http.Handler, cfg Config) http.Handler {
	maxBody := cfg.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = DefaultMaxBodyBytes
	}
	handler = limitBodyMiddleware(handler, maxBody)
	opts := make([]otelprop.MiddlewareOption, 0, 2)
	if cfg.Recorder != nil {
		opts = append(opts, otelprop.WithMetricRecorder(cfg.Recorder))
	}
	if cfg.RouteResolver != nil {
		opts = append(opts, otelprop.WithRouteResolver(cfg.RouteResolver))
	}
	return otelprop.HTTPMiddleware(handler, opts...)
}

func limitBodyMiddleware(next http.Handler, maxBody int64) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxBody {
			http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		next.ServeHTTP(w, r)
	})
}
