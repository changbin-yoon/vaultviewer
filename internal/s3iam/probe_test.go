package s3iam

import (
	"errors"
	"testing"

	"github.com/minio/minio-go/v7"
)

func TestProbeConfigWriteEnabled(t *testing.T) {
	// The write probe stays off without a cleanup account, however it is
	// configured: it would otherwise leave an object behind in exactly the
	// case it is most useful (a tier with write but no delete).
	tests := []struct {
		name string
		cfg  ProbeConfig
		want bool
	}{
		{"모두 설정", ProbeConfig{Write: true, MasterAccessKey: "a", MasterSecretKey: "s"}, true},
		{"Write 꺼짐", ProbeConfig{MasterAccessKey: "a", MasterSecretKey: "s"}, false},
		{"마스터 키 없음", ProbeConfig{Write: true}, false},
		{"시크릿만 없음", ProbeConfig{Write: true, MasterAccessKey: "a"}, false},
	}
	for _, tt := range tests {
		if got := tt.cfg.WriteEnabled(); got != tt.want {
			t.Errorf("%s: WriteEnabled() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestClassify(t *testing.T) {
	denied := minio.ErrorResponse{Code: "AccessDenied", Message: "Access Denied."}
	missing := minio.ErrorResponse{Code: "NoSuchKey", Message: "The specified key does not exist."}
	noBucket := minio.ErrorResponse{Code: "NoSuchBucket"}
	weird := minio.ErrorResponse{Code: "SlowDown", Message: "Please reduce your request rate."}

	tests := []struct {
		name string
		err  error
		want ProbeResult
	}{
		{"성공", nil, ProbeAllow},
		{"거부", denied, ProbeDeny},
		// Reaching "no such key" means authorisation passed — that is the
		// whole point of probing against a key that does not exist.
		{"없는 키", missing, ProbeAllow},
		{"없는 버킷", noBucket, ProbeAllow},
		// Anything else must not be folded into "deny": a fault reported as
		// a permission answer is a confident false claim about access.
		{"기타 S3 오류", weird, ProbeError},
		{"전송 실패", errors.New("dial tcp: connection refused"), ProbeError},
	}
	for _, tt := range tests {
		got, detail := classify(tt.err)
		if got != tt.want {
			t.Errorf("%s: classify(%v) = %q, want %q", tt.name, tt.err, got, tt.want)
		}
		if tt.want == ProbeError && detail == "" {
			t.Errorf("%s: an error result should carry a detail", tt.name)
		}
		if tt.want != ProbeError && detail != "" {
			t.Errorf("%s: detail should be empty for %q, got %q", tt.name, got, detail)
		}
	}
}

func TestProbeKeysShareReservedPrefix(t *testing.T) {
	// Everything the prober touches must be identifiable as its own, and
	// the write key must be fixed rather than unique per run so a failed
	// cleanup leaves at most one object per bucket.
	for _, key := range []string{missingKey, writeKey} {
		if len(key) <= len(probeKeyPrefix) || key[:len(probeKeyPrefix)] != probeKeyPrefix {
			t.Errorf("%q should live under %q", key, probeKeyPrefix)
		}
	}
	if missingKey == writeKey {
		t.Error("the missing-key probe must not collide with the write probe's key")
	}
}
