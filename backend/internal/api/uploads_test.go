package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"render-ai/backend/internal/config"
	"render-ai/backend/internal/jobs"
	"render-ai/backend/internal/store"
)

// fakeUploadStore is a MemoryStore that also takes direct uploads, staged in
// a map the test fills in to play the browser's PUT.
type fakeUploadStore struct {
	*store.MemoryStore
	mu      sync.Mutex
	staged  map[string]store.Blob
	lastURL string
}

func newFakeUploadStore() *fakeUploadStore {
	return &fakeUploadStore{MemoryStore: store.NewMemory(), staged: map[string]store.Blob{}}
}

func (f *fakeUploadStore) UploadURL(uploadID, contentType string, _ time.Duration) (store.SignedUpload, error) {
	f.lastURL = "https://storage.example/uploads/" + uploadID
	return store.SignedUpload{URL: f.lastURL, Headers: map[string]string{"Content-Type": contentType}}, nil
}

func (f *fakeUploadStore) ReadUpload(uploadID string, maxBytes int64) (store.Blob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.staged[uploadID]
	if !ok {
		return store.Blob{}, store.ErrNotFound
	}
	if int64(len(b.Data)) > maxBytes {
		return store.Blob{}, store.ErrUploadTooLarge
	}
	return b, nil
}

func (f *fakeUploadStore) DeleteUpload(uploadID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.staged, uploadID)
}

func (f *fakeUploadStore) stage(uploadID string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.staged[uploadID] = store.Blob{Data: data, ContentType: "image/png"}
}

func jsonRequest(t *testing.T, body any) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding body: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func statusOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	status, _ := statusAndMessage(err)
	return status
}

// requestUpload calls createUpload and returns the minted upload ID.
func requestUpload(t *testing.T, s *Server, pid, contentType string) string {
	t.Helper()
	r := jsonRequest(t, map[string]string{"contentType": contentType})
	r.SetPathValue("pid", pid)
	rec := httptest.NewRecorder()
	if err := s.createUpload(rec, r); err != nil {
		t.Fatalf("createUpload: %v", err)
	}
	var resp uploadResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding upload response: %v", err)
	}
	if resp.Method != http.MethodPut || resp.URL == "" || resp.Headers["Content-Type"] != contentType {
		t.Fatalf("unexpected upload response: %+v", resp)
	}
	return resp.UploadID
}

func TestCreateUploadNotImplementedWithoutDirectUploads(t *testing.T) {
	repo := store.NewMemory()
	p := repo.CreateProject("owner-1", "P")
	s := NewServer(repo, repo, nil, nil, &config.Config{}, "", jobs.NewInline())

	r := jsonRequest(t, map[string]string{"contentType": "image/png"})
	r.SetPathValue("pid", p.ID)
	if got := statusOf(s.createUpload(httptest.NewRecorder(), r)); got != http.StatusNotImplemented {
		t.Fatalf("want 501 so the frontend falls back to multipart, got %d", got)
	}
}

func TestCreateUploadRejectsUnsupportedContentType(t *testing.T) {
	fs := newFakeUploadStore()
	p := fs.CreateProject("owner-1", "P")
	s := NewServer(fs, fs, nil, nil, &config.Config{}, "", jobs.NewInline())

	r := jsonRequest(t, map[string]string{"contentType": "image/gif"})
	r.SetPathValue("pid", p.ID)
	if got := statusOf(s.createUpload(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", got)
	}
}

func TestCreateViewFromDirectUpload(t *testing.T) {
	fs := newFakeUploadStore()
	p := fs.CreateProject("owner-1", "P")
	s := NewServer(fs, fs, nil, nil, &config.Config{}, "", jobs.NewInline())

	uploadID := requestUpload(t, s, p.ID, "image/png")
	fs.stage(uploadID, testPNG(t))

	r := jsonRequest(t, map[string]string{"name": "Living room", "uploadId": uploadID})
	r.SetPathValue("pid", p.ID)
	rec := httptest.NewRecorder()
	if err := s.createView(rec, r); err != nil {
		t.Fatalf("createView: %v", err)
	}
	var v store.View
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding view: %v", err)
	}
	if v.Name != "Living room" || v.Width != 4 || v.Height != 4 || !v.HasScreenshot {
		t.Fatalf("unexpected view: %+v", v)
	}
	if v.ScreenshotImageID == uploadID {
		t.Fatal("screenshot blob must get a fresh ID, not reuse the upload ID")
	}
	if _, ok := fs.GetBlob(v.ScreenshotImageID); !ok {
		t.Fatal("screenshot blob was not stored")
	}

	// The staged upload is consumed: using it again must fail.
	r = jsonRequest(t, map[string]string{"name": "Again", "uploadId": uploadID})
	r.SetPathValue("pid", p.ID)
	if got := statusOf(s.createView(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
		t.Fatalf("reusing an upload: want 400, got %d", got)
	}
}

func TestCreateViewFromDirectUploadRejectsBadInput(t *testing.T) {
	fs := newFakeUploadStore()
	p := fs.CreateProject("owner-1", "P")
	s := NewServer(fs, fs, nil, nil, &config.Config{}, "", jobs.NewInline())

	cases := []struct {
		name     string
		uploadID string
		data     []byte
	}{
		{"not an image", "11111111-1111-1111-1111-111111111111", []byte("not an image")},
		{"too large", "22222222-2222-2222-2222-222222222222", make([]byte, maxUploadBytes+1)},
		{"never uploaded", "33333333-3333-3333-3333-333333333333", nil},
		{"malformed id", "../screenshot", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.data != nil {
				fs.stage(tc.uploadID, tc.data)
			}
			r := jsonRequest(t, map[string]string{"name": "V", "uploadId": tc.uploadID})
			r.SetPathValue("pid", p.ID)
			if got := statusOf(s.createView(httptest.NewRecorder(), r)); got != http.StatusBadRequest {
				t.Fatalf("want 400, got %d", got)
			}
			if _, err := fs.ReadUpload(tc.uploadID, maxUploadBytes*2); err == nil {
				t.Fatal("a rejected upload must be deleted")
			}
		})
	}
	got, err := fs.GetProject(p.ID)
	if err != nil {
		t.Fatalf("getting project: %v", err)
	}
	if len(got.Views) != 0 {
		t.Fatalf("no view should have been created, got %d", len(got.Views))
	}
}

func TestUploadAssetReferenceFromDirectUpload(t *testing.T) {
	fs := newFakeUploadStore()
	p := fs.CreateProject("owner-1", "P")
	asset, err := fs.CreateAsset("owner-1", "Sofa", "a sofa", "#ff0000")
	if err != nil {
		t.Fatalf("creating asset: %v", err)
	}
	s := NewServer(fs, fs, nil, nil, &config.Config{}, "", jobs.NewInline())

	uploadID := requestUpload(t, s, p.ID, "image/png")
	fs.stage(uploadID, testPNG(t))

	// An unknown asset must not consume the upload.
	r := jsonRequest(t, map[string]string{"uploadId": uploadID})
	r.SetPathValue("pid", p.ID)
	r.SetPathValue("aid", "missing")
	if got := statusOf(s.uploadAssetReference(httptest.NewRecorder(), r)); got != http.StatusNotFound {
		t.Fatalf("unknown asset: want 404, got %d", got)
	}

	r = jsonRequest(t, map[string]string{"uploadId": uploadID})
	r.SetPathValue("pid", p.ID)
	r.SetPathValue("aid", asset.ID)
	rec := httptest.NewRecorder()
	if err := s.uploadAssetReference(rec, r); err != nil {
		t.Fatalf("uploadAssetReference: %v", err)
	}
	var a store.Asset
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatalf("decoding asset: %v", err)
	}
	if !a.HasReferenceImage {
		t.Fatalf("asset should have a reference image: %+v", a)
	}
	if _, ok := fs.GetBlob(a.ReferenceImageID); !ok {
		t.Fatal("reference blob was not stored")
	}
}
