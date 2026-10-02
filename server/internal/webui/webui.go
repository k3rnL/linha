// Package webui serves the production console from the server binary.
package webui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:assets
var assets embed.FS

func Handler() (http.Handler, error) {
	bundle, e := fs.Sub(assets, "assets")
	if e != nil {
		return nil, e
	}
	if _, e = fs.Stat(bundle, "index.html"); e != nil {
		return nil, fmt.Errorf("UI assets missing; run make build (frontend build required)")
	}
	files := http.FileServer(http.FS(bundle))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'")
		name := strings.TrimPrefix(r.URL.Path, "/ui/")
		clean := path.Clean(name)
		if strings.Contains(name, "..") || (strings.HasPrefix(clean, ".") && clean != ".") {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			if _, err := fs.Stat(bundle, clean); err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			rr := r.Clone(r.Context())
			rr.URL.Path = "/" + clean
			files.ServeHTTP(w, rr)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		rr := r.Clone(r.Context())
		rr.URL.Path = "/"
		files.ServeHTTP(w, rr)
	}), nil
}
