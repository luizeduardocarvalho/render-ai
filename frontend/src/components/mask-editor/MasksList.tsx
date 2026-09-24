import { useTranslation } from "react-i18next";
import type { Asset, Mask } from "../../types";
import { getMaskColor } from "./maskCanvas";

export type MaskSaveStatus = "idle" | "saving" | "saved" | "error";

interface MasksListProps {
  masks: Mask[];
  assets: Asset[];
  selectedMaskId: string | null;
  onSelect: (id: string) => void;
  onToggleHidden: (mask: Mask) => void;
  onDelete: (mask: Mask) => void;
  onAssignAsset: (mask: Mask, assetId: string | null) => void;
  saveStatus: Record<string, MaskSaveStatus>;
}

export function MasksList({
  masks,
  assets,
  selectedMaskId,
  onSelect,
  onToggleHidden,
  onDelete,
  onAssignAsset,
  saveStatus,
}: MasksListProps) {
  const { t } = useTranslation();
  if (masks.length === 0) {
    return (
      <div className="empty-state masks-empty">
        <strong>{t("masksList.empty.title")}</strong>
        <span>{t("masksList.empty.body")}</span>
      </div>
    );
  }

  return (
    <ul className="masks-list">
      {masks.map((mask, i) => {
        const color = getMaskColor(mask, assets);
        const asset = assets.find((a) => a.id === mask.assetId) ?? null;
        const status = saveStatus[mask.id] ?? "idle";
        const selected = mask.id === selectedMaskId;
        return (
          <li key={mask.id}>
            {/* Row is a div, not a button, because it contains interactive
                controls (a select and action buttons) - a button cannot nest
                other buttons. role/tabIndex/onKeyDown keep it keyboard usable. */}
            <div
              className={`mask-row ${selected ? "mask-row-selected" : ""}`}
              role="button"
              tabIndex={0}
              onClick={() => onSelect(mask.id)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onSelect(mask.id);
                }
              }}
            >
              <span
                className="mask-swatch"
                style={{ backgroundColor: color, opacity: mask.hidden ? 0.35 : 1 }}
              />
              <span className="mask-row-text">
                <span className="mask-row-title">{t("masksList.maskLabel", { index: i + 1 })}</span>
                <span className="mask-row-subtitle">
                  {asset ? asset.name : t("masksList.unassigned")}
                  {status === "saving" && t("masksList.saving")}
                  {status === "error" && t("masksList.saveFailed")}
                </span>
              </span>

              <select
                className="select mask-row-select"
                value={mask.assetId ?? ""}
                onClick={(e) => e.stopPropagation()}
                onChange={(e) => onAssignAsset(mask, e.target.value || null)}
              >
                <option value="">{t("masksList.unassigned")}</option>
                {assets.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>

              <span className="mask-row-actions">
                <button
                  type="button"
                  className="btn btn-ghost btn-icon btn-sm"
                  title={mask.hidden ? t("masksList.show") : t("masksList.hide")}
                  onClick={(e) => {
                    e.stopPropagation();
                    onToggleHidden(mask);
                  }}
                >
                  {mask.hidden ? <EyeOffIcon /> : <EyeIcon />}
                </button>
                <button
                  type="button"
                  className="btn btn-danger btn-icon btn-sm"
                  title={t("masksList.deleteMask")}
                  onClick={(e) => {
                    e.stopPropagation();
                    onDelete(mask);
                  }}
                >
                  <TrashIcon />
                </button>
              </span>
            </div>
          </li>
        );
      })}
    </ul>
  );
}

function EyeIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7-10-7-10-7Z"
        stroke="currentColor"
        strokeWidth="1.6"
      />
      <circle cx="12" cy="12" r="3" stroke="currentColor" strokeWidth="1.6" />
    </svg>
  );
}

function EyeOffIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M3 3l18 18M10.6 5.2A10.8 10.8 0 0 1 12 5c6.4 0 10 7 10 7a15.6 15.6 0 0 1-3.4 4.3M6.7 6.7C4 8.5 2 12 2 12s3.6 7 10 7c1.4 0 2.6-.3 3.7-.8M9.9 9.9a3 3 0 0 0 4.2 4.2"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinecap="round"
      />
    </svg>
  );
}

function TrashIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
