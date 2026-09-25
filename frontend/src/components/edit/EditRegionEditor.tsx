import Konva from "konva";
import { useEffect, useMemo, useRef, useState } from "react";
import { Circle, Image as KonvaImage, Layer, Stage } from "react-konva";
import { useTranslation } from "react-i18next";
import { ApiError, INSUFFICIENT_CREDITS_CODE, getPricing } from "../../api";
import { useImage } from "../../hooks/useImage";
import { useSignedImageUrl } from "../../hooks/useSignedImageUrl";
import { formatCredits } from "../../lib/credits";
import { useProject } from "../../state/ProjectContext";
import type { EditRegionRequest, PricingResponse, Render, RenderJob, View } from "../../types";
import {
  createCanvas,
  exportRawToPng,
  paintSegment,
  recolorRegion,
  strokeDirtyRect,
  type Point,
} from "../mask-editor/maskCanvas";
import type { Tool } from "../mask-editor/MaskEditor";
import "../mask-editor/MaskEditor.css";
import { MaskToolbar } from "../mask-editor/MaskToolbar";
import "./EditRegionEditor.css";
import { EditRegionsList } from "./EditRegionsList";
import {
  MAX_EDIT_REGIONS,
  blobToBase64,
  editCanvasSize,
  isCanvasEmpty,
  isRegionComplete,
  nextRegionColor,
  type EditRegion,
} from "./editRegions";

interface RegionCanvas {
  /** Painted area: opaque white on transparent, exported as the region's bitmap. */
  raw: HTMLCanvasElement;
  /** Tinted copy of raw in the region's color, what the stage shows. */
  display: HTMLCanvasElement;
}

interface EditRegionEditorProps {
  view: View;
  /** The render being edited. */
  render: Render;
  onCancel: () => void;
  /** Called with the new render once the edit has finished. */
  onDone: (edit: Render) => void;
}

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n));
}

export function EditRegionEditor({ view, render, onCancel, onDone }: EditRegionEditorProps) {
  const { t, i18n } = useTranslation();
  const { editRender, me, refreshMe, adjustDisplayedCredits } = useProject();

  const imageUrl = useSignedImageUrl(render.resultImageId);
  const { image, loading: imageLoading, error: imageError } = useImage(imageUrl ?? null);
  const size = useMemo(
    () => (image ? editCanvasSize(image.naturalWidth, image.naturalHeight) : null),
    [image],
  );

  const [regions, setRegions] = useState<EditRegion[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [tool, setTool] = useState<Tool>("brush");
  const [brushSize, setBrushSize] = useState(16);
  const [zoomPct, setZoomPct] = useState(100);
  const [submitting, setSubmitting] = useState(false);
  const [progressJob, setProgressJob] = useState<RenderJob | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pricing, setPricing] = useState<PricingResponse | null>(null);

  const canvases = useRef(new Map<string, RegionCanvas>());
  const paintedIds = useRef(new Set<string>());
  const layerRef = useRef<Konva.Layer>(null);
  const cursorRef = useRef<Konva.Circle>(null);
  const paintingRef = useRef<{ id: string; last: Point } | null>(null);
  // An edit that is running keeps running when the editor goes away (the user
  // picked another render): the job is already paid for, and the provider adds
  // its result to the view when it lands. Only what would jump the user to it
  // is skipped.
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    getPricing()
      .then((p) => {
        if (!cancelled) setPricing(p);
      })
      .catch(() => {
        // The price is a nice-to-have; the button just shows no amount.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Size the brush to the canvas once the image is known.
  const minBrush = size ? Math.max(2, Math.round(size.w / 800)) : 2;
  const maxBrush = size ? Math.max(minBrush + 20, Math.round(size.w / 8)) : 60;

  // Stage geometry: fit the canvas into the viewport, then apply the zoom.
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerSize, setContainerSize] = useState({ w: 0, h: 0 });
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (entry) setContainerSize({ w: entry.contentRect.width, h: entry.contentRect.height });
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const fitScale =
    size && containerSize.w > 0 && containerSize.h > 0
      ? Math.min(containerSize.w / size.w, containerSize.h / size.h)
      : 1;
  const displayScale = Math.max(0.02, fitScale * (zoomPct / 100));
  const stageW = size ? Math.max(1, Math.floor(size.w * displayScale)) : 1;
  const stageH = size ? Math.max(1, Math.floor(size.h * displayScale)) : 1;

  const selected = regions.find((r) => r.id === selectedId) ?? null;

  function addRegion() {
    if (!size || regions.length >= MAX_EDIT_REGIONS) return;
    const id = crypto.randomUUID();
    canvases.current.set(id, {
      raw: createCanvas(size.w, size.h),
      display: createCanvas(size.w, size.h),
    });
    setRegions((prev) => [...prev, { id, color: nextRegionColor(prev), instruction: "", painted: false }]);
    setSelectedId(id);
  }

  // Once the image is known, size the brush to it and start with one region
  // so the user can paint straight away.
  const seededRef = useRef(false);
  useEffect(() => {
    if (!size || seededRef.current) return;
    seededRef.current = true;
    setBrushSize(clamp(Math.round(size.w / 80), 8, 60));
    addRegion();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [size]);

  function deleteRegion(id: string) {
    canvases.current.delete(id);
    paintedIds.current.delete(id);
    const remaining = regions.filter((r) => r.id !== id);
    setRegions(remaining);
    if (selectedId === id) setSelectedId(remaining.at(-1)?.id ?? null);
    layerRef.current?.batchDraw();
  }

  function setInstruction(id: string, instruction: string) {
    setRegions((prev) => prev.map((r) => (r.id === id ? { ...r, instruction } : r)));
  }

  function paintAt(from: Point, to: Point) {
    const painting = paintingRef.current;
    if (!painting || !size) return;
    const entry = canvases.current.get(painting.id);
    const region = regions.find((r) => r.id === painting.id);
    if (!entry || !region) return;
    paintSegment(entry.raw, from, to, brushSize, tool);
    recolorRegion(entry.display, entry.raw, region.color, strokeDirtyRect(from, to, brushSize, size.w, size.h));
    layerRef.current?.batchDraw();
    if (tool === "brush" && !paintedIds.current.has(region.id)) {
      paintedIds.current.add(region.id);
      setRegions((prev) => prev.map((r) => (r.id === region.id ? { ...r, painted: true } : r)));
    }
  }

  function handlePointerDown(e: Konva.KonvaEventObject<MouseEvent | TouchEvent>) {
    if (!selectedId || submitting) return;
    const pos = e.target.getStage()?.getRelativePointerPosition();
    if (!pos) return;
    paintingRef.current = { id: selectedId, last: pos };
    paintAt(pos, { x: pos.x + 0.01, y: pos.y + 0.01 });
  }

  function handlePointerMove(e: Konva.KonvaEventObject<MouseEvent | TouchEvent>) {
    const pos = e.target.getStage()?.getRelativePointerPosition();
    if (pos && cursorRef.current) {
      cursorRef.current.position(pos);
      cursorRef.current.visible(!!selectedId && !submitting);
      cursorRef.current.getLayer()?.batchDraw();
    }
    const painting = paintingRef.current;
    if (!painting || !pos) return;
    paintAt(painting.last, pos);
    painting.last = pos;
  }

  function handlePointerUp() {
    paintingRef.current = null;
  }

  function handlePointerLeave() {
    cursorRef.current?.visible(false);
    cursorRef.current?.getLayer()?.batchDraw();
    handlePointerUp();
  }

  const estimate = pricing?.estimates.find((e) => e.model === render.model && e.resolution === render.resolution);
  const credits = estimate?.credits;
  const devAuthDisabled = me?.userId === "";
  const lowBalance = !devAuthDisabled && me !== null && credits !== undefined && me.credits < credits;
  const ready = regions.length > 0 && regions.every(isRegionComplete);

  async function handleApply() {
    if (!ready || submitting) return;
    setSubmitting(true);
    setError(null);
    setProgressJob(null);

    // Show the debit right away and give it back if the job fails, the way
    // RenderControls does; refreshMe() in finally settles on the real balance.
    const charge = devAuthDisabled ? 0 : (credits ?? 0);
    if (charge > 0) adjustDisplayedCredits(-charge);
    let refunded = false;
    const handleProgress = (job: RenderJob) => {
      setProgressJob(job);
      if (charge > 0 && !refunded && job.variations.some((v) => v.status === "failed")) {
        refunded = true;
        adjustDisplayedCredits(charge);
      }
    };

    try {
      const payload: EditRegionRequest[] = [];
      for (const [i, region] of regions.entries()) {
        const entry = canvases.current.get(region.id);
        if (!entry || isCanvasEmpty(entry.raw)) {
          throw new Error(t("editRegions.errors.emptyRegion", { index: i + 1 }));
        }
        payload.push({
          instruction: region.instruction.trim(),
          bitmap: await blobToBase64(await exportRawToPng(entry.raw)),
        });
      }
      const [edit] = await editRender(view.id, render.id, payload, { onProgress: handleProgress });
      if (edit && mountedRef.current) onDone(edit);
    } catch (err) {
      if (err instanceof ApiError && err.code === INSUFFICIENT_CREDITS_CODE) {
        setError(t("editRegions.errors.insufficientCredits"));
      } else {
        setError(err instanceof Error ? err.message : t("editRegions.errors.failed"));
      }
    } finally {
      setSubmitting(false);
      setProgressJob(null);
      void refreshMe();
      if (refunded) window.setTimeout(() => void refreshMe(), 2000);
    }
  }

  const applyLabel = submitting
    ? progressJob?.status === "running"
      ? t("editRegions.applying")
      : t("editRegions.queued")
    : credits !== undefined
      ? t("editRegions.applyButton", { count: credits, credits: formatCredits(credits, i18n.language) })
      : t("editRegions.applyButtonNoPrice");

  return (
    <section className="panel mask-editor edit-region-editor">
      <div className="panel-header">
        <div>
          <div className="panel-title">{t("editRegions.title")}</div>
          <div className="panel-subtitle">{t("editRegions.subtitle")}</div>
        </div>
        <button type="button" className="btn btn-sm" onClick={onCancel} disabled={submitting}>
          {t("common.cancel")}
        </button>
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
        onAddMask={addRegion}
        addingMask={false}
        addDisabled={!size || regions.length >= MAX_EDIT_REGIONS || submitting}
        addLabel={t("editRegions.addRegion")}
        toolsDisabled={!selected || submitting}
      />

      <div className="mask-editor-body">
        <div className="mask-editor-canvas-col">
          <div className="mask-canvas-viewport" ref={containerRef}>
            {(imageLoading || (!image && !imageError)) && (
              <div className="mask-canvas-status">
                <span className="spinner" /> {t("editRegions.loadingImage")}
              </div>
            )}
            {imageError && (
              <div className="mask-canvas-status mask-canvas-status-error">{t("editRegions.loadError")}</div>
            )}
            {image && size && (
              <div className="mask-canvas-scroll">
                <Stage
                  width={stageW}
                  height={stageH}
                  scaleX={displayScale}
                  scaleY={displayScale}
                  className={`mask-stage tool-${tool} ${selected && !submitting ? "" : "tool-disabled"}`}
                  onMouseDown={handlePointerDown}
                  onMouseMove={handlePointerMove}
                  onMouseUp={handlePointerUp}
                  onMouseLeave={handlePointerLeave}
                  onTouchStart={handlePointerDown}
                  onTouchMove={handlePointerMove}
                  onTouchEnd={handlePointerUp}
                >
                  <Layer listening={false}>
                    <KonvaImage image={image} width={size.w} height={size.h} />
                  </Layer>
                  <Layer ref={layerRef} listening={false}>
                    {regions.map((r) => {
                      const entry = canvases.current.get(r.id);
                      return entry ? (
                        <KonvaImage key={r.id} image={entry.display} width={size.w} height={size.h} />
                      ) : null;
                    })}
                  </Layer>
                  <Layer listening={false}>
                    <Circle
                      ref={cursorRef}
                      radius={brushSize / 2}
                      visible={false}
                      stroke={tool === "eraser" ? "#ffffff" : (selected?.color ?? "#ffffff")}
                      strokeWidth={1.5 / displayScale}
                      dash={tool === "eraser" ? [4 / displayScale, 3 / displayScale] : undefined}
                      fill={tool === "brush" ? (selected?.color ?? "#ffffff") : undefined}
                      opacity={tool === "brush" ? 0.18 : 1}
                    />
                  </Layer>
                </Stage>
              </div>
            )}
          </div>
          <p className="field-hint mask-editor-hint">{t("editRegions.paintHint")}</p>
        </div>

        <div className="mask-editor-list-col">
          <EditRegionsList
            regions={regions}
            selectedId={selectedId}
            disabled={submitting}
            onSelect={setSelectedId}
            onInstructionChange={setInstruction}
            onDelete={deleteRegion}
          />
        </div>
      </div>

      <div className="edit-footer">
        {error && <div className="error-banner">{error}</div>}
        {!submitting && lowBalance && me && credits !== undefined && (
          <div className="error-banner">
            {t("editRegions.lowBalance", {
              available: formatCredits(me.credits, i18n.language),
              required: formatCredits(credits, i18n.language),
            })}
          </div>
        )}
        <button
          type="button"
          className="btn btn-primary btn-block"
          onClick={handleApply}
          disabled={!ready || submitting || lowBalance}
        >
          {submitting ? <span className="spinner" /> : null}
          {applyLabel}
        </button>
        {!submitting && !ready && regions.length > 0 && (
          <div className="field-hint">{t("editRegions.notReady")}</div>
        )}
        {submitting && <div className="field-hint">{t("editRegions.wait")}</div>}
      </div>
    </section>
  );
}
