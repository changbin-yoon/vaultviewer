// Command mcp-server wraps AccessLens's REST API as an MCP (Model Context
// Protocol) server, so AI agents (Claude Code, Claude Desktop, or any other
// MCP client) can query and edit the vault's tree/search/notes/history/
// ontology graph as tools instead of calling the REST API directly.
//
// This binary does not reimplement storage or auth — it authenticates once
// against an already-running AccessLens server as a single service account
// (env-configured, never hardcoded) and proxies each tool call to the
// matching REST endpoint. The service account's LDAP-resolved role is the
// effective permission ceiling for every agent connected through this
// server: the write tools (save_note/delete_note/rename_note) call the same
// RBAC-gated endpoints the web UI does and surface a REST 403 as a tool
// error verbatim — this binary performs no authorization of its own.
//
// One MCP server process authenticates as exactly one AccessLens account.
// Running one process per human/agent identity (rather than a single
// account shared across every caller) is what makes the audit trail
// (GET /api/audit) able to tell which agent made a given change — see the
// startup log line below, which prints the authenticated username so a
// misconfigured shared account is obvious immediately rather than
// discovered later in an audit review.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	transport := flag.String("transport", "stdio", "stdio | http")
	addr := flag.String("addr", ":8090", "listen address, only used with --transport http")
	flag.Parse()

	baseURL := strings.TrimRight(envOr("ACCESSLENS_URL", "http://localhost:8080"), "/")
	username := os.Getenv("ACCESSLENS_USERNAME")
	password := os.Getenv("ACCESSLENS_PASSWORD")
	if username == "" || password == "" {
		log.Fatal("ACCESSLENS_USERNAME and ACCESSLENS_PASSWORD are required")
	}

	ctx := context.Background()

	c := newClient(baseURL, username, password)
	// Log in eagerly (rather than lazily on the first tool call) so the
	// authenticated identity is visible in the startup log immediately —
	// see the package doc comment above on why that matters for the audit
	// trail. Fail fast on bad credentials rather than surfacing a
	// confusing error from an agent's first tool call instead.
	if _, err := c.login(ctx); err != nil {
		log.Fatalf("authenticate to accesslens as %q: %v", username, err)
	}
	log.Printf("authenticated to accesslens (%s) as %q", baseURL, username)

	server := mcp.NewServer(&mcp.Implementation{Name: "accesslens", Version: "0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_ontology_graph",
		Description: "AccessLens 볼트 전체의 온톨로지 그래프(노드 + 타입 있는 관계 엣지)를 " +
			"반환합니다. 인프라 컴포넌트 간 의존 관계나 하나가 죽었을 때 영향받는 범위를 물을 때 사용하세요.",
	}, c.getOntologyGraph)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_vault",
		Description: "AccessLens 볼트 전체를 전문(全文) 검색해 일치하는 노트 경로와 문맥 스니펫을 반환합니다.",
	}, c.searchVault)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_note",
		Description: "AccessLens의 특정 노트 원문(마크다운)을 경로로 읽습니다.",
	}, c.readNote)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tree",
		Description: "AccessLens 볼트의 디렉토리 트리를 나열합니다. path를 비우면 루트 트리를 반환합니다.",
	}, c.listTree)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_note_history",
		Description: "특정 노트의 변경 이력(생성/수정/삭제/이름변경, 사용자, 타임스탬프)을 반환합니다.",
	}, c.getNoteHistory)

	mcp.AddTool(server, &mcp.Tool{
		Name: "save_note",
		Description: "노트를 경로에 저장합니다 — 해당 경로에 노트가 없으면 새로 만들고, 있으면 " +
			"내용을 덮어씁니다(REST /api/file이 create/update를 구분하지 않는 것과 동일). " +
			"dev 이상 역할의 계정으로 연결된 서버에서만 성공하며, view 역할이면 403으로 실패합니다.",
	}, c.saveNote)

	mcp.AddTool(server, &mcp.Tool{
		Name: "delete_note",
		Description: "경로의 노트를 삭제합니다. adm 역할의 계정으로 연결된 서버에서만 성공하며, " +
			"dev/view 역할이면 403으로 실패합니다.",
	}, c.deleteNote)

	mcp.AddTool(server, &mcp.Tool{
		Name: "rename_note",
		Description: "노트 경로를 바꿉니다 — 같은 디렉토리 내에서만 가능합니다(다른 폴더로 옮기는 " +
			"것은 지원하지 않음). dev 이상 역할의 계정으로 연결된 서버에서만 성공합니다.",
	}, c.renameNote)

	switch *transport {
	case "stdio":
		if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
			log.Fatalf("mcp server (stdio) failed: %v", err)
		}
	case "http":
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
		log.Printf("mcp server listening on %s (streamable http)", *addr)
		if err := http.ListenAndServe(*addr, handler); err != nil {
			log.Fatalf("mcp server (http) failed: %v", err)
		}
	default:
		log.Fatalf("unknown --transport %q (expected \"stdio\" or \"http\")", *transport)
	}
}

// client is a thin HTTP client for AccessLens's REST API, authenticating
// as a single service account. The session token is cached in memory and
// transparently refreshed on a 401 (e.g. after the session TTL expires).
type client struct {
	baseURL  string
	username string
	password string
	http     *http.Client

	mu    sync.Mutex
	token string
}

func newClient(baseURL, username, password string) *client {
	return &client{
		baseURL:  baseURL,
		username: username,
		password: password,
		http:     &http.Client{},
	}
}

func (c *client) cachedToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token
}

func (c *client) setToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

func (c *client) login(ctx context.Context) (string, error) {
	body, err := json.Marshal(map[string]string{"username": c.username, "password": c.password})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("login to %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var creds struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}
	if creds.Token == "" {
		return "", fmt.Errorf("login response had no token")
	}
	c.setToken(creds.Token)
	return creds.Token, nil
}

func (c *client) tokenOrLogin(ctx context.Context) (string, error) {
	if tok := c.cachedToken(); tok != "" {
		return tok, nil
	}
	return c.login(ctx)
}

// do issues an authenticated request against AccessLens's REST API. body is
// nil for the read tools' GET calls; the write tools pass the raw note
// content, matching what /api/file's PUT handler expects (no JSON
// envelope). On a 401 — most likely the cached session token expired — it
// discards the cached token, logs in once more, and retries exactly once.
// body is buffered as a []byte (not a single-use io.Reader) precisely so
// that retry can replay it.
func (c *client) do(ctx context.Context, method, path string, query url.Values, body []byte) ([]byte, error) {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	attempt := func(token string) (*http.Response, []byte, error) {
		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, target, reqBody)
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, err
		}
		return resp, data, nil
	}

	token, err := c.tokenOrLogin(ctx)
	if err != nil {
		return nil, err
	}
	resp, data, err := attempt(token)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		c.setToken("")
		token, err = c.login(ctx)
		if err != nil {
			return nil, fmt.Errorf("re-login after 401: %w", err)
		}
		resp, data, err = attempt(token)
		if err != nil {
			return nil, err
		}
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("accesslens %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func rawTextResult(data []byte) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil, nil
}

type emptyArgs struct{}

func (c *client) getOntologyGraph(ctx context.Context, req *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/graph", nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return rawTextResult(data)
}

type searchArgs struct {
	Query string `json:"query" jsonschema:"검색어"`
}

func (c *client) searchVault(ctx context.Context, req *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, any, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/search", url.Values{"q": {args.Query}}, nil)
	if err != nil {
		return nil, nil, err
	}
	return rawTextResult(data)
}

type readNoteArgs struct {
	Path string `json:"path" jsonschema:"볼트 기준 상대 경로"`
}

// vaultFile mirrors internal/model.VaultFile. Content is []byte, which
// encoding/json base64-decodes automatically when unmarshaling a JSON
// string into it — so this struct alone gets us the plaintext note body
// without any manual base64 handling.
type vaultFile struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
}

func (c *client) readNote(ctx context.Context, req *mcp.CallToolRequest, args readNoteArgs) (*mcp.CallToolResult, any, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/file", url.Values{"path": {args.Path}}, nil)
	if err != nil {
		return nil, nil, err
	}
	var vf vaultFile
	if err := json.Unmarshal(data, &vf); err != nil {
		return nil, nil, fmt.Errorf("decode note %q: %w", args.Path, err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(vf.Content)}}}, nil, nil
}

type listTreeArgs struct {
	Path string `json:"path,omitempty" jsonschema:"볼트 기준 상대 디렉토리 경로, 비우면 루트"`
}

func (c *client) listTree(ctx context.Context, req *mcp.CallToolRequest, args listTreeArgs) (*mcp.CallToolResult, any, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/tree", url.Values{"path": {args.Path}}, nil)
	if err != nil {
		return nil, nil, err
	}
	return rawTextResult(data)
}

type historyArgs struct {
	Path string `json:"path" jsonschema:"볼트 기준 상대 경로"`
}

func (c *client) getNoteHistory(ctx context.Context, req *mcp.CallToolRequest, args historyArgs) (*mcp.CallToolResult, any, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/history", url.Values{"path": {args.Path}}, nil)
	if err != nil {
		return nil, nil, err
	}
	return rawTextResult(data)
}

type saveNoteArgs struct {
	Path    string `json:"path" jsonschema:"볼트 기준 상대 경로"`
	Content string `json:"content" jsonschema:"노트 본문(마크다운 원문). 그대로 저장되며 기존 내용을 완전히 대체합니다"`
	Reason  string `json:"reason,omitempty" jsonschema:"변경 사유 — 감사 로그(/api/audit)에 기록됩니다"`
}

// saveNote wraps PUT /api/file, which both creates and overwrites — REST
// draws no distinction between the two, so neither does this tool (see the
// package doc comment / this tool's description for why there's no
// separate create_note).
func (c *client) saveNote(ctx context.Context, req *mcp.CallToolRequest, args saveNoteArgs) (*mcp.CallToolResult, any, error) {
	q := url.Values{"path": {args.Path}}
	if args.Reason != "" {
		q.Set("reason", args.Reason)
	}
	if _, err := c.do(ctx, http.MethodPut, "/api/file", q, []byte(args.Content)); err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("saved %s", args.Path)}}}, nil, nil
}

type deleteNoteArgs struct {
	Path string `json:"path" jsonschema:"볼트 기준 상대 경로"`
}

func (c *client) deleteNote(ctx context.Context, req *mcp.CallToolRequest, args deleteNoteArgs) (*mcp.CallToolResult, any, error) {
	if _, err := c.do(ctx, http.MethodDelete, "/api/file", url.Values{"path": {args.Path}}, nil); err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("deleted %s", args.Path)}}}, nil, nil
}

type renameNoteArgs struct {
	From   string `json:"from" jsonschema:"현재 경로"`
	To     string `json:"to" jsonschema:"새 경로 — 같은 디렉토리 내에서만 가능합니다"`
	Reason string `json:"reason,omitempty" jsonschema:"변경 사유 — 감사 로그(/api/audit)에 기록됩니다"`
}

func (c *client) renameNote(ctx context.Context, req *mcp.CallToolRequest, args renameNoteArgs) (*mcp.CallToolResult, any, error) {
	q := url.Values{"from": {args.From}, "to": {args.To}}
	if args.Reason != "" {
		q.Set("reason", args.Reason)
	}
	if _, err := c.do(ctx, http.MethodPut, "/api/rename", q, nil); err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("renamed %s -> %s", args.From, args.To)}}}, nil, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
