# API Contract (frontend <-> backend)

This is the single source of truth both the Go backend and the React frontend build against.
All state is **in memory on the backend, keyed by project ID**. Restarting the server drops everything.
The frontend holds its own working copy and syncs via these endpoints.

Base URL: `http://localhost:8080`. All JSON unless noted. CORS is open to `http://localhost:5173`.

## Data model (shared shapes)

```ts
// ---- IDs are server-generated UUID strings ----

type ScenePreset = "interior" | "exterior";
type LightingPreset =
  | "morning_sun" | "overcast" | "golden_hour"
  | "evening_interior_lights" | "night_exterior";
type ModelChoice = "pro" | "flash";      // pro = gemini-3-pro-image, flash = gemini-3.1-flash-image
type Resolution  = "1K" | "2K" | "4K";   // flash supports 1K only

interface Asset {
  id: string;
  name: string;
  description: string;      // e.g. "dark green velvet three-seat sofa"
  color: string;           // unique display color, hex "#RRGGBB"
  referenceImageId: string;// -> image blob endpoint
  hasReferenceImage: boolean;
}

interface Mask {
  id: string;
  assetId: string | null;  // which library asset this region becomes
  // Mask pixels are stored as a PNG (white = painted, black = empty) at the
  // screenshot's ORIGINAL resolution. Uploaded/fetched via the mask blob endpoint.
  hasBitmap: boolean;
  hidden: boolean;
}

interface Render {
  id: string;
  createdAt: string;       // ISO
  model: ModelChoice;
  resolution: Resolution;
  anchorUsed: boolean;
  regionCount: number;
  resultImageId: string;   // -> image blob endpoint (full-res PNG)
  metrics: RenderMetrics;
  preservation?: PreservationReport; // present iff preservation check was on
  isStyleAnchor: boolean;
}

interface RenderMetrics {
  model: string;           // resolved model id actually called
  resolution: Resolution;
  regionCount: number;
  anchorUsed: boolean;
  imageCallMs: number;     // latency of the image generation call only
  totalMs: number;         // includes inventory/check calls done for this render
  promptTokens?: number;
  outputTokens?: number;
  estimatedCostUsd?: number;
}

interface PreservationReport {
  edgeScore: number;       // 0..1 edge IoU (dilated, masked regions excluded)
  edgeFlag: boolean;       // true if below threshold
  inventory: {
    removed: string[];
    added: string[];
    moved: string[];
    raw?: string;          // model's raw notes
  };
}

interface View {
  id: string;
  name: string;
  screenshotImageId: string;
  hasScreenshot: boolean;
  width: number;           // screenshot original px
  height: number;
  masks: Mask[];
  inventory: string;       // editable object inventory text, cached per view
  renders: Render[];
}

interface StyleSettings {
  scene: ScenePreset;
  lighting: LightingPreset;
  lightDirection: string;  // free text
  materialNotes: string;   // free text
  extraInstructions: string;
}

interface Project {
  id: string;
  name: string;
  style: StyleSettings;
  assets: Asset[];
  views: View[];
  styleAnchorRenderId: string | null; // which render (in any view) is the anchor
}
```

## Image/blob transport

Images are never inlined in JSON. Each image is a server-side blob with an ID.
- Upload: `multipart/form-data` with a `file` field -> returns `{ "imageId": "..." }` where relevant, or the parent object is returned with the id filled in.
- Fetch: `GET /api/images/{imageId}` -> raw PNG/JPEG bytes with correct `Content-Type`.
- Masks use the same blob mechanism but are always PNG (white-on-black) at screenshot resolution.

## Endpoints

### Projects
- `POST   /api/projects` `{ name }` -> `Project`
- `GET    /api/projects/{pid}` -> `Project`
- `PUT    /api/projects/{pid}/style` `StyleSettings` -> `Project`
- `POST   /api/projects/{pid}/anchor` `{ renderId | null }` -> `Project`  (set/clear style anchor)

### Assets
- `POST   /api/projects/{pid}/assets` `{ name, description, color }` -> `Asset`
- `PUT    /api/projects/{pid}/assets/{aid}` `{ name, description, color }` -> `Asset`
- `DELETE /api/projects/{pid}/assets/{aid}` -> `204`
- `POST   /api/projects/{pid}/assets/{aid}/reference` multipart `file` -> `Asset`

### Views
- `POST   /api/projects/{pid}/views` multipart `file` (+ form field `name`) -> `View`
      (server reads width/height from the screenshot)
- `GET    /api/projects/{pid}/views/{vid}` -> `View`
- `DELETE /api/projects/{pid}/views/{vid}` -> `204`
- `PUT    /api/projects/{pid}/views/{vid}/inventory` `{ inventory }` -> `View`
- `POST   /api/projects/{pid}/views/{vid}/inventory/generate` -> `{ inventory }`
      (calls Gemini text model on the screenshot; overwrites cache; returns text)

### Masks (per view)
- `POST   /api/projects/{pid}/views/{vid}/masks` `{ assetId? }` -> `Mask`
- `PUT    /api/projects/{pid}/views/{vid}/masks/{mid}` `{ assetId?, hidden? }` -> `Mask`
- `PUT    /api/projects/{pid}/views/{vid}/masks/{mid}/bitmap` multipart `file` (PNG, white-on-black, screenshot res) -> `Mask`
- `DELETE /api/projects/{pid}/views/{vid}/masks/{mid}` -> `204`

### Render
- `POST   /api/projects/{pid}/views/{vid}/render`
  ```json
  { "model": "pro", "resolution": "2K", "preservationCheck": true, "variations": 2 }
  ```
  `variations` is optional, defaults to `1`, and is clamped to the range `1-4`
  (values outside that range are a 400). Each variation is an independent
  sample from the model (no seed is exposed, so repeated calls already
  differ) and becomes its own `Render` record, appended to the view's
  `renders` list.
  -> `Render[]` (synchronous; may take 10-60s per variation; backend timeout
  generous ~180s for the whole request). Length equals the number of
  variations that succeeded: normally `variations`, but if a later variation
  fails after at least one earlier one succeeded, the response contains only
  the successes gathered so far (the failure is logged server-side, not
  raised as an error).
  Errors return `{ "error": "message" }` with a 4xx/5xx and a clear message for:
  refusal, no image returned, timeout, invalid resolution for model - but only
  when the *first* variation fails (no successes yet to return instead).

### Images
- `GET /api/images/{imageId}` -> raw bytes

## Error shape
Non-2xx responses: `{ "error": "human readable message" }`.

## Notes for implementers
- **Resolution guard**: `flash` + (`2K`|`4K`) is a 400 with a clear message; frontend also disables those.
- **Image budget**: Pro allows <=14 input images. Render input order: screenshot, region map,
  edge map, style anchor (if any), then asset reference photos for assets used by the view's
  non-hidden masks. If that exceeds 14, drop asset refs beyond the limit and include a warning
  in the response (`metrics` note / server log). Region map is skipped when the view has no masks.
- **Seed is not supported** by the image models; do not send one. Temperature is left at default.
  The metrics/UI should show seed as "n/a (not supported)".
