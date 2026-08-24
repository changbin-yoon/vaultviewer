package auth

import (
	"testing"

	"github.com/accesslens/accesslens/internal/model"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"ACCESSLENS_LDAP_HOST":          "ldap.example.com",
		"ACCESSLENS_LDAP_BASE_DN":       "dc=example,dc=com",
		"ACCESSLENS_LDAP_BIND_DN":       "cn=svc,dc=example,dc=com",
		"ACCESSLENS_LDAP_BIND_PASSWORD": "secret",
	} {
		t.Setenv(k, v)
	}
}

func TestLoadConfigFromEnvCommaSeparatedGroups(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ACCESSLENS_LDAP_GROUP_ADM", "dt-bi-adm, platform-admins")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	if cfg.GroupRoleMap["dt-bi-adm"] != model.RoleAdmin {
		t.Errorf("dt-bi-adm not mapped to RoleAdmin")
	}
	if cfg.GroupRoleMap["platform-admins"] != model.RoleAdmin {
		t.Errorf("platform-admins not mapped to RoleAdmin")
	}
}

func TestLoadConfigFromEnvDefaultGroups(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	want := map[string]model.Role{"adm": model.RoleAdmin, "dev": model.RoleDev, "view": model.RoleView}
	for cn, role := range want {
		if cfg.GroupRoleMap[cn] != role {
			t.Errorf("GroupRoleMap[%q] = %q, want %q", cn, cfg.GroupRoleMap[cn], role)
		}
	}
}
