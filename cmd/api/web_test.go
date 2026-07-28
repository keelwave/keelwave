package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWebRouting verifies the embedded dashboard is served for non-API paths
// while /v1/* routes are untouched. mount() registers routes without touching
// the DB, so a bare application is enough here.
func TestWebRouting(t *testing.T) {
	app := &application{config: config{env: "test"}}
	mux := app.mount()

	t.Run("root serves dashboard html", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		if rr.Code != http.StatusOK {
			t.Fatalf("GET / = %d, want 200", rr.Code)
		}
		if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("GET / content-type = %q, want text/html", ct)
		}
		if !strings.Contains(rr.Body.String(), "<html") {
			t.Fatalf("GET / body is not html: %s", rr.Body.String())
		}
	})

	t.Run("client route falls back to index", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/login", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /login = %d, want 200 (SPA fallback)", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "<html") {
			t.Fatalf("GET /login did not serve the SPA shell")
		}
	})

	t.Run("health still returns json", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /v1/health = %d, want 200", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, `"status"`) || !strings.Contains(body, `"ok"`) {
			t.Fatalf("GET /v1/health body = %s, want health json", body)
		}
		if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("GET /v1/health content-type = %q, want application/json", ct)
		}
	})

	t.Run("missing asset 404s not shell", func(t *testing.T) {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/assets/does-not-exist.js", nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("GET /assets/does-not-exist.js = %d, want 404", rr.Code)
		}
	})
}
