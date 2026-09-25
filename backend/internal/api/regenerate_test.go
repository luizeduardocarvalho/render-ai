package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"render-ai/backend/internal/jobs"
	renderpkg "render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

// stripesPNG is a 64x64 image of vertical black/white stripes, painted only
// where keep(x) says so (black elsewhere). Its edge map is what the tests'
// "screenshot" and "renders" differ in: the same stripes everywhere score 1
// against the screenshot, none score 0, and a half score in between.
func stripesPNG(t *testing.T, keep func(x int) bool) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := color.RGBA{A: 255}
			if keep(x) && (x/8)%2 == 0 {
				c = color.RGBA{R: 255, G: 255, B: 255, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding stripes: %v", err)
	}
	return buf.Bytes()
}

// regenFixture is a render setup whose screenshot is fully striped, with the
// three kinds of model answer a test can hand out: a match (edge score 1), a
// half match (about 0.5) and a miss (0). The flag threshold is 0.9, so only a
// match is good enough.
type regenFixture struct {
	s                 *Server
	repo              *store.MemoryStore
	pid, vid          string
	queue             *jobs.Inline
	match, half, miss []byte
	renderer          *fakeRenderer
	perCallUsd        float64
}

func newRegenFixture(t *testing.T, maxRegenerations int, answers func(f *regenFixture, call int) ([]byte, error)) *regenFixture {
	t.Helper()
	f := &regenFixture{perCallUsd: 0.1}
	f.match = stripesPNG(t, func(int) bool { return true })
	f.half = stripesPNG(t, func(x int) bool { return x < 32 })
	f.miss = stripesPNG(t, func(int) bool { return false })
	f.renderer = &fakeRenderer{fn: func(call int) (renderpkg.RenderResult, error) {
		data, err := answers(f, call)
		if err != nil {
			return renderpkg.RenderResult{}, err
		}
		return renderpkg.RenderResult{ImageData: data, MIMEType: "image/png"}, nil
	}}
	s, repo, p, v, queue := setupRenderTestWithScreenshot(t, f.renderer, f.match)
	s.cfg.Preservation.EdgeScoreFlagThreshold = 0.9
	s.cfg.Preservation.EdgeDilationPx = 2
	s.cfg.Preservation.MaxRegenerations = maxRegenerations
	s.cfg.Pricing.ProImage.PerImage = map[string]float64{"2K": f.perCallUsd}
	f.s, f.repo, f.pid, f.vid, f.queue = s, repo, p.ID, v.ID, queue
	return f
}

// run starts a one-variation render with the preservation check on, waits for
// the queue to drain, and returns the finished job.
func (f *regenFixture) run(t *testing.T) *renderJobResponse {
	t.Helper()
	resp, status := postStartRender(t, f.s, f.pid, f.vid, renderRequestBody{
		Model: "pro", Resolution: "2K", PreservationCheck: true, Variations: 1,
	})
	if status != 202 {
		t.Fatalf("status = %d, want 202", status)
	}
	f.queue.Wait()
	job, err := getRenderJobResponse(t, f.s, f.pid, resp.ID)
	if err != nil {
		t.Fatalf("getRenderJob: %v", err)
	}
	return job
}

func (f *regenFixture) imageOf(t *testing.T, r *store.Render) []byte {
	t.Helper()
	blob, ok := f.repo.GetBlob(r.ResultImageID)
	if !ok {
		t.Fatalf("render %s has no image blob", r.ID)
	}
	return blob.Data
}

func (f *regenFixture) renderCount(t *testing.T) int {
	t.Helper()
	p, err := f.repo.GetProject(f.pid)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	return len(p.Views[0].Renders)
}

// A render that misses the screenshot is generated again until one matches;
// only the match is kept, and its cost and attempt count cover every try.
func TestFlaggedRenderIsRegeneratedUntilItMatches(t *testing.T) {
	f := newRegenFixture(t, 2, func(f *regenFixture, call int) ([]byte, error) {
		if call < 2 {
			return f.miss, nil
		}
		return f.match, nil
	})
	job := f.run(t)

	if job.Status != store.RenderJobDone || len(job.Renders) != 1 {
		t.Fatalf("job status=%s renders=%d, want done with 1 render", job.Status, len(job.Renders))
	}
	if got := f.renderer.callCount(); got != 3 {
		t.Fatalf("model called %d times, want 3", got)
	}
	r := job.Renders[0]
	if r.Metrics.Attempts != 3 {
		t.Errorf("attempts = %d, want 3", r.Metrics.Attempts)
	}
	if r.Preservation == nil || r.Preservation.EdgeFlag {
		t.Errorf("kept render should pass the check, got %+v", r.Preservation)
	}
	if !bytes.Equal(f.imageOf(t, r), f.match) {
		t.Errorf("kept render is not the matching one")
	}
	if r.Metrics.EstimatedCostUsd == nil || math.Abs(*r.Metrics.EstimatedCostUsd-3*f.perCallUsd) > 1e-9 {
		t.Errorf("cost = %v, want %.2f (3 attempts)", r.Metrics.EstimatedCostUsd, 3*f.perCallUsd)
	}
	if got := f.renderCount(t); got != 1 {
		t.Errorf("view history has %d renders, want 1 (discarded attempts are never shown)", got)
	}
}

// The chain of regenerations is bounded: when every attempt misses, the model
// is called 1+MaxRegenerations times and the best attempt is what is shown.
func TestRegenerationStopsAtTheLimitAndKeepsTheBestAttempt(t *testing.T) {
	f := newRegenFixture(t, 2, func(f *regenFixture, call int) ([]byte, error) {
		if call == 0 {
			return f.half, nil // the best of a bad lot
		}
		return f.miss, nil
	})
	job := f.run(t)

	if got := f.renderer.callCount(); got != 3 {
		t.Fatalf("model called %d times, want exactly 1+2", got)
	}
	if job.Status != store.RenderJobDone || len(job.Renders) != 1 {
		t.Fatalf("job status=%s renders=%d, want done with 1 render", job.Status, len(job.Renders))
	}
	r := job.Renders[0]
	if r.Metrics.Attempts != 3 {
		t.Errorf("attempts = %d, want 3", r.Metrics.Attempts)
	}
	if r.Preservation == nil || !r.Preservation.EdgeFlag {
		t.Errorf("render should still be flagged, got %+v", r.Preservation)
	}
	if !bytes.Equal(f.imageOf(t, r), f.half) {
		t.Errorf("kept render is not the best-scoring attempt")
	}
	if r.Metrics.EstimatedCostUsd == nil || math.Abs(*r.Metrics.EstimatedCostUsd-3*f.perCallUsd) > 1e-9 {
		t.Errorf("cost = %v, want %.2f", r.Metrics.EstimatedCostUsd, 3*f.perCallUsd)
	}
}

func TestNoRegenerationWhenTheFirstRenderMatchesOrTheLimitIsZero(t *testing.T) {
	for name, tc := range map[string]struct {
		max    int
		answer func(f *regenFixture) []byte
	}{
		"matches":    {max: 2, answer: func(f *regenFixture) []byte { return f.match }},
		"limit zero": {max: 0, answer: func(f *regenFixture) []byte { return f.miss }},
	} {
		t.Run(name, func(t *testing.T) {
			f := newRegenFixture(t, tc.max, func(f *regenFixture, _ int) ([]byte, error) { return tc.answer(f), nil })
			job := f.run(t)
			if got := f.renderer.callCount(); got != 1 {
				t.Fatalf("model called %d times, want 1", got)
			}
			if len(job.Renders) != 1 || job.Renders[0].Metrics.Attempts != 1 {
				t.Fatalf("want one render made in 1 attempt, got %+v", job.Renders)
			}
		})
	}
}

// If a regeneration fails outright, the flagged render from before is shown
// rather than nothing.
func TestFailedRegenerationFallsBackToTheHeldRender(t *testing.T) {
	f := newRegenFixture(t, 2, func(f *regenFixture, call int) ([]byte, error) {
		if call == 0 {
			return f.half, nil
		}
		return nil, errors.New("model unavailable")
	})
	job := f.run(t)

	if job.Status != store.RenderJobDone || len(job.Renders) != 1 {
		t.Fatalf("job status=%s renders=%d, want done with 1 render", job.Status, len(job.Renders))
	}
	r := job.Renders[0]
	if !bytes.Equal(f.imageOf(t, r), f.half) {
		t.Errorf("kept render is not the held one")
	}
	if r.Metrics.Attempts != 2 {
		t.Errorf("attempts = %d, want 2 (the failed one counts)", r.Metrics.Attempts)
	}
}

// deliverThroughRoute makes the queue hand every task to the worker route the
// way Cloud Tasks does: as the JSON body of an authenticated POST to
// /internal/render-tasks, not as a Go value. The inline queue skips that hop,
// which is how a field dropped from the route's body went unnoticed.
func (f *regenFixture) deliverThroughRoute(t *testing.T) {
	t.Helper()
	const invoker = "render-tasks-invoker@my-project.iam.gserviceaccount.com"
	f.s.cfg.Jobs.WorkerAudience = "https://worker.example.com"
	f.s.cfg.Jobs.InvokerServiceAccount = invoker
	f.s.oidcValidator = func(context.Context, string, string) (oidcClaims, error) {
		return oidcClaims{Email: invoker, EmailVerified: true}, nil
	}
	f.queue.SetHandler(func(ctx context.Context, task jobs.Task) {
		body, err := json.Marshal(task)
		if err != nil {
			t.Errorf("marshaling task: %v", err)
			return
		}
		r := httptest.NewRequest(http.MethodPost, "/internal/render-tasks", bytes.NewReader(body)).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer fake-token")
		w := httptest.NewRecorder()
		if err := f.s.handleRenderTask(w, r); err != nil {
			t.Errorf("handleRenderTask: %v", err)
			return
		}
		if w.Code != http.StatusOK {
			t.Errorf("render-tasks status = %d, want 200", w.Code)
		}
	})
}

// The regeneration task reaches the worker as a request body, so the route
// must carry its attempt number through: run as attempt 0 it would look like
// a stale redelivery, be ignored with a 200, and leave the variation queued
// until the stale rule reports it failed.
func TestRegenerationTaskDeliveredThroughTheWorkerRouteRuns(t *testing.T) {
	f := newRegenFixture(t, 2, func(f *regenFixture, call int) ([]byte, error) {
		if call < 1 {
			return f.miss, nil
		}
		return f.match, nil
	})
	f.deliverThroughRoute(t)
	job := f.run(t)

	if job.Status != store.RenderJobDone || len(job.Renders) != 1 {
		t.Fatalf("job status=%s renders=%d, want done with 1 render (the regeneration task was not run)", job.Status, len(job.Renders))
	}
	if got := f.renderer.callCount(); got != 2 {
		t.Fatalf("model called %d times, want 2", got)
	}
	if r := job.Renders[0]; r.Metrics.Attempts != 2 || !bytes.Equal(f.imageOf(t, r), f.match) {
		t.Errorf("kept render: attempts=%d, matching image=%v, want attempts=2 and the matching image", r.Metrics.Attempts, bytes.Equal(f.imageOf(t, r), f.match))
	}
}

// A task of an earlier attempt, redelivered by the queue, must not run: only
// the attempt the variation is on can claim it.
func TestRegenerationTaskOfAnotherAttemptIsIgnored(t *testing.T) {
	f := newRegenFixture(t, 2, func(f *regenFixture, _ int) ([]byte, error) { return f.match, nil })
	now := time.Now().UTC()
	job, err := f.repo.CreateRenderJob(f.pid, &store.RenderJob{
		ID: "job-1", ViewID: f.vid, CreatedAt: now, UpdatedAt: now,
		Request:    store.RenderJobRequest{Model: store.ModelPro, Resolution: store.Resolution2K, PreservationCheck: true, Variations: 1},
		Variations: []store.RenderJobVariation{{Status: store.RenderJobQueued, Attempt: 1}},
	})
	if err != nil {
		t.Fatalf("CreateRenderJob: %v", err)
	}

	f.s.RunRenderVariation(context.Background(), jobs.Task{ProjectID: f.pid, JobID: job.ID, Variation: 0, Attempt: 0})
	if got := f.renderer.callCount(); got != 0 {
		t.Fatalf("stale attempt ran the model %d times, want 0", got)
	}
	f.s.RunRenderVariation(context.Background(), jobs.Task{ProjectID: f.pid, JobID: job.ID, Variation: 0, Attempt: 1})
	if got := f.renderer.callCount(); got != 1 {
		t.Fatalf("current attempt ran the model %d times, want 1", got)
	}
}

// The color check reports each asset's region against its reference photo,
// and never flags or regenerates on it.
func TestAssetColorScoresReportPerAssetAndSkipUnreadablePhotos(t *testing.T) {
	encode := func(c color.RGBA) []byte {
		img := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	rust, blue := color.RGBA{R: 180, G: 70, B: 40}, color.RGBA{R: 40, G: 90, B: 190}
	render := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := 0; i < len(render.Pix); i += 4 {
		render.Pix[i], render.Pix[i+1], render.Pix[i+2], render.Pix[i+3] = rust.R, rust.G, rust.B, 255
	}
	whole := image.NewGray(render.Bounds())
	for i := range whole.Pix {
		whole.Pix[i] = 255
	}

	got := assetColorScores(render, []regionAsset{
		{asset: &store.Asset{ID: "a-sofa", Name: "Sofa"}, ref: encode(rust), bitmaps: []image.Image{whole}},
		{asset: &store.Asset{ID: "a-lamp", Name: "Lamp"}, ref: encode(blue), bitmaps: []image.Image{whole}},
		{asset: &store.Asset{ID: "a-broken", Name: "Broken"}, ref: []byte("not an image"), bitmaps: []image.Image{whole}},
	})

	if len(got) != 2 {
		t.Fatalf("scores = %+v, want the sofa and the lamp (the unreadable photo is skipped)", got)
	}
	if got[0].AssetID != "a-sofa" || got[0].AssetName != "Sofa" || got[0].Distance > 1 {
		t.Errorf("sofa score = %+v, want a match", got[0])
	}
	if got[1].AssetID != "a-lamp" || got[1].Distance < 30 {
		t.Errorf("lamp score = %+v, want a clear mismatch", got[1])
	}
}
