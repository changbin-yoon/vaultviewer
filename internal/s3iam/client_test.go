package s3iam

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const stsSuccessResponse = `<?xml version="1.0" encoding="UTF-8"?>
<AssumeRoleWithLDAPIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithLDAPIdentityResult>
    <Credentials>
      <AccessKeyId>TESTACCESSKEY</AccessKeyId>
      <SecretAccessKey>testsecret</SecretAccessKey>
    </Credentials>
  </AssumeRoleWithLDAPIdentityResult>
</AssumeRoleWithLDAPIdentityResponse>`

func TestCheckConnectionSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := url.ParseQuery(mustReadBody(r))
		if body.Get("Action") != "AssumeRoleWithLDAPIdentity" || body.Get("LDAPUsername") != "svc" || body.Get("LDAPPassword") != "secret" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(stsSuccessResponse))
	}))
	defer srv.Close()

	c := NewClient(Config{Endpoint: strings.TrimPrefix(srv.URL, "http://"), LDAPUsername: "svc", LDAPPassword: "secret"})
	connected, err := c.CheckConnection(context.Background())
	if err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	if !connected {
		t.Errorf("expected connected=true")
	}
}

func TestCheckConnectionBadCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewClient(Config{Endpoint: strings.TrimPrefix(srv.URL, "http://"), LDAPUsername: "svc", LDAPPassword: "wrong"})
	connected, err := c.CheckConnection(context.Background())
	if err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	if connected {
		t.Errorf("expected connected=false for a non-2xx response")
	}
}

func TestCheckConnectionUnreachable(t *testing.T) {
	c := NewClient(Config{Endpoint: "127.0.0.1:1", LDAPUsername: "svc", LDAPPassword: "secret"})
	if _, err := c.CheckConnection(context.Background()); err == nil {
		t.Fatalf("expected an error dialing a closed port")
	}
}

func mustReadBody(r *http.Request) string {
	buf, _ := io.ReadAll(r.Body)
	return string(buf)
}
