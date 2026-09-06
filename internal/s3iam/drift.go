package s3iam

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/minio/minio-go/v7/pkg/signer"
)

// DriftKind classifies one disagreement between the declaration and what the
// S3 backend is actually enforcing.
type DriftKind string

const (
	// DriftUndeclared: the backend holds an attachment the declaration does
	// not. This is the dangerous direction — someone has permissions nobody
	// wrote down, and no amount of reading the declaration would reveal it.
	DriftUndeclared DriftKind = "undeclared"
	// DriftUnapplied: the declaration holds an attachment the backend does
	// not. The screen promises access the user does not actually have.
	DriftUnapplied DriftKind = "unapplied"
)

// DriftItem is one subject whose declared and actual policies differ.
type DriftItem struct {
	Kind DriftKind `json:"kind"`
	// DN is the LDAP subject, Declared and Actual the policy names each
	// side holds for it.
	DN       string   `json:"dn"`
	Declared []string `json:"declared"`
	Actual   []string `json:"actual"`
}

// DriftReport is the outcome of one comparison.
type DriftReport struct {
	CheckedAt time.Time   `json:"checkedAt"`
	Items     []DriftItem `json:"items"`
	// Subjects is how many LDAP subjects were compared, so "0 differences"
	// can be told apart from "nothing was compared".
	Subjects int `json:"subjects"`
}

// InSync reports whether the declaration and the backend agree.
func (r DriftReport) InSync() bool { return len(r.Items) == 0 }

// DriftChecker compares the attachment declaration against the S3 backend's
// own record of which policies are attached to which LDAP subjects.
//
// This is the question the mirror could never answer on its own: the
// declaration says what should be true, and only the backend knows what is.
// It needs no user credentials — just two read-only admin actions
// (admin:ListUsers, admin:GetPolicy), neither of which grants access to any
// object data.
type DriftChecker struct {
	cfg         Config
	attachments *Attachments
	catalog     *Catalog
	http        *http.Client
}

func NewDriftChecker(cfg Config, attachments *Attachments, catalog *Catalog) *DriftChecker {
	return &DriftChecker{
		cfg:         cfg,
		attachments: attachments,
		catalog:     catalog,
		http:        &http.Client{Timeout: 15 * time.Second},
	}
}

// policyEntitiesResponse is MinIO's reply to the LDAP policy-entities admin
// call, keyed by policy with the subjects each is attached to.
type policyEntitiesResponse struct {
	Timestamp      time.Time `json:"timestamp"`
	PolicyMappings []struct {
		Policy string   `json:"policy"`
		Users  []string `json:"users"`
		Groups []string `json:"groups"`
	} `json:"policyMappings"`
}

// adminGet performs a SigV4-signed GET against the MinIO admin API.
//
// The request is signed with minio-go's signer rather than pulling in
// madmin-go: minio-go is already a dependency, and the drift check needs
// exactly one endpoint.
func (d *DriftChecker) adminGet(ctx context.Context, path string) ([]byte, error) {
	url := fmt.Sprintf("http://%s%s", d.cfg.Endpoint, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// The admin API is signed as the "s3" service in us-east-1 regardless of
	// where the cluster actually is.
	signed := signer.SignV4(*req, d.cfg.AdminAccessKey, d.cfg.AdminSecretKey, "", "us-east-1")

	resp, err := d.http.Do(signed)
	if err != nil {
		return nil, fmt.Errorf("s3iam: admin request %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("s3iam: admin request %s denied — the configured account needs admin:ListUsers and admin:GetPolicy", path)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("s3iam: admin request %s: HTTP %d", path, resp.StatusCode)
	}
	// Capped: this endpoint returns one entry per policy, but a malformed
	// or hostile response should not be read into memory without bound.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("s3iam: read %s: %w", path, err)
	}
	return body, nil
}

// Check fetches the backend's attachments and diffs them against the
// declaration.
func (d *DriftChecker) Check(ctx context.Context) (*DriftReport, error) {
	raw, err := d.adminGet(ctx, "/minio/admin/v3/idp/ldap/policy-entities?")
	if err != nil {
		return nil, err
	}
	var parsed policyEntitiesResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("s3iam: decode policy entities: %w", err)
	}

	// Invert the backend's policy-keyed view into the subject-keyed one the
	// declaration uses, so the two can be compared directly.
	actual := map[string]map[string]bool{}
	for _, m := range parsed.PolicyMappings {
		for _, dn := range append(append([]string{}, m.Groups...), m.Users...) {
			key := normaliseDN(dn)
			if actual[key] == nil {
				actual[key] = map[string]bool{}
			}
			actual[key][m.Policy] = true
		}
	}

	declared := map[string]map[string]bool{}
	for dn, policies := range d.attachments.byGroup {
		declared[dn] = toSet(policies)
	}
	for dn, policies := range d.attachments.byUser {
		if declared[dn] == nil {
			declared[dn] = map[string]bool{}
		}
		for _, p := range policies {
			declared[dn][p] = true
		}
	}

	subjects := map[string]bool{}
	for dn := range declared {
		subjects[dn] = true
	}
	for dn := range actual {
		subjects[dn] = true
	}

	report := &DriftReport{CheckedAt: time.Now(), Items: []DriftItem{}, Subjects: len(subjects)}
	for _, dn := range sortedKeys(subjects) {
		dec, act := declared[dn], actual[dn]
		if setsEqual(dec, act) {
			continue
		}
		// An entry present only on the backend is reported as undeclared —
		// the direction that hides real permissions from anyone reading the
		// declaration. The reverse means the screen is promising access
		// that was never applied.
		kind := DriftUnapplied
		if len(dec) == 0 {
			kind = DriftUndeclared
		}
		report.Items = append(report.Items, DriftItem{
			Kind:     kind,
			DN:       dn,
			Declared: setToSorted(dec),
			Actual:   setToSorted(act),
		})
	}
	return report, nil
}

func toSet(list []string) map[string]bool {
	out := map[string]bool{}
	for _, v := range list {
		out[v] = true
	}
	return out
}

func setsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func setToSorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(set map[string]bool) []string { return setToSorted(set) }
