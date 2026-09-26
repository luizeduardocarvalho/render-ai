package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"google.golang.org/genai"

	"render-ai/backend/internal/inventory"
)

// fakeInventoryModel serves a canned inventory (or a 500) as the Gemini text
// model and records every request body it receives.
type fakeInventoryModel struct {
	mu     sync.Mutex
	bodies []string
}

func (f *fakeInventoryModel) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

func (f *fakeInventoryModel) lastBody() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[len(f.bodies)-1]
}

func newFakeInventoryModel(t *testing.T, status int) (*inventory.TextModel, *fakeInventoryModel) {
	t.Helper()
	rec := &fakeInventoryModel{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.bodies = append(rec.bodies, string(body))
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if status != http.StatusOK {
			w.WriteHeader(status)
			fmt.Fprint(w, `{"error":{"code":500,"message":"boom","status":"INTERNAL"}}`)
			return
		}
		fmt.Fprint(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Floor, whole room - dark walnut, matte"}]}}]}`)
	}))
	t.Cleanup(srv.Close)

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		Backend:     genai.BackendGeminiAPI,
		APIKey:      "test",
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	return inventory.NewTextModel(client, "test-model"), rec
}

func inventoryOf(t *testing.T, s *Server, pid, vid string) string {
	t.Helper()
	p, err := s.repo.GetProject(pid)
	if err != nil {
		t.Fatal(err)
	}
	return findViewIn(p, vid).Inventory
}

// A render on a view with no list builds one first, from the screenshot and
// the project's material notes, and saves it on the view.
func TestStartRenderGeneratesMissingInventoryFromMaterialNotes(t *testing.T) {
	s, repo, p, v, queue := setupRenderTest(t, alwaysSucceeds(t))
	model, gemini := newFakeInventoryModel(t, http.StatusOK)
	s.textModel = model
	style := p.Style
	style.MaterialNotes = "the floor should use dark wood"
	if _, err := repo.UpdateStyle(p.ID, style); err != nil {
		t.Fatal(err)
	}

	if _, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 2}); status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	queue.Wait()

	if gemini.calls() != 1 {
		t.Fatalf("text model called %d times, want once for the whole job", gemini.calls())
	}
	if !strings.Contains(gemini.lastBody(), "the floor should use dark wood") {
		t.Errorf("text model request does not carry the material notes: %s", gemini.lastBody())
	}
	if got := inventoryOf(t, s, p.ID, v.ID); got != "Floor, whole room - dark walnut, matte" {
		t.Errorf("saved inventory = %q, want the generated one", got)
	}
}

// A list the user wrote or edited is never overwritten by a render.
func TestStartRenderKeepsExistingInventory(t *testing.T) {
	s, repo, p, v, queue := setupRenderTest(t, alwaysSucceeds(t))
	model, gemini := newFakeInventoryModel(t, http.StatusOK)
	s.textModel = model
	if _, err := repo.SetInventory(p.ID, v.ID, "1x sofa - grey boucle"); err != nil {
		t.Fatal(err)
	}

	if _, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 1}); status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", status)
	}
	queue.Wait()

	if gemini.calls() != 0 {
		t.Errorf("text model called %d times, want 0 when a list exists", gemini.calls())
	}
	if got := inventoryOf(t, s, p.ID, v.ID); got != "1x sofa - grey boucle" {
		t.Errorf("inventory = %q, want the user's list untouched", got)
	}
}

// The text call is an extra: when it fails the render still starts, with the
// same placeholder it had before the list was built automatically.
func TestStartRenderSurvivesInventoryFailure(t *testing.T) {
	s, _, p, v, queue := setupRenderTest(t, alwaysSucceeds(t))
	model, gemini := newFakeInventoryModel(t, http.StatusInternalServerError)
	s.textModel = model

	resp, status := postStartRender(t, s, p.ID, v.ID, renderRequestBody{Model: "pro", Resolution: "2K", Variations: 1})
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 even though the inventory call failed", status)
	}
	queue.Wait()

	final, err := getRenderJobResponse(t, s, p.ID, resp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gemini.calls() == 0 {
		t.Fatal("expected the text model to be tried")
	}
	if final.Status != "done" {
		t.Errorf("job status = %q, want done", final.Status)
	}
	if got := inventoryOf(t, s, p.ID, v.ID); got != "" {
		t.Errorf("inventory = %q, want it left empty", got)
	}
}
