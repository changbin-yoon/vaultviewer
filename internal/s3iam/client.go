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

type assumeRoleResponse struct {
	XMLName xml.Name `xml:"AssumeRoleWithLDAPIdentityResponse"`
	Result  struct {
		Credentials struct {
			AccessKeyId string `xml:"AccessKeyId"`
		} `xml:"Credentials"`
	} `xml:"AssumeRoleWithLDAPIdentityResult"`
}

// CheckConnection reports whether the configured service account can
// successfully assume a role via the S3 endpoint's LDAP identity provider
// — i.e. that both the endpoint and the LDAP-backed IAM setup behind it
// are working, not just that the endpoint is up.
func (c *Client) CheckConnection(ctx context.Context) (bool, error) {
	form := url.Values{
		"Action":          {"AssumeRoleWithLDAPIdentity"},
		"Version":         {"2011-06-15"},
		"LDAPUsername":    {c.cfg.LDAPUsername},
		"LDAPPassword":    {c.cfg.LDAPPassword},
		"DurationSeconds": {"900"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://%s/", c.cfg.Endpoint), strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, nil // wrong credentials / IAM misconfiguration, not a connectivity error
	}

	var parsed assumeRoleResponse
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return false, fmt.Errorf("s3iam: decode AssumeRoleWithLDAPIdentity response: %w", err)
	}
	return parsed.Result.Credentials.AccessKeyId != "", nil
}
