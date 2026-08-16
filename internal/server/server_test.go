package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type recordingHTTPMetrics struct {
	route      string
	statusCode int
	duration   time.Duration
}

func (r *recordingHTTPMetrics) HTTPRequest(_ context.Context, route string, statusCode int, d time.Duration) {
	r.route = route
	r.statusCode = statusCode
	r.duration = d
}

func TestNewSetsTimeoutsAndBodyCap(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, r.Body)
	})
	srv := New(Config{Addr: "127.0.0.1:0", Handler: mux, MaxBodyBytes: 4})
	if srv.ReadTimeout == 0 || srv.ReadHeaderTimeout == 0 || srv.WriteTimeout == 0 || srv.IdleTimeout == 0 {
		t.Fatalf("expected all server timeouts to be set: %+v", srv)
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		<-errCh
	})

	resp, err := http.Post("http://"+ln.Addr().String()+"/echo", "text/plain", strings.NewReader("too-large"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
}

func TestNewWrapsHandlerWithOTelHTTPRecorder(t *testing.T) {
	recorder := &recordingHTTPMetrics{}
	srv := New(Config{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusTeapot)
		}),
		Recorder: recorder,
		RouteResolver: func(r *http.Request) string {
			return "/api/pages/{bundle}/{slug}"
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/pages/nanite/runtime", nil)
	resp := httptest.NewRecorder()
	srv.Handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418", resp.Code)
	}
	if recorder.route != "/api/pages/{bundle}/{slug}" || recorder.statusCode != http.StatusTeapot || recorder.duration <= 0 {
		t.Fatalf("recorder = %+v", recorder)
	}
}
