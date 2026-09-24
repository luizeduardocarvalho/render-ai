# One-time bootstrap: the GCP projects, their billing link, and the bucket
# that stores infra/terraform's state. Everything else lives in
# infra/terraform. This config starts with local state (the state bucket
# doesn't exist yet); README.md explains moving it into the bucket after the
# first apply.

terraform {
  required_version = ">= 1.6"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 8.4"
    }
  }
}

provider "google" {
  region = var.region
}

locals {
  projects = {
    app    = { id = var.app_project_id, name = var.app_project_name }
    backup = { id = var.backup_project_id, name = var.backup_project_name }
  }
}

resource "google_project" "this" {
  for_each = local.projects

  project_id      = each.value.id
  name            = each.value.name
  billing_account = var.billing_account
  org_id          = var.org_id
  folder_id       = var.folder_id

  # Never delete a project from Terraform: `terraform destroy` would take
  # every resource and all data in it along. Delete projects by hand.
  deletion_policy = "PREVENT"
}

resource "google_project_service" "backup_storage" {
  project            = google_project.this["backup"].project_id
  service            = "storage.googleapis.com"
  disable_on_destroy = false
}

# State for infra/terraform. Lives in the backup project so the production
# service account can't touch it; versioned so a bad apply can be rolled
# back to the previous state file.
resource "google_storage_bucket" "tfstate" {
  project  = google_project.this["backup"].project_id
  name     = var.state_bucket_name
  location = var.region

  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      num_newer_versions = 20
      with_state         = "ARCHIVED"
    }
    action {
      type = "Delete"
    }
  }

  lifecycle {
    prevent_destroy = true
  }

  depends_on = [google_project_service.backup_storage]
}
