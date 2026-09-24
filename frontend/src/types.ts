// Types mirror API_CONTRACT.md exactly. Keep in sync with the backend contract.

export type ScenePreset = "interior" | "exterior";

export type LightingPreset =
  | "morning_sun"
  | "overcast"
  | "golden_hour"
  | "evening_interior_lights"
  | "night_exterior";

// Artificial lights: switched off, or on at a color temperature. Empty on
// projects created before the setting existed (the prompt then omits it).
export type InteriorLights = "off" | "3000k" | "4000k" | "6000k" | "";

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
  // promptTokens/outputTokens/thoughtsTokens are summed across every model
  // call this render made: the image call, plus the preservation check if it
  // ran. outputTokens is TEXT output only - it excludes the image call's own
  // generated-image tokens, which are priced per-image instead of per-token
  // (see PricingResponse). thoughtsTokens is the thinking-token portion,
  // already folded into whatever estimatedCostUsd charges at the output rate.
  promptTokens?: number;
  outputTokens?: number;
  thoughtsTokens?: number;
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
  interiorLights: InteriorLights;
  materialNotes: string;
  extraInstructions: string;
}

export interface Project {
  id: string;
  ownerId?: string;
  name: string;
  createdAt?: string;
  updatedAt?: string;
  style: StyleSettings;
  assets: Asset[];
  views: View[];
  styleAnchorRenderId: string | null;
}

// Lightweight projection returned by GET /api/projects for the picker.
export interface ProjectSummary {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
  viewCount: number;
  renderCount: number;
  thumbnailImageId?: string;
}

export interface ApiErrorBody {
  error: string;
}

// One model+resolution combination's estimated cost of a single render, from
// GET /api/pricing.
export interface PricingEstimate {
  model: ModelChoice;
  resolution: Resolution;
  costUsd: number;
  costBrl: number;
}

export interface PricingResponse {
  usdToBrl: number;
  estimates: PricingEstimate[];
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

export const INTERIOR_LIGHTS_OPTIONS: { value: Exclude<InteriorLights, "">; label: string; hint: string }[] = [
  { value: "off", label: "Off", hint: "All artificial lights switched off" },
  { value: "3000k", label: "3000K", hint: "Lights on, warm white" },
  { value: "4000k", label: "4000K", hint: "Lights on, neutral white" },
  { value: "6000k", label: "6000K", hint: "Lights on, cool daylight white" },
];
