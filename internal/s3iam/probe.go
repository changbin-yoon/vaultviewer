package s3iam

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ProbeResult is what actually happened when a capability was tried against
// the S3 backend, as opposed to what the mirrored policy says should happen.
type ProbeResult string

const (
	// ProbeAllow / ProbeDeny: the backend answered definitively.
	ProbeAllow ProbeResult = "allow"
	ProbeDeny  ProbeResult = "deny"
	// ProbeSkipped: not attempted (see ProbeConfig.Write).
	ProbeSkipped ProbeResult = "skipped"
	// ProbeError: the attempt failed for a reason that is neither an
	// allow nor a deny — a network fault, an expired session. Reported
	// as its own state rather than folded into "deny", which would turn a
	// broken probe into a confident false claim about someone's access.
	ProbeError ProbeResult = "error"
)

// BucketProbe is one bucket's verification outcome.
type BucketProbe struct {
	Bucket string      `json:"bucket"`
	Read   ProbeResult `json:"read"`
	Write  ProbeResult `json:"write"`
	Delete ProbeResult `json:"delete"`
	// Detail explains a ProbeError, empty otherwise.
	Detail string `json:"detail,omitempty"`
}

// probeKeyPrefix is the reserved prefix every probe object lives under, so
// anything this feature leaves behind is identifiable and confined.
const probeKeyPrefix = ".accesslens-probe/"

// missingKey is deliberately an object that does not exist. Read and delete
// are probed against it because S3 checks authorisation before it checks
// existence: with permission the call fails as "no such key" (delete simply
// succeeds, having removed nothing), without it the call fails as
// "access denied". Both answers are obtained without touching real data.
const missingKey = probeKeyPrefix + "does-not-exist"

// writeKey is a fixed key, not a unique one per run, so a probe whose
// cleanup fails leaves at most one zero-byte object per bucket rather than
// accumulating them.
const writeKey = probeKeyPrefix + "write-check"

// ProbeConfig controls what the prober is allowed to attempt.
type ProbeConfig struct {
	// Endpoint is the S3 API host:port.
	Endpoint string
	// Write enables the write probe. Off by default because it is the only
	// one with a side effect: read and delete are answered by a
	// non-existent key, but write can only be settled by actually writing.
	Write bool
	// MasterAccessKey/MasterSecretKey clean up after the write probe. They
	// are needed because the account being tested may hold PutObject
	// without DeleteObject — a dev tier does exactly that — and so cannot
	// remove what it just wrote. Without them the write probe stays off
	// however Write is set.
	MasterAccessKey string
	MasterSecretKey string
	Timeout         time.Duration
}

// WriteEnabled reports whether the write probe can actually run.
func (c ProbeConfig) WriteEnabled() bool {
	return c.Write && c.MasterAccessKey != "" && c.MasterSecretKey != ""
}

// Prober verifies capabilities against the live S3 backend using the
// caller's own temporary session, so what it measures is the caller's
// access and not the application's.
type Prober struct {
	cfg ProbeConfig
}

func NewProber(cfg ProbeConfig) *Prober {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Prober{cfg: cfg}
}

// classify turns a MinIO error into a probe result. "Access denied" is the
// only thing treated as a deny; everything else that isn't success is
// reported as an error, so a fault never masquerades as a permission answer.
func classify(err error) (ProbeResult, string) {
	if err == nil {
		return ProbeAllow, ""
	}
	resp := minio.ToErrorResponse(err)
	switch resp.Code {
	case "AccessDenied":
		return ProbeDeny, ""
	case "NoSuchKey", "NoSuchBucket", "":
		// Reaching a "no such key" means authorisation passed — that is
		// precisely what the missing-key probe is looking for. An empty
		// code with a non-nil error is a transport-level failure.
		if resp.Code == "" {
			return ProbeError, err.Error()
		}
		return ProbeAllow, ""
	default:
		return ProbeError, fmt.Sprintf("%s: %s", resp.Code, resp.Message)
	}
}

// Probe verifies read, write and delete for each bucket using the supplied
// session credentials, which must be the logged-in user's own.
func (p *Prober) Probe(ctx context.Context, creds SessionCredentials, buckets []string) ([]BucketProbe, error) {
	client, err := minio.New(p.cfg.Endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken),
	})
	if err != nil {
		return nil, fmt.Errorf("s3iam: probe client: %w", err)
	}
	var master *minio.Client
	if p.cfg.WriteEnabled() {
		master, err = minio.New(p.cfg.Endpoint, &minio.Options{
			Creds: credentials.NewStaticV4(p.cfg.MasterAccessKey, p.cfg.MasterSecretKey, ""),
		})
		if err != nil {
			return nil, fmt.Errorf("s3iam: probe cleanup client: %w", err)
		}
	}

	sorted := append([]string(nil), buckets...)
	sort.Strings(sorted)

	out := make([]BucketProbe, 0, len(sorted))
	for _, bucket := range sorted {
		out = append(out, p.probeBucket(ctx, client, master, bucket))
	}
	return out, nil
}

func (p *Prober) probeBucket(ctx context.Context, client, master *minio.Client, bucket string) BucketProbe {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	result := BucketProbe{Bucket: bucket, Write: ProbeSkipped}

	_, err := client.StatObject(ctx, bucket, missingKey, minio.StatObjectOptions{})
	result.Read, result.Detail = classify(err)

	// Deleting a key that does not exist removes nothing, so this is safe
	// to run against any bucket regardless of what it holds.
	err = client.RemoveObject(ctx, bucket, missingKey, minio.RemoveObjectOptions{})
	deleteResult, deleteDetail := classify(err)
	result.Delete = deleteResult
	if result.Detail == "" {
		result.Detail = deleteDetail
	}

	if !p.cfg.WriteEnabled() {
		return result
	}

	_, err = client.PutObject(ctx, bucket, writeKey, bytes.NewReader(nil), 0, minio.PutObjectOptions{})
	writeResult, writeDetail := classify(err)
	result.Write = writeResult
	if result.Detail == "" {
		result.Detail = writeDetail
	}
	if writeResult == ProbeAllow && master != nil {
		// Cleanup runs as the master account, not the caller: the caller
		// may hold PutObject without DeleteObject and so be unable to
		// remove what it just wrote.
		if err := master.RemoveObject(ctx, bucket, writeKey, minio.RemoveObjectOptions{}); err != nil {
			result.Detail = fmt.Sprintf("쓰기 프로브 객체 정리 실패 (%s/%s): %v", bucket, writeKey, err)
		}
	}
	return result
}
