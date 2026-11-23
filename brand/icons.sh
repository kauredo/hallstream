#!/bin/zsh
# Rebuild the app icon files from icon.svg (40 px and up) and icon-16.svg
# (16 to 32 px). Needs rsvg-convert and Go.
set -e
cd "${0:A:h}"
out=../web/assets/icon
tmp=$(mktemp -d)
trap 'rm -rf $tmp' EXIT
mkdir -p $out

png() { rsvg-convert -w $2 -h $2 -o $3 $1 }

# Full-bleed variants: iOS rounds the corners itself, and Android masks
# keep only the centre, so the maskable one has a smaller shield.
sed -e 's/ rx="56"//' icon.svg > $tmp/full.svg
sed -e 's/ rx="56"//' -e 's/scale(0.24)/scale(0.2)/' icon.svg > $tmp/maskable.svg

for s in 16 20 24 32; do png icon-16.svg $s $tmp/$s.png; done
for s in 40 48 64 256; do png icon.svg $s $tmp/$s.png; done
go run mkico.go $out/hallstream.ico $tmp/{16,20,24,32,40,48,64,256}.png

/bin/cp -f icon-16.svg $out/favicon.svg
/bin/cp -f $tmp/32.png $out/favicon-32.png
png $tmp/full.svg 180 $out/apple-touch-icon.png
png $tmp/maskable.svg 192 $out/icon-192.png
png $tmp/maskable.svg 512 $out/icon-512.png
ls -l $out
