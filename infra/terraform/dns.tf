# The public domain: Firebase Hosting custom domains for the landing page
# (apex, plus www redirecting to it) and the app, and every DNS record they
# and Clerk's production instance need, in the domain's Cloudflare zone.
#
# Every record is DNS-only (proxied = false): Firebase and Clerk both verify
# the records and issue their own certificates, which fails behind
# Cloudflare's proxy.

locals {
  app_host = "app.${var.domain}"
  www_host = "www.${var.domain}"

  # What Firebase Hosting asks for (https://firebase.google.com/docs/hosting/
  # custom-domain): the apex points at Hosting's anycast IP and names its site
  # in a TXT record, subdomains CNAME to their site's web.app host. Compare
  # with the firebase_required_dns output after an apply.
  firebase_dns_records = {
    landing_a = {
      name    = var.domain
      type    = "A"
      content = "199.36.158.100"
    }
    landing_txt = {
      name    = var.domain
      type    = "TXT"
      content = "\"hosting-site=${google_firebase_hosting_site.landing.site_id}\""
    }
    www_cname = {
      name    = local.www_host
      type    = "CNAME"
      content = "${google_firebase_hosting_site.landing.site_id}.web.app"
    }
    app_cname = {
      name    = local.app_host
      type    = "CNAME"
      content = "${google_firebase_hosting_site.app.site_id}.web.app"
    }
  }

  clerk_dns_records = {
    for name, target in var.clerk_dns_records : "clerk_${name}" => {
      name    = "${name}.${var.domain}"
      type    = "CNAME"
      content = target
    }
  }
}

data "cloudflare_zone" "app" {
  filter = {
    name = var.domain
  }
}

resource "cloudflare_dns_record" "app" {
  for_each = merge(local.firebase_dns_records, local.clerk_dns_records)

  zone_id = data.cloudflare_zone.app.id
  name    = each.value.name
  type    = each.value.type
  content = each.value.content
  ttl     = 1 # automatic
  proxied = false
  comment = "Managed by Terraform (render-ai infra/terraform/dns.tf)"
}

# Certificates are issued asynchronously once the records above resolve, so
# the apply doesn't block on DNS propagation.
resource "google_firebase_hosting_custom_domain" "landing" {
  provider              = google-beta
  project               = google_firebase_project.app.project
  site_id               = google_firebase_hosting_site.landing.site_id
  custom_domain         = var.domain
  wait_dns_verification = false
}

resource "google_firebase_hosting_custom_domain" "www" {
  provider              = google-beta
  project               = google_firebase_project.app.project
  site_id               = google_firebase_hosting_site.landing.site_id
  custom_domain         = local.www_host
  redirect_target       = var.domain
  wait_dns_verification = false

  depends_on = [google_firebase_hosting_custom_domain.landing]
}

resource "google_firebase_hosting_custom_domain" "app" {
  provider              = google-beta
  project               = google_firebase_project.app.project
  site_id               = google_firebase_hosting_site.app.site_id
  custom_domain         = local.app_host
  wait_dns_verification = false
}
