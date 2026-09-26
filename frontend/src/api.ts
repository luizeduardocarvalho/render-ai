import type {
  AdminUsersResponse,
  Asset,
  ApiErrorBody,
  CreditsResponse,
  EditRegionRequest,
  JobNotification,
  Mask,
  Me,
  PricingResponse,
  Project,
  ProjectSummary,
  Render,
  RenderJob,
  RenderRequest,
  StyleSettings,
  View,
} from "./types";
import { getClerkToken } from "./lib/clerkToken";

// Same-origin in production (Firebase Hosting rewrites /api/** to the Cloud
// Run backend, so no base URL is needed and no CORS is involved), local
// backend in dev. Override with VITE_API_URL if needed (e.g. staging).
export const API_BASE =
  import.meta.env.VITE_API_URL ?? (import.meta.env.PROD ? "" : "http://localhost:8080");

// Error code for a 402 render/generation rejection - see errors.go on the
// backend. Callers compare `err.code` against this rather than matching on
// the (possibly localized-by-nobody, but still not-for-matching) message.
export const INSUFFICIENT_CREDITS_CODE = "insufficient_credits";
export const RATE_LIMITED_CODE = "rate_limited";

export class ApiError extends Error {
  status: number;
  /** Machine-readable error code from the response body, e.g. "insufficient_credits". */
  code?: string;
  constructor(status: number, message: string, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

// Signed image URLs -----------------------------------------------------------
//
// Images are never loaded by a stable public path. The backend mints a URL for
// each blob only after the per-project ownership check passes; in production
// that's a short-lived V4 signed GCS URL loaded directly from the bucket, in
// local dev it's the same-origin /api/images/{id} path. Callers pass the owning
// project id so the ownership gate applies.
//
// Signed URLs must be used verbatim - appending query params (e.g. a cache
// buster) to a GCS signed URL breaks its signature. To force a fresh URL, pass
// `force` (or invalidate the cache); re-signing yields a distinct URL that also
// bypasses the browser cache.

interface SignedEntry {
  url: string;
  fetchedAt: number;
  pending?: Promise<string>;
}

// Refresh comfortably before the backend's 1h signature expiry.
const SIGNED_TTL_MS = 50 * 60 * 1000;
const signedCache = new Map<string, SignedEntry>();

function resolveSignedUrl(url: string): string {
  // Dev returns a relative /api/images/... path; prod returns an absolute GCS
  // URL. Only the relative form needs the API base prepended.
  return url.startsWith("http") ? url : `${API_BASE}${url}`;
}

/**
 * Resolves a loadable URL for an image blob, memoized per blob id until shortly
 * before expiry. Pass `force` to bypass the cache and re-sign.
 */
export async function fetchSignedImageUrl(
  pid: string,
  imageId: string,
  force = false,
): Promise<string> {
  const now = Date.now();
  const cached = signedCache.get(imageId);
  if (!force && cached && now - cached.fetchedAt < SIGNED_TTL_MS) return cached.url;
  if (!force && cached?.pending) return cached.pending;

  const pending = request<{ url: string }>(`/api/projects/${pid}/images/${imageId}/url`)
    .then((r) => {
      const url = resolveSignedUrl(r.url);
      signedCache.set(imageId, { url, fetchedAt: Date.now() });
      return url;
    })
    .catch((err) => {
      signedCache.delete(imageId);
      throw err;
    });

  signedCache.set(imageId, {
    url: cached?.url ?? "",
    fetchedAt: cached?.fetchedAt ?? 0,
    pending,
  });
  return pending;
}

/** Drops any cached signed URL for a blob, forcing the next fetch to re-sign. */
export function invalidateSignedImageUrl(imageId: string): void {
  signedCache.delete(imageId);
}

async function request<T>(
  path: string,
  init?: RequestInit,
  timeoutMs = 30_000,
): Promise<T> {
  const controller = new AbortController();
  const timer = timeoutMs > 0 ? setTimeout(() => controller.abort(), timeoutMs) : null;
  try {
    const token = await getClerkToken();
    const headers = new Headers(init?.headers);
    if (token) headers.set("Authorization", `Bearer ${token}`);
    const res = await fetch(`${API_BASE}${path}`, {
      ...init,
      headers,
      signal: controller.signal,
    });
    if (!res.ok) {
      let message = `Request failed (${res.status})`;
      let code: string | undefined;
      try {
        const body = (await res.json()) as ApiErrorBody;
        if (body?.error) message = body.error;
        code = body?.code;
      } catch {
        // ignore body parse failures, keep default message
      }
      throw new ApiError(res.status, message, code);
    }
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  } catch (err) {
    if (err instanceof ApiError) throw err;
    if (err instanceof DOMException && err.name === "AbortError") {
      throw new ApiError(0, "Request timed out. Please try again.");
    }
    throw new ApiError(0, err instanceof Error ? err.message : "Network error");
  } finally {
    if (timer) clearTimeout(timer);
  }
}

function json(body: unknown): RequestInit {
  return {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  };
}

// ---- Me / credits ----

export function getMe(): Promise<Me> {
  return request<Me>("/api/me");
}

/** The caller's render, edit and upscale jobs from the last day, newest first. */
export function listMyRenderJobs(): Promise<JobNotification[]> {
  return request<JobNotification[]>("/api/me/render-jobs");
}

/** Records that the user has seen a finished job's outcome. */
export function markRenderJobSeen(pid: string, jid: string): Promise<void> {
  return request<void>(`/api/projects/${pid}/render-jobs/${jid}/seen`, { method: "POST" });
}

export function getMyCredits(): Promise<CreditsResponse> {
  return request<CreditsResponse>("/api/me/credits");
}

// ---- Admin ----

export function listAdminUsers(
  query: string,
  limit: number,
  offset: number,
): Promise<AdminUsersResponse> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  if (query.trim()) params.set("query", query.trim());
  return request<AdminUsersResponse>(`/api/admin/users?${params.toString()}`);
}

export function grantAdminCredits(
  uid: string,
  amount: number,
  note?: string,
): Promise<{ credits: number }> {
  return request<{ credits: number }>(`/api/admin/users/${uid}/credits`, json({ amount, note }));
}

export function getAdminUserCredits(uid: string): Promise<CreditsResponse> {
  return request<CreditsResponse>(`/api/admin/users/${uid}/credits`);
}

// ---- Projects ----

export function listProjects(): Promise<ProjectSummary[]> {
  return request<ProjectSummary[]>("/api/projects");
}

export function createProject(name: string): Promise<Project> {
  return request<Project>("/api/projects", json({ name }));
}

export function getProject(pid: string): Promise<Project> {
  return request<Project>(`/api/projects/${pid}`);
}

export function updateStyle(pid: string, style: StyleSettings): Promise<Project> {
  return request<Project>(`/api/projects/${pid}/style`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(style),
  });
}

export function setAnchor(pid: string, renderId: string | null): Promise<Project> {
  return request<Project>(`/api/projects/${pid}/anchor`, json({ renderId }));
}

// Soft delete: the project (and its views/renders/blobs) is recoverable by
// support for 30 days, not purged immediately - see PERSISTENCE_HANDOFF.md.
export function deleteProject(pid: string): Promise<void> {
  return request<void>(`/api/projects/${pid}`, { method: "DELETE" });
}

// ---- Assets ----

export function createAsset(
  pid: string,
  data: { name: string; description: string; color: string },
): Promise<Asset> {
  return request<Asset>(`/api/projects/${pid}/assets`, json(data));
}

export function updateAsset(
  pid: string,
  aid: string,
  data: { name: string; description: string; color: string },
): Promise<Asset> {
  return request<Asset>(`/api/projects/${pid}/assets/${aid}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export function deleteAsset(pid: string, aid: string): Promise<void> {
  return request<void>(`/api/projects/${pid}/assets/${aid}`, { method: "DELETE" });
}

export function uploadAssetReference(
  pid: string,
  aid: string,
  file: File,
  opts?: UploadOptions,
): Promise<Asset> {
  return uploadImage(`/api/projects/${pid}/uploads`, file, opts, {
    path: `/api/projects/${pid}/assets/${aid}/reference`,
    fields: {},
  });
}

// ---- Asset library (user-wide, shared by all of the user's projects) ----
//
// Manageable from the project list screen (no project id in scope); the
// in-editor sidebar keeps using the project-scoped routes above, which the
// backend serves from the same underlying per-user library.

export function listAssets(): Promise<Asset[]> {
  return request<Asset[]>("/api/assets");
}

export function createLibraryAsset(data: {
  name: string;
  description: string;
  color: string;
}): Promise<Asset> {
  return request<Asset>("/api/assets", json(data));
}

export function updateLibraryAsset(
  aid: string,
  data: { name: string; description: string; color: string },
): Promise<Asset> {
  return request<Asset>(`/api/assets/${aid}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export function deleteLibraryAsset(aid: string): Promise<void> {
  return request<void>(`/api/assets/${aid}`, { method: "DELETE" });
}

export function uploadLibraryAssetReference(
  aid: string,
  file: File,
  opts?: UploadOptions,
): Promise<Asset> {
  return uploadImage("/api/uploads", file, opts, {
    path: `/api/assets/${aid}/reference`,
    fields: {},
  });
}

const assetRefSignedCache = new Map<string, SignedEntry>();

/**
 * Resolves a loadable URL for a library asset's reference image (used by the
 * library screen, which has no project id to go through the project-scoped
 * image route). Cached the same way fetchSignedImageUrl is.
 */
export async function fetchAssetReferenceUrl(aid: string, force = false): Promise<string> {
  const now = Date.now();
  const cached = assetRefSignedCache.get(aid);
  if (!force && cached && now - cached.fetchedAt < SIGNED_TTL_MS) return cached.url;
  if (!force && cached?.pending) return cached.pending;

  const pending = request<{ url: string }>(`/api/assets/${aid}/reference-url`)
    .then((r) => {
      const url = resolveSignedUrl(r.url);
      assetRefSignedCache.set(aid, { url, fetchedAt: Date.now() });
      return url;
    })
    .catch((err) => {
      assetRefSignedCache.delete(aid);
      throw err;
    });

  assetRefSignedCache.set(aid, {
    url: cached?.url ?? "",
    fetchedAt: cached?.fetchedAt ?? 0,
    pending,
  });
  return pending;
}

/** Drops any cached reference-image URL for a library asset. */
export function invalidateAssetReferenceUrl(aid: string): void {
  assetRefSignedCache.delete(aid);
}

// ---- Direct uploads ----
//
// Images go straight from the browser to the storage bucket through a
// short-lived signed URL, then the API gets just the resulting uploadId. A
// large screenshot through Firebase Hosting would otherwise run into its 60s
// cutoff for requests to Cloud Run (and this module's own 30s default).
// Local dev (in-memory blob store) answers 501, and we fall back to posting
// the file as multipart form data.

export interface UploadOptions {
  /** Upload progress as a fraction from 0 to 1 (direct uploads only). */
  onProgress?: (fraction: number) => void;
}

interface UploadTicket {
  uploadId: string;
  url: string;
  method: string;
  headers: Record<string, string>;
}

// Match the backend's accepted types (backend internal/api/uploads.go);
// anything else goes multipart so it gets the backend's usual validation error.
const DIRECT_UPLOAD_TYPES = new Set(["image/png", "image/jpeg"]);

// Generous: this only bounds a stalled upload, not a slow-but-moving one.
const DIRECT_UPLOAD_TIMEOUT_MS = 10 * 60_000;

// Multipart fallback still passes through Hosting (60s cutoff); give it
// that much rather than the 30s default.
const MULTIPART_UPLOAD_TIMEOUT_MS = 60_000;

const NETWORK_ERROR = "Upload failed: network error";

function putToSignedUrl(ticket: UploadTicket, file: File, onProgress?: (f: number) => void): Promise<void> {
  // XHR rather than fetch: fetch can't report upload progress.
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open(ticket.method, ticket.url);
    for (const [k, v] of Object.entries(ticket.headers)) xhr.setRequestHeader(k, v);
    xhr.timeout = DIRECT_UPLOAD_TIMEOUT_MS;
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress?.(e.loaded / e.total);
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) resolve();
      else reject(new ApiError(xhr.status, `Upload failed (${xhr.status})`));
    };
    xhr.onerror = () => reject(new ApiError(0, NETWORK_ERROR));
    xhr.ontimeout = () => reject(new ApiError(0, "Upload timed out. Please try again."));
    xhr.send(file);
  });
}

/** Returns the uploadId, or null when the backend can't take direct uploads. */
async function directUpload(
  uploadsPath: string,
  file: File,
  onProgress?: (f: number) => void,
): Promise<string | null> {
  if (!DIRECT_UPLOAD_TYPES.has(file.type)) return null;
  let ticket: UploadTicket;
  try {
    ticket = await request<UploadTicket>(uploadsPath, json({ contentType: file.type }));
  } catch (err) {
    if (err instanceof ApiError && err.status === 501) return null;
    throw err;
  }
  try {
    await putToSignedUrl(ticket, file, onProgress);
  } catch (err) {
    // A network-level failure is what a bucket without PUT in its CORS config
    // looks like (e.g. the app deployed before `terraform apply`). Fall back
    // to multipart rather than failing every upload; small files still work.
    if (err instanceof ApiError && err.status === 0 && err.message === NETWORK_ERROR) {
      console.warn("Direct upload failed, falling back to multipart upload");
      return null;
    }
    throw err;
  }
  return ticket.uploadId;
}

/**
 * Uploads an image for `target.path`: direct to storage when possible (via
 * `uploadsPath`, e.g. `/api/projects/{pid}/uploads` or `/api/uploads`), then
 * a JSON POST of {...fields, uploadId}; otherwise a multipart POST of
 * {...fields, file}.
 */
async function uploadImage<T>(
  uploadsPath: string,
  file: File,
  opts: UploadOptions | undefined,
  target: { path: string; fields: Record<string, string> },
): Promise<T> {
  const uploadId = await directUpload(uploadsPath, file, opts?.onProgress);
  if (uploadId) {
    return request<T>(target.path, json({ ...target.fields, uploadId }), MULTIPART_UPLOAD_TIMEOUT_MS);
  }
  const form = new FormData();
  form.append("file", file);
  for (const [k, v] of Object.entries(target.fields)) form.append(k, v);
  return request<T>(target.path, { method: "POST", body: form }, MULTIPART_UPLOAD_TIMEOUT_MS);
}

// ---- Views ----

export function createView(pid: string, name: string, file: File, opts?: UploadOptions): Promise<View> {
  return uploadImage(`/api/projects/${pid}/uploads`, file, opts, {
    path: `/api/projects/${pid}/views`,
    fields: { name },
  });
}

export function getView(pid: string, vid: string): Promise<View> {
  return request<View>(`/api/projects/${pid}/views/${vid}`);
}

export function deleteView(pid: string, vid: string): Promise<void> {
  return request<void>(`/api/projects/${pid}/views/${vid}`, { method: "DELETE" });
}

export function updateInventory(pid: string, vid: string, inventory: string): Promise<View> {
  return request<View>(`/api/projects/${pid}/views/${vid}/inventory`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ inventory }),
  });
}

export function generateInventory(
  pid: string,
  vid: string,
  language: string,
): Promise<{ inventory: string }> {
  return request<{ inventory: string }>(
    `/api/projects/${pid}/views/${vid}/inventory/generate?lang=${encodeURIComponent(language)}`,
    { method: "POST" },
    60_000,
  );
}

// ---- Masks ----

export function createMask(pid: string, vid: string, assetId?: string): Promise<Mask> {
  return request<Mask>(`/api/projects/${pid}/views/${vid}/masks`, json(assetId ? { assetId } : {}));
}

export function updateMask(
  pid: string,
  vid: string,
  mid: string,
  data: { assetId?: string | null; hidden?: boolean },
): Promise<Mask> {
  return request<Mask>(`/api/projects/${pid}/views/${vid}/masks/${mid}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export function uploadMaskBitmap(
  pid: string,
  vid: string,
  mid: string,
  blob: Blob,
): Promise<Mask> {
  const form = new FormData();
  form.append("file", blob, "mask.png");
  return request<Mask>(`/api/projects/${pid}/views/${vid}/masks/${mid}/bitmap`, {
    method: "PUT",
    body: form,
  });
}

export function deleteMask(pid: string, vid: string, mid: string): Promise<void> {
  return request<void>(`/api/projects/${pid}/views/${vid}/masks/${mid}`, { method: "DELETE" });
}

// ---- Render ----

// Renders now run as an async job: the POST enqueues one Cloud Task per
// variation and returns 202 with a RenderJob immediately, and the caller
// polls GET .../render-jobs/{jid} until the job reaches a terminal status.

// The server builds the view's object and material list first when it has none,
// and writes it in the UI language, so it is passed along.
function startRender(
  pid: string,
  vid: string,
  req: RenderRequest,
  language: string,
): Promise<RenderJob> {
  return request<RenderJob>(
    `/api/projects/${pid}/views/${vid}/render?lang=${encodeURIComponent(language)}`,
    json(req),
  );
}

function getRenderJob(pid: string, jid: string): Promise<RenderJob> {
  return request<RenderJob>(`/api/projects/${pid}/render-jobs/${jid}`);
}

const POLL_INTERVAL_MS = 3_000;
const POLL_TIMEOUT_MS = 15 * 60_000;
const MAX_CONSECUTIVE_POLL_FAILURES = 3;
// The edit request carries the painted regions as base64 PNGs, which takes
// longer to send than a render's small body.
const EDIT_START_TIMEOUT_MS = 60_000;

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new ApiError(0, "Render cancelled."));
      return;
    }
    const timer = setTimeout(resolve, ms);
    signal?.addEventListener(
      "abort",
      () => {
        clearTimeout(timer);
        reject(new ApiError(0, "Render cancelled."));
      },
      { once: true },
    );
  });
}

export interface RenderViewOptions {
  onProgress?: (job: RenderJob) => void;
  signal?: AbortSignal;
}

/**
 * Polls a job that was just started to completion, resolving to the finished
 * variations' Render[] (same shape callers got back when the endpoint was
 * synchronous). Polls every ~3s, tolerating a handful of consecutive
 * transient failures (network errors or 5xx) before giving up; a 4xx from a
 * poll is treated as fatal right away. Gives up after ~15 minutes overall.
 */
async function pollJob(
  pid: string,
  job: RenderJob,
  opts?: RenderViewOptions,
): Promise<Render[]> {
  opts?.onProgress?.(job);
  if (job.status === "done") return job.renders;
  if (job.status === "failed") throw new ApiError(502, job.error ?? "Render failed.");

  const deadline = Date.now() + POLL_TIMEOUT_MS;
  let consecutiveFailures = 0;
  while (true) {
    await sleep(POLL_INTERVAL_MS, opts?.signal);
    if (Date.now() > deadline) {
      throw new ApiError(0, "Render is taking too long. Please check back later.");
    }

    let polled: RenderJob;
    try {
      polled = await getRenderJob(pid, job.id);
    } catch (err) {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) throw err;
      consecutiveFailures++;
      if (consecutiveFailures > MAX_CONSECUTIVE_POLL_FAILURES) throw err;
      continue;
    }
    consecutiveFailures = 0;
    opts?.onProgress?.(polled);

    if (polled.status === "done") return polled.renders;
    if (polled.status === "failed") throw new ApiError(502, polled.error ?? "Render failed.");
  }
}

/** Starts a render job and polls it to completion - see pollJob. */
export async function renderView(
  pid: string,
  vid: string,
  req: RenderRequest,
  language: string,
  opts?: RenderViewOptions,
): Promise<Render[]> {
  return pollJob(pid, await startRender(pid, vid, req, language), opts);
}

/**
 * Edits an existing render: the regions (painted areas with an instruction
 * each) are changed and everything else is kept. Runs as a job like a render
 * and resolves to the one new Render, linked to its source.
 */
export async function editRender(
  pid: string,
  vid: string,
  rid: string,
  regions: EditRegionRequest[],
  opts?: RenderViewOptions,
): Promise<Render[]> {
  const job = await request<RenderJob>(
    `/api/projects/${pid}/views/${vid}/renders/${rid}/edit`,
    json({ regions }),
    EDIT_START_TIMEOUT_MS,
  );
  return pollJob(pid, job, opts);
}

/**
 * Makes a 4K version of an existing render, keeping its picture, colors and
 * lighting. Runs as a job like a render and resolves to the one new Render,
 * linked to its source.
 */
export async function upscaleRender(
  pid: string,
  vid: string,
  rid: string,
  opts?: RenderViewOptions,
): Promise<Render[]> {
  const job = await request<RenderJob>(
    `/api/projects/${pid}/views/${vid}/renders/${rid}/upscale`,
    { method: "POST" },
  );
  return pollJob(pid, job, opts);
}

// ---- Pricing ----

// Pricing barely changes and is the same for every user, so it's fetched at
// most once per page load and shared from here rather than refetched by
// every component that wants to show a cost preview.
let pricingPromise: Promise<PricingResponse> | null = null;

/** Estimated per-render cost for every model+resolution the UI offers. */
export function getPricing(): Promise<PricingResponse> {
  if (!pricingPromise) {
    pricingPromise = request<PricingResponse>("/api/pricing").catch((err) => {
      pricingPromise = null; // allow a retry on the next call
      throw err;
    });
  }
  return pricingPromise;
}
