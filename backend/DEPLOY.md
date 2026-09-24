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
