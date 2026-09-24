resource "google_service_account" "render_api" {
  project      = var.app_project_id
  account_id   = "render-ai-api"
  display_name = "render-ai-api Cloud Run service"

  depends_on = [google_project_service.app]
}

# Firestore read/write.
resource "google_project_iam_member" "render_api_datastore_user" {
  project = var.app_project_id
  role    = "roles/datastore.user"
  member  = google_service_account.render_api.member
}

# Read/write/delete blobs, scoped to the bucket (not the whole project).
resource "google_storage_bucket_iam_member" "render_api_blob_object_admin" {
  bucket = google_storage_bucket.blob.name
  role   = "roles/storage.objectAdmin"
  member = google_service_account.render_api.member
}

# Lets the SA sign GCS URLs as itself via the IAM SignBlob API. Binding is on
# the service account resource itself, with the SA as both principal and
# target (self-impersonation for SignBlob only).
resource "google_service_account_iam_member" "render_api_token_creator" {
  service_account_id = google_service_account.render_api.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = google_service_account.render_api.member
}

# Cross-project (or same-project, if vertex_project_id == app_project_id)
# Vertex AI access.
resource "google_project_iam_member" "render_api_vertex_user" {
  project = local.vertex_project_id
  role    = "roles/aiplatform.user"
  member  = google_service_account.render_api.member

  depends_on = [
    google_project_service.vertex,
    google_project_service.app_aiplatform,
  ]
}

# Read access to the Clerk secret.
resource "google_secret_manager_secret_iam_member" "render_api_secret_accessor" {
  project   = var.app_project_id
  secret_id = google_secret_manager_secret.clerk_secret_key.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = google_service_account.render_api.member
}
