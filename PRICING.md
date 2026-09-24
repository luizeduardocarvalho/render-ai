# Pricing - cost basis, plans and packs

Draft, September 2026. All BRL figures use **US$1 = R$5.16**. Re-check the
exchange rate and Vertex AI prices before changing public prices.

## What one render costs us

A render is one call to `gemini-3-pro-image` (see `backend/internal/api/render.go`).
Each variation is a separate, full-price call, so 2 variations cost 2 renders.

Per 2K render with the Pro model, with a typical request of 7 input images
(screenshot, region map, edge map, style anchor, 3 asset reference photos):

| Item | Vertex price | USD | BRL |
|---|---|---|---|
| Output image (1K or 2K) | 1,120 tokens at $120 / 1M | 0.134 | 0.69 |
| Input images and prompt | ~5.4k tokens at $2 / 1M | 0.011 | 0.06 |
| Thinking / text output | a few hundred tokens at $12 / 1M | ~0.005 | ~0.03 |
| Cloud Run + Firestore | | ~0.001 | ~0.005 |
| **Model and infrastructure** | | **≈ 0.15** | **≈ 0.78** |
| Failure and retry buffer (+10%) | | 0.015 | 0.08 |
| Storage (GCS + backup) and image views | | ~0.004 | ~0.02 |
| **Our cost per 2K image** | | **≈ 0.17** | **≈ 0.90** |

Other tiers (before the buffer):

| Tier | BRL |
|---|---|
| 1K draft, Flash model | ≈ 0.23 |
| 2K, Pro model | ≈ 0.80 |
| 4K, Pro model | ≈ 1.34 |

The failure buffer covers model refusals and timeouts: Vertex bills the input
tokens even when no image comes back, and we retry or refund instead of
charging the user.

## Unit

Plans and packs count **2K images**:

- 1 image at 2K = 1 image
- 1 image at 4K = 2 images
- 4 drafts (1K, Flash) = 1 image

## Assumptions behind the margins

- The user uses every image in the plan or pack (worst case).
- Paid by credit card: 4% + R$0.40 per payment. Pix is about 1%.
- Taxes: 6% of revenue (Simples Nacional; confirm the rate with an accountant).
- Payment fees apply once per payment (the monthly charge or each pack
  purchase), not per render.
- Fixed costs (Clerk, domain, Cloud Run minimum instances, support) are not
  included; they come out of the total margin.

## Monthly plans

| Plan | Price/month | Images/month | Price per image | AI cost | Card fee | Taxes (6%) | Margin |
|---|---|---|---|---|---|---|---|
| Free trial (once) | R$0 | 5 | - | R$4.50 | - | - | -R$4.50 (acquisition) |
| Starter | R$49 | 20 | R$2.45 | R$18.00 | R$2.36 | R$2.94 | R$25.70 (52%) |
| Pro | R$129 | 60 | R$2.15 | R$54.00 | R$5.56 | R$7.74 | R$61.70 (48%) |
| Studio | R$299 | 160 | R$1.87 | R$144.00 | R$12.36 | R$17.94 | R$124.70 (42%) |

## Extra image packs (valid for 12 months)

| Pack | Price | Images | Price per image | AI cost | Card fee | Taxes (6%) | Margin |
|---|---|---|---|---|---|---|---|
| Small | R$29 | 10 | R$2.90 | R$9.00 | R$1.56 | R$1.74 | R$16.70 (58%) |
| Medium | R$79 | 30 | R$2.63 | R$27.00 | R$3.56 | R$4.74 | R$43.70 (55%) |
| Large | R$229 | 100 | R$2.29 | R$90.00 | R$9.56 | R$13.74 | R$115.70 (51%) |

Packs cost more per image than the plan of similar size, to steer users toward
subscriptions. Packs have a minimum of R$29 so the fixed card fee stays small.

## Options to consider

- Annual plan with 2 months free.
- Unused monthly images carry over for one month.
- Offer Pix Automático for recurring plans to cut payment fees.

## Billing rules (proposed)

- Charge when the user clicks Generate. The render runs in a background queue,
  so refreshing or closing the page does not cancel it.
- If the model fails after retries, refund the image; we absorb the cost.
- Show the price in images before generating, e.g. "4 variations = 4 images".

## Sources

- [Vertex AI generative AI pricing](https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing)
- [USD/BRL, Investing.com](https://br.investing.com/currencies/usd-brl)
