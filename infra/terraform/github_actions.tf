# The GitHub side of the workflows: the `production` environment the deploy
# workflow runs in, and every variable the deploy and backup-check workflows
# read, filled straight from the resources above so nothing is copied by hand.

data "github_user" "deploy_reviewers" {
  for_each = toset(var.github_deploy_reviewers)
  username = each.value
}

resource "github_repository_environment" "production" {
  repository  = local.github_repo_name
  environment = var.github_deploy_environment

  dynamic "reviewers" {
    for_each = length(var.github_deploy_reviewers) > 0 ? [1] : []
    content {
      users = [for u in data.github_user.deploy_reviewers : tonumber(u.id)]
    }
  }

  # Only the main branch can deploy.
  deployment_branch_policy {
    protected_branches     = false
    custom_branch_policies = true
  }
}

resource "github_repository_environment_deployment_policy" "production_main" {
  repository     = local.github_repo_name
  environment    = github_repository_environment.production.environment
  branch_pattern = "main"
}

locals {
  github_repo_name = split("/", var.github_repository)[1]

  deploy_variables = {
    GCP_WIF_PROVIDER           = google_iam_workload_identity_pool_provider.github.name
    GCP_DEPLOYER_SA            = google_service_account.deployer.email
    GCP_ARTIFACT_REPO          = "${google_artifact_registry_repository.app.location}-docker.pkg.dev/${google_artifact_registry_repository.app.project}/${google_artifact_registry_repository.app.repository_id}"
    GCP_PROJECT                = var.app_project_id
    GCP_REGION                 = var.region
    VITE_CLERK_PUBLISHABLE_KEY = var.clerk_publishable_key
  }

  backup_check_variables = {
    GCP_WIF_PROVIDER       = google_iam_workload_identity_pool_provider.github.name
    GCP_BACKUP_CHECKER_SA  = google_service_account.backup_checker.email
    GCP_APP_PROJECT        = var.app_project_id
    GCP_BACKUP_PROJECT     = var.backup_project_id
    GCP_TRANSFER_JOB       = google_storage_transfer_job.blob_backup.name
    GCP_BACKUP_BUCKET      = google_storage_bucket.backup.name
    GCP_FIRESTORE_LOCATION = var.firestore_location
    # Backups take up to a week to all exist (the Firestore export is
    # weekly); the check tolerates missing ones until 8 days after this.
    BACKUP_CHECK_SINCE = time_static.backups_created.rfc3339
  }
}

# Fixed at first apply; used for the backup check's grace period.
resource "time_static" "backups_created" {
  depends_on = [
    google_storage_transfer_job.blob_backup,
    google_cloud_scheduler_job.firestore_export,
  ]
}

resource "github_actions_environment_variable" "deploy" {
  for_each = local.deploy_variables

  repository    = local.github_repo_name
  environment   = github_repository_environment.production.environment
  variable_name = each.key
  value         = each.value
}

resource "github_actions_variable" "backup_check" {
  for_each = local.backup_check_variables

  repository    = local.github_repo_name
  variable_name = each.key
  value         = each.value
}
