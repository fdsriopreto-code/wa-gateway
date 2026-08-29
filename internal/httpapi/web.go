package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed assets
var assetsFS embed.FS

// spaHandler serve o console web embarcado. Arquivos existentes vao direto;
// qualquer outra rota cai no index.html (roteamento no cliente).
func spaHandler() http.Handler {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(sub))
	index, _ := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.Trim(r.URL.Path, "/")
		// rotas de API/WS nao registradas nao devem cair no HTML.
		if strings.HasPrefix(p, "api/") || p == "api" || p == "ws" {
			writeErr(w, http.StatusNotFound, "not_found", "rota inexistente")
			return
		}
		if p == "" {
			serveIndex(w, index)
			return
		}
		if _, err := fs.Stat(sub, p); err != nil {
			serveIndex(w, index)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}
