package api

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"render-ai/backend/internal/geometry"
	"render-ai/backend/internal/imageutil"
	renderpkg "render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

type renderRequestBody struct {
	Model             string `json:"model"`
	Resolution        string `json:"resolution"`
	PreservationCheck bool   `json:"preservationCheck"`
	// Variations is how many independent render samples to generate from the
	// same request and keep, each as its own store.Render. The image models
	// expose no seed, so every call is already an independent sample - this
	// just repeats the (cheap) call N times instead of one. Omitted/0 means 1.
	Variations int `json:"variations"`
}

// minVariations and maxVariations bound the Variations field. 4 is a
// deliberately small ceiling - each variation is a full-price model call.
const (
	minVariations = 1
	maxVariations = 4
)

// qualifyingMask is a non-hidden, painted mask bound to a library asset -
// these are exactly the masks that drive the region map and the set of
// asset reference photos sent to the model.
type qualifyingMask struct {
	mask  *store.Mask
	asset *store.Asset
}

func (s *Server) renderView(w http.ResponseWriter, r *http.Request) error {
	pid, vid := r.PathValue("pid"), r.PathValue("vid")

	var req renderRequestBody
	if err := readJSON(r, &req); err != nil {
		return err
	}

	model := store.ModelChoice(req.Model)
	if model != store.ModelPro && model != store.ModelFlash {
		return badRequest(`model must be "pro" or "flash"`)
	}
	resolution := store.Resolution(req.Resolution)
	switch resolution {
	case store.Resolution1K, store.Resolution2K, store.Resolution4K:
	default:
		return badRequest(`resolution must be "1K", "2K", or "4K"`)
	}
	if model == store.ModelFlash && resolution != store.Resolution1K {
		return badRequest("flash model only supports 1K resolution")
	}
	variations := req.Variations
	if variations == 0 {
		variations = 1
	}
	if variations < minVariations || variations > maxVariations {
		return badRequest("variations must be between %d and %d", minVariations, maxVariations)
	}
	if s.renderer == nil {
		return internalErr("renderer is not configured (missing Vertex AI credentials)")
	}

	totalStart := time.Now()

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

	screenshotBlob, ok := s.blobs.GetBlob(view.ScreenshotImageID)
	if !ok {
		return internalErr("screenshot blob missing for view %s", vid)
	}
	screenshotImg, _, err := imageutil.Decode(screenshotBlob.Data)
	if err != nil {
		return internalErr("decoding stored screenshot: %v", err)
	}

	qualifying := qualifyingMasks(project, view)
	images := [][]byte{screenshotBlob.Data}
	var maskBitmaps []image.Image

	hasRegionMap := len(qualifying) > 0
	if hasRegionMap {
		regionMapPNG, bitmaps, err := s.buildRegionMap(screenshotImg, qualifying)
		if err != nil {
			return err
		}
		images = append(images, regionMapPNG)
		maskBitmaps = bitmaps
	}

	screenshotEdges := geometry.EdgeMap(screenshotImg)
	edgeMapPNG, err := imageutil.EncodePNG(screenshotEdges)
	if err != nil {
		return internalErr("encoding edge map: %v", err)
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

	images, assetRefs := s.appendAssetRefs(images, qualifying, vid)

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
		return internalErr("building prompt: %v", err)
	}

	modelID := s.cfg.Models.ProImage
	if model == store.ModelFlash {
		modelID = s.cfg.Models.FlashImage
	}
	aspectRatio := renderpkg.PickAspectRatio(view.Width, view.Height)

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.cfg.Server.RenderTimeoutSec)*time.Second)
	defer cancel()

	renderReq := renderpkg.RenderRequest{
		ModelID:     modelID,
		AspectRatio: aspectRatio,
		ImageSize:   string(resolution),
		Images:      images,
		Prompt:      prompt,
	}
	// assemblyMs is the shared setup cost (project/view lookup, screenshot
	// decode, region/edge maps, prompt build, etc.) above - it is attributed
	// to every variation's TotalMs, but never accumulated across variations,
	// so a later variation's total isn't inflated by an earlier variation's
	// image-call latency.
	assemblyMs := time.Since(totalStart).Milliseconds()

	// Each loop iteration below is one independent sample from the model
	// (no seed is exposed, so each call already differs) and becomes its
	// own first-class store.Render, appended to the view's history as soon
	// as it completes.
	created := make([]*store.Render, 0, variations)
	for i := 0; i < variations; i++ {
		variationStart := time.Now()

		result, err := s.renderer.Render(ctx, renderReq)
		imageCallMs := time.Since(variationStart).Milliseconds()
		if err != nil {
			if len(created) == 0 {
				if ctx.Err() == context.DeadlineExceeded {
					return timeoutErr("render timed out after %ds", s.cfg.Server.RenderTimeoutSec)
				}
				return badGateway("render failed: %v", err)
			}
			// At least one variation already succeeded - report what we
			// have instead of discarding it over a later failure.
			log.Printf("render: view=%s variation=%d/%d failed after %d succeeded, stopping early: %v",
				vid, i+1, variations, len(created), err)
			break
		}

		mimeType := result.MIMEType
		if mimeType == "" {
			mimeType = "image/png"
		}
		resultImageID, err := s.blobs.PutBlob(result.ImageData, mimeType)
		if err != nil {
			// A model call was already paid for and returned a usable image -
			// losing it here (unstored) is exactly the failure worth treating
			// like a failed variation, not silently dropping it: if nothing has
			// been saved yet this whole request fails, otherwise we stop early
			// and hand back the variations that did make it to storage.
			if len(created) == 0 {
				return badGateway("storing render result: %v", err)
			}
			log.Printf("render: view=%s variation=%d/%d failed to store result after %d succeeded, stopping early: %v",
				vid, i+1, variations, len(created), err)
			break
		}

		promptTokens, outputTokens := result.PromptTokens, result.OutputTokens

		rec := &store.Render{
			ID:            uuid.NewString(),
			CreatedAt:     time.Now().UTC(),
			Model:         model,
			Resolution:    resolution,
			AnchorUsed:    hasAnchor,
			RegionCount:   len(qualifying),
			ResultImageID: resultImageID,
		}

		if req.PreservationCheck {
			preservation, extraPromptTokens, extraOutputTokens := s.runPreservationCheck(
				ctx, screenshotImg, screenshotEdges, screenshotBlob.Data, result, view, maskBitmaps)
			rec.Preservation = preservation
			promptTokens += extraPromptTokens
			outputTokens += extraOutputTokens
		}

		totalMs := assemblyMs + time.Since(variationStart).Milliseconds()

		metrics := store.RenderMetrics{
			Model:       modelID,
			Resolution:  resolution,
			RegionCount: len(qualifying),
			AnchorUsed:  hasAnchor,
			ImageCallMs: imageCallMs,
			TotalMs:     totalMs,
		}
		if promptTokens > 0 || outputTokens > 0 {
			pt, ot := promptTokens, outputTokens
			metrics.PromptTokens = &pt
			metrics.OutputTokens = &ot
		}
		if cost, ok := renderpkg.EstimateCost(s.pricingTable(), string(model), string(resolution), promptTokens, outputTokens); ok {
			metrics.EstimatedCostUsd = &cost
		}
		rec.Metrics = metrics

		costDisplay := "n/a"
		if metrics.EstimatedCostUsd != nil {
			costDisplay = fmt.Sprintf("%.4f", *metrics.EstimatedCostUsd)
		}
		log.Printf("render: view=%s variation=%d/%d model=%s resolution=%s regions=%d anchor=%v imageCallMs=%d totalMs=%d promptTokens=%d outputTokens=%d costUsd=%s",
			vid, i+1, variations, modelID, resolution, len(qualifying), hasAnchor, imageCallMs, totalMs, promptTokens, outputTokens, costDisplay)

		addedRec, err := s.repo.AddRender(pid, vid, rec)
		if err != nil {
			return mapStoreErr(err, "view %s not found", vid)
		}
		created = append(created, addedRec)
	}

	writeJSON(w, http.StatusOK, created)
	return nil
}

func qualifyingMasks(project *store.Project, view *store.View) []qualifyingMask {
	var out []qualifyingMask
	for _, m := range view.Masks {
		if m.Hidden || !m.HasBitmap || m.AssetID == nil {
			continue
		}
		asset := findAssetIn(project, *m.AssetID)
		if asset == nil {
			continue
		}
		out = append(out, qualifyingMask{mask: m, asset: asset})
	}
	return out
}

// buildRegionMap composites the region map PNG and returns the decoded mask
// bitmaps too (needed later to build the preservation-check exclusion mask).
func (s *Server) buildRegionMap(screenshotImg image.Image, qualifying []qualifyingMask) ([]byte, []image.Image, error) {
	regions := make([]geometry.Region, 0, len(qualifying))
	bitmaps := make([]image.Image, 0, len(qualifying))
	for _, q := range qualifying {
		bitmapBlob, ok := s.blobs.GetBlob(q.mask.ID)
		if !ok {
			return nil, nil, internalErr("mask bitmap blob missing for mask %s", q.mask.ID)
		}
		bitmapImg, _, err := imageutil.Decode(bitmapBlob.Data)
		if err != nil {
			return nil, nil, internalErr("decoding mask bitmap for mask %s: %v", q.mask.ID, err)
		}
		col, err := parseHexColor(q.asset.Color)
		if err != nil {
			return nil, nil, badRequest("asset %s has an invalid color %q: %v", q.asset.ID, q.asset.Color, err)
		}
		regions = append(regions, geometry.Region{Bitmap: bitmapImg, Color: col})
		bitmaps = append(bitmaps, bitmapImg)
	}
	regionMapImg, err := geometry.CompositeRegionMap(screenshotImg, regions)
	if err != nil {
		return nil, nil, internalErr("compositing region map: %v", err)
	}
	regionMapPNG, err := imageutil.EncodePNG(regionMapImg)
	if err != nil {
		return nil, nil, internalErr("encoding region map: %v", err)
	}
	return regionMapPNG, bitmaps, nil
}

// appendAssetRefs adds one reference photo per distinct asset used by a
// qualifying mask (skipping assets with no reference photo), enforcing the
// MaxInputImages budget by dropping the lowest-priority (last-encountered)
// refs and logging a warning.
func (s *Server) appendAssetRefs(images [][]byte, qualifying []qualifyingMask, vid string) ([][]byte, []renderpkg.AssetRef) {
	var assetRefs []renderpkg.AssetRef
	seen := map[string]bool{}
	dropped := 0
	for _, q := range qualifying {
		if seen[q.asset.ID] || !q.asset.HasReferenceImage {
			continue
		}
		if len(images)+1 > renderpkg.MaxInputImages {
			dropped++
			continue
		}
		refBlob, ok := s.blobs.GetBlob(q.asset.ReferenceImageID)
		if !ok {
			continue
		}
		seen[q.asset.ID] = true
		images = append(images, refBlob.Data)
		assetRefs = append(assetRefs, renderpkg.AssetRef{
			Index:       len(images),
			Name:        q.asset.Name,
			Description: q.asset.Description,
			ColorName:   q.asset.Color,
		})
	}
	if dropped > 0 {
		log.Printf("render: dropped %d asset reference photo(s) to stay within the %d-image budget (view %s)",
			dropped, renderpkg.MaxInputImages, vid)
	}
	return images, assetRefs
}

// runPreservationCheck computes the edge-IoU score and asks the text model
// for a removed/added/moved diff. It never fails the overall render - any
// error here is logged and reflected in the report instead.
func (s *Server) runPreservationCheck(
	ctx context.Context,
	screenshotImg image.Image,
	screenshotEdges *image.Gray,
	screenshotPNG []byte,
	result renderpkg.RenderResult,
	view *store.View,
	maskBitmaps []image.Image,
) (*store.PreservationReport, int32, int32) {
	report := &store.PreservationReport{}

	resultImg, _, err := imageutil.Decode(result.ImageData)
	if err != nil {
		log.Printf("preservation check: decoding render result: %v", err)
		report.Inventory.Raw = fmt.Sprintf("preservation check failed: could not decode render result: %v", err)
		return report, 0, 0
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
	} else {
		report.EdgeScore = score
		report.EdgeFlag = score < s.cfg.Preservation.EdgeScoreFlagThreshold
	}

	if s.textModel == nil {
		report.Inventory.Raw = "preservation inventory check skipped: text model is not configured"
		return report, 0, 0
	}

	resultPNG, err := imageutil.EncodePNG(resized)
	if err != nil {
		log.Printf("preservation check: encoding resized result: %v", err)
		return report, 0, 0
	}

	diff, promptTokens, outputTokens, err := s.textModel.CheckPreservation(ctx, screenshotPNG, resultPNG, view.Inventory)
	if err != nil {
		log.Printf("preservation check: text model call: %v", err)
	}
	report.Inventory = store.PreservationInventory{
		Removed: nonNilStrings(diff.Removed),
		Added:   nonNilStrings(diff.Added),
		Moved:   nonNilStrings(diff.Moved),
		Raw:     diff.Raw,
	}
	return report, promptTokens, outputTokens
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

func parseHexColor(s string) (color.RGBA, error) {
	h := strings.TrimPrefix(s, "#")
	if len(h) != 6 {
		return color.RGBA{}, fmt.Errorf("expected 6 hex digits, got %q", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil
}

// nonNilStrings returns an empty (non-nil) slice for a nil input so the JSON
// response carries [] instead of null, matching the contract's array shape.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func inventoryOrPlaceholder(inv string) string {
	if strings.TrimSpace(inv) == "" {
		return "(no object inventory recorded for this view)"
	}
	return inv
}

func (s *Server) pricingTable() renderpkg.PricingTable {
	return renderpkg.PricingTable{
		ImagePro:      s.cfg.Pricing.Image.Pro,
		ImageFlash:    s.cfg.Pricing.Image.Flash,
		InputPerMTok:  s.cfg.Pricing.Text.InputPerMTok,
		OutputPerMTok: s.cfg.Pricing.Text.OutputPerMTok,
	}
}
