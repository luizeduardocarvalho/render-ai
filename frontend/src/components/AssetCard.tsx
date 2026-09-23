import { useRef, useState } from "react";
import { ApiError, imageUrl } from "../api";
import { useProject } from "../state/ProjectContext";
import type { Asset } from "../types";

interface AssetCardProps {
  asset: Asset;
}

export function AssetCard({ asset }: AssetCardProps) {
  const { updateAsset, deleteAsset, uploadAssetReference } = useProject();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(asset.name);
  const [description, setDescription] = useState(asset.description);
  const [color, setColor] = useState(asset.color);
  const [saving, setSaving] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  async function handleSave() {
    if (!name.trim()) return;
    setSaving(true);
    setError(null);
    try {
      await updateAsset(asset.id, { name: name.trim(), description, color });
      setEditing(false);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to update asset");
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    if (!window.confirm(`Delete asset "${asset.name}"? Masks assigned to it will become unassigned.`)) {
      return;
    }
    setDeleting(true);
    setError(null);
    try {
      await deleteAsset(asset.id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to delete asset");
      setDeleting(false);
    }
  }

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploading(true);
    setError(null);
    try {
      await uploadAssetReference(asset.id, file);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to upload reference photo");
    } finally {
      setUploading(false);
      e.target.value = "";
    }
  }

  if (editing) {
    return (
      <li className="asset-card asset-card-editing">
        <div className="field">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Name" />
        </div>
        <div className="field">
          <textarea
            className="textarea"
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Description, e.g. dark green velvet three-seat sofa"
          />
        </div>
        <div className="field asset-color-field">
          <label className="field-label" htmlFor={`color-${asset.id}`}>
            Color
          </label>
          <input
            id={`color-${asset.id}`}
            type="color"
            className="color-input"
            value={color}
            onChange={(e) => setColor(e.target.value)}
          />
          <span className="field-hint">{color}</span>
        </div>
        {error && <div className="error-banner">{error}</div>}
        <div className="asset-card-actions">
          <button className="btn btn-ghost btn-sm" onClick={() => setEditing(false)} disabled={saving}>
            Cancel
          </button>
          <button className="btn btn-primary btn-sm" onClick={handleSave} disabled={saving || !name.trim()}>
            {saving ? <span className="spinner" /> : null}
            Save
          </button>
        </div>
      </li>
    );
  }

  return (
    <li className="asset-card">
      <div className="asset-card-thumb" style={{ borderColor: asset.color }}>
        {asset.hasReferenceImage ? (
          <img src={imageUrl(asset.referenceImageId)} alt={asset.name} />
        ) : (
          <span className="asset-card-thumb-empty">No photo</span>
        )}
        <span className="mask-swatch asset-card-swatch" style={{ backgroundColor: asset.color }} />
      </div>
      <div className="asset-card-body">
        <div className="asset-card-name">{asset.name}</div>
        <div className="asset-card-desc">{asset.description || "No description"}</div>
        {error && <div className="error-banner asset-card-error">{error}</div>}
        <div className="asset-card-actions">
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={() => fileInputRef.current?.click()}
            disabled={uploading}
          >
            {uploading ? <span className="spinner" /> : null}
            {asset.hasReferenceImage ? "Replace photo" : "Add photo"}
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            className="visually-hidden"
            onChange={handleFileChange}
          />
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setEditing(true)}>
            Edit
          </button>
          <button type="button" className="btn btn-danger btn-sm" onClick={handleDelete} disabled={deleting}>
            {deleting ? <span className="spinner" /> : "Delete"}
          </button>
        </div>
      </div>
    </li>
  );
}
