# Manual deploys from GitHub Actions (.github/workflows/deploy.yml). The
# workflow builds the backend image, pushes it here, rolls it out to Cloud
# Run, then builds the frontend and runs `firebase deploy`. It signs in with
# Workload Identity Federation (github.tf) as the deployer service account,
# which only jobs running in the repo's `production` environment can use.

resource "google_artifact_registry_repository" "app" {
  project       = var.app_project_id
  location      = var.region
  repository_id = "render-ai"
  format        = "DOCKER"
  description   = "render-ai backend images, pushed by the deploy workflow."

  # Keep the 10 newest images for rollbacks; delete anything older than 30
  # days beyond those.
  cleanup_policies {
    id     = "keep-recent"
    action = "KEEP"
    most_recent_versions {
      keep_count = 10
    }
  }
  cleanup_policies {
    id     = "delete-old"
    action = "DELETE"
    condition {
      older_than = "2592000s"
    }
  }

  depends_on = [google_project_service.app]
}

resource "google_service_account" "deployer" {
  project      = var.app_project_id
  account_id   = "github-deployer"
  display_name = "GitHub Actions deploy workflow"

  depends_on = [google_project_service.app]
}

# Push backend images.
resource "google_artifact_registry_repository_iam_member" "deployer_writer" {
  project    = google_artifact_registry_repository.app.project
  location   = google_artifact_registry_repository.app.location
  repository = google_artifact_registry_repository.app.name
  role       = "roles/artifactregistry.writer"
  member     = google_service_account.deployer.member
}

# Roll out new Cloud Run revisions.
resource "google_project_iam_member" "deployer_run_developer" {
  project = var.app_project_id
  role    = "roles/run.developer"
  member  = google_service_account.deployer.member
}

# A revision runs as render-ai-api, so deploying one needs actAs on it.
resource "google_service_account_iam_member" "deployer_acts_as_render_api" {
  service_account_id = google_service_account.render_api.name
  role               = "roles/iam.serviceAccountUser"
  member             = google_service_account.deployer.member
}

# `firebase deploy --only hosting,firestore`: hosting releases, the app site's
# /api/** rewrite to Cloud Run (needs to read the service), and Firestore
# security rules.
resource "google_project_iam_member" "deployer_firebase" {
  for_each = toset([
    "roles/firebasehosting.admin",
    "roles/firebaserules.admin",
    "roles/run.viewer",
    "roles/serviceusage.serviceUsageConsumer",
  ])

  project = var.app_project_id
  role    = each.value
  member  = google_service_account.deployer.member
}

# Only jobs in the GitHub `production` environment can impersonate the
# deployer, so environment protection rules such as required reviewers apply
# to every deploy. This matches on the `environment` claim rather than the
# OIDC subject: with GitHub's immutable subjects the subject embeds numeric
# owner/repo ids (repo:owner@123/repo@456:environment:production), so a
# name-based subject never matches. The provider's attribute_condition
# (github.tf) already limits tokens to this repository.
resource "google_service_account_iam_member" "deployer_wif_binding" {
  service_account_id = google_service_account.deployer.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.environment/${var.github_deploy_environment}"
}
