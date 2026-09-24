import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "../api";
import { useAssetReferenceUrl } from "../hooks/useSignedImageUrl";
import { useProject } from "../state/ProjectContext";
import type { Asset } from "../types";
import { ConfirmDialog } from "./ConfirmDialog";

/**
 * Library-screen counterpart to AssetCard: same look, but talks to the
 * per-user library routes (no project id in scope) via ProjectContext.
 */
export function LibraryAssetCard({ asset }: { asset: Asset }) {
  const { t } = useTranslation();
  const { updateLibraryAsset, deleteLibraryAsset, uploadLibraryAssetReference } = useProject();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(asset.name);
  const [description, setDescription] = useState(asset.description);
  const [color, setColor] = useState(asset.color);
  const [saving, setSaving] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const referenceUrl = useAssetReferenceUrl(asset.id, asset.hasReferenceImage);

  async function handleSave() {
    if (!name.trim()) return;
    setSaving(true);
    setError(null);
    try {
      await updateLibraryAsset(asset.id, { name: name.trim(), description, color });
      setEditing(false);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("assetCard.errors.update"));
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    setDeleting(true);
    setError(null);
    try {
      await deleteLibraryAsset(asset.id);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : t("assetCard.errors.delete");
      setError(message);
      setDeleting(false);
      throw new Error(message);
    }
  }

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploading(true);
    setError(null);
    try {
      await uploadLibraryAssetReference(asset.id, file);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("assetCard.errors.upload"));
    } finally {
      setUploading(false);
      e.target.value = "";
    }
  }

  if (editing) {
    return (
      <li className="asset-card asset-card-editing">
        <div className="field">
          <input
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("assetCard.namePlaceholder")}
          />
        </div>
        <div className="field">
          <textarea
            className="textarea"
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t("assetCard.descriptionPlaceholder")}
          />
        </div>
        <div className="field asset-color-field">
          <label className="field-label" htmlFor={`library-color-${asset.id}`}>
            {t("assetCard.colorLabel")}
          </label>
          <input
            id={`library-color-${asset.id}`}
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
            {t("assetCard.cancel")}
          </button>
          <button className="btn btn-primary btn-sm" onClick={handleSave} disabled={saving || !name.trim()}>
            {saving ? <span className="spinner" /> : null}
            {t("assetCard.save")}
          </button>
        </div>
      </li>
    );
  }

  return (
    <li className="asset-card">
      <div className="asset-card-thumb" style={{ borderColor: asset.color }}>
        {asset.hasReferenceImage && referenceUrl ? (
          <img src={referenceUrl} alt={asset.name} />
        ) : (
          <span className="asset-card-thumb-empty">{t("assetCard.noPhoto")}</span>
        )}
        <span className="mask-swatch asset-card-swatch" style={{ backgroundColor: asset.color }} />
      </div>
      <div className="asset-card-body">
        <div className="asset-card-name">{asset.name}</div>
        <div className="asset-card-desc">{asset.description || t("assetCard.noDescription")}</div>
        {error && <div className="error-banner asset-card-error">{error}</div>}
        <div className="asset-card-actions">
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={() => fileInputRef.current?.click()}
            disabled={uploading}
          >
            {uploading ? <span className="spinner" /> : null}
            {asset.hasReferenceImage ? t("assetCard.replacePhoto") : t("assetCard.addPhoto")}
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            className="visually-hidden"
            onChange={handleFileChange}
          />
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setEditing(true)}>
            {t("assetCard.edit")}
          </button>
          <button
            type="button"
            className="btn btn-danger btn-sm"
            onClick={() => setConfirmingDelete(true)}
            disabled={deleting}
          >
            {deleting ? <span className="spinner" /> : t("assetCard.delete")}
          </button>
        </div>
      </div>
      {confirmingDelete && (
        <ConfirmDialog
          title={t("assetCard.delete")}
          body={t("assetCard.deleteConfirm", { name: asset.name })}
          confirmLabel={t("common.delete")}
          onConfirm={handleDelete}
          onCancel={() => setConfirmingDelete(false)}
        />
      )}
    </li>
  );
}
