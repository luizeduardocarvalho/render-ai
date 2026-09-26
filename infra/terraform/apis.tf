# APIs required in the app project (Cloud Run, Firestore, the blob bucket,
# IAM/SignBlob, the Clerk secret, the Firestore export scheduler, and
# Firebase with its Hosting sites and Firestore rules).
locals {
  app_project_services = [
    "run.googleapis.com",
    "cloudbuild.googleapis.com",
    "artifactregistry.googleapis.com",
    "firestore.googleapis.com",
    "storage.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "secretmanager.googleapis.com",
    "cloudscheduler.googleapis.com",
    "sts.googleapis.com",
    "cloudtasks.googleapis.com",
    "firebase.googleapis.com",
    "firebasehosting.googleapis.com",
    "firebaserules.googleapis.com",
  ]

  # aiplatform.googleapis.com is not in app_project_services, so it is
  # always enabled on whichever project runs Vertex AI: this list when that
  # is a separate project (vertex_project_id), google_project_service.
  # app_aiplatform below when it is the app project itself.
  vertex_project_services = ["aiplatform.googleapis.com"]

  backup_project_services = [
    "storage.googleapis.com",
    "storagetransfer.googleapis.com",
  ]

  vertex_is_separate_project = local.vertex_project_id != var.app_project_id
}

resource "google_project_service" "app" {
  for_each = toset(local.app_project_services)

  project            = var.app_project_id
  service            = each.value
  disable_on_destroy = false
}

# Enabled on the app project too when Vertex AI is NOT cross-project.
resource "google_project_service" "app_aiplatform" {
  count = local.vertex_is_separate_project ? 0 : 1

  project            = var.app_project_id
  service            = "aiplatform.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "vertex" {
  for_each = local.vertex_is_separate_project ? toset(local.vertex_project_services) : []

  project            = local.vertex_project_id
  service            = each.value
  disable_on_destroy = false
}

resource "google_project_service" "backup" {
  for_each = toset(local.backup_project_services)

  project            = var.backup_project_id
  service            = each.value
  disable_on_destroy = false
}
