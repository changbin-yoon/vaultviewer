package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"github.com/accesslens/accesslens/internal/audit"
	"github.com/accesslens/accesslens/internal/auth"
	"github.com/accesslens/accesslens/internal/backup"
	"github.com/accesslens/accesslens/internal/model"
	"github.com/accesslens/accesslens/internal/ontology"
	"github.com/accesslens/accesslens/internal/storage"
	"github.com/accesslens/accesslens/internal/storage/git"
	"github.com/accesslens/accesslens/internal/storage/k8s"
	"github.com/accesslens/accesslens/internal/storage/local"
	"github.com/accesslens/accesslens/internal/teams"
	"github.com/accesslens/accesslens/internal/trino"
)

func main() {
	mode := strings.ToLower(envOr("ACCESSLENS_MODE", "local"))

	recorder, err := buildAuditRecorder(mode)
	if err != nil {
		log.Fatalf("init audit recorder: %v", err)
	}

	engine, configInfo, err := buildEngine(mode, recorder)
	if err != nil {
		log.Fatalf("init storage engine: %v", err)
	}
	// Separate from the storage mode above (which can be "local" even when
	// the process itself runs inside Kubernetes, e.g. a PVC-backed
	// Deployment): this only labels *where the process is running* for the
	// UI badge next to the app name. Plain binary/Docker runs default to
	// "LOCAL"; the Helm chart sets it to "CLUSTER" (or a custom cluster
	// name) via ACCESSLENS_DEPLOYMENT_LABEL.
	configInfo["deployment"] = envOr("ACCESSLENS_DEPLOYMENT_LABEL", "LOCAL")

	// S3/MinIO backup only applies to local mode — that's the only mode
	// with a directory on disk to mirror; cluster mode's data already lives
	// in Kubernetes Secrets (backed by etcd), out of scope here. Disabled
	// unless ACCESSLENS_S3_ENDPOINT (etc.) is set, so this is a no-op for
	// every deployment that doesn't opt in.
	if mode == "local" {
		backupCfg, err := backup.LoadConfigFromEnv()
		if err != nil {
			log.Fatalf("load S3 backup config: %v", err)
		}
		if backupCfg.Enabled() {
			syncer, err := backup.NewSyncer(backupCfg, configInfo["root"])
			if err != nil {
				log.Fatalf("init S3 backup syncer: %v", err)
			}
			go syncer.Run(context.Background())
			log.Printf("S3 backup enabled: syncing %s to s3://%s every %s", configInfo["root"], backupCfg.Bucket, backupCfg.Interval)
		}
	}

	teamsStore, err := buildTeamsStore(mode)
	if err != nil {
		log.Fatalf("init group-team store: %v", err)
	}

	ldapCfg, err := auth.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load LDAP config: %v", err)
	}
	authenticator := auth.NewLDAPAuthenticator(ldapCfg, teamsStore)
	sm := auth.NewSessionManager(sessionSecret(), sessionTTL())
	loginThrottle := auth.NewLoginThrottle()

	// Independent of storage mode (unlike S3 backup above) — a Dashboard
	// status card, not tied to how/where vault data is stored. Disabled
	// unless ACCESSLENS_TRINO_ENDPOINT (etc.) is set.
	trinoCfg := trino.LoadConfigFromEnv()
	trinoClient := trino.NewClient(trinoCfg)

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var creds struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		throttleKey := strings.ToLower(strings.TrimSpace(creds.Username))
		if allowed, wait := loginThrottle.Allow(throttleKey); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			http.Error(w, "too many failed login attempts, try again shortly", http.StatusTooManyRequests)
			return
		}

		user, err := authenticator.Authenticate(creds.Username, creds.Password)
		if err != nil {
			status := http.StatusUnauthorized
			if !errors.Is(err, auth.ErrInvalidCredentials) && !errors.Is(err, auth.ErrNoRole) {
				// An LDAP/network failure isn't the caller's fault — don't
				// spend part of their backoff budget on it.
				log.Printf("ldap authenticate error: %v", err)
				status = http.StatusBadGateway
			} else {
				loginThrottle.RecordFailure(throttleKey)
			}
			http.Error(w, "authentication failed", status)
			return
		}
		loginThrottle.RecordSuccess(throttleKey)

		token, err := sm.Issue(*user)
		if err != nil {
			http.Error(w, "failed to issue session", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"token":      token,
			"username":   user.Username,
			"role":       string(user.Role),
			"department": user.Department,
		})
	})

	mux.HandleFunc("/api/tree", auth.RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		items, err := engine.List(r.URL.Query().Get("path"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}))

	mux.HandleFunc("/api/search", auth.RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		results, err := engine.Search(r.URL.Query().Get("q"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}))

	// The vault-wide note graph (nodes + typed/untyped edges), for
	// consumers other than the frontend's own graph view — an AI agent,
	// a script, an MCP server — that want the ontology without fetching
	// every note and re-parsing frontmatter/wikilinks themselves. Read
	// access only, same tier as /api/tree and /api/search.
	mux.HandleFunc("/api/graph", auth.RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		graph, err := ontology.Build(engine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(graph)
	}))

	mux.HandleFunc("/api/namespace", auth.RequireWrite(sm, func(w http.ResponseWriter, r *http.Request, user model.User) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := engine.CreateNamespace(r.URL.Query().Get("path"), user.Username); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("/api/rename", auth.RequireWrite(sm, func(w http.ResponseWriter, r *http.Request, user model.User) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
		if err := engine.Rename(from, to, user.Username, r.URL.Query().Get("reason")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("/api/file", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			auth.RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, _ model.User) {
				file, err := engine.Read(r.URL.Query().Get("path"))
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(file)
			})(w, r)
		case http.MethodPut, http.MethodPost:
			auth.RequireWrite(sm, func(w http.ResponseWriter, r *http.Request, user model.User) {
				content, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "failed to read request body", http.StatusBadRequest)
					return
				}
				if err := engine.Save(r.URL.Query().Get("path"), content, user.Username, r.URL.Query().Get("reason")); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})(w, r)
		case http.MethodDelete:
			auth.RequireDelete(sm, func(w http.ResponseWriter, r *http.Request, user model.User) {
				if err := engine.Delete(r.URL.Query().Get("path"), user.Username); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/history", auth.RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		history, err := engine.GetHistory(r.URL.Query().Get("path"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(history)
	}))

	mux.HandleFunc("/api/audit", auth.RequireAdmin(sm, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		entries, err := recorder.All()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	}))

	mux.HandleFunc("/ws/audit", auditWebSocketHandler(sm, recorder))

	mux.HandleFunc("/api/me", auth.RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, user model.User) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"username":   user.Username,
			"role":       string(user.Role),
			"department": user.Department,
		})
	}))

	// Dashboard status card for Trino — a connectivity check plus the
	// operator-configured role/catalog labels (not a live GRANT lookup, see
	// internal/trino). Always returns 200; "enabled: false" is how the
	// frontend tells "not configured" apart from "configured but down".
	mux.HandleFunc("/api/trino", auth.RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, user model.User) {
		w.Header().Set("Content-Type", "application/json")
		if !trinoCfg.Enabled() {
			json.NewEncoder(w).Encode(map[string]bool{"enabled": false})
			return
		}
		connected, err := trinoClient.CheckConnection(r.Context())
		if err != nil {
			connected = false
		}
		json.NewEncoder(w).Encode(map[string]any{
			"enabled":   true,
			"connected": connected,
			"role":      trinoCfg.RoleMap[user.Role],
			"catalogs":  trinoCfg.Catalogs,
		})
	}))

	mux.HandleFunc("/api/config", auth.RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(configInfo)
	}))

	mux.HandleFunc("/api/group-teams", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			auth.RequireAdmin(sm, func(w http.ResponseWriter, r *http.Request, _ model.User) {
				m, err := teamsStore.Get()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(m)
			})(w, r)
		case http.MethodPut:
			auth.RequireAdmin(sm, func(w http.ResponseWriter, r *http.Request, _ model.User) {
				var m map[string]string
				if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
					http.Error(w, "invalid request body", http.StatusBadRequest)
					return
				}
				if err := teamsStore.Set(m); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	staticDir := envOr("ACCESSLENS_STATIC_DIR", "web/dist")
	if _, err := os.Stat(staticDir); err == nil {
		mux.Handle("/", http.FileServer(http.Dir(staticDir)))
		log.Printf("serving frontend from %s", staticDir)
	} else {
		log.Printf("no frontend build found at %s; API-only mode", staticDir)
	}

	// Named HTTP_PORT rather than PORT: Kubernetes auto-injects a
	// "<SERVICE_NAME>_PORT" env var (e.g. ACCESSLENS_PORT) for every
	// Service visible to the pod, which would otherwise collide with and
	// silently override this one.
	addr := ":" + envOr("ACCESSLENS_HTTP_PORT", "8080")
	srv := &http.Server{Addr: addr, Handler: withCORS(mux)}

	go func() {
		log.Printf("accesslens server listening on %s (mode=%s)", addr, mode)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// On SIGTERM (what Kubernetes sends before killing a pod) or SIGINT,
	// stop accepting new connections and let in-flight requests finish
	// instead of dropping them mid-response. WebSocket connections are
	// hijacked out of net/http's bookkeeping once upgraded, so Shutdown
	// can't wait on those specifically — the frontend already reconnects
	// automatically, so a dropped stream just triggers that path.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Print("shutting down: draining in-flight requests")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown timed out: %v", err)
	}
}

// buildAuditRecorder chooses where the audit trail lives. Local mode
// already has a persistent volume (the same one holding the vault
// content) to put a durable log on, so history survives a restart there.
// Cluster mode has no equivalent persistent location without adding a new
// dependency, so it keeps the audit trail in memory only, as before —
// history there resets on every restart.
func buildAuditRecorder(mode string) (*audit.MemoryRecorder, error) {
	if mode != "local" {
		return audit.NewMemoryRecorder(), nil
	}
	root := envOr("VAULT_LOCAL_ROOT", "/data")
	path := filepath.Join(root, ".vaultviewer-audit.jsonl")
	recorder, err := audit.NewMemoryRecorderWithFile(path)
	if err != nil {
		return nil, err
	}
	log.Printf("audit log persisted to %s", path)
	return recorder, nil
}

// buildTeamsStore chooses where the admin-managed group-to-team-name
// mapping lives, mirroring buildAuditRecorder's local-vs-cluster choice:
// local mode already has a persistent volume to put a durable file on;
// cluster mode has no equivalent persistent location without adding a new
// dependency, so edits there don't survive a restart.
func buildTeamsStore(mode string) (teams.Store, error) {
	if mode != "local" {
		return teams.NewMemoryStore(), nil
	}
	root := envOr("VAULT_LOCAL_ROOT", "/data")
	path := filepath.Join(root, ".vaultviewer-group-teams.json")
	store, err := teams.NewFileStore(path)
	if err != nil {
		return nil, err
	}
	log.Printf("group-team map persisted to %s", path)
	return store, nil
}

// buildEngine constructs the storage backend selected by ACCESSLENS_MODE:
//
//	local   (default) — a host/PVC directory mounted at VAULT_LOCAL_ROOT.
//	cluster            — Kubernetes Secrets in ACCESSLENS_K8S_NAMESPACE,
//	                     using the pod's in-cluster service account.
//
// It also returns the payload served at /api/config, so the frontend's
// mode/backend badges always reflect how the server actually started.
func buildEngine(mode string, recorder storage.AuditRecorder) (storage.VaultStorageEngine, map[string]string, error) {
	switch mode {
	case "local":
		root := envOr("VAULT_LOCAL_ROOT", "/data")
		localEngine, err := local.New(root, recorder)
		if err != nil {
			return nil, nil, fmt.Errorf("init local storage engine: %w", err)
		}
		log.Printf("local storage engine rooted at %s", root)
		if os.Getenv("ACCESSLENS_GIT_ENABLED") != "true" {
			return localEngine, map[string]string{"mode": "LOCAL", "backend": "filesystem", "root": root}, nil
		}
		gitEngine, err := git.New(localEngine, root)
		if err != nil {
			return nil, nil, fmt.Errorf("init git storage engine: %w", err)
		}
		log.Printf("git-backed versioning enabled at %s", root)
		return gitEngine, map[string]string{"mode": "LOCAL", "backend": "filesystem+git", "root": root}, nil
	case "cluster":
		namespace := os.Getenv("ACCESSLENS_K8S_NAMESPACE")
		if namespace == "" {
			return nil, nil, errors.New("ACCESSLENS_K8S_NAMESPACE is required when ACCESSLENS_MODE=cluster")
		}
		engine, err := k8s.NewInCluster(namespace, recorder)
		if err != nil {
			return nil, nil, fmt.Errorf("init cluster storage engine: %w", err)
		}
		log.Printf("cluster storage engine targeting namespace %s", namespace)
		return engine, map[string]string{"mode": "CLUSTER", "backend": "kubernetes-secrets", "namespace": namespace}, nil
	default:
		return nil, nil, fmt.Errorf("unknown ACCESSLENS_MODE %q (expected \"local\" or \"cluster\")", mode)
	}
}

// withCORS allows a separately served frontend (e.g. the Vite dev server)
// to call this API from another origin. Origins are restricted to an
// explicit allowlist rather than reflecting any Origin header. Not needed
// when the frontend is served from this same process (the default), since
// same-origin requests bypass CORS entirely.
func withCORS(next http.Handler) http.Handler {
	allowedOrigin := envOr("ACCESSLENS_CORS_ORIGIN", "http://localhost:5173")
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

const (
	wsPongWait   = 60 * time.Second
	wsPingPeriod = (wsPongWait * 9) / 10
	wsWriteWait  = 10 * time.Second
)

var wsUpgrader = websocket.Upgrader{CheckOrigin: wsCheckOrigin}

// wsCheckOrigin allows same-origin WebSocket handshakes (the default
// deployment, frontend served from this same process) and the explicit
// ACCESSLENS_CORS_ORIGIN allowlist (a separately served dev frontend),
// mirroring withCORS's REST policy. gorilla/websocket otherwise rejects
// every cross-origin handshake by default.
func wsCheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if u, err := url.Parse(origin); err == nil && u.Host == r.Host {
		return true
	}
	return origin == envOr("ACCESSLENS_CORS_ORIGIN", "http://localhost:5173")
}

// auditWebSocketHandler streams every new audit log entry to the client as
// audit.MemoryRecorder.Record is called, so the audit log view updates
// live instead of polling. Auth uses a ?token= query parameter rather than
// the Authorization header every other endpoint requires, since browsers
// cannot set custom headers on a WebSocket handshake request.
func auditWebSocketHandler(sm *auth.SessionManager, recorder *audit.MemoryRecorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := sm.Verify(r.URL.Query().Get("token"))
		if err != nil {
			http.Error(w, "invalid or expired session", http.StatusUnauthorized)
			return
		}
		if !user.Role.IsAdmin() {
			http.Error(w, "role does not permit admin access", http.StatusForbidden)
			return
		}

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("audit ws: upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		ch, unsubscribe := recorder.Subscribe()
		defer unsubscribe()

		conn.SetReadDeadline(time.Now().Add(wsPongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(wsPongWait))
			return nil
		})

		// The client never sends messages; this goroutine's only job is to
		// notice the connection closing (tab closed, network drop) via a
		// read error, so the main loop below can stop promptly.
		closed := make(chan struct{})
		go func() {
			defer close(closed)
			for {
				if _, _, err := conn.NextReader(); err != nil {
					return
				}
			}
		}()

		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-closed:
				return
			case entry, ok := <-ch:
				if !ok {
					return
				}
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := conn.WriteJSON(entry); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}
}

// sessionSecret loads the HMAC key used to sign session tokens from
// ACCESSLENS_SESSION_SECRET. If unset, a random key is generated for the
// life of this process — sessions won't survive a restart, but the secret
// is never hardcoded, per project policy.
func sessionSecret() []byte {
	if v := os.Getenv("ACCESSLENS_SESSION_SECRET"); v != "" {
		return []byte(v)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatalf("generate session secret: %v", err)
	}
	log.Printf("warning: ACCESSLENS_SESSION_SECRET not set; generated an ephemeral key, sessions will not survive a restart")
	return secret
}

func sessionTTL() time.Duration {
	hours := 8
	if v := os.Getenv("ACCESSLENS_SESSION_TTL_HOURS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			log.Fatalf("invalid ACCESSLENS_SESSION_TTL_HOURS %q: must be a positive integer", v)
		}
		hours = parsed
	}
	return time.Duration(hours) * time.Hour
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
