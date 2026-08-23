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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vaultviewer/vaultviewer/internal/audit"
	"github.com/vaultviewer/vaultviewer/internal/auth"
	"github.com/vaultviewer/vaultviewer/internal/backup"
	"github.com/vaultviewer/vaultviewer/internal/model"
	"github.com/vaultviewer/vaultviewer/internal/storage"
	"github.com/vaultviewer/vaultviewer/internal/storage/k8s"
	"github.com/vaultviewer/vaultviewer/internal/storage/local"
)

func main() {
	recorder := audit.NewMemoryRecorder()

	mode := strings.ToLower(envOr("VAULTVIEWER_MODE", "local"))
	engine, configInfo, err := buildEngine(mode, recorder)
	if err != nil {
		log.Fatalf("init storage engine: %v", err)
	}
	// Separate from the storage mode above (which can be "local" even when
	// the process itself runs inside Kubernetes, e.g. a PVC-backed
	// Deployment): this only labels *where the process is running* for the
	// UI badge next to the app name. Plain binary/Docker runs default to
	// "LOCAL"; the Helm chart sets it to "CLUSTER" (or a custom cluster
	// name) via VAULTVIEWER_DEPLOYMENT_LABEL.
	configInfo["deployment"] = envOr("VAULTVIEWER_DEPLOYMENT_LABEL", "LOCAL")

	// S3/MinIO backup only applies to local mode — that's the only mode
	// with a directory on disk to mirror; cluster mode's data already lives
	// in Kubernetes Secrets (backed by etcd), out of scope here. Disabled
	// unless VAULTVIEWER_S3_ENDPOINT (etc.) is set, so this is a no-op for
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

	ldapCfg, err := auth.LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("load LDAP config: %v", err)
	}
	authenticator := auth.NewLDAPAuthenticator(ldapCfg)
	sm := auth.NewSessionManager(sessionSecret(), sessionTTL())

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
		user, err := authenticator.Authenticate(creds.Username, creds.Password)
		if err != nil {
			status := http.StatusUnauthorized
			if !errors.Is(err, auth.ErrInvalidCredentials) && !errors.Is(err, auth.ErrNoRole) {
				log.Printf("ldap authenticate error: %v", err)
				status = http.StatusBadGateway
			}
			http.Error(w, "authentication failed", status)
			return
		}
		token, err := sm.Issue(*user)
		if err != nil {
			http.Error(w, "failed to issue session", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"token":    token,
			"username": user.Username,
			"role":     string(user.Role),
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

	mux.HandleFunc("/api/audit", auth.RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		entries, err := recorder.All()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	}))

	mux.HandleFunc("/api/me", auth.RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, user model.User) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"username": user.Username,
			"role":     string(user.Role),
		})
	}))

	mux.HandleFunc("/api/config", auth.RequireAuth(sm, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(configInfo)
	}))

	staticDir := envOr("VAULTVIEWER_STATIC_DIR", "web/dist")
	if _, err := os.Stat(staticDir); err == nil {
		mux.Handle("/", http.FileServer(http.Dir(staticDir)))
		log.Printf("serving frontend from %s", staticDir)
	} else {
		log.Printf("no frontend build found at %s; API-only mode", staticDir)
	}

	// Named HTTP_PORT rather than PORT: Kubernetes auto-injects a
	// "<SERVICE_NAME>_PORT" env var (e.g. VAULTVIEWER_PORT) for every
	// Service visible to the pod, which would otherwise collide with and
	// silently override this one.
	addr := ":" + envOr("VAULTVIEWER_HTTP_PORT", "8080")
	log.Printf("vaultviewer server listening on %s (mode=%s)", addr, mode)
	log.Fatal(http.ListenAndServe(addr, withCORS(mux)))
}

// buildEngine constructs the storage backend selected by VAULTVIEWER_MODE:
//
//	local   (default) — a host/PVC directory mounted at VAULT_LOCAL_ROOT.
//	cluster            — Kubernetes Secrets in VAULTVIEWER_K8S_NAMESPACE,
//	                     using the pod's in-cluster service account.
//
// It also returns the payload served at /api/config, so the frontend's
// mode/backend badges always reflect how the server actually started.
func buildEngine(mode string, recorder storage.AuditRecorder) (storage.VaultStorageEngine, map[string]string, error) {
	switch mode {
	case "local":
		root := envOr("VAULT_LOCAL_ROOT", "/data")
		engine, err := local.New(root, recorder)
		if err != nil {
			return nil, nil, fmt.Errorf("init local storage engine: %w", err)
		}
		log.Printf("local storage engine rooted at %s", root)
		return engine, map[string]string{"mode": "LOCAL", "backend": "filesystem", "root": root}, nil
	case "cluster":
		namespace := os.Getenv("VAULTVIEWER_K8S_NAMESPACE")
		if namespace == "" {
			return nil, nil, errors.New("VAULTVIEWER_K8S_NAMESPACE is required when VAULTVIEWER_MODE=cluster")
		}
		engine, err := k8s.NewInCluster(namespace, recorder)
		if err != nil {
			return nil, nil, fmt.Errorf("init cluster storage engine: %w", err)
		}
		log.Printf("cluster storage engine targeting namespace %s", namespace)
		return engine, map[string]string{"mode": "CLUSTER", "backend": "kubernetes-secrets", "namespace": namespace}, nil
	default:
		return nil, nil, fmt.Errorf("unknown VAULTVIEWER_MODE %q (expected \"local\" or \"cluster\")", mode)
	}
}

// withCORS allows a separately served frontend (e.g. the Vite dev server)
// to call this API from another origin. Origins are restricted to an
// explicit allowlist rather than reflecting any Origin header. Not needed
// when the frontend is served from this same process (the default), since
// same-origin requests bypass CORS entirely.
func withCORS(next http.Handler) http.Handler {
	allowedOrigin := envOr("VAULTVIEWER_CORS_ORIGIN", "http://localhost:5173")
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

// sessionSecret loads the HMAC key used to sign session tokens from
// VAULTVIEWER_SESSION_SECRET. If unset, a random key is generated for the
// life of this process — sessions won't survive a restart, but the secret
// is never hardcoded, per project policy.
func sessionSecret() []byte {
	if v := os.Getenv("VAULTVIEWER_SESSION_SECRET"); v != "" {
		return []byte(v)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatalf("generate session secret: %v", err)
	}
	log.Printf("warning: VAULTVIEWER_SESSION_SECRET not set; generated an ephemeral key, sessions will not survive a restart")
	return secret
}

func sessionTTL() time.Duration {
	hours := 8
	if v := os.Getenv("VAULTVIEWER_SESSION_TTL_HOURS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			log.Fatalf("invalid VAULTVIEWER_SESSION_TTL_HOURS %q: must be a positive integer", v)
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
