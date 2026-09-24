# Persistence work - handoff

Status of the "add a database + per-project persistence + project selection"
work. **Implementation complete** across backend, frontend, and docs; nothing
committed yet (all changes are in the working tree). The one thing not done is a
live browser click-through with real Clerk auth - see "Remaining" at the bottom.

## Verified

- Backend: `go build`, `go vet`, `go test ./...` all pass (incl. new
  `internal/api/auth_test.go` covering the ownership gate: owner 200,
  non-owner/missing 404, auth-off pass-through).
- Backend E2E (memory mode, auth disabled): create / list / get / create-asset /
  upload-view / signed-URL / image-bytes all round-trip correctly, and the
  denormalized `viewCount` / `thumbnailImageId` update as expected.
- Frontend: `tsc -b` clean, `vite build` succeeds, `oxlint` clean (only
  pre-existing-style warnings that mirror the existing `useImage` hook).

## Decisions made (confirmed with the user)

- **Storage backend:** Firestore (structured data) + Google Cloud Storage
  (image blobs). Matches the existing Firebase/GCP + Cloud Run setup.
- **Scoping:** per-user private projects, keyed by Clerk `userId`. Orgs may come
  later - `Project.OrgID` exists (always `nil` today) so an org dimension can be
  added without a data migration. Do **not** implement orgs now.
- **Image access:** short-lived **V4 signed GCS URLs**. The browser loads images
  directly from GCS; the backend mints a signed URL only after the ownership
  gate passes. In local/dev (memory backend) the "signed URL" is just the
  same-origin `/api/images/{id}` path, so dev needs no GCS.

## What Firestore stores (final schema)

```
projects/{pid}                      core doc:
    ownerId, orgId(null), name, createdAt, updatedAt, style,
    assets[] (inline), styleAnchorRenderId,
    viewCount, renderCount, thumbnailImageId   (denormalized for the picker)
projects/{pid}/views/{vid}          name, screenshotImageId, hasScreenshot,
    width, height, masks[] (inline), inventory, createdAt
projects/{pid}/views/{vid}/renders/{rid}   the render record + metrics + preservation
```

Image bytes never go in Firestore - only blob IDs. A render's `isStyleAnchor`
is **derived at read time** from the project's `styleAnchorRenderId`, so setting
the anchor only writes the project doc. Views/renders are ordered by `createdAt`
sorted in Go (no composite Firestore index needed).

## GCS bucket

Flat objects keyed by blob ID (`gs://<BLOB_BUCKET>/<blobId>`), one object per
screenshot, asset reference photo, mask bitmap, and render result. Content type
stored as object metadata. Signing uses the IAM Credentials `SignBlob` API
(no private key on disk), cached per blob ID until ~5 min before expiry.

## DONE (backend - compiles, `go vet` clean, `go test ./...` passes)

Backend builds with `go 1.27.1`. The go toolchain here is at
`/Users/luizcarvalho/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.1.darwin-arm64/bin`
(no `go` on PATH by default - prepend that dir).

1. **Repository + BlobStore interfaces** - `internal/store/repository.go`.
   Two interfaces the whole API now depends on instead of the concrete store.
2. **In-memory impl** - `internal/store/store.go`: the old `Store` renamed
   `MemoryStore`, implements **both** interfaces (still the zero-config local-dev
   store). `New()` -> `NewMemory()`. `SignedURL` returns `/api/images/{id}`.
3. **Ownership** - `Project.OwnerID` / `OrgID` + `CreatedAt`/`UpdatedAt` added
   (`internal/store/types.go`). `CreateProject(ownerID, name)`. New
   `ListProjects(ownerID)`, `ProjectOwner(pid)`. New `requireOwner` middleware
   (`internal/api/auth.go`) composed as `requireAdmin(requireOwner(...))` on
   every `{pid}` route; non-owner/absent -> 404 (no existence leak). Skipped when
   auth is disabled (dev without `CLERK_SECRET_KEY`).
4. **List endpoint** - `GET /api/projects` -> `[]ProjectSummary`
   (`internal/api/projects.go`, wired in `internal/api/api.go`).
5. **Signed-URL endpoint** - `GET /api/projects/{pid}/images/{id}/url` ->
   `{ "url": "..." }` (ownership-gated via the `{pid}`). Public
   `GET /api/images/{id}` byte route is now **only registered for the in-memory
   backend** (`serveBlobsLocally`); with GCS it's omitted so private bytes are
   never served by blob UUID alone.
6. **FirestoreStore** - `internal/store/firestore.go`. Full `Repository` impl
   with transactions for read-modify-write, denormalized counters/thumbnail,
   `clearAssetFromMasks` on asset delete, blob-ID collection on view delete.
7. **GCSStore** - `internal/store/gcs.go`. Full `BlobStore` impl + V4 signing
   via IAM `SignBlob`, with a per-blob signed-URL cache.
8. **Storage selection** - `internal/config/config.go` new `storage:` section +
   env overrides `STORAGE` (`memory`|`firestore`, default `memory`),
   `BLOB_BUCKET`, `FIRESTORE_DATABASE`, `SIGNER_SERVICE_ACCOUNT`.
   `buildStorage()` in `cmd/server/main.go` wires memory or firestore+gcs.
9. `DeleteView`/`DeleteMask` now return/clean up orphaned blobs via the
   `BlobStore` (metadata store and blob store are decoupled).

New deps added to `go.mod`: `cloud.google.com/go/firestore`,
`cloud.google.com/go/storage`, `google.golang.org/api` (bumped).

## Frontend (DONE - builds, typechecks, lints)

- `state/ProjectContext.tsx`: now loads `GET /api/projects` on mount, holds the
  summary list + selection state, `selectProject` / `closeProject` /
  `refreshProjects`, remembers the last project in `localStorage`, and
  auto-opens it on return.
- `components/ProjectPicker.tsx` (+ `.css`): the picker screen (grid of project
  cards with thumbnails, counts, relative time; inline "New project" form;
  empty state). `App.tsx` shows it when no project is open and gained a "Switch
  project" button. `CreateProjectPanel.tsx` was **deleted** (superseded).
- `hooks/useSignedImageUrl.ts`: resolves a loadable URL for a blob (accepts an
  explicit `projectId` for picker thumbnails). `api.ts` gained
  `fetchSignedImageUrl` / `invalidateSignedImageUrl` (module-level cache, ~50min
  client TTL, re-signs before expiry) and `listProjects`; the old
  `imageUrl`/`maskBitmapUrl` were removed.
- All image consumers converted: `ViewsBar` (ViewThumb), `AssetCard`,
  `ResultView` (slider srcs + download), `MaskEditor` (screenshot via the hook,
  mask bitmaps resolved before `img.src`).

## Docs (DONE)

- `firestore.rules`: commented that `projects/**` is server-mediated via the
  Admin SDK and client access is denied on purpose.
- `backend/DEPLOY.md`: added Firestore DB + GCS bucket creation (with CORS for
  canvas reads), SA roles (`datastore.user`, `storage.objectAdmin`,
  `iam.serviceAccountTokenCreator` for signing), `STORAGE=firestore` /
  `BLOB_BUCKET` / `STORAGE_PROJECT` env, and rewrote the instance-scaling
  caveat (single-instance pin no longer needed with persistence).
- `API_CONTRACT.md` + `README.md`: updated for persistence, `ownerId`/`orgId`,
  `ProjectSummary`, `GET /api/projects`, and the signed-URL endpoint.
- Config decoupled: Firestore project is `STORAGE_PROJECT` (defaults to the
  ambient Cloud Run project via `firestore.DetectProjectID`), independent of the
  cross-project `GOOGLE_CLOUD_PROJECT` used for Vertex AI.

## Remaining

1. **Live browser click-through with real Clerk auth** - the only thing not
   exercised. The dev server has `CLERK_SECRET_KEY` set, so verifying the picker
   UI + signed-image loading in the actual signed-in app is a manual step
   (`pnpm --dir frontend dev` + backend). Everything compiles and the API is
   verified auth-disabled.
2. **Real Firestore/GCS run** - the FirestoreStore/GCSStore compile and are
   written to the client APIs, but haven't been run against live GCP here.
   First `STORAGE=firestore` deploy should sanity-check create/list/upload/
   render/signed-URL. Watch for: a needed Firestore composite index (shouldn't
   be - views/renders are sorted in Go), and bucket CORS for canvas reads.
3. **`DELETE /api/projects/{pid}`** - done. Owner-gated like every other
   `{pid}` route; soft delete only (`Project.DeletedAt`) - views, renders and
   blobs are left in place so a project is recoverable for 30 days. Every
   `Repository` method that loads a project doc (`ProjectOwner`, `GetProject`,
   `ListProjects`, and every mutator) treats a deleted project as not found;
   in `FirestoreStore` this goes through one shared `projectDocFromSnap`
   helper so a direct call can't bypass it even when the API's ownership gate
   is skipped (auth disabled). `ListProjects` filters deleted projects in Go
   rather than adding a Firestore `deletedAt` clause, to avoid a new composite
   index alongside the existing `ownerId` query. The picker has a delete
   button per card (`ProjectPicker.tsx`), confirmed via `window.confirm`,
   naming the project and the 30-day recovery window.
   **Not done:** a scheduled purge job to hard-delete projects whose
   `DeletedAt` is more than 30 days old (project doc + views/renders
   subcollections/docs + their blobs). TODO comments mark where it would go
   in `store.go` and `firestore.go`.
4. **Commit** - nothing is committed yet (as of this handoff; project
   soft-delete above is a separate follow-up commit on top).
