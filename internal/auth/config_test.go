package auth

import (
	"bytes"
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

func TestLoadConfigFromEnvDefaultGroupSearchFilter(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	if cfg.GroupSearchFilter != DefaultGroupSearchFilter {
		t.Errorf("GroupSearchFilter = %q, want default %q", cfg.GroupSearchFilter, DefaultGroupSearchFilter)
	}
}

func TestLoadConfigFromEnvCustomGroupSearchFilter(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ACCESSLENS_LDAP_GROUP_SEARCH_FILTER", "(&(objectClass=posixGroup)(memberUid={{.UID}}))")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	want := "(&(objectClass=posixGroup)(memberUid={{.UID}}))"
	if cfg.GroupSearchFilter != want {
		t.Errorf("GroupSearchFilter = %q, want %q", cfg.GroupSearchFilter, want)
	}
}

func TestParseGroupFilterTemplateDefault(t *testing.T) {
	tmpl, err := ParseGroupFilterTemplate(DefaultGroupSearchFilter)
	if err != nil {
		t.Fatalf("ParseGroupFilterTemplate(default): %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, GroupFilterParams{UserDN: "cn=alice,ou=users,dc=example,dc=com"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "(&(objectClass=groupOfNames)(member=cn=alice,ou=users,dc=example,dc=com))"
	if buf.String() != want {
		t.Errorf("rendered filter = %q, want %q", buf.String(), want)
	}
}

func TestParseGroupFilterTemplateSchemas(t *testing.T) {
	cases := []struct {
		name   string
		filter string
	}{
		{"posixGroup", "(&(objectClass=posixGroup)(memberUid={{.UID}}))"},
		{"groupOfUniqueNames", "(&(objectClass=groupOfUniqueNames)(uniqueMember={{.UserDN}}))"},
		{"AD nested group", "(member:1.2.840.113556.1.4.1941:={{.UserDN}})"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseGroupFilterTemplate(tc.filter); err != nil {
				t.Errorf("ParseGroupFilterTemplate(%q): %v", tc.filter, err)
			}
		})
	}
}

func TestParseGroupFilterTemplateRejectsBadSyntax(t *testing.T) {
	if _, err := ParseGroupFilterTemplate("(member={{.UserDN)"); err == nil {
		t.Error("expected error for malformed template syntax, got nil")
	}
}

func TestParseGroupFilterTemplateRejectsUnknownField(t *testing.T) {
	if _, err := ParseGroupFilterTemplate("(member={{.Bogus}})"); err == nil {
		t.Error("expected error for unknown field {{.Bogus}}, got nil")
	}
}
