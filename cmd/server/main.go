package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/accesslens/accesslens/internal/api"
	"github.com/accesslens/accesslens/internal/audit"
	"github.com/accesslens/accesslens/internal/auth"
	"github.com/accesslens/accesslens/internal/backup"
	"github.com/accesslens/accesslens/internal/opa"
	"github.com/accesslens/accesslens/internal/s3iam"
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
	groupFilterTmpl, err := auth.ParseGroupFilterTemplate(ldapCfg.GroupSearchFilter)
	if err != nil {
		log.Fatalf("parse LDAP group search filter: %v", err)
	}
	authenticator := auth.NewLDAPAuthenticator(ldapCfg, teamsStore, groupFilterTmpl)
	sm := auth.NewSessionManager(sessionSecret(), sessionTTL())
	loginThrottle := auth.NewLoginThrottle()

	// Independent of storage mode (unlike S3 backup above) — a Dashboard
	// status card, not tied to how/where vault data is stored. Disabled
	// unless ACCESSLENS_TRINO_ENDPOINT (etc.) is set.
	trinoCfg := trino.LoadConfigFromEnv()
	trinoClient := trino.NewClient(trinoCfg)

	// Same Dashboard-status-card shape as Trino above. Disabled unless
	// ACCESSLENS_OPA_ENDPOINT is set.
	opaCfg := opa.LoadConfigFromEnv()
	opaClient := opa.NewClient(opaCfg)

	// Same shape again — connectivity check via a fixed LDAP service
	// account, config-driven role/bucket labels. Disabled unless
	// ACCESSLENS_S3IAM_ENDPOINT is set.
	s3iamCfg := s3iam.LoadConfigFromEnv()
	s3iamClient := s3iam.NewClient(s3iamCfg)

	staticDir := envOr("ACCESSLENS_STATIC_DIR", "web/dist")
	if _, err := os.Stat(staticDir); err == nil {
		log.Printf("serving frontend from %s", staticDir)
	} else {
		log.Printf("no frontend build found at %s; API-only mode", staticDir)
		staticDir = ""
	}

	router := api.NewRouter(api.Deps{
		Engine:        engine,
		Sessions:      sm,
		Authenticator: authenticator,
		LoginThrottle: loginThrottle,
		Recorder:      recorder,
		TeamsStore:    teamsStore,
		Trino:         trinoCfg,
		TrinoClient:   trinoClient,
		Opa:           opaCfg,
		OpaClient:     opaClient,
		S3Iam:         s3iamCfg,
		S3IamClient:   s3iamClient,
		ConfigInfo:    configInfo,
		StaticDir:     staticDir,
		CORSOrigin:    envOr("ACCESSLENS_CORS_ORIGIN", "http://localhost:5173"),
		MaxBodyBytes:  maxBodyBytes(),
	})

	// Named HTTP_PORT rather than PORT: Kubernetes auto-injects a
	// "<SERVICE_NAME>_PORT" env var (e.g. ACCESSLENS_PORT) for every
	// Service visible to the pod, which would otherwise collide with and
	// silently override this one.
	addr := ":" + envOr("ACCESSLENS_HTTP_PORT", "8080")
	srv := &http.Server{Addr: addr, Handler: router}

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

// maxBodyBytes caps PUT/POST /api/file request bodies. Default (20MiB) is
// sized off the largest attachment actually seen in a live vault (~2.5MB
// images embedded in notes, as of 2026-08-25) with generous headroom, not
// off the typical note body (a few KB) — a tighter default would reject
// legitimate attachment uploads.
func maxBodyBytes() int64 {
	const defaultBytes = 20 * 1024 * 1024
	v := os.Getenv("ACCESSLENS_MAX_BODY_BYTES")
	if v == "" {
		return defaultBytes
	}
	parsed, err := strconv.ParseInt(v, 10, 64)
	if err != nil || parsed <= 0 {
		log.Fatalf("invalid ACCESSLENS_MAX_BODY_BYTES %q: must be a positive integer", v)
	}
	return parsed
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
