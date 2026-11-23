#!/bin/zsh
# Build the Windows exes and the zip to hand over. Usage: ./build.sh 1.0.0
set -e
version=${1:-dev}
cd "${0:A:h}"
go test ./...
mkdir -p dist

# Icon, version and manifest go in rsrc_windows_*.syso, which go build picks
# up for the matching GOARCH. They carry the version, so they are made here.
# go-winres is pinned as a tool in go.mod.
winres() {
	go tool go-winres make --in $1 --arch $2 --out cmd/tribuna/rsrc \
		--file-version $version --product-version $version
}
winres winres/winres.json amd64
winres winres/winres-386.json 386

# Without TRIBUNA_UPLOAD_TOKEN the exes never send the game logs.
# The token ends up readable inside the exe. The log server (fpb-calendar)
# accepts it only on POST /api/marcador/logs. To rotate it: set a new token on
# the server, build with the same value, and hand over the new zip. Until the
# club swaps the exe its uploads get 401 and stay queued in dados/envios/ for
# up to 60 days, and the new exe sends them.
flags="-s -w -H windowsgui -X main.version=$version -X main.uploadToken=$TRIBUNA_UPLOAD_TOKEN"
GOOS=windows GOARCH=amd64 go build -ldflags $flags -o "dist/Tribuna.exe" ./cmd/tribuna
GOOS=windows GOARCH=386 go build -ldflags $flags -o "dist/Tribuna (32-bit).exe" ./cmd/tribuna
rm -f cmd/tribuna/rsrc_windows_*.syso

/bin/cp -f LEIA-ME.txt dist/LEIA-ME.txt
rm -rf dist/YouTube
mkdir -p dist/YouTube
# A .txt opens with a double click on Windows; the guide is Markdown-light.
/bin/cp -f docs/youtube/SETUP.md dist/YouTube/LIGAR-YOUTUBE.txt
rm -f dist/Tribuna-$version.zip
(cd dist && zip -q -X -9 Tribuna-$version.zip "Tribuna.exe" "Tribuna (32-bit).exe" LEIA-ME.txt YouTube/LIGAR-YOUTUBE.txt)
ls -lh dist
