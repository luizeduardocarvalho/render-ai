package store

import "testing"

// TestMemoryStoreSoftDelete covers the soft-delete contract: ListProjects
// excludes a deleted project, GetProject/ProjectOwner return ErrNotFound for
// it, and deleting it again also returns ErrNotFound instead of succeeding
// silently.
func TestMemoryStoreSoftDelete(t *testing.T) {
	s := NewMemory()
	p := s.CreateProject("user-a", "P")

	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatalf("DeleteProject: unexpected error: %v", err)
	}

	summaries, err := s.ListProjects("user-a")
	if err != nil {
		t.Fatalf("ListProjects: unexpected error: %v", err)
	}
	for _, sm := range summaries {
		if sm.ID == p.ID {
			t.Fatalf("ListProjects: deleted project %s still listed", p.ID)
		}
	}

	if _, err := s.GetProject(p.ID); err != ErrNotFound {
		t.Errorf("GetProject on deleted project: want ErrNotFound, got %v", err)
	}
	if _, err := s.ProjectOwner(p.ID); err != ErrNotFound {
		t.Errorf("ProjectOwner on deleted project: want ErrNotFound, got %v", err)
	}

	if err := s.DeleteProject(p.ID); err != ErrNotFound {
		t.Errorf("second DeleteProject: want ErrNotFound, got %v", err)
	}
}

// TestMemoryStoreDeleteProjectMissing covers deleting a project that never
// existed.
func TestMemoryStoreDeleteProjectMissing(t *testing.T) {
	s := NewMemory()
	if err := s.DeleteProject("does-not-exist"); err != ErrNotFound {
		t.Errorf("DeleteProject on missing project: want ErrNotFound, got %v", err)
	}
}

// TestMemoryStoreSoftDeleteKeepsViewsAndBlobs confirms a soft delete does not
// remove the project's views or blobs - only DeletedAt is set - matching the
// 30-day-recoverable design.
func TestMemoryStoreSoftDeleteKeepsViewsAndBlobs(t *testing.T) {
	s := NewMemory()
	p := s.CreateProject("user-a", "P")
	imgID, err := s.PutBlob([]byte("fake-png"), "image/png")
	if err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	if _, err := s.CreateView(p.ID, "front", imgID, 100, 100); err != nil {
		t.Fatalf("CreateView: %v", err)
	}

	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	s.mu.Lock()
	live, ok := s.projects[p.ID]
	s.mu.Unlock()
	if !ok {
		t.Fatal("project record was removed by a soft delete")
	}
	if live.DeletedAt == nil {
		t.Fatal("DeletedAt was not set")
	}
	if len(live.Views) != 1 {
		t.Fatalf("want the view kept, got %d views", len(live.Views))
	}
	if _, ok := s.GetBlob(imgID); !ok {
		t.Fatal("blob was removed by a soft delete")
	}
}
