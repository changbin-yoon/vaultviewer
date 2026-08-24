package trino

import (
	"testing"

	"github.com/accesslens/accesslens/internal/model"
)

func TestConfigEnabled(t *testing.T) {
	full := Config{Endpoint: "trino:8443", Username: "u", Password: "p"}
	if !full.Enabled() {
		t.Errorf("expected Enabled() with all fields set")
	}
	if (Config{}).Enabled() {
		t.Errorf("expected Enabled() false for zero value")
	}
	missingPassword := full
	missingPassword.Password = ""
	if missingPassword.Enabled() {
		t.Errorf("expected Enabled() false when Password is missing")
	}
}

func TestLoadConfigFromEnvDisabledByDefault(t *testing.T) {
	cfg := LoadConfigFromEnv()
	if cfg.Enabled() {
		t.Errorf("expected trino disabled when no ACCESSLENS_TRINO_* env vars are set")
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

func TestLoadConfigFromEnvCustomRoleMapAndCatalogs(t *testing.T) {
	t.Setenv("ACCESSLENS_TRINO_ROLE_ADM", "sysadmin")
	t.Setenv("ACCESSLENS_TRINO_CATALOGS", "hive, iceberg,tpch")

	cfg := LoadConfigFromEnv()
	if cfg.RoleMap[model.RoleAdmin] != "sysadmin" {
		t.Errorf("RoleMap[adm] = %q, want %q", cfg.RoleMap[model.RoleAdmin], "sysadmin")
	}
	wantCatalogs := []string{"hive", "iceberg", "tpch"}
	if len(cfg.Catalogs) != len(wantCatalogs) {
		t.Fatalf("Catalogs = %v, want %v", cfg.Catalogs, wantCatalogs)
	}
	for i, c := range wantCatalogs {
		if cfg.Catalogs[i] != c {
			t.Errorf("Catalogs[%d] = %q, want %q", i, cfg.Catalogs[i], c)
		}
	}
}
