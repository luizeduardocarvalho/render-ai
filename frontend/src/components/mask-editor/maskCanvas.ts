// Pure canvas helpers for the mask editor. Everything here operates at the
// screenshot's ORIGINAL resolution (view.width x view.height) - the Konva
// stage only ever scales the *display* of these canvases, never their
// contents, so exported bitmaps are always full resolution regardless of
// zoom level.

export interface Point {
  x: number;
  y: number;
}

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export const UNASSIGNED_COLOR = "#8a8f98";
export const OVERLAY_ALPHA = 0.45;

export function getMaskColor(
  mask: { assetId: string | null },
  assets: { id: string; color: string }[],
): string {
  return assets.find((a) => a.id === mask.assetId)?.color ?? UNASSIGNED_COLOR;
}

export function createCanvas(width: number, height: number): HTMLCanvasElement {
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(width));
  canvas.height = Math.max(1, Math.round(height));
  return canvas;
}

function ctx2d(canvas: HTMLCanvasElement): CanvasRenderingContext2D {
  const c = canvas.getContext("2d");
  if (!c) throw new Error("2d canvas context unavailable");
  return c;
}

/** Union of the two points' bounding box, padded by half the brush size, clamped to canvas bounds. */
export function strokeDirtyRect(
  a: Point,
  b: Point,
  brushSize: number,
  canvasW: number,
  canvasH: number,
): Rect {
  const pad = brushSize / 2 + 2;
  const minX = Math.min(a.x, b.x) - pad;
  const minY = Math.min(a.y, b.y) - pad;
  const maxX = Math.max(a.x, b.x) + pad;
  const maxY = Math.max(a.y, b.y) + pad;
  const x = Math.max(0, Math.floor(minX));
  const y = Math.max(0, Math.floor(minY));
  const ex = Math.min(canvasW, Math.ceil(maxX));
  const ey = Math.min(canvasH, Math.ceil(maxY));
  return { x, y, w: Math.max(0, ex - x), h: Math.max(0, ey - y) };
}

/**
 * Paints (or erases) a stroke segment on the raw mask canvas. The raw canvas
 * represents "painted" as opaque white on a transparent background - this
 * makes both export (compose over black) and colored preview (composite
 * with an asset color) trivial and cheap.
 */
export function paintSegment(
  raw: HTMLCanvasElement,
  from: Point,
  to: Point,
  brushSize: number,
  tool: "brush" | "eraser",
): void {
  const ctx = ctx2d(raw);
  ctx.save();
  ctx.lineJoin = "round";
  ctx.lineCap = "round";
  ctx.lineWidth = brushSize;
  ctx.globalCompositeOperation = tool === "brush" ? "source-over" : "destination-out";
  ctx.strokeStyle = "rgba(255,255,255,1)";
  ctx.beginPath();
  ctx.moveTo(from.x, from.y);
  ctx.lineTo(to.x, to.y);
  ctx.stroke();
  ctx.restore();
}

/**
 * Recomputes the tinted display canvas (what Konva renders) for a region of
 * the raw canvas: solid asset color, clipped to wherever the raw canvas has
 * paint (via destination-in, which composites using the source's alpha).
 */
export function recolorRegion(
  display: HTMLCanvasElement,
  raw: HTMLCanvasElement,
  colorHex: string,
  rect: Rect,
): void {
  if (rect.w <= 0 || rect.h <= 0) return;
  const ctx = ctx2d(display);
  ctx.clearRect(rect.x, rect.y, rect.w, rect.h);
  ctx.save();
  ctx.beginPath();
  ctx.rect(rect.x, rect.y, rect.w, rect.h);
  ctx.clip();
  ctx.fillStyle = colorHex;
  ctx.globalAlpha = OVERLAY_ALPHA;
  ctx.fillRect(rect.x, rect.y, rect.w, rect.h);
  ctx.globalAlpha = 1;
  ctx.globalCompositeOperation = "destination-in";
  ctx.drawImage(raw, rect.x, rect.y, rect.w, rect.h, rect.x, rect.y, rect.w, rect.h);
  ctx.restore();
}

export function recolorAll(display: HTMLCanvasElement, raw: HTMLCanvasElement, colorHex: string): void {
  recolorRegion(display, raw, colorHex, { x: 0, y: 0, w: display.width, h: display.height });
}

/** Loads an already-fetched white-on-black mask bitmap into the raw (alpha) canvas. */
export function importBitmapToRaw(raw: HTMLCanvasElement, source: HTMLImageElement): void {
  const w = raw.width;
  const h = raw.height;
  const tmp = createCanvas(w, h);
  const tctx = ctx2d(tmp);
  tctx.drawImage(source, 0, 0, w, h);
  const imgData = tctx.getImageData(0, 0, w, h);
  const px = imgData.data;
  for (let i = 0; i < px.length; i += 4) {
    const luminance = (px[i] + px[i + 1] + px[i + 2]) / 3;
    if (luminance > 127) {
      px[i] = 255;
      px[i + 1] = 255;
      px[i + 2] = 255;
      px[i + 3] = 255;
    } else {
      px[i + 3] = 0;
    }
  }
  const rctx = ctx2d(raw);
  rctx.clearRect(0, 0, w, h);
  rctx.putImageData(imgData, 0, 0);
}

/** Composes the raw (alpha) canvas over black to produce the contract's white-on-black export PNG. */
export function exportRawToPng(raw: HTMLCanvasElement): Promise<Blob> {
  const out = createCanvas(raw.width, raw.height);
  const ctx = ctx2d(out);
  ctx.fillStyle = "#000000";
  ctx.fillRect(0, 0, out.width, out.height);
  ctx.drawImage(raw, 0, 0);
  return new Promise((resolve, reject) => {
    out.toBlob((blob) => {
      if (blob) resolve(blob);
      else reject(new Error("Failed to export mask bitmap"));
    }, "image/png");
  });
}
