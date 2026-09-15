//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// mbDescargaEnCurso devolve os MB xa descargados pola instalación en marcha,
// ou 0 se non se sabe.
//
// Homebrew descarga a un ficheiro ".incomplete" dentro da súa caché e só
// pinta a barra de progreso cando a saída é un terminal - como Yang captura
// stdout nunha tubaría para o log, non emite absolutamente nada durante toda
// a descarga. Con `brew install --cask mactex-no-gui` iso son preto de 6 GB
// de silencio total (medido en real: ~9 MB/s, media hora sen unha soa liña),
// indistinguible dun colgue para quen está a mirar. Ler o tamaño do
// ".incomplete" é a única forma de dar progreso real sen un terminal.
//
// Colle o ficheiro modificado máis recentemente: mentres isto corre, a
// descarga en curso é a nosa.
func mbDescargaEnCurso() int64 {
	if cacheBrew == "" {
		saida, err := exec.Command("brew", "--cache").Output()
		if err != nil {
			cacheBrew = "-" // marcar coma non dispoñible; non reintentar en cada tic
			return 0
		}
		cacheBrew = strings.TrimSpace(string(saida))
	}
	if cacheBrew == "-" {
		return 0
	}

	coincidencias, err := filepath.Glob(filepath.Join(cacheBrew, "downloads", "*.incomplete"))
	if err != nil {
		return 0
	}
	var maisRecente os.FileInfo
	for _, ruta := range coincidencias {
		info, err := os.Stat(ruta)
		if err != nil {
			continue
		}
		if maisRecente == nil || info.ModTime().After(maisRecente.ModTime()) {
			maisRecente = info
		}
	}
	if maisRecente == nil {
		return 0
	}
	return maisRecente.Size() / (1024 * 1024)
}

// cacheBrew memoriza a saída de `brew --cache` ("-" = non dispoñible) para
// non lanzar un subproceso en cada tic do informe de progreso. Só o toca a
// goroutine de informarProgresoInstalacion, que nunca corre máis dunha vez á
// vez (os botóns de Opcións desactívanse mentres instalan).
var cacheBrew string
