// Package web embeds the built React dashboard (a static client-only SPA)
// and serves it from the keelwave binary. The dashboard is produced by
// `pnpm build` in web/, which writes index.html + hashed assets into dist/.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var embedded embed.FS

// Handler returns an http.Handler that serves the embedded dashboard.
//
// Real files (hashed assets, images, favicon) are served directly. Any other
// path that does not look like a file request falls back to index.html so the
// client-side router can handle it (SPA deep-linking). A missing file that
// does look like an asset (has an extension) yields 404 rather than the shell.
func Handler() http.Handler {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		// dist is embedded at compile time; this cannot fail in practice.
		panic(err)
	}

	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(sub))

	serveIndex := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")

		if upath == "" || upath == "index.html" {
			serveIndex(w, r)
			return
		}

		if _, err := fs.Stat(sub, upath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Unknown path: serve the SPA shell for extensionless (route) paths,
		// 404 for anything that looks like a missing asset.
		if path.Ext(upath) == "" {
			serveIndex(w, r)
			return
		}

		http.NotFound(w, r)
	})
}
