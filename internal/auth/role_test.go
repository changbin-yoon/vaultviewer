package auth

import (
	"reflect"
	"testing"

	"github.com/accesslens/accesslens/internal/model"
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

func TestResolveTeamsParsesHyphenAndUnderscoreSeparators(t *testing.T) {
	got := ResolveTeams([]string{"bi-adm", "ml_dev", "ops-view", "adm", "some-other-group"})
	want := []model.TeamGrant{
		{Team: "bi", Role: model.RoleAdmin},
		{Team: "ml", Role: model.RoleDev},
		{Team: "ops", Role: model.RoleView},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestResolveTeamsIgnoresNonTeamGroups(t *testing.T) {
	got := ResolveTeams([]string{"adm", "dev", "view", "platform-admins"})
	if len(got) != 0 {
		t.Fatalf("expected no team grants, got %+v", got)
	}
}

func TestResolveTeamsSortsByTeamName(t *testing.T) {
	got := ResolveTeams([]string{"ops-adm", "bi-view", "ml-dev"})
	want := []model.TeamGrant{
		{Team: "bi", Role: model.RoleView},
		{Team: "ml", Role: model.RoleDev},
		{Team: "ops", Role: model.RoleAdmin},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
