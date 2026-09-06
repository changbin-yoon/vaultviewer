package s3iam

import (
	"strings"
	"testing"
)

func TestParsePolicyAcceptsStringOrArrayFields(t *testing.T) {
	// "Action": "s3:GetObject" and "Action": ["s3:GetObject"] are both legal
	// IAM JSON; a parser that only handles the array form drops real grants.
	single, err := ParsePolicy([]byte(`{"Version":"2012-10-17","Statement":[
		{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::team-bi/*"}]}`))
	if err != nil {
		t.Fatalf("parse scalar form: %v", err)
	}
	array, err := ParsePolicy([]byte(`{"Version":"2012-10-17","Statement":[
		{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::team-bi/*"]}]}`))
	if err != nil {
		t.Fatalf("parse array form: %v", err)
	}
	for _, doc := range []PolicyDoc{single, array} {
		if got := doc.Statement[0].Action; len(got) != 1 || got[0] != "s3:GetObject" {
			t.Errorf("Action = %v, want [s3:GetObject]", got)
		}
		if got := doc.Statement[0].Resource; len(got) != 1 || got[0] != "arn:aws:s3:::team-bi/*" {
			t.Errorf("Resource = %v, want [arn:aws:s3:::team-bi/*]", got)
		}
	}
}

func TestParsePolicyRejectsMalformedActionField(t *testing.T) {
	_, err := ParsePolicy([]byte(`{"Statement":[{"Effect":"Allow","Action":{"bad":1}}]}`))
	if err == nil {
		t.Fatal("expected an error for an object-valued Action")
	}
}

func TestBucketFromARN(t *testing.T) {
	tests := []struct {
		arn        string
		wantBucket string
		wantOK     bool
	}{
		{"arn:aws:s3:::team-bi", "team-bi", true},
		{"arn:aws:s3:::team-bi/*", "team-bi", true},
		{"arn:aws:s3:::team-bi/raw/2026/a.parquet", "team-bi", true},
		{"arn:aws:s3:::*", "*", true},
		{"arn:aws:s3:::", "", false},
		{"arn:aws:iam::123:role/x", "", false},
		{"team-bi", "", false},
	}
	for _, tt := range tests {
		bucket, ok := bucketFromARN(tt.arn)
		if bucket != tt.wantBucket || ok != tt.wantOK {
			t.Errorf("bucketFromARN(%q) = (%q, %v), want (%q, %v)", tt.arn, bucket, ok, tt.wantBucket, tt.wantOK)
		}
	}
}

func TestCapabilitiesForAction(t *testing.T) {
	tests := []struct {
		action string
		want   []Capability
	}{
		{"s3:GetObject", []Capability{CapRead}},
		{"s3:DeleteObject", []Capability{CapDelete}},
		{"admin:CreateServiceAccount", []Capability{CapServiceAccount}},
		// A trailing "*" is a prefix match over every known action.
		{"s3:PutObject*", []Capability{CapWrite}},
		{"s3:Delete*", []Capability{CapDelete}},
		// Get/Put of the same resource now land in different capabilities.
		{"s3:GetLifecycle*", []Capability{CapLifecycleRead}},
		{"s3:PutLifecycle*", []Capability{CapLifecycleWrite}},
		{"s3:*", []Capability{CapList, CapRead, CapLifecycleRead, CapWrite, CapLifecycleWrite, CapDelete, CapBucketPolicy}},
		{"s3:Unheardof", nil},
		{"lambda:InvokeFunction", nil},
	}
	for _, tt := range tests {
		got := capabilitiesForAction(tt.action)
		if len(got) != len(tt.want) {
			t.Errorf("capabilitiesForAction(%q) = %v, want %v", tt.action, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("capabilitiesForAction(%q) = %v, want %v", tt.action, got, tt.want)
				break
			}
		}
	}
}

// foldOnce is a test helper: folds one document and returns the capability
// set per bucket plus the warnings.
func foldOnce(t *testing.T, name, raw string) (map[string]map[Capability]bool, []string) {
	t.Helper()
	doc, err := ParsePolicy([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	buckets := map[string]map[Capability]bool{}
	return buckets, doc.fold(name, buckets)
}

func TestFoldSkipsDenyAndWarnsItMayOverReport(t *testing.T) {
	buckets, warnings := foldOnce(t, "p", `{"Statement":[
		{"Effect":"Deny","Action":"s3:GetObject","Resource":"arn:aws:s3:::team-bi/*"}]}`)
	if len(buckets) != 0 {
		t.Errorf("a Deny statement must not grant anything, got %v", buckets)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "넓게") {
		t.Errorf("expected an over-reporting warning, got %v", warnings)
	}
}

func TestFoldSkipsNotActionWithWarning(t *testing.T) {
	buckets, warnings := foldOnce(t, "p", `{"Statement":[
		{"Effect":"Allow","NotAction":"s3:DeleteObject","Resource":"arn:aws:s3:::team-bi/*"}]}`)
	if len(buckets) != 0 {
		t.Errorf("NotAction must not be interpreted, got %v", buckets)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "NotAction") {
		t.Errorf("expected a NotAction warning, got %v", warnings)
	}
}

func TestFoldAppliesConditionalStatementButWarns(t *testing.T) {
	// Under-reporting access is the more dangerous error, so a conditional
	// grant is counted — and flagged.
	buckets, warnings := foldOnce(t, "p", `{"Statement":[{"Effect":"Allow","Action":"s3:GetObject",
		"Resource":"arn:aws:s3:::team-bi/*","Condition":{"StringLike":{"s3:prefix":"raw/*"}}}]}`)
	if !buckets["team-bi"][CapRead] {
		t.Errorf("conditional Allow should still be counted, got %v", buckets)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "Condition") {
		t.Errorf("expected a Condition warning, got %v", warnings)
	}
}

func TestFoldWarnsOnUnrecognisedActionAndResource(t *testing.T) {
	_, warnings := foldOnce(t, "p", `{"Statement":[
		{"Effect":"Allow","Action":"s3:MakeCoffee","Resource":"arn:aws:s3:::team-bi/*"},
		{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:iam::1:role/x"}]}`)
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings (unknown action, unreadable resource), got %v", warnings)
	}
	if !strings.Contains(warnings[0], "s3:MakeCoffee") {
		t.Errorf("first warning should name the unknown action, got %q", warnings[0])
	}
	if !strings.Contains(warnings[1], "arn:aws:iam::1:role/x") {
		t.Errorf("second warning should name the unreadable resource, got %q", warnings[1])
	}
}

func TestFoldUnionsObjectAndBucketARNsOntoOneBucket(t *testing.T) {
	buckets, warnings := foldOnce(t, "p", `{"Statement":[
		{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::team-bi/*"]},
		{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::team-bi"]}]}`)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(buckets) != 1 {
		t.Fatalf("object- and bucket-level ARNs should collapse to one bucket, got %v", buckets)
	}
	if !buckets["team-bi"][CapRead] || !buckets["team-bi"][CapList] {
		t.Errorf("team-bi = %v, want read+list", buckets["team-bi"])
	}
}
