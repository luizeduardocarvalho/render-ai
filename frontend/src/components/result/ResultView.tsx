import { useState } from "react";
import { ApiError, imageUrl } from "../../api";
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
  const { project, setAnchor } = useProject();
  const [settingAnchor, setSettingAnchor] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isAnchor = project?.styleAnchorRenderId === render.id;

  async function handleSetAnchor() {
    setSettingAnchor(true);
    setError(null);
    try {
      await setAnchor(isAnchor ? null : render.id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to update style anchor");
    } finally {
      setSettingAnchor(false);
    }
  }

  async function handleDownload() {
    setDownloading(true);
    setError(null);
    try {
      const res = await fetch(imageUrl(render.resultImageId));
      if (!res.ok) throw new Error("Failed to download image");
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
      setError(err instanceof Error ? err.message : "Failed to download image");
    } finally {
      setDownloading(false);
    }
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">Result</div>
          <div className="panel-subtitle">{new Date(render.createdAt).toLocaleString()}</div>
        </div>
        <div className="result-header-actions">
          {isAnchor && <span className="badge badge-accent">Style anchor</span>}
          <button type="button" className="btn btn-sm" onClick={handleSetAnchor} disabled={settingAnchor}>
            {settingAnchor ? <span className="spinner" /> : null}
            {isAnchor ? "Clear anchor" : "Set as style anchor"}
          </button>
          <button type="button" className="btn btn-primary btn-sm" onClick={handleDownload} disabled={downloading}>
            {downloading ? <span className="spinner" /> : null}
            Download PNG
          </button>
        </div>
      </div>

      <div className="panel-body">
        {error && <div className="error-banner">{error}</div>}

        <BeforeAfterSlider
          beforeSrc={imageUrl(view.screenshotImageId)}
          afterSrc={imageUrl(render.resultImageId)}
          width={view.width}
          height={view.height}
        />

        <MetricsPanel render={render} />
      </div>
    </section>
  );
}
