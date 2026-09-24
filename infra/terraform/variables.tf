variable "app_project_id" {
  type        = string
  description = "GCP project id that hosts the render-ai-api Cloud Run service, Firestore database, and blob bucket."
}

variable "vertex_project_id" {
  type        = string
  default     = null
  description = "GCP project id where Vertex AI is enabled and billed (may be a different, cross-project id such as labflux-project). Defaults to app_project_id when null, i.e. Vertex AI runs in the same project as the app."
}

variable "backup_project_id" {
  type        = string
  description = "Separate GCP project id, isolated from the production service account, that holds the backup bucket, Storage Transfer job, and Firestore export destination."
}

variable "region" {
  type        = string
  default     = "us-central1"
  description = "Region for Cloud Run, the blob bucket, and the backup infrastructure."
}

variable "firestore_location" {
  type        = string
  default     = "nam5"
  description = "Multi-region or region for the Firestore database (see https://cloud.google.com/firestore/docs/locations)."
}

variable "blob_bucket_name" {
  type        = string
  description = "Globally-unique GCS bucket name for image blobs (screenshots, asset photos, mask bitmaps, render results). Must match the BLOB_BUCKET env var."
}

variable "backup_bucket_name" {
  type        = string
  description = "Globally-unique GCS bucket name, in backup_project_id, that receives the daily blob copy and weekly Firestore export."
}

variable "app_cors_origins" {
  type        = list(string)
  description = "Origins allowed to GET objects from the blob bucket via CORS (the frontend's Hosting URL(s), plus http://localhost:5173 for local dev)."
}

variable "github_repository" {
  type        = string
  default     = "luizeduardocarvalho/render-ai"
  description = "GitHub \"owner/repo\" allowed to assume the backup-checker service account via Workload Identity Federation."
}

variable "backup_retention_days" {
  type        = number
  default     = 0
  description = "Optional GCS retention policy (in days) on the backup bucket, making objects undeletable until they age past this period. 0 disables the retention policy. This is NEVER locked by Terraform (is_locked = false always) - locking is a deliberate, manual, irreversible decision (see README.md)."
}

variable "cloud_run_min_instances" {
  type        = number
  default     = 0
  description = "Minimum Cloud Run instance count for render-ai-api."
}

variable "cloud_run_max_instances" {
  type        = number
  default     = 4
  description = "Maximum Cloud Run instance count for render-ai-api."
}

variable "github_deploy_environment" {
  type        = string
  default     = "production"
  description = "GitHub Actions environment whose jobs may impersonate the deployer service account (.github/workflows/deploy.yml)."
}

variable "github_deploy_reviewers" {
  type        = list(string)
  default     = []
  description = "GitHub usernames who must approve each production deploy. Empty means deploys start without approval."
}

variable "clerk_publishable_key" {
  type        = string
  description = "Clerk publishable key (pk_...), baked into the frontend build. Public by design - not the secret key."
}

variable "firebase_landing_site_id" {
  type        = string
  description = "Firebase Hosting site id for the landing page (.firebaserc target `landing`)."
}

variable "firebase_app_site_id" {
  type        = string
  description = "Firebase Hosting site id for the app (.firebaserc target `app`)."
}
