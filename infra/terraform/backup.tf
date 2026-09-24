# A separate project, isolated from the render-ai-api service account, so a
# compromise of the production project/SA cannot also reach (or destroy) the
# backups. IAM is deny-by-default: render_api never gets any role here -
# only the Storage Transfer service agent (below) and the Firestore service
# agent (export.tf) do, both scoped to this one bucket.
resource "google_storage_bucket" "backup" {
  project  = var.backup_project_id
  name     = var.backup_bucket_name
  location = var.region

  storage_class = "COLDLINE"

  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  dynamic "retention_policy" {
    for_each = var.backup_retention_days > 0 ? [1] : []
    content {
      retention_period = var.backup_retention_days * 24 * 60 * 60
      # Never locked from Terraform: locking is irreversible (retention can
      # only be raised, never lowered or removed) and must be a deliberate,
      # manually-taken decision - see README.md.
      is_locked = false
    }
  }

  lifecycle {
    prevent_destroy = true
  }

  depends_on = [google_project_service.backup]
}

# The Storage Transfer Service's per-project agent (created lazily the first
# time the API is used in a project - reading the data source here also
# provisions it).
data "google_storage_transfer_project_service_account" "backup" {
  project = var.backup_project_id
}

# Read the source (blob) bucket, in the app project.
resource "google_storage_bucket_iam_member" "sts_source_object_viewer" {
  bucket = google_storage_bucket.blob.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${data.google_storage_transfer_project_service_account.backup.email}"
}

resource "google_storage_bucket_iam_member" "sts_source_legacy_reader" {
  bucket = google_storage_bucket.blob.name
  role   = "roles/storage.legacyBucketReader"
  member = "serviceAccount:${data.google_storage_transfer_project_service_account.backup.email}"
}

# Write the destination (backup) bucket, in the backup project.
resource "google_storage_bucket_iam_member" "sts_dest_legacy_writer" {
  bucket = google_storage_bucket.backup.name
  role   = "roles/storage.legacyBucketWriter"
  member = "serviceAccount:${data.google_storage_transfer_project_service_account.backup.email}"
}

resource "google_storage_bucket_iam_member" "sts_dest_object_viewer" {
  bucket = google_storage_bucket.backup.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${data.google_storage_transfer_project_service_account.backup.email}"
}

# Daily copy of the blob bucket into the backup bucket. Deliberately omits
# delete_objects_*: a deletion in production is never propagated to the
# backup copy, only additions/overwrites (overwrite_when=DIFFERENT re-copies
# a blob only if its content changed - covers mask bitmaps, the one blob
# type overwritten in place).
resource "google_storage_transfer_job" "blob_backup" {
  project     = var.backup_project_id
  description = "render-ai daily blob backup"

  transfer_spec {
    gcs_data_source {
      bucket_name = google_storage_bucket.blob.name
    }
    gcs_data_sink {
      bucket_name = google_storage_bucket.backup.name
      path        = "blobs/"
    }
    transfer_options {
      overwrite_when                            = "DIFFERENT"
      delete_objects_unique_in_sink             = false
      delete_objects_from_source_after_transfer = false
    }
  }

  schedule {
    schedule_start_date {
      year  = 2026
      month = 1
      day   = 1
    }
    repeat_interval = "86400s" # daily

    start_time_of_day {
      hours   = 7
      minutes = 0
      seconds = 0
      nanos   = 0
    }
  }

  depends_on = [
    google_project_service.backup,
    google_storage_bucket_iam_member.sts_source_object_viewer,
    google_storage_bucket_iam_member.sts_source_legacy_reader,
    google_storage_bucket_iam_member.sts_dest_legacy_writer,
    google_storage_bucket_iam_member.sts_dest_object_viewer,
  ]
}
