package server

import (
	"context"
	"io"
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

func TestWrapCapsBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(w, r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		}
	})
	srv := httptest.NewServer(Wrap(mux, Config{MaxBodyBytes: 4}))
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/echo", "text/plain", strings.NewReader("too-large"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
}

func TestWrapsHandlerWithOTelHTTPRecorder(t *testing.T) {
	recorder := &recordingHTTPMetrics{}
	handler := Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}), Config{
		Recorder: recorder,
		RouteResolver: func(r *http.Request) string {
			return "/api/pages/{bundle}/{slug}"
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/pages/nanite/runtime", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418", resp.Code)
	}
	if recorder.route != "/api/pages/{bundle}/{slug}" || recorder.statusCode != http.StatusTeapot || recorder.duration <= 0 {
		t.Fatalf("recorder = %+v", recorder)
	}
}
