// Package ui embeds the studio front end: a no-build, dependency-free web
// application served by cmd/studio.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed index.html favicon.svg css js
var assets embed.FS

// Assets returns the embedded UI files.
func Assets() fs.FS { return assets }
