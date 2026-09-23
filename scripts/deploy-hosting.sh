#!/usr/bin/env bash
# Builds the frontend and deploys Firebase Hosting (landing + app sites) and
# Firestore rules. Does NOT deploy the backend - see backend/DEPLOY.md for
# the Cloud Run deployment (the `run` rewrite in firebase.json points at the
# existing `render-ai-api` service, it does not deploy it).
#
# One-time setup (see DEPLOY.md for the full walkthrough):
#   - Replace the __GCP_PROJECT_ID__ / __LANDING_SITE_ID__ / __APP_SITE_ID__
#     placeholders in .firebaserc and this script (or pass -P to override
#     the project below), and create the hosting sites + targets.
#
# Usage:
#   ./scripts/deploy-hosting.sh

set -euo pipefail

PROJECT_ID="__GCP_PROJECT_ID__"

cd "$(dirname "$0")/.."

echo "==> Building frontend (pnpm --dir frontend build)"
pnpm --dir frontend build

echo "==> Deploying hosting (landing + app) and firestore rules"
firebase deploy --only hosting,firestore --project "$PROJECT_ID"
