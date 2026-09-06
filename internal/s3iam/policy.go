package s3iam

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Capability is a human-facing summary of what a group of S3 actions lets
// someone actually do with a bucket. The dashboard shows these rather than
// raw action names: "쓰기 ✓" is readable, "s3:PutObject, s3:PutObjectTagging"
// is not. Folding many actions into one capability is lossy on purpose —
// anything that doesn't fold is reported through Access.Warnings instead of
// being silently dropped.
type Capability string

const (
	CapList           Capability = "list"
	CapRead           Capability = "read"
	CapLifecycleRead  Capability = "lifecycleRead"
	CapWrite          Capability = "write"
	CapLifecycleWrite Capability = "lifecycleWrite"
	CapDelete         Capability = "delete"
	CapBucketPolicy   Capability = "bucketPolicy"
	CapServiceAccount Capability = "serviceAccount"
)

// capabilityOrder is the display order — reads first, then writes, ordered
// least to most privileged, so a rendered row reads as a widening set of
// powers rather than a random list.
//
// Lifecycle is split into read and write rather than folded into one
// "lifecycle" capability because the two sit in different tiers: dev can
// read a bucket's ILM rules, only adm can set them. Folding them would make
// the dashboard tell a dev they have a power they don't.
var capabilityOrder = []Capability{
	CapList, CapRead, CapLifecycleRead, CapWrite, CapLifecycleWrite,
	CapDelete, CapBucketPolicy, CapServiceAccount,
}

// actionCapabilities maps every action used by policy/gen_policies.py to the
// capability it grants. An action that isn't listed here is reported as a
// warning rather than ignored: an unrecognised action means the mirrored
// policy set has outgrown this table, and under-reporting someone's access
// is exactly the failure this feature exists to prevent.
var actionCapabilities = map[string]Capability{
	"s3:ListBucket":                CapList,
	"s3:GetBucketLocation":         CapList,
	"s3:GetObject":                 CapRead,
	"s3:GetObjectTagging":          CapRead,
	"s3:PutObject":                 CapWrite,
	"s3:PutObjectTagging":          CapWrite,
	"s3:GetLifecycleConfiguration": CapLifecycleRead,
	"s3:PutLifecycleConfiguration": CapLifecycleWrite,
	"s3:DeleteObject":              CapDelete,
	// Both bucket-policy actions stay folded into one capability: unlike
	// lifecycle, they live in the same (adm) tier, so no tier can hold one
	// without the other and the fold can't misrepresent anyone.
	"s3:GetBucketPolicy":         CapBucketPolicy,
	"s3:PutBucketPolicy":         CapBucketPolicy,
	"admin:CreateServiceAccount": CapServiceAccount,
}

// arnPrefix is the S3 ARN namespace every Resource in a MinIO bucket policy
// starts with. MinIO keeps AWS's "arn:aws:s3:::" spelling.
const arnPrefix = "arn:aws:s3:::"

// stringList decodes an IAM JSON field that is allowed to be either a bare
// string or an array of strings — "Action": "s3:GetObject" and
// "Action": ["s3:GetObject"] are both legal and both appear in the wild.
// Without this, half of a real policy set fails to parse.
type stringList []string

func (s *stringList) UnmarshalJSON(b []byte) error {
	var single string
	if err := json.Unmarshal(b, &single); err == nil {
		*s = stringList{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("s3iam: Action/Resource must be a string or an array of strings, got %s", string(b))
	}
	*s = many
	return nil
}

// Statement is one statement of an IAM policy document. Condition,
// NotAction and NotResource are decoded only so their presence can be
// detected and warned about — none of them are evaluated (see fold).
type Statement struct {
	SID         string                     `json:"Sid,omitempty"`
	Effect      string                     `json:"Effect"`
	Action      stringList                 `json:"Action"`
	Resource    stringList                 `json:"Resource"`
	NotAction   stringList                 `json:"NotAction,omitempty"`
	NotResource stringList                 `json:"NotResource,omitempty"`
	Condition   map[string]json.RawMessage `json:"Condition,omitempty"`
}

// PolicyDoc is a parsed IAM policy document — the same JSON that is attached
// to a group in MinIO/AIStor, mirrored here so AccessLens can describe a
// user's access without holding admin credentials against the S3 backend.
type PolicyDoc struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

// ParsePolicy decodes one policy document.
func ParsePolicy(b []byte) (PolicyDoc, error) {
	var doc PolicyDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return PolicyDoc{}, fmt.Errorf("s3iam: parse policy: %w", err)
	}
	return doc, nil
}

// bucketFromARN extracts the bucket name an S3 ARN refers to, collapsing the
// object-level and bucket-level forms onto the same bucket:
//
//	arn:aws:s3:::team-bi     -> "team-bi"
//	arn:aws:s3:::team-bi/*   -> "team-bi"
//	arn:aws:s3:::*           -> "*"   (account-wide, e.g. admin actions)
//
// Reports false for an ARN outside the S3 namespace, which the caller turns
// into a warning rather than silently skipping.
func bucketFromARN(arn string) (string, bool) {
	if !strings.HasPrefix(arn, arnPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(arn, arnPrefix)
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" {
		return "", false
	}
	return rest, true
}

// capabilitiesForAction resolves one policy Action to the capabilities it
// grants, expanding a trailing "*" as a prefix match (e.g. "s3:Get*" covers
// every known s3:Get… action, "s3:*" every s3 action). An action matching
// nothing known returns an empty slice, which fold turns into a warning.
func capabilitiesForAction(action string) []Capability {
	if exact, ok := actionCapabilities[action]; ok {
		return []Capability{exact}
	}
	if !strings.HasSuffix(action, "*") {
		return nil
	}
	prefix := strings.TrimSuffix(action, "*")
	seen := map[Capability]bool{}
	for known, capability := range actionCapabilities {
		if strings.HasPrefix(known, prefix) {
			seen[capability] = true
		}
	}
	return sortCapabilities(seen)
}

// sortCapabilities returns the set in canonical display order.
func sortCapabilities(set map[Capability]bool) []Capability {
	out := make([]Capability, 0, len(set))
	for _, c := range capabilityOrder {
		if set[c] {
			out = append(out, c)
		}
	}
	return out
}

// fold accumulates the policy's Allow grants into buckets, a
// bucket -> capability set map owned by the caller, and returns any
// warnings about content it could not faithfully represent.
//
// The mirrored policy set is Allow-only by construction (see the design
// constraints at the top of policy/gen_policies.py): lower tiers are
// restricted by the *absence* of an Allow, never by an explicit Deny. That
// is what makes a plain union of capabilities correct here, with no policy
// evaluator. Anything violating that assumption — a Deny, a NotAction, a
// Condition — is therefore not something to quietly approximate: the
// statement is reported and skipped, and the warning says which direction
// the resulting display is wrong in, so the UI can say so too.
func (p PolicyDoc) fold(name string, buckets map[string]map[Capability]bool) []string {
	var warnings []string
	for i, st := range p.Statement {
		where := fmt.Sprintf("정책 %s의 %s", name, statementLabel(st, i))

		if !strings.EqualFold(st.Effect, "Allow") {
			warnings = append(warnings, fmt.Sprintf(
				"%s는 Effect가 %q입니다 — 이 화면은 Allow만 계산하므로 실제보다 넓게 표시될 수 있습니다.", where, st.Effect))
			continue
		}
		if len(st.NotAction) > 0 || len(st.NotResource) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"%s는 NotAction/NotResource를 씁니다 — MinIO가 지원하지 않는 형식이라 계산에서 제외했습니다.", where))
			continue
		}
		if len(st.Condition) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"%s에는 Condition이 붙어 있습니다 — 조건은 평가하지 않으므로 조건이 걸리는 상황에서는 실제보다 넓게 표시됩니다.", where))
			// Applied anyway: skipping would under-report access that is
			// granted whenever the condition holds, and under-reporting is
			// the more dangerous of the two errors here.
		}

		granted := map[Capability]bool{}
		for _, action := range st.Action {
			caps := capabilitiesForAction(action)
			if len(caps) == 0 {
				warnings = append(warnings, fmt.Sprintf(
					"%s의 액션 %q를 해석하지 못했습니다 — 이 권한은 화면에 표시되지 않습니다.", where, action))
				continue
			}
			for _, c := range caps {
				granted[c] = true
			}
		}
		if len(granted) == 0 {
			continue
		}
		for _, resource := range st.Resource {
			bucket, ok := bucketFromARN(resource)
			if !ok {
				warnings = append(warnings, fmt.Sprintf(
					"%s의 리소스 %q에서 버킷 이름을 읽지 못했습니다 — 이 권한은 화면에 표시되지 않습니다.", where, resource))
				continue
			}
			if buckets[bucket] == nil {
				buckets[bucket] = map[Capability]bool{}
			}
			for c := range granted {
				buckets[bucket][c] = true
			}
		}
	}
	return warnings
}

// statementLabel names a statement for a warning message — by its Sid when
// it has one, otherwise by position.
func statementLabel(st Statement, i int) string {
	if st.SID != "" {
		return fmt.Sprintf("statement %q", st.SID)
	}
	return fmt.Sprintf("%d번째 statement", i+1)
}

// sortedBucketNames returns bucket keys in display order, with the
// account-wide "*" pseudo-bucket last since it isn't a real bucket.
func sortedBucketNames(buckets map[string]map[Capability]bool) []string {
	names := make([]string, 0, len(buckets))
	for name := range buckets {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "*") != (names[j] == "*") {
			return names[j] == "*"
		}
		return names[i] < names[j]
	})
	return names
}
