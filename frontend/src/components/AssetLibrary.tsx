import { useState } from "react";
import { ApiError } from "../api";
import { useProject } from "../state/ProjectContext";
import { AssetCard } from "./AssetCard";

const SUGGESTED_COLORS = [
  "#c0392b",
  "#2980b9",
  "#27ae60",
  "#e67e22",
  "#8e44ad",
  "#16a085",
  "#d35400",
  "#2c3e50",
  "#c2185b",
  "#00838f",
];

function nextSuggestedColor(used: string[]): string {
  const lower = used.map((c) => c.toLowerCase());
  return SUGGESTED_COLORS.find((c) => !lower.includes(c.toLowerCase())) ?? "#4338ca";
}

export function AssetLibrary() {
  const { project, createAsset } = useProject();
  const assets = project?.assets ?? [];

  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [color, setColor] = useState("#4338ca");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function openForm() {
    setColor(nextSuggestedColor(assets.map((a) => a.color)));
    setAdding(true);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    setSubmitting(true);
    setError(null);
    try {
      await createAsset({ name: name.trim(), description, color });
      setName("");
      setDescription("");
      setAdding(false);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create asset");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">Asset library</div>
          <div className="panel-subtitle">Reused across views</div>
        </div>
        {!adding && (
          <button type="button" className="btn btn-primary btn-sm" onClick={openForm}>
            + Add asset
          </button>
        )}
      </div>
      <div className="panel-body">
        {adding && (
          <form className="asset-add-form" onSubmit={handleSubmit}>
            <div className="field">
              <input
                className="input"
                placeholder="Name, e.g. Sofa"
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoFocus
              />
            </div>
            <div className="field">
              <textarea
                className="textarea"
                rows={2}
                placeholder="Description, e.g. dark green velvet three-seat sofa"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </div>
            <div className="field asset-color-field">
              <label className="field-label" htmlFor="new-asset-color">
                Color
              </label>
              <input
                id="new-asset-color"
                type="color"
                className="color-input"
                value={color}
                onChange={(e) => setColor(e.target.value)}
              />
              <span className="field-hint">{color}</span>
            </div>
            {error && <div className="error-banner">{error}</div>}
            <div className="asset-card-actions">
              <button type="button" className="btn btn-ghost btn-sm" onClick={() => setAdding(false)}>
                Cancel
              </button>
              <button type="submit" className="btn btn-primary btn-sm" disabled={submitting || !name.trim()}>
                {submitting ? <span className="spinner" /> : null}
                Create
              </button>
            </div>
          </form>
        )}

        {assets.length === 0 && !adding && (
          <div className="empty-state">
            <strong>No assets yet</strong>
            <span>Add furniture or fixtures to place them in your views.</span>
          </div>
        )}

        {assets.length > 0 && (
          <ul className="asset-list">
            {assets.map((asset) => (
              <AssetCard key={asset.id} asset={asset} />
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
