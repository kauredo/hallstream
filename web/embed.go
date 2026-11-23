// Package web holds the overlay, panel, words, fonts and assets, embedded in the binary.
package web

import "embed"

//go:embed all:overlay all:painel all:i18n all:fonts all:assets
var FS embed.FS
