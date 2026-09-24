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

// Labels/hints for these constants live in the translation files (see
// src/i18n/locales/*.json under style.lightingPresets / style.interiorLights)
// rather than as literal strings here - `value` is the token the API expects
// (unchanged), `labelKey`/`hintKey` are i18next keys resolved with t() at
// render time.

export const LIGHTING_PRESETS: { value: LightingPreset; labelKey: `style.lightingPresets.${LightingPreset}` }[] = [
  { value: "morning_sun", labelKey: "style.lightingPresets.morning_sun" },
  { value: "overcast", labelKey: "style.lightingPresets.overcast" },
  { value: "golden_hour", labelKey: "style.lightingPresets.golden_hour" },
  { value: "evening_interior_lights", labelKey: "style.lightingPresets.evening_interior_lights" },
  { value: "night_exterior", labelKey: "style.lightingPresets.night_exterior" },
];

type InteriorLightsValue = Exclude<InteriorLights, "">;

export const INTERIOR_LIGHTS_OPTIONS: {
  value: InteriorLightsValue;
  labelKey: `style.interiorLights.options.${InteriorLightsValue}.label`;
  hintKey: `style.interiorLights.options.${InteriorLightsValue}.hint`;
}[] = [
  { value: "off", labelKey: "style.interiorLights.options.off.label", hintKey: "style.interiorLights.options.off.hint" },
  { value: "3000k", labelKey: "style.interiorLights.options.3000k.label", hintKey: "style.interiorLights.options.3000k.hint" },
  { value: "4000k", labelKey: "style.interiorLights.options.4000k.label", hintKey: "style.interiorLights.options.4000k.hint" },
  { value: "6000k", labelKey: "style.interiorLights.options.6000k.label", hintKey: "style.interiorLights.options.6000k.hint" },
];
