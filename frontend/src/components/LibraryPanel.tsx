import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "../api";
import { useProject } from "../state/ProjectContext";
import { LibraryAssetCard } from "./LibraryAssetCard";

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

/**
 * "Asset library" tab on the project list screen: the user's whole reusable
 * asset library, independent of any open project. Mirrors AssetLibrary.tsx's
 * layout but talks to the library-scoped routes via ProjectContext.
 */
export function LibraryPanel() {
  const { t } = useTranslation();
  const {
    libraryAssets,
    libraryAssetsLoading,
    libraryAssetsError,
    refreshLibraryAssets,
    createLibraryAsset,
  } = useProject();
  const assets = libraryAssets ?? [];

  useEffect(() => {
    // Always refetch on mount: assets may have changed from inside a project's
    // editor (same per-user library) since this list was last loaded.
    void refreshLibraryAssets();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
      await createLibraryAsset({ name: name.trim(), description, color });
      setName("");
      setDescription("");
      setAdding(false);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("assetLibrary.error"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <div className="panel-title">{t("assetLibrary.title")}</div>
          <div className="panel-subtitle">{t("library.subtitle")}</div>
        </div>
        {!adding && (
          <button type="button" className="btn btn-primary btn-sm" onClick={openForm}>
            {t("assetLibrary.addAsset")}
          </button>
        )}
      </div>
      <div className="panel-body">
        {adding && (
          <form className="asset-add-form" onSubmit={handleSubmit}>
            <div className="field">
              <input
                className="input"
                placeholder={t("assetLibrary.namePlaceholder")}
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoFocus
              />
            </div>
            <div className="field">
              <textarea
                className="textarea"
                rows={2}
                placeholder={t("assetLibrary.descriptionPlaceholder")}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </div>
            <div className="field asset-color-field">
              <label className="field-label" htmlFor="new-library-asset-color">
                {t("assetLibrary.colorLabel")}
              </label>
              <input
                id="new-library-asset-color"
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
                {t("assetLibrary.cancel")}
              </button>
              <button type="submit" className="btn btn-primary btn-sm" disabled={submitting || !name.trim()}>
                {submitting ? <span className="spinner" /> : null}
                {t("assetLibrary.create")}
              </button>
            </div>
          </form>
        )}

        {libraryAssetsLoading && (
          <div className="picker-status">
            <span className="spinner" /> {t("library.loading")}
          </div>
        )}

        {libraryAssetsError && !libraryAssetsLoading && (
          <div className="picker-status picker-status-error">
            <span>{libraryAssetsError}</span>
            <button type="button" className="btn btn-sm" onClick={() => void refreshLibraryAssets()}>
              {t("picker.retry")}
            </button>
          </div>
        )}

        {!libraryAssetsLoading && !libraryAssetsError && assets.length === 0 && !adding && (
          <div className="empty-state">
            <strong>{t("assetLibrary.empty.title")}</strong>
            <span>{t("library.empty.body")}</span>
          </div>
        )}

        {assets.length > 0 && (
          <ul className="asset-list">
            {assets.map((asset) => (
              <LibraryAssetCard key={asset.id} asset={asset} />
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
