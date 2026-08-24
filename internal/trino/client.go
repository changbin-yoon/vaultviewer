package trino

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
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

// CheckConnection reports whether the coordinator is reachable and accepts
// the configured credentials, by calling its unauthenticated-but-gated
// /v1/info status endpoint over HTTPS with Basic Auth. It does not query
// any catalog/GRANT data.
func (c *Client) CheckConnection(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/v1/info", c.cfg.Endpoint), nil)
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(c.cfg.Username, c.cfg.Password)

	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}
