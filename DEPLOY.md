# Deploying render-ai

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

## One-time setup

Replace the placeholders below with real values as you go:
- `__GCP_PROJECT_ID__` - a new, dedicated Firebase/GCP project id.
- `__LANDING_SITE_ID__` - Hosting site id for the landing page.
- `__APP_SITE_ID__` - Hosting site id for the frontend app.

```bash
# 1. Create (or select) the Firebase/GCP project.
firebase projects:create __GCP_PROJECT_ID__
# or, if it already exists:
firebase use --add __GCP_PROJECT_ID__

# 2. Create the two Hosting sites (a Firebase project's default site is not
#    used here - both sites are explicit).
firebase hosting:sites:create __LANDING_SITE_ID__ --project __GCP_PROJECT_ID__
firebase hosting:sites:create __APP_SITE_ID__ --project __GCP_PROJECT_ID__

# 3. Bind the Hosting targets in firebase.json to those sites. This updates
#    .firebaserc; commit the result once the placeholders are filled in.
firebase target:apply hosting landing __LANDING_SITE_ID__ --project __GCP_PROJECT_ID__
firebase target:apply hosting app __APP_SITE_ID__ --project __GCP_PROJECT_ID__

# 4. Deploy the Cloud Run backend first (render-ai-api, us-central1) - see
#    backend/DEPLOY.md. The `app` site's /api/** rewrite expects that service
#    to already exist.

# 5. Build the frontend and deploy both Hosting sites plus Firestore rules.
pnpm --dir frontend build
firebase deploy --only hosting,firestore --project __GCP_PROJECT_ID__
```

`scripts/deploy-hosting.sh` wraps step 5 (build + deploy) for repeat
deploys once the one-time setup above is done.

## Auth (Clerk)

`VITE_CLERK_PUBLISHABLE_KEY` gets baked into the frontend build at build time
(it's the publishable key, not a secret, so this is fine). After the first
deploy:

- Add the deployed app Hosting URL (e.g.
  `https://__APP_SITE_ID__.web.app`) to the Clerk dev instance's allowed
  origins so sign-in/sign-up work from that domain.
- Expect Clerk's "Development mode" banner to keep showing until the app has
  a custom domain and a Clerk production instance configured for it - that's
  a separate step, not part of this Hosting setup.

## See also

- [`backend/DEPLOY.md`](backend/DEPLOY.md) - Cloud Run build/deploy, service
  account, and Secret Manager configuration for the API.
