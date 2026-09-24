resource "google_firestore_database" "default" {
  project     = var.app_project_id
  name        = "(default)"
  location_id = var.firestore_location
  type        = "FIRESTORE_NATIVE"

  point_in_time_recovery_enablement = "POINT_IN_TIME_RECOVERY_ENABLED"
  delete_protection_state           = "DELETE_PROTECTION_ENABLED"

  # ABANDON: `terraform destroy` must never be able to take out the
  # production database. Deleting it for real is a deliberate, manual
  # `gcloud firestore databases delete` after disabling delete protection.
  deletion_policy = "ABANDON"

  depends_on = [google_project_service.app]
}

# Daily backups, kept 14 days - covers "undo a mistake in the last two
# weeks" without relying on the 7-day PITR window alone.
resource "google_firestore_backup_schedule" "daily" {
  project  = var.app_project_id
  database = google_firestore_database.default.name

  retention = "1209600s" # 14 days

  daily_recurrence {}
}

# Weekly backups (Sunday), kept 8 weeks - a longer-lived snapshot that
# survives well past a monthly billing-cycle lookback.
resource "google_firestore_backup_schedule" "weekly" {
  project  = var.app_project_id
  database = google_firestore_database.default.name

  retention = "4838400s" # 8 weeks

  weekly_recurrence {
    day = "SUNDAY"
  }
}
