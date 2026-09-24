import { UserButton } from "@clerk/clerk-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "../api";
import { useSignedImageUrl } from "../hooks/useSignedImageUrl";
import { useProject } from "../state/ProjectContext";
import type { ProjectSummary } from "../types";
import { LanguageSwitcher } from "./LanguageSwitcher";
import "./ProjectPicker.css";

function PickerThumb({ project }: { project: ProjectSummary }) {
  const { t } = useTranslation();
  const url = useSignedImageUrl(project.thumbnailImageId, { projectId: project.id });
  return (
    <span className="picker-card-thumb">
      {url ? <img src={url} alt="" /> : <span className="picker-card-thumb-empty">{t("picker.thumbEmpty")}</span>}
    </span>
  );
}

function useRelativeTime() {
  const { t, i18n } = useTranslation();
  return function relativeTime(iso: string): string {
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return "";
    const diff = Date.now() - then;
    const mins = Math.round(diff / 60000);
    if (mins < 1) return t("picker.time.justNow");
    if (mins < 60) return t("picker.time.minutesAgo", { count: mins });
    const hrs = Math.round(mins / 60);
    if (hrs < 24) return t("picker.time.hoursAgo", { count: hrs });
    const days = Math.round(hrs / 24);
    if (days < 30) return t("picker.time.daysAgo", { count: days });
    return new Date(iso).toLocaleDateString(i18n.language);
  };
}

export function ProjectPicker() {
  const { t } = useTranslation();
  const relativeTime = useRelativeTime();
  const {
    projects,
    projectsLoading,
    projectsError,
    refreshProjects,
    selectProject,
    createProject,
    deleteProject,
  } = useProject();

  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [opening, setOpening] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);

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
      setError(err instanceof ApiError ? err.message : t("picker.errors.createFailed"));
      setSubmitting(false);
    }
  }

  async function handleOpen(id: string) {
    setOpening(id);
    setError(null);
    try {
      await selectProject(id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("picker.errors.openFailed"));
      setOpening(null);
    }
  }

  async function handleDelete(id: string, projectName: string) {
    if (!window.confirm(t("picker.delete.confirm", { name: projectName }))) {
      return;
    }
    setDeleting(id);
    setError(null);
    try {
      await deleteProject(id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("picker.errors.deleteFailed"));
    } finally {
      setDeleting(null);
    }
  }

  return (
    <div className="picker-screen">
      <header className="picker-topbar">
        <div className="app-brand">{t("app.brand")}</div>
        <div className="app-topbar-right">
          <LanguageSwitcher />
          <UserButton afterSignOutUrl="/sign-in" />
        </div>
      </header>

      <main className="picker-main">
        <div className="picker-head">
          <div>
            <h1 className="picker-heading">{t("picker.heading")}</h1>
            <p className="field-hint">{t("picker.subtitle")}</p>
          </div>
          {!showForm && (
            <button type="button" className="btn btn-primary" onClick={() => setCreating(true)}>
              {t("picker.newProject")}
            </button>
          )}
        </div>

        {error && <div className="error-banner">{error}</div>}

        {showForm && (
          <form className="panel picker-create-card" onSubmit={handleCreate}>
            <div className="field">
              <label className="field-label" htmlFor="new-project-name">
                {t("picker.nameLabel")}
              </label>
              <input
                id="new-project-name"
                className="input"
                placeholder={t("picker.namePlaceholder")}
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
                  {t("picker.cancel")}
                </button>
              )}
              <button type="submit" className="btn btn-primary" disabled={submitting || !name.trim()}>
                {submitting ? <span className="spinner" /> : null}
                {t("picker.createProject")}
              </button>
            </div>
          </form>
        )}

        {projectsLoading && (
          <div className="picker-status">
            <span className="spinner" /> {t("picker.loading")}
          </div>
        )}

        {projectsError && !projectsLoading && (
          <div className="picker-status picker-status-error">
            <span>{projectsError}</span>
            <button type="button" className="btn btn-sm" onClick={() => void refreshProjects()}>
              {t("picker.retry")}
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
                disabled={opening !== null || deleting !== null}
              >
                <PickerThumb project={p} />
                <span
                  role="button"
                  tabIndex={0}
                  className="picker-card-delete"
                  title={t("picker.deleteTitle")}
                  onClick={(e) => {
                    e.stopPropagation();
                    void handleDelete(p.id, p.name);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.stopPropagation();
                      void handleDelete(p.id, p.name);
                    }
                  }}
                >
                  {deleting === p.id ? <span className="spinner" /> : "×"}
                </span>
                <span className="picker-card-body">
                  <span className="picker-card-name">{p.name}</span>
                  <span className="picker-card-meta">
                    {t("picker.card.views", { count: p.viewCount })} - {t("picker.card.renders", { count: p.renderCount })}
                  </span>
                  <span className="picker-card-time">
                    {t("picker.card.updated", { time: relativeTime(p.updatedAt) })}
                  </span>
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
