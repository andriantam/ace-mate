// Package web embeds the built React UI (web/dist) into the Go binary.
package web

import "embed"

// Dist holds the Vite build output. Run `make web` before `go build` to populate it.
//
//go:embed all:dist
var Dist embed.FS
