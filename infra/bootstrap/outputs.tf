output "state_bucket" {
  value       = google_storage_bucket.tfstate.name
  description = "Pass to infra/terraform: terraform init -backend-config=\"bucket=<this>\" -backend-config=\"prefix=render-ai\"."
}

output "app_project_id" {
  value = google_project.this["app"].project_id
}

output "backup_project_id" {
  value = google_project.this["backup"].project_id
}
