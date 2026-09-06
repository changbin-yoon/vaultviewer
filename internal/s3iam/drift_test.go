package s3iam

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/minio/madmin-go/v3"
)

// MinIO requires credentials of a plausible length; madmin rejects shorter
// ones before a request is ever made.
const (
	driftTestAccess = "driftTESTaccessKEY01"
	driftTestSecret = "driftTESTsecretKEYdriftTESTsecretKEY0102"
)

// driftFixture builds a checker whose declaration is the given YAML and whose
// backend is a stub serving the given policy-entities response.
func driftFixture(t *testing.T, declaration string, entities map[string]any) (*DriftChecker, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.yaml")
	if err := os.WriteFile(path, []byte(declaration), 0o600); err != nil {
		t.Fatal(err)
	}
	attachments, err := LoadAttachments(path)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/minio/admin/v3/idp/ldap/policy-entities") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// The request must carry a SigV4 signature; the stub does not verify
		// it, but its absence would mean the client never signed at all.
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// MinIO encrypts admin API response bodies with the requester's
		// secret key, so the stub has to as well — otherwise the test would
		// pass against a client that never learned to decrypt.
		body, err := json.Marshal(entities)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		enc, err := madmin.EncryptData(driftTestSecret, body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(enc)
	}))
	cfg := Config{
		Endpoint:       strings.TrimPrefix(srv.URL, "http://"),
		AdminAccessKey: driftTestAccess,
		AdminSecretKey: driftTestSecret,
	}
	return NewDriftChecker(cfg, attachments, nil), srv.Close
}

func mappings(entries ...map[string]any) map[string]any {
	return map[string]any{"policyMappings": entries}
}

func mapping(policy string, groups ...string) map[string]any {
	return map[string]any{"policy": policy, "groups": groups, "users": nil}
}

const oneDeclared = `
attachments:
  - policy: bi-dev
    groups: ["cn=bi-dev,ou=groups,dc=example,dc=com"]
`

func TestDriftInSyncWhenDeclarationMatchesBackend(t *testing.T) {
	d, done := driftFixture(t, oneDeclared, mappings(mapping("bi-dev", "cn=bi-dev,ou=groups,dc=example,dc=com")))
	defer done()
	report, err := d.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.InSync() {
		t.Errorf("expected no drift, got %+v", report.Items)
	}
	// "0 differences" must be distinguishable from "nothing was compared".
	if report.Subjects != 1 {
		t.Errorf("Subjects = %d, want 1", report.Subjects)
	}
}

func TestDriftDetectsUndeclaredAttachment(t *testing.T) {
	// The dangerous direction: the backend grants something nobody wrote
	// down, so reading the declaration alone would never reveal it. This is
	// the shape of the real finding on cluster-mesh2 (cn=view -> readonly).
	d, done := driftFixture(t, oneDeclared, mappings(
		mapping("bi-dev", "cn=bi-dev,ou=groups,dc=example,dc=com"),
		mapping("readonly", "cn=view,ou=groups,dc=example,dc=com"),
	))
	defer done()
	report, err := d.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("expected one drift item, got %+v", report.Items)
	}
	item := report.Items[0]
	if item.Kind != DriftUndeclared {
		t.Errorf("Kind = %q, want %q", item.Kind, DriftUndeclared)
	}
	if item.DN != "cn=view,ou=groups,dc=example,dc=com" {
		t.Errorf("DN = %q", item.DN)
	}
	if len(item.Declared) != 0 || len(item.Actual) != 1 || item.Actual[0] != "readonly" {
		t.Errorf("Declared=%v Actual=%v", item.Declared, item.Actual)
	}
}

func TestDriftDetectsUnappliedAttachment(t *testing.T) {
	// The other direction: the screen promises access that was never applied.
	d, done := driftFixture(t, oneDeclared, mappings())
	defer done()
	report, err := d.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 || report.Items[0].Kind != DriftUnapplied {
		t.Fatalf("expected one unapplied item, got %+v", report.Items)
	}
}

func TestDriftDetectsPartialPolicySetOnSameSubject(t *testing.T) {
	// A subject present on both sides but holding different policies is
	// drift too — the earlier naming-convention model could not even
	// represent a subject with more than one policy.
	decl := `
attachments:
  - policy: bi-dev
    groups: ["cn=team,ou=groups,dc=example,dc=com"]
  - policy: ml-view
    groups: ["cn=team,ou=groups,dc=example,dc=com"]
`
	d, done := driftFixture(t, decl, mappings(mapping("bi-dev", "cn=team,ou=groups,dc=example,dc=com")))
	defer done()
	report, err := d.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("expected one item, got %+v", report.Items)
	}
	if got := report.Items[0]; len(got.Declared) != 2 || len(got.Actual) != 1 {
		t.Errorf("Declared=%v Actual=%v, want both sides reported", got.Declared, got.Actual)
	}
}

func TestDriftNormalisesDNSpelling(t *testing.T) {
	// MinIO and the directory disagree on case and spacing; a DN that fails
	// to match would be reported as drift on both sides at once.
	d, done := driftFixture(t, oneDeclared,
		mappings(mapping("bi-dev", "CN=Bi-Dev, OU=Groups, DC=Example, DC=Com")))
	defer done()
	report, err := d.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.InSync() {
		t.Errorf("DN spelling differences must not read as drift, got %+v", report.Items)
	}
}

func TestDriftReportsDeniedAdminCredentialsClearly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A real MinIO denial carries an error document; the client has to
		// recognise it whether or not one is present.
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<Error><Code>AccessDenied</Code><Message>Access Denied.</Message></Error>`))
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "a.yaml")
	if err := os.WriteFile(path, []byte(oneDeclared), 0o600); err != nil {
		t.Fatal(err)
	}
	attachments, err := LoadAttachments(path)
	if err != nil {
		t.Fatal(err)
	}
	d := NewDriftChecker(Config{
		Endpoint:       strings.TrimPrefix(srv.URL, "http://"),
		AdminAccessKey: driftTestAccess, AdminSecretKey: driftTestSecret,
	}, attachments, nil)

	_, err = d.Check(context.Background())
	if err == nil {
		t.Fatal("expected an error when the admin call is denied")
	}
	// The message must name the two actions needed, or an operator has to
	// guess which of MinIO's many admin permissions is missing.
	if !strings.Contains(err.Error(), "admin:ListUsers") || !strings.Contains(err.Error(), "admin:GetPolicy") {
		t.Errorf("error should name the required admin actions, got %q", err)
	}
}
