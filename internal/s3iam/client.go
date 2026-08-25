package s3iam

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client checks connectivity to an S3-compatible endpoint's STS
// AssumeRoleWithLDAPIdentity action.
type Client struct {
	cfg  Config
	http *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 5 * time.Second}}
}

// Credentials describes the temporary STS session issued by a successful
// AssumeRoleWithLDAPIdentity call — surfaced to the dashboard so an admin
// can see proof of a real, live session (which access key, when it
// expires), not so the caller can actually use it. The secret key and
// session token are deliberately never parsed here: nothing downstream of
// CheckConnection needs them, and an access key ID alone (no secret key)
// can't authenticate anything, so it's safe to show to any logged-in role.
type Credentials struct {
	AccessKeyID string
	Expiration  time.Time
}

type assumeRoleResponse struct {
	XMLName xml.Name `xml:"AssumeRoleWithLDAPIdentityResponse"`
	Result  struct {
		Credentials struct {
			AccessKeyId string `xml:"AccessKeyId"`
			Expiration  string `xml:"Expiration"`
		} `xml:"Credentials"`
	} `xml:"AssumeRoleWithLDAPIdentityResult"`
}

// CheckConnection reports whether the configured service account can
// successfully assume a role via the S3 endpoint's LDAP identity provider
// — i.e. that both the endpoint and the LDAP-backed IAM setup behind it
// are working, not just that the endpoint is up. Returns the resulting
// session's Credentials on success, or a nil *Credentials (no error) if the
// endpoint responded but didn't issue one (e.g. a misconfigured service
// account password) — mirroring the old connected=false case.
func (c *Client) CheckConnection(ctx context.Context) (*Credentials, error) {
	form := url.Values{
		"Action":          {"AssumeRoleWithLDAPIdentity"},
		"Version":         {"2011-06-15"},
		"LDAPUsername":    {c.cfg.LDAPUsername},
		"LDAPPassword":    {c.cfg.LDAPPassword},
		"DurationSeconds": {"900"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://%s/", c.cfg.Endpoint), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil // wrong credentials / IAM misconfiguration, not a connectivity error
	}

	var parsed assumeRoleResponse
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("s3iam: decode AssumeRoleWithLDAPIdentity response: %w", err)
	}
	if parsed.Result.Credentials.AccessKeyId == "" {
		return nil, nil
	}
	// Best-effort parse — an unparseable/missing expiration still means the
	// connection itself succeeded, so don't fail the whole check over it.
	expiration, _ := time.Parse(time.RFC3339, parsed.Result.Credentials.Expiration)
	return &Credentials{AccessKeyID: parsed.Result.Credentials.AccessKeyId, Expiration: expiration}, nil
}
