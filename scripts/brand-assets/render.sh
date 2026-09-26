#!/usr/bin/env bash
# Regenerates the landing page's raster brand assets from the HTML sources in
# this folder, using headless Chrome for rendering and Pillow for resizing:
#
#   og-image.html -> landing/og-image.png          (1200x630 social share card)
#   icon.html     -> landing/apple-touch-icon.png  (180x180)
#                    landing/icon-192.png, landing/icon-512.png (web manifest)
#                    landing/favicon.ico           (16/32/48, for /favicon.ico requests)
#
# Both sources draw the symbol from build-symbol.py's output (landing/symbol.svg,
# and the inline copy it syncs), so run that first if the mark changed. The
# vector favicon (landing/favicon.svg) also comes from build-symbol.py.
# Run after changing the brand mark, the headline, or the hero illustration.
#
# Usage:
#   ./scripts/brand-assets/render.sh
# Needs: Google Chrome (or set CHROME), python3 with Pillow.

set -euo pipefail

cd "$(dirname "$0")"
here="$(pwd)"
out="$here/../../landing"
chrome="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

shoot() { # <source.html> <width> <height> <out.png>
  "$chrome" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
    --default-background-color=00000000 --virtual-time-budget=2000 \
    --window-size="$2,$3" --screenshot="$4" "file://$here/$1" >/dev/null 2>&1
}

shoot og-image.html 1200 630 "$tmp/og.png"
shoot icon.html 512 512 "$tmp/icon.png"

python3 - "$tmp" "$out" <<'PY'
import sys
from PIL import Image

tmp, out = sys.argv[1], sys.argv[2]

og = Image.open(f"{tmp}/og.png").convert("RGB")
assert og.size == (1200, 630), og.size
og.save(f"{out}/og-image.png", optimize=True)

icon = Image.open(f"{tmp}/icon.png").convert("RGB")
assert icon.size == (512, 512), icon.size
icon.save(f"{out}/icon-512.png", optimize=True)
icon.resize((192, 192), Image.LANCZOS).save(f"{out}/icon-192.png", optimize=True)
icon.resize((180, 180), Image.LANCZOS).save(f"{out}/apple-touch-icon.png", optimize=True)
icon.save(f"{out}/favicon.ico", sizes=[(16, 16), (32, 32), (48, 48)])
PY

echo "Wrote og-image.png, icon-512.png, icon-192.png, apple-touch-icon.png, favicon.ico to landing/"
