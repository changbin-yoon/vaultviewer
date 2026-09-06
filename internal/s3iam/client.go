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

// SessionCredentials is the temporary STS session issued by a successful
// AssumeRoleWithLDAPIdentity call.
//
// Only the access key ID and expiry are parsed. They are surfaced to the
// dashboard as proof of a real, live session, and an access key ID with no
// secret authenticates nothing, so it is safe to show to any logged-in role.
//
// The secret key and session token are deliberately left on the floor. A
// permission prober briefly needed them — verifying what a user can do means
// calling as that user — but this session belongs to the fixed service
// account, not to whoever is logged in, so it could only ever have measured
// the wrong identity. Drift detection answers the same question by comparing
// the declaration against the backend's own record instead, and needs no
// user credentials at all.
type SessionCredentials struct {
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
// session on success, or nil (no error) if the
// endpoint responded but didn't issue one (e.g. a misconfigured service
// account password) — mirroring the old connected=false case.
func (c *Client) CheckConnection(ctx context.Context) (*SessionCredentials, error) {
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
	return &SessionCredentials{AccessKeyID: parsed.Result.Credentials.AccessKeyId, Expiration: expiration}, nil
}
