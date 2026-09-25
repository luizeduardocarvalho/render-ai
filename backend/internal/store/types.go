// Package store defines the render-ai data model and its persistence
// interfaces (Repository for structured data, BlobStore for image bytes). It
// ships two backends: an in-memory one for local dev (lost on restart) and a
// Firestore + GCS one for the deployed service.
package store

import (
	"slices"
	"time"
)

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
// Assets belong to a user (see Repository's asset-library methods), not a
// project - CreatedAt orders a user's library newest-first.
type Asset struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Color             string    `json:"color"`
	ReferenceImageID  string    `json:"referenceImageId"`
	HasReferenceImage bool      `json:"hasReferenceImage"`
	CreatedAt         time.Time `json:"createdAt"`
}

// Credit ledger reasons - see Repository.AdjustCredits and
// API_CONTRACT.md's credits section.
const (
	CreditReasonGrant  = "grant"  // an admin topped the account up (or corrected it down).
	CreditReasonRender = "render" // a render job debited the account.
	CreditReasonRefund = "refund" // a render variation that never delivered was refunded.
)

// CreditLedgerEntry is one entry in a user's credit ledger: a signed delta
// (in integer units - see internal/api/credits.go) plus enough context to
// explain it later. AdjustCredits fills ID/CreatedAt/DeltaUnits/
// BalanceAfterUnits; the caller sets Reason and whichever of
// ProjectID/JobID/Note/ActorID applies.
type CreditLedgerEntry struct {
	ID                string
	CreatedAt         time.Time
	DeltaUnits        int64
	BalanceAfterUnits int64
	// Reason is one of the CreditReason* constants above.
	Reason    string
	ProjectID *string
	JobID     *string
	Note      string
	// ActorID is the Clerk user id of who made this change, when it wasn't
	// the account owner themselves - set for CreditReasonGrant (the admin
	// who granted it).
	ActorID string
}

// clone returns a deep copy of the entry.
func (e CreditLedgerEntry) clone() CreditLedgerEntry {
	out := e
	out.ProjectID = clonePtr(e.ProjectID)
	out.JobID = clonePtr(e.JobID)
	return out
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
//
// PromptTokens/OutputTokens/ThoughtsTokens are summed across every model call
// this render made: the image generation call, plus the text-model
// preservation check if it ran. OutputTokens is TEXT output only - it never
// includes the image call's own generated-image tokens, which are priced
// per-image (see internal/render/pricing.go) rather than per-token, to avoid
// double-counting them at the text/thinking output rate. ThoughtsTokens is
// the thinking-token portion, broken out for visibility; it's already
// included in whatever EstimatedCostUsd charges at the output rate.
type RenderMetrics struct {
	Model            string     `json:"model"`
	Resolution       Resolution `json:"resolution"`
	RegionCount      int        `json:"regionCount"`
	AnchorUsed       bool       `json:"anchorUsed"`
	ImageCallMs      int64      `json:"imageCallMs"`
	TotalMs          int64      `json:"totalMs"`
	PromptTokens     *int32     `json:"promptTokens,omitempty"`
	OutputTokens     *int32     `json:"outputTokens,omitempty"`
	ThoughtsTokens   *int32     `json:"thoughtsTokens,omitempty"`
	EstimatedCostUsd *float64   `json:"estimatedCostUsd,omitempty"`
	// Attempts is how many times the model was called to get this render: 1
	// normally, more when earlier attempts were discarded for not following
	// the screenshot (see PreservationConfig.MaxRegenerations). Every
	// cumulative figure above (latencies, tokens, cost) covers all of them.
	Attempts int `json:"attempts,omitempty"`
}

// PreservationReport is attached to a Render when preservationCheck was on.
type PreservationReport struct {
	EdgeScore float64 `json:"edgeScore"`
	EdgeFlag  bool    `json:"edgeFlag"`
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
	// SourceRenderID is set when this Render is an Edit: the Render (in the
	// same view) it was derived from. Empty for a Render made from the
	// screenshot.
	SourceRenderID string `json:"sourceRenderId,omitempty"`
	// EditInstructions are the per-region instructions the Edit was asked
	// for, in region order. Empty unless SourceRenderID is set.
	EditInstructions []string `json:"editInstructions,omitempty"`
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
//
// DeletedAt marks a soft-deleted project: everything else about it (views,
// masks, renders, blobs) is left in place so it can be restored, but every
// Repository method treats it as not found (see each implementation's
// project-loading helper). Nil means the project is live.
type Project struct {
	ID                  string        `json:"id"`
	OwnerID             string        `json:"ownerId"`
	OrgID               *string       `json:"orgId"`
	Name                string        `json:"name"`
	CreatedAt           time.Time     `json:"createdAt"`
	UpdatedAt           time.Time     `json:"updatedAt"`
	DeletedAt           *time.Time    `json:"deletedAt,omitempty"`
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
		DeletedAt:           clonePtr(p.DeletedAt),
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
	clone.Metrics.ThoughtsTokens = clonePtr(r.Metrics.ThoughtsTokens)
	clone.Metrics.EstimatedCostUsd = clonePtr(r.Metrics.EstimatedCostUsd)
	if r.Preservation != nil {
		p := *r.Preservation
		clone.Preservation = &p
	}
	clone.EditInstructions = slices.Clone(r.EditInstructions)
	return &clone
}

// RenderJobStatus is the lifecycle state of a RenderJob or one of its
// variations.
type RenderJobStatus string

const (
	RenderJobQueued  RenderJobStatus = "queued"
	RenderJobRunning RenderJobStatus = "running"
	RenderJobDone    RenderJobStatus = "done"
	RenderJobFailed  RenderJobStatus = "failed"
)

// RenderJobRequest is the render request a RenderJob was created from -
// unchanged from the old synchronous POST body (see API_CONTRACT.md).
type RenderJobRequest struct {
	Model             ModelChoice `json:"model"`
	Resolution        Resolution  `json:"resolution"`
	PreservationCheck bool        `json:"preservationCheck"`
	Variations        int         `json:"variations"`
	// Edit is set when the job edits an existing Render instead of rendering
	// the view's screenshot; Model and Resolution are then the source
	// Render's own and Variations is 1.
	Edit *RenderJobEdit `json:"edit,omitempty"`
}

// RenderJobEdit is the Edit-specific half of a RenderJobRequest.
type RenderJobEdit struct {
	SourceRenderID string       `json:"sourceRenderId"`
	Regions        []EditRegion `json:"regions"`
}

// EditRegion is one area to change on the source Render, with what to change
// it into. BitmapImageID is a blob holding the area as a white-on-black PNG
// (any size, same aspect ratio as the source); it only lives as long as the
// job does.
type EditRegion struct {
	Instruction   string `json:"instruction"`
	BitmapImageID string `json:"bitmapImageId"`
}

// RenderJobVariation tracks one Cloud Task's progress: one independent
// sample of the job's request. RunningAt is set when the worker claims the
// variation (queued -> running) and is used only to detect a variation stuck
// in "running" because its worker died without ever marking it terminal
// (see the stale-running rule in API_CONTRACT.md) - it is never surfaced to
// the frontend.
type RenderJobVariation struct {
	Status    RenderJobStatus `json:"status"`
	RenderID  *string         `json:"renderId,omitempty"`
	Error     *string         `json:"error,omitempty"`
	RunningAt *time.Time      `json:"-"`
	// Refunded marks that this variation's charged credits have already been
	// refunded (or never need to be - it's still queued/running, or it
	// succeeded). Set inside the same UpdateRenderJob call that first marks
	// the variation failed, so a redelivered task or a second failure
	// observation can never refund it twice. Not surfaced to the frontend.
	Refunded bool `json:"-"`
	// Attempt is the 0-based attempt this variation is on: how many times it
	// has been regenerated. It only ever grows, and is what bounds the chain
	// of regenerations - see Server.RunRenderVariation. Not surfaced to the
	// frontend.
	Attempt int `json:"-"`
	// Held is the best flagged Render of the earlier attempts, kept (image
	// blob included) but not yet in the view's history: it is what the user
	// gets if the remaining attempts fail or are flagged too. Its Metrics
	// are cumulative over the attempts so far. Nil on the first attempt. Not
	// surfaced to the frontend.
	Held *Render `json:"-"`
}

// RenderJob is one POST .../render request turned into a job: one Cloud
// Task (or, in "inline" queue mode, one goroutine) per variation. Its
// derived JSON fields - overall status, the first failure's error, and the
// full Render objects for finished variations - are computed by the API
// layer (see internal/api/render.go), not stored here; see
// API_CONTRACT.md's RenderJob shape and its status-derivation rules.
type RenderJob struct {
	ID         string               `json:"id"`
	ViewID     string               `json:"viewId"`
	CreatedAt  time.Time            `json:"createdAt"`
	UpdatedAt  time.Time            `json:"updatedAt"`
	Request    RenderJobRequest     `json:"request"`
	Variations []RenderJobVariation `json:"variations"`
	// ChargedUnitsPerVariation is the credit cost (in integer units - see
	// internal/api/credits.go) of one variation of this job, fixed at
	// creation time so a later change to the price table never changes what
	// a refund gives back. 0 when auth was disabled at creation time (no
	// credits were ever charged, so none are ever refunded either).
	ChargedUnitsPerVariation int64 `json:"-"`
}

// clone returns a deep copy of the job, so callers can freely read or
// mutate it after the store's lock has been released.
func (j *RenderJob) clone() *RenderJob {
	out := *j
	if j.Request.Edit != nil {
		edit := *j.Request.Edit
		edit.Regions = slices.Clone(edit.Regions)
		out.Request.Edit = &edit
	}
	out.Variations = make([]RenderJobVariation, len(j.Variations))
	for i, v := range j.Variations {
		out.Variations[i] = v.clone()
	}
	return &out
}

func (v RenderJobVariation) clone() RenderJobVariation {
	out := v
	out.RenderID = clonePtr(v.RenderID)
	out.Error = clonePtr(v.Error)
	out.RunningAt = clonePtr(v.RunningAt)
	if v.Held != nil {
		out.Held = v.Held.clone()
	}
	return out
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
