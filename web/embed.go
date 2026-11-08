// Package webassets embeds the built frontend.
package webassets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var raw embed.FS

// FS returns the dist tree rooted at ".".
func FS() (fs.FS, error) {
	return fs.Sub(raw, "dist")
}
