# The secret container only - deliberately NO google_secret_manager_secret_version
# here. The Clerk secret value must never enter Terraform state or a
# plan/apply log. Add the value out-of-band with gcloud (see README.md):
#
#   printf '%s' "$CLERK_SECRET_KEY_VALUE" | gcloud secrets versions add \
#     CLERK_SECRET_KEY --project=<app_project_id> --data-file=-
resource "google_secret_manager_secret" "clerk_secret_key" {
  project   = var.app_project_id
  secret_id = "CLERK_SECRET_KEY"

  replication {
    auto {}
  }

  depends_on = [google_project_service.app]
}
