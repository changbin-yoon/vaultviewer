package trino

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client checks connectivity to a Trino coordinator.
type Client struct {
	cfg  Config
	http *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify}, //nolint:gosec // opt-in, test/internal coordinators only
			},
		},
	}
}

// CheckConnection reports whether the coordinator is reachable **and
// actually accepts the configured credentials**.
//
// It submits a trivial statement rather than calling /v1/info. /v1/info was
// the obvious choice and is wrong: Trino serves it without authentication,
// so it answers 200 for a completely bogus password (measured 2026-09-07) —
// the card would show "연결됨" for credentials that cannot log in.
// POST /v1/statement is gated: 401 on bad credentials, 200 on good.
//
// "SELECT 1" touches no catalog, so this says nothing about whether the
// account is *authorised* to query anything — in this deployment Trino
// delegates authorisation to OPA, which can deny every statement while
// authentication still succeeds. Connectivity and permission are separate
// claims and this only makes the first.
func (c *Client) CheckConnection(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("https://%s/v1/statement", c.cfg.Endpoint), strings.NewReader("SELECT 1"))
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	// Trino attributes the query to this user in its own UI/logs; without it
	// the coordinator rejects the submission outright.
	req.Header.Set("X-Trino-User", c.cfg.Username)

	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, nil
	}

	// The statement is now queued on the coordinator. Cancel it rather than
	// leaving it for the query timeout to reap — a health check should not
	// accumulate work on the system it is checking.
	var submitted struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&submitted); err == nil && submitted.ID != "" {
		c.cancel(ctx, submitted.ID)
	}
	return true, nil
}

// cancel best-effort deletes a submitted query. A failure here does not
// change the connectivity answer that was already established.
func (c *Client) cancel(ctx context.Context, id string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("https://%s/v1/query/%s", c.cfg.Endpoint, id), nil)
	if err != nil {
		return
	}
	req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	req.Header.Set("X-Trino-User", c.cfg.Username)
	if resp, err := c.http.Do(req); err == nil {
		resp.Body.Close()
	}
}
