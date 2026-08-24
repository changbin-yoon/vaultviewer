package trino

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckConnectionSuccess(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "svc" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v1/info" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(Config{
		Endpoint:           strings.TrimPrefix(srv.URL, "https://"),
		Username:           "svc",
		Password:           "secret",
		InsecureSkipVerify: true,
	})

	connected, err := c.CheckConnection(context.Background())
	if err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	if !connected {
		t.Errorf("expected connected=true")
	}
}

func TestCheckConnectionBadCredentials(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(Config{
		Endpoint:           strings.TrimPrefix(srv.URL, "https://"),
		Username:           "svc",
		Password:           "wrong",
		InsecureSkipVerify: true,
	})

	connected, err := c.CheckConnection(context.Background())
	if err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	if connected {
		t.Errorf("expected connected=false for a 401 response")
	}
}

func TestCheckConnectionUnreachable(t *testing.T) {
	c := NewClient(Config{Endpoint: "127.0.0.1:1", Username: "svc", Password: "secret"})
	if _, err := c.CheckConnection(context.Background()); err == nil {
		t.Fatalf("expected an error dialing a closed port")
	}
}
