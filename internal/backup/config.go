// Package backup periodically mirrors the local-mode storage directory to
// an S3-compatible bucket (MinIO or AWS S3), so a lost/corrupted PVC isn't
// unrecoverable. Kept separate from storage/auth/audit per the project's
// functional-separation convention.
package backup

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config configures the S3 backup syncer. All values are sourced from the
// environment (or a Kubernetes Secret projected into it) — the access/secret
// key are never hardcoded, per project policy.
type Config struct {
	// Endpoint is the S3/MinIO host:port, e.g. "minio.example.svc.cluster.local:9000".
	Endpoint string
	// AccessKey/SecretKey authenticate against the bucket.
	AccessKey string
	SecretKey string
	// Bucket must already exist — this package never creates or deletes
	// buckets, only objects inside one.
	Bucket string
	Region string
	UseSSL bool
	// Prefix namespaces objects inside the bucket (e.g. the Helm release
	// name), so multiple VaultViewer instances can safely share one bucket.
	Prefix string
	// Interval between sync passes.
	Interval time.Duration
}

// Enabled reports whether enough configuration is present to start the
// syncer. Backup is opt-in: leaving VAULTVIEWER_S3_ENDPOINT unset disables
// it entirely rather than failing startup.
func (c Config) Enabled() bool {
	return c.Endpoint != "" && c.Bucket != "" && c.AccessKey != "" && c.SecretKey != ""
}

// LoadConfigFromEnv builds a Config from environment variables:
//
//	VAULTVIEWER_S3_ENDPOINT               (unset disables backup entirely)
//	VAULTVIEWER_S3_ACCESS_KEY
//	VAULTVIEWER_S3_SECRET_KEY
//	VAULTVIEWER_S3_BUCKET
//	VAULTVIEWER_S3_REGION                 (default "us-east-1")
//	VAULTVIEWER_S3_USE_SSL                (default false, matches VAULTVIEWER_LDAP_TLS convention)
//	VAULTVIEWER_S3_PREFIX                 (default "")
//	VAULTVIEWER_BACKUP_INTERVAL_MINUTES   (default 30)
func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		Endpoint:  os.Getenv("VAULTVIEWER_S3_ENDPOINT"),
		AccessKey: os.Getenv("VAULTVIEWER_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("VAULTVIEWER_S3_SECRET_KEY"),
		Bucket:    os.Getenv("VAULTVIEWER_S3_BUCKET"),
		Region:    envOr("VAULTVIEWER_S3_REGION", "us-east-1"),
		UseSSL:    os.Getenv("VAULTVIEWER_S3_USE_SSL") == "true",
		Prefix:    os.Getenv("VAULTVIEWER_S3_PREFIX"),
	}

	minutes := 30
	if v := os.Getenv("VAULTVIEWER_BACKUP_INTERVAL_MINUTES"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("invalid VAULTVIEWER_BACKUP_INTERVAL_MINUTES %q: must be a positive integer", v)
		}
		minutes = parsed
	}
	cfg.Interval = time.Duration(minutes) * time.Minute

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
