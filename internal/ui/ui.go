// Package ui holds the single-page frontend (and its fonts) shared by the desktop (Wails) and web builds.
package ui

import "embed"

//go:embed index.html logo.png mark.png fonts/*.woff2
var FS embed.FS
