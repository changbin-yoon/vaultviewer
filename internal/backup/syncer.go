package backup

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Syncer periodically mirrors a local directory tree into an S3-compatible
// bucket. Two safety properties by design:
//   - Only changed or missing files are (re-)uploaded — each pass compares
//     local content against the remote object already in S3 (see
//     needsUpload), not against any in-memory state, so a pod restart can
//     never cause a stale cache to wrongly skip a file.
//   - It never deletes objects it previously uploaded: a transient bug or
//     partial failure in a sync pass must never be able to destroy an
//     earlier, good backup — at worst a pass leaves stale objects behind,
//     which is the safer failure mode for something whose whole purpose is
//     disaster recovery.
type Syncer struct {
	cfg    Config
	root   string
	client *minio.Client
}

// NewSyncer constructs a Syncer that will mirror root into cfg's bucket. It
// does not verify the bucket exists yet — Run logs (but does not fail on) a
// missing bucket so a syncer started before its bucket is provisioned will
// self-heal once the bucket shows up.
func NewSyncer(cfg Config, root string) (*Syncer, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	return &Syncer{cfg: cfg, root: root, client: client}, nil
}

// Run performs an immediate sync pass and then one every cfg.Interval,
// until ctx is cancelled. A failed or partial pass is logged, never fatal —
// a backup outage must not take down the main server.
func (s *Syncer) Run(ctx context.Context) {
	s.syncOnce(ctx)
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncOnce(ctx)
		}
	}
}

// syncOnce walks every file under root and uploads it to a date-prefixed
// key (<prefix>/<YYYY-MM-DD>/<relative path>) only if it's new or its
// content differs from what's already there — see needsUpload. Scoping
// each day to its own prefix means a bad sync on one day can never
// overwrite or corrupt a previous day's snapshot — every calendar day is
// an independent, safely re-syncable recovery point. Comparing against S3
// directly (rather than remembering what we last uploaded in memory) means
// a pod restart can never cause a stale in-memory cache to wrongly skip a
// file that actually needs (re-)uploading.
func (s *Syncer) syncOnce(ctx context.Context) {
	if exists, err := s.client.BucketExists(ctx, s.cfg.Bucket); err != nil {
		log.Printf("backup: checking bucket %q: %v", s.cfg.Bucket, err)
	} else if !exists {
		log.Printf("backup: bucket %q does not exist yet — skipping this pass", s.cfg.Bucket)
		return
	}

	datePrefix := time.Now().UTC().Format("2006-01-02")
	uploaded, skipped, failed := 0, 0, 0

	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			log.Printf("backup: skipping %s: %v", path, walkErr)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			log.Printf("backup: skipping %s: %v", path, err)
			return nil
		}
		key := objectKey(s.cfg.Prefix, datePrefix, rel)

		upload, err := s.needsUpload(ctx, key, path)
		if err != nil {
			log.Printf("backup: checking %s: %v (uploading anyway)", rel, err)
		}
		if !upload {
			skipped++
			return nil
		}

		if _, err := s.client.FPutObject(ctx, s.cfg.Bucket, key, path, minio.PutObjectOptions{}); err != nil {
			log.Printf("backup: upload %s: %v", rel, err)
			failed++
			return nil
		}
		uploaded++
		return nil
	})
	if err != nil {
		log.Printf("backup: walk %s: %v", s.root, err)
		return
	}
	log.Printf("backup: synced to s3://%s/%s — %d uploaded, %d unchanged, %d failed",
		s.cfg.Bucket, objectKey(s.cfg.Prefix, datePrefix, ""), uploaded, skipped, failed)
}

// needsUpload reports whether localPath's content differs from whatever
// object currently sits at key (or the object doesn't exist yet). It never
// trusts an ambiguous signal into skipping a real upload: any error
// checking the remote object or hashing the local file is reported back to
// the caller alongside upload=true, so the caller falls back to uploading
// rather than silently — and possibly wrongly — leaving a file stale.
func (s *Syncer) needsUpload(ctx context.Context, key, localPath string) (bool, error) {
	info, err := s.client.StatObject(ctx, s.cfg.Bucket, key, minio.StatObjectOptions{})
	if err != nil {
		resp := minio.ToErrorResponse(err)
		if resp.Code == "NoSuchKey" || resp.StatusCode == 404 {
			return true, nil
		}
		return true, err
	}

	localHash, err := md5File(localPath)
	if err != nil {
		return true, err
	}
	return !sameContent(localHash, info.ETag), nil
}

// sameContent reports whether a remote object's ETag matches the local
// file's MD5 hash. A single-part S3/MinIO upload's ETag is exactly the hex
// MD5 of its content, but a multipart upload's ETag takes the form
// "<hex>-<n>" and isn't a content hash at all — those are conservatively
// treated as "different" (forcing a re-upload) since they can't be safely
// compared. FPutObject only multiparts files above minio-go's ~128MiB
// threshold, well above anything in a notes vault, so this only matters
// for unusually large attachments.
func sameContent(localMD5Hex, remoteETag string) bool {
	tag := strings.Trim(remoteETag, "\"")
	if strings.Contains(tag, "-") {
		return false
	}
	return strings.EqualFold(localMD5Hex, tag)
}

func md5File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// objectKey joins prefix/datePrefix/rel into a forward-slash S3 key,
// omitting any empty segment (prefix is optional, rel is empty when
// building a display-only path).
func objectKey(prefix, datePrefix, rel string) string {
	parts := make([]string, 0, 3)
	if prefix != "" {
		parts = append(parts, strings.Trim(prefix, "/"))
	}
	if datePrefix != "" {
		parts = append(parts, datePrefix)
	}
	if rel != "" {
		parts = append(parts, filepath.ToSlash(rel))
	}
	return strings.Join(parts, "/")
}
