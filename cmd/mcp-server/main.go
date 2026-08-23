// Command mcp-server wraps VaultViewer's REST API as an MCP (Model Context
// Protocol) server, so AI agents (Claude Code, Claude Desktop, or any other
// MCP client) can query the vault's tree/search/notes/history/ontology
// graph as tools instead of calling the REST API directly.
//
// This binary does not reimplement storage or auth — it authenticates once
// against an already-running VaultViewer server as a single service
// account (env-configured, never hardcoded) and proxies each tool call to
// the matching REST endpoint. All tools are read-only; the service
// account's role (view is enough) is the effective permission ceiling for
// every agent connected through this server.
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

	baseURL := strings.TrimRight(envOr("VAULTVIEWER_URL", "http://localhost:8080"), "/")
	username := os.Getenv("VAULTVIEWER_USERNAME")
	password := os.Getenv("VAULTVIEWER_PASSWORD")
	if username == "" || password == "" {
		log.Fatal("VAULTVIEWER_USERNAME and VAULTVIEWER_PASSWORD are required")
	}

	c := newClient(baseURL, username, password)
	server := mcp.NewServer(&mcp.Implementation{Name: "vaultviewer", Version: "0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_ontology_graph",
		Description: "VaultViewer 볼트 전체의 온톨로지 그래프(노드 + 타입 있는 관계 엣지)를 " +
			"반환합니다. 인프라 컴포넌트 간 의존 관계나 하나가 죽었을 때 영향받는 범위를 물을 때 사용하세요.",
	}, c.getOntologyGraph)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_vault",
		Description: "VaultViewer 볼트 전체를 전문(全文) 검색해 일치하는 노트 경로와 문맥 스니펫을 반환합니다.",
	}, c.searchVault)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_note",
		Description: "VaultViewer의 특정 노트 원문(마크다운)을 경로로 읽습니다.",
	}, c.readNote)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tree",
		Description: "VaultViewer 볼트의 디렉토리 트리를 나열합니다. path를 비우면 루트 트리를 반환합니다.",
	}, c.listTree)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_note_history",
		Description: "특정 노트의 변경 이력(생성/수정/삭제/이름변경, 사용자, 타임스탬프)을 반환합니다.",
	}, c.getNoteHistory)

	ctx := context.Background()
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

// client is a thin HTTP client for VaultViewer's REST API, authenticating
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

// do issues an authenticated GET (or DELETE, but no tools in this binary
// need one) against VaultViewer's REST API. On a 401 — most likely the
// cached session token expired — it discards the cached token, logs in
// once more, and retries exactly once.
func (c *client) do(ctx context.Context, method, path string, query url.Values) ([]byte, error) {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	attempt := func(token string) (*http.Response, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, method, target, nil)
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
		return nil, fmt.Errorf("vaultviewer %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func rawTextResult(data []byte) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil, nil
}

type emptyArgs struct{}

func (c *client) getOntologyGraph(ctx context.Context, req *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/graph", nil)
	if err != nil {
		return nil, nil, err
	}
	return rawTextResult(data)
}

type searchArgs struct {
	Query string `json:"query" jsonschema:"검색어"`
}

func (c *client) searchVault(ctx context.Context, req *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, any, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/search", url.Values{"q": {args.Query}})
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
	data, err := c.do(ctx, http.MethodGet, "/api/file", url.Values{"path": {args.Path}})
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
	data, err := c.do(ctx, http.MethodGet, "/api/tree", url.Values{"path": {args.Path}})
	if err != nil {
		return nil, nil, err
	}
	return rawTextResult(data)
}

type historyArgs struct {
	Path string `json:"path" jsonschema:"볼트 기준 상대 경로"`
}

func (c *client) getNoteHistory(ctx context.Context, req *mcp.CallToolRequest, args historyArgs) (*mcp.CallToolResult, any, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/history", url.Values{"path": {args.Path}})
	if err != nil {
		return nil, nil, err
	}
	return rawTextResult(data)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
