package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	renderpkg "render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

func postStartUpscale(t *testing.T, s *Server, pid, vid, rid string) (*renderJobResponse, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.SetPathValue("pid", pid)
	r.SetPathValue("vid", vid)
	r.SetPathValue("rid", rid)
	w := httptest.NewRecorder()
	if err := s.startUpscale(w, r); err != nil {
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

// redrawnPNG is src as a model might answer an upscale: n times the size, with
// the colours pushed (redder, less blue) the way the real one shifts them.
func redrawnPNG(t *testing.T, src []byte, n int) []byte {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx()*n, b.Dy()*n))
	for y := 0; y < b.Dy()*n; y++ {
		for x := 0; x < b.Dx()*n; x++ {
			c := color.NRGBAModel.Convert(img.At(x/n, y/n)).(color.NRGBA)
			out.SetNRGBA(x, y, color.NRGBA{R: uint8(min(int(c.R)+30, 255)), G: c.G, B: uint8(max(int(c.B)-20, 0)), A: 255})
		}
	}
	return encodePNG(t, out)
}

func meanChannels(t *testing.T, data []byte) (r, g, b float64) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()
	n := float64(bounds.Dx() * bounds.Dy())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			r += float64(c.R)
			g += float64(c.G)
			b += float64(c.B)
		}
	}
	return r / n, g / n, b / n
}

// TestStartUpscaleProducesLinked4KRenderInTheSourcesColours is the whole
// feature end to end: an upscale job runs through the queue, the model is asked
// to reproduce the source at 4K, and the stored result is a new, larger Render
// linked to its source whose colours are the source's own.
func TestStartUpscaleProducesLinked4KRenderInTheSourcesColours(t *testing.T) {
	// The model answers at twice the source's size, with its colours shifted.
	renderer := &recordingRenderer{}
	s, repo, p, v, queue := setupRenderTest(t, renderer)
	source, sourceData := addSourceRender(t, repo, p.ID, v.ID)
	renderer.answer = redrawnPNG(t, sourceData, 2)

	resp, err := postStartUpscale(t, s, p.ID, v.ID, source.ID)
	if err != nil {
		t.Fatalf("startUpscale: %v", err)
	}
	if resp.Request.Model != store.ModelPro || resp.Request.Resolution != store.Resolution4K || resp.Request.Variations != 1 {
		t.Errorf("request = %+v, want pro/4K with one variation", resp.Request)
	}
	queue.Wait()

	final, err := getRenderJobResponse(t, s, p.ID, resp.ID)
	if err != nil {
		t.Fatalf("getRenderJob: %v", err)
	}
	if final.Status != store.RenderJobDone || len(final.Renders) != 1 {
		t.Fatalf("job = %s with %d renders (error %v), want done with one", final.Status, len(final.Renders), final.Error)
	}
	up := final.Renders[0]
	if up.UpscaledFromRenderID != source.ID || up.SourceRenderID != "" || len(up.EditInstructions) != 0 {
		t.Errorf("links = upscaledFrom %q, source %q, instructions %q; want it linked to %q only as an upscale",
			up.UpscaledFromRenderID, up.SourceRenderID, up.EditInstructions, source.ID)
	}
	if up.Model != store.ModelPro || up.Resolution != store.Resolution4K || up.RegionCount != 0 || up.Preservation != nil || up.Metrics.Attempts != 1 {
		t.Errorf("render = %+v, want pro/4K, no regions, no preservation check, one attempt", up)
	}

	// What the model was asked: the source, alone, at 4K, with the upscale prompt.
	if len(renderer.requests) != 1 {
		t.Fatalf("model called %d times, want 1", len(renderer.requests))
	}
	req := renderer.requests[0]
	if len(req.Images) != 1 || !bytes.Equal(req.Images[0], sourceData) {
		t.Errorf("model got %d images (source = %v), want just the source", len(req.Images), len(req.Images) > 0 && bytes.Equal(req.Images[0], sourceData))
	}
	if req.ImageSize != "4K" || req.AspectRatio != "16:9" {
		t.Errorf("ImageSize/AspectRatio = %q/%q, want 4K and the source's 16:9", req.ImageSize, req.AspectRatio)
	}
	if !strings.Contains(req.Prompt, "Reproduce IMAGE 1 exactly") {
		t.Errorf("prompt is not the upscale prompt:\n%s", req.Prompt)
	}

	// What was stored: twice the size, in the source's colours rather than the
	// model's.
	blob, ok := s.blobs.GetBlob(up.ResultImageID)
	if !ok {
		t.Fatal("upscale result blob is missing")
	}
	if cfg, _, err := decodeImageConfig(blob.Data); err != nil || cfg.Width != editW*2 || cfg.Height != editH*2 {
		t.Fatalf("upscale is %+v (%v), want %dx%d", cfg, err, editW*2, editH*2)
	}
	sr, sg, sb := meanChannels(t, sourceData)
	mr, mg, mb := meanChannels(t, renderer.answer)
	gr, gg, gb := meanChannels(t, blob.Data)
	if mr-sr < 20 {
		t.Fatalf("test setup: the model's answer is only %.1f redder than the source", mr-sr)
	}
	for _, c := range []struct {
		name          string
		got, src, raw float64
	}{{"red", gr, sr, mr}, {"green", gg, sg, mg}, {"blue", gb, sb, mb}} {
		if d := c.got - c.src; d > 3 || d < -3 {
			t.Errorf("mean %s = %.1f, want the source's %.1f (the model's own was %.1f)", c.name, c.got, c.src, c.raw)
		}
	}

	// The source is left as it was.
	if again, ok := s.blobs.GetBlob(source.ResultImageID); !ok || !bytes.Equal(again.Data, sourceData) {
		t.Error("the source render's image was changed")
	}
}

func TestStartUpscaleRejectsBadRequests(t *testing.T) {
	renderer := alwaysSucceeds(t)
	s, repo, p, v, queue := setupRenderTest(t, renderer)
	source, sourceData := addSourceRender(t, repo, p.ID, v.ID)

	id, err := repo.PutBlob(sourceData, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	already4K, err := repo.AddRender(p.ID, v.ID, &store.Render{
		ID: "render-4k", Model: store.ModelPro, Resolution: store.Resolution4K, ResultImageID: id,
	})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, vid, rid string
		want           int
	}{
		{"unknown render", v.ID, "nope", http.StatusNotFound},
		{"unknown view", "nope", source.ID, http.StatusNotFound},
		{"already 4K", v.ID, already4K.ID, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := postStartUpscale(t, s, p.ID, tc.vid, tc.rid)
			wantStatus(t, err, tc.want)
		})
	}

	_, err = postStartUpscale(t, s, "no-such-project", v.ID, source.ID)
	wantStatus(t, err, http.StatusNotFound)

	// A rejected request must never reach the queue, let alone the model.
	queue.Wait()
	if n := renderer.callCount(); n != 0 {
		t.Fatalf("the model was called %d times for rejected requests, want 0", n)
	}
}

// An upscale is a 4K pro render whatever the source was made with, so it is
// charged as one.
func TestStartUpscaleChargesThe4KPriceAndRefundsFailure(t *testing.T) {
	failing := &fakeRenderer{fn: func(int) (renderpkg.RenderResult, error) {
		return renderpkg.RenderResult{}, context.DeadlineExceeded
	}}
	s, repo, p, v, queue := setupRenderTest(t, failing)
	source, _ := addSourceRender(t, repo, p.ID, v.ID)
	price := unitsPerVariation(store.ModelPro, store.Resolution4K)
	enableAuthAndGrant(t, s, repo, p.OwnerID, price)

	resp, err := postStartUpscale(t, s, p.ID, v.ID, source.ID)
	if err != nil {
		t.Fatalf("startUpscale: %v", err)
	}
	if resp.CreditsCharged != unitsToCredits(price) || resp.CreditsCharged != 2 {
		t.Errorf("CreditsCharged = %v, want the pro 4K price of 2 credits", resp.CreditsCharged)
	}
	if units, err := repo.GetCredits(p.OwnerID); err != nil || units != 0 {
		t.Fatalf("balance right after starting = %d, %v, want 0 (the whole price debited)", units, err)
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
		t.Fatalf("balance after a failed upscale = %d, %v, want the %d units refunded", units, err, price)
	}

	// One credit short of the price: 402, and nothing is stored.
	if _, err := repo.AdjustCredits(p.OwnerID, -price+unitsPerCredit, store.CreditLedgerEntry{Reason: store.CreditReasonRender}); err != nil {
		t.Fatal(err)
	}
	_, err = postStartUpscale(t, s, p.ID, v.ID, source.ID)
	wantStatus(t, err, http.StatusPaymentRequired)
}

// The model answering with anything but a larger version of the source must
// not put a render in the history: the user is refunded and the job fails.
func TestUpscaleFailsAndRefundsWhenTheModelDoesNotReturnALargerImage(t *testing.T) {
	cases := []struct {
		name   string
		answer func(t *testing.T, source []byte) []byte
	}{
		{"same size", func(t *testing.T, source []byte) []byte { return redrawnPNG(t, source, 1) }},
		{"smaller", func(t *testing.T, source []byte) []byte { return solidPNG(t, editW/2, editH/2, color.NRGBA{A: 255}) }},
		{"another aspect ratio", func(t *testing.T, source []byte) []byte { return solidPNG(t, editW*2, editW*2, color.NRGBA{A: 255}) }},
		{"not an image", func(t *testing.T, source []byte) []byte { return []byte("nope") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			renderer := &recordingRenderer{}
			s, repo, p, v, queue := setupRenderTest(t, renderer)
			source, sourceData := addSourceRender(t, repo, p.ID, v.ID)
			renderer.answer = tc.answer(t, sourceData)
			price := unitsPerVariation(store.ModelPro, store.Resolution4K)
			enableAuthAndGrant(t, s, repo, p.OwnerID, price)

			resp, err := postStartUpscale(t, s, p.ID, v.ID, source.ID)
			if err != nil {
				t.Fatalf("startUpscale: %v", err)
			}
			queue.Wait()
			final, err := getRenderJobResponse(t, s, p.ID, resp.ID)
			if err != nil {
				t.Fatal(err)
			}
			if final.Status != store.RenderJobFailed || len(final.Renders) != 0 {
				t.Fatalf("job = %s with %d renders, want failed with none", final.Status, len(final.Renders))
			}
			if final.Error == nil || !strings.Contains(*final.Error, "upscale failed") {
				t.Errorf("error = %v, want it to say the upscale failed", final.Error)
			}
			if units, err := repo.GetCredits(p.OwnerID); err != nil || units != price {
				t.Errorf("balance = %d, %v, want the %d units refunded", units, err, price)
			}
			project, err := repo.GetProject(p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(findViewIn(project, v.ID).Renders); got != 1 {
				t.Errorf("the view has %d renders, want only the source", got)
			}
		})
	}
}
