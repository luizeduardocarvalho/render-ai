import type {
  Asset,
  ApiErrorBody,
  Mask,
  PricingResponse,
  Project,
  ProjectSummary,
  Render,
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

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
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
      try {
        const body = (await res.json()) as ApiErrorBody;
        if (body?.error) message = body.error;
      } catch {
        // ignore body parse failures, keep default message
      }
      throw new ApiError(res.status, message);
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

export function uploadAssetReference(pid: string, aid: string, file: File): Promise<Asset> {
  const form = new FormData();
  form.append("file", file);
  return request<Asset>(`/api/projects/${pid}/assets/${aid}/reference`, {
    method: "POST",
    body: form,
  });
}

// ---- Views ----

export function createView(pid: string, name: string, file: File): Promise<View> {
  const form = new FormData();
  form.append("file", file);
  form.append("name", name);
  return request<View>(`/api/projects/${pid}/views`, { method: "POST", body: form });
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

export function generateInventory(pid: string, vid: string): Promise<{ inventory: string }> {
  return request<{ inventory: string }>(
    `/api/projects/${pid}/views/${vid}/inventory/generate`,
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

export function renderView(pid: string, vid: string, req: RenderRequest): Promise<Render[]> {
  // Rendering is slow (10-60s per variation, server timeout ~180s for the
  // whole request); give it a generous client timeout too. The response is
  // always an array, one Render per requested (successful) variation.
  return request<Render[]>(
    `/api/projects/${pid}/views/${vid}/render`,
    json(req),
    180_000,
  );
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
