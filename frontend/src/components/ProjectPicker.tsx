import { UserButton } from "@clerk/clerk-react";
import { useState } from "react";
import { ApiError } from "../api";
import { useSignedImageUrl } from "../hooks/useSignedImageUrl";
import { useProject } from "../state/ProjectContext";
import type { ProjectSummary } from "../types";
import "./ProjectPicker.css";

function PickerThumb({ project }: { project: ProjectSummary }) {
  const url = useSignedImageUrl(project.thumbnailImageId, { projectId: project.id });
  return (
    <span className="picker-card-thumb">
      {url ? <img src={url} alt="" /> : <span className="picker-card-thumb-empty">No views yet</span>}
    </span>
  );
}

function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const diff = Date.now() - then;
  const mins = Math.round(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.round(hrs / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(iso).toLocaleDateString();
}

export function ProjectPicker() {
  const { projects, projectsLoading, projectsError, refreshProjects, selectProject, createProject } =
    useProject();

  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [opening, setOpening] = useState<string | null>(null);

  const isEmpty = !projectsLoading && !projectsError && (projects?.length ?? 0) === 0;
  const showForm = creating || isEmpty;

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    setSubmitting(true);
    setError(null);
    try {
      await createProject(name.trim());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create project");
      setSubmitting(false);
    }
  }

  async function handleOpen(id: string) {
    setOpening(id);
    setError(null);
    try {
      await selectProject(id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to open project");
      setOpening(null);
    }
  }

  return (
    <div className="picker-screen">
      <header className="picker-topbar">
        <div className="app-brand">render-ai</div>
        <UserButton afterSignOutUrl="/sign-in" />
      </header>

      <main className="picker-main">
        <div className="picker-head">
          <div>
            <h1 className="picker-heading">Your projects</h1>
            <p className="field-hint">Pick up where you left off, or start something new.</p>
          </div>
          {!showForm && (
            <button type="button" className="btn btn-primary" onClick={() => setCreating(true)}>
              + New project
            </button>
          )}
        </div>

        {error && <div className="error-banner">{error}</div>}

        {showForm && (
          <form className="panel picker-create-card" onSubmit={handleCreate}>
            <div className="field">
              <label className="field-label" htmlFor="new-project-name">
                Project name
              </label>
              <input
                id="new-project-name"
                className="input"
                placeholder="e.g. Lakeside renovation"
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoFocus
              />
            </div>
            <div className="picker-create-actions">
              {!isEmpty && (
                <button
                  type="button"
                  className="btn btn-ghost"
                  onClick={() => {
                    setCreating(false);
                    setName("");
                    setError(null);
                  }}
                >
                  Cancel
                </button>
              )}
              <button type="submit" className="btn btn-primary" disabled={submitting || !name.trim()}>
                {submitting ? <span className="spinner" /> : null}
                Create project
              </button>
            </div>
          </form>
        )}

        {projectsLoading && (
          <div className="picker-status">
            <span className="spinner" /> Loading projects...
          </div>
        )}

        {projectsError && !projectsLoading && (
          <div className="picker-status picker-status-error">
            <span>{projectsError}</span>
            <button type="button" className="btn btn-sm" onClick={() => void refreshProjects()}>
              Retry
            </button>
          </div>
        )}

        {!showForm && projects && projects.length > 0 && (
          <div className="picker-grid">
            {projects.map((p) => (
              <button
                key={p.id}
                type="button"
                className="picker-card"
                onClick={() => void handleOpen(p.id)}
                disabled={opening !== null}
              >
                <PickerThumb project={p} />
                <span className="picker-card-body">
                  <span className="picker-card-name">{p.name}</span>
                  <span className="picker-card-meta">
                    {p.viewCount} view{p.viewCount === 1 ? "" : "s"} - {p.renderCount} render
                    {p.renderCount === 1 ? "" : "s"}
                  </span>
                  <span className="picker-card-time">Updated {relativeTime(p.updatedAt)}</span>
                </span>
                {opening === p.id && <span className="spinner picker-card-spinner" />}
              </button>
            ))}
          </div>
        )}
      </main>
    </div>
  );
}
