package s3iam

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BucketAccess is one row of the dashboard's access table: what the user can
// do with one bucket, and which subject/policy pairs granted it. Bucket "*"
// means account-wide rather than a real bucket (an ARN of arn:aws:s3:::*,
// which is how admin actions are scoped).
type BucketAccess struct {
	Bucket       string       `json:"bucket"`
	Capabilities []Capability `json:"capabilities"`
	Via          []Source     `json:"via"`
}

// Access is everything AccessLens can say about one logged-in user's S3
// permissions from the mirrored policy set alone.
type Access struct {
	Buckets []BucketAccess `json:"buckets"`
	// Warnings describes policy content that could not be represented
	// faithfully (see PolicyDoc.fold). Surface these in the UI — each one
	// states which direction the display is wrong in.
	Warnings []string `json:"warnings,omitempty"`
}

// Catalog is the mirrored policy set: the same documents attached to groups
// in MinIO/AIStor, loaded from a directory of JSON files (in this repo,
// policy/generated, mounted as a ConfigMap in the cluster).
//
// It is a mirror, not a query. Nothing here talks to the S3 backend, so it
// cannot see policies attached directly to a user DN, a service account's
// narrowing session policy, or an out-of-band change made straight against
// MinIO. Digest and LoadedAt exist so the UI can date what it is showing
// rather than implying it is live.
type Catalog struct {
	policies map[string]PolicyDoc
	// Digest is a sha256 over the policy file names and contents — a stable
	// fingerprint of "which version of the policy set is this".
	Digest string
	// LoadedAt is when the directory was read.
	LoadedAt time.Time
}

// LoadCatalog reads every *.json file in dir as a policy document named
// after its file (bi-dev.json -> "bi-dev"). A directory with no policy files
// is an error, not an empty catalog: silently showing every user "권한 없음"
// because of a bad mount path is the worst possible failure for this
// feature.
func LoadCatalog(dir string) (*Catalog, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("s3iam: scan policy dir %q: %w", dir, err)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, fmt.Errorf("s3iam: no *.json policy documents in %q", dir)
	}

	catalog := &Catalog{policies: make(map[string]PolicyDoc, len(matches)), LoadedAt: time.Now()}
	digest := sha256.New()
	for _, path := range matches {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("s3iam: read policy %q: %w", path, err)
		}
		doc, err := ParsePolicy(b)
		if err != nil {
			return nil, fmt.Errorf("s3iam: policy %q: %w", filepath.Base(path), err)
		}
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		catalog.policies[name] = doc
		fmt.Fprintf(digest, "%s\n", name)
		digest.Write(b)
	}
	catalog.Digest = hex.EncodeToString(digest.Sum(nil))
	return catalog, nil
}

// Names lists the policies in the catalog, sorted.
func (c *Catalog) Names() []string {
	names := make([]string, 0, len(c.policies))
	for name := range c.policies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Resolve computes what a set of policy attachments grants, by unioning the
// capabilities of every attached policy.
//
// A plain union is correct only because the mirrored policy set is
// Allow-only — see PolicyDoc.fold. Sources naming a policy the catalog does
// not hold are reported as warnings, not skipped: the declaration and the
// policy documents are meant to be two halves of one configuration, so a
// name in one and not the other is a real misconfiguration rather than a
// group that simply isn't ours.
func (c *Catalog) Resolve(sources []Source) Access {
	buckets := map[string]map[Capability]bool{}
	bucketSources := map[string]map[Source]bool{}
	var warnings []string

	for _, src := range sources {
		doc, ok := c.policies[src.Policy]
		if !ok {
			warnings = append(warnings, fmt.Sprintf(
				"%s %s에 붙은 정책 %q의 문서가 없습니다 — 이 권한은 화면에 표시되지 않습니다.",
				subjectLabel(src.Kind), src.DN, src.Policy))
			continue
		}
		// Fold into a per-policy map first so each source is attributed to
		// only the buckets its own policy touched.
		granted := map[string]map[Capability]bool{}
		warnings = append(warnings, doc.fold(src.Policy, granted)...)

		for bucket, caps := range granted {
			if buckets[bucket] == nil {
				buckets[bucket] = map[Capability]bool{}
				bucketSources[bucket] = map[Source]bool{}
			}
			for c := range caps {
				buckets[bucket][c] = true
			}
			bucketSources[bucket][src] = true
		}
	}

	access := Access{Buckets: []BucketAccess{}, Warnings: warnings}
	for _, bucket := range sortedBucketNames(buckets) {
		access.Buckets = append(access.Buckets, BucketAccess{
			Bucket:       bucket,
			Capabilities: sortCapabilities(buckets[bucket]),
			Via:          sortedSources(bucketSources[bucket]),
		})
	}
	return access
}

func subjectLabel(kind SubjectKind) string {
	if kind == SubjectUser {
		return "사용자"
	}
	return "그룹"
}

// BucketNames lists just the bucket names the access covers, excluding the
// account-wide "*" pseudo-bucket. This is the list the dashboard card used
// to take from the operator-configured s3iam.bucketMap — deriving it from
// the policies removes that duplicated configuration.
func (a Access) BucketNames() []string {
	names := make([]string, 0, len(a.Buckets))
	for _, b := range a.Buckets {
		if b.Bucket == "*" {
			continue
		}
		names = append(names, b.Bucket)
	}
	return names
}

// Can reports whether the access includes a capability for a bucket.
func (a Access) Can(bucket string, capability Capability) bool {
	for _, b := range a.Buckets {
		if b.Bucket != bucket {
			continue
		}
		for _, c := range b.Capabilities {
			if c == capability {
				return true
			}
		}
	}
	return false
}

func sortedSources(set map[Source]bool) []Source {
	out := make([]Source, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DN != out[j].DN {
			return out[i].DN < out[j].DN
		}
		return out[i].Policy < out[j].Policy
	})
	return out
}
