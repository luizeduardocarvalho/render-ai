output "render_api_service_account_email" {
  value       = google_service_account.render_api.email
  description = "Runtime service account for the render-ai-api Cloud Run service."
}

output "firestore_exporter_service_account_email" {
  value       = google_service_account.firestore_exporter.email
  description = "Service account Cloud Scheduler uses to trigger the weekly Firestore export."
}

output "backup_checker_service_account_email" {
  value       = google_service_account.backup_checker.email
  description = "Read-only service account the GitHub Actions backup-check workflow impersonates. Set as the GCP_BACKUP_CHECKER_SA repo variable."
}

output "workload_identity_provider" {
  value       = google_iam_workload_identity_pool_provider.github.name
  description = "Full resource name of the WIF provider. Set as the GCP_WIF_PROVIDER repo variable."
}

output "blob_bucket_name" {
  value       = google_storage_bucket.blob.name
  description = "GCS bucket holding image blobs."
}

output "backup_bucket_name" {
  value       = google_storage_bucket.backup.name
  description = "GCS bucket, in the backup project, holding the daily blob copy and weekly Firestore export. Set as the GCP_BACKUP_BUCKET repo variable."
}

output "transfer_job_name" {
  value       = google_storage_transfer_job.blob_backup.name
  description = "Storage Transfer job name (transferJobs/...). Set as the GCP_TRANSFER_JOB repo variable."
}

output "cloud_run_url" {
  value       = google_cloud_run_v2_service.render_api.uri
  description = "Public URL of the render-ai-api Cloud Run service."
}
