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

output "render_worker_url" {
  value       = google_cloud_run_v2_service.render_worker.uri
  description = "URL of the non-public render-ai-worker Cloud Run service Cloud Tasks delivers render tasks to."
}

output "deployer_service_account_email" {
  value       = google_service_account.deployer.email
  description = "Service account the deploy workflow impersonates. Set as GCP_DEPLOYER_SA in the GitHub production environment."
}

output "render_tasks_queue" {
  value       = google_cloud_tasks_queue.render.id
  description = "Full Cloud Tasks queue id (projects/P/locations/L/queues/Q), set as RENDER_TASKS_QUEUE on the Cloud Run service."
}

output "render_tasks_invoker_service_account_email" {
  value       = google_service_account.render_tasks_invoker.email
  description = "Service account Cloud Tasks uses to mint the OIDC token it presents to /internal/render-tasks."
}

output "artifact_registry_repository" {
  value       = "${google_artifact_registry_repository.app.location}-docker.pkg.dev/${google_artifact_registry_repository.app.project}/${google_artifact_registry_repository.app.repository_id}"
  description = "Docker repository for backend images. Set as GCP_ARTIFACT_REPO in the GitHub production environment."
}

output "firebase_required_dns" {
  value = {
    for k, d in {
      landing = google_firebase_hosting_custom_domain.landing
      www     = google_firebase_hosting_custom_domain.www
      app     = google_firebase_hosting_custom_domain.app
    } : k => try(d.required_dns_updates[0].desired, [])
  }
  description = "DNS records Firebase Hosting asks for, per custom domain. Should match cloudflare_dns_record.app (dns.tf)."
}
