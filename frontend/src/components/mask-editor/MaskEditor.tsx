import Konva from "konva";
import { useEffect, useMemo, useReducer, useRef, useState } from "react";
import { Circle, Image as KonvaImage, Layer, Stage } from "react-konva";
import { useTranslation } from "react-i18next";
import { fetchSignedImageUrl } from "../../api";
import { useImage } from "../../hooks/useImage";
import { useSignedImageUrl } from "../../hooks/useSignedImageUrl";
import { useProject } from "../../state/ProjectContext";
import type { Mask, View } from "../../types";
import {
  createCanvas,
  exportRawToPng,
  getMaskColor,
  importBitmapToRaw,
  paintSegment,
  recolorAll,
  recolorRegion,
  strokeDirtyRect,
  type Point,
} from "./maskCanvas";
import "./MaskEditor.css";
import { MaskToolbar } from "./MaskToolbar";
import { MasksList, type MaskSaveStatus } from "./MasksList";
import { ConfirmDialog } from "../ConfirmDialog";

export type Tool = "brush" | "eraser";

interface CanvasEntry {
  raw: HTMLCanvasElement;
  display: HTMLCanvasElement;
  lastColor: string | null;
  bitmapState: "none" | "loading" | "loaded";
}

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n));
}

export function MaskEditor({ view }: { view: View }) {
  const { t } = useTranslation();
  const { project, createMask, updateMask, deleteMask, uploadMaskBitmap } = useProject();
  const assets = project?.assets ?? [];

  const [selectedMaskId, setSelectedMaskId] = useState<string | null>(view.masks[0]?.id ?? null);
  const [tool, setTool] = useState<Tool>("brush");
  const defaultBrush = clamp(Math.round(view.width / 120), 6, 80);
  const [brushSize, setBrushSize] = useState(defaultBrush);
  const minBrush = Math.max(2, Math.round(view.width / 800));
  const maxBrush = Math.max(minBrush + 20, Math.round(view.width / 8));
  const [zoomPct, setZoomPct] = useState(100);
  const [addingMask, setAddingMask] = useState(false);
  const [saveStatus, setSaveStatus] = useState<Record<string, MaskSaveStatus>>({});
  const [editorError, setEditorError] = useState<string | null>(null);
  const [pendingDeleteMask, setPendingDeleteMask] = useState<Mask | null>(null);

  // Ensure the selected mask stays valid as masks are added/removed.
  useEffect(() => {
    if (selectedMaskId && view.masks.some((m) => m.id === selectedMaskId)) return;
    setSelectedMaskId(view.masks[0]?.id ?? null);
  }, [view.masks, selectedMaskId]);

  const screenshotUrl = useSignedImageUrl(view.hasScreenshot ? view.screenshotImageId : null);
  const { image: screenshotImg, loading: screenshotLoading, error: screenshotError } = useImage(
    screenshotUrl ?? null,
  );

  const canvasStore = useRef(new Map<string, CanvasEntry>());
  const [, forceTick] = useReducer((n: number) => n + 1, 0);
  const layerRef = useRef<Konva.Layer>(null);
  const stageRef = useRef<Konva.Stage>(null);
  const cursorRef = useRef<Konva.Circle>(null);

  const containerRef = useRef<HTMLDivElement>(null);
  const [containerSize, setContainerSize] = useState({ w: 0, h: 0 });

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) return;
      setContainerSize({ w: entry.contentRect.width, h: entry.contentRect.height });
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const fitScale = useMemo(() => {
    if (containerSize.w <= 0 || containerSize.h <= 0) return 1;
    return Math.min(containerSize.w / view.width, containerSize.h / view.height);
  }, [containerSize, view.width, view.height]);

  const displayScale = Math.max(0.02, fitScale * (zoomPct / 100));
  const stageW = Math.max(1, Math.round(view.width * displayScale));
  const stageH = Math.max(1, Math.round(view.height * displayScale));

  // Keep the offscreen raw/display canvases in sync with the mask list:
  // create blank ones for new masks, drop ones for deleted masks, import
  // bitmaps for masks that have one but haven't been loaded locally yet,
  // and recolor whenever the assigned asset's color changes.
  useEffect(() => {
    const store = canvasStore.current;
    let structuralChange = false;

    for (const id of Array.from(store.keys())) {
      if (!view.masks.some((m) => m.id === id)) {
        store.delete(id);
        structuralChange = true;
      }
    }

    view.masks.forEach((mask) => {
      let entry = store.get(mask.id);
      if (!entry) {
        entry = {
          raw: createCanvas(view.width, view.height),
          display: createCanvas(view.width, view.height),
          lastColor: null,
          bitmapState: "none",
        };
        store.set(mask.id, entry);
        structuralChange = true;
      }

      const color = getMaskColor(mask, assets);

      if (mask.hasBitmap && entry.bitmapState === "none") {
        entry.bitmapState = "loading";
        const img = new window.Image();
        img.crossOrigin = "anonymous";
        img.onload = () => {
          const e = store.get(mask.id);
          if (!e) return;
          importBitmapToRaw(e.raw, img);
          recolorAll(e.display, e.raw, getMaskColor(mask, assets));
          e.lastColor = color;
          e.bitmapState = "loaded";
          layerRef.current?.batchDraw();
          forceTick();
        };
        img.onerror = () => {
          const e = store.get(mask.id);
          if (e) e.bitmapState = "none";
          setEditorError(t("maskEditor.errors.bitmapLoad"));
        };
        // Resolve a signed URL for the mask's bitmap blob, then load it. crossOrigin
        // is already set so the loaded image can be read back into a canvas.
        if (project) {
          void fetchSignedImageUrl(project.id, mask.id)
            .then((url) => {
              img.src = url;
            })
            .catch(() => {
              const e = store.get(mask.id);
              if (e) e.bitmapState = "none";
              setEditorError(t("maskEditor.errors.bitmapLoad"));
            });
        }
      } else if (entry.lastColor !== color) {
        recolorAll(entry.display, entry.raw, color);
        entry.lastColor = color;
        layerRef.current?.batchDraw();
      }
    });

    if (structuralChange) forceTick();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view.masks, assets]);

  const paintingRef = useRef<{ maskId: string; last: Point } | null>(null);

  function relPos(stage: Konva.Stage): Point | null {
    return stage.getRelativePointerPosition();
  }

  function paintAt(maskId: string, from: Point, to: Point) {
    const entry = canvasStore.current.get(maskId);
    const mask = view.masks.find((m) => m.id === maskId);
    if (!entry || !mask) return;
    paintSegment(entry.raw, from, to, brushSize, tool);
    const rect = strokeDirtyRect(from, to, brushSize, view.width, view.height);
    recolorRegion(entry.display, entry.raw, getMaskColor(mask, assets), rect);
    layerRef.current?.batchDraw();
    setSaveStatus((prev) => (prev[maskId] === "saving" ? prev : { ...prev, [maskId]: "idle" }));
  }

  function handlePointerDown(e: Konva.KonvaEventObject<MouseEvent | TouchEvent>) {
    if (!selectedMaskId) return;
    const stage = e.target.getStage();
    if (!stage) return;
    const pos = relPos(stage);
    if (!pos) return;
    paintingRef.current = { maskId: selectedMaskId, last: pos };
    paintAt(selectedMaskId, pos, { x: pos.x + 0.01, y: pos.y + 0.01 });
  }

  function handlePointerMove(e: Konva.KonvaEventObject<MouseEvent | TouchEvent>) {
    const stage = e.target.getStage();
    if (!stage) return;
    const pos = relPos(stage);
    if (pos && cursorRef.current) {
      cursorRef.current.position(pos);
      cursorRef.current.visible(!!selectedMaskId);
      cursorRef.current.getLayer()?.batchDraw();
    }
    const painting = paintingRef.current;
    if (!painting || !pos) return;
    paintAt(painting.maskId, painting.last, pos);
    painting.last = pos;
  }

  function handlePointerLeave() {
    cursorRef.current?.visible(false);
    cursorRef.current?.getLayer()?.batchDraw();
    handlePointerUp();
  }

  function handlePointerUp() {
    const painting = paintingRef.current;
    paintingRef.current = null;
    if (painting) void saveMask(painting.maskId);
  }

  async function saveMask(maskId: string) {
    const entry = canvasStore.current.get(maskId);
    if (!entry) return;
    setSaveStatus((prev) => ({ ...prev, [maskId]: "saving" }));
    try {
      const blob = await exportRawToPng(entry.raw);
      await uploadMaskBitmap(view.id, maskId, blob);
      setSaveStatus((prev) => ({ ...prev, [maskId]: "saved" }));
    } catch (err) {
      setSaveStatus((prev) => ({ ...prev, [maskId]: "error" }));
      setEditorError(err instanceof Error ? err.message : t("maskEditor.errors.saveMask"));
    }
  }

  async function handleAddMask() {
    setAddingMask(true);
    setEditorError(null);
    try {
      const mask = await createMask(view.id);
      setSelectedMaskId(mask.id);
    } catch (err) {
      setEditorError(err instanceof Error ? err.message : t("maskEditor.errors.addMask"));
    } finally {
      setAddingMask(false);
    }
  }

  async function handleToggleHidden(mask: Mask) {
    try {
      await updateMask(view.id, mask.id, { hidden: !mask.hidden });
    } catch (err) {
      setEditorError(err instanceof Error ? err.message : t("maskEditor.errors.updateMask"));
    }
  }

  async function handleAssignAsset(mask: Mask, assetId: string | null) {
    try {
      await updateMask(view.id, mask.id, { assetId });
    } catch (err) {
      setEditorError(err instanceof Error ? err.message : t("maskEditor.errors.assignAsset"));
    }
  }

  async function confirmDeleteMask() {
    if (!pendingDeleteMask) return;
    try {
      await deleteMask(view.id, pendingDeleteMask.id);
      canvasStore.current.delete(pendingDeleteMask.id);
      setPendingDeleteMask(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : t("maskEditor.errors.deleteMask");
      setEditorError(message);
      throw new Error(message);
    }
  }

  const savingCount = Object.values(saveStatus).filter((s) => s === "saving").length;
  const selectedMask = view.masks.find((m) => m.id === selectedMaskId) ?? null;
  const cursorColor = selectedMask ? getMaskColor(selectedMask, assets) : "#ffffff";

  return (
    <section className="panel mask-editor">
      <div className="panel-header">
        <div>
          <div className="panel-title">{t("maskEditor.title")}</div>
          <div className="panel-subtitle">
            {t("maskEditor.subtitle", { width: view.width, height: view.height })}
          </div>
        </div>
      </div>

      <MaskToolbar
        tool={tool}
        onToolChange={setTool}
        brushSize={brushSize}
        onBrushSizeChange={setBrushSize}
        minBrush={minBrush}
        maxBrush={maxBrush}
        zoomPct={zoomPct}
        onZoomChange={setZoomPct}
        onAddMask={handleAddMask}
        addingMask={addingMask}
        toolsDisabled={!selectedMaskId}
        savingCount={savingCount}
      />

      {editorError && (
        <div className="error-banner mask-editor-error">
          <span>{editorError}</span>
          <button className="btn btn-ghost btn-sm" onClick={() => setEditorError(null)}>
            {t("common.dismiss")}
          </button>
        </div>
      )}

      <div className="mask-editor-body">
        <div className="mask-editor-canvas-col">
          <div className="mask-canvas-viewport" ref={containerRef}>
            {screenshotLoading && (
              <div className="mask-canvas-status">
                <span className="spinner" /> {t("maskEditor.loadingScreenshot")}
              </div>
            )}
            {screenshotError && (
              <div className="mask-canvas-status mask-canvas-status-error">
                {t("maskEditor.loadError")}
              </div>
            )}
            {screenshotImg && (
              <div
                className="mask-canvas-scroll"
                style={{ width: containerSize.w, height: containerSize.h }}
              >
                <Stage
                  ref={stageRef}
                  width={stageW}
                  height={stageH}
                  scaleX={displayScale}
                  scaleY={displayScale}
                  className={`mask-stage tool-${tool} ${selectedMaskId ? "" : "tool-disabled"}`}
                  onMouseDown={handlePointerDown}
                  onMouseMove={handlePointerMove}
                  onMouseUp={handlePointerUp}
                  onMouseLeave={handlePointerLeave}
                  onTouchStart={handlePointerDown}
                  onTouchMove={handlePointerMove}
                  onTouchEnd={handlePointerUp}
                >
                  <Layer listening={false}>
                    <KonvaImage image={screenshotImg} width={view.width} height={view.height} />
                  </Layer>
                  <Layer ref={layerRef} listening={false}>
                    {view.masks
                      .filter((m) => !m.hidden)
                      .map((m) => {
                        const entry = canvasStore.current.get(m.id);
                        if (!entry) return null;
                        return (
                          <KonvaImage
                            key={m.id}
                            image={entry.display}
                            width={view.width}
                            height={view.height}
                          />
                        );
                      })}
                  </Layer>
                  <Layer listening={false}>
                    <Circle
                      ref={cursorRef}
                      radius={brushSize / 2}
                      visible={false}
                      stroke={tool === "eraser" ? "#ffffff" : cursorColor}
                      strokeWidth={1.5 / displayScale}
                      dash={tool === "eraser" ? [4 / displayScale, 3 / displayScale] : undefined}
                      fill={tool === "brush" ? cursorColor : undefined}
                      opacity={tool === "brush" ? 0.18 : 1}
                    />
                  </Layer>
                </Stage>
              </div>
            )}
          </div>
          {!selectedMaskId && view.masks.length > 0 && (
            <p className="field-hint mask-editor-hint">{t("maskEditor.selectHint")}</p>
          )}
          {view.masks.length === 0 && (
            <p className="field-hint mask-editor-hint">{t("maskEditor.addHint")}</p>
          )}
        </div>

        <div className="mask-editor-list-col">
          <MasksList
            masks={view.masks}
            assets={assets}
            selectedMaskId={selectedMaskId}
            onSelect={setSelectedMaskId}
            onToggleHidden={handleToggleHidden}
            onDelete={setPendingDeleteMask}
            onAssignAsset={handleAssignAsset}
            saveStatus={saveStatus}
          />
        </div>
      </div>

      {pendingDeleteMask && (
        <ConfirmDialog
          title={t("masksList.deleteMask")}
          body={t("maskEditor.deleteConfirm")}
          confirmLabel={t("common.delete")}
          onConfirm={confirmDeleteMask}
          onCancel={() => setPendingDeleteMask(null)}
        />
      )}
    </section>
  );
}
