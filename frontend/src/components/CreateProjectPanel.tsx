import { useState } from "react";
import { ApiError } from "../api";
import { useProject } from "../state/ProjectContext";

export function CreateProjectPanel() {
  const { createProject } = useProject();
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    setSubmitting(true);
    setError(null);
    try {
      await createProject(name.trim());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create project");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="create-project-screen">
      <form className="panel create-project-card" onSubmit={handleSubmit}>
        <div className="create-project-brand">render-ai</div>
        <h1 className="create-project-heading">Start a new project</h1>
        <p className="field-hint">
          Give it a name - you can add style settings, assets and views once it's created.
        </p>
        <div className="field">
          <label className="field-label" htmlFor="project-name">
            Project name
          </label>
          <input
            id="project-name"
            className="input"
            placeholder="e.g. Lakeside renovation"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
        </div>
        {error && <div className="error-banner">{error}</div>}
        <button type="submit" className="btn btn-primary btn-block" disabled={submitting || !name.trim()}>
          {submitting ? <span className="spinner" /> : null}
          Create project
        </button>
      </form>
    </div>
  );
}
