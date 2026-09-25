package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"render-ai/backend/internal/store"
)

func notifJob(id, vid string, created time.Time, req store.RenderJobRequest, variations ...store.RenderJobVariation) *store.RenderJob {
	return &store.RenderJob{
		ID:         id,
		ViewID:     vid,
		CreatedAt:  created,
		UpdatedAt:  created,
		Request:    req,
		Variations: variations,
	}
}

func doneVariation(renderID string) store.RenderJobVariation {
	return store.RenderJobVariation{Status: store.RenderJobDone, RenderID: &renderID}
}

func listNotifications(t *testing.T, s *Server, userID string) []notificationView {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), userIDCtxKey, userID))
	w := httptest.NewRecorder()
	if err := s.listMyRenderJobs(w, r); err != nil {
		t.Fatalf("listMyRenderJobs: %v", err)
	}
	var out []notificationView
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return out
}

func postSeen(s *Server, pid, jid string) (int, error) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.SetPathValue("pid", pid)
	r.SetPathValue("jid", jid)
	w := httptest.NewRecorder()
	if err := s.markRenderJobSeen(w, r); err != nil {
		return 0, err
	}
	return w.Code, nil
}

func TestListMyRenderJobsDescribesEachJobNewestFirst(t *testing.T) {
	s, repo, p, v, _ := setupRenderTest(t, alwaysSucceeds(t))
	now := time.Now().UTC()
	failed := "model said no"

	jobs := []*store.RenderJob{
		notifJob("j-render", v.ID, now.Add(-3*time.Minute), store.RenderJobRequest{Model: store.ModelPro, Resolution: store.Resolution2K, Variations: 2},
			doneVariation("r1"), store.RenderJobVariation{Status: store.RenderJobRunning}),
		notifJob("j-edit", v.ID, now.Add(-2*time.Minute),
			store.RenderJobRequest{Model: store.ModelPro, Resolution: store.Resolution2K, Variations: 1, Edit: &store.RenderJobEdit{SourceRenderID: "r1"}},
			doneVariation("r2")),
		notifJob("j-upscale", v.ID, now.Add(-1*time.Minute),
			store.RenderJobRequest{Model: store.ModelPro, Resolution: store.Resolution4K, Variations: 1, Upscale: &store.RenderJobUpscale{SourceRenderID: "r2"}},
			store.RenderJobVariation{Status: store.RenderJobFailed, Error: &failed}),
	}
	for _, j := range jobs {
		if _, err := repo.CreateRenderJob(p.ID, j); err != nil {
			t.Fatalf("CreateRenderJob: %v", err)
		}
	}

	got := listNotifications(t, s, "owner-1")
	if len(got) != 3 {
		t.Fatalf("got %d notifications, want 3: %+v", len(got), got)
	}
	if got[0].JobID != "j-upscale" || got[1].JobID != "j-edit" || got[2].JobID != "j-render" {
		t.Errorf("order = %s, %s, %s; want newest created first", got[0].JobID, got[1].JobID, got[2].JobID)
	}

	up := got[0]
	if up.Kind != "upscale" || up.Status != store.RenderJobFailed || up.Error == nil || *up.Error != failed || up.RenderID != nil {
		t.Errorf("upscale = %+v, want a failed upscale carrying its error and no render", up)
	}
	edit := got[1]
	if edit.Kind != "edit" || edit.Status != store.RenderJobDone || edit.RenderID == nil || *edit.RenderID != "r2" || edit.Error != nil {
		t.Errorf("edit = %+v, want a done edit pointing at r2", edit)
	}
	render := got[2]
	if render.Kind != "render" || render.Status != store.RenderJobRunning ||
		render.Variations != (notificationVariations{Done: 1, Failed: 0, Total: 2}) || render.RenderID == nil || *render.RenderID != "r1" {
		t.Errorf("render = %+v, want a running render with 1 of 2 variations done and r1 already available", render)
	}
	if render.ProjectID != p.ID || render.ProjectName != "P" || render.ViewID != v.ID || render.ViewName != "front" || render.Seen {
		t.Errorf("render names = %+v, want project P / view front, unseen", render)
	}
}

func TestListMyRenderJobsOnlyListsTheCallersRecentJobs(t *testing.T) {
	s, repo, p, v, _ := setupRenderTest(t, alwaysSucceeds(t))
	now := time.Now().UTC()
	req := store.RenderJobRequest{Model: store.ModelPro, Resolution: store.Resolution2K, Variations: 1}

	fresh := notifJob("j-fresh", v.ID, now.Add(-time.Hour), req, doneVariation("r1"))
	old := notifJob("j-old", v.ID, now.Add(-notificationWindow-time.Hour), req, doneVariation("r0"))
	for _, j := range []*store.RenderJob{fresh, old} {
		if _, err := repo.CreateRenderJob(p.ID, j); err != nil {
			t.Fatalf("CreateRenderJob: %v", err)
		}
	}

	// Another user's project, with a job of its own.
	other := repo.CreateProject("owner-2", "Other")
	ov, err := repo.CreateView(other.ID, "back", "img", 10, 10)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	if _, err := repo.CreateRenderJob(other.ID, notifJob("j-other", ov.ID, now, req, doneVariation("r9"))); err != nil {
		t.Fatalf("CreateRenderJob: %v", err)
	}

	got := listNotifications(t, s, "owner-1")
	if len(got) != 1 || got[0].JobID != "j-fresh" {
		t.Fatalf("owner-1 sees %+v, want only j-fresh (not the day-old job, not owner-2's)", got)
	}
	if got := listNotifications(t, s, "nobody"); len(got) != 0 {
		t.Errorf("a user with no projects sees %+v, want an empty list", got)
	}
}

func TestListMyRenderJobsSkipsDeletedViewsAndProjects(t *testing.T) {
	s, repo, p, v, _ := setupRenderTest(t, alwaysSucceeds(t))
	now := time.Now().UTC()
	req := store.RenderJobRequest{Model: store.ModelPro, Resolution: store.Resolution2K, Variations: 1}

	gone, err := repo.CreateView(p.ID, "gone", "img", 10, 10)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	for _, j := range []*store.RenderJob{
		notifJob("j-kept", v.ID, now.Add(-time.Minute), req, doneVariation("r1")),
		notifJob("j-gone", gone.ID, now, req, doneVariation("r2")),
	} {
		if _, err := repo.CreateRenderJob(p.ID, j); err != nil {
			t.Fatalf("CreateRenderJob: %v", err)
		}
	}
	if _, err := repo.DeleteView(p.ID, gone.ID); err != nil {
		t.Fatalf("DeleteView: %v", err)
	}

	got := listNotifications(t, s, "owner-1")
	if len(got) != 1 || got[0].JobID != "j-kept" {
		t.Fatalf("got %+v, want only the job of the surviving view", got)
	}

	if err := repo.DeleteProject(p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if got := listNotifications(t, s, "owner-1"); len(got) != 0 {
		t.Errorf("after deleting the project got %+v, want none", got)
	}
}

func TestMarkRenderJobSeen(t *testing.T) {
	s, repo, p, v, _ := setupRenderTest(t, alwaysSucceeds(t))
	now := time.Now().UTC().Add(-time.Minute)
	req := store.RenderJobRequest{Model: store.ModelPro, Resolution: store.Resolution2K, Variations: 1}
	for _, j := range []*store.RenderJob{
		notifJob("j-done", v.ID, now, req, doneVariation("r1")),
		notifJob("j-running", v.ID, now, req, store.RenderJobVariation{Status: store.RenderJobRunning}),
	} {
		if _, err := repo.CreateRenderJob(p.ID, j); err != nil {
			t.Fatalf("CreateRenderJob: %v", err)
		}
	}

	seen := func(jid string) bool {
		for _, n := range listNotifications(t, s, "owner-1") {
			if n.JobID == jid {
				return n.Seen
			}
		}
		t.Fatalf("job %s not listed", jid)
		return false
	}

	if code, err := postSeen(s, p.ID, "j-done"); err != nil || code != http.StatusNoContent {
		t.Fatalf("seen = %d, %v; want 204", code, err)
	}
	if !seen("j-done") {
		t.Error("j-done is not seen after marking it")
	}

	// A second call is a no-op: it succeeds and does not touch the job.
	before, _ := repo.GetRenderJob(p.ID, "j-done")
	if code, err := postSeen(s, p.ID, "j-done"); err != nil || code != http.StatusNoContent {
		t.Fatalf("second seen = %d, %v; want 204", code, err)
	}
	after, _ := repo.GetRenderJob(p.ID, "j-done")
	if !after.UpdatedAt.Equal(before.UpdatedAt) || !after.SeenAt.Equal(*before.SeenAt) {
		t.Errorf("marking twice changed the job: %+v -> %+v", before, after)
	}

	// A job still running has no outcome to have seen yet.
	if code, err := postSeen(s, p.ID, "j-running"); err != nil || code != http.StatusNoContent {
		t.Fatalf("seen on running job = %d, %v; want 204", code, err)
	}
	if seen("j-running") {
		t.Error("a running job was marked seen, so its result would arrive already read")
	}

	if _, err := postSeen(s, p.ID, "nope"); err == nil {
		t.Error("unknown job: want an error")
	} else if status, _ := statusAndMessage(err); status != http.StatusNotFound {
		t.Errorf("unknown job status = %d, want 404", status)
	}
}
