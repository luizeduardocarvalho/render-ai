# Firebase on the app project and its two Hosting sites. Deploying content
# to them (and firestore.rules) stays with the firebase CLI, run by
# .github/workflows/deploy.yml; .firebaserc maps the `landing` and `app`
# targets to these site ids.
resource "google_firebase_project" "app" {
  provider = google-beta
  project  = var.app_project_id

  depends_on = [google_project_service.app]
}

resource "google_firebase_hosting_site" "landing" {
  provider = google-beta
  project  = google_firebase_project.app.project
  site_id  = var.firebase_landing_site_id
}

resource "google_firebase_hosting_site" "app" {
  provider = google-beta
  project  = google_firebase_project.app.project
  site_id  = var.firebase_app_site_id
}
