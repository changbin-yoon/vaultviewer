package opa

import (
	"testing"

	"github.com/accesslens/accesslens/internal/model"
)

func TestConfigEnabled(t *testing.T) {
	if !(Config{Endpoint: "opa:8181"}).Enabled() {
		t.Errorf("expected Enabled() with Endpoint set")
	}
	if (Config{}).Enabled() {
		t.Errorf("expected Enabled() false for zero value")
	}
}

func TestLoadConfigFromEnvDisabledByDefault(t *testing.T) {
	if LoadConfigFromEnv().Enabled() {
		t.Errorf("expected opa disabled when ACCESSLENS_OPA_ENDPOINT is unset")
	}
}

func TestLoadConfigFromEnvGroupMap(t *testing.T) {
	t.Setenv("ACCESSLENS_OPA_ENDPOINT", "opa.example:8181")
	t.Setenv("ACCESSLENS_OPA_GROUP_ADM", "dt-bi-adm")

	cfg := LoadConfigFromEnv()
	if !cfg.Enabled() {
		t.Fatalf("expected Enabled() with ACCESSLENS_OPA_ENDPOINT set")
	}
	if cfg.GroupMap[model.RoleAdmin] != "dt-bi-adm" {
		t.Errorf("GroupMap[adm] = %q, want %q", cfg.GroupMap[model.RoleAdmin], "dt-bi-adm")
	}
	if cfg.GroupMap[model.RoleDev] != "" {
		t.Errorf("GroupMap[dev] = %q, want empty (unset env var)", cfg.GroupMap[model.RoleDev])
	}
}
