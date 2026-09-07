// Package api wires AccessLens's REST endpoints to an http.Handler. It owns
// no business logic of its own — every handler is a thin adapter over
// internal/storage, internal/auth, internal/audit, internal/teams, and the
// internal/trino, internal/opa, internal/s3iam status-card clients — so
// cmd/server stays limited to process startup (flag/env parsing, choosing
// which storage/audit backend to construct) and this package stays limited
// to routing and RBAC wiring.
package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/accesslens/accesslens/internal/audit"
	"github.com/accesslens/accesslens/internal/auth"
	"github.com/accesslens/accesslens/internal/opa"
	"github.com/accesslens/accesslens/internal/s3iam"
	"github.com/accesslens/accesslens/internal/storage"
	"github.com/accesslens/accesslens/internal/teams"
	"github.com/accesslens/accesslens/internal/trino"
)

// Deps are the already-constructed dependencies every route needs. cmd/server
// builds these (choosing local vs. cluster storage, local vs. in-memory
// audit, etc.) and hands the finished set to NewRouter — this package never
// reads an environment variable or constructs a backend itself.
type Deps struct {
	Engine        storage.VaultStorageEngine
	Sessions      *auth.SessionManager
	Authenticator auth.Authenticator
	LoginThrottle *auth.LoginThrottle
	Recorder      *audit.MemoryRecorder
	TeamsStore    teams.Store

	Trino       trino.Config
	TrinoClient *trino.Client
	Opa         opa.Config
	OpaClient   *opa.Client
	S3Iam       s3iam.Config
	S3IamClient *s3iam.Client
	// S3IamCatalog is the mirrored MinIO policy set (what each policy
	// permits) and S3IamAttachments the declaration of who holds which
	// policy. Both are needed for the per-bucket capability breakdown, and
	// both are nil when it isn't configured or failed to load — the S3 IAM
	// card then serves its connectivity check and bucket list as before.
	S3IamCatalog     *s3iam.Catalog
	S3IamAttachments *s3iam.Attachments
	// S3IamDrift compares the attachment declaration against what the S3
	// backend actually enforces. Nil when no admin credentials are
	// configured — the card then reports the declared answer without being
	// able to say whether the backend agrees.
	S3IamDrift *s3iam.DriftChecker

	// ConfigInfo is served verbatim at GET /api/config (mode/backend/root
	// or namespace, deployment label) — see cmd/server's buildEngine.
	ConfigInfo map[string]string
	// StaticDir is the built frontend's directory (web/dist). Leave empty
	// to run API-only (no "/" handler) — cmd/server decides this by
	// checking the directory exists, and logs which mode it picked, so
	// that stays a startup-time concern rather than this package's.
	StaticDir string
	// CORSOrigin is the single allowed cross-origin caller (e.g. a
	// separately served Vite dev server). Same-origin requests never need
	// this — see withCORS.
	CORSOrigin string
	// MaxBodyBytes caps request bodies on write endpoints (currently just
	// PUT/POST /api/file — the only endpoint that accepts an
	// operator-sized payload: a note body or an embedded attachment).
	// Same cap regardless of caller (web UI or an MCP server proxying
	// through this same endpoint).
	MaxBodyBytes int64
}

// NewRouter builds the complete HTTP handler: every /api/* and /ws/* route,
// RBAC-wrapped per internal/auth's role helpers, plus the static frontend
// fallback, wrapped in CORS/WebSocket-origin handling.
func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealthz)

	registerAuthRoutes(mux, d)
	registerFileRoutes(mux, d)
	registerAuditRoutes(mux, d)
	registerIntegrationRoutes(mux, d)
	registerStatic(mux, d)

	return withCORS(mux, d.CORSOrigin)
}

// withCORS allows a separately served frontend (e.g. the Vite dev server)
// to call this API from another origin. Origins are restricted to an
// explicit allowlist rather than reflecting any Origin header. Not needed
// when the frontend is served from this same process (the default), since
// same-origin requests bypass CORS entirely.
func withCORS(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// wsCheckOrigin allows same-origin WebSocket handshakes (the default
// deployment, frontend served from this same process) and the explicit
// CORSOrigin allowlist (a separately served dev frontend), mirroring
// withCORS's REST policy. gorilla/websocket otherwise rejects every
// cross-origin handshake by default.
func wsCheckOrigin(allowedOrigin string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		if u, err := url.Parse(origin); err == nil && u.Host == r.Host {
			return true
		}
		return origin == allowedOrigin
	}
}

// cacheHeaders tells browsers which built files are safe to keep.
//
// The bundler gives every asset a content hash in its filename, so those can
// be cached indefinitely — a new build produces a new name. index.html is the
// opposite: its name never changes and its whole job is to point at the
// current hashed bundle. Letting a browser cache it means a deployed change
// can stay invisible for as long as that copy lives, which is exactly the
// failure this rule exists to prevent. http.FileServer sets neither header on
// its own, leaving index.html to the browser's heuristic caching.
func cacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// no-cache means "revalidate before use", not "never store" —
			// the browser still gets a cheap 304 when nothing changed.
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

func registerStatic(mux *http.ServeMux, d Deps) {
	if d.StaticDir == "" {
		return // API-only mode — cmd/server already logged why.
	}
	mux.Handle("/", cacheHeaders(http.FileServer(http.Dir(d.StaticDir))))

	// A request under /api/ or /ws/ that reaches this point matched no
	// "METHOD /api/..." pattern above — most likely the right path with
	// the wrong HTTP method (Go's ServeMux prefers a method+path match
	// when one exists, so this only fires on a mismatch or a genuinely
	// unknown API path). Without this, such a request would otherwise
	// fall through to the static file server above and get served (or
	// 404) as if it were a frontend asset path, which is never correct
	// for something under /api/ or /ws/. "/api/" and "/ws/" are more
	// specific than "/" (longer fixed prefix, per net/http's pattern
	// precedence), so they intercept first.
	notAnAPIRoute := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.Handle("/api/", notAnAPIRoute)
	mux.Handle("/ws/", notAnAPIRoute)
}
