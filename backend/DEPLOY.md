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
  --project=__GCP_PROJECT_ID__
```

- `run.googleapis.com` - Cloud Run itself.
- `cloudbuild.googleapis.com` - builds the Dockerfile when deploying with
  `--source .` (no manual `docker build`/`push`).
- `artifactregistry.googleapis.com` - Cloud Build pushes the built image
  here (Cloud Run source deploys use Artifact Registry, not the deprecated
  Container Registry).
- `secretmanager.googleapis.com` - stores `CLERK_SECRET_KEY`.

## 2. Create the render-ai-api service account

```bash
gcloud iam service-accounts create render-ai-api \
  --project=__GCP_PROJECT_ID__ \
  --display-name="render-ai-api Cloud Run service"
```

This gives you:
`render-ai-api@__GCP_PROJECT_ID__.iam.gserviceaccount.com`

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
  --set-env-vars GOOGLE_CLOUD_PROJECT=labflux-project,GOOGLE_CLOUD_LOCATION=global,GOOGLE_CLOUD_TEXT_LOCATION=us-central1 \
  --set-secrets CLERK_SECRET_KEY=CLERK_SECRET_KEY:latest \
  --service-account render-ai-api@__GCP_PROJECT_ID__.iam.gserviceaccount.com
```

Cloud Run injects `PORT`; the server binds to it automatically (see
`internal/config/config.go`), defaulting to 8080 only when `PORT` is unset.

## Caveat: min-instances=1, max-instances=1

`internal/store` is a plain in-memory store - there is no database. Pinning
`--min-instances=1 --max-instances=1` keeps exactly one instance running at
all times so:

- State (projects, assets, views, masks, rendered images) survives between
  requests instead of vanishing the moment Cloud Run scales to zero.
- Concurrent requests never land on two different instances with two
  different copies of the data.

The tradeoffs, which are acceptable for this PoC but not beyond it:

- No scale-to-zero: you pay for one always-on instance even with zero
  traffic.
- No horizontal scaling: a burst of concurrent requests is served by that
  single instance, not spread across replicas.
- Every deploy (new revision) or crash restarts the process and wipes all
  in-memory state - there is nothing to migrate, but users do lose
  in-progress projects.

Moving beyond a PoC means adding a real datastore and only then relaxing
these instance limits.
