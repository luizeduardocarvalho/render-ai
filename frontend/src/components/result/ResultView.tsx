import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError, fetchSignedImageUrl } from "../../api";
import { useSignedImageUrl } from "../../hooks/useSignedImageUrl";
import { useProject } from "../../state/ProjectContext";
import type { Render, View } from "../../types";
import { BeforeAfterSlider } from "./BeforeAfterSlider";
import { MetricsPanel } from "./MetricsPanel";
import "./ResultView.css";

interface ResultViewProps {
  view: View;
  render: Render;
}

export function ResultView({ view, render }: ResultViewProps) {
  const { t, i18n } = useTranslation();
  const { project, setAnchor } = useProject();
  const [settingAnchor, setSettingAnchor] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const beforeUrl = useSignedImageUrl(view.screenshotImageId);
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

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">{t("resultView.title")}</div>
          <div className="panel-subtitle">{new Date(render.createdAt).toLocaleString(i18n.language)}</div>
        </div>
        <div className="result-header-actions">
          {isAnchor && <span className="badge badge-accent">{t("resultView.anchorBadge")}</span>}
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
