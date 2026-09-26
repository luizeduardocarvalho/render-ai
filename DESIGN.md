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

- Wordmark: "Studio" regular + "IA" bold, like "Studio" + "3D" in the logo.
  Always set as live text next to the inline symbol.
- Headings: regular weight, large (hero `clamp(40px, 5vw, 64px)`, sections
  `clamp(30px, 3.6vw, 44px)`); one key word may go bold, echoing the wordmark.
- Labels: 13px uppercase, +0.08em.
- Body: 16-20px, `--ink-2` for secondary text.

## Composition

- **Wrap:** centered, `max-width 1200px`, gutter `clamp(16px, 5vw, 64px)`.
- **Header:** sticky, translucent white, symbol + wordmark left, nav and a
  small primary button right.
- **Hero:** headline, lede, two buttons, and three facts (label over value,
  a brand-color tick above each) beside the before/after figure.
- **Texture band:** full-bleed full-color texture between the hero and the
  first dark band, as on the manual's cover.
- **Capabilities:** dark band, three columns, a brand-color 3px rule over each.
- **How it works:** white, four numbered steps, numbers in the brand colors.
- **Contact:** dark band; about text with a fact list, and the form with white
  fields, as in the manual's site mockup.
- **Footer:** the tint texture as a thin strip, then the small print.

## Symbol, watermark, texture

- The symbol is inline once per page (`<symbol id="brand-symbol">`, synced by
  `scripts/brand-assets/build-symbol.py`) and used with `<use>`. Each piece
  reads a CSS variable, so dark mode turns the black piece light (the manual's
  dark-ground signature) and the watermark turns every piece white.
- **Watermark:** every product image carries the one-color symbol at 50%
  opacity in its bottom-right corner (manual rule). Image tags go top.
- Never place the full logo on a solid color block, around an image's edges, or
  as the identification inside an image (manual's incorrect uses).
- Texture: `texture.svg` (full color) for bands; `texture-tint.svg` for quiet
  strips on light grounds. On dark grounds the tint would glare, so the strip
  uses the full-color texture at 35% opacity.

## Shape and depth

Rounded, echoing the symbol's pieces: buttons and tags are pills, the figure
has 20px corners, form fields 12px. No decorative shadows, except the compare
grip, which needs to read over any image.

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
