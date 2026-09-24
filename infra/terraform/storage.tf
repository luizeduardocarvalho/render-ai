resource "google_storage_bucket" "blob" {
  project  = var.app_project_id
  name     = var.blob_bucket_name
  location = var.region

  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  # Protects against an accidental/buggy delete or overwrite of a blob:
  # previous bytes stay recoverable for 30 days with no code changes.
  soft_delete_policy {
    retention_duration_seconds = 30 * 24 * 60 * 60 # 30 days
  }

  # The frontend loads render results and screenshots into a <canvas> (mask
  # editor, before/after slider), so CORS must allow GET from the app
  # origin(s) or those crossOrigin image loads taint the canvas / fail.
  # PUT is for direct browser uploads to signed URLs (backend
  # internal/api/uploads.go); the browser sends the two signed headers
  # below, so the preflight must allow them.
  cors {
    origin          = var.app_cors_origins
    method          = ["GET", "PUT"]
    response_header = ["Content-Type", "x-goog-if-generation-match"]
    max_age_seconds = 3600
  }

  # Direct uploads are staged under uploads/ and deleted once the backend
  # copies them into a blob. One the browser uploaded but never used (tab
  # closed, validation failed mid-way) is cleaned up here.
  lifecycle_rule {
    condition {
      age            = 1
      matches_prefix = ["uploads/"]
    }
    action {
      type = "Delete"
    }
  }

  lifecycle {
    prevent_destroy = true
  }

  depends_on = [google_project_service.app]
}
