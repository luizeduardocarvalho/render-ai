#!/usr/bin/env bash
# Deploys Firebase Hosting from this machine - the local counterpart of the
# Deploy workflow's "frontend" and "landing" options (.github/workflows/
# deploy.yml), which is the usual way to deploy.
#
#   all      builds the frontend, then deploys both Hosting sites (landing +
#            app) and the Firestore rules
#   landing  deploys only the landing site, straight from landing/ (no build)
#
# Does NOT deploy the backend - see backend/DEPLOY.md for the Cloud Run
# deployment (the `run` rewrite in firebase.json points at the existing
# `render-ai-api` service, it does not deploy it).
#
# Needs the Firebase CLI, logged in (`firebase login`) with deploy access to
# the project. The Hosting targets are bound in .firebaserc.
#
# Usage:
#   ./scripts/deploy-hosting.sh [all|landing]     (default: all)
#   PROJECT_ID=other-project ./scripts/deploy-hosting.sh

set -euo pipefail

PROJECT_ID="${PROJECT_ID:-studioia-app}"
what="${1:-all}"

cd "$(dirname "$0")/.."

case "$what" in
  all)
    echo "==> Building frontend (pnpm --dir frontend build)"
    pnpm --dir frontend build
    echo "==> Deploying hosting (landing + app) and firestore rules to $PROJECT_ID"
    firebase deploy --only hosting,firestore --project "$PROJECT_ID"
    ;;
  landing)
    echo "==> Deploying the landing site to $PROJECT_ID"
    firebase deploy --only hosting:landing --project "$PROJECT_ID"
    ;;
  *)
    echo "usage: $0 [all|landing]" >&2
    exit 2
    ;;
esac
