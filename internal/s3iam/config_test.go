package s3iam

import (
	"testing"

	"github.com/accesslens/accesslens/internal/model"
)

func TestConfigEnabled(t *testing.T) {
	full := Config{Endpoint: "minio:9000", LDAPUsername: "u", LDAPPassword: "p"}
	if !full.Enabled() {
		t.Errorf("expected Enabled() with all fields set")
	}
	if (Config{}).Enabled() {
		t.Errorf("expected Enabled() false for zero value")
	}
	missingPassword := full
	missingPassword.LDAPPassword = ""
	if missingPassword.Enabled() {
		t.Errorf("expected Enabled() false when LDAPPassword is missing")
	}
}

func TestLoadConfigFromEnvDisabledByDefault(t *testing.T) {
	if LoadConfigFromEnv().Enabled() {
		t.Errorf("expected s3iam disabled when no ACCESSLENS_S3IAM_* env vars are set")
	}
}

func TestLoadConfigFromEnvRoleMapDefaults(t *testing.T) {
	cfg := LoadConfigFromEnv()
	want := map[model.Role]string{model.RoleAdmin: "adm", model.RoleDev: "dev", model.RoleView: "view"}
	for role, label := range want {
		if cfg.RoleMap[role] != label {
			t.Errorf("RoleMap[%q] = %q, want %q", role, cfg.RoleMap[role], label)
		}
	}
}

func TestLoadConfigFromEnvCustomBuckets(t *testing.T) {
	t.Setenv("ACCESSLENS_S3IAM_BUCKETS", "team-bi, team-ml,team-ops")
	cfg := LoadConfigFromEnv()
	want := []string{"team-bi", "team-ml", "team-ops"}
	if len(cfg.Buckets) != len(want) {
		t.Fatalf("Buckets = %v, want %v", cfg.Buckets, want)
	}
	for i, b := range want {
		if cfg.Buckets[i] != b {
			t.Errorf("Buckets[%d] = %q, want %q", i, cfg.Buckets[i], b)
		}
	}
}

func TestLoadConfigFromEnvBucketMap(t *testing.T) {
	t.Setenv("ACCESSLENS_S3IAM_BUCKET_BI", "team-bi")
	t.Setenv("ACCESSLENS_S3IAM_BUCKET_ML", "team-ml, team-ml-archive")
	cfg := LoadConfigFromEnv()
	if got := cfg.BucketMap["bi"]; len(got) != 1 || got[0] != "team-bi" {
		t.Errorf("BucketMap[bi] = %v, want [team-bi]", got)
	}
	if got := cfg.BucketMap["ml"]; len(got) != 2 || got[0] != "team-ml" || got[1] != "team-ml-archive" {
		t.Errorf("BucketMap[ml] = %v, want [team-ml team-ml-archive]", got)
	}
}
