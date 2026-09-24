# Deploying render-ai-api to Cloud Run

One-time setup for a new Firebase/GCP project, followed by the actual
deploy. Replace `__GCP_PROJECT_ID__` everywhere below with the real project
id (the same placeholder used in `deploy.sh` and the Firebase Hosting
config).

Vertex AI itself is NOT deployed here - it already runs in the existing
`labflux-project` (models and billing/credits are enabled there). This new
project only hosts the Cloud Run service, which is granted cross-project
access to `labflux-project`'s Vertex AI.

## 0. Prerequisites

```bash
gcloud auth login
gcloud config set project __GCP_PROJECT_ID__
```

## 1. Enable required APIs on the new project

```bash
gcloud services enable \
  run.googleapis.com \
  cloudbuild.googleapis.com \
  artifactregistry.googleapis.com \
  secretmanager.googleapis.com \
  firestore.googleapis.com \
  storage.googleapis.com \
  iamcredentials.googleapis.com \
  --project=__GCP_PROJECT_ID__
```

- `run.googleapis.com` - Cloud Run itself.
- `cloudbuild.googleapis.com` - builds the Dockerfile when deploying with
  `--source .` (no manual `docker build`/`push`).
- `artifactregistry.googleapis.com` - Cloud Build pushes the built image
  here (Cloud Run source deploys use Artifact Registry, not the deprecated
  Container Registry).
- `secretmanager.googleapis.com` - stores `CLERK_SECRET_KEY`.
- `firestore.googleapis.com` - the projects/views/renders datastore.
- `storage.googleapis.com` - the GCS bucket that holds image blobs.
- `iamcredentials.googleapis.com` - used to sign GCS URLs (`SignBlob`) with the
  runtime service account, which carries no private key to sign with locally.

## 1b. Create the Firestore database and the blob bucket

Persistence lives in **Firestore** (structured data: projects, views, masks,
renders) plus a **GCS bucket** (image bytes: screenshots, asset reference
photos, mask bitmaps, render results). Both live in the new project (unlike
Vertex AI, which stays cross-project on `labflux-project`).

```bash
# Firestore in Native mode (one per project; pick a location, e.g. nam5 / us).
gcloud firestore databases create \
  --project=__GCP_PROJECT_ID__ \
  --location=nam5

# The blob bucket. The name must be globally unique; BLOB_BUCKET below must
# match it exactly. Uniform bucket-level access + no public access.
gcloud storage buckets create gs://__BLOB_BUCKET__ \
  --project=__GCP_PROJECT_ID__ \
  --location=us-central1 \
  --uniform-bucket-level-access \
  --public-access-prevention
```

The frontend reads render results and screenshots into a `<canvas>` (the mask
editor and before/after slider), so the bucket needs **CORS** allowing the app
origin, or those `crossOrigin` image loads taint the canvas / fail. Apply it:

```bash
cat > /tmp/cors.json <<'JSON'
[{ "origin": ["https://__APP_SITE_ID__.web.app", "http://localhost:5173"],
   "method": ["GET"],
   "responseHeader": ["Content-Type"],
   "maxAgeSeconds": 3600 }]
JSON
gcloud storage buckets update gs://__BLOB_BUCKET__ --cors-file=/tmp/cors.json
```

## 2. Create the render-ai-api service account

```bash
gcloud iam service-accounts create render-ai-api \
  --project=__GCP_PROJECT_ID__ \
  --display-name="render-ai-api Cloud Run service"
```

This gives you:
`render-ai-api@__GCP_PROJECT_ID__.iam.gserviceaccount.com`

## 2b. Grant the service account persistence + signing roles

```bash
SA=render-ai-api@__GCP_PROJECT_ID__.iam.gserviceaccount.com

# Firestore read/write.
gcloud projects add-iam-policy-binding __GCP_PROJECT_ID__ \
  --member="serviceAccount:${SA}" --role="roles/datastore.user"

# Read/write/delete blobs in the bucket (scope to the bucket, not the project).
gcloud storage buckets add-iam-policy-binding gs://__BLOB_BUCKET__ \
  --member="serviceAccount:${SA}" --role="roles/storage.objectAdmin"

# Let the SA sign GCS URLs as itself via the IAM SignBlob API. This binding is
# on the service account resource, with the SA as both principal and target.
gcloud iam service-accounts add-iam-policy-binding "${SA}" \
  --project=__GCP_PROJECT_ID__ \
  --member="serviceAccount:${SA}" \
  --role="roles/iam.serviceAccountTokenCreator"
```

## 3. Grant it Vertex AI access - CROSS-PROJECT, on labflux-project

This is the important cross-project step: the IAM binding is added to
`labflux-project` (where Vertex AI is enabled), not to the new project,
naming the new project's service account as the principal.

```bash
gcloud projects add-iam-policy-binding labflux-project \
  --member="serviceAccount:render-ai-api@__GCP_PROJECT_ID__.iam.gserviceaccount.com" \
  --role="roles/aiplatform.user"
```

You need permission to modify IAM policy on `labflux-project` to run this
(e.g. `roles/resourcemanager.projectIamAdmin` or `roles/owner` there).

## 4. Create the CLERK_SECRET_KEY secret and grant access

```bash
# Create the secret (prompts for the value on stdin - avoids it landing in
# shell history). Use the Clerk *secret* key (sk_live_... or sk_test_...),
# not the publishable key.
printf '%s' "$CLERK_SECRET_KEY_VALUE" | gcloud secrets create CLERK_SECRET_KEY \
  --project=__GCP_PROJECT_ID__ \
  --data-file=- \
  --replication-policy=automatic

# Grant the service account read access to it.
gcloud secrets add-iam-policy-binding CLERK_SECRET_KEY \
  --project=__GCP_PROJECT_ID__ \
  --member="serviceAccount:render-ai-api@__GCP_PROJECT_ID__.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

If the secret already exists and you're rotating the value:

```bash
printf '%s' "$NEW_CLERK_SECRET_KEY_VALUE" | gcloud secrets versions add CLERK_SECRET_KEY \
  --project=__GCP_PROJECT_ID__ \
  --data-file=-
```

`deploy.sh` always mounts `:latest`, so a new version is picked up on the
next deploy (or Cloud Run revision restart) with no other changes needed.

## 5. Deploy

```bash
./deploy.sh
```

See the comments in `deploy.sh` for what each flag does. In short, it runs:

```bash
gcloud run deploy render-ai-api \
  --source . \
  --region us-central1 \
  --project __GCP_PROJECT_ID__ \
  --allow-unauthenticated \
  --min-instances=1 --max-instances=1 \
  --timeout=300 \
  --set-env-vars GOOGLE_CLOUD_PROJECT=labflux-project,GOOGLE_CLOUD_LOCATION=global,GOOGLE_CLOUD_TEXT_LOCATION=us-central1,STORAGE=firestore,BLOB_BUCKET=__BLOB_BUCKET__ \
  --set-secrets CLERK_SECRET_KEY=CLERK_SECRET_KEY:latest \
  --service-account render-ai-api@__GCP_PROJECT_ID__.iam.gserviceaccount.com
```

Storage env vars:

- `STORAGE=firestore` selects the Firestore + GCS backend (the default,
  `memory`, is in-process and lost on restart - local dev only).
- `BLOB_BUCKET` must match the bucket created in step 1b.
- `STORAGE_PROJECT` is optional and defaults to the **ambient Cloud Run
  project** (`__GCP_PROJECT_ID__`) - which is where step 1b created the
  Firestore DB and bucket. It is deliberately independent of
  `GOOGLE_CLOUD_PROJECT` (which stays `labflux-project` for cross-project
  Vertex AI). Set it only if Firestore lives in a different project than the
  one Cloud Run runs in.
- `FIRESTORE_DATABASE` is optional (defaults to the project's `(default)`
  database).
- `SIGNER_SERVICE_ACCOUNT` is optional - the signer email is auto-detected from
  the runtime service account on Cloud Run.

Cloud Run injects `PORT`; the server binds to it automatically (see
`internal/config/config.go`), defaulting to 8080 only when `PORT` is unset.

## Instance scaling

With `STORAGE=firestore`, state lives in Firestore + GCS, not in the process,
so the single-instance pin the in-memory PoC required is no longer necessary:
requests can land on any instance and see the same data, and a deploy or crash
loses nothing. You can drop `--min-instances=1 --max-instances=1` from
`deploy.sh` and let Cloud Run scale (including to zero) normally.

Two things to keep in mind if you do:

- **Cold starts** now include opening the Firestore and GCS clients. That's
  fast, but scaling to zero adds first-request latency.
- A render is a synchronous, long request (10-60s per variation). Keep the
  Cloud Run `--timeout` generous (300s) and consider a small `--min-instances`
  if you want to avoid cold starts on the render path.

If you keep `STORAGE=memory` (not recommended in production), the old caveat
still applies: pin `--min-instances=1 --max-instances=1`, because in-memory
state is per-instance and wiped on every restart.

## Backups and recovery

Nothing below is configured by default. Data lives in exactly one place each:
the Firestore database created in step 1b (`projects`, `views`, `renders`) and
the `__BLOB_BUCKET__` GCS bucket in `us-central1` (screenshots, asset photos,
mask bitmaps, render images). Blobs are written once under a random blob ID
and never overwritten, except mask bitmaps, which are overwritten in place -
so a blob-level accident is either "one object is gone" or "one mask lost its
history," never a bulk corruption. The layers below go from "undo a mistake
in the next few weeks" to "recover even if the production project/SA is fully
compromised."

Targets: RPO <= 24h (daily backups + PITR cover anything more recent),
RTO on the order of a few hours (restores create a new Firestore database or
copy objects back - no data-loss-free way to do it faster).

### 1. GCS soft delete on the blob bucket (30 days)

Protects against: an accidental or buggy delete/overwrite of a blob (e.g. a
`DeleteView`/`DeleteMask` bug, or someone deleting the wrong object by hand).
Soft delete keeps the previous bytes of a deleted or overwritten object
recoverable for a window, with no code changes needed.

```bash
gcloud storage buckets update gs://__BLOB_BUCKET__ \
  --project=__GCP_PROJECT_ID__ \
  --soft-delete-duration=30d
```

List and restore soft-deleted objects:

```bash
# List every soft-deleted object/version in the bucket.
gcloud storage ls gs://__BLOB_BUCKET__/** --soft-deleted

# List soft-deleted versions of one blob.
gcloud storage ls gs://__BLOB_BUCKET__/<blobId> --soft-deleted

# Restore the most recent soft-deleted version of a blob.
gcloud storage restore gs://__BLOB_BUCKET__/<blobId>

# Restore a specific generation (from the ls output above).
gcloud storage restore "gs://__BLOB_BUCKET__/<blobId>#<generation>"
```

Soft-deleted bytes are billed as normal storage for the retention window, so
this raises the bucket's baseline storage cost slightly - see the cost note
at the end of this section.

### 2. Firestore PITR, delete protection, and scheduled backups

Protects against: **PITR** - an accidental or buggy write/delete anywhere in
the database, recoverable to any minute in the last 7 days. **Delete
protection** - someone (or some script) running `gcloud firestore databases
delete` against the production database. **Scheduled backups** - anything
PITR's 7-day window doesn't reach, or a Firestore-service-level incident;
each backup is an independent, longer-lived snapshot.

```bash
gcloud firestore databases update \
  --project=__GCP_PROJECT_ID__ \
  --database='(default)' \
  --enable-pitr \
  --delete-protection
```

(Use `--database=__FIRESTORE_DATABASE__` instead of `(default)` if
`FIRESTORE_DATABASE` is set to a non-default database id - see step 1b/5.)

Daily scheduled backups. `--retention` currently accepts up to 14 weeks
(`8467200s`); verify the current cap with
`gcloud firestore backups schedules create --help` before relying on it, as
it has changed before. 6 weeks comfortably covers a monthly billing-cycle
lookback without pushing the limit:

```bash
gcloud firestore backups schedules create \
  --project=__GCP_PROJECT_ID__ \
  --database='(default)' \
  --recurrence=daily \
  --retention=6w
```

List what exists:

```bash
gcloud firestore backups schedules list --project=__GCP_PROJECT_ID__ --database='(default)'
gcloud firestore backups list --project=__GCP_PROJECT_ID__ --location=nam5
```

**Restore from PITR.** This is a beta command and always creates a *new*
database (it can't restore into the live one in place):

```bash
gcloud beta firestore databases clone \
  --project=__GCP_PROJECT_ID__ \
  --source-database="projects/__GCP_PROJECT_ID__/databases/(default)" \
  --snapshot-time="2026-09-20T10:20:00Z" \
  --destination-database=restore-pitr-20260920
```

**Restore from a scheduled backup** - also always a new database:

```bash
gcloud firestore databases restore \
  --project=__GCP_PROJECT_ID__ \
  --source-backup="projects/__GCP_PROJECT_ID__/locations/nam5/backups/<backupId>" \
  --destination-database=restore-backup-20260920
```

Either way you now have data in a *new* database id, not `(default)`. Two
ways to make the service see it:

- **Point the service at it** (fast, no downtime): redeploy with
  `FIRESTORE_DATABASE=restore-pitr-20260920` (see step 5's env vars). Good for
  verifying a restore, or as the new production database while you sort out
  the original.
- **Copy the data back into `(default)`** (keeps `FIRESTORE_DATABASE` unset
  long-term): export the restored database with `gcloud firestore export` and
  import it into `(default)` with `gcloud firestore import`, or delete
  `(default)` (only possible with `--delete-protection` off) and restore
  in-place with `--destination-database='(default)'`. In-place restore needs
  the original database deleted first and has a documented downtime window -
  prefer "point the service at it" unless you specifically need to keep the
  `(default)` id.

### 3. A separate backup project, isolated from the production service account

Protects against: the production project or the `render-ai-api` service
account being compromised or its keys leaked. If backups lived in the same
project under IAM the prod SA can reach, that compromise could also destroy
the backups. A separate project with its own IAM boundary means the prod SA
has to have **zero** access to reach them.

```bash
gcloud projects create __BACKUP_PROJECT_ID__ --name="render-ai backups"
gcloud services enable storage.googleapis.com storagetransfer.googleapis.com \
  --project=__BACKUP_PROJECT_ID__

# Coldline: backups are written once a day and read back only for a restore,
# so infrequent-access pricing is the right tradeoff (see the cost note below).
gcloud storage buckets create gs://__BACKUP_BUCKET__ \
  --project=__BACKUP_PROJECT_ID__ \
  --location=us-central1 \
  --default-storage-class=COLDLINE \
  --uniform-bucket-level-access \
  --public-access-prevention
```

Do **not** grant `render-ai-api@__GCP_PROJECT_ID__.iam.gserviceaccount.com`,
or any other production principal, any role on `__BACKUP_PROJECT_ID__` or
`gs://__BACKUP_BUCKET__` - IAM is deny-by-default, so simply never adding a
binding is the control. Only the Storage Transfer Service agent (step 4) and
the Firestore export service account (step 5) get write access, and both are
scoped to this one bucket. Spot-check the bucket's policy periodically:

```bash
gcloud storage buckets get-iam-policy gs://__BACKUP_BUCKET__ --project=__BACKUP_PROJECT_ID__
```

Optional: a **retention policy**, locked, makes objects in the bucket
undeletable (even by a project owner) until the retention period elapses -
tamper-resistant against a compromised owner account, not just the prod SA.

```bash
gcloud storage buckets update gs://__BACKUP_BUCKET__ \
  --project=__BACKUP_PROJECT_ID__ \
  --retention-period=90d

# Only once you are certain - this cannot be undone or shortened, ever:
gcloud storage buckets update gs://__BACKUP_BUCKET__ \
  --project=__BACKUP_PROJECT_ID__ \
  --lock-retention-period
```

Caveat: locking is irreversible - you can raise the retention period later
but never lower or remove it, and you can't delete the bucket (or any
unexpired object in it) until every object ages past the retention period.
Treat this as a deliberate, tested decision, not a default to flip on.

### 4. Daily Storage Transfer Service copy of the blob bucket

Protects against: the same class of incident as step 3, but for GCS objects -
a copy that production (and anything that can reach `__BLOB_BUCKET__`) has no
delete/overwrite access to.

Grant the Storage Transfer Service agent (project-number-scoped, created the
first time the API is used) read on the source and write on the destination:

```bash
PROJECT_NUMBER=$(gcloud projects describe __BACKUP_PROJECT_ID__ --format='value(projectNumber)')
STS_SA="project-${PROJECT_NUMBER}@storage-transfer-service.iam.gserviceaccount.com"

# Read the source bucket (production project).
gcloud storage buckets add-iam-policy-binding gs://__BLOB_BUCKET__ \
  --project=__GCP_PROJECT_ID__ \
  --member="serviceAccount:${STS_SA}" \
  --role="roles/storage.objectViewer"
gcloud storage buckets add-iam-policy-binding gs://__BLOB_BUCKET__ \
  --project=__GCP_PROJECT_ID__ \
  --member="serviceAccount:${STS_SA}" \
  --role="roles/storage.legacyBucketReader"

# Write the destination bucket (backup project).
gcloud storage buckets add-iam-policy-binding gs://__BACKUP_BUCKET__ \
  --project=__BACKUP_PROJECT_ID__ \
  --member="serviceAccount:${STS_SA}" \
  --role="roles/storage.legacyBucketWriter"
gcloud storage buckets add-iam-policy-binding gs://__BACKUP_BUCKET__ \
  --project=__BACKUP_PROJECT_ID__ \
  --member="serviceAccount:${STS_SA}" \
  --role="roles/storage.objectViewer"
```

Create the daily job, owned by the backup project. The destination path
(`blobs/`) is just a prefix on the `DESTINATION` argument, not a separate
flag. Deliberately **omit `--delete-from`**: its only two values delete
objects from the destination or the source, and omitting it entirely means
the transfer only ever adds/overwrites objects at the destination - a
deletion in production is never propagated to the backup copy.

```bash
gcloud transfer jobs create \
  gs://__BLOB_BUCKET__ gs://__BACKUP_BUCKET__/blobs/ \
  --project=__BACKUP_PROJECT_ID__ \
  --name=render-ai-blob-backup \
  --schedule-starts=2026-09-25T07:00:00Z \
  --schedule-repeats-every=1d \
  --overwrite-when=different
```

`--overwrite-when=different` re-copies a blob only if its content changed
(covers mask bitmaps, the one blob type that's overwritten in place); every
other blob type is written once and never touched again.

### 5. Daily Firestore managed export into the backup bucket

Protects against: the same isolated-copy need as step 4, but for Firestore
data, using the REST `exportDocuments` API on a schedule so a restore doesn't
depend on PITR/backups still existing in the production project.

```bash
gcloud iam service-accounts create render-ai-firestore-exporter \
  --project=__GCP_PROJECT_ID__ \
  --display-name="Scheduled Firestore export"

EXPORT_SA=render-ai-firestore-exporter@__GCP_PROJECT_ID__.iam.gserviceaccount.com

# Read access to Firestore in the production project.
gcloud projects add-iam-policy-binding __GCP_PROJECT_ID__ \
  --member="serviceAccount:${EXPORT_SA}" \
  --role="roles/datastore.importExportAdmin"

# Write access to the backup bucket only (nothing else in that project).
gcloud storage buckets add-iam-policy-binding gs://__BACKUP_BUCKET__ \
  --project=__BACKUP_PROJECT_ID__ \
  --member="serviceAccount:${EXPORT_SA}" \
  --role="roles/storage.objectCreator"

gcloud scheduler jobs create http render-ai-firestore-export \
  --project=__GCP_PROJECT_ID__ \
  --location=us-central1 \
  --schedule="0 6 * * *" \
  --uri="https://firestore.googleapis.com/v1/projects/__GCP_PROJECT_ID__/databases/(default):exportDocuments" \
  --http-method=post \
  --oauth-service-account-email="${EXPORT_SA}" \
  --oauth-token-scope="https://www.googleapis.com/auth/datastore" \
  --message-body='{"outputUriPrefix":"gs://__BACKUP_BUCKET__/firestore-exports"}'
```

Documented alternative: a small Cloud Run job (or Cloud Function) that runs
`gcloud firestore export gs://__BACKUP_BUCKET__/firestore-exports` on its own
schedule (e.g. via Cloud Scheduler triggering the job) instead of calling the
REST API directly - simpler to read, at the cost of one more deployed thing.

### 6. Restore runbook

**Restore one user's deleted asset/render blob** (most common case, minutes):

1. Get the blob ID from the render/asset record (or from the user's report).
2. Check GCS soft delete first - it's newest and fastest:
   `gcloud storage ls gs://__BLOB_BUCKET__/<blobId> --soft-deleted`, then
   `gcloud storage restore gs://__BLOB_BUCKET__/<blobId>`.
3. If it's aged out of the 30-day soft-delete window, copy it back from the
   backup bucket instead:
   `gcloud storage cp gs://__BACKUP_BUCKET__/blobs/<blobId> gs://__BLOB_BUCKET__/<blobId>`.

**Restore the whole Firestore database** (incident/corruption case, hours):

1. Decide the target: a recent bad write/delete -> PITR (step 2's `clone`
   command); older, or PITR unavailable -> the latest daily backup or export.
2. Restore into a **new** database id (never directly into `(default)`).
3. Sanity-check the restored database (spot-check a few `projects` docs and
   their `views`/`renders` subcollections) before cutting over.
4. Point the service at it via `FIRESTORE_DATABASE=<restored-db-id>` and
   redeploy (step 5's env vars), or copy the data back into `(default)` per
   step 2's two options.
5. Once confirmed good, decide whether to keep the restored database as the
   new production one or migrate back to `(default)` - don't leave both
   receiving traffic.

**Quarterly restore drill checklist:**

- [ ] Restore a GCS blob from the backup bucket (step 3 above) into a scratch
      object, not back into `__BLOB_BUCKET__`, and diff it against a known-good
      copy.
- [ ] Clone the production database via PITR into a scratch database id,
      confirm a handful of documents look right, then delete the scratch
      database.
- [ ] Restore the latest scheduled backup into a scratch database id and
      confirm it matches the PITR clone for the same day.
- [ ] Confirm the daily Storage Transfer Service job and the Firestore export
      Cloud Scheduler job both ran successfully in the last 24h
      (`gcloud transfer operations list`, `gcloud scheduler jobs describe
      render-ai-firestore-export`).
- [ ] Confirm the production service account still has no IAM bindings on
      `__BACKUP_PROJECT_ID__` (see step 3's `get-iam-policy` check).
- [ ] Record how long each restore took, against the RPO/RTO targets above.

### Rough cost note

Coldline storage is much cheaper per GB-month than Standard, but has a
**90-day minimum storage duration** (deleting or overwriting an object before
90 days still bills for the full 90) and per-GB **retrieval fees** - fine for
a write-once, rarely-read backup copy, wrong for anything read back
regularly. PITR and scheduled backups are billed on the storage they hold
(PITR as extra retained versions, backups per backup-GB-month), on top of the
live database's normal read/write/storage pricing - both scale with total
data volume, not request volume, so they stay cheap while the app is small
and are worth re-checking as it grows.
