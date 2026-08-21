#!/bin/sh
# Regenerate assets/static/og-image.png — the share card behind the og:image and
# twitter:image tags on landing.html and team.html.
#
#   sh internal/webui/ogimage.sh
#
# The card's source is ogimage.html beside this script; read its comment for why the
# card is HTML-and-a-browser rather than an SVG. This script only shoots it.
#
# ⚠️ 1200x630 is stated in three places: here, in ogimage.html's body rule, and in the
# og:image:width / og:image:height meta tags on landing.html and team.html. Changing
# one means changing all three, or a scraper will letterbox the card.
#
# ⚠️ Chromium on the development host is a strictly-confined snap. It can read and
# write only under $HOME, and NOT under dot-directories — so a checkout at
# ~/.cache/..., ~/.local/... or any hidden agent scratch directory fails with a bare
# "Permission denied (13)" that reads as a broken browser rather than a wrong path.
# If you hit that, move the checkout somewhere non-hidden under $HOME and re-run. The
# same constraint applies to the layout goldens in visual_test.go.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
out="$here/assets/static/og-image.png"

# --force-device-scale-factor=1 keeps the shot at exactly 1200x630 on a HiDPI host;
# without it the window size is multiplied and the card comes out oversized.
chromium \
	--headless \
	--disable-gpu \
	--no-sandbox \
	--hide-scrollbars \
	--force-device-scale-factor=1 \
	--default-background-color=00000000 \
	--window-size=1200,630 \
	--screenshot="$out" \
	"file://$here/ogimage.html"

# Strip the metadata chromium stamps in, so a rebuild that changed no pixels also
# changes no bytes and stays out of the diff.
convert "$out" -depth 8 -strip "$out"

identify "$out"
