#!/bin/sh
# Regenerate the raster icons from their SVG sources.
#
# The PNGs under assets/static are BUILD PRODUCTS. They are committed because the
# server embeds assets/ wholesale (see webui.go's //go:embed) and a build step that
# has to run before the binary is buildable would be a worse trade — but a committed
# binary with no generator is how an asset drifts from the drawing it came from. This
# script is that generator. Run it after ANY change to logo.svg, favicon.svg or
# icon.svg, and commit what it writes.
#
#   sh internal/webui/icons.sh
#
# Sizes, and why each exists:
#
#   icon-180.png   apple-touch-icon. iOS ignores SVG here and applies its own squircle
#                  mask, so the source must be the square, full-bleed cut.
#   icon-192.png   web manifest, the "any" purpose entry.
#   icon-512.png   web manifest, splash and install surfaces.
#   favicon-32.png the <link rel=icon> raster fallback, from the CIRCLE cut on a light
#                  ground — a favicon sits in browser chrome, not on our page, and a
#                  dark tile on a dark tab bar loses its container.
#
# ⚠️ Density, not just -resize. ImageMagick rasterises the SVG at -density FIRST and
# then samples down; rendering straight to 32px produces visibly broken curves on the
# C. 900 is comfortably above every target here.
#
# ⚠️ An XML comment may not contain a double hyphen. `convert` rejects the file
# outright with "Comment must not contain '--'", which is why the CSS custom-property
# spellings are written without their leading hyphens in all three SVG sources. A
# browser tolerates it; this step does not.
set -eu

cd "$(dirname "$0")/assets/static"

for size in 180 192 512; do
	convert -background none -density 900 icon.svg \
		-resize "${size}x${size}" -depth 8 -strip "icon-${size}.png"
done

convert -background none -density 900 favicon.svg \
	-resize 32x32 -depth 8 -strip favicon-32.png

echo "wrote icon-180.png icon-192.png icon-512.png favicon-32.png"
