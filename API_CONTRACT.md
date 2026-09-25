# API Contract (frontend <-> backend)

This is the single source of truth both the Go backend and the React frontend build against.

State is **persisted per user**. The deployed backend stores structured data in
Firestore and image blobs in a GCS bucket (`STORAGE=firestore`); local dev uses
an in-memory store (`STORAGE=memory`, the default) that is lost on restart. Each
project has an `ownerId` (the Clerk user id); every project-scoped route is
authorized against it, and a caller only ever sees their own projects. The
frontend holds a working copy of the open project and syncs via these endpoints.

Base URL: `http://localhost:8080`. All JSON unless noted. CORS is open to `http://localhost:5173`.

**Auth.** Every data route requires a verified Clerk session (`requireAuth`)
plus, for anything under `{pid}`, that the caller owns that project
(`requireOwner`) - the app is open to any signed-in user, not just admins.
Only `/api/admin/*` stays gated on the Clerk `publicMetadata.role` being
exactly `"admin"` (`requireAdmin`): listing every user and granting credits.
When `CLERK_SECRET_KEY` is unset (local dev), all of this is skipped and
every request is treated as user `""`.

## Data model (shared shapes)

```ts
// ---- IDs are server-generated UUID strings ----

type ScenePreset = "interior" | "exterior";
type LightingPreset =
  | "morning_sun" | "midday" | "overcast"
  | "afternoon_sun" | "late_afternoon" | "night";
// Projects saved with the retired ids read back mapped: golden_hour -> late_afternoon,
// evening_interior_lights and night_exterior -> night.
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
  createdAt: string;        // ISO
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
  sourceRenderId?: string; // set iff this render is an Edit: the render (same view) it was made from
  editInstructions?: string[]; // set iff Edit: what each edited region was asked to become, in region order
  upscaledFromRenderId?: string; // set iff this render is an Upscale: the render (same view) it is a 4K version of
  editDrift?: EditRegionDrift[]; // set iff Edit: how much each region's look changed from its source
}

interface RenderMetrics {
  model: string;           // resolved model id actually called
  resolution: Resolution;
  regionCount: number;
  anchorUsed: boolean;
  imageCallMs: number;     // latency of the image generation call only
  totalMs: number;         // includes the preservation check's edge-IoU compute, if it ran
  // promptTokens/outputTokens/thoughtsTokens are from the image generation
  // call only. outputTokens is TEXT output only - it never includes the
  // image call's own generated-image tokens, which are priced per-image
  // (see PricingResponse) rather than per-token, to avoid double-counting
  // them at the text/thinking output rate. thoughtsTokens is the
  // thinking-token portion, broken out for visibility; it's already
  // included in whatever estimatedCostUsd charges at the output rate.
  promptTokens?: number;
  outputTokens?: number;
  thoughtsTokens?: number;
  estimatedCostUsd?: number;
  // Model calls it took to get this render: 1, or more when earlier attempts
  // were discarded (see "Regeneration"). imageCallMs, totalMs, the token
  // counts and estimatedCostUsd cover every attempt. Absent on old renders.
  attempts?: number;
}

interface EditRegionDrift {
  number: number;          // 1-based, in region order
  instruction: string;
  // Mean CIELAB distance between the region's block-averaged colors before and
  // after (lightness at half weight): ~0-6 is the same look (grass made more
  // realistic grass), 15+ another material or color (grass turned to pebbles).
  drift: number;
  changedShare: number;    // 0..1, share of the region's blocks with drift >= 12; shows a change that covers only part of the region, which the mean dilutes
  // INFORMATION ONLY: nothing flags or regenerates an edit on it, and a
  // deliberate change of material or color scores high by design.
}

interface PreservationReport {
  edgeScore: number;       // 0..1 edge IoU (dilated, masked regions excluded)
  edgeFlag: boolean;       // true if below threshold
  // One entry per asset painted on the screenshot (omitted when none): how far
  // the colors of its region in the render are from its reference photo, as
  // the distance between the two color palettes in CIELAB (hue and chroma
  // fully, lightness at half weight; the mask is shrunk slightly first).
  // Lower is better: ~5 is the same asset lit differently, 20+ a clearly
  // different color. INFORMATION ONLY for now - it never sets edgeFlag and
  // never triggers regeneration; it is there to validate a future threshold.
  assetColors?: AssetColorScore[];
}

interface AssetColorScore {
  assetId: string;
  assetName: string;
  distance: number;
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
  assets: Asset[];         // the OWNER's asset library (see "Asset library" below), not
                           // project-specific data - every one of that user's projects
                           // reports the same list here.
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

// A render request turned into a job: POST .../render returns 202 with one
// of these instead of running the render synchronously, and the frontend
// polls GET .../render-jobs/{jid} until status is "done" or "failed". See
// the Render endpoint section below.
interface RenderJob {
  id: string;
  viewId: string;
  status: "queued" | "running" | "done" | "failed";
  createdAt: string;       // ISO
  updatedAt: string;       // ISO
  request: {
    model: ModelChoice;
    resolution: Resolution;
    preservationCheck: boolean;
    variations: number;
  };
  variations: {
    status: "queued" | "running" | "done" | "failed";
    renderId?: string;     // set once this variation's status is "done"
    error?: string;        // set once this variation's status is "failed"
  }[];
  renders: Render[];       // full Render objects for "done" variations, in variation order
  error?: string;          // set iff status == "failed": the first variation's error
  creditsCharged: number;  // credits charged for the WHOLE job as originally charged
                           // (unitsPerVariation * variations - see "Credits" below); not
                           // net of any refunds, and 0 when auth is disabled.
}

// One entry in a user's credit ledger - see GET /api/me/credits below.
interface CreditEntry {
  id: string;
  createdAt: string;       // ISO
  delta: number;            // credits, signed (positive = grant/refund, negative = charge)
  balanceAfter: number;     // credits, the balance right after this entry
  reason: "grant" | "render" | "refund";
  projectId?: string;
  jobId?: string;
  note?: string;
  actorId?: string;         // set for "grant": the admin who granted it
}

// One row of GET /api/admin/users - see "Admin" below.
interface AdminUser {
  id: string;
  email: string;
  firstName: string;
  lastName: string;
  imageUrl: string;
  role: string;             // Clerk publicMetadata.role ("" if unset)
  credits: number;
  createdAt: string;        // ISO
  lastSignInAt?: string;    // ISO
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

### Direct uploads
Images (screenshots, asset reference photos) are uploaded by the browser
straight to the storage bucket, so large files never pass through Firebase
Hosting (60s cutoff) or the API:
1. `POST /api/projects/{pid}/uploads` `{ contentType: "image/png" | "image/jpeg" }`
   -> `{ uploadId, url, method: "PUT", headers }`. `501` when the blob store
   can't take direct uploads (in-memory store, local dev): use multipart instead.
2. `PUT url` with the file as the body and exactly `headers` (they are signed).
   The URL is valid for 15 minutes and creates the object once.
3. Pass `uploadId` to the endpoint that uses the image (JSON body, below). The
   server validates the image (PNG/JPEG, at most 50 MB) and stores it as a new
   blob; an `uploadId` can be used only once. Unused uploads are deleted
   after a day.

### Asset library
Assets belong to a USER, not a project: every one of a user's projects shares
the same library, manageable from the project list screen (no project id
needed) and still usable inside the editor. All routes below require only a
signed-in user (`requireAuth`), scoped to the caller - there is no ownership
check beyond "you are logged in as this user".

- `GET    /api/assets` -> `Asset[]`, newest first
- `POST   /api/assets` `{ name, description, color }` -> `Asset`
- `PUT    /api/assets/{aid}` `{ name, description, color }` -> `Asset`
- `DELETE /api/assets/{aid}` -> `204`. Does not scan the caller's projects to
  clear the deleted id from masks: render already skips a mask whose
  `assetId` no longer resolves to a library asset, and the frontend must
  treat an unknown `assetId` as "unassigned".
- `POST   /api/assets/{aid}/reference` multipart `file`, or JSON `{ uploadId }` -> `Asset`
- `GET    /api/assets/{aid}/reference-url` -> `{ url }`, a signed URL for that
  asset's reference image (404 if it has none) - for the library screen,
  which has no project id to authenticate a project-scoped
  `images/{id}/url` call through.
- `POST   /api/uploads` `{ contentType }` -> same body/behavior as the
  project-scoped direct-upload route below (`501` on the in-memory blob
  store -> fall back to multipart).

**Back-compat.** `GET /api/projects/{pid}` keeps returning `assets`, filled
from the project OWNER's library (every project of that owner reports the
same list). The project-scoped routes below keep working, operating on the
owner's library (resolved from `{pid}` via the existing ownership check, so
there is no separate per-project data any more):
- `POST   /api/projects/{pid}/assets` `{ name, description, color }` -> `Asset`
- `PUT    /api/projects/{pid}/assets/{aid}` `{ name, description, color }` -> `Asset`
- `DELETE /api/projects/{pid}/assets/{aid}` -> `204`
- `POST   /api/projects/{pid}/assets/{aid}/reference` multipart `file`, or JSON `{ uploadId }` -> `Asset`
- `GET    /api/projects/{pid}/images/{id}/url` also keeps working for library
  reference images (it only checks project ownership today, which is fine).

**Migration.** Projects created before the library existed have assets
embedded directly in the project doc, with masks pointing at those ids. The
first time such a project is loaded (`GET /api/projects/{pid}` or any handler
that reads the project), those assets are moved into the owner's library
under the SAME ids (idempotently - an id already present in the library is
left untouched) and then cleared from the project doc, so this only ever
runs once per project and every existing mask binding keeps resolving.

### Views
- `POST   /api/projects/{pid}/views` multipart `file` (+ form field `name`), or JSON `{ name, uploadId }` -> `View`
      (server reads width/height from the screenshot)
- `GET    /api/projects/{pid}/views/{vid}` -> `View`
- `DELETE /api/projects/{pid}/views/{vid}` -> `204`
- `PUT    /api/projects/{pid}/views/{vid}/inventory` `{ inventory }` -> `View`
- `POST   /api/projects/{pid}/views/{vid}/inventory/generate` -> `{ inventory }`
      (calls Gemini text model on the screenshot; overwrites cache; returns text).
      Vertex quota errors (429 RESOURCE_EXHAUSTED) and 503s are retried up to 3
      attempts with backoff. If the last answer is still a quota error the
      response is `429` with `code: "rate_limited"` (the client shows a "busy,
      try again in a minute" message). Any other failure, including a 503 that
      persists, is `502`.

### Masks (per view)
- `POST   /api/projects/{pid}/views/{vid}/masks` `{ assetId? }` -> `Mask`
- `PUT    /api/projects/{pid}/views/{vid}/masks/{mid}` `{ assetId?, hidden? }` -> `Mask`
- `PUT    /api/projects/{pid}/views/{vid}/masks/{mid}/bitmap` multipart `file` (PNG, white-on-black, screenshot res) -> `Mask`
- `DELETE /api/projects/{pid}/views/{vid}/masks/{mid}` -> `204`

### Render

Rendering is **async**: the POST below validates the request and returns a
job immediately, and the caller polls a second endpoint until it's terminal.
This exists because the app is served through Firebase Hosting, which cuts a
proxied request off at ~60s - too short for a slow render (Pro / 2K-4K /
several variations) even though the backend itself allows up to ~300s. Behind
the scenes, one Cloud Task is enqueued per variation and a worker route
(`POST /internal/render-tasks`, not part of this frontend-facing contract -
see `backend/DEPLOY.md`) does the actual Vertex AI call for each.

- `POST   /api/projects/{pid}/views/{vid}/render`
  ```json
  { "model": "pro", "resolution": "2K", "preservationCheck": true, "variations": 2 }
  ```
  `variations` is optional, defaults to `1`, and is clamped to the range `1-4`
  (values outside that range are a 400). Each variation is an independent
  sample from the model (no seed is exposed, so repeated calls already
  differ) and becomes its own `Render` record, appended to the view's
  `renders` list once its task completes.
  Synchronous checks are unchanged from before: 400 for a bad model/
  resolution/variations count or flash+non-1K, 404 if the project or view
  doesn't exist, 400 if the view has no screenshot, 500 if the renderer isn't
  configured (missing Vertex AI credentials).
  -> **`202 Accepted`** with a `RenderJob` (its `variations` all start
  `"queued"`; `renders` is `[]`). If enqueueing a task fails, the job (and
  every not-yet-enqueued variation) is marked `"failed"` and the handler
  returns `502 { "error": "queueing render: ..." }` instead - see "Credits"
  below for what happens to the charge in that case.
  Before creating the job, `unitsPerVariation * variations` credits are
  debited from the project owner's balance atomically (see "Credits"); too
  little balance -> **`402`** `{ "error": "insufficient credits", "code":
  "insufficient_credits" }`, and nothing is created.

  **Regeneration.** With `preservationCheck` on, a render whose
  `preservation.edgeFlag` is set (its edge score is below
  `preservation.edgeScoreFlagThreshold`, i.e. it does not follow the
  screenshot) is generated again by a new task, at most
  `preservation.maxRegenerations` times (default config: 2, so at most 3
  model calls per variation; hard ceiling 5). Attempts that are discarded are
  never shown or added to `renders`; the variation stays `"queued"`/`"running"`
  meanwhile. It ends with the first render that passes, or, when every attempt
  is flagged (or a later attempt fails outright), the best-scoring one, still
  flagged. `metrics.attempts` says how many calls that took and
  `metrics.estimatedCostUsd` includes all of them. **Credits are not charged
  for regenerations** - the up-front debit is per variation, as before.
  Edits (below) are never regenerated.

- `GET    /api/projects/{pid}/render-jobs/{jid}` -> `200` with the current
  `RenderJob`, or `404` if it doesn't exist (or belongs to another project).
  Poll this every ~3s until `status` is `"done"` or `"failed"`; a generous
  overall cap (~15 minutes) protects against a poll loop that never sees a
  terminal status. `status` is derived from `variations`:
  - any variation `"queued"`/`"running"` -> `"running"` if at least one has
    started or finished, otherwise `"queued"`.
  - every variation terminal and at least one `"done"` -> `"done"` (this is
    also the *partial-success* case: some variations may still be `"failed"`,
    each with its own `error`, but as long as one render came back the job as
    a whole is `"done"` and `renders` holds whatever succeeded).
  - every variation terminal and none `"done"` -> `"failed"`, and the job's
    top-level `error` is the *first* variation's error message.
  A variation stuck `"running"` for longer than the render timeout (its
  worker died mid-render - a killed instance, a crash) is reported as
  `"failed"` with `error: "render worker did not finish"` once enough time
  has passed - this is computed each time the job is read, not written back.

### Edit

An **Edit** changes marked areas of an existing render and keeps everything
else. It is a new `Render` appended to the same view, linked to the render it
came from through `sourceRenderId`; the source is never touched, and an edit
can itself be edited. See `CONTEXT.md` for the vocabulary.

- `POST   /api/projects/{pid}/views/{vid}/renders/{rid}/edit`
  ```json
  { "regions": [ { "instruction": "a brass floor lamp", "bitmap": "<base64 PNG>" } ] }
  ```
  Each region is one area to change plus what to change it into. `bitmap` is a
  base64 (standard alphabet, no `data:` prefix) white-on-black PNG of the
  painted area. It may be drawn at a reduced size but must keep the render's
  aspect ratio (within 1%); the server scales it to the render's size.
  Limits: 1-8 regions, each `instruction` non-blank and at most 500
  characters, each bitmap a PNG with at least one painted pixel, request body
  at most 32 MiB. Any of these failing is a `400`; an unknown project, view or
  render (the render must belong to `{vid}`) is a `404`; a missing renderer is
  a `500`, as for renders.
  -> **`202 Accepted`** with a `RenderJob` exactly like the render endpoint's,
  polled through `GET .../render-jobs/{jid}` until terminal. The job has one
  variation and its `request` carries the source's own `model` and
  `resolution` plus an `edit` block (`sourceRenderId` and the regions'
  instructions). On `"done"`, `renders[0]` is the new Render.
  **Price and charging** are a render's: one variation at the source's
  model+resolution (`unitsPerVariation`), debited up front, `402` on too
  little balance, refunded exactly once if the variation fails - the whole
  "Credits" section applies unchanged.
  **How it works.** The model gets the source render, a copy of it with the
  regions filled in distinct colors, and the view's original screenshot as the
  ground truth for what the room holds, plus a prompt listing each color's
  instruction (`backend/prompts/edit.tmpl`); no style settings, asset photos or
  preservation check are sent. The answer is scaled to the source's size, then
  scrubbed of overlay colors (`geometry.ScrubOverlayColors`): the model
  sometimes draws lines in the regions' label colors despite being told not to,
  along a region's border, along the image's edge, or along a layout of its own
  that matches no region. Inside the edited area, anything in a region's own
  color within ~2% of the shorter side of that region's border, and any thin
  line (under ~1.2% of the shorter side wide) in any region's color, is
  repaired from the texture around it; wider blobs of a label color are scene
  content and kept. The answer is then
  blended over the source through the union of the regions, grown by
  ~0.5% of the shorter side and blurred with a sigma of ~0.5% of it
  (`geometry.EditAlpha`), so the edit fades in at its edge and **every pixel
  more than ~2% of the shorter side away from a region is the source's own,
  byte for byte**. The region bitmaps are stored only
  while the job runs and deleted when it ends. The resulting `Render` has
  `regionCount` = the number of regions and no `preservation`.

### Upscale

An **Upscale** is the 4K version of a finished render. Rendering the same
request again at 4K would give a different picture (the image models take no
seed), so an upscale reproduces the render the user already has instead. It is
a new `Render` appended to the same view, linked to its source through
`upscaledFromRenderId` (never `sourceRenderId`, which means Edit); the source is
never touched. See `CONTEXT.md` for the vocabulary.

- `POST   /api/projects/{pid}/views/{vid}/renders/{rid}/upscale` (no body)
  A `400` if the render is already 4K; a `404` for an unknown project, view or
  render (the render must belong to `{vid}`); a `500` for a missing renderer or
  image blob, as for renders.
  -> **`202 Accepted`** with a `RenderJob` exactly like the render endpoint's,
  polled through `GET .../render-jobs/{jid}` until terminal. The job has one
  variation and its `request` is `model: "pro"`, `resolution: "4K"` plus an
  `upscale` block (`sourceRenderId`), whatever the source was made with (the
  flash model stops at 1K). On `"done"`, `renders[0]` is the new Render, with
  `regionCount` 0 and no `preservation`.
  **Price and charging** are a pro 4K render's (2 credits): debited up front,
  `402` on too little balance, refunded exactly once if the variation fails -
  the whole "Credits" section applies unchanged.
  **How it works.** The model gets only the source render and a prompt asking
  for the same image at 4K (`backend/prompts/upscale.tmpl`); no screenshot,
  style settings or asset photos are sent. The model reproduces the picture and
  adds fine detail, but shifts colors a little (measured on one render: paler
  bricks, mean red +5, blue +4), so its answer is pinned back to the source's
  colors and lighting (`geometry.PinLowFrequency`): both images are blurred
  (sigma ~0.5% of the width) and the difference between the blurred source and
  the blurred answer is added to the answer. Everything finer than that blur,
  which is all the detail the model added, is kept. This works on a
  640-px-wide copy and changes the answer in place, so a 5504x3072 result takes
  ~0.2 s and adds no full-size copies to a 512 MiB worker. Fine texture (brick
  grain, weave, foliage) is still the model's own, so an upscale is close to the
  source, not pixel-identical to it.
  **Failure.** An answer that is not a larger image with the source's aspect
  ratio fails the job, and so refunds it, instead of putting a different picture
  or no more pixels in the history under the name of an upscale. Like an Edit,
  an upscale is never regenerated.

### Notifications

The header bell lists the user's recent render jobs (a Render, an Edit or an
Upscale each make one) so a user who left the screen, the project or the page
can see what is running and what finished, and open it. The list is the source
of truth: the frontend keeps no job state of its own across reloads.

- `GET    /api/me/render-jobs` -> `Notification[]`
  Any signed-in user (not project-gated): the caller's own projects only, jobs
  last updated within the past 24 hours (marking a job seen does not count as an
  update, so seeing one late does not keep it listed), newest **created** first.
  A job whose view has been deleted, or whose project is soft-deleted, is left out - there is
  nothing to open. Status is derived exactly as for `GET .../render-jobs/{jid}`
  (including the stale-running rule), but no `Render` objects are loaded.

```ts
interface Notification {
  projectId: string;
  projectName: string;
  viewId: string;
  viewName: string;
  jobId: string;
  kind: "render" | "edit" | "upscale";
  status: "queued" | "running" | "done" | "failed";
  createdAt: string;     // ISO
  updatedAt: string;     // ISO
  variations: { done: number; failed: number; total: number };
  renderId?: string;     // the first finished variation's Render, once there is one
  error?: string;        // set iff status == "failed"
  seen: boolean;         // the user has opened this outcome
}
```

  A notification is **unread** when `status` is `"done"` or `"failed"` and
  `seen` is `false`. Jobs from before this feature have no `seen`, so for the
  first 24 hours after it ships they all show as unread.

- `POST   /api/projects/{pid}/render-jobs/{jid}/seen` -> `204`
  Records that the user has seen the job's outcome (`seen` becomes `true`).
  Idempotent: a second call changes nothing. A job that has not finished yet is
  left unseen (its outcome is still to come), also with `204`. `404` for an
  unknown project or job, or one that belongs to another user.

### Pricing
- `GET /api/pricing` -> `PricingResponse`. Requires only a signed-in user (like
  `/api/me`), not project ownership - it's a static price list, not project
  data. For every model+resolution combination the UI offers, `costUsd`/
  `costBrl` estimate the cost of one render: the per-image price plus an
  assumed typical request (6,000 input tokens, 500 thinking/text output
  tokens) at that model's own rates - see `ImageCallCost` in
  `backend/internal/render/pricing.go`. It does not reflect the actual tokens
  of any render that has run; `Render.metrics.estimatedCostUsd` is the real
  figure for a given render. Each estimate also carries `credits`: the EXACT
  credit cost of one variation at that model+resolution (see "Credits"
  below) - unlike `costUsd`/`costBrl` this isn't an estimate, it's what
  `POST .../render` actually charges.

### Credits
Credits gate rendering: everyone, including admins, pays credits for
renders, from a per-user balance. 1 credit = one 2K Pro image (see
`PRICING.md`): Pro 1K/2K = 1 credit/variation, Pro 4K = 2 credits/variation,
Flash (1K) = 0.25 credit/variation. Balances are stored server-side as
integer quarter-credit units, but the JSON API always speaks **credits as a
number** (e.g. `12.25`).

- `GET /api/me` -> `{ userId, role, isAdmin, credits: number }` (credits
  added to the existing shape).
- `GET /api/me/credits` -> `{ credits: number, ledger: CreditEntry[] }`,
  ledger newest first, at most 50 entries.
- Rendering debits credits atomically before a job is created (see the
  Render section above for the 402 and the enqueue-failure/refund behavior).
  `POST .../inventory/generate` requires a balance `> 0` (else the same 402)
  but is not itself charged - it's a cheap text-model call.
- **Refunds.** A render-job variation that ends up `"failed"` (a model
  error, a storage error, or never getting queued because enqueueing itself
  failed) is refunded its charged units exactly once - the job stores the
  per-variation charge at creation time (so a later price-table change never
  changes what a refund gives back) and a `Refunded` marker per variation
  that's set inside the same job-update transaction that marks it failed, so
  a redelivered worker task, or any other repeated failure observation, can
  never refund the same variation twice. If job creation/enqueueing fails
  outright (nothing was ever queued), the whole charge is refunded.
- **Auth disabled** (`CLERK_SECRET_KEY` unset, local dev): all credit
  checks/charges are skipped entirely, consistent with every other
  auth-disabled bypass in this API. `GET /api/me` then reports the balance
  of user `""` (normally 0). Frontend rule: the client-side "not enough
  credits" disable is advisory, the server's 402 is the source of truth, and
  when `userId === ""` the frontend skips the client-side check.

### Admin
Admin-only (`requireAdmin` - a signed-in user whose Clerk
`publicMetadata.role` is exactly `"admin"`).

- `GET  /api/admin/users?query=&limit=50&offset=0` -> `{ users: AdminUser[],
  totalCount }`. Users come from the Clerk Backend API (search/paginated by
  `query`/`limit`/`offset`), with `credits` merged in from the store.
- `POST /api/admin/users/{uid}/credits` `{ amount: number, note?: string }`
  -> `{ credits: number }`. `amount` is in credits: non-zero, a multiple of
  0.25, `|amount| <= 10000`. Negative amounts are allowed (a correction) but
  the balance may not go below zero - `400` if it would. Ledger reason
  `"grant"`, `actorId` = the admin who made the call.
- `GET  /api/admin/users/{uid}/credits` -> same shape as `GET
  /api/me/credits`, for the named user.

### Images
- `GET /api/images/{imageId}` -> raw bytes

## Error shape
Non-2xx responses: `{ "error": "human readable message" }`. One error also
carries a machine-readable `code`: a render (or inventory generation) that
can't be paid for is `402 { "error": "insufficient credits", "code":
"insufficient_credits" }` - the frontend should branch on `code`, not the
message text.

## Notes for implementers
- **Resolution guard**: `flash` + (`2K`|`4K`) is a 400 with a clear message; frontend also disables those.
- **Image budget**: Pro allows <=14 input images. Render input order: screenshot, region map,
  edge map, style anchor (if any), then asset reference photos for assets used by the view's
  non-hidden masks. If that exceeds 14, drop asset refs beyond the limit and include a warning
  in the response (`metrics` note / server log). Region map is skipped when the view has no masks.
- **Seed is not supported** by the image models; do not send one. Temperature is left at default.
  The metrics/UI should show seed as "n/a (not supported)".
