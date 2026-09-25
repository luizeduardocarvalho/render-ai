// Types mirror API_CONTRACT.md exactly. Keep in sync with the backend contract.

export type ScenePreset = "interior" | "exterior";

export type LightingPreset =
  | "morning_sun"
  | "overcast"
  | "midday"
  | "afternoon_sun"
  | "late_afternoon"
  | "night";

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
  // How many model calls it took to get this render (1 unless earlier
  // attempts were discarded for not following the screenshot). Latencies,
  // tokens and estimatedCostUsd above cover all of them. Absent on renders
  // made before this existed.
  attempts?: number;
}

export interface PreservationReport {
  edgeScore: number;
  edgeFlag: boolean;
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
  // Set when this render is an Edit: the render (in the same view) it was
  // made from, and what each edited region was asked to become.
  sourceRenderId?: string;
  editInstructions?: string[];
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
  code?: string;
}

// One model+resolution combination's estimated cost of a single render, from
// GET /api/pricing.
export interface PricingEstimate {
  model: ModelChoice;
  resolution: Resolution;
  costUsd: number;
  costBrl: number;
  credits: number; // cost of ONE variation, in credits (1 credit = one 2K Pro image)
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

// One region of an edit request: what to change it into, and the painted area
// as a base64 (no data: prefix) white-on-black PNG. The bitmap may be drawn at
// a reduced size but must keep the render's aspect ratio.
export interface EditRegionRequest {
  instruction: string;
  bitmap: string;
}

// Async render job: POST .../render returns one of these (202) instead of
// Render[] directly, and the client polls GET .../render-jobs/{jid} until it
// reaches a terminal status. See api.ts's renderView, which hides the
// polling behind the old Render[]-returning signature.
export type RenderJobStatus = "queued" | "running" | "done" | "failed";

export interface RenderJobVariation {
  status: RenderJobStatus;
  renderId?: string;
  error?: string;
}

export interface RenderJob {
  id: string;
  viewId: string;
  status: RenderJobStatus;
  createdAt: string;
  updatedAt: string;
  request: RenderRequest;
  variations: RenderJobVariation[];
  renders: Render[]; // full Render objects for variations that are done, in order
  error?: string; // set when status === "failed": first variation error
  creditsCharged?: number; // credits charged for the whole job, original charge (not net of refunds)
}

// ---- Credits ----

// GET /api/me: the signed-in user's identity and credit balance. userId is ""
// when Clerk auth is disabled (local dev without CLERK_SECRET_KEY) - the
// frontend then skips client-side "not enough credits" checks, since the
// backend does too (see api.ts renderView / RenderControls).
export interface Me {
  userId: string;
  role?: string;
  isAdmin: boolean;
  credits: number;
}

export type CreditReason = "grant" | "render" | "refund";

export interface CreditEntry {
  id: string;
  createdAt: string;
  delta: number; // credits, signed
  balanceAfter: number;
  reason: CreditReason;
  projectId?: string;
  jobId?: string;
  note?: string;
  actorId?: string;
}

export interface CreditsResponse {
  credits: number;
  ledger: CreditEntry[]; // newest first, at most 50 entries
}

// ---- Admin ----

export interface AdminUser {
  id: string;
  email: string;
  firstName?: string;
  lastName?: string;
  imageUrl?: string;
  role?: string;
  credits: number;
  createdAt: string;
  lastSignInAt?: string;
}

export interface AdminUsersResponse {
  users: AdminUser[];
  totalCount: number;
}

// Labels/hints for these constants live in the translation files (see
// src/i18n/locales/*.json under style.lightingPresets / style.interiorLights)
// rather than as literal strings here - `value` is the token the API expects
// (unchanged), `labelKey`/`hintKey` are i18next keys resolved with t() at
// render time.

export const LIGHTING_PRESETS: { value: LightingPreset; labelKey: `style.lightingPresets.${LightingPreset}` }[] = [
  { value: "morning_sun", labelKey: "style.lightingPresets.morning_sun" },
  { value: "midday", labelKey: "style.lightingPresets.midday" },
  { value: "overcast", labelKey: "style.lightingPresets.overcast" },
  { value: "afternoon_sun", labelKey: "style.lightingPresets.afternoon_sun" },
  { value: "late_afternoon", labelKey: "style.lightingPresets.late_afternoon" },
  { value: "night", labelKey: "style.lightingPresets.night" },
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
