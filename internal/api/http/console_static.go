package httpapi

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
)

func registerConsoleRoutes(mux *http.ServeMux, deps Dependencies) {
	if deps.ConsoleRoot == "" {
		return
	}

	indexPath := filepath.Join(deps.ConsoleRoot, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return
	}

	fileServer := http.FileServer(http.Dir(deps.ConsoleRoot))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		cleanPath := path.Clean(r.URL.Path)
		if cleanPath != "/" {
			candidate := filepath.Join(deps.ConsoleRoot, filepath.FromSlash(cleanPath[1:]))
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		http.ServeFile(w, r, indexPath)
	})
}
