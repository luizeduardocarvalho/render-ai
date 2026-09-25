package store

import "testing"

// Projects saved with a retired lighting preset must read back as the
// current one, so the frontend and the prompt never see the old ids.
func TestRetiredLightingPresetsReadBackAsCurrentOnes(t *testing.T) {
	cases := []struct {
		saved LightingPreset
		want  LightingPreset
	}{
		{"golden_hour", LightingLateAfternoon},
		{"evening_interior_lights", LightingNight},
		{"night_exterior", LightingNight},
		{LightingMorningSun, LightingMorningSun},
		{LightingMidday, LightingMidday},
		{LightingOvercast, LightingOvercast},
		{LightingAfternoonSun, LightingAfternoonSun},
		{LightingLateAfternoon, LightingLateAfternoon},
		{LightingNight, LightingNight},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(string(tc.saved), func(t *testing.T) {
			st := NewMemory()
			p := st.CreateProject("u", "P")
			if _, err := st.UpdateStyle(p.ID, StyleSettings{Lighting: tc.saved}); err != nil {
				t.Fatal(err)
			}
			got, err := st.GetProject(p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Style.Lighting != tc.want {
				t.Errorf("lighting = %q, want %q", got.Style.Lighting, tc.want)
			}
		})
	}
}
