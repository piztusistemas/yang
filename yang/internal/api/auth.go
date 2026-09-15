package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

// novoToken xera un token opaco para esta sesión de Yang — mesmo formato
// (prefixo + hex) ca piztu/internal/api, para recoñecelo axiña coma un
// token de módulos de Piztu nun log ou nunha captura de tráfico.
func novoToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "yg_" + hex.EncodeToString(b)
}

// autenticar esixe un token Bearer que coincida co único token vixente
// desta sesión de Yang. A diferenza de piztu/internal/api non hai
// permisos por chamador que comprobar (ver yang/docs/api-yang.md §4): un
// token válido dá acceso a todo o catálogo. 401 sen token válido; nunca 403
// (non hai nada que negar) nin 404 por falta del (a ruta existir non é
// segredo).
func (s *Servidor) autenticar(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extraerBearer(r.Header.Get("Authorization"))
		if token == "" || token != s.token {
			escribirErro(w, http.StatusUnauthorized, "token_invalido", "falta ou non é válida a cabeceira Authorization: Bearer <token>")
			return
		}
		next(w, r)
	}
}

func extraerBearer(cabeceira string) string {
	const prefixo = "Bearer "
	if !strings.HasPrefix(cabeceira, prefixo) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(cabeceira, prefixo))
}

// recuperar captura un panic en calquera handler e devolve JSON (nunca a
// páxina HTML por defecto de Go) — mesmo motivo ca piztu/internal/api.
func (s *Servidor) recuperar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				escribirErro(w, http.StatusInternalServerError, "erro_interno", "erro interno inesperado")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
