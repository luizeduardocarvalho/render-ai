# render-ai infrastructure (Terraform)

This directory manages the render-ai GCP infrastructure as code, so it can be
reproduced in a different GCP project by changing variables:

- The `render-ai-api` Cloud Run service (`cloudrun.tf`), its service account
  and IAM grants (`iam.tf`), the Clerk secret container (`secrets.tf`).
- The Firestore database, PITR, delete protection, and backup schedules
  (`firestore.tf`).
- The blob bucket, with CORS, soft delete, and uniform access
  (`storage.tf`).
- A separate backup project: a Coldline bucket, a daily Storage Transfer
  job copying the blob bucket into it, and read-only IAM scoped to that
  bucket only (`backup.tf`).
- A weekly Firestore managed export into the backup bucket, via Cloud
  Scheduler (`export.tf`).
- Workload Identity Federation for the GitHub Actions backup-check workflow,
  plus its read-only service account (`github.tf`).

## What this does NOT manage

- **Firebase Hosting** (the `landing` and `app` sites, `.firebaserc`
  targets, and `firestore.rules` deploys) - these stay with the `firebase`
  CLI. See the root [`DEPLOY.md`](../../DEPLOY.md).
- **Application code deploys** - `backend/deploy.sh` ships the Cloud Run
  container image; Terraform only owns the service's configuration (env
  vars, scaling, SA, secret wiring) and deliberately ignores drift on the
  image/client fields it doesn't touch (see the `lifecycle` block in
  `cloudrun.tf`).
- **Secret values** - `secrets.tf` creates the `CLERK_SECRET_KEY` container
  only. The value is added with `gcloud` (below), never with Terraform, so
  it never enters `.tfstate` or a plan/apply log.
- **Clerk configuration itself** (allowed origins, production instance
  setup) - see the root `DEPLOY.md`.

## Bootstrap: the Terraform state bucket

State needs somewhere to live before `terraform init` can run. Create a
small, versioned GCS bucket for it - the backup project is a reasonable home
(it already has the tightest IAM boundary, and state contains no secret
values, only resource metadata):

```bash
gcloud storage buckets create gs://render-ai-tfstate \
  --project=render-ai-backups \
  --location=us-central1 \
  --uniform-bucket-level-access \
  --public-access-prevention
gcloud storage buckets update gs://render-ai-tfstate --versioning
```

Then initialize with that bucket via `-backend-config` (the `backend "gcs"
{}` block in `versions.tf` is intentionally empty/partial so the same config
works for any project):

```bash
terraform init \
  -backend-config="bucket=render-ai-tfstate" \
  -backend-config="prefix=render-ai"
```

## First run: adopting the existing render-ai-studio project

The real `render-ai-studio` / `labflux-project` setup already exists,
created by hand via the steps in `backend/DEPLOY.md`'s history. To bring it
under Terraform without recreating anything:

1. `cp terraform.tfvars.example terraform.tfvars` and fill in the real
   values (`render-ai-studio`, `labflux-project`, `render-ai-studio-images`,
   your chosen `backup_project_id`/`backup_bucket_name`, the real CORS
   origins).
2. `cp imports.tf.example imports.tf` - it has `import {}` blocks for every
   resource that already exists (Firestore db, blob bucket, the SA, its IAM
   bindings, the secret container, the Cloud Run service). Read the comments
   in it; a few resources (enabled APIs, the Cloud Run invoker binding)
   are called out as optional or needing a version check.
3. `terraform init` (see above), then `terraform plan`. Review carefully:
   imported resources should show as updates-in-place at most (e.g. adding a
   `lifecycle` block), never destroy/recreate. Resources with no import
   block (the backup project's contents, the GitHub WIF pool, the export
   scheduler) will show as **new** - that's expected, none of that exists
   yet.
4. `terraform apply`.
5. Delete `imports.tf` - it has done its job, and leaving it in place makes
   every future `terraform apply` re-attempt the same imports.
6. Add the Clerk secret's value (Terraform never touches this):
   ```bash
   printf '%s' "$CLERK_SECRET_KEY_VALUE" | gcloud secrets versions add \
     CLERK_SECRET_KEY --project=render-ai-studio --data-file=-
   ```
7. Set the GitHub Actions repo variables the backup-check workflow needs
   (from `terraform output`) - see
   [`../../.github/workflows/backup-check.yml`](../../.github/workflows/backup-check.yml)
   and its section below. Run the workflow once by hand (Actions -> Backup
   check -> Run workflow); when it passes, uncomment its `schedule:` block so
   it runs daily.

## Recreating in a new GCP project

1. Create the two projects (Terraform does not create projects, only
   configures resources inside them):
   ```bash
   gcloud projects create <new-app-project-id>
   gcloud projects create <new-backup-project-id>
   ```
   Link both to a billing account, and grant your Terraform principal
   `roles/owner` (or the narrower set: project IAM admin, service usage
   admin, etc.) on each. If Vertex AI should also run in the new project
   rather than staying cross-project on `labflux-project`, set
   `vertex_project_id` to the same value as `app_project_id` (or omit it).
2. New `terraform.tfvars` (or a new `-backend-config` prefix + tfvars for a
   fully separate state), no `imports.tf` this time - everything is created
   fresh.
3. `terraform init -backend-config=... ; terraform apply`.
4. Add the Clerk secret value (step 6 above).
5. Deploy the application code: `cd backend && ./deploy.sh` (with `PROJECT`/
   `REGION` env vars if not using the defaults - see `backend/deploy.sh`).
6. Point Firebase Hosting's `app` site's `/api/**` rewrite at the new
   region/service if it differs (`firebase.json`), and follow the root
   `DEPLOY.md` for the Hosting sites themselves.
7. **Migrate data**, if this is a move rather than a fresh start:
   - Firestore: `gcloud firestore export gs://<old-bucket>/migration` against
     the old project, then `gcloud firestore import
     gs://<old-bucket>/migration` against the new project's database (grant
     the new project's default compute/Firestore service agent read access
     to the export bucket first, or copy the export into a bucket the new
     project can read).
   - Blobs: `gcloud storage cp -r gs://<old-blob-bucket>/**
     gs://<new-blob-bucket>/`.

## GitHub Actions backup-check

`.github/workflows/backup-check.yml` verifies daily that the blob transfer,
Firestore backup schedule, and weekly export are all actually running, using
Workload Identity Federation (`github.tf`) - no service account key is
stored in GitHub. After `terraform apply`, set these **repository
variables** (Settings -> Secrets and variables -> Actions -> Variables), not
secrets - none of them are sensitive:

| Repo variable | From |
| --- | --- |
| `GCP_WIF_PROVIDER` | `terraform output -raw workload_identity_provider` |
| `GCP_BACKUP_CHECKER_SA` | `terraform output -raw backup_checker_service_account_email` |
| `GCP_APP_PROJECT` | `app_project_id` (e.g. `render-ai-studio`) |
| `GCP_BACKUP_PROJECT` | `backup_project_id` |
| `GCP_TRANSFER_JOB` | `terraform output -raw transfer_job_name` |
| `GCP_BACKUP_BUCKET` | `terraform output -raw backup_bucket_name` |
| `GCP_FIRESTORE_LOCATION` | `firestore_location` (e.g. `nam5`) - optional, defaults to `nam5` in the workflow |

## Notes / things to double check before relying on this

- `retention` on `google_firestore_backup_schedule` for a weekly recurrence
  tops out at 14 weeks per the provider docs; this config uses 8 weeks
  (`4838400s`), comfortably under that.
- `google_project_service_identity` (used in `export.tf` to get the
  Firestore service agent) is a `google-beta`-only resource.
- The backup bucket's `retention_policy` is only ever created with
  `is_locked = false`. Locking it is a one-way door (see
  `backend/DEPLOY.md`'s "Backups and recovery" section) and is left as a
  manual, deliberate `gcloud storage buckets update --lock-retention-period`
  step, never something `terraform apply` can do.

## GitHub Actions deploy

`.github/workflows/deploy.yml` deploys by hand from GitHub (Actions -> Deploy
-> Run workflow, choosing backend, frontend or both). It builds the backend
image, pushes it to the Artifact Registry repository in `deploy.tf`, rolls it
out to Cloud Run (only the image changes), then builds the frontend and runs
`firebase deploy --only hosting,firestore`.

It signs in as the `github-deployer` service account (`deploy.tf`), which
only jobs in this repo's GitHub **`production` environment** can impersonate.
After `terraform apply`:

1. GitHub -> Settings -> Environments -> New environment `production`.
   Optionally add yourself under *Required reviewers* so every deploy needs
   an approval click.
2. Add these **environment variables** (not secrets):

| Variable | From |
| --- | --- |
| `GCP_WIF_PROVIDER` | `terraform output -raw workload_identity_provider` |
| `GCP_DEPLOYER_SA` | `terraform output -raw deployer_service_account_email` |
| `GCP_ARTIFACT_REPO` | `terraform output -raw artifact_registry_repository` |
| `GCP_PROJECT` | the app project id, e.g. `render-ai-studio` |
| `GCP_REGION` | `us-central1` |
| `VITE_CLERK_PUBLISHABLE_KEY` | Clerk dashboard (publishable key, not the secret key) |

`backend/deploy.sh` still works for deploys from a laptop.
