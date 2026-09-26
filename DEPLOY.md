# Deploying StudioIA

The app deploys as three pieces:

- **Landing page** - a static Firebase Hosting site, served from `landing/`.
- **Frontend app** - a static Firebase Hosting site, served from `frontend/dist`
  (built with `pnpm --dir frontend build`). This is the React SPA the user
  signs into and works in.
- **Backend API** - a Go service on Cloud Run named `render-ai-api` in region
  `us-central1`. See [`backend/DEPLOY.md`](backend/DEPLOY.md) for building the
  container, the service account, and Secret Manager setup - not covered
  here.

Both Hosting sites live under one Firebase project, using Hosting **targets**
so `firebase deploy` can address each site independently.

## How the app talks to the API

In production the frontend never calls the backend cross-origin. Firebase
Hosting on the `app` site rewrites any `/api/**` request straight to the
`render-ai-api` Cloud Run service (region `us-central1`) before it reaches the
SPA catch-all. That rewrite is declared first in `firebase.json`, ahead of the
`**` -> `/index.html` catch-all, so `/api/...` paths hit Cloud Run and every
other path (e.g. `/sign-in`, `/sign-up`, any react-router route) falls back to
the SPA shell.

Because of this, the frontend calls the API at same-origin `/api/...` in
production - no CORS configuration is needed. In local dev it falls back to
`http://localhost:8080` (see `frontend/src/api.ts` and
`frontend/.env.example` for the `VITE_API_URL` override).

## Deploying

The usual way is the **Deploy** workflow: Actions -> Deploy -> Run workflow
([`.github/workflows/deploy.yml`](.github/workflows/deploy.yml)). Pick what to
deploy:

| Option     | Deploys                                                              |
| ---------- | -------------------------------------------------------------------- |
| `both`     | the backend, then everything `frontend` deploys                      |
| `backend`  | the Cloud Run services (`render-ai-api`, `render-ai-worker`)         |
| `frontend` | builds the app, then both Hosting sites and `firestore.rules`        |
| `landing`  | only the landing Hosting site, straight from `landing/` - no build   |

Nothing deploys on merge; a run is always started by hand, in the GitHub
`production` environment.

To deploy Hosting from your own machine instead (Firebase CLI logged in with
deploy access):

```bash
./scripts/deploy-hosting.sh           # build the app, deploy both sites + Firestore rules
./scripts/deploy-hosting.sh landing   # deploy only the landing site
```

## Where things live

- Firebase/GCP project: `studioia-app`, in the `studioia.app` organization
- Landing site: `studioia-landing`, at `https://studioia.app` (`www` redirects
  there; `https://studioia-landing.web.app` also works)
- App site: `studioia-app`, at `https://app.studioia.app` (also
  `https://studioia-app.web.app`)
- The custom domains and their DNS records (Cloudflare) are Terraform, in
  `infra/terraform/dns.tf`.

The `landing` and `app` Hosting targets in `firebase.json` are bound to those
sites in `.firebaserc`.

## Setting it up again from scratch

Only needed for a new project (the one above is already set up). The GCP side -
APIs, Cloud Run, service accounts, and the GitHub deploy identity - is
Terraform, in [`infra/terraform/`](infra/terraform/README.md). The Hosting
side is:

```bash
# 1. Create the two Hosting sites (the project's default site is not used -
#    both sites are explicit).
firebase hosting:sites:create <landing-site-id> --project <project-id>
firebase hosting:sites:create <app-site-id> --project <project-id>

# 2. Bind the Hosting targets in firebase.json to those sites. This updates
#    .firebaserc; commit the result.
firebase target:apply hosting landing <landing-site-id> --project <project-id>
firebase target:apply hosting app <app-site-id> --project <project-id>

# 3. Deploy the Cloud Run backend first - the `app` site's /api/** rewrite
#    expects render-ai-api to exist. Then deploy Hosting.
PROJECT_ID=<project-id> ./scripts/deploy-hosting.sh
```

## Auth (Clerk)

`VITE_CLERK_PUBLISHABLE_KEY` gets baked into the frontend build at build time
(it's the publishable key, not a secret, so this is fine). After the first
deploy:

- Add the deployed app Hosting URL (`https://studioia-app.web.app`)
  to the Clerk dev instance's allowed origins so sign-in/sign-up work from
  that domain.
- Expect Clerk's "Development mode" banner to keep showing until the app has
  a custom domain and a Clerk production instance configured for it - that's
  a separate step, not part of this Hosting setup.

## See also

- [`backend/DEPLOY.md`](backend/DEPLOY.md) - Cloud Run build/deploy, service
  account, and Secret Manager configuration for the API.
