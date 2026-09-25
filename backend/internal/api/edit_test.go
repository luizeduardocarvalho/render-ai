package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	renderpkg "render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

const editW, editH = 80, 48

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding png: %v", err)
	}
	return buf.Bytes()
}

// patternedPNG is a w x h image no two neighbouring cells of which match, so
// a pixel that moved or blurred is caught.
func patternedPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 3), G: uint8(y * 5), B: uint8((x + y) * 2), A: 255})
		}
	}
	return encodePNG(t, img)
}

// maskB64 is a w x h white-on-black PNG with r painted, base64 encoded the
// way the edit endpoint takes it.
func maskB64(t *testing.T, w, h int, r image.Rectangle) string {
	t.Helper()
	g := image.NewGray(image.Rect(0, 0, w, h))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			g.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	return base64.StdEncoding.EncodeToString(encodePNG(t, g))
}

// hugeMaskB64 is a valid PNG with the source's aspect ratio whose header
// declares a size far past what the editor ever sends.
func hugeMaskB64(t *testing.T) string {
	t.Helper()
	w := maxEditBitmapSide + 80
	return base64.StdEncoding.EncodeToString(encodePNG(t, image.NewGray(image.Rect(0, 0, w, w*editH/editW))))
}

// addSourceRender stores a finished pro/2K render of size editW x editH in the
// view, standing in for one the user made earlier.
func addSourceRender(t *testing.T, repo *store.MemoryStore, pid, vid string) (*store.Render, []byte) {
	t.Helper()
	data := patternedPNG(t, editW, editH)
	id, err := repo.PutBlob(data, "image/png")
	if err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	rec, err := repo.AddRender(pid, vid, &store.Render{
		ID: "render-source", Model: store.ModelPro, Resolution: store.Resolution2K, ResultImageID: id,
	})
	if err != nil {
		t.Fatalf("AddRender: %v", err)
	}
	return rec, data
}

func postStartEdit(t *testing.T, s *Server, pid, vid, rid string, body editRequestBody) (*renderJobResponse, error) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshaling request: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	r.SetPathValue("pid", pid)
	r.SetPathValue("vid", vid)
	r.SetPathValue("rid", rid)
	w := httptest.NewRecorder()
	if err := s.startEdit(w, r); err != nil {
		return nil, err
	}
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	var resp renderJobResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return &resp, nil
}

func wantStatus(t *testing.T, err error, want int) {
	t.Helper()
	var he *httpError
	if !errors.As(err, &he) {
		t.Fatalf("want an *httpError with status %d, got %T (%v)", want, err, err)
	}
	if he.status != want {
		t.Fatalf("status = %d (%s), want %d", he.status, he.message, want)
	}
}

// recordingRenderer answers every call with a solid image of the given size
// and remembers the requests.
type recordingRenderer struct {
	mu       sync.Mutex
	requests []renderpkg.RenderRequest
	answer   []byte
}

func (r *recordingRenderer) Render(_ context.Context, req renderpkg.RenderRequest) (renderpkg.RenderResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
	return renderpkg.RenderResult{ImageData: r.answer, MIMEType: "image/png"}, nil
}

func solidPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return encodePNG(t, img)
}

// TestStartEditProducesLinkedRenderThatKeepsOutsideIdentical is the whole
// feature end to end: an edit job runs through the queue, the model is asked
// with the source plus an overlay and the instruction, and the stored result is
// a new Render linked to its source whose pixels outside the region are the
// source's own.
func TestStartEditProducesLinkedRenderThatKeepsOutsideIdentical(t *testing.T) {
	red := color.NRGBA{R: 220, G: 20, B: 20, A: 255}
	// The model answers at twice the source's size, as a real one may.
	renderer := &recordingRenderer{answer: solidPNG(t, editW*2, editH*2, red)}
	s, repo, p, v, queue := setupRenderTest(t, renderer)
	source, sourceData := addSourceRender(t, repo, p.ID, v.ID)

	// The mask is drawn at half the render's size, like the editor does.
	resp, err := postStartEdit(t, s, p.ID, v.ID, source.ID, editRequestBody{Regions: []editRegionBody{
		{Instruction: "  a brass floor lamp  ", Bitmap: maskB64(t, editW/2, editH/2, image.Rect(15, 8, 25, 16))},
	}})
	if err != nil {
		t.Fatalf("startEdit: %v", err)
	}
	if resp.Request.Model != store.ModelPro || resp.Request.Resolution != store.Resolution2K || resp.Request.Variations != 1 {
		t.Errorf("request = %+v, want the source's pro/2K with one variation", resp.Request)
	}
	regionBlob := resp.Request.Edit.Regions[0].BitmapImageID
	queue.Wait()

	final, err := getRenderJobResponse(t, s, p.ID, resp.ID)
	if err != nil {
		t.Fatalf("getRenderJob: %v", err)
	}
	if final.Status != store.RenderJobDone || len(final.Renders) != 1 {
		t.Fatalf("job = %s with %d renders (error %v), want done with one", final.Status, len(final.Renders), final.Error)
	}
	edit := final.Renders[0]
	if edit.SourceRenderID != source.ID {
		t.Errorf("SourceRenderID = %q, want %q", edit.SourceRenderID, source.ID)
	}
	if len(edit.EditInstructions) != 1 || edit.EditInstructions[0] != "a brass floor lamp" {
		t.Errorf("EditInstructions = %q, want the trimmed instruction", edit.EditInstructions)
	}
	if edit.Model != store.ModelPro || edit.Resolution != store.Resolution2K || edit.RegionCount != 1 || edit.Preservation != nil {
		t.Errorf("render = %+v, want pro/2K, one region, no preservation check", edit)
	}

	// What the model was asked.
	if len(renderer.requests) != 1 {
		t.Fatalf("model called %d times, want 1", len(renderer.requests))
	}
	req := renderer.requests[0]
	screenshot, ok := s.blobs.GetBlob(v.ScreenshotImageID)
	if !ok {
		t.Fatal("the view's screenshot blob is missing")
	}
	if len(req.Images) != 3 || !bytes.Equal(req.Images[0], sourceData) || !bytes.Equal(req.Images[2], screenshot.Data) {
		t.Errorf("model got %d images (source first = %v), want 3: the source, the overlay, then the view's screenshot",
			len(req.Images), bytes.Equal(req.Images[0], sourceData))
	}
	if !strings.Contains(req.Prompt, "IMAGE 3: The original 3D model screenshot") {
		t.Errorf("prompt does not describe the screenshot:\n%s", req.Prompt)
	}
	if !strings.Contains(req.Prompt, "red area (region 1): a brass floor lamp") {
		t.Errorf("prompt does not carry the instruction:\n%s", req.Prompt)
	}
	if req.ImageSize != "2K" {
		t.Errorf("ImageSize = %q, want the source's 2K", req.ImageSize)
	}

	// What was stored.
	blob, ok := s.blobs.GetBlob(edit.ResultImageID)
	if !ok {
		t.Fatal("edit result blob is missing")
	}
	got, err := png.Decode(bytes.NewReader(blob.Data))
	if err != nil {
		t.Fatalf("decoding the edit result: %v", err)
	}
	want, err := png.Decode(bytes.NewReader(sourceData))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != want.Bounds() {
		t.Fatalf("edit is %v, want the source's %v", got.Bounds(), want.Bounds())
	}
	if c := color.NRGBAModel.Convert(got.At(40, 24)).(color.NRGBA); c != red {
		t.Errorf("centre of the region = %v, want the model's red", c)
	}
	// Far from the region every pixel is the source's, bit for bit.
	for y := 0; y < editH; y++ {
		for x := 0; x < editW; x++ {
			if x >= 20 && x < 60 && y >= 4 && y < 44 { // region 30..50 x 16..32, plus feather
				continue
			}
			if got.At(x, y) != want.At(x, y) {
				t.Fatalf("pixel (%d,%d) outside the edit changed", x, y)
			}
		}
	}

	if _, ok := s.blobs.GetBlob(regionBlob); ok {
		t.Error("the region bitmap should be deleted once the edit has finished")
	}
}

func TestStartEditRejectsBadRequests(t *testing.T) {
	renderer := alwaysSucceeds(t)
	s, repo, p, v, queue := setupRenderTest(t, renderer)
	source, _ := addSourceRender(t, repo, p.ID, v.ID)
	okMask := maskB64(t, editW, editH, image.Rect(10, 10, 20, 20))
	region := func(instruction, bitmap string) editRegionBody {
		return editRegionBody{Instruction: instruction, Bitmap: bitmap}
	}

	nine := make([]editRegionBody, 9)
	for i := range nine {
		nine[i] = region("x", okMask)
	}
	notPNG := base64.StdEncoding.EncodeToString([]byte("not an image"))
	jpegLike := func() string {
		var buf bytes.Buffer
		_ = png.Encode(&buf, image.NewGray(image.Rect(0, 0, 4, 4)))
		return base64.StdEncoding.EncodeToString(buf.Bytes()[:20]) // truncated
	}()

	cases := []struct {
		name    string
		rid     string
		regions []editRegionBody
		want    int
	}{
		{"no regions", source.ID, nil, http.StatusBadRequest},
		{"too many regions", source.ID, nine, http.StatusBadRequest},
		{"blank instruction", source.ID, []editRegionBody{region("   ", okMask)}, http.StatusBadRequest},
		{"instruction too long", source.ID, []editRegionBody{region(strings.Repeat("é", maxEditInstructionRunes+1), okMask)}, http.StatusBadRequest},
		{"bitmap not base64", source.ID, []editRegionBody{region("x", "%%%")}, http.StatusBadRequest},
		{"bitmap not an image", source.ID, []editRegionBody{region("x", notPNG)}, http.StatusBadRequest},
		{"bitmap truncated", source.ID, []editRegionBody{region("x", jpegLike)}, http.StatusBadRequest},
		{"wrong aspect ratio", source.ID, []editRegionBody{region("x", maskB64(t, editW, editW, image.Rect(1, 1, 5, 5)))}, http.StatusBadRequest},
		{"bitmap too large", source.ID, []editRegionBody{region("x", hugeMaskB64(t))}, http.StatusBadRequest},
		{"nothing painted", source.ID, []editRegionBody{region("x", maskB64(t, editW, editH, image.Rectangle{}))}, http.StatusBadRequest},
		{"unknown render", "nope", []editRegionBody{region("x", okMask)}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := postStartEdit(t, s, p.ID, v.ID, tc.rid, editRequestBody{Regions: tc.regions})
			wantStatus(t, err, tc.want)
		})
	}

	// A rejected request must never reach the queue, let alone the model.
	queue.Wait()
	if n := renderer.callCount(); n != 0 {
		t.Fatalf("the model was called %d times for rejected requests, want 0", n)
	}
}

func TestStartEditChargesLikeARenderAndRefundsFailure(t *testing.T) {
	failing := &fakeRenderer{fn: func(int) (renderpkg.RenderResult, error) { return renderpkg.RenderResult{}, errors.New("boom") }}
	s, repo, p, v, queue := setupRenderTest(t, failing)
	source, _ := addSourceRender(t, repo, p.ID, v.ID)
	price := unitsPerVariation(store.ModelPro, store.Resolution2K)
	enableAuthAndGrant(t, s, repo, p.OwnerID, price)

	body := editRequestBody{Regions: []editRegionBody{{Instruction: "x", Bitmap: maskB64(t, editW, editH, image.Rect(10, 10, 20, 20))}}}
	resp, err := postStartEdit(t, s, p.ID, v.ID, source.ID, body)
	if err != nil {
		t.Fatalf("startEdit: %v", err)
	}
	if resp.CreditsCharged != unitsToCredits(price) {
		t.Errorf("CreditsCharged = %v, want the price of one pro/2K render (%v)", resp.CreditsCharged, unitsToCredits(price))
	}
	queue.Wait()

	final, err := getRenderJobResponse(t, s, p.ID, resp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != store.RenderJobFailed {
		t.Fatalf("status = %s, want failed", final.Status)
	}
	if units, err := repo.GetCredits(p.OwnerID); err != nil || units != price {
		t.Fatalf("balance after a failed edit = %d, %v, want the %d units refunded", units, err, price)
	}

	// With the balance spent, the next edit is a 402 and stores nothing.
	if _, err := repo.AdjustCredits(p.OwnerID, -price, store.CreditLedgerEntry{Reason: store.CreditReasonRender}); err != nil {
		t.Fatal(err)
	}
	_, err = postStartEdit(t, s, p.ID, v.ID, source.ID, body)
	wantStatus(t, err, http.StatusPaymentRequired)
}

// TestStartEditDeletesBitmapsWhenNothingWasQueued: a 502 from a dead queue
// leaves no worker to delete the region bitmaps, so the request must.
func TestStartEditDeletesBitmapsWhenNothingWasQueued(t *testing.T) {
	s, repo, p, v, _ := setupRenderTest(t, alwaysSucceeds(t))
	s.queue = failingQueue{}
	source, _ := addSourceRender(t, repo, p.ID, v.ID)
	blobs := &recordingBlobs{BlobStore: repo}
	s.blobs = blobs

	_, err := postStartEdit(t, s, p.ID, v.ID, source.ID, editRequestBody{Regions: []editRegionBody{
		{Instruction: "x", Bitmap: maskB64(t, editW, editH, image.Rect(10, 10, 20, 20))},
	}})
	wantStatus(t, err, http.StatusBadGateway)
	if len(blobs.ids) != 1 {
		t.Fatalf("stored %d region bitmaps, want 1", len(blobs.ids))
	}
	if _, ok := repo.GetBlob(blobs.ids[0]); ok {
		t.Fatal("the region bitmap of an edit that was never queued should have been deleted")
	}
}

// recordingBlobs remembers the ids it was asked to store.
type recordingBlobs struct {
	store.BlobStore
	ids []string
}

func (r *recordingBlobs) PutBlob(data []byte, contentType string) (string, error) {
	id, err := r.BlobStore.PutBlob(data, contentType)
	if err == nil {
		r.ids = append(r.ids, id)
	}
	return id, err
}
