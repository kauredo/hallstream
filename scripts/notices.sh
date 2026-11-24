#!/bin/zsh
# Writes THIRD-PARTY-NOTICES.txt for the Windows exes: the licence text of Go
# and of every module linked into them, read from the module cache. Their
# licences (BSD, MIT, Apache-2.0) ask for these notices to ship with the binary.
# Usage: scripts/notices.sh <out file>. build.sh runs it.
set -e
out=${1:?usage: notices.sh <out file>}
cd "${0:A:h}/.."
{
	print "Hallstream includes the third-party software below. Each part is under its own licence, reproduced in full."
	print
	print "================================================================"
	print "Go (standard library and runtime), https://go.dev"
	print "================================================================"
	# The official Go download keeps LICENSE in GOROOT; Homebrew's keeps it one level up.
	goroot=$(go env GOROOT)
	golic=( $goroot/LICENSE(N) $goroot/../LICENSE(N) )
	(( ${#golic} )) || { print -u2 "notices.sh: no Go LICENSE near $goroot"; exit 1 }
	cat $golic[1]
	for arch in amd64 386; do
		GOOS=windows GOARCH=$arch go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}} {{.Dir}}{{end}}{{end}}' ./cmd/hallstream
	done | sort -u | while read -r mod version dir; do
		lic=( $dir/(LICENSE|LICENSE.*|LICENCE|COPYING)(N) )
		(( ${#lic} )) || { print -u2 "notices.sh: no licence file in $mod $version"; exit 1 }
		print
		print "================================================================"
		print "$mod $version"
		print "================================================================"
		cat $lic[1]
		notice=( $dir/NOTICE(N) )
		if (( ${#notice} )); then print; cat $notice[1]; fi
	done
} >"$out"
