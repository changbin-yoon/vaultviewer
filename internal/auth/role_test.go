package auth

import (
	"testing"

	"github.com/vaultviewer/vaultviewer/internal/model"
)

func TestResolveRolePicksHighestPrecedence(t *testing.T) {
	mapping := map[string]model.Role{
		"dt-bi-adm":  model.RoleAdmin,
		"dt-bi-dev":  model.RoleDev,
		"dt-bi-view": model.RoleView,
	}

	role, ok := ResolveRole([]string{"dt-bi-view", "dt-bi-adm"}, mapping)
	if !ok || role != model.RoleAdmin {
		t.Fatalf("got role=%q ok=%v, want RoleAdmin", role, ok)
	}
}

func TestResolveRoleNoMatchingGroup(t *testing.T) {
	mapping := map[string]model.Role{"adm": model.RoleAdmin}
	if _, ok := ResolveRole([]string{"some-other-group"}, mapping); ok {
		t.Fatalf("expected no role to be resolved")
	}
}

func TestResolveRoleEmptyGroups(t *testing.T) {
	mapping := map[string]model.Role{"adm": model.RoleAdmin}
	if _, ok := ResolveRole(nil, mapping); ok {
		t.Fatalf("expected no role for a user with no groups")
	}
}
