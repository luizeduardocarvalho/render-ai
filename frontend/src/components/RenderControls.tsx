import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError, getPricing } from "../api";
import { useProject } from "../state/ProjectContext";
import type { ModelChoice, PricingResponse, Render, Resolution, View } from "../types";

interface RenderControlsProps {
  view: View;
  onRendered: (renders: Render[]) => void;
}

export function RenderControls({ view, onRendered }: RenderControlsProps) {
  const { t, i18n } = useTranslation();
  const { renderView } = useProject();
  const [model, setModel] = useState<ModelChoice>("pro");
  const [resolution, setResolution] = useState<Resolution>("2K");
  const [preservationCheck, setPreservationCheck] = useState(true);
  const [variations, setVariations] = useState<1 | 2>(1);
  const [rendering, setRendering] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pricing, setPricing] = useState<PricingResponse | null>(null);

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

  function handleModelChange(next: ModelChoice) {
    setModel(next);
    if (next === "flash") setResolution("1K");
  }

  async function handleRender() {
    setRendering(true);
    setError(null);
    try {
      const renders = await renderView(view.id, { model, resolution, preservationCheck, variations });
      onRendered(renders);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("renderControls.error"));
    } finally {
      setRendering(false);
    }
  }

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

        <button type="button" className="btn btn-primary btn-block" onClick={handleRender} disabled={rendering}>
          {rendering ? <span className="spinner" /> : null}
          {rendering ? t("renderControls.rendering") : t("renderControls.renderButton", { count: variations })}
        </button>
        {!rendering && estimatedTotalBrl !== undefined && (
          <div className="field-hint render-cost-preview">
            {t("renderControls.costPreview", {
              count: variations,
              cost: brlFormatter.format(estimatedTotalBrl),
            })}
          </div>
        )}
      </div>
    </section>
  );
}
