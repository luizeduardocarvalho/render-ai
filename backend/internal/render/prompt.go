package render

import (
	"bytes"
	"fmt"
	"os"
	"text/template"
)

// AssetRef describes one asset reference photo included in the render
// request, for the "{{ range .AssetRefs }}" section of the prompt template.
type AssetRef struct {
	Index       int
	Name        string
	Description string
	ColorName   string
}

// TemplateData is the exact set of fields prompts/render.tmpl expects.
type TemplateData struct {
	HasRegionMap   bool
	EdgeMapIndex   int
	HasAnchor      bool
	AnchorIndex    int
	AssetRefs      []AssetRef
	Scene          string
	Lighting       string
	LightDirection string
	// InteriorLightsOff and InteriorLightsKelvin come from InteriorLights.
	// Both are zero when the project never chose a setting.
	InteriorLightsOff    bool
	InteriorLightsKelvin string // e.g. "3000K"
	InteriorLightsTone   string // e.g. "warm white"
	MaterialNotes        string
	ExtraInstructions    string
	Inventory            string
}

// InteriorLights fills the artificial-light fields of d from the project's
// interiorLights setting ("off", "3000k", "4000k", "6000k" or empty).
func (d *TemplateData) InteriorLights(setting string) {
	switch setting {
	case "off":
		d.InteriorLightsOff = true
	case "3000k":
		d.InteriorLightsKelvin, d.InteriorLightsTone = "3000K", "warm white"
	case "4000k":
		d.InteriorLightsKelvin, d.InteriorLightsTone = "4000K", "neutral white"
	case "6000k":
		d.InteriorLightsKelvin, d.InteriorLightsTone = "6000K", "cool daylight white"
	}
}

// RenderPrompt loads the template fresh from templatePath on every call (so
// it can be tweaked without a rebuild) and executes it with data.
func RenderPrompt(templatePath string, data TemplateData) (string, error) {
	raw, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("reading prompt template %s: %w", templatePath, err)
	}
	tmpl, err := template.New("render").Parse(string(raw))
	if err != nil {
		return "", fmt.Errorf("parsing prompt template %s: %w", templatePath, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing prompt template %s: %w", templatePath, err)
	}
	return buf.String(), nil
}
