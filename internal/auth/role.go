package auth

import (
	"regexp"
	"sort"
	"strings"

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

// GroupCN extracts the common name from a group's distinguished name —
// "cn=bi-dev,ou=groups,dc=example,dc=com" becomes "bi-dev".
//
// The session carries group DNs because that is what an external system such
// as MinIO keys its policy attachments on (see model.User). The UI wants the
// short name, and deriving it here avoids widening the session token a second
// time to carry both spellings of the same thing.
//
// A DN whose first component is not a cn= is returned unchanged: it is more
// useful to show an operator something they can recognise than a blank.
func GroupCN(dn string) string {
	first, _, _ := strings.Cut(dn, ",")
	name, value, found := strings.Cut(first, "=")
	if !found || !strings.EqualFold(strings.TrimSpace(name), "cn") {
		return dn
	}
	return strings.TrimSpace(value)
}
