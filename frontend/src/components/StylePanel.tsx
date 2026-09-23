import { useState } from "react";
import { ApiError } from "../api";
import { useProject } from "../state/ProjectContext";
import { LIGHTING_PRESETS, type StyleSettings } from "../types";

export function StylePanel() {
  const { project, updateStyle } = useProject();
  const style = project?.style ?? null;

  const [draft, setDraft] = useState<StyleSettings | null>(style);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedTick, setSavedTick] = useState(0);

  // Resync the draft whenever the server-confirmed style object changes
  // (initial load or after a successful save), without fighting in-progress
  // edits. This is React's documented "adjust state during render" pattern.
  const [lastStyle, setLastStyle] = useState(style);
  if (lastStyle !== style) {
    setLastStyle(style);
    setDraft(style);
  }

  if (!draft) return null;

  const dirty = JSON.stringify(draft) !== JSON.stringify(style);

  function set<K extends keyof StyleSettings>(key: K, value: StyleSettings[K]) {
    setDraft((prev) => (prev ? { ...prev, [key]: value } : prev));
  }

  async function handleSave() {
    if (!draft) return;
    setSaving(true);
    setError(null);
    try {
      await updateStyle(draft);
      setSavedTick((n) => n + 1);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save style");
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">Style</div>
          <div className="panel-subtitle">Applied to every render in this project</div>
        </div>
        {!dirty && savedTick > 0 && <span className="badge badge-success">Saved</span>}
      </div>
      <div className="panel-body">
        <div className="field">
          <span className="field-label">Scene</span>
          <div className="segmented">
            <button
              type="button"
              className={`segmented-btn ${draft.scene === "interior" ? "segmented-btn-active" : ""}`}
              onClick={() => set("scene", "interior")}
            >
              Interior
            </button>
            <button
              type="button"
              className={`segmented-btn ${draft.scene === "exterior" ? "segmented-btn-active" : ""}`}
              onClick={() => set("scene", "exterior")}
            >
              Exterior
            </button>
          </div>
        </div>

        <div className="field">
          <label className="field-label" htmlFor="lighting">
            Lighting preset
          </label>
          <select
            id="lighting"
            className="select"
            value={draft.lighting}
            onChange={(e) => set("lighting", e.target.value as StyleSettings["lighting"])}
          >
            {LIGHTING_PRESETS.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </select>
        </div>

        <div className="field">
          <label className="field-label" htmlFor="light-direction">
            Light direction
          </label>
          <input
            id="light-direction"
            className="input"
            placeholder="e.g. low sun from the west"
            value={draft.lightDirection}
            onChange={(e) => set("lightDirection", e.target.value)}
          />
        </div>

        <div className="field">
          <label className="field-label" htmlFor="material-notes">
            Material notes
          </label>
          <textarea
            id="material-notes"
            className="textarea"
            rows={3}
            placeholder="e.g. white oak flooring, matte black fixtures"
            value={draft.materialNotes}
            onChange={(e) => set("materialNotes", e.target.value)}
          />
        </div>

        <div className="field">
          <label className="field-label" htmlFor="extra-instructions">
            Extra instructions
          </label>
          <textarea
            id="extra-instructions"
            className="textarea"
            rows={3}
            placeholder="Anything else the model should know"
            value={draft.extraInstructions}
            onChange={(e) => set("extraInstructions", e.target.value)}
          />
        </div>

        {error && <div className="error-banner">{error}</div>}

        <button
          type="button"
          className="btn btn-primary btn-block"
          onClick={handleSave}
          disabled={!dirty || saving}
        >
          {saving ? <span className="spinner" /> : null}
          {dirty ? "Save style" : "Saved"}
        </button>
      </div>
    </section>
  );
}
