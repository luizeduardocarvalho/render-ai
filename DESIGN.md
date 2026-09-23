# Design

Visual world for the render-ai **landing page** (`landing/`). Scope: this
marketing surface only. The in-app tool (`frontend/`) keeps its own indigo
system; this document does not govern it.

## World

**Architecture monograph.** The page reads as a published building study (El
Croquis / 2G): the SketchUp drawing and its photograph presented as one indexed
figure, on paper, with hairline rules and plate folios doing the structure. It
refuses the SaaS gradient hero and the icon-card grid. Dark mode is a cyanotype
negative (paper-white ink on charcoal).

## Color

Ink on paper. Hairline rules and poche carry structure - not boxes or shadows
(print is flat). One accent only: blueprint blue, used **exclusively** on
actionable elements (primary button, links on hover, the compare plumb line,
spec indices, form focus/caret). Never decorative.

| Token | Light | Dark (cyanotype) |
|---|---|---|
| `--paper` | `#efeae1` | `#14161a` |
| `--paper-2` | `#e7e1d5` | `#191c21` |
| `--ink` | `#1a1917` | `#e8e3d8` |
| `--ink-2` (secondary) | `#56534b` | `#a6a294` |
| `--ink-3` (labels) | `#6f6b5f` | `#837f73` |
| `--rule` (hairline) | `#d0cabb` | `#2b2e34` |
| `--rule-strong` | `#b6af9d` | `#3b3f47` |
| `--accent` (actionable only) | `#22406b` | `#6f9bcd` |
| `--accent-ink` | `#f4f1ea` | `#12141a` |

Strategy: Restrained (neutrals + one accent). Light/dark chosen from theme, both
first-class. All body/label text meets WCAG AA on its ground.

## Type

Self-hosted variable faces in `landing/fonts/`:

- **Archivo** (`--display` / `--sans`) - grotesque with a width axis (62-125%).
  Display and structural labels run expanded + heavy (`font-stretch` 104-112%,
  weight 700-800, tracking -0.03em). Body runs normal width.
- **Spline Sans Mono** (`--mono`) - all measurement/metadata: folios, plate
  numbers, the fact ledger (tabular-nums), figure captions, form labels,
  buttons. Mono here is data/measurement, not costume.

Scale: hero display `clamp(40px,6vw,76px)`; plate headings `clamp(26px,3.4vw,
40px)`; body 16-18px at 62-72ch; mono labels 12-14px, uppercase, tracked +0.1em.
Emphasis is weight/size or a drawn underline - never color, never gradient text.

## Composition

- **Sheet**: centered column, `max-width 1200px`, gutter `clamp(20px,5vw,72px)`.
- **Folio**: sticky running header - wordmark, nav, "Plate 01 / Study" ref, over
  a full-width ink rule.
- **Plate header**: section heading set *inline* in a running rule with its plate
  number and a meta label (never a kicker/eyebrow stacked above a heading).
- **Capabilities**: a 2-column specification ledger (indexed 2.1-2.6, hairline
  rule per row, one-stroke hairline icons) - not icon cards.
- **Method**: 4-column numbered sequence, big light mono numerals, heavy top
  rule; the numbers carry real order.
- **Colophon**: about-as-imprint + contact form with underline-only ledger
  fields; accent on focus and the submit button.

## Motion

One authored on-load moment: hero content rises (12px, ease-out) and rules draw
in left-to-right, once. Content is fully visible without it; `prefers-reduced-
motion` disables it.

## Browser surfaces

Themed from the palette: selection (accent), focus-visible ring (accent), caret
(accent), custom scrollbar (rule colors), tabular-nums in the ledger.

## Corners / elevation

Squared: buttons `2px`, plates/fields `0`. No decorative shadows - depth is
rules, poche, and paper.

## Known synthetic assets (replace with real material)

The hero before/after figure is an inline **SVG stand-in** (SketchUp line-art ->
schematic photograph), labeled as a study. Replace with a real screenshot +
render pair when available; see PRODUCT.md > Evidence on Hand.
