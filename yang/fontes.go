package main

import (
	"os/exec"
	"sort"
	"strings"
)

// ListarFontesSistema devolve os nomes de familia de TODAS as fontes
// instaladas no equipo, para o selector do botón "Axustes do texto"
// (frontend/src/main.js). Usa fc-list (fontconfig): sempre presente en
// Linux, habitual en macOS. Se non está (Windows sen fontconfig, fc-list
// ausente...) devolve lista baleira e o frontend queda só cos presets
// recomendados.
//
// IMPORTANTE: unha fonte do sistema só a pode usar xelatex/lualatex (vía
// fontspec, \setmainfont); pdflatex non. GeneratePDF xa forza o motor
// Unicode cando a tipografía escolle unha destas fontes (ver
// esFonteDoSistema / tipografiaPreambulo en latexdoc.go).
func (a *App) ListarFontesSistema() []string {
	cmd := exec.Command("fc-list", ":", "family")
	ocultarConsola(cmd)
	out, err := cmd.Output()
	if err != nil {
		return []string{}
	}

	set := map[string]bool{}
	for _, liña := range strings.Split(string(out), "\n") {
		// fc-list pode dar "Nome,NomeLocalizado" por liña - quedámonos con
		// todos os alias.
		for _, nome := range strings.Split(liña, ",") {
			nome = strings.TrimSpace(nome)
			// As familias que empezan por "." (".SF NS Text"...) son internas
			// de macOS e non se poden pedir por nome.
			if nome != "" && !strings.HasPrefix(nome, ".") {
				set[nome] = true
			}
		}
	}

	nomes := make([]string, 0, len(set))
	for n := range set {
		nomes = append(nomes, n)
	}
	sort.Slice(nomes, func(i, j int) bool {
		return strings.ToLower(nomes[i]) < strings.ToLower(nomes[j])
	})
	return nomes
}
