package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	otelprop "github.com/hollis-labs/libs/util/otel/propagation"
)

const DefaultMaxBodyBytes int64 = 2 << 20

type Config struct {
	Addr          string
	Handler       http.Handler
	MaxBodyBytes  int64
	Logger        *log.Logger
	Recorder      otelprop.HTTPMetricRecorder
	RouteResolver func(*http.Request) string
}

func New(cfg Config) *http.Server {
	maxBody := cfg.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = DefaultMaxBodyBytes
	}
	handler := recoverMiddleware(limitBodyMiddleware(cfg.Handler, maxBody), cfg.Logger)
	opts := make([]otelprop.MiddlewareOption, 0, 2)
	if cfg.Recorder != nil {
		opts = append(opts, otelprop.WithMetricRecorder(cfg.Recorder))
	}
	if cfg.RouteResolver != nil {
		opts = append(opts, otelprop.WithRouteResolver(cfg.RouteResolver))
	}
	handler = otelprop.HTTPMiddleware(handler, opts...)
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

func Serve(ctx context.Context, srv *http.Server) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			return err
		}
		return <-errCh
	}
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

func recoverMiddleware(next http.Handler, logger *log.Logger) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				logger.Printf("panic serving %s %s: %v\n%s", r.Method, r.URL.Path, p, debug.Stack())
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
