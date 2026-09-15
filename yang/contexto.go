package main

import (
	"encoding/json"
	"os"
)

// contextoModulo é o subconxunto do contrato que Piztu escribe ao lanzar un
// módulo externo (ver ContextoModulo/escribirContexto en piztu/app.go) —
// mesmo patrón ca xesta/contexto.go. Yang só precisa TmpDir: onde publicar
// o seu propio socket e ficheiro de descubrimento (ver
// yang/internal/api, yang/apiservidor.go).
type contextoModulo struct {
	TmpDir string `json:"tmp_dir"`
	// Idioma é o código (gl/es/en/pt) que o profesorado ten escollido en ⚙
	// Aula para o propio Piztu — Yang úsao coma valor por defecto do seu
	// idioma SÓ SE o profesorado nunca escolleu un idioma propio en Opcións
	// (Settings.Idioma == ""), ver IdiomaPiztu() e frontend/src/main.js. Yang
	// pódese instalar e usar sen Piztu (executable á man), así que isto é
	// sempre un valor por defecto, nunca unha orde: "" cando non hai
	// contexto de lanzamento.
	Idioma string `json:"idioma"`
}

// lerContextoLanzamento intenta ler o contexto que Piztu escribiu ao abrir
// Yang (variable de contorno PIZTU_CONTEXTO). Devolve ok=false se Yang se
// abriu sen pasar por Piztu (ex.: en desenvolvemento, executando o binario
// á man) — nese caso non hai tmp_dir compartido coñecido e a API interna de
// Yang simplemente non arrinca (ver yang/docs/api-yang.md §3).
func lerContextoLanzamento() (contextoModulo, bool) {
	var ctx contextoModulo
	ruta := os.Getenv("PIZTU_CONTEXTO")
	if ruta == "" {
		return ctx, false
	}
	dados, err := os.ReadFile(ruta)
	if err != nil {
		return ctx, false
	}
	if err := json.Unmarshal(dados, &ctx); err != nil {
		return ctx, false
	}
	if ctx.TmpDir == "" {
		return ctx, false
	}
	return ctx, true
}
