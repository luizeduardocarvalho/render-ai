# API Contract (frontend <-> backend)

This is the single source of truth both the Go backend and the React frontend build against.

State is **persisted per user**. The deployed backend stores structured data in
Firestore and image blobs in a GCS bucket (`STORAGE=firestore`); local dev uses
an in-memory store (`STORAGE=memory`, the default) that is lost on restart. Each
project has an `ownerId` (the Clerk user id); every project-scoped route is
authorized against it, and a caller only ever sees their own projects. The
frontend holds a working copy of the open project and syncs via these endpoints.

Base URL: `http://localhost:8080`. All JSON unless noted. CORS is open to `http://localhost:5173`.

## Data model (shared shapes)

```ts
// ---- IDs are server-generated UUID strings ----

type ScenePreset = "interior" | "exterior";
type LightingPreset =
  | "morning_sun" | "overcast" | "golden_hour"
  | "evening_interior_lights" | "night_exterior";
type InteriorLights = "off" | "3000k" | "4000k" | "6000k" | "";  // "" = never set (older projects)
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
  // promptTokens/outputTokens/thoughtsTokens are summed across every model
  // call this render made: the image generation call, plus the text-model
  // preservation check if it ran. outputTokens is TEXT output only - it
  // never includes the image call's own generated-image tokens, which are
  // priced per-image (see PricingResponse) rather than per-token, to avoid
  // double-counting them at the text/thinking output rate. thoughtsTokens is
  // the thinking-token portion, broken out for visibility; it's already
  // included in whatever estimatedCostUsd charges at the output rate.
  promptTokens?: number;
  outputTokens?: number;
  thoughtsTokens?: number;
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
  interiorLights: InteriorLights; // artificial lights off, or on at a color temperature; new projects default to "off"
  materialNotes: string;   // free text
  extraInstructions: string;
}

interface Project {
  id: string;
  ownerId: string;         // Clerk user id of the owner (server-set)
  orgId: string | null;    // reserved for future org scoping; always null today
  name: string;
  createdAt: string;       // ISO
  updatedAt: string;       // ISO
  deletedAt?: string;      // ISO; present only on a soft-deleted project - see DELETE below.
                           // A caller never actually observes this: every route treats a
                           // deleted project as 404, so it is omitted from all live responses.
  style: StyleSettings;
  assets: Asset[];
  views: View[];
  styleAnchorRenderId: string | null; // which render (in any view) is the anchor
}

// Lightweight projection returned by GET /api/projects (the project picker).
interface ProjectSummary {
  id: string;
  name: string;
  createdAt: string;       // ISO
  updatedAt: string;       // ISO
  viewCount: number;
  renderCount: number;
  thumbnailImageId?: string; // first view's screenshot blob id, if any
}

// Returned by GET /api/pricing - see the Pricing endpoint below.
interface PricingResponse {
  usdToBrl: number;
  estimates: {
    model: ModelChoice;
    resolution: Resolution;
    costUsd: number;
    costBrl: number;
  }[];
}
```

## Image/blob transport

Images are never inlined in JSON. Each image is a server-side blob with an ID.
- Upload: `multipart/form-data` with a `file` field -> returns `{ "imageId": "..." }` where relevant, or the parent object is returned with the id filled in.
- Masks use the same blob mechanism but are always PNG (white-on-black) at screenshot resolution.

**Reading images (signed URLs).** The frontend never hardcodes a blob's URL.
To load a blob it first asks for a signed URL, then uses that as the `<img src>`
(or fetches it for download):

- `GET /api/projects/{pid}/images/{imageId}/url` -> `{ "url": "..." }`

This route is authenticated and ownership-gated via `{pid}`, so per-user access
is enforced when the URL is minted. In production the URL is a short-lived (~1h)
V4-signed GCS URL loaded directly from the bucket; in local dev (`memory`
backend) it is the same-origin `/api/images/{imageId}` path. Signed URLs must be
used verbatim - do not append query params to a GCS signed URL (it breaks the
signature); to refresh, request a new one.

- `GET /api/images/{imageId}` -> raw bytes. **Only registered for the in-memory
  (dev) backend**, where the signed URL above points back at it. With the GCS
  backend this route does not exist - images load straight from signed GCS URLs.

## Endpoints

### Projects
- `GET    /api/projects` -> `ProjectSummary[]` (the caller's own projects, most-recently-updated first)
- `POST   /api/projects` `{ name }` -> `Project` (owner set to the caller)
- `GET    /api/projects/{pid}` -> `Project`
- `DELETE /api/projects/{pid}` -> `204` (**soft delete** - see below)
- `PUT    /api/projects/{pid}/style` `StyleSettings` -> `Project`
- `POST   /api/projects/{pid}/anchor` `{ renderId | null }` -> `Project`  (set/clear style anchor)

**Deleting a project is a soft delete.** It sets `deletedAt` on the project
doc; nothing else is touched - views, renders and their blobs are left in
place, so the project is recoverable (by support, directly in the store) for
30 days. From that point on every route treats the project as gone: it drops
out of `GET /api/projects`, and `GET/PUT/POST/DELETE` on `{pid}` (and anything
nested under it) all return 404, exactly like a project that never existed or
belongs to someone else. Deleting an already-deleted or missing project also
returns 404. A scheduled purge job to hard-delete projects 30+ days past
`deletedAt` (doc + views/renders + blobs) is not implemented yet - see
`PERSISTENCE_HANDOFF.md`.

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

### Pricing
- `GET /api/pricing` -> `PricingResponse`. Requires only a signed-in user (like
  `/api/me`), not project ownership - it's a static price list, not project
  data. For every model+resolution combination the UI offers, `costUsd`/
  `costBrl` estimate the cost of one render: the per-image price plus an
  assumed typical request (6,000 input tokens, 500 thinking/text output
  tokens) at that model's own rates - see `ImageCallCost` in
  `backend/internal/render/pricing.go`. It does not reflect the actual tokens
  of any render that has run; `Render.metrics.estimatedCostUsd` is the real
  figure for a given render.

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
