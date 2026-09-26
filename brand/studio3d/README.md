# Studio3D visual identity

StudioIA uses the visual identity of Studio3D, the architectural visualization
studio behind it. The identity was designed in 2021 as a specialization thesis
("Redesenho da Identidade Visual do Studio3D", Carolina Ferreira da Costa
Carvalho, UFRGS), whose appendix is the brand manual.

The thesis PDF is kept locally in this folder and is not committed
(`brand/**/*.pdf` in `.gitignore`).

## Files

- `logo-vertical.png`, `logo-horizontal.png` - the symbol with the Studio3D
  wordmark, as supplied.
- `texture.png` - the tile texture, as supplied.

The landing page does not use these rasters directly. Clean vectors are built
from them by `scripts/brand-assets/build-symbol.py` (symbol, favicon) and
`scripts/brand-assets/build-texture.py` (texture, full color and tint).

## Rules from the manual

- **Colors:** Orange `#C25939` (Pantone 173U), Navy `#263E5A` (282U), Teal
  `#18726E` (329U), Red `#A53B3F` (1805U), Black `#2A2929` (Natural Black).
  Tints: `#F6E6E1`, `#DFE2E6`, `#DDEAE9`, `#F2E2E2`, `#DADADA`.
- **Typeface:** D-DIN, regular, with no change to letter spacing. The wordmark
  is "Studio" in regular and the second word in bold, both in the dark color.
- **Symbol:** the black piece stands for projects still in technical language;
  the colored pieces for the same project translated into images.
- **Signatures:** symbol before the wordmark (site, stationery) or above it
  (social media, optionally with the slogan "Visualização em Arquitetura").
  On dark grounds the black piece and the wordmark turn light.
- **Watermark:** the one-color symbol, 50% opacity, bottom-right corner of
  images.
- **Don't:** change the logo's colors or font, put it on a solid color block,
  place it around the edges of an image, or use the full logo as the
  identification on images.
- **Texture:** the symbol's pieces in varied positions; the tint version is
  for materials where the content must stay in front.
