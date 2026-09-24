locals {
  # Vertex AI defaults to running in the app project unless a distinct
  # cross-project id (e.g. labflux-project) is given.
  vertex_project_id = coalesce(var.vertex_project_id, var.app_project_id)
}
