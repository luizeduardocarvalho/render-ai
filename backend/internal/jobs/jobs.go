// Package jobs defines the render-job queue abstraction: one Task per
// render variation, dispatched either in-process (Inline, for the memory
// store / local dev / tests) or via Cloud Tasks (CloudTasks, for the
// deployed backend). See internal/api/render.go for how it's wired in.
package jobs

import "context"

// Task identifies one render-job variation to work on. Its JSON shape is
// the exact body POST /internal/render-tasks receives (see
// API_CONTRACT.md).
type Task struct {
	ProjectID string `json:"projectId"`
	JobID     string `json:"jobId"`
	Variation int    `json:"variation"`
}

// Queue enqueues one Task for later (or immediate, for Inline) processing.
// Enqueue itself only needs to durably schedule the work - the queue never
// reports back whether the task later succeeded or failed; that is recorded
// on the RenderJob itself by whatever runs the task (see
// internal/api.Server.RunRenderVariation).
type Queue interface {
	Enqueue(ctx context.Context, task Task) error
}

// Handler processes one Task to completion (recording its outcome on the
// job) rather than returning an error - see Queue's doc comment.
type Handler func(ctx context.Context, task Task)
