#!/bin/sh
# Renders the i2a icon (internal/notify/assets/icon.svg) to icon.png.
# terminal-notifier shows icon.png as the image of each notification.
# Needs rsvg-convert (brew install librsvg).
set -eu
dir="$(cd "$(dirname "$0")/.." && pwd)/internal/notify/assets"
rsvg-convert -w 256 -h 256 "$dir/icon.svg" -o "$dir/icon.png"
echo "wrote $dir/icon.png"
