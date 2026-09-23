import { useState } from "react";
import { ApiError } from "../api";
import { useProject } from "../state/ProjectContext";
import type { ModelChoice, Render, Resolution, View } from "../types";

interface RenderControlsProps {
  view: View;
  onRendered: (renders: Render[]) => void;
}

export function RenderControls({ view, onRendered }: RenderControlsProps) {
  const { renderView } = useProject();
  const [model, setModel] = useState<ModelChoice>("pro");
  const [resolution, setResolution] = useState<Resolution>("2K");
  const [preservationCheck, setPreservationCheck] = useState(true);
  const [variations, setVariations] = useState<1 | 2>(1);
  const [rendering, setRendering] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const flashLocked = model === "flash";

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
      setError(err instanceof ApiError ? err.message : "Render failed. Please try again.");
    } finally {
      setRendering(false);
    }
  }

  const activeMaskCount = view.masks.filter((m) => !m.hidden && m.assetId).length;

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">Render</div>
          <div className="panel-subtitle">
            {activeMaskCount} region{activeMaskCount === 1 ? "" : "s"} will be sent to the model
          </div>
        </div>
      </div>
      <div className="panel-body">
        <div className="render-controls-grid">
          <div className="field">
            <span className="field-label">Model</span>
            <div className="segmented">
              <button
                type="button"
                className={`segmented-btn ${model === "pro" ? "segmented-btn-active" : ""}`}
                onClick={() => handleModelChange("pro")}
                disabled={rendering}
              >
                Pro
              </button>
              <button
                type="button"
                className={`segmented-btn ${model === "flash" ? "segmented-btn-active" : ""}`}
                onClick={() => handleModelChange("flash")}
                disabled={rendering}
              >
                Flash
              </button>
            </div>
          </div>

          <div className="field">
            <span className="field-label">Resolution</span>
            <div className="segmented">
              {(["1K", "2K", "4K"] as Resolution[]).map((r) => (
                <button
                  key={r}
                  type="button"
                  className={`segmented-btn ${resolution === r ? "segmented-btn-active" : ""}`}
                  onClick={() => setResolution(r)}
                  disabled={rendering || (flashLocked && r !== "1K")}
                  title={flashLocked && r !== "1K" ? "Flash only supports 1K" : undefined}
                >
                  {r}
                </button>
              ))}
            </div>
            {flashLocked && <span className="field-hint">Flash only supports 1K resolution.</span>}
          </div>
        </div>

        <label className="checkbox-row">
          <input
            type="checkbox"
            checked={preservationCheck}
            onChange={(e) => setPreservationCheck(e.target.checked)}
            disabled={rendering}
          />
          Preservation check (compares unmasked regions against the original)
        </label>

        <label className="checkbox-row">
          <input
            type="checkbox"
            checked={variations === 2}
            onChange={(e) => setVariations(e.target.checked ? 2 : 1)}
            disabled={rendering}
          />
          Generate 2 variations (independent samples, both kept for comparison)
        </label>

        {error && <div className="error-banner">{error}</div>}

        <button type="button" className="btn btn-primary btn-block" onClick={handleRender} disabled={rendering}>
          {rendering ? <span className="spinner" /> : null}
          {rendering
            ? "Rendering... this can take up to a minute"
            : `Generate render${variations > 1 ? ` (${variations} variations)` : ""}`}
        </button>
      </div>
    </section>
  );
}
