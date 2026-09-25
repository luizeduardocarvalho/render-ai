package api

import (
	"errors"
	"net/http"
	"sort"
	"sync"
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

// listConcurrency bounds how many projects are read at once by
// listMyRenderJobs: enough that a user with many projects is not waiting on
// them one after another, few enough not to flood the store.
const listConcurrency = 8

// listMyRenderJobs returns the caller's render jobs from the last
// notificationWindow, across all their projects, newest first. A job whose
// view has since been deleted is left out: there is nothing to open.
//
// It is polled every few seconds while a job runs, so it reads as little as it
// can: per project the recent jobs and, only for a project that has some, its
// view names - never the views' masks or renders.
func (s *Server) listMyRenderJobs(w http.ResponseWriter, r *http.Request) error {
	ownerID, _ := userIDFromContext(r.Context())
	projects, err := s.repo.ListProjects(ownerID)
	if err != nil {
		return internalErr("listing projects: %v", err)
	}

	since := time.Now().Add(-notificationWindow)
	perProject := make([][]notificationView, len(projects))
	errs := make([]error, len(projects))
	sem := make(chan struct{}, listConcurrency)
	var wg sync.WaitGroup
	for i, summary := range projects {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			perProject[i], errs[i] = s.projectNotifications(summary, since)
		}()
	}
	wg.Wait()

	out := []notificationView{}
	for i := range projects {
		if errs[i] != nil {
			return errs[i]
		}
		out = append(out, perProject[i]...)
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

// projectNotifications is one project's share of listMyRenderJobs.
func (s *Server) projectNotifications(summary store.ProjectSummary, since time.Time) ([]notificationView, error) {
	jobList, err := s.repo.ListRenderJobs(summary.ID, since)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil // deleted since the project list was read
	}
	if err != nil {
		return nil, internalErr("listing render jobs of project %s: %v", summary.ID, err)
	}
	if len(jobList) == 0 {
		return nil, nil
	}
	viewNames, err := s.repo.ViewNames(summary.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, internalErr("reading view names of project %s: %v", summary.ID, err)
	}
	var out []notificationView
	for _, job := range jobList {
		viewName, ok := viewNames[job.ViewID]
		if !ok {
			continue
		}
		out = append(out, s.notificationFor(summary, viewName, job))
	}
	return out, nil
}

func (s *Server) notificationFor(project store.ProjectSummary, viewName string, job *store.RenderJob) notificationView {
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

// markRenderJobSeen records that the user has seen the job's outcome. It is
// idempotent, and does nothing for a job that has not finished yet: its
// notification only becomes unread once there is an outcome to read.
func (s *Server) markRenderJobSeen(w http.ResponseWriter, r *http.Request) error {
	pid, jid := r.PathValue("pid"), r.PathValue("jid")
	job, err := s.repo.GetRenderJob(pid, jid)
	if err != nil {
		return mapStoreErr(err, "render job %s not found", jid)
	}
	if isTerminal(s.deriveJobState(job).status) {
		if err := s.repo.MarkRenderJobSeen(pid, jid, time.Now()); err != nil {
			return mapStoreErr(err, "render job %s not found", jid)
		}
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
