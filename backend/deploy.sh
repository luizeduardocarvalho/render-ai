#!/usr/bin/env bash
# Ships render-ai-api's code to Cloud Run. That's all this script does.
#
# Everything else about the service - env vars, secrets, scaling, timeout,
# service account, IAM invoker binding - is owned by Terraform
# (infra/terraform/cloudrun.tf) so it's reproducible and lives in one place.
# `gcloud run deploy --source .` without those flags builds and pushes a new
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
