package api

import (
	"encoding/base64"
	"fmt"
	"image"
	"log"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"render-ai/backend/internal/geometry"
	"render-ai/backend/internal/imageutil"
	renderpkg "render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

// An Edit changes marked areas of an existing Render: the caller paints one
// or more regions on the render, each with a text instruction, and the result
// is a new Render whose pixels outside the regions are the source's own. It
// runs as an ordinary render job (see render.go) so it is charged, refunded,
// queued and polled exactly like a render.

const (
	// maxEditRegions bounds the regions of one edit. An edit is one model
	// call however many regions it has, so this only bounds the prompt.
	maxEditRegions = 8
	// maxEditInstructionRunes bounds one region's instruction text.
	maxEditInstructionRunes = 500
	// maxEditBodyBytes bounds the request body: up to maxEditRegions PNG
	// bitmaps, base64 encoded.
	maxEditBodyBytes = 32 << 20
	// maxEditAspectSkew is how far a region bitmap's aspect ratio may differ
	// from the source render's (as a fraction). The editor draws at a reduced
	// size, so the two only agree to within rounding to whole pixels.
	maxEditAspectSkew = 0.01
	// maxEditBitmapSide bounds a region bitmap's width and height. The editor
	// draws at most 1600 px on a side; the cap only stops a tiny PNG that
	// declares an enormous size from making the decoder allocate gigabytes.
	maxEditBitmapSide = 4096
)

type editRegionBody struct {
	Instruction string `json:"instruction"`
	// Bitmap is a base64 (standard alphabet) encoded white-on-black PNG of
	// the region, at any size with the source render's aspect ratio.
	Bitmap string `json:"bitmap"`
}

type editRequestBody struct {
	Regions []editRegionBody `json:"regions"`
}

// startEdit is POST /api/projects/{pid}/views/{vid}/renders/{rid}/edit.
func (s *Server) startEdit(w http.ResponseWriter, r *http.Request) error {
	pid, vid, rid := r.PathValue("pid"), r.PathValue("vid"), r.PathValue("rid")

	r.Body = http.MaxBytesReader(w, r.Body, maxEditBodyBytes)
	var body editRequestBody
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if len(body.Regions) < 1 || len(body.Regions) > maxEditRegions {
		return badRequest("an edit needs between 1 and %d regions", maxEditRegions)
	}
	instructions := make([]string, len(body.Regions))
	for i, region := range body.Regions {
		instruction := strings.TrimSpace(region.Instruction)
		if instruction == "" {
			return badRequest("region %d needs an instruction", i+1)
		}
		if utf8.RuneCountInString(instruction) > maxEditInstructionRunes {
			return badRequest("region %d: instruction is longer than %d characters", i+1, maxEditInstructionRunes)
		}
		instructions[i] = instruction
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
	var source *store.Render
	for _, candidate := range view.Renders {
		if candidate.ID == rid {
			source = candidate
			break
		}
	}
	if source == nil {
		return notFoundErr("render %s not found in view %s", rid, vid)
	}
	sourceBlob, ok := s.blobs.GetBlob(source.ResultImageID)
	if !ok {
		return internalErr("image blob missing for render %s", rid)
	}
	sourceCfg, _, err := decodeImageConfig(sourceBlob.Data)
	if err != nil {
		return internalErr("decoding render %s: %v", rid, err)
	}

	bitmapPNGs := make([][]byte, len(body.Regions))
	for i, region := range body.Regions {
		data, err := base64.StdEncoding.DecodeString(region.Bitmap)
		if err != nil {
			return badRequest("region %d: bitmap is not valid base64: %v", i+1, err)
		}
		// The header alone says how big it is; check that before decoding.
		cfg, format, err := decodeImageConfig(data)
		if err != nil || format != "png" {
			return badRequest("region %d: bitmap must be a PNG image", i+1)
		}
		if cfg.Width > maxEditBitmapSide || cfg.Height > maxEditBitmapSide {
			return badRequest("region %d: bitmap is larger than %d px on a side", i+1, maxEditBitmapSide)
		}
		if !sameAspect(cfg.Width, cfg.Height, sourceCfg.Width, sourceCfg.Height) {
			return badRequest("region %d: bitmap size %dx%d does not match the render's aspect ratio (%dx%d)",
				i+1, cfg.Width, cfg.Height, sourceCfg.Width, sourceCfg.Height)
		}
		img, _, err := imageutil.Decode(data)
		if err != nil {
			return badRequest("region %d: bitmap must be a PNG image", i+1)
		}
		if !hasPaintedPixel(img) {
			return badRequest("region %d: nothing is painted", i+1)
		}
		bitmapPNGs[i] = data
	}

	edit := &store.RenderJobEdit{SourceRenderID: rid}
	var blobIDs []string
	cleanup := func() {
		for _, id := range blobIDs {
			s.blobs.DeleteBlob(id)
		}
	}
	for i, data := range bitmapPNGs {
		id, err := s.blobs.PutBlob(data, "image/png")
		if err != nil {
			cleanup()
			return badGateway("storing region bitmap: %v", err)
		}
		blobIDs = append(blobIDs, id)
		edit.Regions = append(edit.Regions, store.EditRegion{Instruction: instructions[i], BitmapImageID: id})
	}

	// The edit renders with the source's own model and resolution, one
	// variation - see the design notes in API_CONTRACT.md.
	resp, queued, err := s.launchRenderJob(r.Context(), project, pid, vid, store.RenderJobRequest{
		Model:      source.Model,
		Resolution: source.Resolution,
		Variations: 1,
		Edit:       edit,
	})
	if err != nil {
		// Once the task is queued a worker owns the bitmaps and deletes them.
		if !queued {
			cleanup()
		}
		return err
	}
	writeJSON(w, http.StatusAccepted, resp)
	return nil
}

// sameAspect reports whether a w x h bitmap has the aspect ratio of a
// refW x refH image, to within maxEditAspectSkew.
func sameAspect(w, h, refW, refH int) bool {
	if w <= 0 || h <= 0 || refW <= 0 || refH <= 0 {
		return false
	}
	got, want := float64(w)/float64(h), float64(refW)/float64(refH)
	return math.Abs(got-want)/want <= maxEditAspectSkew
}

// hasPaintedPixel reports whether any pixel of a white-on-black mask bitmap
// is painted.
func hasPaintedPixel(img image.Image) bool {
	for _, v := range geometry.ToGray(img).Pix {
		if v >= 128 {
			return true
		}
	}
	return false
}

// editAssembly is what compositing an Edit needs once the model has answered.
type editAssembly struct {
	sourceRenderID string
	source         image.Image
	// alpha is the feathered union of the edit regions at the source's size.
	alpha *image.Gray
	// regions are the edit regions as the overlay painted them, for scrubbing
	// any trace of the overlay out of the model's answer.
	regions      []geometry.Region
	instructions []string
}

func (s *Server) editPromptPath() string {
	return filepath.Join(filepath.Dir(s.promptPath), "edit.tmpl")
}

// assembleEditRequest is assembleRenderRequest for an Edit job: it loads the
// source render and the region bitmaps, builds the colored overlay image and
// the feathered blend mask, and renders the edit prompt. All of it happens
// before the model call, so a broken bitmap fails the job before anything is
// spent.
func (s *Server) assembleEditRequest(pid string, job *store.RenderJob) (*renderAssembly, error) {
	start := time.Now()
	edit := job.Request.Edit

	project, err := s.repo.GetProject(pid)
	if err != nil {
		return nil, mapStoreErr(err, "project %s not found", pid)
	}
	view := findViewIn(project, job.ViewID)
	if view == nil {
		return nil, notFoundErr("view %s not found", job.ViewID)
	}
	sourceRender, err := s.repo.FindRender(pid, edit.SourceRenderID)
	if err != nil {
		return nil, mapStoreErr(err, "render %s not found", edit.SourceRenderID)
	}
	sourceBlob, ok := s.blobs.GetBlob(sourceRender.ResultImageID)
	if !ok {
		return nil, internalErr("image blob missing for render %s", sourceRender.ID)
	}
	sourceImg, _, err := imageutil.Decode(sourceBlob.Data)
	if err != nil {
		return nil, internalErr("decoding render %s: %v", sourceRender.ID, err)
	}
	w, h := sourceImg.Bounds().Dx(), sourceImg.Bounds().Dy()

	// Bring every region bitmap to the source's size once; the overlay and
	// the blend mask both use them.
	bitmaps := make([]image.Image, len(edit.Regions))
	overlayRegions := make([]geometry.Region, len(edit.Regions))
	instructions := make([]string, len(edit.Regions))
	promptRegions := make([]renderpkg.EditRegionPrompt, len(edit.Regions))
	for i, region := range edit.Regions {
		blob, ok := s.blobs.GetBlob(region.BitmapImageID)
		if !ok {
			return nil, internalErr("region %d bitmap is missing", i+1)
		}
		img, _, err := imageutil.Decode(blob.Data)
		if err != nil {
			return nil, internalErr("decoding region %d bitmap: %v", i+1, err)
		}
		gray := geometry.ToGray(img)
		if gray.Bounds().Dx() != w || gray.Bounds().Dy() != h {
			gray = geometry.ResizeGray(gray, w, h)
		}
		swatch := geometry.RegionPalette[i%len(geometry.RegionPalette)]
		bitmaps[i] = gray
		overlayRegions[i] = geometry.Region{Bitmap: gray, Color: swatch.Color}
		instructions[i] = region.Instruction
		promptRegions[i] = renderpkg.EditRegionPrompt{Number: i + 1, Color: swatch.Name, Instruction: region.Instruction}
	}

	overlay, err := geometry.CompositeRegionMap(sourceImg, overlayRegions)
	if err != nil {
		return nil, internalErr("compositing edit overlay: %v", err)
	}
	overlayPNG, err := imageutil.EncodePNG(overlay)
	if err != nil {
		return nil, internalErr("encoding edit overlay: %v", err)
	}
	alpha, err := geometry.EditAlpha(bitmaps, w, h, geometry.EditFeatherPx(min(w, h)))
	if err != nil {
		return nil, internalErr("building edit mask: %v", err)
	}

	// The view's original screenshot is the ground truth for what the room
	// holds, so the model can tell how a region ought to look. A view always
	// has one; if its blob is somehow gone the edit still runs without it.
	images := [][]byte{sourceBlob.Data, overlayPNG}
	screenshotBlob, hasScreenshot := s.blobs.GetBlob(view.ScreenshotImageID)
	if view.HasScreenshot && hasScreenshot {
		images = append(images, screenshotBlob.Data)
	} else {
		hasScreenshot = false
	}

	prompt, err := renderpkg.EditPrompt(s.editPromptPath(), renderpkg.EditTemplateData{
		HasScreenshot: hasScreenshot,
		Regions:       promptRegions,
	})
	if err != nil {
		return nil, internalErr("building prompt: %v", err)
	}

	modelID := s.cfg.Models.ProImage
	if job.Request.Model == store.ModelFlash {
		modelID = s.cfg.Models.FlashImage
	}
	return &renderAssembly{
		view:            view,
		qualifyingCount: len(edit.Regions),
		modelID:         modelID,
		req: renderpkg.RenderRequest{
			ModelID:     modelID,
			AspectRatio: renderpkg.PickAspectRatio(w, h),
			ImageSize:   string(job.Request.Resolution),
			Images:      images,
			Prompt:      prompt,
		},
		edit: &editAssembly{
			sourceRenderID: sourceRender.ID,
			source:         sourceImg,
			alpha:          alpha,
			regions:        overlayRegions,
			instructions:   instructions,
		},
		assemblyMs: time.Since(start).Milliseconds(),
	}, nil
}

// composite blends the model's answer over the source through the edit
// mask and returns the PNG bytes of the result, plus how much each region's
// look changed from the source. Everything outside the mask is the source's
// own pixels.
func (e *editAssembly) composite(generated []byte) ([]byte, []store.EditRegionDrift, error) {
	genImg, _, err := imageutil.Decode(generated)
	if err != nil {
		return nil, nil, fmt.Errorf("decoding the model's image: %w", err)
	}
	// The model answers at the nearest aspect ratio it supports, so an oddly
	// shaped source gets an answer that is stretched to fit. Worth seeing in
	// the logs if edits ever land off their region.
	sb, gb := e.source.Bounds(), genImg.Bounds()
	if !sameAspect(gb.Dx(), gb.Dy(), sb.Dx(), sb.Dy()) {
		log.Printf("edit: model answered %dx%d for a %dx%d source; scaling it to fit", gb.Dx(), gb.Dy(), sb.Dx(), sb.Dy())
	}
	// The model sometimes traces the borders of the colored areas it was shown;
	// take that out of its answer before it is blended in.
	answer := geometry.ToNRGBA(genImg)
	if gb := answer.Bounds(); gb.Dx() != sb.Dx() || gb.Dy() != sb.Dy() {
		answer = geometry.ResizeBilinear(genImg, sb.Dx(), sb.Dy())
	}
	if n := geometry.ScrubOverlayColors(answer, e.regions); n > 0 {
		log.Printf("edit: removed %d pixels of overlay color the model drew along the region borders", n)
	}
	blended, err := geometry.BlendEdit(e.source, answer, e.alpha)
	if err != nil {
		return nil, nil, err
	}
	png, err := imageutil.EncodePNG(blended)
	if err != nil {
		return nil, nil, err
	}
	return png, e.regionDrift(blended), nil
}

// regionDrift measures how much the look of each edit region changed between
// the source and the finished edit. A region with nothing to measure is left
// out.
func (e *editAssembly) regionDrift(edited image.Image) []store.EditRegionDrift {
	var out []store.EditRegionDrift
	for i, region := range e.regions {
		drift, share, ok := geometry.RegionDrift(e.source, edited, geometry.ToGray(region.Bitmap))
		if !ok {
			continue
		}
		out = append(out, store.EditRegionDrift{Number: i + 1, Instruction: e.instructions[i], Drift: drift, ChangedShare: share})
	}
	return out
}

// deleteEditBlobs drops the region bitmaps of a finished edit job.
func (s *Server) deleteEditBlobs(edit *store.RenderJobEdit) {
	for _, region := range edit.Regions {
		s.blobs.DeleteBlob(region.BitmapImageID)
	}
}
