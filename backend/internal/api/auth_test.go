package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"render-ai/backend/internal/config"
	"render-ai/backend/internal/store"
)

// authedConfig enables auth so requireOwner actually runs its check (it is a
// no-op when CLERK_SECRET_KEY is empty).
func authedConfig() *config.Config {
	return &config.Config{Auth: config.AuthConfig{ClerkSecretKey: "sk_test_dummy"}}
}

func newTestServer() (*Server, *store.MemoryStore) {
	st := store.NewMemory()
	return NewServer(st, st, nil, nil, authedConfig(), ""), st
}

// call invokes the requireOwner-wrapped handler for pid as user, returning
// whether the inner handler ran and the HTTP status the error maps to (200 if
// no error).
func call(s *Server, pid, userID string) (called bool, status int) {
	h := s.requireOwner(func(http.ResponseWriter, *http.Request) error {
		called = true
		return nil
	})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("pid", pid)
	r = r.WithContext(context.WithValue(r.Context(), userIDCtxKey, userID))
	err := h(httptest.NewRecorder(), r)
	if err == nil {
		return called, http.StatusOK
	}
	st, _ := statusAndMessage(err)
	return called, st
}

func TestRequireOwnerAllowsOwner(t *testing.T) {
	s, st := newTestServer()
	p := st.CreateProject("user-a", "P")

	called, status := call(s, p.ID, "user-a")
	if !called || status != http.StatusOK {
		t.Fatalf("owner should pass: called=%v status=%d", called, status)
	}
}

func TestRequireOwnerDeniesNonOwnerAs404(t *testing.T) {
	s, st := newTestServer()
	p := st.CreateProject("user-a", "P")

	// A different user must not learn the project exists: 404, not 403, and the
	// inner handler must never run.
	called, status := call(s, p.ID, "user-b")
	if called {
		t.Fatal("handler ran for a non-owner")
	}
	if status != http.StatusNotFound {
		t.Fatalf("non-owner: want 404, got %d", status)
	}
}

func TestRequireOwnerMissingProjectAs404(t *testing.T) {
	s, _ := newTestServer()
	called, status := call(s, "does-not-exist", "user-a")
	if called || status != http.StatusNotFound {
		t.Fatalf("missing project: called=%v status=%d", called, status)
	}
}

// TestDeleteProjectEndpoint exercises DELETE /api/projects/{pid} through the
// same requireOwner + handler composition Router() wires it with: the owner
// gets 204 and the project then reads back as gone (soft delete), while a
// non-owner is refused with 404 and the project is left untouched.
func TestDeleteProjectEndpoint(t *testing.T) {
	s, st := newTestServer()
	p := st.CreateProject("user-a", "P")

	deleteAs := func(userID string) int {
		h := s.requireOwner(s.deleteProject)
		r := httptest.NewRequest(http.MethodDelete, "/api/projects/"+p.ID, nil)
		r.SetPathValue("pid", p.ID)
		r = r.WithContext(context.WithValue(r.Context(), userIDCtxKey, userID))
		rec := httptest.NewRecorder()
		if err := h(rec, r); err != nil {
			status, _ := statusAndMessage(err)
			return status
		}
		return rec.Code
	}

	if status := deleteAs("user-b"); status != http.StatusNotFound {
		t.Fatalf("non-owner delete: want 404, got %d", status)
	}
	if _, err := st.GetProject(p.ID); err != nil {
		t.Fatalf("project should still exist after a non-owner's delete attempt: %v", err)
	}

	if status := deleteAs("user-a"); status != http.StatusNoContent {
		t.Fatalf("owner delete: want 204, got %d", status)
	}

	if _, err := st.GetProject(p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetProject after delete: want ErrNotFound, got %v", err)
	}

	// Deleting again (already gone) is a 404, not a second success.
	if status := deleteAs("user-a"); status != http.StatusNotFound {
		t.Fatalf("delete of already-deleted project: want 404, got %d", status)
	}
}

func TestRequireOwnerDisabledWhenAuthOff(t *testing.T) {
	st := store.NewMemory()
	p := st.CreateProject("user-a", "P")
	// No CLERK_SECRET_KEY -> auth disabled -> ownership check skipped entirely.
	s := NewServer(st, st, nil, nil, &config.Config{}, "")

	called, status := call(s, p.ID, "anyone")
	if !called || status != http.StatusOK {
		t.Fatalf("auth-disabled should pass through: called=%v status=%d", called, status)
	}
}
