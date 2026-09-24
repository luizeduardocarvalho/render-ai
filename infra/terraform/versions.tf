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
    github = {
      source  = "integrations/github"
      version = "~> 6.13"
    }
    time = {
      source  = "hashicorp/time"
      version = "~> 0.14"
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

# Manages the repo's Actions environment and variables (github.tf). Reads the
# token from the GITHUB_TOKEN env var: a fine-grained token for this one
# repository with Administration, Environments and Variables read/write.
provider "github" {
  owner = split("/", var.github_repository)[0]
}
