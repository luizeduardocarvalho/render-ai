import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "../api";
import { useSignedImageUrl } from "../hooks/useSignedImageUrl";
import { useProject } from "../state/ProjectContext";
import type { View } from "../types";

function ViewThumb({ view }: { view: View }) {
  const url = useSignedImageUrl(view.hasScreenshot ? view.screenshotImageId : null);
  return <span className="view-tab-thumb">{url && <img src={url} alt="" />}</span>;
}

export function ViewsBar() {
  const { t } = useTranslation();
  const { project, selectedViewId, setSelectedViewId, createView, deleteView } = useProject();
  const views = project?.views ?? [];

  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  function handleFilePick(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0];
    if (!f) return;
    setFile(f);
    if (!name) setName(f.name.replace(/\.[^.]+$/, ""));
    setAdding(true);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!file || !name.trim()) return;
    setSubmitting(true);
    setError(null);
    try {
      await createView(name.trim(), file);
      setAdding(false);
      setFile(null);
      setName("");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("viewsBar.error"));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete(vid: string, viewName: string) {
    if (!window.confirm(t("viewsBar.deleteConfirm", { name: viewName }))) return;
    try {
      await deleteView(vid);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("viewsBar.deleteError"));
    }
  }

  return (
    <div className="views-bar">
      <div className="views-bar-tabs">
        {views.map((v) => (
          <button
            key={v.id}
            type="button"
            className={`view-tab ${v.id === selectedViewId ? "view-tab-active" : ""}`}
            onClick={() => setSelectedViewId(v.id)}
          >
            <ViewThumb view={v} />
            <span className="view-tab-text">
              <span className="view-tab-name">{v.name}</span>
              <span className="view-tab-meta">
                {t("viewsBar.meta", { width: v.width, height: v.height, count: v.renders.length })}
              </span>
            </span>
            <span
              role="button"
              tabIndex={0}
              className="view-tab-delete"
              title={t("viewsBar.deleteTitle")}
              onClick={(e) => {
                e.stopPropagation();
                void handleDelete(v.id, v.name);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") void handleDelete(v.id, v.name);
              }}
            >
              &times;
            </span>
          </button>
        ))}

        {!adding && (
          <button type="button" className="view-tab view-tab-add" onClick={() => fileInputRef.current?.click()}>
            {t("viewsBar.addView")}
          </button>
        )}
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          className="visually-hidden"
          onChange={handleFilePick}
        />
      </div>

      {adding && (
        <form className="views-bar-form" onSubmit={handleSubmit}>
          <input
            className="input"
            placeholder={t("viewsBar.namePlaceholder")}
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
          <span className="field-hint">{file?.name}</span>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setAdding(false)}>
            {t("viewsBar.cancel")}
          </button>
          <button type="submit" className="btn btn-primary btn-sm" disabled={submitting || !name.trim()}>
            {submitting ? <span className="spinner" /> : null}
            {t("viewsBar.upload")}
          </button>
        </form>
      )}

      {error && <div className="error-banner views-bar-error">{error}</div>}
    </div>
  );
}
