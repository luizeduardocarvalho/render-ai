import { useTranslation } from "react-i18next";
import type { Tool } from "./MaskEditor";

interface MaskToolbarProps {
  tool: Tool;
  onToolChange: (tool: Tool) => void;
  brushSize: number;
  onBrushSizeChange: (size: number) => void;
  minBrush: number;
  maxBrush: number;
  zoomPct: number;
  onZoomChange: (zoomPct: number) => void;
  onAddMask: () => void;
  addingMask: boolean;
  toolsDisabled: boolean;
  savingCount: number;
}

export function MaskToolbar({
  tool,
  onToolChange,
  brushSize,
  onBrushSizeChange,
  minBrush,
  maxBrush,
  zoomPct,
  onZoomChange,
  onAddMask,
  addingMask,
  toolsDisabled,
  savingCount,
}: MaskToolbarProps) {
  const { t } = useTranslation();
  return (
    <div className="mask-toolbar">
      <div className="mask-toolbar-group" role="group" aria-label={t("maskToolbar.brush")}>
        <button
          type="button"
          className={`tool-btn ${tool === "brush" ? "tool-btn-active" : ""}`}
          onClick={() => onToolChange("brush")}
          disabled={toolsDisabled}
          title={t("maskToolbar.brush")}
        >
          <BrushIcon /> {t("maskToolbar.brush")}
        </button>
        <button
          type="button"
          className={`tool-btn ${tool === "eraser" ? "tool-btn-active" : ""}`}
          onClick={() => onToolChange("eraser")}
          disabled={toolsDisabled}
          title={t("maskToolbar.eraser")}
        >
          <EraserIcon /> {t("maskToolbar.eraser")}
        </button>
      </div>

      <div className="mask-toolbar-group mask-toolbar-slider">
        <span className="mask-toolbar-label">{t("maskToolbar.size")}</span>
        <input
          type="range"
          min={minBrush}
          max={maxBrush}
          value={brushSize}
          disabled={toolsDisabled}
          onChange={(e) => onBrushSizeChange(Number(e.target.value))}
        />
        <span className="mask-toolbar-value">{brushSize}px</span>
      </div>

      <div className="mask-toolbar-group mask-toolbar-slider">
        <span className="mask-toolbar-label">{t("maskToolbar.zoom")}</span>
        <input
          type="range"
          min={50}
          max={300}
          step={10}
          value={zoomPct}
          onChange={(e) => onZoomChange(Number(e.target.value))}
        />
        <span className="mask-toolbar-value">{zoomPct}%</span>
        <button type="button" className="btn btn-ghost btn-sm" onClick={() => onZoomChange(100)}>
          {t("maskToolbar.fit")}
        </button>
      </div>

      <div className="mask-toolbar-spacer" />

      {savingCount > 0 && (
        <span className="mask-toolbar-saving">
          <span className="spinner" /> {t("maskToolbar.saving", { count: savingCount })}
        </span>
      )}

      <button type="button" className="btn btn-primary btn-sm" onClick={onAddMask} disabled={addingMask}>
        {addingMask ? <span className="spinner" /> : "+"} {t("maskToolbar.addMask")}
      </button>
    </div>
  );
}

function BrushIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M7 21c-2 0-3.5-1.2-3.5-3.4C3.5 15.4 5 14 7 14c1.4 0 2.5.7 3 1.8L18 7.6a1.7 1.7 0 0 1 2.4 2.4l-8.2 8a3.9 3.9 0 0 1-1.8 3.3A4.6 4.6 0 0 1 7 21Z"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function EraserIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M18.5 13.5 10 22H5l-2-2a2 2 0 0 1 0-2.8L14.7 5.5a2 2 0 0 1 2.8 0l3.5 3.5a2 2 0 0 1 0 2.8L18.5 14"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinejoin="round"
        strokeLinecap="round"
      />
      <path d="M8 22h11" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  );
}
