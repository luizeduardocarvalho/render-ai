package api

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"render-ai/backend/internal/store"
)

// notificationWindow is how far back the notification list reaches: a job is
// listed while it was last updated within this long.
const notificationWindow = 24 * time.Hour

// Notification kinds: what a job makes.
const (
	notificationKindRender  = "render"
	notificationKindEdit    = "edit"
	notificationKindUpscale = "upscale"
)

// notificationVariations counts a job's variations by outcome. Total is all
// of them; Done and Failed are the ones that have finished either way.
type notificationVariations struct {
	Done   int `json:"done"`
	Failed int `json:"failed"`
	Total  int `json:"total"`
}

// notificationView is one entry of GET /api/me/render-jobs: a render job
// with the names needed to show and link to it - see API_CONTRACT.md.
type notificationView struct {
	ProjectID   string                 `json:"projectId"`
	ProjectName string                 `json:"projectName"`
	ViewID      string                 `json:"viewId"`
	ViewName    string                 `json:"viewName"`
	JobID       string                 `json:"jobId"`
	Kind        string                 `json:"kind"`
	Status      store.RenderJobStatus  `json:"status"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
	Variations  notificationVariations `json:"variations"`
	// RenderID is the first finished variation's Render, set once there is one.
	RenderID *string `json:"renderId,omitempty"`
	// Error is the job's error, set iff Status is "failed".
	Error *string `json:"error,omitempty"`
	Seen  bool    `json:"seen"`
}

func notificationKind(job *store.RenderJob) string {
	switch {
	case job.Request.Edit != nil:
		return notificationKindEdit
	case job.Request.Upscale != nil:
		return notificationKindUpscale
	default:
		return notificationKindRender
	}
}

func isTerminal(status store.RenderJobStatus) bool {
	return status == store.RenderJobDone || status == store.RenderJobFailed
}

// listMyRenderJobs returns the caller's render jobs from the last
// notificationWindow, across all their projects, newest first. A job whose
// view has since been deleted is left out: there is nothing to open.
func (s *Server) listMyRenderJobs(w http.ResponseWriter, r *http.Request) error {
	ownerID, _ := userIDFromContext(r.Context())
	projects, err := s.repo.ListProjects(ownerID)
	if err != nil {
		return internalErr("listing projects: %v", err)
	}

	since := time.Now().Add(-notificationWindow)
	out := []notificationView{}
	for _, summary := range projects {
		jobList, err := s.repo.ListRenderJobs(summary.ID, since)
		if errors.Is(err, store.ErrNotFound) {
			continue // deleted between the two calls
		}
		if err != nil {
			return internalErr("listing render jobs of project %s: %v", summary.ID, err)
		}
		if len(jobList) == 0 {
			continue
		}
		project, err := s.repo.GetProject(summary.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return internalErr("loading project %s: %v", summary.ID, err)
		}
		viewNames := make(map[string]string, len(project.Views))
		for _, v := range project.Views {
			viewNames[v.ID] = v.Name
		}
		for _, job := range jobList {
			viewName, ok := viewNames[job.ViewID]
			if !ok {
				continue
			}
			out = append(out, s.notificationFor(project, viewName, job))
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].JobID < out[j].JobID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) notificationFor(project *store.Project, viewName string, job *store.RenderJob) notificationView {
	state := s.deriveJobState(job)
	n := notificationView{
		ProjectID:   project.ID,
		ProjectName: project.Name,
		ViewID:      job.ViewID,
		ViewName:    viewName,
		JobID:       job.ID,
		Kind:        notificationKind(job),
		Status:      state.status,
		CreatedAt:   job.CreatedAt,
		UpdatedAt:   job.UpdatedAt,
		Error:       state.firstErr,
		Seen:        job.SeenAt != nil,
	}
	n.Variations.Total = len(state.variations)
	for _, v := range state.variations {
		switch v.Status {
		case store.RenderJobDone:
			n.Variations.Done++
			if n.RenderID == nil {
				n.RenderID = v.RenderID
			}
		case store.RenderJobFailed:
			n.Variations.Failed++
		}
	}
	return n
}

// errAlreadySeen aborts markRenderJobSeen's update when there is nothing to
// write, so the job is left untouched (UpdateRenderJob bumps UpdatedAt on
// every successful write).
var errAlreadySeen = errors.New("render job already seen")

// markRenderJobSeen records that the user has seen the job's outcome. It is
// idempotent, and does nothing for a job that has not finished yet: its
// notification only becomes unread once there is an outcome to read.
func (s *Server) markRenderJobSeen(w http.ResponseWriter, r *http.Request) error {
	pid, jid := r.PathValue("pid"), r.PathValue("jid")
	_, err := s.repo.UpdateRenderJob(pid, jid, func(j *store.RenderJob) error {
		if j.SeenAt != nil || !isTerminal(s.deriveJobState(j).status) {
			return errAlreadySeen
		}
		now := time.Now().UTC()
		j.SeenAt = &now
		return nil
	})
	if err != nil && !errors.Is(err, errAlreadySeen) {
		return mapStoreErr(err, "render job %s not found", jid)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
