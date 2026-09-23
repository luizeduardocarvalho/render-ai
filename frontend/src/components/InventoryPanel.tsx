import { useState } from "react";
import { ApiError } from "../api";
import { useProject } from "../state/ProjectContext";
import type { View } from "../types";

export function InventoryPanel({ view }: { view: View }) {
  const { updateInventory, generateInventory } = useProject();
  const [text, setText] = useState(view.inventory);
  const [generating, setGenerating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset the draft only when switching views (not on every keystroke-unrelated
  // prop change), so we don't fight with in-progress edits. This is React's
  // documented "adjust state during render" pattern, not an effect.
  const [lastViewId, setLastViewId] = useState(view.id);
  if (lastViewId !== view.id) {
    setLastViewId(view.id);
    setText(view.inventory);
  }

  const dirty = text !== view.inventory;

  async function handleGenerate() {
    setGenerating(true);
    setError(null);
    try {
      const inventory = await generateInventory(view.id);
      setText(inventory);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to generate inventory");
    } finally {
      setGenerating(false);
    }
  }

  async function handleSave() {
    setSaving(true);
    setError(null);
    try {
      await updateInventory(view.id, text);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save inventory");
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">Object inventory</div>
          <div className="panel-subtitle">Used to check what the render preserves</div>
        </div>
        <button type="button" className="btn btn-sm" onClick={handleGenerate} disabled={generating}>
          {generating ? <span className="spinner" /> : null}
          Generate from screenshot
        </button>
      </div>
      <div className="panel-body">
        <textarea
          className="textarea inventory-textarea"
          rows={8}
          placeholder="No inventory yet. Generate one from the screenshot, or write it yourself."
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
        {error && <div className="error-banner">{error}</div>}
        <button type="button" className="btn btn-primary" onClick={handleSave} disabled={!dirty || saving}>
          {saving ? <span className="spinner" /> : null}
          {dirty ? "Save inventory" : "Saved"}
        </button>
      </div>
    </section>
  );
}
