# Deploying render-ai-api to Cloud Run

The one-time infrastructure setup for a new Firebase/GCP project - enabling
APIs, creating the Firestore database and blob bucket, the `render-ai-api`
service account and its IAM grants, the cross-project Vertex AI binding, the
`CLERK_SECRET_KEY` secret container, and the backup infrastructure - is now
managed as Terraform. See
[`infra/terraform/README.md`](../infra/terraform/README.md) for:

- Bootstrapping the Terraform state bucket.
- Adopting the existing `render-ai-studio`/`labflux-project` setup with
  `import` blocks (first run).
- Recreating the whole stack in a new GCP project by changing variables.

This file (`backend/DEPLOY.md`) covers what Terraform does *not* do: shipping
code (step 5 below) and the restore runbook / drills for when a backup is
actually needed.

Vertex AI itself is NOT deployed by Terraform here in the app project by
default - it runs in the existing `labflux-project` (models and
billing/credits are enabled there), and the Cloud Run service is granted
cross-project access to it. See `vertex_project_id` in
`infra/terraform/variables.tf`.

## 5. Deploy

Once the infrastructure exists (`terraform apply` in `infra/terraform/`) and
the `CLERK_SECRET_KEY` secret has a value (see that README), ship code with:

```bash
./deploy.sh
```

`deploy.sh` only builds and pushes a new revision - it does not set env vars,
secrets, scaling, the service account, or the public-invoker binding.
Terraform owns all of that (`infra/terraform/cloudrun.tf`), and
`gcloud run deploy --source .` without those flags preserves the existing
service's configuration on the new revision. In short, it runs:

```bash
gcloud run deploy render-ai-api \
  --source . \
  --region us-central1 \
  --project render-ai-studio
```

Storage env vars (set by Terraform, not `deploy.sh` - documented here because
they're read by `internal/config/config.go`):

- `STORAGE=firestore` selects the Firestore + GCS backend (the default,
  `memory`, is in-process and lost on restart - local dev only).
- `BLOB_BUCKET` must match the bucket Terraform created
  (`infra/terraform/variables.tf`'s `blob_bucket_name`).
- `STORAGE_PROJECT` is optional and defaults to the **ambient Cloud Run
  project** (`render-ai-studio`) - which is where Terraform created the
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
so a single-instance pin is not necessary: requests can land on any instance
and see the same data, and a deploy or crash loses nothing. Scaling
(`cloud_run_min_instances` / `cloud_run_max_instances`, default `0`/`4`) is a
Terraform variable (`infra/terraform/variables.tf`), not a `deploy.sh` flag.

Two things to keep in mind when choosing those values:

- **Cold starts** include opening the Firestore and GCS clients. That's fast,
  but scaling to zero adds first-request latency.
- A render is a synchronous, long request (10-60s per variation). Keep the
  Cloud Run timeout generous (300s, set in `infra/terraform/cloudrun.tf`) and
  consider a small `cloud_run_min_instances` if you want to avoid cold starts
  on the render path.

If you use `STORAGE=memory` (not recommended in production), the old caveat
applies: pin `min_instances=1, max_instances=1`, because in-memory state is
per-instance and wiped on every restart.

## Backups and recovery

Data lives in exactly one place each: the Firestore database
(`projects`, `views`, `renders`) and the `__BLOB_BUCKET__` GCS bucket in
`us-central1` (screenshots, asset photos,
mask bitmaps, render images). Blobs are written once under a random blob ID
and never overwritten, except mask bitmaps, which are overwritten in place -
so a blob-level accident is either "one object is gone" or "one mask lost its
history," never a bulk corruption. The layers below go from "undo a mistake
in the next few weeks" to "recover even if the production project/SA is fully
compromised."

Targets: RPO <= 24h (daily backups + PITR cover anything more recent),
RTO on the order of a few hours (restores create a new Firestore database or
copy objects back - no data-loss-free way to do it faster).

All five layers below are created and configured by Terraform
(`infra/terraform/storage.tf`, `firestore.tf`, `backup.tf`, `export.tf`) -
see [`infra/terraform/README.md`](../infra/terraform/README.md) for setup.
What follows here is what Terraform does *not* automate: the operational
commands for actually listing/restoring backups, the restore runbook, the
quarterly drill checklist, and the cost note.

### 1. GCS soft delete on the blob bucket (30 days)

Protects against: an accidental or buggy delete/overwrite of a blob (e.g. a
`DeleteView`/`DeleteMask` bug, or someone deleting the wrong object by hand).
Soft delete keeps the previous bytes of a deleted or overwritten object
recoverable for a window, with no code changes needed.

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
each backup is an independent, longer-lived snapshot: **daily backups kept 14
days**, plus a **weekly (Sunday) backup kept 8 weeks** for a longer lookback
(`infra/terraform/firestore.tf`).

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
  `(default)` (only possible with delete protection off) and restore
  in-place with `--destination-database='(default)'`. In-place restore needs
  the original database deleted first and has a documented downtime window -
  prefer "point the service at it" unless you specifically need to keep the
  `(default)` id.

### 3. A separate backup project, isolated from the production service account

Protects against: the production project or the `render-ai-api` service
account being compromised or its keys leaked. If backups lived in the same
project under IAM the prod SA can reach, that compromise could also destroy
the backups. A separate project with its own IAM boundary means the prod SA
has to have **zero** access to reach them (`infra/terraform/backup.tf` never
grants it one). Spot-check the bucket's policy periodically:

```bash
gcloud storage buckets get-iam-policy gs://__BACKUP_BUCKET__ --project=__BACKUP_PROJECT_ID__
```

Optional: a **retention policy** (`backup_retention_days` in
`infra/terraform/variables.tf`), locked by hand, makes objects in the bucket
undeletable (even by a project owner) until the retention period elapses -
tamper-resistant against a compromised owner account, not just the prod SA.
Terraform will only ever create this policy unlocked (`is_locked = false`);
locking it is a deliberate, manual, irreversible step:

```bash
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
delete/overwrite access to. The job (`infra/terraform/backup.tf`) uses
`overwrite_when = "DIFFERENT"` (re-copies a blob only if its content changed
- covers mask bitmaps, the one blob type overwritten in place; every other
blob type is written once and never touched again) and deliberately never
deletes at either end, so a deletion in production is never propagated to the
backup copy.

### 5. Weekly Firestore managed export into the backup bucket

Protects against: the same isolated-copy need as step 4, but for Firestore
data, using the REST `exportDocuments` API on a schedule
(`infra/terraform/export.tf`, Sunday 06:00 UTC) so a restore doesn't depend
on PITR/backups still existing in the production project.

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
- [ ] Confirm the daily Storage Transfer Service job, the daily+weekly
      Firestore backup schedules, and the weekly Firestore export all ran
      successfully and recently (`gcloud transfer operations list`,
      `gcloud firestore backups list`, or just check the
      `.github/workflows/backup-check.yml` GitHub Actions run, which checks
      all three daily and is the first place to look if a drill turns up a
      gap).
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

At the app's current scale (roughly 400 GB of images in the blob bucket, ~2
GB of Firestore data), the whole backup stack - Coldline storage for the
blob copy, the daily Storage Transfer job, PITR + daily/weekly Firestore
backups, and the weekly export - comes out to roughly **US$4/month total**.
