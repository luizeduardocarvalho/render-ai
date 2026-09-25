package render

import (
	"strings"
	"testing"
)

// The interiorLights setting must reach the real prompt template: "off"
// forbids lit fixtures, a color temperature turns them on at that value, and
// an unset value (older projects) leaves the prompt silent on the topic.
func TestRenderPromptInteriorLights(t *testing.T) {
	cases := []struct {
		setting string
		want    []string
		notWant []string
	}{
		{setting: "off", want: []string{"Artificial lights: ALL OFF"}, notWant: []string{"ALL ON"}},
		{setting: "3000k", want: []string{"ALL ON at 3000K (warm white)"}, notWant: []string{"ALL OFF"}},
		{setting: "4000k", want: []string{"ALL ON at 4000K (neutral white)"}, notWant: []string{"ALL OFF"}},
		{setting: "6000k", want: []string{"ALL ON at 6000K (cool daylight white)"}, notWant: []string{"ALL OFF"}},
		{setting: "", notWant: []string{"Artificial lights"}},
	}
	for _, tc := range cases {
		t.Run(tc.setting, func(t *testing.T) {
			data := TemplateData{EdgeMapIndex: 2, Scene: "interior", Lighting: "morning_sun"}
			data.InteriorLights(tc.setting)
			prompt, err := RenderPrompt("../../prompts/render.tmpl", data)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range tc.want {
				if !strings.Contains(prompt, s) {
					t.Errorf("prompt missing %q", s)
				}
			}
			for _, s := range tc.notWant {
				if strings.Contains(prompt, s) {
					t.Errorf("prompt unexpectedly contains %q", s)
				}
			}
		})
	}
}

// Each lighting preset must add its own description under the "Lighting
// preset" line, and an unknown or empty preset must still print just the line
// without breaking the list that follows it.
func TestRenderPromptLightingPreset(t *testing.T) {
	cases := []struct {
		preset string
		want   string
	}{
		{"morning_sun", "Clear early-morning sun"},
		{"midday", "Midday sun high overhead"},
		{"overcast", "Fully overcast sky"},
		{"afternoon_sun", "Afternoon sun past its peak"},
		{"late_afternoon", "Late afternoon, sun just above the horizon"},
		{"night", "Night. Dark sky and no sunlight"},
		{"unknown_preset", ""},
		{"", ""},
	}
	presetDescriptions := []string{
		"Clear early-morning sun", "Midday sun high overhead", "Fully overcast sky",
		"Afternoon sun past its peak", "Late afternoon, sun just above the horizon",
		"Night. Dark sky and no sunlight",
	}
	for _, tc := range cases {
		t.Run(tc.preset, func(t *testing.T) {
			prompt, err := RenderPrompt("../../prompts/render.tmpl", TemplateData{
				EdgeMapIndex: 2, Scene: "interior", Lighting: tc.preset, LightDirection: "from the left",
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(prompt, "- Lighting preset: "+tc.preset+"\n") {
				t.Errorf("prompt missing the lighting preset line for %q", tc.preset)
			}
			if !strings.Contains(prompt, "\n- Main light direction (as seen in IMAGE 1): from the left") {
				t.Errorf("light direction line is not a clean list item after the preset")
			}
			for _, d := range presetDescriptions {
				if got := strings.Contains(prompt, d); got != (d == tc.want) {
					t.Errorf("description %q present=%v, want %v", d, got, d == tc.want)
				}
			}
		})
	}
}

// Every region's number, overlay color and instruction must reach the edit
// prompt, and the keep-everything-else-identical rule must stay in it.
func TestEditPromptListsEveryRegion(t *testing.T) {
	prompt, err := EditPrompt("../../prompts/edit.tmpl", EditTemplateData{Regions: []EditRegionPrompt{
		{Number: 1, Color: "red", Instruction: "a brass floor lamp"},
		{Number: 2, Color: "blue", Instruction: "paint the wall sage green"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"red area (region 1): a brass floor lamp",
		"blue area (region 2): paint the wall sage green",
		"EVERYTHING ELSE STAYS IDENTICAL",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("edit prompt missing %q", want)
		}
	}
}

// The screenshot is only described when it is actually sent.
func TestEditPromptDescribesTheScreenshotOnlyWhenSent(t *testing.T) {
	regions := []EditRegionPrompt{{Number: 1, Color: "red", Instruction: "x"}}
	with, err := EditPrompt("../../prompts/edit.tmpl", EditTemplateData{HasScreenshot: true, Regions: regions})
	if err != nil {
		t.Fatal(err)
	}
	without, err := EditPrompt("../../prompts/edit.tmpl", EditTemplateData{Regions: regions})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(with, "IMAGE 3: The original 3D model screenshot") {
		t.Errorf("prompt should describe IMAGE 3 when the screenshot is sent:\n%s", with)
	}
	if strings.Contains(without, "IMAGE 3") {
		t.Errorf("prompt mentions IMAGE 3 although no screenshot is sent:\n%s", without)
	}
}
