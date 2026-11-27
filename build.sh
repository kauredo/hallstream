#!/bin/zsh
# Build what to hand over. Usage: ./build.sh 1.0.0 [windows|macos]
# windows (the default) makes the two exes and the zip. macos makes an
# ad-hoc-signed Hallstream.app and, unless HALLSTREAM_SIGN is set, the DMG;
# with it set, the release workflow signs the app and makes the DMG.
set -e
version=${1:-dev}
target=${2:-windows}
cd "${0:A:h}"
go test ./...
mkdir -p dist

# The texts, licences and guides that ship beside the program, into $1.
texts() {
	/bin/cp -f LEIA-ME.txt $1/LEIA-ME.txt
	/bin/cp -f READ-ME.txt $1/READ-ME.txt
	# The AGPL text and the licences of the code linked into the binaries ship with them.
	/bin/cp -f LICENSE $1/LICENSE.txt
	scripts/notices.sh $1/THIRD-PARTY-NOTICES.txt $2
	rm -rf $1/YouTube
	mkdir -p $1/YouTube
	# A .txt opens with a double click on Windows; the guide is Markdown-light.
	/bin/cp -f docs/youtube/SETUP.md $1/YouTube/LIGAR-YOUTUBE.txt
	/bin/cp -f docs/youtube/SETUP.en.md $1/YouTube/CONNECT-YOUTUBE.txt
	/bin/cp -f docs/cameras/GUIDE.md $1/CAMARAS.txt
	/bin/cp -f docs/cameras/GUIDE.en.md $1/CAMERAS.txt
}

if [[ $target == windows ]]; then

# Icon, version and manifest go in rsrc_windows_*.syso, which go build picks
# up for the matching GOARCH. They carry the version, so they are made here.
# go-winres is pinned as a tool in go.mod.
winres() {
	go tool go-winres make --in $1 --arch $2 --out cmd/hallstream/rsrc \
		--file-version $version --product-version $version
}
winres winres/winres.json amd64
winres winres/winres-386.json 386

# Without HALLSTREAM_UPLOAD_TOKEN the exes never send the game logs.
# The token ends up readable inside the exe. The log server (fpb-calendar)
# accepts it only on POST /api/marcador/logs. To rotate it: set a new token on
# the server, build with the same value, and hand over the new zip. Until the
# club swaps the exe its uploads get 401 and stay queued in dados/envios/ for
# up to 60 days, and the new exe sends them.
flags="-s -w -H windowsgui -X main.version=$version -X main.uploadToken=$HALLSTREAM_UPLOAD_TOKEN -X main.feedbackURL=$HALLSTREAM_FEEDBACK_URL"
GOOS=windows GOARCH=amd64 go build -ldflags $flags -o "dist/Hallstream.exe" ./cmd/hallstream
GOOS=windows GOARCH=386 go build -ldflags $flags -o "dist/Hallstream (32-bit).exe" ./cmd/hallstream
rm -f cmd/hallstream/rsrc_windows_*.syso

texts dist windows
rm -f dist/Hallstream-$version.zip
(cd dist && zip -q -X -9 Hallstream-$version.zip "Hallstream.exe" "Hallstream (32-bit).exe" LEIA-ME.txt READ-ME.txt LICENSE.txt THIRD-PARTY-NOTICES.txt CAMARAS.txt CAMERAS.txt YouTube/CONNECT-YOUTUBE.txt YouTube/LIGAR-YOUTUBE.txt)

elif [[ $target == macos ]]; then
[[ $(uname) == Darwin ]] || { print -u2 "build.sh: the macos target builds on macOS only"; exit 1 }
rm -rf dist/mac dist/mac-dmg
app=dist/mac/Hallstream.app
mkdir -p $app/Contents/MacOS $app/Contents/Resources
# Go's build cache ignores MACOSX_DEPLOYMENT_TARGET, so -a keeps cgo objects
# built for a newer macOS out of the binary, and the linker's warning fails it.
export MACOSX_DEPLOYMENT_TARGET=13.0 CGO_ENABLED=1
flags="-s -w -X main.version=$version -X main.uploadToken=$HALLSTREAM_UPLOAD_TOKEN -X main.feedbackURL=$HALLSTREAM_FEEDBACK_URL"
tmp=$(mktemp -d)
for arch in amd64 arm64; do
	GOARCH=$arch go build -a -ldflags $flags -o $tmp/hallstream-$arch ./cmd/hallstream 2>$tmp/build.log || { cat $tmp/build.log >&2; exit 1 }
	cat $tmp/build.log >&2
	if grep -q "built for newer" $tmp/build.log; then
		print -u2 "build.sh: $arch linked an object built for a newer macOS than 13.0"
		exit 1
	fi
done
lipo -create -output $app/Contents/MacOS/Hallstream $tmp/hallstream-amd64 $tmp/hallstream-arm64

# Apple wants numbers only in the bundle version.
bundleversion=0.0.0
if [[ $version =~ '^[0-9]+\.[0-9]+\.[0-9]+$' ]]; then bundleversion=$version; fi
sed "s/@VERSION@/$bundleversion/g" macres/Info.plist >$app/Contents/Info.plist
/bin/cp -Rf macres/en.lproj macres/pt.lproj $app/Contents/Resources/

iconset=$tmp/Hallstream.iconset
mkdir -p $iconset
for n in 16 32 128 256 512; do
	sips -z $n $n web/assets/icon/icon-512.png --out $iconset/icon_${n}x$n.png >/dev/null
	if (( n < 512 )); then
		sips -z $((n * 2)) $((n * 2)) web/assets/icon/icon-512.png --out $iconset/icon_${n}x$n@2x.png >/dev/null
	fi
done
iconutil -c icns -o $app/Contents/Resources/Hallstream.icns $iconset
rm -rf $tmp

codesign --force --sign - $app

mkdir -p dist/mac-dmg
ditto $app dist/mac-dmg/Hallstream.app
ln -s /Applications dist/mac-dmg/Applications
texts dist/mac-dmg darwin
if [[ -z $HALLSTREAM_SIGN ]]; then
	# hdiutil create sometimes fails with "Resource busy", so it gets three tries.
	for try in 1 2 3; do
		hdiutil create -quiet -volname Hallstream -srcfolder dist/mac-dmg -ov -format UDZO dist/Hallstream-$version-macos.dmg && break
		(( try < 3 )) || { print -u2 "build.sh: hdiutil create failed 3 times"; exit 1 }
		print -u2 "build.sh: hdiutil create failed (try $try of 3), trying again in 5 seconds"
		sleep 5
	done
fi

else
print -u2 "build.sh: unknown target $target (windows or macos)"
exit 1
fi
ls -lh dist
