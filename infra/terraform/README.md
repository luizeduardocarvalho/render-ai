# render-ai infrastructure (Terraform)

Two Terraform configs describe everything render-ai needs, so the whole setup
can be recreated in new GCP projects by changing variables:

- **`infra/bootstrap/`**, run once: the app and backup GCP projects, their
  billing link, and the bucket that stores the state of the main config.
- **`infra/terraform/`** (this directory), everything else:
  - `apis.tf`: APIs in each project.
  - `firestore.tf`: Firestore with PITR, delete protection, daily backups
    kept 14 days and weekly backups kept 8 weeks.
  - `storage.tf`: the image bucket with CORS, 30-day soft delete and uniform
    access.
  - `iam.tf`, `secrets.tf`, `cloudrun.tf`: the `render-ai-api` service
    account and roles (including Vertex AI on `vertex_project_id`), the
    `CLERK_SECRET_KEY` container, and the Cloud Run service's settings.
  - `tasks.tf`: the `render-jobs` Cloud Tasks queue (one task per render
    variation, `max_attempts = 1` because renders are billed) and the
    dedicated `render-tasks-invoker` service account Cloud Tasks uses to call
    the non-public `render-ai-worker` service's `/internal/render-tasks`
    route. `cloudrun.tf` defines that worker (same image as `render-ai-api`,
    one render per instance) and wires the queue and invoker into both
    services via the `RENDER_QUEUE`, `RENDER_TASKS_QUEUE`,
    `RENDER_WORKER_URL` and `RENDER_TASKS_INVOKER_SA` env vars.
  - `backup.tf`, `export.tf`: the Coldline backup bucket in the backup
    project, the daily Storage Transfer copy that never deletes, and the
    weekly Firestore export.
  - `firebase.tf`: Firebase on the app project and the `landing` and `app`
    Hosting sites.
  - `deploy.tf`: the Artifact Registry repository and the `github-deployer`
    service account used by `.github/workflows/deploy.yml`.
  - `github.tf`, `github_actions.tf`: Workload Identity Federation for
    GitHub Actions, the GitHub `production` environment (reviewers, main
    branch only), and every variable the deploy and backup-check workflows
    read. Nothing is copied into GitHub by hand.

## What stays outside Terraform, on purpose

- **The Clerk secret key's value.** Terraform creates the secret, but a
  value set through Terraform would sit in the state file in plain text. Add
  it with one `gcloud` command (below).
- **Deploying code.** The deploy workflow (or `backend/deploy.sh` and
  `firebase deploy` from a laptop) ships new images, Hosting content and
  `firestore.rules`. Terraform owns the Cloud Run settings and ignores the
  image (see the `lifecycle` block in `cloudrun.tf`).
- **Clerk's own settings** (allowed origins, production instance): Clerk
  dashboard, see the root `DEPLOY.md`.

## What you need before running it

- `gcloud` signed in (`gcloud auth application-default login`) as someone
  who can create projects, link billing, and set IAM on the Vertex AI project
  (`labflux-project` today).
- Terraform 1.6 or later.
- A **GitHub fine-grained token** for this repository with *Administration*,
  *Environments* and *Variables* set to read/write (plus the default
  *Metadata: read*). Export it as `GITHUB_TOKEN` when running
  `infra/terraform`. It is only used on your machine and is never stored.

## 1. Bootstrap (once)

```bash
cd infra/bootstrap
cp terraform.tfvars.example terraform.tfvars   # billing account, project ids, state bucket name
# Existing setup only: render-ai-studio already exists, so adopt it.
cp imports.tf.example imports.tf
terraform init
terraform apply
rm -f imports.tf
```

Bootstrap starts with local state, because the state bucket doesn't exist
until this apply. Keep `infra/bootstrap/terraform.tfstate` somewhere safe, or
move it into the new bucket: add `backend "gcs" {}` to the `terraform` block
in `main.tf`, then run
`terraform init -migrate-state -backend-config="bucket=<state bucket>" -backend-config="prefix=bootstrap"`.

## 2. Main config

```bash
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars   # project ids, buckets, CORS origins, Clerk publishable key, Hosting site ids, reviewers
export GITHUB_TOKEN=<fine-grained token>
terraform init -backend-config="bucket=render-ai-tfstate" -backend-config="prefix=render-ai"
```

**Adopting the existing `render-ai-studio` setup (first run only):**

1. `cp imports.tf.example imports.tf`. It imports what was created by hand:
   the Firestore database, image bucket, `render-ai-api` service account and
   roles, secret, Cloud Run service, Firebase project and Hosting sites.
2. `terraform plan`, and read it before applying. Existing resources must
   show as **import**, at most with in-place updates. New resources show as
   **create**: backups, the backup bucket, the transfer job, the export, the
   GitHub sign-in, the deployer account, the image repository and the GitHub
   environment and variables. If anything existing shows as **destroy** or
   **replace**, stop.
3. `terraform apply`, then `rm imports.tf`.

**A fresh setup** skips the imports: `terraform apply` creates everything.

## 3. After apply

1. Add the Clerk secret's value:
   ```bash
   printf '%s' "$CLERK_SECRET_KEY_VALUE" | gcloud secrets versions add \
     CLERK_SECRET_KEY --project=<app project> --data-file=-
   ```
2. Deploy: GitHub → Actions → **Deploy** → Run workflow → `both`. If
   `github_deploy_reviewers` is set, approve the run.

   **`terraform apply` must run before this the first time the render queue
   changes** (a fresh setup, or after editing `tasks.tf`/`cloudrun.tf`): the
   Deploy workflow only swaps the container image, it never sets environment
   variables - `RENDER_QUEUE`, `RENDER_TASKS_QUEUE`, `RENDER_WORKER_URL` and
   `RENDER_TASKS_INVOKER_SA` all come from this Terraform config. Order:
   `terraform apply` (creates/updates the queue, the invoker SA and its IAM,
   and the Cloud Run env vars) **then** run the Deploy workflow (ships the
   backend image that actually reads those env vars). Deploying the image
   first is harmless but the worker route won't have a queue to serve until
   Terraform has applied.
3. The **Backup check** workflow runs daily by itself once Terraform has set
   its variables. For 8 days after setup, a backup that doesn't exist yet
   (the Firestore export is weekly) is only a warning; after that, a missing,
   failed or stale backup fails the run and GitHub emails you.

## Recreating in new GCP projects

1. Bootstrap with the new project ids and a new state bucket name (no
   imports).
2. In `infra/terraform/terraform.tfvars`, set the new ids. Hosting site ids
   and bucket names must be globally unique. To run Vertex AI in the app
   project instead of `labflux-project`, set `vertex_project_id` to the app
   project (or leave it out).
3. Update `.firebaserc` so the `landing` and `app` targets point at the new
   project and site ids, and `firebase.json` if the region changes.
4. `terraform init` with the new state bucket, then `terraform apply` (no
   imports).
5. Add the Clerk secret value, update the Clerk dashboard's allowed origins
   for the new URLs, and run the Deploy workflow.
6. **Move data**, if this is a move rather than a fresh start:
   - Firestore: `gcloud firestore export gs://<bucket>/migration` in the old
     project, then `gcloud firestore import gs://<bucket>/migration` in the
     new one. The new project's Firestore service agent needs read access to
     that bucket.
   - Images: `gcloud storage cp -r "gs://<old-image-bucket>/*" gs://<new-image-bucket>/`.

## Notes

- `retention` on `google_firestore_backup_schedule` for a weekly recurrence
  tops out at 14 weeks; this config uses 8 weeks (`4838400s`).
- `google_project_service_identity` (`export.tf`) and the Firebase resources
  (`firebase.tf`) use the `google-beta` provider.
- The backup bucket's `retention_policy` is only ever created unlocked.
  Locking it can't be undone (see `backend/DEPLOY.md`, "Backups and
  recovery") and is left as a deliberate manual step.
- A few import IDs in `imports.tf.example` are marked "verify". If an import
  fails, `terraform plan` names the resource and the expected ID format.
