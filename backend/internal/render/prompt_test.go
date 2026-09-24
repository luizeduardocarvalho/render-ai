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
