package store

import "testing"

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
