# Weekly, REST-API-driven Firestore export into the backup bucket, so a
# restore doesn't depend on PITR/scheduled backups still existing in the
# production project.
resource "google_service_account" "firestore_exporter" {
  project      = var.app_project_id
  account_id   = "render-ai-firestore-exporter"
  display_name = "Scheduled Firestore export"

  depends_on = [google_project_service.app]
}

# Lets the exporter SA call exportDocuments.
resource "google_project_iam_member" "exporter_import_export_admin" {
  project = var.app_project_id
  role    = "roles/datastore.importExportAdmin"
  member  = google_service_account.firestore_exporter.member
}

# The export itself is written to the bucket as the app project's Firestore
# service agent, NOT as the exporter SA. Give that agent create + list only
# on the backup bucket - no delete, so it can never remove older exports.
resource "google_project_service_identity" "firestore" {
  provider = google-beta

  project = var.app_project_id
  service = "firestore.googleapis.com"

  depends_on = [google_project_service.app]
}

resource "google_storage_bucket_iam_member" "firestore_agent_object_creator" {
  bucket = google_storage_bucket.backup.name
  role   = "roles/storage.objectCreator"
  member = google_project_service_identity.firestore.member
}

resource "google_storage_bucket_iam_member" "firestore_agent_legacy_reader" {
  bucket = google_storage_bucket.backup.name
  role   = "roles/storage.legacyBucketReader"
  member = google_project_service_identity.firestore.member
}

resource "google_cloud_scheduler_job" "firestore_export" {
  project   = var.app_project_id
  region    = var.region
  name      = "render-ai-firestore-export"
  schedule  = "0 6 * * 0" # Sunday 06:00 UTC
  time_zone = "UTC"

  http_target {
    http_method = "POST"
    uri         = "https://firestore.googleapis.com/v1/projects/${var.app_project_id}/databases/(default):exportDocuments"

    body = base64encode(jsonencode({
      outputUriPrefix = "gs://${google_storage_bucket.backup.name}/firestore-exports"
    }))

    oauth_token {
      service_account_email = google_service_account.firestore_exporter.email
      scope                 = "https://www.googleapis.com/auth/datastore"
    }
  }

  depends_on = [
    google_project_service.app,
    google_project_iam_member.exporter_import_export_admin,
    google_storage_bucket_iam_member.firestore_agent_object_creator,
    google_storage_bucket_iam_member.firestore_agent_legacy_reader,
  ]
}
