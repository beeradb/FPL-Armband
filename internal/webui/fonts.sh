#!/usr/bin/env bash
#
# Regenerate assets/static/fonts.css and assets/static/fonts/ from Google Fonts.
#
# This is the only thing in the tree that reaches the network to build an asset, so it is
# a script you run deliberately rather than a step in the build. The output is committed;
# a machine without a network can still build, test and serve.
#
# It ships latin and latin-ext only. latin-ext is not optional -- see the comment at the
# top of the generated fonts.css.
#
# Usage: internal/webui/fonts.sh   (from the repository root)

set -euo pipefail

cd "$(dirname "$0")/../.."

FAMILIES='family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500;700&display=swap'
# A modern desktop UA, because the css2 endpoint serves woff2 only to browsers it
# recognises. curl's default UA gets a truetype fallback several times the size.
UA='Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36'

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

curl -sSf -A "$UA" "https://fonts.googleapis.com/css2?${FAMILIES}" -o "$work/gfonts.css"

GFONTS_CSS="$work/gfonts.css" python3 internal/webui/fontsubset.py

echo "regenerated internal/webui/assets/static/fonts.css and assets/static/fonts/"
