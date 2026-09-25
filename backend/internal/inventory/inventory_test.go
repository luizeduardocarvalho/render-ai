package inventory

import (
	"strings"
	"testing"
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
