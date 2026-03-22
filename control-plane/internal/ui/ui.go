package ui

//go:generate templ generate

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed style.css js
var content embed.FS

var assets = computeAssets(content)

// Handler serves the control plane UI by rendering the templ Page component.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		Page(assets).Render(r.Context(), w)
	}
}

// StaticHandler serves CSS/JS assets under /ui/.
func StaticHandler() http.Handler {
	sub, _ := fs.Sub(content, ".")
	return http.StripPrefix("/ui/", http.FileServer(http.FS(sub)))
}
