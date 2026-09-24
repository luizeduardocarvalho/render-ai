import { useTranslation } from "react-i18next";
import { MAX_INSTRUCTION_LENGTH, isRegionComplete, type EditRegion } from "./editRegions";

interface EditRegionsListProps {
  regions: EditRegion[];
  selectedId: string | null;
  disabled: boolean;
  onSelect: (id: string) => void;
  onInstructionChange: (id: string, instruction: string) => void;
  onDelete: (id: string) => void;
}

export function EditRegionsList({
  regions,
  selectedId,
  disabled,
  onSelect,
  onInstructionChange,
  onDelete,
}: EditRegionsListProps) {
  const { t } = useTranslation();

  if (regions.length === 0) {
    return (
      <div className="empty-state masks-empty">
        <strong>{t("editRegions.empty.title")}</strong>
        <span>{t("editRegions.empty.body")}</span>
      </div>
    );
  }

  return (
    <ul className="masks-list">
      {regions.map((region, i) => {
        const selected = region.id === selectedId;
        const hint = !region.painted
          ? t("editRegions.hint.paint")
          : region.instruction.trim() === ""
            ? t("editRegions.hint.describe")
            : null;
        return (
          <li key={region.id}>
            <div
              className={`edit-region-row ${selected ? "mask-row-selected" : ""}`}
              onClick={() => onSelect(region.id)}
            >
              <div className="edit-region-row-head">
                <span className="mask-swatch" style={{ backgroundColor: region.color }} />
                <span className="mask-row-title">{t("editRegions.regionLabel", { index: i + 1 })}</span>
                {isRegionComplete(region) ? (
                  <span className="edit-region-ok">{t("editRegions.ready")}</span>
                ) : (
                  <span className="edit-region-hint">{hint}</span>
                )}
                <button
                  type="button"
                  className="btn btn-danger btn-icon btn-sm"
                  title={t("editRegions.deleteRegion")}
                  disabled={disabled}
                  onClick={(e) => {
                    e.stopPropagation();
                    onDelete(region.id);
                  }}
                >
                  <TrashIcon />
                </button>
              </div>
              <textarea
                className="textarea edit-region-textarea"
                rows={3}
                value={region.instruction}
                maxLength={MAX_INSTRUCTION_LENGTH}
                disabled={disabled}
                placeholder={t("editRegions.placeholder")}
                aria-label={t("editRegions.instructionLabel", { index: i + 1 })}
                onFocus={() => onSelect(region.id)}
                onChange={(e) => onInstructionChange(region.id, e.target.value)}
              />
            </div>
          </li>
        );
      })}
    </ul>
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
