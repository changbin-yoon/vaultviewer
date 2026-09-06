package s3iam

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/minio/madmin-go/v3"
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
}

func NewDriftChecker(cfg Config, attachments *Attachments, catalog *Catalog) *DriftChecker {
	return &DriftChecker{cfg: cfg, attachments: attachments, catalog: catalog}
}

// client builds the admin client.
//
// madmin-go is used rather than signing the one endpoint by hand with
// minio-go's signer. Signing turned out to be only half the problem: MinIO
// encrypts admin API response bodies with the requester's secret key, so a
// correctly signed request still comes back as 974 bytes of binary labelled
// application/json. madmin handles both halves, and reimplementing the
// response encryption is well past the point where "just one endpoint"
// justified doing it manually.
func (d *DriftChecker) client() (*madmin.AdminClient, error) {
	c, err := madmin.New(d.cfg.Endpoint, d.cfg.AdminAccessKey, d.cfg.AdminSecretKey, false)
	if err != nil {
		return nil, fmt.Errorf("s3iam: admin client: %w", err)
	}
	return c, nil
}

// Check fetches the backend's attachments and diffs them against the
// declaration.
func (d *DriftChecker) Check(ctx context.Context) (*DriftReport, error) {
	client, err := d.client()
	if err != nil {
		return nil, err
	}
	parsed, err := client.GetLDAPPolicyEntities(ctx, madmin.PolicyEntitiesQuery{})
	if err != nil {
		// Name the two actions needed: MinIO has dozens of admin
		// permissions and "access denied" alone leaves an operator guessing.
		// The string check backs up the typed one because a bare 403 with no
		// MinIO error body parses into neither a code nor a message.
		if madmin.ToErrorResponse(err).Code == "AccessDenied" ||
			strings.Contains(strings.ToLower(err.Error()), "access denied") ||
			strings.Contains(err.Error(), "403") {
			return nil, fmt.Errorf("s3iam: 백엔드가 admin 조회를 거부했습니다 — 설정된 계정에 admin:ListUsers와 admin:GetPolicy가 필요합니다 (%w)", err)
		}
		return nil, fmt.Errorf("s3iam: read policy entities: %w", err)
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
