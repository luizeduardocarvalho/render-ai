import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError, INSUFFICIENT_CREDITS_CODE, fetchSignedImageUrl, getPricing } from "../../api";
import { useSignedImageUrl } from "../../hooks/useSignedImageUrl";
import { formatCredits } from "../../lib/credits";
import { useProject } from "../../state/ProjectContext";
import type { PricingResponse, Render, RenderJob, View } from "../../types";
import { EditRegionEditor } from "../edit/EditRegionEditor";
import { BeforeAfterSlider } from "./BeforeAfterSlider";
import { MetricsPanel } from "./MetricsPanel";
import "./ResultView.css";

interface ResultViewProps {
  view: View;
  render: Render;
  /** Called with a finished edit so the parent can show it. */
  onEdited: (edit: Render) => void;
  /** Called with a finished 4K upscale so the parent can show it. */
  onUpscaled: (upscale: Render) => void;
}

export function ResultView({ view, render, onEdited, onUpscaled }: ResultViewProps) {
  const { t, i18n } = useTranslation();
  const { project, setAnchor, upscaleRender, me, refreshMe, adjustDisplayedCredits } = useProject();
  const [settingAnchor, setSettingAnchor] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  // The render being upscaled, if any. Only one upscale runs at a time, so
  // switching to another render cannot start a second (and second charge).
  const [upscalingId, setUpscalingId] = useState<string | null>(null);
  const [upscaleJob, setUpscaleJob] = useState<RenderJob | null>(null);
  const [pricing, setPricing] = useState<PricingResponse | null>(null);
  const shownRenderId = useRef(render.id);
  useEffect(() => {
    shownRenderId.current = render.id;
  }, [render.id]);

  useEffect(() => {
    let cancelled = false;
    getPricing()
      .then((p) => {
        if (!cancelled) setPricing(p);
      })
      .catch(() => {
        // The price on the button is a nice-to-have.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Leave edit mode when another render gets selected (adjust state during
  // render rather than in an effect, so the old editor never flashes).
  const [lastRenderId, setLastRenderId] = useState(render.id);
  if (lastRenderId !== render.id) {
    setLastRenderId(render.id);
    setEditing(false);
  }

  // An edit or an upscale is compared with the render it was made from, which
  // is what changed; any other render with the screenshot it was made from.
  const sourceRenderId = render.sourceRenderId ?? render.upscaledFromRenderId;
  const sourceRender = sourceRenderId ? view.renders.find((r) => r.id === sourceRenderId) : undefined;
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

  // An upscale is always made by the pro model at 4K, whatever the render was.
  const upscaleCredits = pricing?.estimates.find((e) => e.model === "pro" && e.resolution === "4K")?.credits;
  const canUpscale = render.resolution !== "4K";
  const upscaling = upscalingId !== null;
  const devAuthDisabled = me?.userId === "";
  const lowBalance =
    !devAuthDisabled && me !== null && upscaleCredits !== undefined && me.credits < upscaleCredits;

  async function handleUpscale() {
    if (upscaling) return;
    setUpscalingId(render.id);
    setUpscaleJob(null);
    setError(null);

    // Show the debit right away and give it back if the job fails, the way
    // RenderControls does; refreshMe() in finally settles on the real balance.
    const charge = devAuthDisabled ? 0 : (upscaleCredits ?? 0);
    if (charge > 0) adjustDisplayedCredits(-charge);
    let refunded = false;
    const handleProgress = (job: RenderJob) => {
      setUpscaleJob(job);
      if (charge > 0 && !refunded && job.variations.some((v) => v.status === "failed")) {
        refunded = true;
        adjustDisplayedCredits(charge);
      }
    };

    try {
      const [upscale] = await upscaleRender(view.id, render.id, { onProgress: handleProgress });
      // Only jump to the result if the user is still looking at what they
      // upscaled.
      if (upscale && shownRenderId.current === render.id) onUpscaled(upscale);
    } catch (err) {
      if (err instanceof ApiError && err.code === INSUFFICIENT_CREDITS_CODE) {
        setError(t("resultView.errors.upscaleInsufficientCredits"));
      } else {
        setError(err instanceof Error ? err.message : t("resultView.errors.upscale"));
      }
    } finally {
      setUpscalingId(null);
      setUpscaleJob(null);
      void refreshMe();
      if (refunded) window.setTimeout(() => void refreshMe(), 2000);
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
      <div className="panel-header result-header">
        <div>
          <div className="panel-title">{t("resultView.title")}</div>
          <div className="panel-subtitle">{new Date(render.createdAt).toLocaleString(i18n.language)}</div>
        </div>
        <div className="result-header-actions">
          {isAnchor && <span className="badge badge-accent">{t("resultView.anchorBadge")}</span>}
          <button type="button" className="btn btn-sm" onClick={() => setEditing(true)} disabled={upscaling}>
            {t("resultView.edit")}
          </button>
          {canUpscale && (
            <button
              type="button"
              className="btn btn-sm"
              onClick={handleUpscale}
              disabled={upscaling || lowBalance}
              title={t("resultView.upscaleHint")}
            >
              {upscalingId === render.id ? <span className="spinner" /> : null}
              {upscalingId === render.id
                ? upscaleJob?.status === "running"
                  ? t("resultView.upscaling")
                  : t("resultView.upscaleQueued")
                : upscaleCredits !== undefined
                  ? t("resultView.upscale", {
                      count: upscaleCredits,
                      credits: formatCredits(upscaleCredits, i18n.language),
                    })
                  : t("resultView.upscaleNoPrice")}
            </button>
          )}
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
        {upscalingId === render.id && <div className="field-hint">{t("resultView.upscaleWait")}</div>}
        {canUpscale && !upscaling && lowBalance && me && upscaleCredits !== undefined && (
          <div className="field-hint">
            {t("renderControls.insufficientCredits", {
              available: formatCredits(me.credits, i18n.language),
              required: formatCredits(upscaleCredits, i18n.language),
            })}
          </div>
        )}

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
