package store

import (
	"testing"
	"time"
)

func newTestJob(id, vid string) *RenderJob {
	return &RenderJob{
		ID:     id,
		ViewID: vid,
		Request: RenderJobRequest{
			Model:      ModelPro,
			Resolution: Resolution2K,
			Variations: 2,
		},
		Variations: []RenderJobVariation{
			{Status: RenderJobQueued},
			{Status: RenderJobQueued},
		},
	}
}

// TestMemoryStoreRenderJobCRUD covers create/get and confirms GetRenderJob
// returns a deep copy (mutating it must not affect the store).
func TestMemoryStoreRenderJobCRUD(t *testing.T) {
	s := NewMemory()
	p := s.CreateProject("user-a", "P")
	v, err := s.CreateView(p.ID, "front", "shot-1", 100, 100)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}

	job := newTestJob("job-1", v.ID)
	created, err := s.CreateRenderJob(p.ID, job)
	if err != nil {
		t.Fatalf("CreateRenderJob: %v", err)
	}
	if created.ID != "job-1" || created.ViewID != v.ID || len(created.Variations) != 2 {
		t.Fatalf("unexpected created job: %+v", created)
	}

	got, err := s.GetRenderJob(p.ID, "job-1")
	if err != nil {
		t.Fatalf("GetRenderJob: %v", err)
	}
	if got.Request.Variations != 2 || got.Variations[0].Status != RenderJobQueued {
		t.Fatalf("unexpected fetched job: %+v", got)
	}

	// Mutating the fetched copy must not affect the store.
	got.Variations[0].Status = RenderJobDone
	got2, err := s.GetRenderJob(p.ID, "job-1")
	if err != nil {
		t.Fatalf("GetRenderJob (2nd): %v", err)
	}
	if got2.Variations[0].Status != RenderJobQueued {
		t.Fatalf("GetRenderJob leaked a mutation: got status %q", got2.Variations[0].Status)
	}
}

// TestMemoryStoreGetRenderJobMissing covers a job id that was never created.
func TestMemoryStoreGetRenderJobMissing(t *testing.T) {
	s := NewMemory()
	p := s.CreateProject("user-a", "P")
	if _, err := s.GetRenderJob(p.ID, "does-not-exist"); err != ErrNotFound {
		t.Errorf("GetRenderJob on missing job: want ErrNotFound, got %v", err)
	}
}

// TestMemoryStoreRenderJobMissingProject covers every render-job method
// returning ErrNotFound for a project that never existed.
func TestMemoryStoreRenderJobMissingProject(t *testing.T) {
	s := NewMemory()
	job := newTestJob("job-1", "view-1")
	if _, err := s.CreateRenderJob("no-such-project", job); err != ErrNotFound {
		t.Errorf("CreateRenderJob: want ErrNotFound, got %v", err)
	}
	if _, err := s.GetRenderJob("no-such-project", "job-1"); err != ErrNotFound {
		t.Errorf("GetRenderJob: want ErrNotFound, got %v", err)
	}
	if _, err := s.UpdateRenderJob("no-such-project", "job-1", func(j *RenderJob) error { return nil }); err != ErrNotFound {
		t.Errorf("UpdateRenderJob: want ErrNotFound, got %v", err)
	}
}

// TestMemoryStoreRenderJobSoftDeletedProject confirms a soft-deleted project
// hides its render jobs exactly like every other Repository method (see
// TestMemoryStoreSoftDelete).
func TestMemoryStoreRenderJobSoftDeletedProject(t *testing.T) {
	s := NewMemory()
	p := s.CreateProject("user-a", "P")
	v, err := s.CreateView(p.ID, "front", "shot-1", 100, 100)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	job := newTestJob("job-1", v.ID)
	if _, err := s.CreateRenderJob(p.ID, job); err != nil {
		t.Fatalf("CreateRenderJob: %v", err)
	}
	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := s.GetRenderJob(p.ID, "job-1"); err != ErrNotFound {
		t.Errorf("GetRenderJob on soft-deleted project: want ErrNotFound, got %v", err)
	}
}

// TestMemoryStoreUpdateRenderJobAtomic covers the read-modify-write contract:
// fn's mutation is applied and UpdatedAt bumped, and a returned error leaves
// the job untouched.
func TestMemoryStoreUpdateRenderJobAtomic(t *testing.T) {
	s := NewMemory()
	p := s.CreateProject("user-a", "P")
	v, err := s.CreateView(p.ID, "front", "shot-1", 100, 100)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	job := newTestJob("job-1", v.ID)
	created, err := s.CreateRenderJob(p.ID, job)
	if err != nil {
		t.Fatalf("CreateRenderJob: %v", err)
	}
	firstUpdatedAt := created.UpdatedAt

	updated, err := s.UpdateRenderJob(p.ID, "job-1", func(j *RenderJob) error {
		j.Variations[0].Status = RenderJobRunning
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateRenderJob: %v", err)
	}
	if updated.Variations[0].Status != RenderJobRunning {
		t.Fatalf("variation 0 status = %q, want running", updated.Variations[0].Status)
	}
	if updated.Variations[1].Status != RenderJobQueued {
		t.Fatalf("variation 1 status = %q, want unaffected queued", updated.Variations[1].Status)
	}
	if !updated.UpdatedAt.After(firstUpdatedAt) && updated.UpdatedAt != firstUpdatedAt {
		// UpdatedAt should be >= the original (time resolution may collide in
		// a fast test run); it must never go backwards.
		t.Fatalf("UpdatedAt did not advance: %v -> %v", firstUpdatedAt, updated.UpdatedAt)
	}

	errBoom := &boomErr{}
	if _, err := s.UpdateRenderJob(p.ID, "job-1", func(j *RenderJob) error {
		j.Variations[0].Status = RenderJobFailed // must not stick
		return errBoom
	}); err != errBoom {
		t.Fatalf("UpdateRenderJob: want the fn's own error, got %v", err)
	}

	after, err := s.GetRenderJob(p.ID, "job-1")
	if err != nil {
		t.Fatalf("GetRenderJob: %v", err)
	}
	if after.Variations[0].Status != RenderJobRunning {
		t.Fatalf("a failed UpdateRenderJob call must not persist its mutation: status = %q", after.Variations[0].Status)
	}
}

// TestMemoryStoreUpdateRenderJobMissingJob covers an unknown job id under an
// existing project.
func TestMemoryStoreUpdateRenderJobMissingJob(t *testing.T) {
	s := NewMemory()
	p := s.CreateProject("user-a", "P")
	if _, err := s.UpdateRenderJob(p.ID, "does-not-exist", func(j *RenderJob) error { return nil }); err != ErrNotFound {
		t.Errorf("UpdateRenderJob on missing job: want ErrNotFound, got %v", err)
	}
}

type boomErr struct{}

func (*boomErr) Error() string { return "boom" }

// TestMemoryStoreListRenderJobs covers the window (jobs last updated before
// since are left out), the newest-created-first order, per-project scoping,
// and that SeenAt round-trips and is deep-copied.
func TestMemoryStoreListRenderJobs(t *testing.T) {
	s := NewMemory()
	p := s.CreateProject("user-a", "P")
	other := s.CreateProject("user-a", "Other")
	now := time.Now().UTC()

	add := func(pid, jid string, created, updated time.Time) {
		t.Helper()
		j := newTestJob(jid, "v")
		j.CreatedAt, j.UpdatedAt = created, updated
		if _, err := s.CreateRenderJob(pid, j); err != nil {
			t.Fatalf("CreateRenderJob %s: %v", jid, err)
		}
	}
	add(p.ID, "older", now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	add(p.ID, "newer", now.Add(-time.Hour), now.Add(-time.Hour))
	add(p.ID, "stale", now.Add(-48*time.Hour), now.Add(-48*time.Hour))
	// Created long ago but updated recently (finished just now): still listed.
	add(p.ID, "long-running", now.Add(-3*time.Hour), now.Add(-time.Minute))
	add(other.ID, "elsewhere", now, now)

	got, err := s.ListRenderJobs(p.ID, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("ListRenderJobs: %v", err)
	}
	var ids []string
	for _, j := range got {
		ids = append(ids, j.ID)
	}
	want := []string{"newer", "older", "long-running"}
	if len(ids) != 3 || ids[0] != want[0] || ids[1] != want[1] || ids[2] != want[2] {
		t.Fatalf("listed %v, want %v (in the window, project-scoped, newest created first)", ids, want)
	}

	seenAt := now
	if _, err := s.UpdateRenderJob(p.ID, "newer", func(j *RenderJob) error {
		j.SeenAt = &seenAt
		return nil
	}); err != nil {
		t.Fatalf("UpdateRenderJob: %v", err)
	}
	got, _ = s.ListRenderJobs(p.ID, now.Add(-24*time.Hour))
	if got[0].SeenAt == nil || !got[0].SeenAt.Equal(seenAt) {
		t.Fatalf("SeenAt = %v, want %v", got[0].SeenAt, seenAt)
	}
	*got[0].SeenAt = now.Add(time.Hour) // mutate the copy
	again, _ := s.GetRenderJob(p.ID, "newer")
	if !again.SeenAt.Equal(seenAt) {
		t.Errorf("mutating a listed job's SeenAt changed the stored one: %v", again.SeenAt)
	}
}

func TestMemoryStoreListRenderJobsMissingOrDeletedProject(t *testing.T) {
	s := NewMemory()
	if _, err := s.ListRenderJobs("nope", time.Now()); err != ErrNotFound {
		t.Errorf("missing project: err = %v, want ErrNotFound", err)
	}
	p := s.CreateProject("user-a", "P")
	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := s.ListRenderJobs(p.ID, time.Now()); err != ErrNotFound {
		t.Errorf("deleted project: err = %v, want ErrNotFound", err)
	}
}

func TestMemoryStoreMarkRenderJobSeen(t *testing.T) {
	s := NewMemory()
	p := s.CreateProject("user-a", "P")
	created := time.Now().UTC().Add(-time.Hour)
	j := newTestJob("j", "v")
	j.CreatedAt, j.UpdatedAt = created, created
	if _, err := s.CreateRenderJob(p.ID, j); err != nil {
		t.Fatalf("CreateRenderJob: %v", err)
	}

	first := created.Add(10 * time.Minute)
	if err := s.MarkRenderJobSeen(p.ID, "j", first); err != nil {
		t.Fatalf("MarkRenderJobSeen: %v", err)
	}
	if err := s.MarkRenderJobSeen(p.ID, "j", first.Add(time.Hour)); err != nil {
		t.Fatalf("second MarkRenderJobSeen: %v", err)
	}
	got, _ := s.GetRenderJob(p.ID, "j")
	if got.SeenAt == nil || !got.SeenAt.Equal(first) {
		t.Errorf("SeenAt = %v, want the first time %v (a second call must not move it)", got.SeenAt, first)
	}
	if !got.UpdatedAt.Equal(created) {
		t.Errorf("UpdatedAt = %v, want untouched %v", got.UpdatedAt, created)
	}

	if err := s.MarkRenderJobSeen(p.ID, "nope", first); err != ErrNotFound {
		t.Errorf("unknown job: err = %v, want ErrNotFound", err)
	}
	if err := s.MarkRenderJobSeen("nope", "j", first); err != ErrNotFound {
		t.Errorf("unknown project: err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if err := s.MarkRenderJobSeen(p.ID, "j", first); err != ErrNotFound {
		t.Errorf("deleted project: err = %v, want ErrNotFound", err)
	}
}

func TestMemoryStoreViewNames(t *testing.T) {
	s := NewMemory()
	p := s.CreateProject("user-a", "P")
	a, _ := s.CreateView(p.ID, "front", "img", 10, 10)
	b, _ := s.CreateView(p.ID, "back", "img", 10, 10)

	names, err := s.ViewNames(p.ID)
	if err != nil {
		t.Fatalf("ViewNames: %v", err)
	}
	if len(names) != 2 || names[a.ID] != "front" || names[b.ID] != "back" {
		t.Errorf("names = %v, want front and back by id", names)
	}

	if _, err := s.ViewNames("nope"); err != ErrNotFound {
		t.Errorf("unknown project: err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := s.ViewNames(p.ID); err != ErrNotFound {
		t.Errorf("deleted project: err = %v, want ErrNotFound", err)
	}
}
