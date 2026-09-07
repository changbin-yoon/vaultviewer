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
		// The check must submit a statement, not call /v1/info: Trino
		// serves /v1/info without authentication, so a check against it
		// reports success for credentials that cannot log in.
		if r.Method != http.MethodPost || r.URL.Path != "/v1/statement" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"20260907_000000_00001_abcde"}`))
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

// The submitted statement must be cancelled — a health check should not
// leave work queued on the system it is checking.
func TestCheckConnectionCancelsItsQuery(t *testing.T) {
	var deleted string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/statement":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"q-123"}`))
		case r.Method == http.MethodDelete:
			deleted = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{
		Endpoint:           strings.TrimPrefix(srv.URL, "https://"),
		Username:           "svc",
		Password:           "secret",
		InsecureSkipVerify: true,
	})
	connected, err := c.CheckConnection(context.Background())
	if err != nil || !connected {
		t.Fatalf("CheckConnection: connected=%v err=%v", connected, err)
	}
	if deleted != "/v1/query/q-123" {
		t.Errorf("cancelled %q, want /v1/query/q-123", deleted)
	}
}

// A coordinator that is up but rejects the credentials must not read as
// connected — /v1/info would have said 200 here.
func TestCheckConnectionUnauthenticatedInfoEndpointIsNotUsed(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/info" {
			w.WriteHeader(http.StatusOK) // Trino really does this, unauthenticated
			return
		}
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
		t.Error("credentials that cannot authenticate must not report connected")
	}
}
