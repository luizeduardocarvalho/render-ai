variable "billing_account" {
  type        = string
  description = "Billing account id (XXXXXX-XXXXXX-XXXXXX) linked to both projects. `gcloud billing accounts list` shows it."
}

variable "org_id" {
  type        = string
  default     = null
  description = "Organization id to create the projects under. Leave null for an account without an organization. Set at most one of org_id and folder_id."
}

variable "folder_id" {
  type        = string
  default     = null
  description = "Folder id to create the projects under, instead of org_id."
}

variable "app_project_id" {
  type        = string
  description = "Project for Cloud Run, Firestore, the image bucket and Firebase Hosting (today: render-ai-studio)."
}

variable "app_project_name" {
  type        = string
  default     = "render-ai"
  description = "Display name of the app project."
}

variable "backup_project_id" {
  type        = string
  description = "Separate project for backups and Terraform state (e.g. render-ai-backups)."
}

variable "backup_project_name" {
  type        = string
  default     = "render-ai backups"
  description = "Display name of the backup project."
}

variable "state_bucket_name" {
  type        = string
  description = "Globally unique bucket name for infra/terraform state (e.g. render-ai-tfstate)."
}

variable "region" {
  type        = string
  default     = "us-central1"
  description = "Location of the state bucket."
}
