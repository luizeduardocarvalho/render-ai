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
# --min-instances=0 --max-instances=1
#   This is an in-memory PoC: internal/store keeps all state (projects,
#   assets, views, masks, rendered images) in the process, with no database.
#   Pinning min=max=1 keeps exactly one instance alive at all times, so state
#   survives between requests and never gets split or lost across replicas.
#   Caveat: this means no scale-to-zero savings and no horizontal scaling -
#   a redeploy or crash still loses all in-memory state, and a spike in
#   traffic cannot be absorbed by adding instances. See DEPLOY.md.
#
# --timeout=300
#   Generous per-request timeout for synchronous Vertex AI renders.
#   cfg.Server.RenderTimeoutSec (config.yaml) is 180s server-side; this Cloud
#   Run request timeout must stay comfortably above that.
#
# --set-env-vars GOOGLE_CLOUD_PROJECT=labflux-project,GOOGLE_CLOUD_LOCATION=global,GOOGLE_CLOUD_TEXT_LOCATION=us-central1
#   Vertex AI stays in the existing labflux-project (models/credits already
#   enabled there), accessed cross-project from this Cloud Run service.
#   LOCATION=global is required for the Gemini 3 image models; TEXT_LOCATION
#   =us-central1 is required because text models are not served from
#   "global" (see backend/config/config.yaml comments and
#   internal/config/config.go).
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
  --min-instances=0 --max-instances=1 \
  --timeout=300 \
  --set-env-vars GOOGLE_CLOUD_PROJECT=labflux-project,GOOGLE_CLOUD_LOCATION=global,GOOGLE_CLOUD_TEXT_LOCATION=us-central1 \
  --set-secrets CLERK_SECRET_KEY=CLERK_SECRET_KEY:latest \
  --service-account "render-ai-api@render-ai-studio.iam.gserviceaccount.com"
