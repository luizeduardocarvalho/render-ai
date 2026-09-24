package store

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
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

// PutBlob writes data under a new random ID and returns it.
func (g *GCSStore) PutBlob(data []byte, contentType string) string {
	id := uuid.NewString()
	g.PutBlobAt(id, data, contentType)
	return id
}

// PutBlobAt writes data under a caller-chosen ID, overwriting any existing
// object there. Errors are logged via the returned-nothing contract of the
// interface; a failed write surfaces later as a missing blob, matching the
// in-memory store's best-effort semantics.
func (g *GCSStore) PutBlobAt(id string, data []byte, contentType string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	w := g.object(id).NewWriter(ctx)
	w.ContentType = contentType
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		logStoreErr("gcs: writing blob %s: %v", id, err)
		return
	}
	if err := w.Close(); err != nil {
		logStoreErr("gcs: closing blob %s: %v", id, err)
	}
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
