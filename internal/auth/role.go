package auth

import (
	"regexp"
	"sort"

	"github.com/accesslens/accesslens/internal/model"
)

// teamGroupPattern matches an LDAP group CN of the form "<team>-<role>" or
// "<team>_<role>" — e.g. cluster-mesh1's bi-adm/bi-dev/bi-view, ml-*, ops-*
// test groups (real data uses a hyphen; underscore is also accepted since
// it's an equally common convention and costs nothing extra to support).
var teamGroupPattern = regexp.MustCompile(`^(.+)[-_](adm|dev|view)$`)

// ResolveTeams extracts every team-scoped role grant from a user's LDAP
// group CNs (see TeamGrant's doc comment). Unlike ResolveRole, this doesn't
// consult GroupRoleMap — it's a pure naming-convention parse, so it works
// for any group whose CN matches the pattern regardless of whether that CN
// is also wired into GroupRoleMap for the overall app role. Result is
// sorted by team name for a stable UI order.
func ResolveTeams(groupCNs []string) []model.TeamGrant {
	var teams []model.TeamGrant
	for _, cn := range groupCNs {
		m := teamGroupPattern.FindStringSubmatch(cn)
		if m == nil {
			continue
		}
		teams = append(teams, model.TeamGrant{Team: m[1], Role: model.Role(m[2])})
	}
	sort.Slice(teams, func(i, j int) bool { return teams[i].Team < teams[j].Team })
	return teams
}

// rolePrecedence orders roles from most to least privileged, used when a
// user's LDAP groups map to more than one role.
var rolePrecedence = []model.Role{model.RoleAdmin, model.RoleDev, model.RoleView}

// ResolveRole maps a user's LDAP group CNs to the most privileged role they
// grant under mapping. It reports false if none of the groups are mapped,
// meaning the user is authenticated but has no assigned role.
func ResolveRole(groupCNs []string, mapping map[string]model.Role) (model.Role, bool) {
	granted := make(map[model.Role]bool, len(groupCNs))
	for _, cn := range groupCNs {
		if role, ok := mapping[cn]; ok {
			granted[role] = true
		}
	}
	for _, role := range rolePrecedence {
		if granted[role] {
			return role, true
		}
	}
	return "", false
}
