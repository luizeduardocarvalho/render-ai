# App design (frontend/)

The signed-in app wears the same Studio3D visual identity as the landing page.
Read these first - they are the authority, this document applies them to the
app:

- [`../brand/studio3d/README.md`](../brand/studio3d/README.md) - the brand rules
  (colors, D-DIN, symbol, watermark, don'ts).
- [`../DESIGN.md`](../DESIGN.md) - how the landing page applies them. Open
  `landing/index.html` in a browser (light and dark) as the visual reference.

**Mode: Operate.** The landing page persuades; the app is a tool people work in
for hours. Scanability, consistency and calm come first; the brand shows in
precise details (type, the symbol, color used as signal), not decoration. No
texture bands inside the working screens.

## Scope

Visual only. Do not change behavior, routes, API calls, copy, or i18n keys
(new keys only if a new label is truly needed, added to both `en.json` and
`pt-BR.json`). Tests, `pnpm lint`, `pnpm run check:i18n` and `pnpm build`
must stay green.

## Tokens (index.css)

The app is already built on tokens in `src/index.css`. Keep the token names the
components use (`--bg`, `--surface`, `--border`, `--text`, `--text-muted`,
`--accent`, `--danger`, ...) and remap their values. Move every color token to
`light-dark(light, dark)` (as the landing page does) so a `data-theme`
attribute on `<html>` can force a theme (see Theme below); remove the
`@media (prefers-color-scheme: dark)` token block.

Brand constants (never recolor the symbol):

| Brand | Hex |
|---|---|
| black | `#2A2929` |
| orange | `#C25939` |
| teal | `#18726E` |
| red | `#A53B3F` |
| navy | `#263E5A` |

Neutrals are warm (they sit next to the brand black), not the current cool
grays:

| Token | Light | Dark |
|---|---|---|
| `--bg` (app background) | `#F6F5F4` | `#1E1D1D` |
| `--surface` (panels, cards) | `#FFFFFF` | `#2A2929` |
| `--surface-raised` (menus, modals) | `#FFFFFF` | `#333131` |
| `--border` | `#E4E1DF` | `#3A3838` |
| `--border-strong` | `#C9C5C2` | `#4F4C4B` |
| `--text` | `#2A2929` | `#F3F1F0` |
| `--text-muted` | `#5B5858` | `#C9C5C3` |
| `--text-faint` (placeholders, meta) | `#6E6A68` | `#A8A3A0` |

Rebuild the `--gray-*` scale on these warm values (0 = surface ... 900 = most
contrast) so components that use grays directly follow.

Signal colors:

| Token | Light | Dark | Use |
|---|---|---|---|
| `--accent` | `#18726E` (teal) | `#5FB3AC` | selection and active states: active segment, selected view tab, selected render row, focus ring, links, sliders, checkboxes |
| `--accent-contrast` | `#FFFFFF` | `#1E1D1D` | text on `--accent` (5.7:1 and 6.9:1) |
| `--accent-hover` / `--accent-active` | `#135F5B` / `#0F4F4C` | `#74C2BB` / `#4FA39C` | |
| `--accent-soft`, `--accent-soft-strong`, `--accent-border` | teal at 8% / 14% / 35% | `#5FB3AC` at 14% / 22% / 40% | |
| `--danger` | `#A53B3F` (brand red, 6.4:1) | `#E07A7E` | |
| `--success` | `#2F6B4F` | `#6CBB92` | |
| `--warning` | `#9A4A2E` (orange darkened for text, 6:1) | `#E39A7E` | |

Keep the `*-soft` variants of danger/success/warning at the same alphas as
today, recomputed from the new colors.

**Primary buttons are brand black, not the accent** - exactly like the landing
page: `.btn-primary` = `#2A2929` bg / white text (hover `#444141`); in dark,
`#F3F1F0` bg / `#2A2929` text (hover `#FFFFFF`). Add `--btn-primary-bg`,
`--btn-primary-ink`, `--btn-primary-hover` tokens for this. The accent is for
"selected / active / focus", never for the main call to action.

Orange is 4.4:1 on white: only for large text (24px+) or non-text marks. The
mask/region colors in the editor are functional (they tell regions apart) -
leave them as they are.

## Type

- **D-DIN** 400 and 700 for everything. Import the two `.woff2` files from
  `src/assets/fonts/` (copy them from `landing/fonts/`, plus `D-DIN-OFL.txt`) in
  `index.css` with `@font-face` (`font-display: swap`) so Vite fingerprints
  them. `--sans: "D-DIN", "DIN Alternate", "Bahnschrift", "Roboto Condensed",
  Arial, sans-serif`.
- D-DIN is narrow with a small x-height: raise the base size from 14px to
  **15px** and keep line-height 1.5. Check dense areas (tables, chips, the
  metrics panel) still fit.
- **Textareas and inputs use the sans font.** Today the Style panel's
  textareas render in monospace - that goes. Keep `--mono` only where a value
  is genuinely code-like (ids); numbers use `font-variant-numeric:
  tabular-nums` instead.
- Headings (`.panel-title`, page titles): D-DIN regular for large titles
  (project list "Your projects" at 30px), bold for panel titles (16-17px).
- Field labels (`.field-label`, "CENA"/"SCENE"): 12.5px, uppercase, +0.08em,
  `--text-faint` - the landing page's label style.

## Shape and depth

- Buttons, segmented controls, chips and badges: **pills** (`999px`), like the
  landing page. Button height 38px (sm 32px), padding 0 16px, weight 700.
- Panels and cards: `16px` radius, 1px `--border`, no shadow (`--shadow-sm`
  may stay on floating things only).
- Inputs, selects, textareas: `12px` radius, 1px `--border-strong`, focus =
  2px `--accent` ring (outline, offset 1px) - no glow.
- Modals, menus, the notifications panel: `--surface-raised`, `16px` radius,
  `--shadow-lg`.

## Components

- **Segmented control** (`.segmented`): a pill track (`--bg` fill, 1px
  `--border`, 3px inner padding) with pill segments; the active segment is
  `--accent` with `--accent-contrast` text. Same for the language switcher.
- **Badges**: pills, 12px, bold; soft background + strong text of their color.
- **Credits chip**: pill, tabular numbers.
- **View tabs** (`.view-tab`): 16px radius cards; selected = 2px `--accent`
  border (not a filled indigo tint).
- **Render history** selected row: `--accent-soft` background + a 3px
  `--accent` left bar.
- **Empty states**: centered, `--text-muted`, may use the one-color symbol at
  32px above the text (see Brand mark), never an illustration.
- **Error banner**: `--danger-soft` background, `--danger` text, 12px radius.
  (Today an API failure shows the raw message twice - banner and inline - on
  the project list; that is behavior, leave it, but note it in your report.)

## Header and brand mark

Every screen's top bar (`.app-topbar`, the project picker's header, admin,
not-authorized, auth screens) shows the **symbol + wordmark** instead of the
plain "StudioIA" text:

- Create `src/components/BrandMark.tsx` rendering the symbol as inline SVG (the
  five paths from `landing/symbol.svg`, each `fill` read from a CSS variable
  with the manual color as fallback: `--sym-black`, `--sym-orange`,
  `--sym-teal`, `--sym-red`, `--sym-navy`), 26px tall in the top bar, and the
  wordmark next to it: "Studio" D-DIN 400 with `letter-spacing: 0.06em`, then
  "IA" D-DIN 700, `letter-spacing: -0.04em`, `margin-left: 0.1em`,
  `-webkit-text-stroke: 0.035em currentColor` - copy the landing page's
  `.brand-name` rules. 21px in the top bar. Links to `/`.
- Dark mode: `--sym-black` becomes `#F3F1F0` (the manual's dark signature);
  the other pieces keep their colors. A one-color variant (all pieces
  `currentColor`) is what empty states use.
- Top bar: `--surface` background, 1px bottom `--border`, 60px tall; the
  project name and "Switch project" stay as today, restyled (project name
  `--text-muted`, separator `/` in `--text-faint`).
- Favicon: already the new symbol (`public/favicon.svg`); keep it.

## Theme

Add the landing page's light/dark button to the top bar, next to the language
switcher: a 34px round icon button (moon icon in light, sun in dark), toggling
`data-theme="light|dark"` on `<html>`, saved in `localStorage` under the key
`theme` (the same key the landing page uses), applied before first paint by a
small inline script in `index.html`. Without a saved choice the app follows
the system. Labels via i18n (`theme.toDark`, `theme.toLight` - add to both
locale files: "Switch to dark mode" / "Mudar para o modo escuro", etc.).

## Clerk

Update `src/lib/clerkAppearance.ts` so the sign-in, sign-up and user menu
match: D-DIN, brand black primary button (pill), teal focus/links, 16px card
radius, warm neutrals, dark variant. `AuthLayout` shows the BrandMark above
the Clerk card and may use the landing page's full-color texture as a band at
the top or bottom of the auth screen (the one place the texture is welcome in
the app).

## Screens to check

With the harness (see the task brief), check each in **light and dark**, at
**1440x900** and **390x844**:

1. Project list - Projects tab and Library tab (with cards), "New project".
2. Workspace - Style panel, asset library in the sidebar, views bar, mask
   editor with its toolbar, render controls, render history / results.
3. Notifications panel open, a toast.
4. A confirm dialog / modal.
5. Admin page.
6. Sign-in screen (stubbed Clerk shows a placeholder box - check the frame
   around it).

On phones the top bar may wrap to two rows (as today) but must not overflow
horizontally; the page must never scroll sideways.

## Done means

- All screens above look like one family with the landing page.
- No hard-coded indigo (`#4338ca`, `#8b7ef0`, `rgba(67, 56, 202`, `rgba(139,
  126, 240`) or cool grays left in `src/` (grep).
- Text contrast AA everywhere (4.5:1 body, 3:1 large); focus visible.
- Tests, lint, i18n check and build green.
