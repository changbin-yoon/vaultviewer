package opa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

// Client fetches OPA's grants document and resolves it for a given LDAP
// group — the same "policy config-driven" spirit as internal/trino, but
// one step more live: the team/catalog/operation data is read straight from
// OPA rather than duplicated into AccessLens's own Helm values.
type Client struct {
	cfg  Config
	http *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 5 * time.Second}}
}

// Grant is one resolved entitlement for an LDAP group — mirrors what
// trino.rego's grants/team_matches/role_ops rules compute for a query, but
// evaluated for display rather than for a specific Trino action.
type Grant struct {
	Team       string   `json:"team"`
	Role       string   `json:"role"`
	Catalogs   []string `json:"catalogs"`
	Operations []string `json:"operations"`
}

type grantsDocument struct {
	Groups map[string][]struct {
		Team string `json:"team"`
		Role string `json:"role"`
	} `json:"groups"`
	Teams map[string]struct {
		Catalogs []string `json:"catalogs"`
	} `json:"teams"`
	RoleOps map[string][]string `json:"role_ops"`
}

type dataResponse struct {
	Result grantsDocument `json:"result"`
}

// Resolve fetches OPA's current grants document and returns what the given
// LDAP group is entitled to. An empty group (no ACCESSLENS_OPA_GROUP_* set
// for the caller's role) or a group OPA doesn't know about resolves to no
// grants, not an error.
func (c *Client) Resolve(ctx context.Context, group string) ([]Grant, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s/v1/data/grants", c.cfg.Endpoint), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("opa: unexpected status %s", resp.Status)
	}

	var doc dataResponse
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("opa: decode grants document: %w", err)
	}

	if group == "" {
		return nil, nil
	}

	allCatalogs := func() []string {
		set := map[string]struct{}{}
		for _, t := range doc.Result.Teams {
			for _, c := range t.Catalogs {
				set[c] = struct{}{}
			}
		}
		out := make([]string, 0, len(set))
		for c := range set {
			out = append(out, c)
		}
		sort.Strings(out)
		return out
	}

	var grants []Grant
	for _, g := range doc.Result.Groups[group] {
		var catalogs []string
		if g.Team == "*" { // matches trino.rego's team_matches(g, _) if g.team == "*"
			catalogs = allCatalogs()
		} else {
			catalogs = doc.Result.Teams[g.Team].Catalogs
		}
		grants = append(grants, Grant{
			Team:       g.Team,
			Role:       g.Role,
			Catalogs:   catalogs,
			Operations: doc.Result.RoleOps[g.Role],
		})
	}
	return grants, nil
}
