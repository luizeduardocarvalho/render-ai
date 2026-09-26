# StudioIA landing page

A standalone marketing page for StudioIA. Static HTML/CSS/JS in [`landing/`](landing/),
deployed on **Firebase Hosting**, with the contact form writing straight to
**Cloud Firestore**.

```
scripts/landing/
  index.html   the page template ({{key}} placeholders)
  strings/     en.json, pt-BR.json - the page text per language
  build.py     writes landing/index.html (/) and landing/pt-br/index.html (/pt-br)
landing/
  index.html, pt-br/index.html
               generated from the template - don't edit by hand
  styles.css   Studio3D identity tokens + layout (light/dark) - see DESIGN.md
  ui.js        before/after slider, footer year
  contact.js   Firebase init + Firestore write for the contact form (SDK lazy-loaded)
  404.html     served by Firebase Hosting for any missing URL
  robots.txt, sitemap.xml, site.webmanifest
  fonts/       D-DIN regular + bold (SIL OFL, license alongside)
  generated - don't edit by hand:
    symbol.svg, favicon.svg, the inline <symbol> in *.html
                 scripts/brand-assets/build-symbol.py
    texture.svg, texture-tint.svg
                 scripts/brand-assets/build-texture.py (needs potrace)
    og-image.png, apple-touch-icon.png, icon-*.png, favicon.ico
                 scripts/brand-assets/render.sh (needs Chrome + Pillow)
firebase.json  Hosting + Firestore rules config
firestore.rules  create-only, validated contact messages
.firebaserc    Firebase project alias
brand/studio3d/  the supplied Studio3D logos and texture, and the brand rules
```

The page is fully self-contained and follows Studio3D's visual identity
(`brand/studio3d/README.md`), applied as described in `DESIGN.md`, including
dark mode.

## Editing the page

The page exists in English (`/`) and Portuguese (`/pt-br`), built from one
template so the two cannot drift apart. Edit `scripts/landing/index.html` for
structure and `scripts/landing/strings/*.json` for text (every language must
have the same keys - the build fails otherwise), then run:

```bash
python3 scripts/landing/build.py
```

and commit the regenerated pages. The header's language switch links the two
versions; `hreflang` alternates are in each page's `<head>` and in
`sitemap.xml`. The light/dark button saves the visitor's choice in
`localStorage`; without a choice the page follows the system setting.

## Brand assets

The brand only exists as raster files, so the page's vectors are rebuilt from
them. After changing the mark or the texture source, run in order:

```bash
python3 scripts/brand-assets/build-symbol.py   # also syncs the symbol into the template
python3 scripts/landing/build.py
nix shell nixpkgs#potrace -c python3 scripts/brand-assets/build-texture.py   # or any potrace on PATH
./scripts/brand-assets/render.sh
```

## Search and link previews

The page's `<head>` carries the canonical URL, Open Graph / X card tags, and
schema.org JSON-LD. All of them, plus `robots.txt` and `sitemap.xml`, use the
absolute site URL `https://render-ai-studio.web.app`. When the custom domain is
live, replace that string everywhere it appears in `landing/`
(`grep -rl render-ai-studio.web.app landing`).

The share image and PNG icons are rendered from the HTML sources in
`scripts/brand-assets/`. After changing the headline or the hero illustration,
run `./scripts/brand-assets/render.sh` and commit the regenerated files.

The app site (`frontend/`) is deliberately kept out of search results: its
`index.html` has `<meta name="robots" content="noindex">` and `firebase.json`
sends `X-Robots-Tag: noindex` on every response.

## Preview locally

No build step - it's static files. Any static server works:

```bash
cd landing
python3 -m http.server 5000      # then open http://localhost:5000
```

(The Firebase Web SDK loads from Google's CDN, so previewing needs internet.)

## Connect Firebase (one-time)

1. Create a Firebase project at <https://console.firebase.google.com> (or reuse one).
2. Add a **Web app** to it and copy the SDK config values.
3. Enable **Cloud Firestore** (Build -> Firestore Database -> Create database,
   in production mode).
4. Fill in the placeholders:
   - `landing/contact.js` -> the `firebaseConfig` object (replace every `__FILL_ME__`).
   - `.firebaserc` -> your real project id in place of `__YOUR_FIREBASE_PROJECT_ID__`.

Until step 4 is done the form still validates input, but tells the visitor the
backend isn't connected yet instead of erroring.

## Deploy

Install the CLI once (`npm i -g firebase-tools`) and log in (`firebase login`).

```bash
firebase deploy --only firestore:rules,hosting
```

- `firestore:rules` publishes [`firestore.rules`](firestore.rules) - the form
  can only **create** well-formed `contactMessages` documents; reads/updates/
  deletes are denied to clients.
- `hosting` publishes everything in `landing/`.

## Read submissions

Contact messages land in the `contactMessages` collection. Browse them in the
Firebase console (Firestore Database), or pull them with the Admin SDK, which
bypasses the security rules. Each doc: `{ email, message, createdAt }`.
