//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// refrescarPathDendeRexistro en macOS reconstrúe o PATH deste proceso para
// que inclúa os directorios onde viven REALMENTE as ferramentas que Yang
// chama (maxima, pdflatex, pdftoppm, pandoc, kpsewhich, synctex, pdftotext,
// gnuplot... e o propio brew de InstallMaxima/InstallLatex/InstallPandoc).
//
// Por que fai falla, e por que é o arranxo máis importante do porte a macOS:
// unha aplicación lanzada dende o Finder, o Dock, Spotlight ou Launchpad NON
// é fillo de ningunha shell - lánzaa launchd, que lle dá un PATH mínimo
// ("/usr/bin:/bin:/usr/sbin:/sbin"). Nada do que instalan Homebrew
// (/opt/homebrew/bin en Apple Silicon, /usr/local/bin en Intel) nin MacTeX
// (/Library/TeX/texbin) está aí dentro. Sen isto, commandExists() devolve
// false para TODA a cadea de compilación aínda estando perfectamente
// instalada, e Yang amosa "non atopado" sen que o profesorado poida facer
// nada ao respecto - o mesmo Yang lanzado dende un terminal funcionaría, o
// que fai o fallo especialmente confuso. Non se pode arranxar dende o
// frontend nin dende Opcións: hai que amañar o PATH do proceso.
//
// Chámase dúas veces: unha ao arrincar (main.go, antes de NewApp →
// loadSettings → findMaxima) e outra despois de cada instalación
// (InstallMaxima/InstallLatex/InstallPandoc, installer.go), porque
// `brew install --cask mactex-no-gui` crea /Library/TeX/texbin e rexistra
// /etc/paths.d/TeX DURANTE a execución de Yang - sen a segunda chamada, o
// botón "Instalar" de Opcións remataría ben pero seguiría a dicir que
// pdflatex non está ata reiniciar Yang.
func refrescarPathDendeRexistro() {
	var novo []string
	vistos := map[string]bool{}
	engadir := func(dir string, comprobarQueExiste bool) {
		dir = strings.TrimSpace(dir)
		if dir == "" || vistos[dir] {
			return
		}
		// Os directorios que ENGADIMOS nós compróbanse antes (un
		// /opt/homebrew/bin nun Mac Intel sen Homebrew só sería ruído); os
		// que xa viñan no PATH orixinal respéctanse tal cal, existan ou non
		// - non nos toca a nós depuralos.
		if comprobarQueExiste {
			if st, err := os.Stat(dir); err != nil || !st.IsDir() {
				return
			}
		}
		vistos[dir] = true
		novo = append(novo, dir)
	}

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		engadir(dir, false)
	}
	for _, dir := range pathHelperDirsMacOS() {
		engadir(dir, true)
	}
	for _, dir := range directoriosPathMacOS {
		engadir(dir, true)
	}

	os.Setenv("PATH", strings.Join(novo, string(os.PathListSeparator)))
}

// directoriosPathMacOS son as localizacións coñecidas que se engaden ao PATH
// aínda que ningún ficheiro de /etc/paths.d as declare. Non substitúen a
// pathHelperDirsMacOS: complétana (Homebrew, por exemplo, NON escribe en
// /etc/paths.d - o seu instalador limítase a engadir un `shellenv` ao
// perfil da shell, que unha app de launchd nunca le).
var directoriosPathMacOS = []string{
	"/opt/homebrew/bin",  // Homebrew en Apple Silicon (o habitual hoxe)
	"/opt/homebrew/sbin", //
	"/usr/local/bin",     // Homebrew en Intel (e destino de moitos .pkg)
	"/usr/local/sbin",    //
	"/Library/TeX/texbin", // MacTeX / BasicTeX (o symlink oficial e estable)
	"/usr/texbin",         // MacTeX anterior a 2015, aínda vivo nalgún equipo
	"/opt/local/bin",      // MacPorts, por se o centro o usa no canto de brew
	"/opt/local/sbin",     //
}

// pathHelperDirsMacOS le as mesmas fontes que /usr/libexec/path_helper (o
// que constrúe o PATH das shells de login): /etc/paths e cada ficheiro de
// /etc/paths.d, un directorio por liña. É a vía OFICIAL pola que un
// instalador .pkg se engade ao PATH de todo o sistema - MacTeX, por
// exemplo, deixa alí "/etc/paths.d/TeX" con "/Library/TeX/texbin". Lelo
// aquí (en vez de fiarnos só da lista de enriba) fai que Yang atope tamén
// calquera outra ferramenta instalada por .pkg no equipo do centro.
//
// Non se invoca path_helper coma subproceso a propósito: devolve unha liña
// de shell para avaliar ("PATH=...; export PATH;") e ademais REORDENA o
// PATH herdado, que non é o que queremos aquí.
func pathHelperDirsMacOS() []string {
	var dirs []string
	ler := func(ruta string) {
		data, err := os.ReadFile(ruta)
		if err != nil {
			return
		}
		for _, liña := range strings.Split(string(data), "\n") {
			if d := strings.TrimSpace(liña); d != "" && !strings.HasPrefix(d, "#") {
				dirs = append(dirs, d)
			}
		}
	}

	ler("/etc/paths")
	entradas, err := os.ReadDir("/etc/paths.d")
	if err != nil {
		return dirs
	}
	for _, e := range entradas {
		if !e.IsDir() {
			ler(filepath.Join("/etc/paths.d", e.Name()))
		}
	}
	return dirs
}
