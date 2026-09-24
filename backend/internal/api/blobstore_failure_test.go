package api

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"render-ai/backend/internal/config"
	"render-ai/backend/internal/jobs"
	"render-ai/backend/internal/store"
)

// failingBlobStore wraps a real BlobStore but makes every write fail, so
// tests can assert that a blob-store write failure is surfaced as an HTTP
// error and never followed by a Repository write that would record a
// reference to data that was never actually stored.
type failingBlobStore struct {
	store.BlobStore
}

var errBlobWrite = errors.New("gcs: writing blob fake-id: simulated write failure")

func (f *failingBlobStore) PutBlob(data []byte, contentType string) (string, error) {
	return "", errBlobWrite
}

func (f *failingBlobStore) PutBlobAt(id string, data []byte, contentType string) error {
	return errBlobWrite
}

// testPNG returns a tiny valid PNG so decodeImageConfig succeeds.
func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding test png: %v", err)
	}
	return buf.Bytes()
}

// multipartFileRequest builds a POST/PUT request carrying data under a
// multipart "file" field, the shape readMultipartFile expects.
func multipartFileRequest(t *testing.T, method, url string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "image.png")
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("writing form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}
	r := httptest.NewRequest(method, url, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

// TestUploadAssetReferenceFailsLoudlyOnBlobWriteError checks that when the
// blob store's write fails, the handler returns a 5xx and never calls
// through to the repository - so no asset ends up pointing at a reference
// image that was never actually stored.
func TestUploadAssetReferenceFailsLoudlyOnBlobWriteError(t *testing.T) {
	repo := store.NewMemory()
	p := repo.CreateProject("owner-1", "P")
	asset, err := repo.CreateAsset(p.ID, "Sofa", "a sofa", "#ff0000")
	if err != nil {
		t.Fatalf("creating asset: %v", err)
	}

	s := NewServer(repo, &failingBlobStore{BlobStore: repo}, nil, nil, &config.Config{}, "", jobs.NewInline())

	r := multipartFileRequest(t, http.MethodPost, "/", testPNG(t))
	r.SetPathValue("pid", p.ID)
	r.SetPathValue("aid", asset.ID)

	err = s.uploadAssetReference(httptest.NewRecorder(), r)
	if err == nil {
		t.Fatal("expected an error from uploadAssetReference, got nil")
	}
	status, _ := statusAndMessage(err)
	if status < 500 || status > 599 {
		t.Fatalf("want a 5xx status, got %d", status)
	}

	// The asset must not have been updated to reference an unsaved blob.
	got, getErr := repo.GetProject(p.ID)
	if getErr != nil {
		t.Fatalf("getting project: %v", getErr)
	}
	for _, a := range got.Assets {
		if a.ID == asset.ID && a.HasReferenceImage {
			t.Fatal("asset was recorded as having a reference image despite the blob write failing")
		}
	}
}

// TestCreateViewFailsLoudlyOnBlobWriteError checks the same contract for
// view creation: a failed screenshot write must return a 5xx and must not
// create a view record pointing at a missing screenshot blob.
func TestCreateViewFailsLoudlyOnBlobWriteError(t *testing.T) {
	repo := store.NewMemory()
	p := repo.CreateProject("owner-1", "P")

	s := NewServer(repo, &failingBlobStore{BlobStore: repo}, nil, nil, &config.Config{}, "", jobs.NewInline())

	r := multipartFileRequest(t, http.MethodPost, "/", testPNG(t))
	r.SetPathValue("pid", p.ID)

	err := s.createView(httptest.NewRecorder(), r)
	if err == nil {
		t.Fatal("expected an error from createView, got nil")
	}
	status, _ := statusAndMessage(err)
	if status < 500 || status > 599 {
		t.Fatalf("want a 5xx status, got %d", status)
	}

	got, getErr := repo.GetProject(p.ID)
	if getErr != nil {
		t.Fatalf("getting project: %v", getErr)
	}
	if len(got.Views) != 0 {
		t.Fatalf("expected no view to be created, got %d", len(got.Views))
	}
}

// TestUploadMaskBitmapFailsLoudlyOnBlobWriteError checks the mask-bitmap
// upload path: a failed write must return a 5xx and must not mark the mask
// as having a bitmap.
func TestUploadMaskBitmapFailsLoudlyOnBlobWriteError(t *testing.T) {
	repo := store.NewMemory()
	p := repo.CreateProject("owner-1", "P")
	png := testPNG(t)
	cfg, _, err := decodeImageConfig(png)
	if err != nil {
		t.Fatalf("decoding test png: %v", err)
	}
	view, err := repo.CreateView(p.ID, "V", "screenshot-blob", cfg.Width, cfg.Height)
	if err != nil {
		t.Fatalf("creating view: %v", err)
	}
	mask, err := repo.CreateMask(p.ID, view.ID, nil)
	if err != nil {
		t.Fatalf("creating mask: %v", err)
	}

	s := NewServer(repo, &failingBlobStore{BlobStore: repo}, nil, nil, &config.Config{}, "", jobs.NewInline())

	r := multipartFileRequest(t, http.MethodPut, "/", png)
	r.SetPathValue("pid", p.ID)
	r.SetPathValue("vid", view.ID)
	r.SetPathValue("mid", mask.ID)

	uploadErr := s.uploadMaskBitmap(httptest.NewRecorder(), r)
	if uploadErr == nil {
		t.Fatal("expected an error from uploadMaskBitmap, got nil")
	}
	status, _ := statusAndMessage(uploadErr)
	if status < 500 || status > 599 {
		t.Fatalf("want a 5xx status, got %d", status)
	}

	gotView, getErr := repo.GetView(p.ID, view.ID)
	if getErr != nil {
		t.Fatalf("getting view: %v", getErr)
	}
	for _, m := range gotView.Masks {
		if m.ID == mask.ID && m.HasBitmap {
			t.Fatal("mask was recorded as having a bitmap despite the blob write failing")
		}
	}
}
