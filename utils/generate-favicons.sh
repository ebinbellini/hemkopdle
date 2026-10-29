#!/usr/bin/env bash
#
# generate-favicons.sh -- build the favicon set for Hemkopdle.
#
#   source : utils/Hemköpdle favicon.svg     (red "H:-" lockup, 32x32 viewBox)
#   output : web_static/icons/
#
#     favicon.svg              vector favicon (transparent, editor metadata stripped)
#     favicon.ico              16+32+48 px
#     favicon-16x16.png        
#     favicon-32x32.png
#     favicon-48x48.png
#     favicon-96x96.png
#     apple-touch-icon.png     180 px, white mark on brand red (iOS masks the corners)
#     icon-192.png             white mark on brand red (Android / manifest)
#     icon-512.png             white mark on brand red
#     icon-maskable-512.png    same, shrunk into the 80% maskable safe zone
#     manifest.json            web app manifest, linked from server/templates/layout.html
#                              (plain .json so every static server sends a JSON type;
#                               .webmanifest often ends up as text/plain)
#
# Requires: rsvg-convert (librsvg), ImageMagick 7 (`magick`), scour (optional).
#
# Usage: utils/generate-favicons.sh
#
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
root_dir=$(dirname -- "$script_dir")

src_svg="$script_dir/Hemköpdle favicon.svg"
out_dir="$root_dir/web_static/icons"

brand_red='#ee1c2e'   # the red used inside the SVG itself
raster=1024           # internal render size; every icon is downscaled from this

die() { printf 'error: %s\n' "$*" >&2; exit 1; }

for tool in rsvg-convert magick; do
	command -v "$tool" >/dev/null 2>&1 || die "'$tool' not found in PATH"
done
[ -f "$src_svg" ] || die "source SVG not found: $src_svg"

mkdir -p "$out_dir"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# ------------------------------------------------------------------ clean SVG
# Strip the Inkscape/Sodipodi/RDF editor baggage so the shipped file is small.
if command -v scour >/dev/null 2>&1; then
	scour -i "$src_svg" -o "$work/scour.svg" \
		--enable-viewboxing --remove-metadata --indent=none >/dev/null
	# scour drops the intrinsic size; put it back so the file also works as a
	# plain <img src="...">.
	sed 's|<svg |<svg width="32" height="32" |' "$work/scour.svg" >"$work/clean.svg"
else
	printf 'warning: scour not found -- shipping the unoptimised SVG\n' >&2
	cp "$src_svg" "$work/clean.svg"
fi

# ----------------------------------------------------------------- rasterise
rsvg-convert -w "$raster" -h "$raster" "$work/clean.svg" -o "$work/full.png"
# Trim the empty canvas so every icon is sized relative to the artwork itself.
# The full "H:" lockup is used at every size, including 16 px.
magick "$work/full.png" -trim +repage "$work/mark.png"

# White copy of the mark for the app icons, which sit on a red tile.
sed "s/$brand_red/#ffffff/g" "$work/clean.svg" >"$work/white.svg"
rsvg-convert -w "$raster" -h "$raster" "$work/white.svg" -o "$work/white-full.png"
magick "$work/white-full.png" -trim +repage "$work/white-mark.png"

# ------------------------------------------------------------------- helpers
# square <src.png> <px> <fill-fraction> <out.png> [background]
#
# Scales <src> so its long edge is <px>*<fill-fraction> and centres it on a
# <px>x<px> canvas. Background defaults to transparent.
square() {
	local src=$1 px=$2 frac=$3 out=$4 bg=${5:-none} box
	box=$(awk -v p="$px" -v f="$frac" 'BEGIN { printf "%d", p * f }')
	if [ "$bg" = none ]; then
		magick "$src" -filter Lanczos -resize "${box}x${box}" \
			-background none -gravity center -extent "${px}x${px}" \
			-strip "$out"
	else
		magick "$src" -filter Lanczos -resize "${box}x${box}" \
			-background "$bg" -gravity center -extent "${px}x${px}" \
			-alpha remove -alpha off -strip "$out"
	fi
	printf '  %-24s %sx%s\n' "${out#"$out_dir/"}" "$px" "$px"
}

# -------------------------------------------------------------------- outputs
printf 'Generating favicons in %s\n' "${out_dir#"$root_dir/"}"

cp "$work/clean.svg" "$out_dir/favicon.svg"
printf '  %-24s vector\n' favicon.svg

square "$work/mark.png" 16 0.95 "$out_dir/favicon-16x16.png"
square "$work/mark.png" 32 0.95 "$out_dir/favicon-32x32.png"
square "$work/mark.png" 48 0.95 "$out_dir/favicon-48x48.png"
square "$work/mark.png" 96 0.92 "$out_dir/favicon-96x96.png"

square "$work/white-mark.png" 180 0.66 "$out_dir/apple-touch-icon.png" "$brand_red"
square "$work/white-mark.png" 192 0.66 "$out_dir/icon-192.png" "$brand_red"
square "$work/white-mark.png" 512 0.66 "$out_dir/icon-512.png" "$brand_red"
square "$work/white-mark.png" 512 0.56 "$out_dir/icon-maskable-512.png" "$brand_red"

# Legacy multi-resolution icon; every browser since IE11 accepts PNG-compressed
# entries, so no BMP fallback is needed.
magick "$out_dir/favicon-16x16.png" "$out_dir/favicon-32x32.png" \
	"$out_dir/favicon-48x48.png" -strip "$out_dir/favicon.ico"
printf '  %-24s %s\n' favicon.ico \
	"$(magick identify -format '%w ' "$out_dir/favicon.ico" | sed 's/ $//; s/ /+/g')"

cat >"$out_dir/manifest.json" <<'JSON'
{
  "name": "Hemköpdle",
  "short_name": "Hemköpdle",
  "description": "Gissa priset på dagens vara från Hemköp.",
  "lang": "sv",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "background_color": "#f8f7f6",
  "theme_color": "#ee1c2e",
  "icons": [
    { "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/icons/icon-512.png", "sizes": "512x512", "type": "image/png" },
    {
      "src": "/icons/icon-maskable-512.png",
      "sizes": "512x512",
      "type": "image/png",
      "purpose": "maskable"
    }
  ]
}
JSON
printf '  %-24s manifest\n' manifest.json

cat <<'HTML'

Reference them from the page <head> (done in server/templates/layout.html):

  <link rel="icon" href="/icons/favicon.ico" sizes="32x32">
  <link rel="icon" href="/icons/favicon.svg" type="image/svg+xml">
  <link rel="apple-touch-icon" href="/icons/apple-touch-icon.png">
  <link rel="manifest" href="/icons/manifest.json">
  <meta name="theme-color" content="#ee1c2e">
HTML
