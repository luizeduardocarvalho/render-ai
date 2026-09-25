package render

import (
	"strings"
	"testing"
)

// The model was seen tracing the borders of the colored areas in the color of
// each area, so the edit prompt has to forbid it.
func TestEditPromptForbidsTracingTheColoredAreas(t *testing.T) {
	prompt, err := EditPrompt("../../prompts/edit.tmpl", EditTemplateData{Regions: []EditRegionPrompt{
		{Number: 1, Color: "red", Instruction: "x"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Do NOT draw an outline, border, stroke", "leave no trace"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("edit prompt missing %q", want)
		}
	}
}
