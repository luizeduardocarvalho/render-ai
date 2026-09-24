// Shared constants and pure helpers for the edit-region editor. The limits
// mirror the backend's (internal/api/edit.go) - the server is what enforces
// them, these only keep the UI from offering what it would reject.

export const MAX_EDIT_REGIONS = 8;
export const MAX_INSTRUCTION_LENGTH = 500;

// The longest side of the canvases regions are painted on. A 4K render would
// otherwise need two full-size canvases per region; the server scales the
// bitmap back up and feathers its edge, so the reduced size costs nothing
// visible.
export const MAX_EDIT_CANVAS_SIDE = 1600;

// Region overlay colors, far apart in hue so regions stay tellable on any
// render. Same order and colors the backend paints the overlay it sends the
// model in.
export const EDIT_REGION_COLORS = [
  "#e61919",
  "#1e5af0",
  "#1ebe3c",
  "#fadc14",
  "#e61ec8",
  "#14d2e6",
  "#fa8214",
  "#8232d2",
] as const;

export interface EditRegion {
  id: string;
  color: string;
  instruction: string;
  /** True once a brush stroke has landed; erasing everything does not reset it. */
  painted: boolean;
}

/** The first palette color no region uses; falls back to the first. */
export function nextRegionColor(regions: EditRegion[]): string {
  return EDIT_REGION_COLORS.find((c) => !regions.some((r) => r.color === c)) ?? EDIT_REGION_COLORS[0];
}

/** Canvas size for painting on an image of the given natural size. */
export function editCanvasSize(width: number, height: number): { w: number; h: number } {
  const scale = Math.min(1, MAX_EDIT_CANVAS_SIDE / Math.max(width, height));
  return { w: Math.max(1, Math.round(width * scale)), h: Math.max(1, Math.round(height * scale)) };
}

export function isRegionComplete(region: EditRegion): boolean {
  return region.painted && region.instruction.trim().length > 0;
}

/** True if the canvas has no painted (non-transparent) pixel at all. */
export function isCanvasEmpty(canvas: HTMLCanvasElement): boolean {
  const ctx = canvas.getContext("2d");
  if (!ctx) return true;
  const { data } = ctx.getImageData(0, 0, canvas.width, canvas.height);
  for (let i = 3; i < data.length; i += 4) {
    if (data[i] > 0) return false;
  }
  return true;
}

/** Base64 of a blob's bytes, without a data: URL prefix. */
export function blobToBase64(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = String(reader.result);
      resolve(result.slice(result.indexOf(",") + 1));
    };
    reader.onerror = () => reject(reader.error ?? new Error("Could not read the mask"));
    reader.readAsDataURL(blob);
  });
}
