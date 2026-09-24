#!/usr/bin/env bash
# Ships the backend's code to Cloud Run: builds it once as render-ai-api,
# then rolls the same image out to render-ai-worker (which only runs render
# tasks). That's all this script does.
#
# Everything else about the services - env vars, secrets, scaling, timeout,
# service account, IAM invoker bindings - is owned by Terraform
# (infra/terraform/cloudrun.tf) so it's reproducible and lives in one place.
# `gcloud run deploy` without those flags builds and pushes a new
# revision using the *existing* service's configuration; it does not reset
# them to gcloud's defaults. See infra/terraform/README.md for the one-time
# setup and infra/terraform/cloudrun.tf for what's configured.
#
# PROJECT/REGION default to the real render-ai-studio project; override via
# env vars for a different project (e.g. after `terraform apply` elsewhere).
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

PROJECT="${PROJECT:-render-ai-studio}"
REGION="${REGION:-us-central1}"

gcloud run deploy render-ai-api \
  --source . \
  --region "$REGION" \
  --project "$PROJECT"

image="$(gcloud run services describe render-ai-api \
  --region "$REGION" \
  --project "$PROJECT" \
  --format='value(spec.template.spec.containers[0].image)')"

gcloud run deploy render-ai-worker \
  --image "$image" \
  --region "$REGION" \
  --project "$PROJECT"
