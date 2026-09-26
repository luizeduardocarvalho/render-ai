locals {
  backend_memory = "512Mi"

  # Cloud Tasks POSTs render tasks here. Built from the project number
  # because the worker can't reference its own `uri` in its own template (a
  # dependency cycle); it's the same run.app URL Cloud Run generates.
  render_worker_url = "https://render-ai-worker-${data.google_project.app.number}.${var.region}.run.app"

  # Both services run the same image with the same config; the worker only
  # ever receives /internal/render-tasks. RENDER_WORKER_AUDIENCE is left
  # unset - it defaults to RENDER_WORKER_URL.
  backend_env = [
    { name = "GOOGLE_CLOUD_PROJECT", value = local.vertex_project_id },
    { name = "GOOGLE_CLOUD_LOCATION", value = "global" },
    { name = "GOOGLE_CLOUD_TEXT_LOCATION", value = "us-central1" },
    { name = "STORAGE", value = "firestore" },
    { name = "BLOB_BUCKET", value = google_storage_bucket.blob.name },
    { name = "STORAGE_PROJECT", value = var.app_project_id },
    { name = "RENDER_QUEUE", value = "cloudtasks" },
    { name = "RENDER_TASKS_QUEUE", value = "projects/${var.app_project_id}/locations/${var.region}/queues/${google_cloud_tasks_queue.render.name}" },
    { name = "RENDER_WORKER_URL", value = local.render_worker_url },
    { name = "RENDER_TASKS_INVOKER_SA", value = google_service_account.render_tasks_invoker.email },
    # Makes the Go GC work harder near the container limit instead of letting
    # the heap grow to ~2x live data and getting OOM-killed. Keep it ~100Mi
    # below backend_memory for non-heap memory.
    { name = "GOMEMLIMIT", value = "400MiB" },
  ]
}

resource "google_cloud_run_v2_service" "render_api" {
  project  = var.app_project_id
  name     = "render-ai-api"
  location = var.region

  # Public HTTP endpoint. Cloud Run's own IAM invoker check is turned off
  # rather than granting roles/run.invoker to allUsers, because the
  # organization's domain-restricted sharing policy
  # (iam.allowedPolicyMemberDomains) rejects allUsers bindings. The API's
  # own Clerk auth still gates every route except the deliberately public
  # GET /api/images/{id}.
  ingress              = "INGRESS_TRAFFIC_ALL"
  invoker_iam_disabled = true

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

      resources {
        limits = {
          cpu    = "1000m"
          memory = local.backend_memory
        }
        cpu_idle          = true
        startup_cpu_boost = true
      }

      dynamic "env" {
        for_each = local.backend_env
        content {
          name  = env.value.name
          value = env.value.value
        }
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
    google_cloud_tasks_queue_iam_member.render_api_enqueuer,
    google_service_account_iam_member.render_api_invoker_sa_user,
    # RENDER_WORKER_URL must point at a live, invokable worker before the API
    # starts enqueueing to it - a task that can't be delivered is dropped.
    google_cloud_run_v2_service_iam_member.render_tasks_invoker,
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

# Runs render tasks only, one per instance, so a render's memory never adds
# up with other renders or with API traffic on the same instance. Not public:
# only the render-tasks-invoker service account can call it (tasks.tf).
resource "google_cloud_run_v2_service" "render_worker" {
  project  = var.app_project_id
  name     = "render-ai-worker"
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.render_api.email
    # At least the Cloud Tasks dispatch deadline (renderTimeoutSec + 120s).
    timeout                          = "300s"
    max_instance_request_concurrency = 1

    scaling {
      min_instance_count = 0
      # Every task the queue dispatches must find a free instance: a request
      # Cloud Run rejects for lack of capacity is dropped (max_attempts = 1).
      max_instance_count = var.render_queue_max_concurrent
    }

    containers {
      # Placeholder image on first create, like render-ai-api. It would
      # answer every task with 200 and drop it, but nothing enqueues yet:
      # render-ai-api runs the same placeholder until the first deploy, and
      # the Deploy workflow rolls the real image out to this worker before
      # the API.
      image = "us-docker.pkg.dev/cloudrun/container/hello"

      resources {
        limits = {
          cpu    = "1000m"
          memory = local.backend_memory
        }
        cpu_idle          = true
        startup_cpu_boost = true
      }

      dynamic "env" {
        for_each = local.backend_env
        content {
          name  = env.value.name
          value = env.value.value
        }
      }
      # Same binary, so its /api routes exist too; keep them authenticated.
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
      template[0].containers[0].image,
      client,
      client_version,
    ]
  }
}
