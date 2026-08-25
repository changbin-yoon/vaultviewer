package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestStaticCatchAllNeverServesAPIPaths guards against a real regression:
// once a static frontend dir is configured, Go's http.ServeMux prefers a
// method-specific pattern ("GET /api/tree") when the method matches, but
// falls through to a registered "/" catch-all — the static file server —
// on any other method or an unknown /api/ path, rather than returning the
// 405 one might expect. Without registerStatic's explicit "/api/" and
// "/ws/" guards, a wrong-method or typo'd request under those prefixes
// would be resolved as a static asset lookup (404, or worse, would
// actually serve a file if one happened to exist on disk at that path).
func TestStaticCatchAllNeverServesAPIPaths(t *testing.T) {
	staticDir := t.TempDir()
	// Files at the exact literal paths a fallthrough would try to serve —
	// proves the guard, not just an absence of a matching file on disk, is
	// what stops these from resolving as static assets. Without
	// registerStatic's "/api/"/"/ws/" guards, both would come back 200
	// with this placeholder content instead of 404.
	for _, p := range []string{filepath.Join("api", "tree"), filepath.Join("api", "unknown-route"), filepath.Join("ws", "audit")} {
		full := filepath.Join(staticDir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("should never be served"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tree", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tree"))
	})
	registerStatic(mux, Deps{StaticDir: staticDir})

	cases := []struct {
		method, path string
		wantStatus   int
	}{
		{http.MethodGet, "/api/tree", http.StatusOK},
		{http.MethodPost, "/api/tree", http.StatusNotFound},
		{http.MethodDelete, "/api/unknown-route", http.StatusNotFound},
		{http.MethodGet, "/ws/audit", http.StatusNotFound},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != tc.wantStatus {
			t.Errorf("%s %s: got status %d, want %d", tc.method, tc.path, w.Code, tc.wantStatus)
		}
	}
}
