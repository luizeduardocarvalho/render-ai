package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"render-ai/backend/internal/config"
	"render-ai/backend/internal/jobs"
	renderpkg "render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

// fakeRenderer is a render.Renderer whose per-call outcome is driven by a
// caller-supplied function, keyed by call index (0-based, in call order) so
// tests can make e.g. "the second variation fails" deterministic regardless
// of goroutine scheduling.
type fakeRenderer struct {
	mu    sync.Mutex
	calls int
	fn    func(callIndex int) (renderpkg.RenderResult, error)
}

func (f *fakeRenderer) Render(_ context.Context, _ renderpkg.RenderRequest) (renderpkg.RenderResult, error) {
	f.mu.Lock()
	idx := f.calls
	f.calls++
	f.mu.Unlock()
	return f.fn(idx)
}

func (f *fakeRenderer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// alwaysSucceeds returns a fakeRenderer that succeeds every call with a
// tiny valid PNG.
func alwaysSucceeds(t *testing.T) *fakeRenderer {
	png := testPNG(t)
	return &fakeRenderer{fn: func(int) (renderpkg.RenderResult, error) {
		return renderpkg.RenderResult{ImageData: png, MIMEType: "image/png"}, nil
	}}
}

const renderTestTimeoutSec = 30

// setupRenderTest builds a Server wired to an inline queue, a project with
// one view with a screenshot, and returns everything a render-job test
// needs. Auth is left disabled (zero-value config.AuthConfig), matching how
// the other handler tests in this package call handlers directly.
func setupRenderTest(t *testing.T, renderer renderpkg.Renderer) (*Server, *store.MemoryStore, *store.Project, *store.View, *jobs.Inline) {
	t.Helper()
	return setupRenderTestWithScreenshot(t, renderer, testPNG(t))
}

// setupRenderTestWithScreenshot is setupRenderTest with a chosen screenshot.
func setupRenderTestWithScreenshot(t *testing.T, renderer renderpkg.Renderer, png []byte) (*Server, *store.MemoryStore, *store.Project, *store.View, *jobs.Inline) {
	t.Helper()
	repo := store.NewMemory()
	p := repo.CreateProject("owner-1", "P")

	cfg, _, err := decodeImageConfig(png)
	if err != nil {
		t.Fatalf("decoding test png: %v", err)
	}
	imgID, err := repo.PutBlob(png, "image/png")
	if err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	view, err := repo.CreateView(p.ID, "front", imgID, cfg.Width, cfg.Height)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}

	queue := jobs.NewInline()
	srvCfg := &config.Config{
		Server: config.ServerConfig{RenderTimeoutSec: renderTestTimeoutSec},
		Jobs:   config.JobsConfig{Queue: "inline"},
	}
	s := NewServer(repo, repo, renderer, nil, srvCfg, "../../prompts/render.tmpl", queue)
	queue.SetHandler(s.RunRenderVariation)
	return s, repo, p, view, queue
}

// postStartRender invokes startRender the way Router() would (path values
// set, JSON body), returning the parsed 202 response.
func postStartRender(t *testing.T, s *Server, pid, vid string, body renderRequestBody) (*renderJobResponse, int) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshaling request: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	r.SetPathValue("pid", pid)
	r.SetPathValue("vid", vid)
	w := httptest.NewRecorder()

	err = s.startRender(w, r)
	if err != nil {
		status, _ := statusAndMessage(err)
		return nil, status
	}
	var resp renderJobResponse
	if decErr := json.NewDecoder(w.Body).Decode(&resp); decErr != nil {
		t.Fatalf("decoding response: %v", decErr)
	}
	return &resp, w.Code
}

func getRenderJobResponse(t *testing.T, s *Server, pid, jid string) (*renderJobResponse, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("pid", pid)
	r.SetPathValue("jid", jid)
	w := httptest.NewRecorder()
	if err := s.getRenderJob(w, r); err != nil {
		return nil, err
	}
	var resp renderJobResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return &resp, nil
}

// TestStartRenderReturns202AndPollsToDone covers the full happy path: a 202
// with a queued job, and once the inline queue's goroutines finish, GET
// render-jobs reports "done" with every variation's Render attached.
func TestStartRenderReturns202AndPollsToDone(t *testing.T) {
	s, _, p, v, queue := setupRenderTest(t, alwaysSucceeds(t))

	resp, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 2})
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	if resp.Status != store.RenderJobQueued && resp.Status != store.RenderJobRunning {
		t.Fatalf("initial status = %q, want queued or running", resp.Status)
	}
	if len(resp.Variations) != 2 {
		t.Fatalf("got %d variations, want 2", len(resp.Variations))
	}

	queue.Wait()

	final, err := getRenderJobResponse(t, s, p.ID, resp.ID)
	if err != nil {
		t.Fatalf("getRenderJob: %v", err)
	}
	if final.Status != store.RenderJobDone {
		t.Fatalf("final status = %q, want done", final.Status)
	}
	if len(final.Renders) != 2 {
		t.Fatalf("got %d renders, want 2", len(final.Renders))
	}
	for i, v := range final.Variations {
		if v.Status != store.RenderJobDone || v.RenderID == nil {
			t.Fatalf("variation %d = %+v, want done with a renderId", i, v)
		}
	}
	if final.Error != nil {
		t.Fatalf("Error = %v, want nil on success", *final.Error)
	}
}

// TestStartRenderPartialFailureIsDone covers one variation failing after
// another succeeds: the job is still "done" (partial success), the failed
// variation carries its own error, and the top-level error stays unset.
func TestStartRenderPartialFailureIsDone(t *testing.T) {
	renderer := &fakeRenderer{fn: func(idx int) (renderpkg.RenderResult, error) {
		if idx == 1 {
			return renderpkg.RenderResult{}, errors.New("simulated model failure")
		}
		return renderpkg.RenderResult{ImageData: testPNGBytes, MIMEType: "image/png"}, nil
	}}
	s, _, p, v, queue := setupRenderTest(t, renderer)

	resp, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 2})
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	queue.Wait()

	final, err := getRenderJobResponse(t, s, p.ID, resp.ID)
	if err != nil {
		t.Fatalf("getRenderJob: %v", err)
	}
	if final.Status != store.RenderJobDone {
		t.Fatalf("status = %q, want done (partial success)", final.Status)
	}
	if len(final.Renders) != 1 {
		t.Fatalf("got %d renders, want 1", len(final.Renders))
	}
	if final.Error != nil {
		t.Fatalf("Error = %v, want nil - only set when the whole job fails", *final.Error)
	}
	doneCount, failedCount := 0, 0
	for _, v := range final.Variations {
		switch v.Status {
		case store.RenderJobDone:
			doneCount++
		case store.RenderJobFailed:
			failedCount++
			if v.Error == nil {
				t.Fatal("failed variation has no error message")
			}
		}
	}
	if doneCount != 1 || failedCount != 1 {
		t.Fatalf("done=%d failed=%d, want 1 and 1", doneCount, failedCount)
	}
}

// TestStartRenderAllVariationsFailIsFailed covers every variation failing:
// the job is "failed" and its top-level error is the first variation's
// error.
func TestStartRenderAllVariationsFailIsFailed(t *testing.T) {
	renderer := &fakeRenderer{fn: func(idx int) (renderpkg.RenderResult, error) {
		return renderpkg.RenderResult{}, fmt.Errorf("simulated failure %d", idx)
	}}
	s, _, p, v, queue := setupRenderTest(t, renderer)

	resp, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 2})
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	queue.Wait()

	final, err := getRenderJobResponse(t, s, p.ID, resp.ID)
	if err != nil {
		t.Fatalf("getRenderJob: %v", err)
	}
	if final.Status != store.RenderJobFailed {
		t.Fatalf("status = %q, want failed", final.Status)
	}
	if len(final.Renders) != 0 {
		t.Fatalf("got %d renders, want 0", len(final.Renders))
	}
	if final.Error == nil {
		t.Fatal("Error is nil, want the first variation's error")
	}
}

// TestRunRenderVariationIdempotentRedelivery covers a re-delivered Cloud
// Tasks request for a variation that's already done: the model must not be
// called a second time, and the render/job state is unchanged.
func TestRunRenderVariationIdempotentRedelivery(t *testing.T) {
	renderer := alwaysSucceeds(t)
	s, repo, p, v, queue := setupRenderTest(t, renderer)

	resp, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 1})
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	queue.Wait()

	if got := renderer.callCount(); got != 1 {
		t.Fatalf("renderer called %d times, want 1", got)
	}

	before, err := repo.GetRenderJob(p.ID, resp.ID)
	if err != nil {
		t.Fatalf("GetRenderJob: %v", err)
	}

	// Simulate Cloud Tasks redelivering the same task after the first
	// delivery already completed it.
	s.RunRenderVariation(context.Background(), jobs.Task{ProjectID: p.ID, JobID: resp.ID, Variation: 0})

	if got := renderer.callCount(); got != 1 {
		t.Fatalf("renderer called %d times after redelivery, want still 1 (idempotent no-op)", got)
	}
	after, err := repo.GetRenderJob(p.ID, resp.ID)
	if err != nil {
		t.Fatalf("GetRenderJob: %v", err)
	}
	if *after.Variations[0].RenderID != *before.Variations[0].RenderID {
		t.Fatalf("redelivery changed the recorded renderId: %s -> %s", *before.Variations[0].RenderID, *after.Variations[0].RenderID)
	}
}

// TestVerifyWorkerRequestOIDC covers the worker route's auth gate: a
// missing token, a validator error, an unverified/mismatched email, and a
// valid token, using an injected OIDCValidator so the test needs no real
// Google credentials or network access.
func TestVerifyWorkerRequestOIDC(t *testing.T) {
	s, _, _, _, _ := setupRenderTest(t, alwaysSucceeds(t))
	s.cfg.Jobs.WorkerAudience = "https://worker.example.com"
	s.cfg.Jobs.InvokerServiceAccount = "render-tasks-invoker@my-project.iam.gserviceaccount.com"

	newReq := func(token string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/internal/render-tasks", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		return r
	}

	t.Run("missing token", func(t *testing.T) {
		s.oidcValidator = func(context.Context, string, string) (oidcClaims, error) {
			t.Fatal("validator must not be called with no bearer token")
			return oidcClaims{}, nil
		}
		err := s.verifyWorkerRequest(newReq(""))
		assertUnauthorized(t, err)
	})

	t.Run("validator rejects the token", func(t *testing.T) {
		s.oidcValidator = func(context.Context, string, string) (oidcClaims, error) {
			return oidcClaims{}, errors.New("simulated: bad signature")
		}
		err := s.verifyWorkerRequest(newReq("bad-token"))
		assertUnauthorized(t, err)
	})

	t.Run("email not verified", func(t *testing.T) {
		s.oidcValidator = func(_ context.Context, _, audience string) (oidcClaims, error) {
			if audience != s.cfg.Jobs.WorkerAudience {
				t.Fatalf("audience = %q, want %q", audience, s.cfg.Jobs.WorkerAudience)
			}
			return oidcClaims{Email: s.cfg.Jobs.InvokerServiceAccount, EmailVerified: false}, nil
		}
		err := s.verifyWorkerRequest(newReq("token"))
		assertUnauthorized(t, err)
	})

	t.Run("wrong service account", func(t *testing.T) {
		s.oidcValidator = func(context.Context, string, string) (oidcClaims, error) {
			return oidcClaims{Email: "someone-else@my-project.iam.gserviceaccount.com", EmailVerified: true}, nil
		}
		err := s.verifyWorkerRequest(newReq("token"))
		assertUnauthorized(t, err)
	})

	t.Run("valid token passes", func(t *testing.T) {
		s.oidcValidator = func(context.Context, string, string) (oidcClaims, error) {
			return oidcClaims{Email: s.cfg.Jobs.InvokerServiceAccount, EmailVerified: true}, nil
		}
		if err := s.verifyWorkerRequest(newReq("token")); err != nil {
			t.Fatalf("verifyWorkerRequest: unexpected error: %v", err)
		}
	})
}

// TestHandleRenderTaskAlwaysRespondsOKOnceAuthed covers the worker route's
// contract end to end: once the OIDC token checks out and the body parses,
// it always answers 2xx - even for a variation whose render fails - and a
// redelivered request for an already-done variation is also a 200 no-op.
func TestHandleRenderTaskAlwaysRespondsOKOnceAuthed(t *testing.T) {
	renderer := &fakeRenderer{fn: func(int) (renderpkg.RenderResult, error) {
		return renderpkg.RenderResult{}, errors.New("simulated model failure")
	}}
	s, repo, p, v, queue := setupRenderTest(t, renderer)
	s.cfg.Jobs.WorkerAudience = "https://worker.example.com"
	s.cfg.Jobs.InvokerServiceAccount = "render-tasks-invoker@my-project.iam.gserviceaccount.com"
	s.oidcValidator = func(context.Context, string, string) (oidcClaims, error) {
		return oidcClaims{Email: s.cfg.Jobs.InvokerServiceAccount, EmailVerified: true}, nil
	}

	resp, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 1})
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	// Let the inline queue's own goroutine (started inside postStartRender)
	// run the task to completion before simulating a redelivery below -
	// otherwise the manual call below races the automatic one instead of
	// exercising a genuine "already terminal" redelivery.
	queue.Wait()

	job, err := repo.GetRenderJob(p.ID, resp.ID)
	if err != nil {
		t.Fatalf("GetRenderJob: %v", err)
	}
	if job.Variations[0].Status != store.RenderJobFailed {
		t.Fatalf("variation status = %q, want failed", job.Variations[0].Status)
	}
	if got := renderer.callCount(); got != 1 {
		t.Fatalf("renderer called %d times, want 1", got)
	}

	// Cloud Tasks redelivers (e.g. it never saw the 200): the variation is
	// already terminal, so this must still be a 200 no-op, not a second
	// model call or a changed error.
	body, err := json.Marshal(renderTaskBody{ProjectID: p.ID, JobID: resp.ID, Variation: 0})
	if err != nil {
		t.Fatalf("marshaling task body: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/internal/render-tasks", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer fake-token")
	w := httptest.NewRecorder()
	if handlerErr := s.handleRenderTask(w, r); handlerErr != nil {
		t.Fatalf("handleRenderTask returned an error (want none - failures are recorded on the job): %v", handlerErr)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("redelivery: status = %d, want 200", w.Code)
	}
	if got := renderer.callCount(); got != 1 {
		t.Fatalf("renderer called %d times after redelivery, want still 1 (idempotent no-op)", got)
	}
}

func assertUnauthorized(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	status, _ := statusAndMessage(err)
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

// testPNGBytes is a small package-level valid PNG for tests that build a
// fakeRenderer outside of a *testing.T-scoped helper.
var testPNGBytes = func() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}()
