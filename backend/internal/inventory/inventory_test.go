package inventory

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/genai"
)

// The inventory is also the material specification the renderer follows, so
// both language prompts must ask for a material on every line.
func TestInventoryPromptsAskForMaterials(t *testing.T) {
	cases := map[string][]string{
		"en":    {inventoryPromptEN, "<material>", "finish", "never answer just"},
		"pt-BR": {inventoryPromptPTBR, "<material>", "acabamento", "nunca responda apenas"},
	}
	for lang, c := range cases {
		prompt := c[0]
		for _, want := range c[1:] {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s inventory prompt missing %q", lang, want)
			}
		}
		if strings.Contains(prompt, "materials.") || strings.Contains(prompt, "materiais.") {
			t.Errorf("%s inventory prompt still tells the model to skip materials", lang)
		}
	}
}

// A 429 from Vertex is a quota blip, not a failure of the request, so the
// handler must be able to tell it apart (even when wrapped).
func TestIsRateLimited(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"429 code", genai.APIError{Code: 429}, true},
		{"RESOURCE_EXHAUSTED status", genai.APIError{Code: 0, Status: "RESOURCE_EXHAUSTED"}, true},
		{"wrapped", fmt.Errorf("generating inventory: %w", genai.APIError{Code: 429}), true},
		{"other api error", genai.APIError{Code: 500, Status: "INTERNAL"}, false},
		{"plain error", fmt.Errorf("boom"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := IsRateLimited(c.err); got != c.want {
			t.Errorf("%s: IsRateLimited = %v, want %v", c.name, got, c.want)
		}
	}
}

// fakeGemini serves the given statuses in order (then 200 with an inventory
// line) and returns a TextModel pointed at it plus the request counter.
func fakeGemini(t *testing.T, statuses ...int) (*TextModel, *atomic.Int32) {
	t.Helper()
	retryInitialDelay, retryMaxDelay, retryJitter = 0, 0, 0
	t.Cleanup(func() { retryInitialDelay, retryMaxDelay, retryJitter = 2, 8, 1 })

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1))
		w.Header().Set("Content-Type", "application/json")
		if n <= len(statuses) {
			w.WriteHeader(statuses[n-1])
			fmt.Fprint(w, `{"error":{"code":429,"message":"Resource exhausted","status":"RESOURCE_EXHAUSTED"}}`)
			return
		}
		fmt.Fprint(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"1x sofa, left - grey boucle"}]}}]}`)
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
	return NewTextModel(client, "test-model"), &calls
}

func TestGenerateInventoryRetriesRateLimit(t *testing.T) {
	m, calls := fakeGemini(t, 429, 429)
	text, _, _, err := m.GenerateInventory(context.Background(), []byte("png"), "en", "")
	if err != nil {
		t.Fatalf("expected recovery after two 429s, got %v", err)
	}
	if text != "1x sofa, left - grey boucle" || calls.Load() != 3 {
		t.Errorf("text=%q calls=%d, want inventory after 3 calls", text, calls.Load())
	}
}

func TestGenerateInventoryGivesUpOnPersistentRateLimit(t *testing.T) {
	m, calls := fakeGemini(t, 429, 429, 429, 429)
	_, _, _, err := m.GenerateInventory(context.Background(), []byte("png"), "en", "")
	if !IsRateLimited(err) {
		t.Fatalf("expected a rate-limit error, got %v", err)
	}
	if calls.Load() != int32(retryAttempts) {
		t.Errorf("calls=%d, want %d attempts", calls.Load(), retryAttempts)
	}
}

func TestBuildPromptFoldsInMaterialNotes(t *testing.T) {
	cases := map[string]struct{ lang, closing, base string }{
		"en":    {"en", "Write the catalogue in English.", inventoryPromptEN},
		"pt-BR": {"pt-BR", "Escreva o catálogo em português do Brasil.", inventoryPromptPTBR},
	}
	for name, c := range cases {
		withNotes := buildPrompt(c.lang, "  the floor should use dark wood  ")
		if !strings.Contains(withNotes, "the floor should use dark wood") {
			t.Errorf("%s: prompt does not carry the material notes", name)
		}
		if !strings.HasSuffix(withNotes, c.closing) {
			t.Errorf("%s: the language line should stay last, got tail %q", name, withNotes[len(withNotes)-60:])
		}
		for _, empty := range []string{"", "   \n"} {
			if buildPrompt(c.lang, empty) != c.base {
				t.Errorf("%s: blank notes %q should leave the prompt untouched", name, empty)
			}
		}
	}
}
