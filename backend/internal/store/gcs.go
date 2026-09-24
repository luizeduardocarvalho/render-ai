package store

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"sync"
	"time"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"google.golang.org/api/iamcredentials/v1"
)

// GCSStore implements BlobStore over a Google Cloud Storage bucket. Image bytes
// live as objects keyed by blob ID; SignedURL mints short-lived V4 signed URLs
// so the browser loads images straight from GCS, without the bytes passing
// through this backend.
type GCSStore struct {
	client *storage.Client
	bucket string

	// signerEmail is the service account whose identity signs URLs, and signBytes
	// signs the string-to-sign via the IAM credentials API - the way to produce
	// V4 signed URLs on Cloud Run, where the runtime credentials carry no private
	// key to sign with locally.
	signerEmail string
	iam         *iamcredentials.Service

	// urlCache memoizes signed URLs per blob ID so a working session's repeated
	// requests for the same image don't each incur an IAM SignBlob round-trip.
	urlCache sync.Map // blobID -> cachedURL
}

type cachedURL struct {
	url       string
	expiresAt time.Time
}

var _ BlobStore = (*GCSStore)(nil)

// NewGCS builds a GCS-backed blob store for the given bucket. signerEmail is
// the service account used to sign URLs; when empty it is auto-detected from
// the runtime metadata server (the normal case on Cloud Run).
func NewGCS(ctx context.Context, bucket, signerEmail string) (*GCSStore, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating storage client: %w", err)
	}
	iamSvc, err := iamcredentials.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating iam credentials client: %w", err)
	}
	if signerEmail == "" {
		email, err := metadata.EmailWithContext(ctx, "default")
		if err != nil {
			return nil, fmt.Errorf("detecting signer service account (set storage.signerServiceAccount / SIGNER_SERVICE_ACCOUNT): %w", err)
		}
		signerEmail = email
	}
	return &GCSStore{client: client, bucket: bucket, signerEmail: signerEmail, iam: iamSvc}, nil
}

func (g *GCSStore) object(id string) *storage.ObjectHandle {
	return g.client.Bucket(g.bucket).Object(id)
}

// PutBlob writes data under a new random ID and returns it. An error means
// the object was never durably written - the ID must not be used.
func (g *GCSStore) PutBlob(data []byte, contentType string) (string, error) {
	id := uuid.NewString()
	if err := g.PutBlobAt(id, data, contentType); err != nil {
		return "", err
	}
	return id, nil
}

// putBlobMaxAttempts bounds how many times PutBlobAt retries a failed write.
const putBlobMaxAttempts = 3

// putBlobBackoff is the delay before retry attempt n (n=1 before the 2nd
// attempt, n=2 before the 3rd): 200ms, then 400ms. Short and few, since these
// are best-effort retries of transient network/server errors held inside one
// request's lifetime, not a long-running background retry.
func putBlobBackoff(n int) time.Duration {
	return 200 * time.Millisecond * time.Duration(1<<uint(n-1))
}

// retryWrite runs fn up to attempts times, sleeping (via sleep) between
// attempts on error, and returns nil as soon as fn succeeds or fn's error
// from the final attempt otherwise. It is deliberately free of any GCS types
// so it can be unit-tested with a fake fn and a fake sleep (see
// gcs_test.go), independent of a real storage client.
func retryWrite(attempts int, sleep func(time.Duration), backoff func(attempt int) time.Duration, fn func(attempt int) error) error {
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = fn(attempt); err == nil {
			return nil
		}
		if attempt < attempts {
			sleep(backoff(attempt))
		}
	}
	return err
}

// crc32cTable is the Castagnoli polynomial GCS uses for its CRC32C object
// checksum.
var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

// PutBlobAt writes data under a caller-chosen ID, overwriting any existing
// object there.
//
// The write's CRC32C is computed up front and attached to the writer
// (SendCRC32C), so GCS itself verifies the bytes it received against the
// checksum and rejects the object on mismatch instead of silently storing a
// corrupted upload.
//
// The whole write (a fresh Writer, Write, Close) is retried up to
// putBlobMaxAttempts times with a short backoff between attempts (via
// retryWrite). This is deliberately an outer retry rather than relying on the
// storage client's own retry machinery: the client only auto-retries a write
// when it is idempotent by its own definition (a precondition such as
// GenerationMatch or DoesNotExist is set), which callers here don't set -
// they intentionally overwrite by ID. Retrying the whole object write is
// still safe in our case because every attempt targets the same fixed blob ID
// with the same bytes, so a retried overwrite converges to the same object
// rather than risking a duplicate. If every attempt fails, the final error is
// returned wrapped with the blob ID; the object is left exactly as it was
// before the call (GCS writes are atomic on Close - an object is only
// created/replaced when Close succeeds, so a failed Write/Close leaves no
// partial object behind).
func (g *GCSStore) PutBlobAt(id string, data []byte, contentType string) error {
	checksum := crc32.Checksum(data, crc32cTable)

	err := retryWrite(putBlobMaxAttempts, time.Sleep, putBlobBackoff, func(attempt int) error {
		err := g.writeBlobOnce(id, data, contentType, checksum)
		if err != nil {
			logStoreErr("gcs: writing blob id=%s size=%d contentType=%s attempt=%d/%d: %v",
				id, len(data), contentType, attempt, putBlobMaxAttempts, err)
		}
		return err
	})
	if err != nil {
		return wrapPutBlobErr(id, err)
	}
	return nil
}

// wrapPutBlobErr wraps a final write failure with the blob ID, so callers'
// logs and error messages can identify which blob was lost without walking
// back through the retry loop. Split out from PutBlobAt so the wrapping can
// be unit-tested (gcs_test.go) without a real GCS client.
func wrapPutBlobErr(id string, err error) error {
	return fmt.Errorf("gcs: writing blob %s: %w", id, err)
}

// writeBlobOnce performs a single write attempt: a fresh writer, its own
// 60s timeout, and Close. Kept separate from the retry loop so it (and the
// loop) can be exercised without a real GCS client - see gcs_test.go.
func (g *GCSStore) writeBlobOnce(id string, data []byte, contentType string, checksum uint32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	w := g.object(id).NewWriter(ctx)
	w.ContentType = contentType
	w.CRC32C = checksum
	w.SendCRC32C = true
	if _, err := w.Write(data); err != nil {
		// Cancel before Close so the upload is aborted rather than committed
		// with whatever bytes made it through.
		cancel()
		_ = w.Close()
		return err
	}
	return w.Close()
}

// GetBlob reads an object's bytes and content type by ID.
func (g *GCSStore) GetBlob(id string) (Blob, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	obj := g.object(id)
	rc, err := obj.NewReader(ctx)
	if err != nil {
		if !errors.Is(err, storage.ErrObjectNotExist) {
			logStoreErr("gcs: opening blob %s: %v", id, err)
		}
		return Blob{}, false
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		logStoreErr("gcs: reading blob %s: %v", id, err)
		return Blob{}, false
	}
	return Blob{Data: data, ContentType: rc.Attrs.ContentType}, true
}

// DeleteBlob removes an object by ID. A missing object is not an error.
func (g *GCSStore) DeleteBlob(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := g.object(id).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		logStoreErr("gcs: deleting blob %s: %v", id, err)
	}
	g.urlCache.Delete(id)
}

// SignedURL returns a V4 signed GET URL for the blob, valid for at least ttl.
// Results are cached per blob ID until shortly before they expire, so repeat
// requests reuse one signature instead of re-signing on every call.
func (g *GCSStore) SignedURL(id string, ttl time.Duration) (string, error) {
	now := time.Now()
	if v, ok := g.urlCache.Load(id); ok {
		if c := v.(cachedURL); c.expiresAt.After(now.Add(refreshBefore)) {
			return c.url, nil
		}
	}
	url, err := storage.SignedURL(g.bucket, id, &storage.SignedURLOptions{
		Scheme:         storage.SigningSchemeV4,
		Method:         "GET",
		GoogleAccessID: g.signerEmail,
		Expires:        now.Add(ttl),
		SignBytes:      g.signBytes,
	})
	if err != nil {
		return "", fmt.Errorf("signing url for blob %s: %w", id, err)
	}
	g.urlCache.Store(id, cachedURL{url: url, expiresAt: now.Add(ttl)})
	return url, nil
}

// refreshBefore is how far ahead of expiry a cached signed URL is treated as
// stale, so we hand out a fresh one before the old lapses mid-use.
const refreshBefore = 5 * time.Minute

// signBytes signs b with the signer service account via the IAM credentials
// SignBlob API - the private key never leaves Google.
func (g *GCSStore) signBytes(b []byte) ([]byte, error) {
	name := "projects/-/serviceAccounts/" + g.signerEmail
	resp, err := g.iam.Projects.ServiceAccounts.SignBlob(name, &iamcredentials.SignBlobRequest{
		Payload: base64.StdEncoding.EncodeToString(b),
	}).Do()
	if err != nil {
		return nil, fmt.Errorf("iam signBlob: %w", err)
	}
	return base64.StdEncoding.DecodeString(resp.SignedBlob)
}

var _ DirectUploads = (*GCSStore)(nil)

// uploadPrefix is where direct browser uploads are staged, apart from real
// blobs. A bucket lifecycle rule (infra/terraform/storage.tf) deletes anything
// left there by an upload that was never finalized.
const uploadPrefix = "uploads/"

// UploadURL returns a V4 signed PUT URL for staging an upload. The content
// type and an if-generation-match:0 precondition are signed in, so the browser
// must send exactly those headers: the URL can only create the object once,
// with the declared type, and never overwrite it.
func (g *GCSStore) UploadURL(uploadID, contentType string, ttl time.Duration) (SignedUpload, error) {
	const precondition = "x-goog-if-generation-match"
	url, err := storage.SignedURL(g.bucket, uploadPrefix+uploadID, &storage.SignedURLOptions{
		Scheme:         storage.SigningSchemeV4,
		Method:         "PUT",
		GoogleAccessID: g.signerEmail,
		Expires:        time.Now().Add(ttl),
		SignBytes:      g.signBytes,
		ContentType:    contentType,
		Headers:        []string{precondition + ":0"},
	})
	if err != nil {
		return SignedUpload{}, fmt.Errorf("signing upload url for %s: %w", uploadID, err)
	}
	return SignedUpload{
		URL:     url,
		Headers: map[string]string{"Content-Type": contentType, precondition: "0"},
	}, nil
}

// ReadUpload reads a staged upload, checking its size before reading it.
func (g *GCSStore) ReadUpload(uploadID string, maxBytes int64) (Blob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	rc, err := g.object(uploadPrefix + uploadID).NewReader(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return Blob{}, ErrNotFound
	}
	if err != nil {
		return Blob{}, fmt.Errorf("opening upload %s: %w", uploadID, err)
	}
	defer rc.Close()
	if rc.Attrs.Size > maxBytes {
		return Blob{}, ErrUploadTooLarge
	}
	data, err := io.ReadAll(rc)
	if err != nil {
		return Blob{}, fmt.Errorf("reading upload %s: %w", uploadID, err)
	}
	return Blob{Data: data, ContentType: rc.Attrs.ContentType}, nil
}

// DeleteUpload removes a staged upload.
func (g *GCSStore) DeleteUpload(uploadID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := g.object(uploadPrefix + uploadID).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		logStoreErr("gcs: deleting upload %s: %v", uploadID, err)
	}
}
