#!/usr/bin/env bash
# Deploys the render-ai backend to Cloud Run.
#
# Prerequisites (see DEPLOY.md for the full one-time setup):
#   - gcloud CLI authenticated (`gcloud auth login`) with a config pointed at
#     the target Firebase/GCP project.
#   - The APIs, service account, IAM bindings, and CLERK_SECRET_KEY secret
#     described in DEPLOY.md already exist.
#
# Replace render-ai-studio below with the actual (new, dedicated) Firebase
# project id before running this script.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

# --source .
#   Build straight from this directory's Dockerfile via Cloud Build - no
#   container registry or separate `docker build`/`push` step to manage.
#
# --region us-central1
#   Must match the region the hosting side expects for render-ai-api.
#
# --project render-ai-studio
#   The new, dedicated Firebase/GCP project (NOT labflux-project - Vertex AI
#   stays in labflux-project and is granted cross-project below).
#
# --allow-unauthenticated
#   Public HTTP endpoint. Cloud Run's own IAM invoker check is not used for
#   per-request auth; the API's own Clerk auth (internal/api/auth.go) still
#   gates every route except the deliberately-public GET /api/images/{id}.
#
# --min-instances=0 --max-instances=4
#   With STORAGE=firestore, all state lives in Firestore + GCS, not in the
#   process, so requests can land on any instance and a redeploy/crash loses
#   nothing. The old in-memory PoC single-instance pin is no longer needed;
#   Cloud Run can scale (including to zero) normally. See DEPLOY.md. Cold
#   starts now open the Firestore/GCS clients; if first-request latency on the
#   render path matters, bump --min-instances to 1.
#
# --timeout=300
#   Generous per-request timeout for synchronous Vertex AI renders.
#   cfg.Server.RenderTimeoutSec (config.yaml) is 180s server-side; this Cloud
#   Run request timeout must stay comfortably above that.
#
# --set-env-vars GOOGLE_CLOUD_PROJECT=labflux-project,...,STORAGE=firestore,BLOB_BUCKET=render-ai-studio-images
#   Vertex AI stays in the existing labflux-project (models/credits already
#   enabled there), accessed cross-project from this Cloud Run service.
#   LOCATION=global is required for the Gemini 3 image models; TEXT_LOCATION
#   =us-central1 is required because text models are not served from
#   "global" (see backend/config/config.yaml comments and
#   internal/config/config.go).
#   STORAGE=firestore selects the Firestore + GCS persistence backend (default
#   "memory" is in-process, local-dev only). BLOB_BUCKET must match the bucket
#   created in DEPLOY.md 1b. STORAGE_PROJECT defaults to the ambient Cloud Run
#   project (render-ai-studio), independent of GOOGLE_CLOUD_PROJECT.
#
# --set-secrets CLERK_SECRET_KEY=CLERK_SECRET_KEY:latest
#   Pulled from Secret Manager at container start time rather than baked
#   into the image or passed as a plain env var.
#
# --service-account render-ai-api@render-ai-studio.iam.gserviceaccount.com
#   Dedicated, least-privilege service account (created in DEPLOY.md) that
#   holds only the cross-project roles/aiplatform.user grant on
#   labflux-project and secretAccessor on CLERK_SECRET_KEY - nothing else.
#
# PORT is intentionally not set here: Cloud Run injects it automatically,
# and internal/config/config.go's Load() overrides config.yaml's
# server.port with it (falling back to 8080 if PORT is unset).
gcloud run deploy render-ai-api \
  --source . \
  --region us-central1 \
  --project render-ai-studio \
  --allow-unauthenticated \
  --min-instances=0 --max-instances=4 \
  --timeout=300 \
  --set-env-vars GOOGLE_CLOUD_PROJECT=labflux-project,GOOGLE_CLOUD_LOCATION=global,GOOGLE_CLOUD_TEXT_LOCATION=us-central1,STORAGE=firestore,BLOB_BUCKET=render-ai-studio-images \
  --set-secrets CLERK_SECRET_KEY=CLERK_SECRET_KEY:latest \
  --service-account "render-ai-api@render-ai-studio.iam.gserviceaccount.com"
