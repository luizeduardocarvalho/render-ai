// Types mirror API_CONTRACT.md exactly. Keep in sync with the backend contract.

export type ScenePreset = "interior" | "exterior";

export type LightingPreset =
  | "morning_sun"
  | "overcast"
  | "golden_hour"
  | "evening_interior_lights"
  | "night_exterior";

export type ModelChoice = "pro" | "flash"; // pro = gemini-3-pro-image, flash = gemini-3.1-flash-image

export type Resolution = "1K" | "2K" | "4K"; // flash supports 1K only

export interface Asset {
  id: string;
  name: string;
  description: string;
  color: string; // hex "#RRGGBB"
  referenceImageId: string;
  hasReferenceImage: boolean;
}

export interface Mask {
  id: string;
  assetId: string | null;
  hasBitmap: boolean;
  hidden: boolean;
}

export interface RenderMetrics {
  model: string;
  resolution: Resolution;
  regionCount: number;
  anchorUsed: boolean;
  imageCallMs: number;
  totalMs: number;
  promptTokens?: number;
  outputTokens?: number;
  estimatedCostUsd?: number;
}

export interface PreservationInventory {
  removed: string[];
  added: string[];
  moved: string[];
  raw?: string;
}

export interface PreservationReport {
  edgeScore: number;
  edgeFlag: boolean;
  inventory: PreservationInventory;
}

export interface Render {
  id: string;
  createdAt: string;
  model: ModelChoice;
  resolution: Resolution;
  anchorUsed: boolean;
  regionCount: number;
  resultImageId: string;
  metrics: RenderMetrics;
  preservation?: PreservationReport;
  isStyleAnchor: boolean;
}

export interface View {
  id: string;
  name: string;
  screenshotImageId: string;
  hasScreenshot: boolean;
  width: number;
  height: number;
  masks: Mask[];
  inventory: string;
  renders: Render[];
}

export interface StyleSettings {
  scene: ScenePreset;
  lighting: LightingPreset;
  lightDirection: string;
  materialNotes: string;
  extraInstructions: string;
}

export interface Project {
  id: string;
  name: string;
  style: StyleSettings;
  assets: Asset[];
  views: View[];
  styleAnchorRenderId: string | null;
}

export interface ApiErrorBody {
  error: string;
}

export interface RenderRequest {
  model: ModelChoice;
  resolution: Resolution;
  preservationCheck: boolean;
  variations?: number; // 1-4, defaults to 1 server-side
}

export const LIGHTING_PRESETS: { value: LightingPreset; label: string }[] = [
  { value: "morning_sun", label: "Morning sun" },
  { value: "overcast", label: "Overcast" },
  { value: "golden_hour", label: "Golden hour" },
  { value: "evening_interior_lights", label: "Evening interior lights" },
  { value: "night_exterior", label: "Night exterior" },
];
