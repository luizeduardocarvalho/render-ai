import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError, INSUFFICIENT_CREDITS_CODE, getPricing } from "../api";
import { useProject } from "../state/ProjectContext";
import type { ModelChoice, PricingResponse, Render, RenderJob, Resolution, View } from "../types";
import { formatCredits } from "../lib/credits";

interface RenderControlsProps {
  view: View;
  onRendered: (renders: Render[]) => void;
}

export function RenderControls({ view, onRendered }: RenderControlsProps) {
  const { t, i18n } = useTranslation();
  const { renderView, me, refreshMe, adjustDisplayedCredits } = useProject();
  const [model, setModel] = useState<ModelChoice>("pro");
  const [resolution, setResolution] = useState<Resolution>("2K");
  const [preservationCheck, setPreservationCheck] = useState(true);
  const [variations, setVariations] = useState<1 | 2>(1);
  const [rendering, setRendering] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pricing, setPricing] = useState<PricingResponse | null>(null);
  const [progressJob, setProgressJob] = useState<RenderJob | null>(null);
  // Aborts the in-flight render's polling loop if the component unmounts
  // (e.g. the user navigates away) while a job is still running.
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    return () => {
      abortRef.current?.abort();
    };
  }, []);

  const brlFormatter = useMemo(
    () => new Intl.NumberFormat(i18n.language, { style: "currency", currency: "BRL" }),
    [i18n.language],
  );

  useEffect(() => {
    let cancelled = false;
    getPricing()
      .then((p) => {
        if (!cancelled) setPricing(p);
      })
      .catch(() => {
        // Cost preview is a nice-to-have - if pricing can't be fetched, just
        // hide it rather than blocking or erroring the render controls.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const flashLocked = model === "flash";
  const estimate = pricing?.estimates.find((e) => e.model === model && e.resolution === resolution);
  const estimatedTotalBrl = estimate ? estimate.costBrl * variations : undefined;
  const estimatedTotalCredits = estimate ? estimate.credits * variations : undefined;

  // Client-side "not enough credits" is advisory only - the server's 402 is
  // the source of truth. Auth-disabled dev (userId === "") never charges, so
  // the check is skipped there too (see API_CONTRACT.md / api.ts renderView).
  const devAuthDisabled = me?.userId === "";
  const lowBalance =
    !devAuthDisabled &&
    me !== null &&
    estimatedTotalCredits !== undefined &&
    me.credits < estimatedTotalCredits;

  function handleModelChange(next: ModelChoice) {
    setModel(next);
    if (next === "flash") setResolution("1K");
  }

  async function handleRender() {
    setRendering(true);
    setError(null);
    setProgressJob(null);
    const controller = new AbortController();
    abortRef.current = controller;

    // Show the debit in the header right away, then give each failed
    // variation's share back as soon as polling reports it (the server
    // refunds failed variations). refreshMe() in `finally` reconciles with
    // the server's real balance either way - including a 402 or any other
    // error before the job started, which undoes this debit.
    const perVariation = devAuthDisabled ? 0 : (estimate?.credits ?? 0);
    if (perVariation > 0) adjustDisplayedCredits(-perVariation * variations);
    let refundedCount = 0;
    const handleProgress = (job: RenderJob) => {
      setProgressJob(job);
      const failed = job.variations.filter((v) => v.status === "failed").length;
      if (perVariation > 0 && failed > refundedCount) {
        adjustDisplayedCredits(perVariation * (failed - refundedCount));
        refundedCount = failed;
      }
    };

    try {
      const renders = await renderView(
        view.id,
        { model, resolution, preservationCheck, variations },
        { onProgress: handleProgress, signal: controller.signal },
      );
      onRendered(renders);
    } catch (err) {
      if (err instanceof ApiError && err.code === INSUFFICIENT_CREDITS_CODE) {
        setError(t("renderControls.insufficientCreditsError"));
      } else {
        setError(err instanceof ApiError ? err.message : t("renderControls.error"));
      }
    } finally {
      abortRef.current = null;
      setRendering(false);
      setProgressJob(null);
      // The debit (and any refund) already happened server-side by the time
      // this resolves either way - replace the optimistic balance with it.
      void refreshMe();
      // The worker records a variation as failed a moment before its refund
      // lands, so a refresh right at the end can still miss it: check once more.
      if (refundedCount > 0) window.setTimeout(() => void refreshMe(), 2000);
    }
  }

  const doneCount = progressJob?.variations.filter((v) => v.status === "done" || v.status === "failed").length ?? 0;
  const totalCount = progressJob?.variations.length ?? variations;
  const renderingLabel =
    progressJob && totalCount > 1
      ? t("renderControls.renderingProgress", { done: doneCount, count: totalCount })
      : t("renderControls.rendering");

  const activeMaskCount = view.masks.filter((m) => !m.hidden && m.assetId).length;

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">{t("renderControls.title")}</div>
          <div className="panel-subtitle">{t("renderControls.subtitle", { count: activeMaskCount })}</div>
        </div>
      </div>
      <div className="panel-body">
        <div className="render-controls-grid">
          <div className="field">
            <span className="field-label">{t("renderControls.model.label")}</span>
            <div className="segmented">
              <button
                type="button"
                className={`segmented-btn ${model === "pro" ? "segmented-btn-active" : ""}`}
                onClick={() => handleModelChange("pro")}
                disabled={rendering}
              >
                {t("renderControls.model.pro")}
              </button>
              <button
                type="button"
                className={`segmented-btn ${model === "flash" ? "segmented-btn-active" : ""}`}
                onClick={() => handleModelChange("flash")}
                disabled={rendering}
              >
                {t("renderControls.model.flash")}
              </button>
            </div>
          </div>

          <div className="field">
            <span className="field-label">{t("renderControls.resolution.label")}</span>
            <div className="segmented">
              {(["1K", "2K", "4K"] as Resolution[]).map((r) => (
                <button
                  key={r}
                  type="button"
                  className={`segmented-btn ${resolution === r ? "segmented-btn-active" : ""}`}
                  onClick={() => setResolution(r)}
                  disabled={rendering || (flashLocked && r !== "1K")}
                  title={flashLocked && r !== "1K" ? t("renderControls.resolution.flashOnlyTitle") : undefined}
                >
                  {r}
                </button>
              ))}
            </div>
            {flashLocked && <span className="field-hint">{t("renderControls.resolution.flashOnly")}</span>}
          </div>
        </div>

        <label className="checkbox-row">
          <input
            type="checkbox"
            checked={preservationCheck}
            onChange={(e) => setPreservationCheck(e.target.checked)}
            disabled={rendering}
          />
          {t("renderControls.preservationCheck")}
        </label>

        <label className="checkbox-row">
          <input
            type="checkbox"
            checked={variations === 2}
            onChange={(e) => setVariations(e.target.checked ? 2 : 1)}
            disabled={rendering}
          />
          {t("renderControls.variations")}
        </label>

        {error && <div className="error-banner">{error}</div>}

        {!rendering && lowBalance && me && estimatedTotalCredits !== undefined && (
          <div className="error-banner">
            {t("renderControls.insufficientCredits", {
              available: formatCredits(me.credits, i18n.language),
              required: formatCredits(estimatedTotalCredits, i18n.language),
            })}
          </div>
        )}

        <button
          type="button"
          className="btn btn-primary btn-block"
          onClick={handleRender}
          disabled={rendering || lowBalance}
        >
          {rendering ? <span className="spinner" /> : null}
          {rendering ? renderingLabel : t("renderControls.renderButton", { count: variations })}
        </button>
        {!rendering && estimatedTotalBrl !== undefined && (
          <div className={`field-hint render-cost-preview ${lowBalance ? "render-cost-preview-insufficient" : ""}`}>
            {t("renderControls.costPreview", {
              count: variations,
              cost: brlFormatter.format(estimatedTotalBrl),
              credits: estimatedTotalCredits !== undefined ? formatCredits(estimatedTotalCredits, i18n.language) : "",
            })}
          </div>
        )}
      </div>
    </section>
  );
}
