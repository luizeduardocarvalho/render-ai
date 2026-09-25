import { UserButton } from "@clerk/clerk-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { ApiError } from "../api";
import { useRelativeTime } from "../hooks/useRelativeTime";
import { useSignedImageUrl } from "../hooks/useSignedImageUrl";
import { useProject } from "../state/ProjectContext";
import type { ProjectSummary } from "../types";
import { ConfirmDialog } from "./ConfirmDialog";
import { CreditsChip } from "./CreditsChip";
import { LanguageSwitcher } from "./LanguageSwitcher";
import { NotificationsBell } from "./NotificationsBell";
import { LibraryPanel } from "./LibraryPanel";
import "./ProjectPicker.css";

type PickerTab = "projects" | "library";

function PickerThumb({ project }: { project: ProjectSummary }) {
  const { t } = useTranslation();
  const url = useSignedImageUrl(project.thumbnailImageId, { projectId: project.id });
  return (
    <span className="picker-card-thumb">
      {url ? <img src={url} alt="" /> : <span className="picker-card-thumb-empty">{t("picker.thumbEmpty")}</span>}
    </span>
  );
}

export function ProjectPicker() {
  const { t } = useTranslation();
  const relativeTime = useRelativeTime();
  const {
    me,
    openError,
    projects,
    projectsLoading,
    projectsError,
    refreshProjects,
    selectProject,
    createProject,
    deleteProject,
  } = useProject();

  const [tab, setTab] = useState<PickerTab>("projects");
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [opening, setOpening] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<ProjectSummary | null>(null);

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

  async function confirmDelete() {
    if (!pendingDelete) return;
    const id = pendingDelete.id;
    setDeleting(id);
    setError(null);
    try {
      await deleteProject(id);
      setPendingDelete(null);
    } catch (err) {
      // Shown inside the still-open ConfirmDialog, not the page banner.
      throw new Error(err instanceof ApiError ? err.message : t("picker.errors.deleteFailed"));
    } finally {
      setDeleting(null);
    }
  }

  return (
    <div className="picker-screen">
      <header className="picker-topbar">
        <div className="app-brand">{t("app.brand")}</div>
        <div className="app-topbar-right">
          <NotificationsBell />
          <CreditsChip />
          {me?.isAdmin && (
            <Link to="/admin" className="btn btn-ghost btn-sm">
              {t("admin.link")}
            </Link>
          )}
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
          {tab === "projects" && !showForm && (
            <button type="button" className="btn btn-primary" onClick={() => setCreating(true)}>
              {t("picker.newProject")}
            </button>
          )}
        </div>

        <div className="segmented picker-tabs">
          <button
            type="button"
            className={`segmented-btn ${tab === "projects" ? "segmented-btn-active" : ""}`}
            onClick={() => setTab("projects")}
          >
            {t("picker.tabs.projects")}
          </button>
          <button
            type="button"
            className={`segmented-btn ${tab === "library" ? "segmented-btn-active" : ""}`}
            onClick={() => setTab("library")}
          >
            {t("picker.tabs.library")}
          </button>
        </div>

        {tab === "library" && <LibraryPanel />}

        {tab === "projects" && (
          <>
        {(error ?? openError) && <div className="error-banner">{error ?? openError}</div>}

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
                    setPendingDelete(p);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.stopPropagation();
                      setPendingDelete(p);
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
          </>
        )}
      </main>

      {pendingDelete && (
        <ConfirmDialog
          title={t("picker.deleteTitle")}
          body={t("picker.delete.confirm", { name: pendingDelete.name })}
          confirmLabel={t("common.delete")}
          onConfirm={confirmDelete}
          onCancel={() => setPendingDelete(null)}
        />
      )}
    </div>
  );
}
