package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newTestServer wires up a fake AccessLens backend: /api/login always
// succeeds and issues a fixed token, and every other route requires
// exactly that token in the Authorization header, returning 401 otherwise.
// loginCount lets tests assert how many times the client actually logged in.
func newTestServer(t *testing.T, validToken string, loginCount *atomic.Int32, routes map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		loginCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": validToken})
	})
	for path, handler := range routes {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+validToken {
				http.Error(w, "invalid or expired session", http.StatusUnauthorized)
				return
			}
			handler(w, r)
		})
	}
	return httptest.NewServer(mux)
}

func TestClientLoginsOnceAndReusesToken(t *testing.T) {
	var loginCount atomic.Int32
	calls := 0
	srv := newTestServer(t, "tok-123", &loginCount, map[string]http.HandlerFunc{
		"/api/tree": func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Write([]byte(`[]`))
		},
	})
	defer srv.Close()

	c := newClient(srv.URL, "svc", "pw")
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := c.do(ctx, http.MethodGet, "/api/tree", nil, nil); err != nil {
			t.Fatalf("do: %v", err)
		}
	}
	if got := loginCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 login, got %d", got)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls to /api/tree, got %d", calls)
	}
}

func TestClientReLoginsOnceAfter401(t *testing.T) {
	var loginCount atomic.Int32
	const realToken = "tok-real"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		loginCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": realToken})
	})
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if auth != realToken {
			http.Error(w, "invalid or expired session", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`[{"path":"a.md","snippet":"hit"}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newClient(srv.URL, "svc", "pw")
	// Prime the cache with a token the server does not recognize, simulating
	// an already-cached session that has since expired server-side.
	c.setToken("tok-stale")

	data, err := c.do(context.Background(), http.MethodGet, "/api/search", nil, nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if !strings.Contains(string(data), "a.md") {
		t.Errorf("expected search result body, got %q", data)
	}
	if got := loginCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 re-login after the 401, got %d", got)
	}
	if c.cachedToken() != realToken {
		t.Errorf("expected cached token to be refreshed to %q, got %q", realToken, c.cachedToken())
	}
}

func TestClientSurfacesLoginFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "authentication failed", http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newClient(srv.URL, "svc", "wrong-password")
	if _, err := c.do(context.Background(), http.MethodGet, "/api/tree", nil, nil); err == nil {
		t.Fatal("expected an error when login itself fails, got nil")
	}
}

func TestListTreeEncodesPathQueryParam(t *testing.T) {
	var loginCount atomic.Int32
	var gotQuery string
	srv := newTestServer(t, "tok", &loginCount, map[string]http.HandlerFunc{
		"/api/tree": func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.Query().Get("path")
			w.Write([]byte(`[]`))
		},
	})
	defer srv.Close()

	c := newClient(srv.URL, "svc", "pw")
	_, _, err := c.listTree(context.Background(), &mcp.CallToolRequest{}, listTreeArgs{Path: "04-데이터플랫폼-컴포넌트"})
	if err != nil {
		t.Fatalf("listTree: %v", err)
	}
	if gotQuery != "04-데이터플랫폼-컴포넌트" {
		t.Errorf("expected path query param to round-trip, got %q", gotQuery)
	}
}

func TestSearchVaultEncodesQueryParam(t *testing.T) {
	var loginCount atomic.Int32
	var gotQuery string
	srv := newTestServer(t, "tok", &loginCount, map[string]http.HandlerFunc{
		"/api/search": func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.Query().Get("q")
			w.Write([]byte(`[]`))
		},
	})
	defer srv.Close()

	c := newClient(srv.URL, "svc", "pw")
	_, _, err := c.searchVault(context.Background(), &mcp.CallToolRequest{}, searchArgs{Query: "Trino 장애"})
	if err != nil {
		t.Fatalf("searchVault: %v", err)
	}
	if gotQuery != "Trino 장애" {
		t.Errorf("expected query param to round-trip, got %q", gotQuery)
	}
}

func TestReadNoteDecodesBase64Content(t *testing.T) {
	var loginCount atomic.Int32
	plaintext := "# Trino\ndepends_on: [[HMS-메타스토어]]"
	srv := newTestServer(t, "tok", &loginCount, map[string]http.HandlerFunc{
		"/api/file": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"path":    r.URL.Query().Get("path"),
				"content": base64.StdEncoding.EncodeToString([]byte(plaintext)),
			})
		},
	})
	defer srv.Close()

	c := newClient(srv.URL, "svc", "pw")
	result, _, err := c.readNote(context.Background(), &mcp.CallToolRequest{}, readNoteArgs{Path: "Trino.md"})
	if err != nil {
		t.Fatalf("readNote: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected exactly one content block, got %d", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if text.Text != plaintext {
		t.Errorf("expected decoded plaintext body, got %q", text.Text)
	}
}

func TestGetOntologyGraphAndHistoryPassThroughRawJSON(t *testing.T) {
	var loginCount atomic.Int32
	graphJSON := `{"nodes":[{"id":"a","name":"a","resolved":true}],"edges":[]}`
	historyJSON := `[{"path":"a.md","action":"create","user":"ycb"}]`
	srv := newTestServer(t, "tok", &loginCount, map[string]http.HandlerFunc{
		"/api/graph":   func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(graphJSON)) },
		"/api/history": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(historyJSON)) },
	})
	defer srv.Close()

	c := newClient(srv.URL, "svc", "pw")

	graphResult, _, err := c.getOntologyGraph(context.Background(), &mcp.CallToolRequest{}, emptyArgs{})
	if err != nil {
		t.Fatalf("getOntologyGraph: %v", err)
	}
	if text := graphResult.Content[0].(*mcp.TextContent).Text; text != graphJSON {
		t.Errorf("expected graph JSON to pass through unchanged, got %q", text)
	}

	historyResult, _, err := c.getNoteHistory(context.Background(), &mcp.CallToolRequest{}, historyArgs{Path: "a.md"})
	if err != nil {
		t.Fatalf("getNoteHistory: %v", err)
	}
	if text := historyResult.Content[0].(*mcp.TextContent).Text; text != historyJSON {
		t.Errorf("expected history JSON to pass through unchanged, got %q", text)
	}
}

func TestDoSurfacesNonSuccessStatus(t *testing.T) {
	var loginCount atomic.Int32
	srv := newTestServer(t, "tok", &loginCount, map[string]http.HandlerFunc{
		"/api/file": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "path not found", http.StatusBadRequest)
		},
	})
	defer srv.Close()

	c := newClient(srv.URL, "svc", "pw")
	if _, err := c.do(context.Background(), http.MethodGet, "/api/file", nil, nil); err == nil {
		t.Fatal("expected an error for a non-2xx response, got nil")
	}
}

func TestSaveNoteSendsRawBodyAndQueryParams(t *testing.T) {
	var loginCount atomic.Int32
	var gotMethod, gotPath, gotReason string
	var gotBody []byte
	srv := newTestServer(t, "tok", &loginCount, map[string]http.HandlerFunc{
		"/api/file": func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Query().Get("path")
			gotReason = r.URL.Query().Get("reason")
			gotBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusNoContent)
		},
	})
	defer srv.Close()

	c := newClient(srv.URL, "svc", "pw")
	result, _, err := c.saveNote(context.Background(), &mcp.CallToolRequest{}, saveNoteArgs{
		Path: "04-데이터플랫폼-컴포넌트/Trino.md", Content: "# Trino\n본문", Reason: "mcp test",
	})
	if err != nil {
		t.Fatalf("saveNote: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "04-데이터플랫폼-컴포넌트/Trino.md" {
		t.Errorf("path query param did not round-trip, got %q", gotPath)
	}
	if gotReason != "mcp test" {
		t.Errorf("reason query param did not round-trip, got %q", gotReason)
	}
	if string(gotBody) != "# Trino\n본문" {
		t.Errorf("expected raw note body (no JSON envelope), got %q", gotBody)
	}
	if text := result.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, "04-데이터플랫폼-컴포넌트/Trino.md") {
		t.Errorf("expected confirmation to mention the saved path, got %q", text)
	}
}

func TestSaveNoteSurfacesForbiddenAsToolError(t *testing.T) {
	var loginCount atomic.Int32
	srv := newTestServer(t, "tok", &loginCount, map[string]http.HandlerFunc{
		"/api/file": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "role does not permit write access", http.StatusForbidden)
		},
	})
	defer srv.Close()

	c := newClient(srv.URL, "svc", "pw")
	if _, _, err := c.saveNote(context.Background(), &mcp.CallToolRequest{}, saveNoteArgs{Path: "a.md", Content: "x"}); err == nil {
		t.Fatal("expected the REST 403 to surface as a tool error, got nil")
	}
}

func TestDeleteNoteSendsDelete(t *testing.T) {
	var loginCount atomic.Int32
	var gotMethod, gotPath string
	srv := newTestServer(t, "tok", &loginCount, map[string]http.HandlerFunc{
		"/api/file": func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Query().Get("path")
			w.WriteHeader(http.StatusNoContent)
		},
	})
	defer srv.Close()

	c := newClient(srv.URL, "svc", "pw")
	if _, _, err := c.deleteNote(context.Background(), &mcp.CallToolRequest{}, deleteNoteArgs{Path: "old.md"}); err != nil {
		t.Fatalf("deleteNote: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if gotPath != "old.md" {
		t.Errorf("path query param did not round-trip, got %q", gotPath)
	}
}

func TestRenameNoteSendsFromAndTo(t *testing.T) {
	var loginCount atomic.Int32
	var gotFrom, gotTo string
	srv := newTestServer(t, "tok", &loginCount, map[string]http.HandlerFunc{
		"/api/rename": func(w http.ResponseWriter, r *http.Request) {
			gotFrom = r.URL.Query().Get("from")
			gotTo = r.URL.Query().Get("to")
			w.WriteHeader(http.StatusNoContent)
		},
	})
	defer srv.Close()

	c := newClient(srv.URL, "svc", "pw")
	if _, _, err := c.renameNote(context.Background(), &mcp.CallToolRequest{}, renameNoteArgs{From: "a.md", To: "b.md"}); err != nil {
		t.Fatalf("renameNote: %v", err)
	}
	if gotFrom != "a.md" || gotTo != "b.md" {
		t.Errorf("from/to query params did not round-trip, got from=%q to=%q", gotFrom, gotTo)
	}
}
