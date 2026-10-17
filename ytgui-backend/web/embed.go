// Package web embeds the built React UI (copied in from ytgui-web/dist).
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

// Dist is the UI's root folder.
func Dist() fs.FS {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
