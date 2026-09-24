// Package store defines the render-ai data model and its persistence
// interfaces (Repository for structured data, BlobStore for image bytes). It
// ships two backends: an in-memory one for local dev (lost on restart) and a
// Firestore + GCS one for the deployed service.
package store

import "time"

// ScenePreset is the project's scene type.
type ScenePreset string

// LightingPreset is a canned lighting mood for the render prompt.
type LightingPreset string

// InteriorLights says whether the scene's artificial lights are switched on
// and, if so, at which color temperature.
type InteriorLights string

// ModelChoice selects which Gemini image model a render uses.
type ModelChoice string

// Resolution is the requested output resolution of a render.
type Resolution string

// Known enum values for the string-typed fields above. These are advisory -
// the API does not reject unknown values, it only special-cases "pro"/"flash"
// and "1K"/"2K"/"4K" for the resolution guard.
const (
	SceneInterior ScenePreset = "interior"
	SceneExterior ScenePreset = "exterior"

	LightingMorningSun            LightingPreset = "morning_sun"
	LightingOvercast              LightingPreset = "overcast"
	LightingGoldenHour            LightingPreset = "golden_hour"
	LightingEveningInteriorLights LightingPreset = "evening_interior_lights"
	LightingNightExterior         LightingPreset = "night_exterior"

	InteriorLightsOff   InteriorLights = "off"
	InteriorLights3000K InteriorLights = "3000k"
	InteriorLights4000K InteriorLights = "4000k"
	InteriorLights6000K InteriorLights = "6000k"

	ModelPro   ModelChoice = "pro"
	ModelFlash ModelChoice = "flash"

	Resolution1K Resolution = "1K"
	Resolution2K Resolution = "2K"
	Resolution4K Resolution = "4K"
)

// Asset is a library item (a real product) that a mask region can resolve to.
type Asset struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Color             string `json:"color"`
	ReferenceImageID  string `json:"referenceImageId"`
	HasReferenceImage bool   `json:"hasReferenceImage"`
}

// Mask is a painted region on a view, optionally bound to an Asset. Its
// bitmap (white-on-black PNG, at the view's screenshot resolution) is stored
// as a blob under the mask's own ID - so GET /api/images/{maskId} fetches it.
type Mask struct {
	ID        string  `json:"id"`
	AssetID   *string `json:"assetId"`
	HasBitmap bool    `json:"hasBitmap"`
	Hidden    bool    `json:"hidden"`
}

// RenderMetrics captures cost/latency/token accounting for one render.
type RenderMetrics struct {
	Model            string     `json:"model"`
	Resolution       Resolution `json:"resolution"`
	RegionCount      int        `json:"regionCount"`
	AnchorUsed       bool       `json:"anchorUsed"`
	ImageCallMs      int64      `json:"imageCallMs"`
	TotalMs          int64      `json:"totalMs"`
	PromptTokens     *int32     `json:"promptTokens,omitempty"`
	OutputTokens     *int32     `json:"outputTokens,omitempty"`
	EstimatedCostUsd *float64   `json:"estimatedCostUsd,omitempty"`
}

// PreservationInventory is the text model's structured diff of what changed
// between the screenshot and the render.
type PreservationInventory struct {
	Removed []string `json:"removed"`
	Added   []string `json:"added"`
	Moved   []string `json:"moved"`
	Raw     string   `json:"raw,omitempty"`
}

// PreservationReport is attached to a Render when preservationCheck was on.
type PreservationReport struct {
	EdgeScore float64               `json:"edgeScore"`
	EdgeFlag  bool                  `json:"edgeFlag"`
	Inventory PreservationInventory `json:"inventory"`
}

// Render is one image generation result for a view.
type Render struct {
	ID            string              `json:"id"`
	CreatedAt     time.Time           `json:"createdAt"`
	Model         ModelChoice         `json:"model"`
	Resolution    Resolution          `json:"resolution"`
	AnchorUsed    bool                `json:"anchorUsed"`
	RegionCount   int                 `json:"regionCount"`
	ResultImageID string              `json:"resultImageId"`
	Metrics       RenderMetrics       `json:"metrics"`
	Preservation  *PreservationReport `json:"preservation,omitempty"`
	IsStyleAnchor bool                `json:"isStyleAnchor"`
}

// View is one camera angle: a screenshot plus its masks and render history.
type View struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	ScreenshotImageID string    `json:"screenshotImageId"`
	HasScreenshot     bool      `json:"hasScreenshot"`
	Width             int       `json:"width"`
	Height            int       `json:"height"`
	Masks             []*Mask   `json:"masks"`
	Inventory         string    `json:"inventory"`
	Renders           []*Render `json:"renders"`
}

// StyleSettings are the project-wide render style controls.
type StyleSettings struct {
	Scene          ScenePreset    `json:"scene"`
	Lighting       LightingPreset `json:"lighting"`
	LightDirection string         `json:"lightDirection"`
	// InteriorLights is empty on projects created before the field existed;
	// the prompt then says nothing about artificial lights.
	InteriorLights    InteriorLights `json:"interiorLights"`
	MaterialNotes     string         `json:"materialNotes"`
	ExtraInstructions string         `json:"extraInstructions"`
}

// Project is the top-level container for everything the frontend works with.
//
// OwnerID is the Clerk user id of the project's owner; every project-scoped
// route is authorized against it (see the API layer). OrgID is reserved for a
// future org-scoping model - it is always nil today, but present so projects
// can gain an org dimension without a data migration.
type Project struct {
	ID                  string        `json:"id"`
	OwnerID             string        `json:"ownerId"`
	OrgID               *string       `json:"orgId"`
	Name                string        `json:"name"`
	CreatedAt           time.Time     `json:"createdAt"`
	UpdatedAt           time.Time     `json:"updatedAt"`
	Style               StyleSettings `json:"style"`
	Assets              []*Asset      `json:"assets"`
	Views               []*View       `json:"views"`
	StyleAnchorRenderID *string       `json:"styleAnchorRenderId"`
}

// Valid reports whether l is one of the known values. Empty is valid: it
// means the setting was never chosen.
func (l InteriorLights) Valid() bool {
	switch l {
	case "", InteriorLightsOff, InteriorLights3000K, InteriorLights4000K, InteriorLights6000K:
		return true
	}
	return false
}

func defaultStyle() StyleSettings {
	return StyleSettings{
		Scene:          SceneInterior,
		Lighting:       LightingMorningSun,
		InteriorLights: InteriorLightsOff,
	}
}

// clone returns a deep copy of the project, so callers can freely read or
// JSON-encode it after the store's lock has been released.
func (p *Project) clone() *Project {
	out := &Project{
		ID:                  p.ID,
		OwnerID:             p.OwnerID,
		OrgID:               clonePtr(p.OrgID),
		Name:                p.Name,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
		Style:               p.Style,
		StyleAnchorRenderID: clonePtr(p.StyleAnchorRenderID),
		Assets:              make([]*Asset, len(p.Assets)),
		Views:               make([]*View, len(p.Views)),
	}
	for i, a := range p.Assets {
		clone := *a
		out.Assets[i] = &clone
	}
	for i, v := range p.Views {
		out.Views[i] = v.clone()
	}
	return out
}

func (v *View) clone() *View {
	out := &View{
		ID:                v.ID,
		Name:              v.Name,
		ScreenshotImageID: v.ScreenshotImageID,
		HasScreenshot:     v.HasScreenshot,
		Width:             v.Width,
		Height:            v.Height,
		Inventory:         v.Inventory,
		Masks:             make([]*Mask, len(v.Masks)),
		Renders:           make([]*Render, len(v.Renders)),
	}
	for i, m := range v.Masks {
		clone := *m
		clone.AssetID = clonePtr(m.AssetID)
		out.Masks[i] = &clone
	}
	for i, r := range v.Renders {
		out.Renders[i] = r.clone()
	}
	return out
}

func (r *Render) clone() *Render {
	clone := *r
	clone.Metrics = r.Metrics
	clone.Metrics.PromptTokens = clonePtr(r.Metrics.PromptTokens)
	clone.Metrics.OutputTokens = clonePtr(r.Metrics.OutputTokens)
	clone.Metrics.EstimatedCostUsd = clonePtr(r.Metrics.EstimatedCostUsd)
	if r.Preservation != nil {
		p := *r.Preservation
		p.Inventory.Removed = cloneStrings(r.Preservation.Inventory.Removed)
		p.Inventory.Added = cloneStrings(r.Preservation.Inventory.Added)
		p.Inventory.Moved = cloneStrings(r.Preservation.Inventory.Moved)
		clone.Preservation = &p
	}
	return &clone
}

// cloneStrings copies a slice while preserving its nil-vs-empty distinction
// (unlike append([]string(nil), s...), which collapses an empty slice to nil).
func cloneStrings(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
