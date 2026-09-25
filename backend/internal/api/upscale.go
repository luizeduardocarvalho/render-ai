package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"render-ai/backend/internal/geometry"
	"render-ai/backend/internal/imageutil"
	renderpkg "render-ai/backend/internal/render"
	"render-ai/backend/internal/store"
)

// An Upscale is the 4K version of a finished Render. Making the same render
// again at 4K would give a different picture (the image models take no seed),
// so the source is sent to the model as the thing to reproduce, and the answer
// is pinned back to the source's colour and lighting - the model adds the fine
// detail, the user keeps the render they approved. It runs as an ordinary
// render job (see render.go), so it is charged, refunded, queued and polled
// exactly like a render.

// startUpscale is POST /api/projects/{pid}/views/{vid}/renders/{rid}/upscale.
func (s *Server) startUpscale(w http.ResponseWriter, r *http.Request) error {
	pid, vid, rid := r.PathValue("pid"), r.PathValue("vid"), r.PathValue("rid")

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
	if source.Resolution == store.Resolution4K {
		return badRequest("render %s is already 4K", rid)
	}
	if _, ok := s.blobs.GetBlob(source.ResultImageID); !ok {
		return internalErr("image blob missing for render %s", rid)
	}

	// The upscale is always made by the pro model at 4K, whatever the source
	// was made with: the flash model stops at 1K.
	resp, _, err := s.launchRenderJob(r.Context(), project, pid, vid, store.RenderJobRequest{
		Model:      store.ModelPro,
		Resolution: store.Resolution4K,
		Variations: 1,
		Upscale:    &store.RenderJobUpscale{SourceRenderID: rid},
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, resp)
	return nil
}

// upscaleAssembly is what finishing an Upscale needs once the model has
// answered: the source, to check the answer against and pin its colours to.
type upscaleAssembly struct {
	sourceRenderID string
	// sourceData is the source's stored image. It is only decoded once the model
	// has answered, so a 4K answer and the source are never both in memory
	// while the call is in flight.
	sourceData []byte
}

func (s *Server) upscalePromptPath() string {
	return filepath.Join(filepath.Dir(s.promptPath), "upscale.tmpl")
}

// assembleUpscaleRequest is assembleRenderRequest for an Upscale job: it loads
// the source render and builds the request. All of it happens before the model
// call, so a missing source fails the job before anything is spent.
func (s *Server) assembleUpscaleRequest(pid string, job *store.RenderJob) (*renderAssembly, error) {
	start := time.Now()
	upscale := job.Request.Upscale

	project, err := s.repo.GetProject(pid)
	if err != nil {
		return nil, mapStoreErr(err, "project %s not found", pid)
	}
	view := findViewIn(project, job.ViewID)
	if view == nil {
		return nil, notFoundErr("view %s not found", job.ViewID)
	}
	source, err := s.repo.FindRender(pid, upscale.SourceRenderID)
	if err != nil {
		return nil, mapStoreErr(err, "render %s not found", upscale.SourceRenderID)
	}
	sourceBlob, ok := s.blobs.GetBlob(source.ResultImageID)
	if !ok {
		return nil, internalErr("image blob missing for render %s", source.ID)
	}
	cfg, _, err := decodeImageConfig(sourceBlob.Data)
	if err != nil {
		return nil, internalErr("decoding render %s: %v", source.ID, err)
	}
	prompt, err := renderpkg.UpscalePrompt(s.upscalePromptPath())
	if err != nil {
		return nil, internalErr("building prompt: %v", err)
	}

	modelID := s.cfg.Models.ProImage
	return &renderAssembly{
		view:    view,
		modelID: modelID,
		req: renderpkg.RenderRequest{
			ModelID:     modelID,
			AspectRatio: renderpkg.PickAspectRatio(cfg.Width, cfg.Height),
			ImageSize:   string(store.Resolution4K),
			Images:      [][]byte{sourceBlob.Data},
			Prompt:      prompt,
		},
		upscale:    &upscaleAssembly{sourceRenderID: source.ID, sourceData: sourceBlob.Data},
		assemblyMs: time.Since(start).Milliseconds(),
	}, nil
}

// finish checks the model's answer really is a larger version of the source and
// returns it as PNG bytes with the source's colour and lighting.
//
// An answer that is not, is an error: it costs the user credits and would put
// a different picture, or no more pixels than they had, in their history under
// the name of an upscale. The job fails and refunds instead.
func (u *upscaleAssembly) finish(generated []byte) ([]byte, error) {
	src, _, err := imageutil.Decode(u.sourceData)
	if err != nil {
		return nil, fmt.Errorf("decoding the source render: %w", err)
	}
	gen, _, err := imageutil.Decode(generated)
	if err != nil {
		return nil, fmt.Errorf("decoding the model's image: %w", err)
	}
	sb, gb := src.Bounds(), gen.Bounds()
	if !sameAspect(gb.Dx(), gb.Dy(), sb.Dx(), sb.Dy()) {
		return nil, fmt.Errorf("the model returned a %dx%d image, a different shape from the %dx%d render", gb.Dx(), gb.Dy(), sb.Dx(), sb.Dy())
	}
	if gb.Dx() <= sb.Dx() {
		return nil, fmt.Errorf("the model returned a %dx%d image, not larger than the %dx%d render", gb.Dx(), gb.Dy(), sb.Dx(), sb.Dy())
	}
	pinned, err := geometry.PinLowFrequency(gen, src)
	if err != nil {
		return nil, err
	}
	return imageutil.EncodePNG(pinned)
}
