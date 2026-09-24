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
  cors {
    origin          = var.app_cors_origins
    method          = ["GET"]
    response_header = ["Content-Type"]
    max_age_seconds = 3600
  }

  lifecycle {
    prevent_destroy = true
  }

  depends_on = [google_project_service.app]
}
