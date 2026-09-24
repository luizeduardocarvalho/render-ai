# Workload Identity Federation for the GitHub Actions backup-check workflow
# (.github/workflows/backup-check.yml) - no long-lived service account key
# leaves this repo.
resource "google_iam_workload_identity_pool" "github" {
  project                   = var.app_project_id
  workload_identity_pool_id = "github-actions"
  display_name              = "GitHub Actions"
  description               = "Used by GitHub Actions workflows (OIDC) to impersonate GCP service accounts."
}

resource "google_iam_workload_identity_pool_provider" "github" {
  project                            = var.app_project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github"
  display_name                       = "GitHub OIDC"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
    "attribute.ref"        = "assertion.ref"
  }

  # Restrict to this one repository regardless of what attribute_mapping
  # would otherwise accept.
  attribute_condition = "assertion.repository == \"${var.github_repository}\""

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

# Read-only SA for the backup-check workflow: only what it needs to verify
# the daily blob transfer, the Firestore backup schedule, and the weekly
# export ran - nothing else, and no role in the backup project other than
# read access to the backup bucket's objects.
resource "google_service_account" "backup_checker" {
  project      = var.app_project_id
  account_id   = "backup-checker"
  display_name = "GitHub Actions backup-check (read-only)"
}

# List/inspect Storage Transfer operations and jobs in the backup project.
resource "google_project_iam_member" "backup_checker_transfer_viewer" {
  project = var.backup_project_id
  role    = "roles/storagetransfer.viewer"
  member  = google_service_account.backup_checker.member
}

# List Firestore backups in the app project.
resource "google_project_iam_member" "backup_checker_backups_viewer" {
  project = var.app_project_id
  role    = "roles/datastore.backupsViewer"
  member  = google_service_account.backup_checker.member
}

# List/read objects in the backup bucket only (to check the weekly export
# landed) - scoped to the bucket, not the whole backup project.
resource "google_storage_bucket_iam_member" "backup_checker_object_viewer" {
  bucket = google_storage_bucket.backup.name
  role   = "roles/storage.objectViewer"
  member = google_service_account.backup_checker.member
}

# Let the WIF provider impersonate the backup-checker SA, scoped to this repo.
resource "google_service_account_iam_member" "backup_checker_wif_binding" {
  service_account_id = google_service_account.backup_checker.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/${var.github_repository}"
}
