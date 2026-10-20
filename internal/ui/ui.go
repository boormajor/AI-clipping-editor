package ui

import (
	"embed"
	"io/fs"
)

//go:embed index.html app.js style.css
var assets embed.FS

func Assets() fs.FS { return assets }
