# Async render queue: render-ai-api enqueues one Cloud Task per render
# variation, and Cloud Tasks POSTs it to the render-ai-worker service's
# /internal/render-tasks route with an OIDC token minted as the dedicated
# invoker service account below (see cloudrun.tf for the env vars that wire
# the backend up to this).

# cloudrun.tf derives the worker URL from the project number.
data "google_project" "app" {
  project_id = var.app_project_id
}

resource "google_cloud_tasks_queue" "render" {
  project  = var.app_project_id
  location = var.region
  name     = "render-jobs"

  rate_limits {
    max_concurrent_dispatches = var.render_queue_max_concurrent
  }

  # Renders are billed Vertex AI calls - never let Cloud Tasks retry one.
  # Failures are recorded on the render job instead, and the worker route
  # always answers 2xx once a task is accepted.
  retry_config {
    max_attempts = 1
  }

  depends_on = [google_project_service.app]
}

resource "google_service_account" "render_tasks_invoker" {
  project      = var.app_project_id
  account_id   = "render-tasks-invoker"
  display_name = "Cloud Tasks invoker for render-ai-worker"

  depends_on = [google_project_service.app]
}

# Lets render_api create tasks on the queue.
resource "google_cloud_tasks_queue_iam_member" "render_api_enqueuer" {
  project  = google_cloud_tasks_queue.render.project
  location = google_cloud_tasks_queue.render.location
  name     = google_cloud_tasks_queue.render.name

  role   = "roles/cloudtasks.enqueuer"
  member = google_service_account.render_api.member
}

# Lets render_api create tasks that mint OIDC tokens as the invoker SA
# (Cloud Tasks requires the enqueuing identity to be able to act as the SA
# named in the task's OidcToken).
resource "google_service_account_iam_member" "render_api_invoker_sa_user" {
  service_account_id = google_service_account.render_tasks_invoker.name
  role               = "roles/iam.serviceAccountUser"
  member             = google_service_account.render_api.member
}

# The only principal allowed to call the (non-public) worker service.
resource "google_cloud_run_v2_service_iam_member" "render_tasks_invoker" {
  project  = google_cloud_run_v2_service.render_worker.project
  location = google_cloud_run_v2_service.render_worker.location
  name     = google_cloud_run_v2_service.render_worker.name

  role   = "roles/run.invoker"
  member = google_service_account.render_tasks_invoker.member
}
