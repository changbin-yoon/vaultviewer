package auth

import "github.com/vaultviewer/vaultviewer/internal/model"

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
