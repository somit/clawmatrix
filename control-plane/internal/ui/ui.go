package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html style.css js
var content embed.FS

// Handler serves index.html at the root path.
func Handler() http.HandlerFunc {
	b, _ := content.ReadFile("index.html")
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
	}
}

// StaticHandler serves CSS/JS assets under /ui/.
func StaticHandler() http.Handler {
	sub, _ := fs.Sub(content, ".")
	return http.StripPrefix("/ui/", http.FileServer(http.FS(sub)))
}
