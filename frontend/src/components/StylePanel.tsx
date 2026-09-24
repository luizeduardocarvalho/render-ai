import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "../api";
import { useProject } from "../state/ProjectContext";
import { INTERIOR_LIGHTS_OPTIONS, LIGHTING_PRESETS, type StyleSettings } from "../types";

export function StylePanel() {
  const { t } = useTranslation();
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
      setError(err instanceof ApiError ? err.message : t("style.error"));
    } finally {
      setSaving(false);
    }
  }

  const activeHint = INTERIOR_LIGHTS_OPTIONS.find((o) => o.value === draft.interiorLights);

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">{t("style.title")}</div>
          <div className="panel-subtitle">{t("style.subtitle")}</div>
        </div>
        {!dirty && savedTick > 0 && <span className="badge badge-success">{t("style.saved")}</span>}
      </div>
      <div className="panel-body">
        <div className="field">
          <span className="field-label">{t("style.scene.label")}</span>
          <div className="segmented">
            <button
              type="button"
              className={`segmented-btn ${draft.scene === "interior" ? "segmented-btn-active" : ""}`}
              onClick={() => set("scene", "interior")}
            >
              {t("style.scene.interior")}
            </button>
            <button
              type="button"
              className={`segmented-btn ${draft.scene === "exterior" ? "segmented-btn-active" : ""}`}
              onClick={() => set("scene", "exterior")}
            >
              {t("style.scene.exterior")}
            </button>
          </div>
        </div>

        <div className="field">
          <label className="field-label" htmlFor="lighting">
            {t("style.lighting.label")}
          </label>
          <select
            id="lighting"
            className="select"
            value={draft.lighting}
            onChange={(e) => set("lighting", e.target.value as StyleSettings["lighting"])}
          >
            {LIGHTING_PRESETS.map((p) => (
              <option key={p.value} value={p.value}>
                {t(p.labelKey)}
              </option>
            ))}
          </select>
        </div>

        <div className="field">
          <label className="field-label" htmlFor="light-direction">
            {t("style.lightDirection.label")}
          </label>
          <input
            id="light-direction"
            className="input"
            placeholder={t("style.lightDirection.placeholder")}
            value={draft.lightDirection}
            onChange={(e) => set("lightDirection", e.target.value)}
          />
        </div>

        <div className="field">
          <span className="field-label" id="interior-lights-label">
            {t("style.interiorLights.label")}
          </span>
          <div className="segmented" role="radiogroup" aria-labelledby="interior-lights-label">
            {INTERIOR_LIGHTS_OPTIONS.map((o) => (
              <button
                key={o.value}
                type="button"
                role="radio"
                aria-checked={draft.interiorLights === o.value}
                title={t(o.hintKey)}
                className={`segmented-btn ${draft.interiorLights === o.value ? "segmented-btn-active" : ""}`}
                onClick={() => set("interiorLights", o.value)}
              >
                {t(o.labelKey)}
              </button>
            ))}
          </div>
          <span className="field-hint">{activeHint ? t(activeHint.hintKey) : t("style.interiorLights.notSet")}</span>
        </div>

        <div className="field">
          <label className="field-label" htmlFor="material-notes">
            {t("style.materialNotes.label")}
          </label>
          <textarea
            id="material-notes"
            className="textarea"
            rows={3}
            placeholder={t("style.materialNotes.placeholder")}
            value={draft.materialNotes}
            onChange={(e) => set("materialNotes", e.target.value)}
          />
        </div>

        <div className="field">
          <label className="field-label" htmlFor="extra-instructions">
            {t("style.extraInstructions.label")}
          </label>
          <textarea
            id="extra-instructions"
            className="textarea"
            rows={3}
            placeholder={t("style.extraInstructions.placeholder")}
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
          {dirty ? t("style.save") : t("style.saved")}
        </button>
      </div>
    </section>
  );
}
