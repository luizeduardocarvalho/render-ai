# StudioIA

Turn SketchUp screenshots into photorealistic architectural photographs, placing
specific real products into user-masked regions, using Google's Gemini 3 image
models (Nano Banana Pro / Nano Banana 2) through **Vertex AI**.

Projects are **persisted per user**: the deployed backend stores structured data in
Firestore and image blobs (screenshots, masks, renders) in a Google Cloud Storage
bucket, scoped to the signed-in user, who picks a project from a list. Set
`STORAGE=firestore` + `BLOB_BUCKET=...` (see `backend/DEPLOY.md`). Local dev
defaults to `STORAGE=memory` - an in-process store that is lost on restart, so no
cloud setup is needed to run locally.

- `frontend/` - React + TypeScript + Vite, Konva mask editor.
- `backend/`  - Go, official Google Gen AI Go SDK (`google.golang.org/genai`), Vertex AI backend, ADC.
- `API_CONTRACT.md` - the HTTP contract shared by both.

## Models used

| Choice | Model ID | Resolutions | Notes |
|--------|----------|-------------|-------|
| Pro (main)   | `gemini-3-pro-image`    | 1K / 2K / 4K | Nano Banana Pro. **Global endpoint only.** Up to 14 input images. |
| Flash (compare) | `gemini-3.1-flash-image` | 1K only | Nano Banana 2. Global endpoint only. |
| Text (inventory + checks) | `gemini-2.5-flash` | - | Object inventory and preservation inventory check. Runs on a **regional** endpoint (`textLocation`, e.g. `us-central1`), not `global`. |

Notes on parameters (confirmed from Vertex AI docs):
- Image output is requested with `ResponseModalities: ["IMAGE","TEXT"]`.
- Resolution/aspect are set via `ImageConfig{ ImageSize, AspectRatio }`; `ImageSize` is `1K`/`2K`/`4K`.
- **Seed is not exposed** for these image models, and temperature has no documented effect on
  image generation, so we do not send them. Cross-angle consistency relies on the style anchor
  image plus the prompt.
- `gemini-2.5-flash-image` is intentionally **not** used (being retired).

## Prerequisites

- Go 1.24+ and Node 20+.
- Google Cloud SDK (`gcloud`) and a GCP project with billing enabled.

### 1. Enable APIs

```bash
gcloud services enable aiplatform.googleapis.com --project YOUR_PROJECT
```

Then **enable the image models in Model Garden** for the project (one-time, free - you only
pay per render). In the Cloud console: **Vertex AI -> Model Garden**, search for
**"Nano Banana Pro"** (Gemini 3 Pro Image) and **"Nano Banana 2"** (Gemini 3.1 Flash Image)
and click **Enable** on each. Until this is done, render calls return HTTP 404
("your project does not have access to it"). The text model (`gemini-2.5-flash`) needs no
extra enablement.

### 2. Local credentials (Application Default Credentials)

```bash
gcloud auth application-default login
gcloud config set project YOUR_PROJECT
```

Your user (or the ADC principal) needs the **Vertex AI User** role
(`roles/aiplatform.user`) on the project. That is enough to call the models.

### 3. Environment variables

The backend reads these (env overrides `config/config.yaml`):

```bash
export GOOGLE_CLOUD_PROJECT=labflux-project       # already the default in config/config.yaml
export GOOGLE_CLOUD_LOCATION=global               # image models require the global endpoint
export GOOGLE_CLOUD_TEXT_LOCATION=us-central1     # text model endpoint (optional; defaults to config)
# ADC is picked up automatically from the gcloud login above.
```

> The default project is `labflux-project` (the same GCP project as the pipecat / cold-call
> stack, where Vertex AI and the credits live). Note it uses `us-central1` for its chat agent,
> but the Nano Banana image models run **only** on the `global` endpoint, so keep location
> `global` here.

## Run

Backend:

```bash
cd backend
go run ./cmd/server         # serves http://localhost:8080
```

Frontend:

```bash
cd frontend
pnpm install
pnpm dev                    # serves http://localhost:5173
```

Open http://localhost:5173.

## Prompt template

The render instructions live **outside the code** at
[`backend/prompts/render.tmpl`](backend/prompts/render.tmpl) (Go `text/template`). Edit it and
just re-run a render - no rebuild needed; the backend reads it fresh per render. It receives the
image ordering, the region -> product mapping, project style settings, and the object inventory.
Tweak wording there to experiment with preservation strength or style without touching Go code.

Pricing used for the cost estimate is in [`backend/config/config.yaml`](backend/config/config.yaml)
under `pricing:` - fill in current prices.

## How a render is assembled

Single model call per render. Input images, in order (Pro allows <=14 total):
1. Original screenshot.
2. Region map (screenshot with each mask filled in its asset's color; skipped if no masks).
3. Canny edge map of the screenshot (computed in Go).
4. Style anchor render, if one is set for the project.
5. Reference photos of the assets used by this view's visible masks.

Plus the text prompt from the template. Optional preservation check runs afterward:
edge IoU vs. the screenshot, and an inventory diff via the text model. Results are reported,
never auto-retried.
