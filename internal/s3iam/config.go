// Package s3iam checks connectivity to an S3-compatible endpoint's LDAP
// identity provider (MinIO's AssumeRoleWithLDAPIdentity STS action) using a
// fixed service account, and reports operator-configured role/bucket
// labels — the same "connectivity check + config-driven labels" shape as
// internal/trino, not a live bucket-policy lookup. Kept separate from
// storage/auth/audit per the project's functional-separation convention.
package s3iam

import (
	"os"
	"strings"

	"github.com/accesslens/accesslens/internal/model"
)

// Config configures the S3 IAM status client. All values are sourced from
// the environment (or a Kubernetes Secret projected into it) — the LDAP
// username/password are never hardcoded, per project policy.
type Config struct {
	// Endpoint is the S3 API host:port, e.g. "minio.example:9000". Always
	// dialed over HTTP — MinIO in this deployment has no TLS in front of
	// it (unlike Trino's coordinator).
	Endpoint string
	// LDAPUsername/LDAPPassword authenticate a fixed service account
	// against the S3 endpoint's LDAP identity provider via
	// AssumeRoleWithLDAPIdentity — the same directory AccessLens itself
	// authenticates against (see internal/auth), just a dedicated
	// least-privilege account rather than the logged-in user's own
	// credentials (which AccessLens doesn't retain after login).
	LDAPUsername string
	LDAPPassword string
	// RoleMap maps an AccessLens role to the label shown on the
	// dashboard. Defaults to reusing the AccessLens role name unchanged.
	RoleMap map[model.Role]string
	// Buckets is the configured list of bucket names to display for
	// accounts with no team-scoped LDAP groups (see model.TeamGrant) — not
	// queried live from the S3 endpoint.
	Buckets []string
	// PolicyDir is the directory holding the mirrored MinIO/AIStor policy
	// documents (this repo's policy/generated, mounted as a ConfigMap in
	// the cluster). Empty disables the access breakdown while leaving the
	// rest of the S3 IAM card working — the connectivity check and bucket
	// list don't depend on it.
	PolicyDir string
	// Probe turns on live verification: the dashboard asks the S3 backend
	// what the logged-in user can actually do, using that user's own
	// temporary session, instead of only reporting what the mirrored policy
	// says. Off by default — it costs extra round trips per dashboard load.
	Probe bool
	// ProbeWrite additionally verifies write access. Separate from Probe
	// because read and delete are settled against a key that does not exist
	// and touch nothing, while write can only be settled by writing.
	// Requires the master credentials below; without them it stays off.
	ProbeWrite bool
	// MasterAccessKey/MasterSecretKey identify the account that removes
	// what the write probe writes. A separate account is needed because the
	// user being tested may hold PutObject without DeleteObject — the dev
	// tier does exactly that — and so cannot clean up after itself.
	// See policy/accesslens-master.json.
	MasterAccessKey string
	MasterSecretKey string
	// AttachmentsPath is the YAML declaration of which policies are attached
	// to which LDAP group/user DNs (see Attachments). Required whenever
	// PolicyDir is set: the policy documents say what a policy permits, this
	// says who holds it, and neither half is useful alone.
	AttachmentsPath string
	// BucketMap maps a team name (e.g. "bi", matching model.TeamGrant.Team)
	// to the bucket(s) that team can access. For an account with
	// team-scoped groups, internal/api computes its displayed buckets as
	// the union of BucketMap[team] across every team it belongs to,
	// deduplicated — falling back to the flat Buckets list above when the
	// account has no team grants at all.
	BucketMap map[string][]string
}

// Enabled reports whether enough configuration is present to check S3 IAM
// connectivity. Opt-in: leaving ACCESSLENS_S3IAM_ENDPOINT unset disables it
// entirely rather than failing startup.
func (c Config) Enabled() bool {
	return c.Endpoint != "" && c.LDAPUsername != "" && c.LDAPPassword != ""
}

// LoadConfigFromEnv builds a Config from environment variables:
//
//	ACCESSLENS_S3IAM_ENDPOINT       (unset disables the integration entirely)
//	ACCESSLENS_S3IAM_LDAP_USERNAME
//	ACCESSLENS_S3IAM_LDAP_PASSWORD
//	ACCESSLENS_S3IAM_ROLE_ADM       (default "adm")
//	ACCESSLENS_S3IAM_ROLE_DEV       (default "dev")
//	ACCESSLENS_S3IAM_ROLE_VIEW      (default "view")
//	ACCESSLENS_S3IAM_POLICY_DIR     (unset disables the access breakdown)
//	ACCESSLENS_S3IAM_ATTACHMENTS    (policy-to-DN declaration, required with POLICY_DIR)
//	ACCESSLENS_S3IAM_PROBE          ("true" enables live read/delete verification)
//	ACCESSLENS_S3IAM_PROBE_WRITE    ("true" also verifies write; needs master keys)
//	ACCESSLENS_S3IAM_MASTER_ACCESS_KEY
//	ACCESSLENS_S3IAM_MASTER_SECRET_KEY
//	ACCESSLENS_S3IAM_BUCKETS        (comma-separated, default empty)
//	ACCESSLENS_S3IAM_BUCKET_<TEAM>  (comma-separated, one per team — e.g.
//	                                ACCESSLENS_S3IAM_BUCKET_BI="team-bi")
func LoadConfigFromEnv() Config {
	cfg := Config{
		Endpoint:     os.Getenv("ACCESSLENS_S3IAM_ENDPOINT"),
		LDAPUsername: os.Getenv("ACCESSLENS_S3IAM_LDAP_USERNAME"),
		LDAPPassword: os.Getenv("ACCESSLENS_S3IAM_LDAP_PASSWORD"),
		RoleMap: map[model.Role]string{
			model.RoleAdmin: envOr("ACCESSLENS_S3IAM_ROLE_ADM", "adm"),
			model.RoleDev:   envOr("ACCESSLENS_S3IAM_ROLE_DEV", "dev"),
			model.RoleView:  envOr("ACCESSLENS_S3IAM_ROLE_VIEW", "view"),
		},
		// Non-nil so the /api/s3iam response serializes as `[]`, not
		// `null`, when no buckets are configured.
		PolicyDir:       os.Getenv("ACCESSLENS_S3IAM_POLICY_DIR"),
		AttachmentsPath: os.Getenv("ACCESSLENS_S3IAM_ATTACHMENTS"),
		Probe:           os.Getenv("ACCESSLENS_S3IAM_PROBE") == "true",
		ProbeWrite:      os.Getenv("ACCESSLENS_S3IAM_PROBE_WRITE") == "true",
		MasterAccessKey: os.Getenv("ACCESSLENS_S3IAM_MASTER_ACCESS_KEY"),
		MasterSecretKey: os.Getenv("ACCESSLENS_S3IAM_MASTER_SECRET_KEY"),
		Buckets:         []string{},
		BucketMap:       map[string][]string{},
	}
	if v := os.Getenv("ACCESSLENS_S3IAM_BUCKETS"); v != "" {
		for _, b := range strings.Split(v, ",") {
			b = strings.TrimSpace(b)
			if b != "" {
				cfg.Buckets = append(cfg.Buckets, b)
			}
		}
	}
	const bucketPrefix = "ACCESSLENS_S3IAM_BUCKET_"
	for _, kv := range os.Environ() {
		key, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, bucketPrefix) {
			continue
		}
		team := strings.ToLower(strings.TrimPrefix(key, bucketPrefix))
		for _, b := range strings.Split(value, ",") {
			b = strings.TrimSpace(b)
			if b != "" {
				cfg.BucketMap[team] = append(cfg.BucketMap[team], b)
			}
		}
	}
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
