package s3iam

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

// Attachment declares that a policy is attached to a set of LDAP subjects,
// mirroring exactly the shape MinIO/AIStor reports for its own state:
//
//	{"policy": "bi-dev", "users": null, "groups": ["cn=bi-dev,ou=groups,dc=example,dc=com"]}
//
// Keeping the same shape is the point. Earlier this app inferred attachment
// from a group-name convention ("a bi-dev group must hold the bi-dev
// policy"), which happened to be right only because someone had lined the
// names up by hand. It could not express a group holding two policies, a
// policy attached to several groups, a policy attached straight to a user
// DN, or any group that broke the naming pattern — and it had no way to be
// compared against what MinIO actually stores. A declaration can be.
type Attachment struct {
	Policy string   `json:"policy"`
	Groups []string `json:"groups,omitempty"`
	Users  []string `json:"users,omitempty"`
}

// attachmentFile is the on-disk document.
type attachmentFile struct {
	Attachments []Attachment `json:"attachments"`
}

// Attachments is the loaded declaration, indexed for lookup by subject.
//
// It is a copy of what the S3 backend had attached when it was last synced
// (see examples/ldap-verify/sync-from-minio.sh), not a live read. AccessLens
// explains permissions; it never changes them. Keeping the copy in a file is
// what makes the drift check possible — a copy can be diffed against the
// backend, a naming convention cannot.
type Attachments struct {
	byGroup map[string][]string // normalised group DN -> policy names
	byUser  map[string][]string // normalised user DN  -> policy names
	// Digest fingerprints the declaration, LoadedAt records when it was
	// read — both surfaced so the UI can date what it shows.
	Digest   string
	LoadedAt time.Time
	// Count is the number of declared attachment entries.
	Count int
}

// SubjectKind distinguishes an attachment reached through group membership
// from one attached directly to the user's own DN.
type SubjectKind string

const (
	SubjectGroup SubjectKind = "group"
	SubjectUser  SubjectKind = "user"
)

// Source records which subject, through which policy, contributed a grant —
// so the UI can answer "왜 내가 이 버킷에 쓸 수 있지?" with the actual DN
// rather than an unattributable checkmark.
type Source struct {
	Kind   SubjectKind `json:"kind"`
	DN     string      `json:"dn"`
	Policy string      `json:"policy"`
}

// normaliseDN puts a DN in a comparable form: lowercase, with the optional
// space after each comma removed. Directories and MinIO disagree on both,
// and a DN that fails to match is indistinguishable from "no permissions" —
// the failure mode this whole feature exists to prevent.
func normaliseDN(dn string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(dn)), ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return strings.Join(parts, ",")
}

// LoadAttachments reads the attachment declaration from a YAML file.
//
// A declaration with no entries is an error, not an empty result: the most
// likely cause is a wrong path or an unmounted ConfigMap, and the resulting
// screen ("you have access to nothing") is indistinguishable from a true
// answer.
func LoadAttachments(path string) (*Attachments, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("s3iam: read attachments %q: %w", path, err)
	}
	var doc attachmentFile
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("s3iam: parse attachments %q: %w", path, err)
	}
	if len(doc.Attachments) == 0 {
		return nil, fmt.Errorf("s3iam: no attachments declared in %q", path)
	}

	a := &Attachments{
		byGroup:  map[string][]string{},
		byUser:   map[string][]string{},
		LoadedAt: time.Now(),
		Count:    len(doc.Attachments),
	}
	for _, entry := range doc.Attachments {
		if entry.Policy == "" {
			return nil, fmt.Errorf("s3iam: attachment with no policy name in %q", path)
		}
		for _, dn := range entry.Groups {
			key := normaliseDN(dn)
			a.byGroup[key] = appendUnique(a.byGroup[key], entry.Policy)
		}
		for _, dn := range entry.Users {
			key := normaliseDN(dn)
			a.byUser[key] = appendUnique(a.byUser[key], entry.Policy)
		}
	}
	sum := sha256.Sum256(raw)
	a.Digest = hex.EncodeToString(sum[:])
	return a, nil
}

// Resolve returns every policy attached to the user, whether through one of
// their groups or straight to their own DN, with the subject that granted
// it. Sorted for a stable UI order.
func (a *Attachments) Resolve(userDN string, groupDNs []string) []Source {
	var sources []Source
	seen := map[Source]bool{}

	add := func(kind SubjectKind, dn string, policies []string) {
		for _, p := range policies {
			s := Source{Kind: kind, DN: dn, Policy: p}
			if !seen[s] {
				seen[s] = true
				sources = append(sources, s)
			}
		}
	}
	if userDN != "" {
		add(SubjectUser, userDN, a.byUser[normaliseDN(userDN)])
	}
	for _, dn := range groupDNs {
		add(SubjectGroup, dn, a.byGroup[normaliseDN(dn)])
	}

	sort.Slice(sources, func(i, j int) bool {
		if sources[i].DN != sources[j].DN {
			return sources[i].DN < sources[j].DN
		}
		return sources[i].Policy < sources[j].Policy
	})
	return sources
}

// PolicyNames lists every policy named anywhere in the declaration, sorted.
// Used to check the declaration against the loaded policy documents.
func (a *Attachments) PolicyNames() []string {
	set := map[string]bool{}
	for _, policies := range a.byGroup {
		for _, p := range policies {
			set[p] = true
		}
	}
	for _, policies := range a.byUser {
		for _, p := range policies {
			set[p] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func appendUnique(list []string, v string) []string {
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}
