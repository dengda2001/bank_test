package main

import (
	"embed"
	"io/fs"
	"net/http"
)

// webFiles keeps the SSR templates and their static dependencies inside the
// application binary. This preserves the current single-binary deployment
// while allowing page layout and responsive styles to evolve as frontend files.
//
//go:embed web/templates web/static
var webFiles embed.FS

func embeddedWebText(path string) string {
	contents, err := fs.ReadFile(webFiles, path)
	if err != nil {
		panic(err)
	}
	return string(contents)
}

func embeddedWebStaticHandler() http.Handler {
	staticFS, err := fs.Sub(webFiles, "web/static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))
}
