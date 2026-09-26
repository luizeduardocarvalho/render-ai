package api

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"render-ai/backend/internal/geometry"
	"render-ai/backend/internal/imageutil"
	"render-ai/backend/internal/jobs"
	renderpkg "render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

type renderRequestBody struct {
	Model             string `json:"model"`
	Resolution        string `json:"resolution"`
	PreservationCheck bool   `json:"preservationCheck"`
	// Variations is how many independent render samples to generate from the
	// same request and keep, each as its own store.Render (one Cloud Task /
	// goroutine per variation - see internal/jobs). The image models expose
	// no seed, so every call is already an independent sample - this just
	// repeats the (cheap) call N times instead of one. Omitted/0 means 1.
	Variations int `json:"variations"`
}

// minVariations and maxVariations bound the Variations field. 4 is a
// deliberately small ceiling - each variation is a full-price model call.
const (
	minVariations = 1
	maxVariations = 4
)

// staleRunningGrace is added to the render timeout to decide how long a
// variation may sit in "running" before GET render-jobs reports it as
// failed instead (its worker died - instance killed, crashed, etc. -
// without ever marking it terminal). Matches the plan's
// "renderTimeoutSec + 120s" rule.
const staleRunningGrace = 120 * time.Second

// staleRunningError is the error message surfaced for a variation the
// stale-running rule gives up on. See buildRenderJobResponse.
const staleRunningError = "render worker did not finish"

// qualifyingMask is a non-hidden, painted mask bound to a library asset that
// has a reference photo - these are exactly the masks that drive the region
// map and the set of asset reference photos sent to the model.
type qualifyingMask struct {
	mask  *store.Mask
	asset *store.Asset
}

// validateRenderRequest applies the same synchronous validation the old
// endpoint did before it became a job: model/resolution/variations shape,
// and the flash+non-1K combination. It does not touch the store - the
// caller (startRender) does the project/view existence checks separately,
// so validation stays cheap and side-effect free.
func validateRenderRequest(body renderRequestBody) (store.RenderJobRequest, error) {
	model := store.ModelChoice(body.Model)
	if model != store.ModelPro && model != store.ModelFlash {
		return store.RenderJobRequest{}, badRequest(`model must be "pro" or "flash"`)
	}
	resolution := store.Resolution(body.Resolution)
	switch resolution {
	case store.Resolution1K, store.Resolution2K, store.Resolution4K:
	default:
		return store.RenderJobRequest{}, badRequest(`resolution must be "1K", "2K", or "4K"`)
	}
	if model == store.ModelFlash && resolution != store.Resolution1K {
		return store.RenderJobRequest{}, badRequest("flash model only supports 1K resolution")
	}
	variations := body.Variations
	if variations == 0 {
		variations = 1
	}
	if variations < minVariations || variations > maxVariations {
		return store.RenderJobRequest{}, badRequest("variations must be between %d and %d", minVariations, maxVariations)
	}
	return store.RenderJobRequest{
		Model:             model,
		Resolution:        resolution,
		PreservationCheck: body.PreservationCheck,
		Variations:        variations,
	}, nil
}

// startRender is POST /api/projects/{pid}/views/{vid}/render. It does the
// same synchronous validation and cheap existence checks the old
// synchronous handler did, then persists a RenderJob and enqueues one task
// per variation instead of running the renders itself, so the request
// finishes well inside Firebase Hosting's 60s cutoff. The actual render work (assembly + model
// call + storage) happens in RunRenderVariation, invoked by the queue.
func (s *Server) startRender(w http.ResponseWriter, r *http.Request) error {
	pid, vid := r.PathValue("pid"), r.PathValue("vid")

	var body renderRequestBody
	if err := readJSON(r, &body); err != nil {
		return err
	}
	jobReq, err := validateRenderRequest(body)
	if err != nil {
		return err
	}
	if s.renderer == nil {
		return internalErr("renderer is not configured (missing Vertex AI credentials)")
	}

	project, err := s.repo.GetProject(pid)
	if err != nil {
		return mapStoreErr(err, "project %s not found", pid)
	}
	view := findViewIn(project, vid)
	if view == nil {
		return notFoundErr("view %s not found", vid)
	}
	if !view.HasScreenshot {
		return badRequest("view %s has no screenshot", vid)
	}

	s.ensureInventory(r.Context(), project, view, r.URL.Query().Get("lang"))

	resp, _, err := s.launchRenderJob(r.Context(), project, pid, vid, jobReq)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, resp)
	return nil
}

// ensureInventory builds the view's object and material list when it has none,
// so a render never goes out without the checklist and the material spec the
// prompt relies on. It is best effort: a render must not fail because the
// cheap text call did, so any problem is logged and the render carries on with
// the placeholder, exactly as it did before this existed. It runs once per
// job, before the per-variation tasks are queued, so variations share one
// list. The list is saved on the view, where the user can see and edit it.
func (s *Server) ensureInventory(ctx context.Context, project *store.Project, view *store.View, lang string) {
	if strings.TrimSpace(view.Inventory) != "" || s.textModel == nil {
		return
	}
	// Skip the call when the render is about to be refused for lack of credits.
	if err := s.requireInventoryCredit(project.ID); err != nil {
		return
	}
	text, err := s.generateAndStoreInventory(ctx, project, view, lang)
	if err != nil {
		log.Printf("render: generating missing inventory project=%s view=%s: %v", project.ID, view.ID, err)
		return
	}
	view.Inventory = text
}

// launchRenderJob is the part of starting a job that a render and an edit
// share: debit the owner, persist the job, enqueue one task per variation.
// The price is the source's model+resolution price per variation, so an edit
// costs what a render of the same model and resolution costs.
//
// queued reports whether a task was handed to the queue: once it was, a worker
// may read whatever the job refers to at any moment, so a caller that made
// anything for the job (an edit's region bitmaps) must only clean it up when
// queued is false.
func (s *Server) launchRenderJob(ctx context.Context, project *store.Project, pid, vid string, jobReq store.RenderJobRequest) (resp *renderJobResponse, queued bool, err error) {
	// Debit the project owner's balance atomically before the job even
	// exists, so two concurrent renders can never both pass a check against
	// a balance that's about to go negative - see API_CONTRACT.md's credits
	// section. authEnabled mirrors every other dev-bypass in this package
	// (see requireAuth): with CLERK_SECRET_KEY unset, nothing is charged,
	// unitsPerVar stays 0, and nothing is ever refunded either.
	authEnabled := s.cfg.Auth.ClerkSecretKey != ""
	jobID := uuid.NewString()
	var unitsPerVar int64
	if authEnabled {
		unitsPerVar = unitsPerVariation(jobReq.Model, jobReq.Resolution)
		total := unitsPerVar * int64(jobReq.Variations)
		pidCopy, jobIDCopy := pid, jobID
		if _, err := s.repo.AdjustCredits(project.OwnerID, -total, store.CreditLedgerEntry{
			Reason:    store.CreditReasonRender,
			ProjectID: &pidCopy,
			JobID:     &jobIDCopy,
		}); err != nil {
			if errors.Is(err, store.ErrInsufficientCredits) {
				return nil, false, insufficientCreditsErr()
			}
			return nil, false, internalErr("charging credits: %v", err)
		}
	}

	now := time.Now().UTC()
	variations := make([]store.RenderJobVariation, jobReq.Variations)
	for i := range variations {
		variations[i] = store.RenderJobVariation{Status: store.RenderJobQueued}
	}
	job := &store.RenderJob{
		ID:                       jobID,
		ViewID:                   vid,
		CreatedAt:                now,
		UpdatedAt:                now,
		Request:                  jobReq,
		Variations:               variations,
		ChargedUnitsPerVariation: unitsPerVar,
	}
	created, err := s.repo.CreateRenderJob(pid, job)
	if err != nil {
		// The debit above already happened and nothing was queued: refund it
		// all back.
		s.refundUnits(project.OwnerID, pid, jobID, unitsPerVar*int64(jobReq.Variations), "job creation failed")
		return nil, false, mapStoreErr(err, "project %s not found", pid)
	}

	for i := range created.Variations {
		task := jobs.Task{ProjectID: pid, JobID: created.ID, Variation: i}
		if err := s.queue.Enqueue(ctx, task); err != nil {
			// Per the contract: mark the job failed (this variation and any
			// after it that never got a task) and report 502 - never leave a
			// variation stuck "queued" with nothing that will ever run it.
			// failRemainingVariations also refunds their charge, exactly once.
			s.failRemainingVariations(pid, created.ID, i, fmt.Sprintf("queueing render: %v", err))
			return nil, queued, badGateway("queueing render: %v", err)
		}
		queued = true
	}

	resp, err = s.buildRenderJobResponse(pid, created)
	return resp, queued, err
}

// failRemainingVariations marks every variation from idx onward that is
// still queued as failed with msg - used when enqueueing itself fails
// partway through, so no variation is left queued with no task that will
// ever claim it - and refunds their charged credits, exactly once: each
// variation's Refunded flag is claimed inside this same UpdateRenderJob call,
// so a variation already failed/refunded by something else is left alone.
func (s *Server) failRemainingVariations(pid, jid string, from int, msg string) {
	var refundUnitsTotal int64
	_, err := s.repo.UpdateRenderJob(pid, jid, func(j *store.RenderJob) error {
		refundUnitsTotal = 0
		for i := from; i < len(j.Variations); i++ {
			if j.Variations[i].Status != store.RenderJobQueued {
				continue
			}
			j.Variations[i].Status = store.RenderJobFailed
			errCopy := msg
			j.Variations[i].Error = &errCopy
			if !j.Variations[i].Refunded && j.ChargedUnitsPerVariation > 0 {
				j.Variations[i].Refunded = true
				refundUnitsTotal += j.ChargedUnitsPerVariation
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("render: recording enqueue failure project=%s job=%s: %v", pid, jid, err)
		return
	}
	if refundUnitsTotal == 0 {
		return
	}
	ownerID, err := s.repo.ProjectOwner(pid)
	if err != nil {
		log.Printf("render: refunding credits after enqueue failure project=%s job=%s: %v", pid, jid, err)
		return
	}
	s.refundUnits(ownerID, pid, jid, refundUnitsTotal, fmt.Sprintf("queueing failed: %s", msg))
}

// refundUnits credits units back to ownerID with reason "refund", logging
// (never failing its caller - there's no HTTP response left to report
// through by the time this runs) if the write itself fails. units <= 0 is a
// no-op: auth was disabled, or nothing was ever charged.
func (s *Server) refundUnits(ownerID, pid, jid string, units int64, note string) {
	if units <= 0 {
		return
	}
	pidCopy, jidCopy := pid, jid
	if _, err := s.repo.AdjustCredits(ownerID, units, store.CreditLedgerEntry{
		Reason:    store.CreditReasonRefund,
		ProjectID: &pidCopy,
		JobID:     &jidCopy,
		Note:      note,
	}); err != nil {
		log.Printf("render: refunding credits owner=%s project=%s job=%s units=%d: %v", ownerID, pid, jid, units, err)
	}
}

// renderJobResponse is the JSON shape GET render-jobs/{jid} (and the 202
// from startRender) return - see API_CONTRACT.md's RenderJob.
type renderJobResponse struct {
	ID         string                   `json:"id"`
	ViewID     string                   `json:"viewId"`
	Status     store.RenderJobStatus    `json:"status"`
	CreatedAt  time.Time                `json:"createdAt"`
	UpdatedAt  time.Time                `json:"updatedAt"`
	Request    store.RenderJobRequest   `json:"request"`
	Variations []renderJobVariationView `json:"variations"`
	Renders    []*store.Render          `json:"renders"`
	Error      *string                  `json:"error,omitempty"`
	// CreditsCharged is the credit cost of the whole job as originally
	// charged (unitsPerVariation * variations, converted to credits) - not
	// net of any refunds. 0 when auth was disabled.
	CreditsCharged float64 `json:"creditsCharged"`
}

type renderJobVariationView struct {
	Status   store.RenderJobStatus `json:"status"`
	RenderID *string               `json:"renderId,omitempty"`
	Error    *string               `json:"error,omitempty"`
}

func (s *Server) getRenderJob(w http.ResponseWriter, r *http.Request) error {
	pid, jid := r.PathValue("pid"), r.PathValue("jid")
	job, err := s.repo.GetRenderJob(pid, jid)
	if err != nil {
		return mapStoreErr(err, "render job %s not found", jid)
	}
	resp, err := s.buildRenderJobResponse(pid, job)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

// jobState is a job's variations and overall status as reported to the
// client, derived from what is stored.
type jobState struct {
	variations []renderJobVariationView
	status     store.RenderJobStatus
	// firstErr is the first failed variation's error, set only when the job as
	// a whole is failed.
	firstErr *string
}

// deriveJobState computes the job's overall status and per-variation view,
// applying the stale-running rule (a variation stuck "running" past
// renderTimeoutSec+120s is reported as failed - its worker died without
// marking it terminal) at read time. Nothing about the stale check is
// persisted; see API_CONTRACT.md.
func (s *Server) deriveJobState(job *store.RenderJob) jobState {
	staleAfter := time.Duration(s.cfg.Server.RenderTimeoutSec)*time.Second + staleRunningGrace
	now := time.Now()

	variations := make([]renderJobVariationView, len(job.Variations))
	var anyQueued, anyRunning, anyDone, anyFailed bool
	var firstFailedErr *string

	for i, v := range job.Variations {
		status, errMsg := v.Status, v.Error
		// A variation waiting for its regeneration task (queued, Attempt > 0)
		// carries RunningAt from the hand-off, so a task that never arrives is
		// given up on the same way a worker that never finished is.
		inFlight := status == store.RenderJobRunning || (status == store.RenderJobQueued && v.Attempt > 0)
		if inFlight && v.RunningAt != nil && now.Sub(*v.RunningAt) > staleAfter {
			status = store.RenderJobFailed
			msg := staleRunningError
			errMsg = &msg
		}
		variations[i] = renderJobVariationView{Status: status, RenderID: v.RenderID, Error: errMsg}

		switch status {
		case store.RenderJobQueued:
			anyQueued = true
		case store.RenderJobRunning:
			anyRunning = true
		case store.RenderJobDone:
			anyDone = true
		case store.RenderJobFailed:
			anyFailed = true
			if firstFailedErr == nil {
				firstFailedErr = errMsg
			}
		}
	}

	var status store.RenderJobStatus
	switch {
	case anyQueued || anyRunning:
		if anyRunning || anyDone || anyFailed {
			status = store.RenderJobRunning
		} else {
			status = store.RenderJobQueued
		}
	case anyDone:
		status = store.RenderJobDone
	default:
		status = store.RenderJobFailed
	}

	state := jobState{variations: variations, status: status}
	if status == store.RenderJobFailed {
		state.firstErr = firstFailedErr
	}
	return state
}

// buildRenderJobResponse derives the job's overall status, its renders list
// and its top-level error from its variations - see deriveJobState and
// API_CONTRACT.md.
func (s *Server) buildRenderJobResponse(pid string, job *store.RenderJob) (*renderJobResponse, error) {
	state := s.deriveJobState(job)

	var renders []*store.Render
	for _, v := range state.variations {
		if v.Status != store.RenderJobDone || v.RenderID == nil {
			continue
		}
		rec, err := s.repo.FindRender(pid, *v.RenderID)
		if err != nil {
			return nil, mapStoreErr(err, "render %s not found", *v.RenderID)
		}
		renders = append(renders, rec)
	}

	return &renderJobResponse{
		ID:             job.ID,
		ViewID:         job.ViewID,
		Status:         state.status,
		CreatedAt:      job.CreatedAt,
		UpdatedAt:      job.UpdatedAt,
		Request:        job.Request,
		Variations:     state.variations,
		Renders:        nonNilRenders(renders),
		Error:          state.firstErr,
		CreditsCharged: unitsToCredits(job.ChargedUnitsPerVariation * int64(len(job.Variations))),
	}, nil
}

func nonNilRenders(rs []*store.Render) []*store.Render {
	if rs == nil {
		return []*store.Render{}
	}
	return rs
}

// renderTaskBody is the JSON body POST /internal/render-tasks receives -
// see API_CONTRACT.md.
type renderTaskBody struct {
	ProjectID string `json:"projectId"`
	JobID     string `json:"jobId"`
	Variation int    `json:"variation"`
	Attempt   int    `json:"attempt"`
}

// handleRenderTask is the worker route Cloud Tasks calls. Per the contract
// it always answers 2xx once auth passes and the body parses - a render
// failure is recorded on the job, never signalled back to Cloud Tasks,
// since a retry would bill a second model call for work already accounted
// for. It runs the variation synchronously (not in a goroutine): this
// request already has Cloud Tasks' own generous DispatchDeadline behind it,
// and running inline means we control exactly when the response is sent.
func (s *Server) handleRenderTask(w http.ResponseWriter, r *http.Request) error {
	if err := s.verifyWorkerRequest(r); err != nil {
		return err
	}
	var body renderTaskBody
	if err := readJSON(r, &body); err != nil {
		return err
	}
	s.RunRenderVariation(r.Context(), jobs.Task{
		ProjectID: body.ProjectID,
		JobID:     body.JobID,
		Variation: body.Variation,
		Attempt:   body.Attempt,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	return nil
}

// verifyWorkerRequest validates the bearer token Cloud Tasks attaches
// against cfg.Jobs.WorkerAudience and requires it to identify exactly
// cfg.Jobs.InvokerServiceAccount, with a verified email - see
// API_CONTRACT.md's POST /internal/render-tasks auth section.
func (s *Server) verifyWorkerRequest(r *http.Request) error {
	token := bearerToken(r)
	if token == "" {
		return unauthorized("missing bearer token")
	}
	claims, err := s.oidcValidator(r.Context(), token, s.cfg.Jobs.WorkerAudience)
	if err != nil {
		return unauthorized("invalid OIDC token: %v", err)
	}
	if !claims.EmailVerified || claims.Email != s.cfg.Jobs.InvokerServiceAccount {
		return unauthorized("token not authorized for the render worker")
	}
	return nil
}

// renderAssembly is everything RunRenderVariation needs beyond the model
// call itself: the assembled RenderRequest plus the context
// runPreservationCheck needs (screenshot image/edges/bytes, mask bitmaps,
// view) and the bits that feed a Render's own fields (region count, anchor
// used). Building it is the "assembly" step from the old synchronous
// handler, now run once per variation (each is its own worker call, so
// there's no longer a shared assembly to reuse across variations the way
// the old single-request loop did).
type renderAssembly struct {
	view            *store.View
	screenshotImg   image.Image
	screenshotEdges *image.Gray
	screenshotBlob  store.Blob
	maskBitmaps     []image.Image
	// assets are the distinct assets used, for the asset color-match report.
	assets          []regionAsset
	qualifyingCount int
	hasAnchor       bool
	modelID         string
	req             renderpkg.RenderRequest
	// edit is set instead of the screenshot-derived fields above when the job
	// edits an existing Render (see edit.go).
	edit *editAssembly
	// upscale is set instead of them when the job makes a 4K version of an
	// existing Render (see upscale.go).
	upscale *upscaleAssembly
	// assemblyMs is the setup cost (project/view lookup, screenshot decode,
	// region/edge maps, prompt build) attributed to this variation's TotalMs,
	// mirroring the old handler's assemblyMs semantics.
	assemblyMs int64
}

// assembleRenderRequest reproduces the pre-model-call half of the old
// synchronous renderView handler: load project/view, build the region map,
// edge map, style-anchor reference and asset reference photos, and render
// the prompt. Returns the same *httpError-shaped errors the old handler did
// (via badRequest/notFoundErr/internalErr), which RunRenderVariation turns
// into a variation failure rather than an HTTP response.
func (s *Server) assembleRenderRequest(pid, vid string, jobReq store.RenderJobRequest) (*renderAssembly, error) {
	start := time.Now()

	// withLibraryAssets so qualifyingMasks/buildRegionSet below resolve
	// against the owner's asset library, exactly like the getProject
	// handler - this is the worker path, so it needs its own call (see
	// API_CONTRACT.md's asset library section).
	project, err := s.withLibraryAssets(s.repo.GetProject(pid))
	if err != nil {
		return nil, mapStoreErr(err, "project %s not found", pid)
	}
	view := findViewIn(project, vid)
	if view == nil {
		return nil, notFoundErr("view %s not found", vid)
	}
	if !view.HasScreenshot {
		return nil, badRequest("view %s has no screenshot", vid)
	}

	screenshotBlob, ok := s.blobs.GetBlob(view.ScreenshotImageID)
	if !ok {
		return nil, internalErr("screenshot blob missing for view %s", vid)
	}
	screenshotImg, _, err := imageutil.Decode(screenshotBlob.Data)
	if err != nil {
		return nil, internalErr("decoding stored screenshot: %v", err)
	}

	regions, err := s.buildRegionSet(screenshotImg, qualifyingMasks(project, view), vid)
	if err != nil {
		return nil, err
	}
	images := [][]byte{screenshotBlob.Data}
	var maskBitmaps []image.Image

	hasRegionMap := len(regions.assets) > 0
	if hasRegionMap {
		images = append(images, regions.mapPNG)
		maskBitmaps = regions.bitmaps
	}

	screenshotEdges := geometry.EdgeMap(screenshotImg)
	edgeMapPNG, err := imageutil.EncodePNG(screenshotEdges)
	if err != nil {
		return nil, internalErr("encoding edge map: %v", err)
	}
	images = append(images, edgeMapPNG)
	edgeMapIndex := len(images)

	hasAnchor, anchorIndex := false, 0
	if project.StyleAnchorRenderID != nil {
		if anchorRender, err := s.repo.FindRender(pid, *project.StyleAnchorRenderID); err == nil {
			if anchorBlob, ok := s.blobs.GetBlob(anchorRender.ResultImageID); ok {
				images = append(images, anchorBlob.Data)
				hasAnchor = true
				anchorIndex = len(images)
			}
		}
	}

	var assetRefs []renderpkg.AssetRef
	for _, ra := range regions.assets {
		images = append(images, ra.ref)
		assetRefs = append(assetRefs, renderpkg.AssetRef{
			Index:       len(images),
			Name:        ra.asset.Name,
			Description: ra.asset.Description,
			ColorName:   ra.swatch.Name,
		})
	}

	promptData := renderpkg.TemplateData{
		HasRegionMap:      hasRegionMap,
		EdgeMapIndex:      edgeMapIndex,
		HasAnchor:         hasAnchor,
		AnchorIndex:       anchorIndex,
		AssetRefs:         assetRefs,
		Scene:             string(project.Style.Scene),
		Lighting:          string(project.Style.Lighting),
		LightDirection:    project.Style.LightDirection,
		MaterialNotes:     project.Style.MaterialNotes,
		ExtraInstructions: project.Style.ExtraInstructions,
		Inventory:         inventoryOrPlaceholder(view.Inventory),
	}
	promptData.InteriorLights(string(project.Style.InteriorLights))
	prompt, err := renderpkg.RenderPrompt(s.promptPath, promptData)
	if err != nil {
		return nil, internalErr("building prompt: %v", err)
	}

	modelID := s.cfg.Models.ProImage
	if jobReq.Model == store.ModelFlash {
		modelID = s.cfg.Models.FlashImage
	}
	aspectRatio := renderpkg.PickAspectRatio(view.Width, view.Height)

	renderReq := renderpkg.RenderRequest{
		ModelID:     modelID,
		AspectRatio: aspectRatio,
		ImageSize:   string(jobReq.Resolution),
		Images:      images,
		Prompt:      prompt,
	}

	return &renderAssembly{
		view:            view,
		screenshotImg:   screenshotImg,
		screenshotEdges: screenshotEdges,
		screenshotBlob:  screenshotBlob,
		maskBitmaps:     maskBitmaps,
		assets:          regions.assets,
		qualifyingCount: regions.masks,
		hasAnchor:       hasAnchor,
		modelID:         modelID,
		req:             renderReq,
		// assemblyMs is the shared setup cost above - attributed to this
		// variation's TotalMs, matching the old handler's semantics.
		assemblyMs: time.Since(start).Milliseconds(),
	}, nil
}

// RunRenderVariation is the worker: it claims one queued variation (a
// no-op if it's already running/terminal - the idempotent-redelivery case),
// assembles the render request, calls the model, stores the result, runs
// the optional preservation check, appends the Render to the view, and
// marks the variation done or failed. It never returns an error - every
// outcome, including its own bugs, is recorded on the RenderJob via
// failVariation/completeVariation so a stuck task never leaves the job
// hanging; see buildRenderJobResponse's stale-running rule as the last
// resort if even that fails.
//
// Exported so cmd/server/main.go can wire it as the jobs.Inline handler and
// so it satisfies jobs.Handler directly.
func (s *Server) RunRenderVariation(ctx context.Context, task jobs.Task) {
	pid, jid, idx, attempt := task.ProjectID, task.JobID, task.Variation, task.Attempt

	// claimed (not just "the job now shows running") is what tells this
	// delivery apart from a concurrent one: UpdateRenderJob's fn runs
	// exactly once per delivery under the store's lock, but if two
	// deliveries race, the loser also observes Status == running -
	// possibly its own competitor's write - and job.Variations[idx].Status
	// alone can't tell it apart from actually having won the claim.
	//
	// fn may also run more than once for one delivery (Firestore retries a
	// transaction on contention), so claimed is reset on every attempt and
	// only the attempt that actually commits decides it.
	var claimed bool
	job, err := s.repo.UpdateRenderJob(pid, jid, func(j *store.RenderJob) error {
		claimed = false
		if idx < 0 || idx >= len(j.Variations) {
			return fmt.Errorf("variation index %d out of range (job has %d)", idx, len(j.Variations))
		}
		if j.Variations[idx].Status != store.RenderJobQueued || j.Variations[idx].Attempt != task.Attempt {
			// Already claimed/terminal, or a redelivered task of an earlier
			// attempt: leave it alone, checked below.
			return nil
		}
		now := time.Now().UTC()
		j.Variations[idx].Status = store.RenderJobRunning
		j.Variations[idx].RunningAt = &now
		claimed = true
		return nil
	})
	if err != nil {
		log.Printf("render worker: claiming project=%s job=%s variation=%d: %v", pid, jid, idx, err)
		return
	}
	if !claimed {
		// Redelivered task for a variation that's already running (claimed by
		// an earlier delivery) or terminal (already done/failed) - idempotent
		// no-op, exactly as the contract requires.
		return
	}

	variationStart := time.Now()
	variations := job.Request.Variations
	// held is the best flagged render of the earlier attempts, if any.
	held := job.Variations[idx].Held

	var assembly *renderAssembly
	switch {
	case job.Request.Edit != nil:
		// The region bitmaps only exist for this job; whichever way the
		// variation ends they are no longer needed.
		defer s.deleteEditBlobs(job.Request.Edit)
		assembly, err = s.assembleEditRequest(pid, job)
	case job.Request.Upscale != nil:
		assembly, err = s.assembleUpscaleRequest(pid, job)
	default:
		assembly, err = s.assembleRenderRequest(pid, job.ViewID, job.Request)
	}
	if err != nil {
		log.Printf("render worker: assembling project=%s job=%s view=%s variation=%d/%d: %v",
			pid, jid, job.ViewID, idx+1, variations, err)
		s.failOrSalvage(pid, jid, job.ViewID, idx, held, err.Error(), nil)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.Server.RenderTimeoutSec)*time.Second)
	defer cancel()

	pricingTable := s.pricingTable()
	// job.Request.Model is already validated to be "pro"/"flash" (see
	// validateRenderRequest), so this always resolves; an unpriced
	// resolution still yields ok=false further down.
	imgPricing, _ := pricingTable.ImagePricingFor(string(job.Request.Model))

	result, err := s.renderer.Render(ctx, assembly.req)
	imageCallMs := time.Since(variationStart).Milliseconds()
	if err != nil {
		// A failed call still bills its input (and sometimes thinking)
		// tokens - result carries whatever usage the model reported even
		// though no image came back. Log that estimated cost for
		// visibility; persisting it isn't in scope yet (it will come with a
		// jobs/credits system).
		failCost, failCostOK := renderpkg.ImageCallCost(imgPricing, string(job.Request.Resolution), result.PromptTokens, result.TextOutputTokens, result.ThoughtsTokens)
		failCostDisplay := "n/a"
		if failCostOK {
			failCostDisplay = fmt.Sprintf("%.4f", failCost)
		}
		log.Printf("render: failed attempt project=%s job=%s view=%s variation=%d/%d model=%s resolution=%s promptTokens=%d outputTokens=%d thoughtsTokens=%d costUsd=%s error=%v",
			pid, jid, job.ViewID, idx+1, variations, assembly.modelID, job.Request.Resolution, result.PromptTokens, result.TextOutputTokens, result.ThoughtsTokens, failCostDisplay, err)

		msg := fmt.Sprintf("render failed: %v", err)
		if ctx.Err() == context.DeadlineExceeded {
			msg = fmt.Sprintf("render timed out after %ds", s.cfg.Server.RenderTimeoutSec)
		}
		var lostCost *float64
		if failCostOK {
			lostCost = &failCost
		}
		s.failOrSalvage(pid, jid, job.ViewID, idx, held, msg, lostCost)
		return
	}

	var editDrift []store.EditRegionDrift
	if assembly.edit != nil {
		// Keep the source's own pixels everywhere outside the edit regions.
		composited, drift, err := assembly.edit.composite(result.ImageData)
		if err != nil {
			log.Printf("render worker: compositing edit project=%s job=%s view=%s: %v", pid, jid, job.ViewID, err)
			s.failVariation(pid, jid, idx, fmt.Sprintf("compositing edit: %v", err))
			return
		}
		result.ImageData, result.MIMEType = composited, "image/png"
		editDrift = drift
	}
	if assembly.upscale != nil {
		// Keep the source's colour and lighting; the model only adds detail.
		finished, err := assembly.upscale.finish(result.ImageData)
		if err != nil {
			log.Printf("render worker: finishing upscale project=%s job=%s view=%s: %v", pid, jid, job.ViewID, err)
			s.failVariation(pid, jid, idx, fmt.Sprintf("upscale failed: %v", err))
			return
		}
		result.ImageData, result.MIMEType = finished, "image/png"
	}

	mimeType := result.MIMEType
	if mimeType == "" {
		mimeType = "image/png"
	}
	resultImageID, err := s.blobs.PutBlob(result.ImageData, mimeType)
	if err != nil {
		lostCost, lostCostOK := renderpkg.ImageCallCost(imgPricing, string(job.Request.Resolution), result.PromptTokens, result.TextOutputTokens, result.ThoughtsTokens)
		log.Printf("render: failed attempt project=%s job=%s view=%s variation=%d/%d model=%s resolution=%s stage=store costUsd=%.4f error=%v",
			pid, jid, job.ViewID, idx+1, variations, assembly.modelID, job.Request.Resolution, lostCost, err)
		var lost *float64
		if lostCostOK {
			lost = &lostCost
		}
		s.failOrSalvage(pid, jid, job.ViewID, idx, held, fmt.Sprintf("storing render result: %v", err), lost)
		return
	}

	// promptTokens/outputTokens/thoughtsTokens accumulate across both calls
	// this render makes: the image call, and (if preservationCheck is on)
	// the text-model preservation check. outputTokens is TEXT output only -
	// it never includes the image call's own generated-image tokens, which
	// are priced per-image instead (see render/pricing.go and
	// RenderResult.TextOutputTokens).
	promptTokens := result.PromptTokens
	outputTokens := result.TextOutputTokens
	thoughtsTokens := result.ThoughtsTokens

	imageCost, haveCost := renderpkg.ImageCallCost(imgPricing, string(job.Request.Resolution), result.PromptTokens, result.TextOutputTokens, result.ThoughtsTokens)
	totalCost := imageCost

	rec := &store.Render{
		ID:            uuid.NewString(),
		CreatedAt:     time.Now().UTC(),
		Model:         job.Request.Model,
		Resolution:    job.Request.Resolution,
		AnchorUsed:    assembly.hasAnchor,
		RegionCount:   assembly.qualifyingCount,
		ResultImageID: resultImageID,
	}
	if assembly.edit != nil {
		rec.SourceRenderID = assembly.edit.sourceRenderID
		rec.EditInstructions = assembly.edit.instructions
		rec.EditDrift = editDrift
	}
	if assembly.upscale != nil {
		rec.UpscaledFromRenderID = assembly.upscale.sourceRenderID
	}

	if job.Request.PreservationCheck {
		rec.Preservation = s.runPreservationCheck(
			assembly.screenshotImg, assembly.screenshotEdges, result, assembly.view, assembly.maskBitmaps, assembly.assets)
	}

	totalMs := assembly.assemblyMs + time.Since(variationStart).Milliseconds()

	metrics := store.RenderMetrics{
		Model:       assembly.modelID,
		Resolution:  job.Request.Resolution,
		RegionCount: assembly.qualifyingCount,
		AnchorUsed:  assembly.hasAnchor,
		ImageCallMs: imageCallMs,
		TotalMs:     totalMs,
	}
	if promptTokens > 0 || outputTokens > 0 {
		pt, ot := promptTokens, outputTokens
		metrics.PromptTokens = &pt
		metrics.OutputTokens = &ot
	}
	if thoughtsTokens > 0 {
		tt := thoughtsTokens
		metrics.ThoughtsTokens = &tt
	}
	if haveCost {
		metrics.EstimatedCostUsd = &totalCost
	}
	// The render's cost, latency and tokens cover every attempt made for it,
	// the discarded ones included.
	rec.Metrics = metrics
	if held != nil {
		rec.Metrics = accumulateMetrics(held.Metrics, metrics)
	}
	rec.Metrics.Attempts = attempt + 1

	costDisplay := "n/a"
	if metrics.EstimatedCostUsd != nil {
		costDisplay = fmt.Sprintf("%.4f", *metrics.EstimatedCostUsd)
	}
	log.Printf("render: project=%s job=%s view=%s variation=%d/%d model=%s resolution=%s regions=%d anchor=%v imageCallMs=%d totalMs=%d promptTokens=%d outputTokens=%d thoughtsTokens=%d costUsd=%s",
		pid, jid, job.ViewID, idx+1, variations, assembly.modelID, job.Request.Resolution, assembly.qualifyingCount, assembly.hasAnchor, imageCallMs, totalMs, promptTokens, outputTokens, thoughtsTokens, costDisplay)

	// Keep the better of this attempt and the best earlier one. Edits and
	// upscales are never regenerated: they are derived from a render the user
	// already has and carry no check.
	rec = s.settleCandidates(held, rec)
	if assembly.edit == nil && assembly.upscale == nil && attempt < s.cfg.Preservation.MaxRegenerations && flaggedRender(rec) {
		if s.requeueVariation(ctx, pid, jid, idx, attempt, rec) {
			log.Printf("render: regenerating project=%s job=%s view=%s variation=%d/%d attempt=%d edgeScore=%.3f",
				pid, jid, job.ViewID, idx+1, variations, attempt+1, rec.Preservation.EdgeScore)
			return
		}
		// Could not hand the next attempt over: show what we have.
	}

	addedRec, err := s.repo.AddRender(pid, job.ViewID, rec)
	if err != nil {
		log.Printf("render worker: saving render project=%s job=%s view=%s variation=%d/%d: %v",
			pid, jid, job.ViewID, idx+1, variations, err)
		s.failVariation(pid, jid, idx, fmt.Sprintf("saving render: %v", err))
		return
	}

	s.completeVariation(pid, jid, idx, addedRec.ID)
}

// flaggedRender reports whether the preservation check found the render not
// to follow its screenshot.
func flaggedRender(r *store.Render) bool {
	return r.Preservation != nil && r.Preservation.EdgeFlag
}

// settleCandidates picks what to keep out of the best earlier attempt (held,
// may be nil) and the attempt that just finished (cur): cur, unless both are
// flagged and held scored better. The other one's image is deleted - a
// discarded attempt is never shown. The result carries cur's metrics, which
// are already cumulative over all attempts.
func (s *Server) settleCandidates(held, cur *store.Render) *store.Render {
	if held == nil {
		return cur
	}
	winner, loser := cur, held
	if flaggedRender(cur) && held.Preservation != nil && held.Preservation.EdgeScore > cur.Preservation.EdgeScore {
		winner, loser = held, cur
	}
	s.blobs.DeleteBlob(loser.ResultImageID)
	out := *winner
	out.Metrics = cur.Metrics
	out.CreatedAt = cur.CreatedAt
	return &out
}

// requeueVariation hands a variation whose render was flagged over to a fresh
// task for its next attempt, keeping keep (the best render so far, image
// included) aside in case the remaining attempts do no better. It returns
// false, leaving the variation exactly as it was, if the hand-off did not
// happen - the caller then publishes keep.
//
// The variation is put back to "queued" with Attempt+1 before the task is
// enqueued (a task can start the moment it exists), and only a task carrying
// that Attempt can claim it; that persisted counter, checked against
// cfg.Preservation.MaxRegenerations by the caller, is what stops the chain.
func (s *Server) requeueVariation(ctx context.Context, pid, jid string, idx, attempt int, keep *store.Render) bool {
	errStale := errors.New("variation is no longer on this attempt")
	now := time.Now().UTC()
	_, err := s.repo.UpdateRenderJob(pid, jid, func(j *store.RenderJob) error {
		if idx < 0 || idx >= len(j.Variations) {
			return errStale
		}
		v := &j.Variations[idx]
		if v.Status != store.RenderJobRunning || v.Attempt != attempt {
			return errStale
		}
		v.Status = store.RenderJobQueued
		v.RunningAt = &now
		v.Attempt = attempt + 1
		v.Held = keep
		return nil
	})
	if err != nil {
		log.Printf("render worker: requeueing project=%s job=%s variation=%d attempt=%d: %v", pid, jid, idx, attempt, err)
		return false
	}

	// The render timeout may be nearly spent by now, and the hand-off must not
	// be cut short by it.
	enqueueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := s.queue.Enqueue(enqueueCtx, jobs.Task{ProjectID: pid, JobID: jid, Variation: idx, Attempt: attempt + 1}); err != nil {
		log.Printf("render worker: enqueueing regeneration project=%s job=%s variation=%d attempt=%d: %v", pid, jid, idx, attempt+1, err)
		// The variation now says queued but no task exists for it. The caller
		// completes it straight away, which overwrites that.
		return false
	}
	return true
}

// failOrSalvage ends a variation whose attempt failed with msg: it fails (and
// is refunded) if it has nothing to show, but if an earlier attempt left a
// held render - flagged, yet a real render - that one is published instead,
// with the failed attempt counted in its cost when lostCostUsd is known.
func (s *Server) failOrSalvage(pid, jid, viewID string, idx int, held *store.Render, msg string, lostCostUsd *float64) {
	if held == nil {
		s.failVariation(pid, jid, idx, msg)
		return
	}
	out := *held
	out.Metrics = held.Metrics
	out.Metrics.Attempts++
	if lostCostUsd != nil && out.Metrics.EstimatedCostUsd != nil {
		sum := *out.Metrics.EstimatedCostUsd + *lostCostUsd
		out.Metrics.EstimatedCostUsd = &sum
	}
	out.CreatedAt = time.Now().UTC()
	added, err := s.repo.AddRender(pid, viewID, &out)
	if err != nil {
		log.Printf("render worker: saving held render project=%s job=%s variation=%d: %v", pid, jid, idx, err)
		s.failVariation(pid, jid, idx, msg)
		return
	}
	log.Printf("render: attempt failed, keeping the best earlier one project=%s job=%s variation=%d: %s", pid, jid, idx, msg)
	s.completeVariation(pid, jid, idx, added.ID)
}

// accumulateMetrics adds the latency, token and cost figures of one more
// attempt (cur) to those of the attempts before it (prev); the descriptive
// fields are cur's. A missing token count is zero, but the cost is only known
// if it is known for every attempt.
func accumulateMetrics(prev, cur store.RenderMetrics) store.RenderMetrics {
	out := cur
	out.ImageCallMs += prev.ImageCallMs
	out.TotalMs += prev.TotalMs
	out.PromptTokens = sumPtr(prev.PromptTokens, cur.PromptTokens)
	out.OutputTokens = sumPtr(prev.OutputTokens, cur.OutputTokens)
	out.ThoughtsTokens = sumPtr(prev.ThoughtsTokens, cur.ThoughtsTokens)
	out.EstimatedCostUsd = nil
	if prev.EstimatedCostUsd != nil && cur.EstimatedCostUsd != nil {
		sum := *prev.EstimatedCostUsd + *cur.EstimatedCostUsd
		out.EstimatedCostUsd = &sum
	}
	return out
}

func sumPtr[T int32 | float64](a, b *T) *T {
	switch {
	case a == nil && b == nil:
		return nil
	case a == nil:
		v := *b
		return &v
	case b == nil:
		v := *a
		return &v
	}
	v := *a + *b
	return &v
}

// failVariation marks one variation failed with msg and refunds its charged
// credits, exactly once: the claim (Refunded false -> true) happens inside
// the same UpdateRenderJob call as the failed-status write, so a variation
// that's already terminal or already refunded is left alone - no second
// refund is ever issued, even under a redelivered task (RunRenderVariation's
// own claimed-by-status-transition check already keeps this from being
// called twice for one delivery, but the Refunded flag is the belt-and-
// braces guarantee actually promised by API_CONTRACT.md). Logged (not
// returned) on its own storage error, since RunRenderVariation has no caller
// left to report to by this point - see buildRenderJobResponse's
// stale-running rule as the eventual fallback if this write itself never
// lands.
func (s *Server) failVariation(pid, jid string, idx int, msg string) {
	var refund int64
	_, err := s.repo.UpdateRenderJob(pid, jid, func(j *store.RenderJob) error {
		refund = 0
		if idx < 0 || idx >= len(j.Variations) {
			return nil
		}
		v := &j.Variations[idx]
		wasTerminal := v.Status == store.RenderJobDone || v.Status == store.RenderJobFailed
		v.Status = store.RenderJobFailed
		errCopy := msg
		v.Error = &errCopy
		v.RunningAt = nil
		if !wasTerminal && !v.Refunded && j.ChargedUnitsPerVariation > 0 {
			v.Refunded = true
			refund = j.ChargedUnitsPerVariation
		}
		return nil
	})
	if err != nil {
		log.Printf("render worker: recording failure project=%s job=%s variation=%d: %v", pid, jid, idx, err)
		return
	}
	if refund == 0 {
		return
	}
	ownerID, err := s.repo.ProjectOwner(pid)
	if err != nil {
		log.Printf("render worker: refunding credits project=%s job=%s variation=%d: %v", pid, jid, idx, err)
		return
	}
	s.refundUnits(ownerID, pid, jid, refund, fmt.Sprintf("variation %d failed: %s", idx, msg))
}

// completeVariation marks one variation done, pointing at the Render that
// was just created.
func (s *Server) completeVariation(pid, jid string, idx int, renderID string) {
	_, err := s.repo.UpdateRenderJob(pid, jid, func(j *store.RenderJob) error {
		if idx < 0 || idx >= len(j.Variations) {
			return nil
		}
		j.Variations[idx].Status = store.RenderJobDone
		id := renderID
		j.Variations[idx].RenderID = &id
		j.Variations[idx].RunningAt = nil
		return nil
	})
	if err != nil {
		log.Printf("render worker: recording completion project=%s job=%s variation=%d: %v", pid, jid, idx, err)
	}
}

// qualifyingMasks returns the view's masks that can steer a render: visible,
// painted, bound to an asset that still exists, and that asset has a
// reference photo. A region without a photo would be tinted on the region map
// but have no product image or mapping line in the prompt, so it is skipped
// rather than left as an unexplained colored area.
func qualifyingMasks(project *store.Project, view *store.View) []qualifyingMask {
	var out []qualifyingMask
	for _, m := range view.Masks {
		if m.Hidden || !m.HasBitmap || m.AssetID == nil {
			continue
		}
		asset := findAssetIn(project, *m.AssetID)
		if asset == nil || !asset.HasReferenceImage {
			continue
		}
		out = append(out, qualifyingMask{mask: m, asset: asset})
	}
	return out
}

// regionAsset is one distinct asset steering a render: its reference photo and
// the palette swatch that marks its regions on the region map and names them
// in the prompt.
type regionAsset struct {
	asset  *store.Asset
	ref    []byte
	swatch geometry.Swatch
	// bitmaps are the decoded bitmaps of this asset's masks.
	bitmaps []image.Image
}

// regionSet is everything the masks contribute to a render request.
type regionSet struct {
	mapPNG []byte
	// bitmaps are the decoded bitmaps of the masks actually used, needed later
	// to build the preservation-check exclusion mask.
	bitmaps []image.Image
	// assets are the distinct assets used, in order of first appearance.
	assets []regionAsset
	// masks is how many masks were used (the render's regionCount metric).
	masks int
}

// baseRenderImages is how many images always precede the asset reference
// photos: screenshot, region map, edge map and the optional style anchor.
const baseRenderImages = 4

// buildRegionSet turns the qualifying masks into the region map and the asset
// reference photos, so the two always agree: an asset is used only if its
// reference photo loads, and every used asset gets its own palette swatch.
//
// Regions are painted from geometry.RenderPalette, not from the asset's own
// display color: the prompt names the swatch ("magenta tinted region"), and a
// hex code would never match the semi-transparent tint the model actually
// sees. AssignSwatches steers each asset away from hues already present in the
// scene so a real object of that color is not mistaken for a region.
//
// The palette also caps how many distinct assets one request can use, which
// keeps the total within MaxInputImages; assets beyond the cap (the
// lowest-priority, last-encountered) are dropped with their masks and logged.
func (s *Server) buildRegionSet(screenshotImg image.Image, qualifying []qualifyingMask, vid string) (*regionSet, error) {
	type group struct {
		asset   *store.Asset
		ref     []byte
		bitmaps []image.Image
	}
	type usedMask struct {
		bitmap image.Image
		group  int
	}
	maxAssets := min(len(geometry.RenderPalette), renderpkg.MaxInputImages-baseRenderImages)

	var groups []*group
	groupOf := map[string]int{}
	skipped := map[string]bool{}
	var used []usedMask
	dropped := 0
	for _, q := range qualifying {
		id := q.asset.ID
		if skipped[id] {
			continue
		}
		gi, known := groupOf[id]
		if !known {
			refBlob, ok := s.blobs.GetBlob(q.asset.ReferenceImageID)
			if !ok {
				log.Printf("render: skipping asset %s in view %s: reference photo blob is missing", id, vid)
				skipped[id] = true
				continue
			}
			if len(groups) >= maxAssets {
				dropped++
				skipped[id] = true
				continue
			}
			gi = len(groups)
			groupOf[id] = gi
			groups = append(groups, &group{asset: q.asset, ref: refBlob.Data})
		}
		bitmapBlob, ok := s.blobs.GetBlob(q.mask.ID)
		if !ok {
			return nil, internalErr("mask bitmap blob missing for mask %s", q.mask.ID)
		}
		bitmapImg, _, err := imageutil.Decode(bitmapBlob.Data)
		if err != nil {
			return nil, internalErr("decoding mask bitmap for mask %s: %v", q.mask.ID, err)
		}
		groups[gi].bitmaps = append(groups[gi].bitmaps, bitmapImg)
		used = append(used, usedMask{bitmap: bitmapImg, group: gi})
	}
	if dropped > 0 {
		log.Printf("render: dropped %d asset(s) beyond the %d-asset limit, with their masks (view %s)", dropped, maxAssets, vid)
	}
	if len(groups) == 0 {
		return &regionSet{}, nil
	}

	bitmapGroups := make([][]image.Image, len(groups))
	for i, g := range groups {
		bitmapGroups[i] = g.bitmaps
	}
	swatches, err := geometry.AssignSwatches(screenshotImg, bitmapGroups, geometry.RenderPalette)
	if err != nil {
		return nil, internalErr("choosing region colors: %v", err)
	}

	regions := make([]geometry.Region, len(used))
	bitmaps := make([]image.Image, len(used))
	for i, u := range used {
		regions[i] = geometry.Region{Bitmap: u.bitmap, Color: swatches[u.group].Color}
		bitmaps[i] = u.bitmap
	}
	regionMapImg, err := geometry.CompositeRegionMap(screenshotImg, regions)
	if err != nil {
		return nil, internalErr("compositing region map: %v", err)
	}
	regionMapPNG, err := imageutil.EncodePNG(regionMapImg)
	if err != nil {
		return nil, internalErr("encoding region map: %v", err)
	}

	assets := make([]regionAsset, len(groups))
	for i, g := range groups {
		assets[i] = regionAsset{asset: g.asset, ref: g.ref, swatch: swatches[i], bitmaps: g.bitmaps}
	}
	return &regionSet{mapPNG: regionMapPNG, bitmaps: bitmaps, assets: assets, masks: len(used)}, nil
}

// runPreservationCheck computes the edge-IoU score between the screenshot
// and the render result. It never fails the overall render - any error here
// is logged and reflected in the report instead.
func (s *Server) runPreservationCheck(
	screenshotImg image.Image,
	screenshotEdges *image.Gray,
	result renderpkg.RenderResult,
	view *store.View,
	maskBitmaps []image.Image,
	assets []regionAsset,
) *store.PreservationReport {
	report := &store.PreservationReport{}

	resultImg, _, err := imageutil.Decode(result.ImageData)
	if err != nil {
		log.Printf("preservation check: decoding render result: %v", err)
		return report
	}

	resized := geometry.Resize(resultImg, view.Width, view.Height)
	resultEdges := geometry.EdgeMap(resized)

	var exclude *image.Gray
	if len(maskBitmaps) > 0 {
		exclude = geometry.UnionMask(screenshotImg.Bounds(), maskBitmaps)
	}

	score, err := geometry.EdgeIoU(screenshotEdges, resultEdges, s.cfg.Preservation.EdgeDilationPx, exclude)
	if err != nil {
		log.Printf("preservation check: computing edge IoU: %v", err)
		return report
	}
	report.EdgeScore = score
	report.EdgeFlag = score < s.cfg.Preservation.EdgeScoreFlagThreshold
	report.AssetColors = assetColorScores(resized, assets)
	return report
}

// assetColorScores measures, per asset, how close the colors of its region in
// the render are to its reference photo. An asset whose photo cannot be read,
// or whose region has no pixels, is left out; nothing here can fail the render.
func assetColorScores(render image.Image, assets []regionAsset) []store.AssetColorScore {
	var out []store.AssetColorScore
	for _, a := range assets {
		ref, _, err := imageutil.Decode(a.ref)
		if err != nil {
			log.Printf("asset color check: decoding reference photo of asset %s: %v", a.asset.ID, err)
			continue
		}
		mask := geometry.UnionMask(render.Bounds(), a.bitmaps)
		dist, ok := geometry.RegionColorDistance(render, mask, ref)
		if !ok {
			continue
		}
		out = append(out, store.AssetColorScore{AssetID: a.asset.ID, AssetName: a.asset.Name, Distance: dist})
	}
	return out
}

func findViewIn(p *store.Project, vid string) *store.View {
	for _, v := range p.Views {
		if v.ID == vid {
			return v
		}
	}
	return nil
}

func findAssetIn(p *store.Project, aid string) *store.Asset {
	for _, a := range p.Assets {
		if a.ID == aid {
			return a
		}
	}
	return nil
}

func inventoryOrPlaceholder(inv string) string {
	if strings.TrimSpace(inv) == "" {
		return "(no object inventory recorded for this view)"
	}
	return inv
}

func (s *Server) pricingTable() renderpkg.PricingTable {
	return renderpkg.PricingTable{
		ProImage: renderpkg.ImagePricing{
			PerImage: s.cfg.Pricing.ProImage.PerImage,
			TokenPricing: renderpkg.TokenPricing{
				InputPerMTok:  s.cfg.Pricing.ProImage.InputPerMTok,
				OutputPerMTok: s.cfg.Pricing.ProImage.OutputPerMTok,
			},
		},
		FlashImage: renderpkg.ImagePricing{
			PerImage: s.cfg.Pricing.FlashImage.PerImage,
			TokenPricing: renderpkg.TokenPricing{
				InputPerMTok:  s.cfg.Pricing.FlashImage.InputPerMTok,
				OutputPerMTok: s.cfg.Pricing.FlashImage.OutputPerMTok,
			},
		},
		Text: renderpkg.TokenPricing{
			InputPerMTok:  s.cfg.Pricing.Text.InputPerMTok,
			OutputPerMTok: s.cfg.Pricing.Text.OutputPerMTok,
		},
		UsdToBrl: s.cfg.Pricing.UsdToBrl,
	}
}
