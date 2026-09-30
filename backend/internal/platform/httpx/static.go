package httpx

import (
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

// SecurityHeaders aplica os cabeçalhos básicos de segurança a todas as respostas
// [docs/seguranca.md §9]: sem sniffing de tipo, sem referer para terceiros e sem iframe.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// SPA serve o build estático da interface: arquivos existentes são servidos diretamente
// (os de /assets/ têm hash no nome e cache longo) e as demais rotas caem no index.html, que
// é revalidado a cada acesso. Caminhos /api/ nunca chegam aqui (404 JSON da API).
func SPA(dir string) http.Handler {
	root := http.Dir(dir)
	files := http.FileServer(root)
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			MethodNotAllowedHandler(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			NotFoundHandler(w, r)
			return
		}
		clean := path.Clean("/" + r.URL.Path)
		if clean != "/" && isFile(root, clean) {
			if strings.HasPrefix(clean, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}

// isFile indica se o caminho existe como arquivo dentro da raiz (http.Dir impede sair dela).
func isFile(root http.FileSystem, name string) bool {
	f, err := root.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	return err == nil && !info.IsDir()
}
