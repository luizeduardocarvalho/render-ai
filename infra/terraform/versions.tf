terraform {
  required_version = ">= 1.6"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 8.4"
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = "~> 8.4"
    }
  }

  # Partial backend config: the bucket/prefix are passed at `terraform init`
  # time via `-backend-config`, so the same config works against the
  # bootstrap state bucket for any project (see README.md).
  backend "gcs" {}
}

provider "google" {
  project = var.app_project_id
  region  = var.region
}

provider "google-beta" {
  project = var.app_project_id
  region  = var.region
}
