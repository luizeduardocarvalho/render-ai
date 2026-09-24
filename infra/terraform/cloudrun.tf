resource "google_cloud_run_v2_service" "render_api" {
  project  = var.app_project_id
  name     = "render-ai-api"
  location = var.region

  # Public HTTP endpoint (matches --allow-unauthenticated in deploy.sh).
  # Cloud Run's own IAM invoker check is not used for per-request auth; the
  # API's own Clerk auth still gates every route except the deliberately
  # public GET /api/images/{id}. See google_cloud_run_v2_service_iam_member
  # below.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.render_api.email
    timeout         = "300s"

    scaling {
      min_instance_count = var.cloud_run_min_instances
      max_instance_count = var.cloud_run_max_instances
    }

    containers {
      # Placeholder image on first create. Real code ships via
      # `deploy.sh` (`gcloud run deploy --source .`), which Cloud Build
      # pushes to Artifact Registry and Cloud Run then serves - Terraform
      # never builds or pushes the application image itself (see
      # lifecycle.ignore_changes below).
      image = "us-docker.pkg.dev/cloudrun/container/hello"

      env {
        name  = "GOOGLE_CLOUD_PROJECT"
        value = local.vertex_project_id
      }
      env {
        name  = "GOOGLE_CLOUD_LOCATION"
        value = "global"
      }
      env {
        name  = "GOOGLE_CLOUD_TEXT_LOCATION"
        value = "us-central1"
      }
      env {
        name  = "STORAGE"
        value = "firestore"
      }
      env {
        name  = "BLOB_BUCKET"
        value = google_storage_bucket.blob.name
      }
      env {
        name  = "STORAGE_PROJECT"
        value = var.app_project_id
      }
      env {
        name = "CLERK_SECRET_KEY"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.clerk_secret_key.secret_id
            version = "latest"
          }
        }
      }
    }
  }

  depends_on = [
    google_project_service.app,
    google_project_iam_member.render_api_vertex_user,
    google_secret_manager_secret_iam_member.render_api_secret_accessor,
  ]

  lifecycle {
    ignore_changes = [
      # Code deploys (deploy.sh) update the image and Cloud Run's own
      # deploy-client metadata directly; Terraform must not fight them on
      # every plan/apply.
      template[0].containers[0].image,
      client,
      client_version,
    ]
  }
}

# Public invoker binding, equivalent to `--allow-unauthenticated`.
resource "google_cloud_run_v2_service_iam_member" "public_invoker" {
  project  = google_cloud_run_v2_service.render_api.project
  location = google_cloud_run_v2_service.render_api.location
  name     = google_cloud_run_v2_service.render_api.name

  role   = "roles/run.invoker"
  member = "allUsers"
}
