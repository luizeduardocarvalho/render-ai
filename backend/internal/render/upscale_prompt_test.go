package render

import (
	"strings"
	"testing"
)

// An upscale that recolors or redraws the render defeats its purpose, which is
// keeping the render the user approved, so the prompt has to say so.
func TestUpscalePromptDemandsTheSameImage(t *testing.T) {
	prompt, err := UpscalePrompt("../../prompts/upscale.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"4K resolution",
		"Reproduce IMAGE 1 exactly",
		"Do NOT recolor, re-light",
		"Do NOT add, remove, move, resize or redraw any object",
		"Do NOT crop, zoom, pan, rotate or",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("upscale prompt missing %q", want)
		}
	}
}
