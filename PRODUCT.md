# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Architects and interior designers who model in SketchUp. Primary situation: at
their desk, working from SketchUp screenshots of a space (interior or exterior),
assembling client-facing visuals and specifying real, purchasable products for
each surface or furnishing. They need photoreal, product-accurate images without
standing up a manual render pipeline (V-Ray, Enscape, Lumion, physical staging).

## Product Purpose

render-ai turns a SketchUp screenshot into a photorealistic architectural
photograph, placing specific real products into user-masked regions of the image.
Success is a render that reads as a real photograph, keeps the original geometry,
and shows the exact specified products in place - produced in one model call,
in seconds to a minute, at up to 4K.

## Positioning

The differentiating mechanism: mask a region, assign it a real product from a
library (with a reference photo), and the model places that exact product into
the scene matching the screenshot's perspective and lighting - not a generic
"make it photoreal" pass. Cross-angle coherence comes from pinning a chosen
render as a style anchor reused across other views. Preservation checks (edge-IoU
against the screenshot) report when a render drifts from the original geometry.
Runs locally and in memory - no cloud storage, no database.

## Operating Context

Workflow: upload a SketchUp screenshot (a "view") -> paint mask regions and
assign each a library asset -> set scene/style (interior|exterior, lighting
preset, light direction, material notes, extra instructions) -> render (choose
model + resolution, 1-4 variations) -> optionally set a result as the style
anchor and repeat for other views. Tools and artifacts in the user's world:
SketchUp, product/spec sheets and reference photos, material sample boards,
mask/region editing. The app is a local proof of concept (frontend on
localhost:5173, Go backend on :8080); the landing page is a separate marketing
surface.

## Capabilities and Constraints

- Models (via Google Vertex AI): "Pro" = `gemini-3-pro-image` (Nano Banana Pro),
  1K/2K/4K, up to 14 input images; "Flash" = `gemini-3.1-flash-image`
  (Nano Banana 2), 1K only; a text model (`gemini-2.5-flash`) for the object
  inventory.
- One image model call per render. Inputs, in order: screenshot, region map,
  Canny edge map, style anchor (if set), then asset reference photos.
- Flash supports 1K only (2K/4K is rejected). Seed is not exposed by the image
  models; temperature has no documented effect and is not sent.
- Per-render metrics: latency, token usage, estimated cost. Preservation report
  is optional and never auto-retries.
- Terminology: Project, View, Mask, Asset, Render, Style Anchor, Preservation
  Report, Inventory.

## Brand Commitments

- Product name: **render-ai** (lowercase).
- Built on Google's Gemini 3 image models ("Nano Banana Pro" / "Nano Banana 2")
  via Vertex AI - a real, nameable technical foundation.
- Existing app design language: neutral gray scale with an indigo accent
  (#4338ca), light/dark aware. This is the incumbent app look, not a binding
  constraint on the marketing surface.

## Evidence on Hand

- Real product facts, model IDs, pipeline, and workflow: `README.md`,
  `API_CONTRACT.md`, `backend/`, `frontend/`.
- No real customers, testimonials, pricing, benchmarks, or case studies exist
  yet - future work must not fabricate them. It is a proof of concept.
- No real before/after render screenshots are available in-repo yet; the landing
  page currently uses a schematic SVG illustration as a labeled stand-in.

## Product Principles

- Product-accurate, not just "photoreal": the specified real product must appear.
- Preserve the architect's geometry; drift is measured and reported, not hidden.
- Fast, single-call renders with visible cost/metrics - no black-box pipeline.
- Local and private by design; nothing leaves the session.
- Honest proof over hype - it is a working PoC, claims stay truthful.
