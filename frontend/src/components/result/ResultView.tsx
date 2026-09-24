import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError, fetchSignedImageUrl } from "../../api";
import { useSignedImageUrl } from "../../hooks/useSignedImageUrl";
import { useProject } from "../../state/ProjectContext";
import type { Render, View } from "../../types";
import { EditRegionEditor } from "../edit/EditRegionEditor";
import { BeforeAfterSlider } from "./BeforeAfterSlider";
import { MetricsPanel } from "./MetricsPanel";
import "./ResultView.css";

interface ResultViewProps {
  view: View;
  render: Render;
  /** Called with a finished edit so the parent can show it. */
  onEdited: (edit: Render) => void;
}

export function ResultView({ view, render, onEdited }: ResultViewProps) {
  const { t, i18n } = useTranslation();
  const { project, setAnchor } = useProject();
  const [settingAnchor, setSettingAnchor] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);

  // Leave edit mode when another render gets selected (adjust state during
  // render rather than in an effect, so the old editor never flashes).
  const [lastRenderId, setLastRenderId] = useState(render.id);
  if (lastRenderId !== render.id) {
    setLastRenderId(render.id);
    setEditing(false);
  }

  // An edit is compared with the render it was made from, which is what
  // changed; any other render with the screenshot it was made from.
  const sourceRender = render.sourceRenderId
    ? view.renders.find((r) => r.id === render.sourceRenderId)
    : undefined;
  const beforeUrl = useSignedImageUrl(sourceRender?.resultImageId ?? view.screenshotImageId);
  const afterUrl = useSignedImageUrl(render.resultImageId);

  const isAnchor = project?.styleAnchorRenderId === render.id;

  async function handleSetAnchor() {
    setSettingAnchor(true);
    setError(null);
    try {
      await setAnchor(isAnchor ? null : render.id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("resultView.errors.anchor"));
    } finally {
      setSettingAnchor(false);
    }
  }

  async function handleDownload() {
    if (!project) return;
    setDownloading(true);
    setError(null);
    try {
      const signedUrl = await fetchSignedImageUrl(project.id, render.resultImageId);
      const res = await fetch(signedUrl);
      if (!res.ok) throw new Error(t("resultView.errors.download"));
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${view.name.replace(/\s+/g, "-").toLowerCase()}-${render.id.slice(0, 8)}.png`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("resultView.errors.download"));
    } finally {
      setDownloading(false);
    }
  }

  if (editing) {
    return (
      <EditRegionEditor
        view={view}
        render={render}
        onCancel={() => setEditing(false)}
        onDone={(edit) => {
          setEditing(false);
          onEdited(edit);
        }}
      />
    );
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">{t("resultView.title")}</div>
          <div className="panel-subtitle">{new Date(render.createdAt).toLocaleString(i18n.language)}</div>
        </div>
        <div className="result-header-actions">
          {isAnchor && <span className="badge badge-accent">{t("resultView.anchorBadge")}</span>}
          <button type="button" className="btn btn-sm" onClick={() => setEditing(true)}>
            {t("resultView.edit")}
          </button>
          <button type="button" className="btn btn-sm" onClick={handleSetAnchor} disabled={settingAnchor}>
            {settingAnchor ? <span className="spinner" /> : null}
            {isAnchor ? t("resultView.clearAnchor") : t("resultView.setAsAnchor")}
          </button>
          <button type="button" className="btn btn-primary btn-sm" onClick={handleDownload} disabled={downloading}>
            {downloading ? <span className="spinner" /> : null}
            {t("resultView.downloadPng")}
          </button>
        </div>
      </div>

      <div className="panel-body">
        {error && <div className="error-banner">{error}</div>}

        {beforeUrl && afterUrl ? (
          <BeforeAfterSlider
            beforeSrc={beforeUrl}
            afterSrc={afterUrl}
            width={view.width}
            height={view.height}
          />
        ) : (
          <div className="result-image-loading">
            <span className="spinner" /> {t("resultView.loadingImage")}
          </div>
        )}

        <MetricsPanel render={render} />
      </div>
    </section>
  );
}
