package opa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testGrantsDoc = `{
  "decision_id": "test",
  "result": {
    "groups": {
      "dt-bi-adm": [{"team": "bi", "role": "adm"}],
      "view-all": [{"team": "*", "role": "view"}]
    },
    "teams": {
      "bi": {"catalogs": ["bi", "bi_mart"]},
      "ml": {"catalogs": ["ml"]}
    },
    "role_ops": {
      "adm": ["SelectFromColumns", "DropTable"],
      "view": ["SelectFromColumns"]
    }
  }
}`

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/data/grants" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(testGrantsDoc))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestResolveDirectTeam(t *testing.T) {
	srv := newTestServer(t)
	c := NewClient(Config{Endpoint: strings.TrimPrefix(srv.URL, "http://")})

	grants, err := c.Resolve(context.Background(), "dt-bi-adm")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(grants) != 1 {
		t.Fatalf("expected 1 grant, got %d", len(grants))
	}
	g := grants[0]
	if g.Team != "bi" || g.Role != "adm" {
		t.Errorf("grant = %+v, want team=bi role=adm", g)
	}
	if len(g.Catalogs) != 2 || g.Catalogs[0] != "bi" || g.Catalogs[1] != "bi_mart" {
		t.Errorf("Catalogs = %v, want [bi bi_mart]", g.Catalogs)
	}
	if len(g.Operations) != 2 {
		t.Errorf("Operations = %v, want 2 ops", g.Operations)
	}
}

func TestResolveWildcardTeamUnionsAllCatalogs(t *testing.T) {
	srv := newTestServer(t)
	c := NewClient(Config{Endpoint: strings.TrimPrefix(srv.URL, "http://")})

	grants, err := c.Resolve(context.Background(), "view-all")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(grants) != 1 {
		t.Fatalf("expected 1 grant, got %d", len(grants))
	}
	want := map[string]bool{"bi": true, "bi_mart": true, "ml": true}
	if len(grants[0].Catalogs) != len(want) {
		t.Fatalf("Catalogs = %v, want union of all teams' catalogs", grants[0].Catalogs)
	}
	for _, c := range grants[0].Catalogs {
		if !want[c] {
			t.Errorf("unexpected catalog %q in wildcard resolution", c)
		}
	}
}

func TestResolveUnmappedGroupReturnsNoGrantsNoError(t *testing.T) {
	srv := newTestServer(t)
	c := NewClient(Config{Endpoint: strings.TrimPrefix(srv.URL, "http://")})

	grants, err := c.Resolve(context.Background(), "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(grants) != 0 {
		t.Errorf("expected no grants for an empty group, got %v", grants)
	}
}

func TestResolveUnreachable(t *testing.T) {
	c := NewClient(Config{Endpoint: "127.0.0.1:1"})
	if _, err := c.Resolve(context.Background(), "dt-bi-adm"); err == nil {
		t.Fatalf("expected an error dialing a closed port")
	}
}

func TestResolveTeamsLooksUpByTeamNameNotGroupCN(t *testing.T) {
	srv := newTestServer(t)
	c := NewClient(Config{Endpoint: strings.TrimPrefix(srv.URL, "http://")})

	// "bi-adm" isn't a key in testGrantsDoc's groups map at all (only
	// "dt-bi-adm" is) — ResolveTeams must still resolve it via the team
	// name "bi" directly, proving it doesn't depend on OPA's groups map.
	grants, err := c.ResolveTeams(context.Background(), []TeamRole{
		{Team: "bi", Role: "adm"},
		{Team: "ml", Role: "view"},
	})
	if err != nil {
		t.Fatalf("ResolveTeams: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("expected 2 grants, got %d: %+v", len(grants), grants)
	}
	if grants[0].Team != "bi" || len(grants[0].Catalogs) != 2 {
		t.Errorf("grants[0] = %+v, want team=bi with 2 catalogs", grants[0])
	}
	if grants[1].Team != "ml" || len(grants[1].Catalogs) != 1 || grants[1].Catalogs[0] != "ml" {
		t.Errorf("grants[1] = %+v, want team=ml with catalogs=[ml]", grants[1])
	}
	if len(grants[1].Operations) != 1 || grants[1].Operations[0] != "SelectFromColumns" {
		t.Errorf("grants[1].Operations = %v, want [SelectFromColumns] (role_ops[view])", grants[1].Operations)
	}
}

func TestResolveTeamsEmptyInputReturnsNoGrantsNoError(t *testing.T) {
	srv := newTestServer(t)
	c := NewClient(Config{Endpoint: strings.TrimPrefix(srv.URL, "http://")})

	grants, err := c.ResolveTeams(context.Background(), nil)
	if err != nil {
		t.Fatalf("ResolveTeams: %v", err)
	}
	if len(grants) != 0 {
		t.Errorf("expected no grants for empty input, got %v", grants)
	}
}
