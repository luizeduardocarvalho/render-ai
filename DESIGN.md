# Design

Visual world for the StudioIA **landing page** (`landing/`). Scope: this
marketing surface only. The in-app tool (`frontend/`) keeps its own indigo
system for now; this document does not govern it.

## World

**Studio3D visual identity.** StudioIA is built by Studio3D, an architectural
visualization studio, and wears its identity: the manual in
[`brand/studio3d/`](brand/studio3d/README.md) is the authority, and this
document is how the landing page applies it. The page follows the manual's own
site layout: a light header over the hero, a dark band for the services, the
work on plain white, a dark band for contact. The brand's tile texture, built
from the symbol's pieces, ties the sections together.

The symbol's story is the product's: the black piece is a project still in
technical language, the colored pieces the same project translated into images.

## Color

Manual colors, never recolored in the symbol or wordmark:

| Token | Value | Use |
|---|---|---|
| `--brand-black` | `#2A2929` | text, dark bands, primary button |
| `--brand-orange` | `#C25939` | large headings, first separator/tick |
| `--brand-teal` | `#18726E` | links, focus, selection, separators |
| `--brand-red` | `#A53B3F` | separators, step numbers |
| `--brand-navy` | `#263E5A` | separators, step numbers |

Page tokens:

| Token | Light | Dark |
|---|---|---|
| `--bg` | `#FFFFFF` | `#1E1D1D` |
| `--ink` | `#2A2929` | `#F3F1F0` |
| `--ink-2` (secondary) | `#5B5858` | `#C9C5C3` |
| `--ink-3` (labels) | `#6E6A68` | `#A8A3A0` |
| `--rule` | `#DADADA` | `#3A3838` |
| `--link` / `--focus` | `#18726E` | `#7CC4BD` |
| `--band` (dark sections) | `#2A2929` | `#2A2929` |

Contrast rules (WCAG AA, checked):

- **Orange is 4.4:1 on white** (3.9:1 on the dark ground), so it is for large
  text only: section headings of 30px and up, step numbers. Never body text,
  labels or button text.
- Teal (5.7:1) and red (6.4:1) pass for small text on white. On dark grounds,
  teal, navy and red are lifted (`#7CC4BD`, `#6F93BD`, `#D0676B`) wherever they
  carry meaning; the symbol itself keeps the manual colors.
- Color is structure, not emphasis: the four brand colors appear as the symbol,
  the texture, 3px separators and ticks, and the step numbers, cycling
  orange, teal, navy, red.

## Type

**D-DIN** (SIL Open Font License, `landing/fonts/`, `D-DIN-OFL.txt`), regular
400 and bold 700, default letter spacing as the manual requires. It covers
Portuguese; it has no arrow glyphs, so arrows are drawn (the compare grip).

- Wordmark: matched to the logo - "Studio" regular, tracked +0.06em, a small
  gap, then "IA" bold with a thin stroke (the logo's "3D" is heavier than the
  stock bold), tracked -0.04em. Always live text next to the inline symbol.
- Headings: regular weight, large (hero `clamp(40px, 5vw, 64px)`, sections
  `clamp(30px, 3.6vw, 44px)`); one key word may go bold, echoing the wordmark.
- Labels: 13px uppercase, +0.08em.
- Body: 16-20px, `--ink-2` for secondary text.

## Composition

- **Wrap:** centered, `max-width 1200px`, gutter `clamp(16px, 5vw, 64px)`.
- **Header:** sticky, translucent white, symbol + wordmark left; nav, the
  language switch (EN/PT pill), the light/dark button and a small primary
  button right.
- **Hero:** headline, lede, two buttons, and three facts (label over value,
  a brand-color tick above each) beside the before/after figure. The hero and
  the texture band under it fill the first screen, so the next section starts
  at the fold.
- **Texture band:** full-bleed full-color texture between the hero and the
  first dark band, as on the manual's cover.
- **Capabilities:** dark band, three columns, a brand-color 3px rule over each.
- **How it works:** white, four numbered steps, numbers in the brand colors.
- **Contact:** dark band; about text with a fact list, and the form with white
  fields, as in the manual's site mockup.
- **Footer:** the full-color texture repeated small (180px tile, two rows,
  18px) as a strip; then a brand column (mark, one-line description, "A
  Studio3D product") and Product / Company (with the Studio3D Instagram) /
  Language link columns; then a bottom row with the copyright and "Back to
  top".
- **About page:** intro (headline + lede), the texture band, a Studio3D
  section with the symbol large beside its meaning, a product gallery (one
  large and two small app screenshots, 16px frames, watermark), the principles
  on a dark band, and a centered call to action.

Copy uses no dashes as punctuation (commas, colons, periods instead); page
titles use " | " as the separator.

## Symbol, watermark, texture

- The symbol is inline once per page (`<symbol id="brand-symbol">`, synced by
  `scripts/brand-assets/build-symbol.py`) and used with `<use>`. Each piece
  reads a CSS variable, so dark mode turns the black piece light (the manual's
  dark-ground signature) and the watermark turns every piece white.
- **Watermark:** every product image carries the one-color symbol at 50%
  opacity in its bottom-right corner (manual rule). Image tags go top.
- Never place the full logo on a solid color block, around an image's edges, or
  as the identification inside an image (manual's incorrect uses).
- Texture: `texture.svg` (full color) for the band and the footer strip;
  `texture-tint.svg` for quiet backgrounds on light grounds only (on dark it
  would glare).

## Shape and depth

Rounded, echoing the symbol's pieces: buttons and tags are pills, the figure
has 20px corners, form fields 12px. No decorative shadows, except the compare
grip, which needs to read over any image.

## Theme and language

Colors are `light-dark()` tokens: the page follows the system, and the header
button forces a theme by setting `data-theme` on `<html>` (saved in
`localStorage`, applied before first paint). The page ships in English and
Portuguese from one template (`scripts/landing/`); D-DIN covers Portuguese.

## Motion

One on-load moment: hero content and figure rise 12px, once. Everything is
visible without it; `prefers-reduced-motion` disables it.

## Browser surfaces

Selection in teal with white text, focus rings in `--focus` (teal, lifted on
dark), caret in teal.

## Known synthetic assets (replace with real material)

The hero before/after figure is an inline **SVG stand-in** (model line-art ->
schematic photograph). Replace it with a real screenshot + render pair when
available; see PRODUCT.md > Evidence on Hand.
