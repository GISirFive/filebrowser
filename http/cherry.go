package fbhttp

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed cherry-assets/*
var cherryAssets embed.FS

func cherryAssetsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the /api/cherry/ prefix to get the file path inside cherry-assets/
		path := strings.TrimPrefix(r.URL.Path, "/api/cherry/")
		if path == "" {
			http.NotFound(w, r)
			return
		}

		// Determine content type
		switch {
		case strings.HasSuffix(path, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case strings.HasSuffix(path, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(path, ".woff2"):
			w.Header().Set("Content-Type", "font/woff2")
		}

		w.Header().Set("Cache-Control", "public, max-age=86400")

		data, err := cherryAssets.ReadFile("cherry-assets/" + path)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Write(data)
	})
}
