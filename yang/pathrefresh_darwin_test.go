//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestRefrescarPathEngadeHomebrewETeX reproduce a situación real que rompía
// Yang en macOS: unha app lanzada polo Finder recibe de launchd un PATH
// mínimo, sen Homebrew nin MacTeX. Despois de refrescarPathDendeRexistro,
// os directorios que existan neste equipo teñen que estar no PATH.
func TestRefrescarPathEngadeHomebrewETeX(t *testing.T) {
	restaurar := fixarPath(t, "/usr/bin:/bin:/usr/sbin:/sbin")
	defer restaurar()

	refrescarPathDendeRexistro()
	dirs := filepath.SplitList(os.Getenv("PATH"))

	// Só se esixen os que EXISTEN nesta máquina: refrescarPathDendeRexistro
	// filtra os que non, e a lista de candidatos cobre Apple Silicon e Intel
	// á vez (nunca van estar os dous).
	algunhaComprobacion := false
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/Library/TeX/texbin"} {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		algunhaComprobacion = true
		if !slices.Contains(dirs, dir) {
			t.Errorf("%s existe neste equipo pero non quedou no PATH: %v", dir, dirs)
		}
	}
	if !algunhaComprobacion {
		t.Skip("neste equipo non hai nin Homebrew nin MacTeX: nada que comprobar")
	}
}

// TestRefrescarPathConservaOOriginal: engadir directorios non pode implicar
// perder os que xa viñan. Se Yang se lanza dende un terminal cun PATH
// personalizado (ex. un Maxima compilado a man en ~/bin), ese camiño ten que
// seguir aí despois.
func TestRefrescarPathConservaOOriginal(t *testing.T) {
	propio := t.TempDir()
	restaurar := fixarPath(t, propio+":/usr/bin:/bin")
	defer restaurar()

	refrescarPathDendeRexistro()
	dirs := filepath.SplitList(os.Getenv("PATH"))

	if len(dirs) == 0 || dirs[0] != propio {
		t.Errorf("o primeiro elemento do PATH orixinal debe seguir sendo o primeiro; PATH = %v", dirs)
	}
	for _, dir := range []string{"/usr/bin", "/bin"} {
		if !slices.Contains(dirs, dir) {
			t.Errorf("perdeuse %s do PATH orixinal: %v", dir, dirs)
		}
	}
}

// TestRefrescarPathSenDuplicados: chamar dúas veces (arranque + despois de
// instalar MacTeX dende Opcións, ver InstallLatex) non pode ir inflando o
// PATH con repeticións.
func TestRefrescarPathSenDuplicados(t *testing.T) {
	restaurar := fixarPath(t, "/usr/bin:/bin")
	defer restaurar()

	refrescarPathDendeRexistro()
	primeiro := os.Getenv("PATH")
	refrescarPathDendeRexistro()
	segundo := os.Getenv("PATH")

	if primeiro != segundo {
		t.Errorf("a segunda chamada cambiou o PATH:\n1ª: %s\n2ª: %s", primeiro, segundo)
	}
	vistos := map[string]bool{}
	for _, dir := range filepath.SplitList(segundo) {
		if vistos[dir] {
			t.Errorf("directorio duplicado no PATH: %q (%s)", dir, segundo)
		}
		vistos[dir] = true
	}
}

// TestPathHelperDirsLeEtcPaths comproba que se len as fontes oficiais de
// /usr/libexec/path_helper: é por aí por onde MacTeX (/etc/paths.d/TeX) se
// declara, e non pola lista fixa do código.
func TestPathHelperDirsLeEtcPaths(t *testing.T) {
	dirs := pathHelperDirsMacOS()
	if len(dirs) == 0 {
		t.Fatal("esperaba polo menos o contido de /etc/paths")
	}
	// /usr/bin está en /etc/paths en calquera macOS.
	if !slices.Contains(dirs, "/usr/bin") {
		t.Errorf("esperaba /usr/bin dende /etc/paths, atopei %v", dirs)
	}
	for _, d := range dirs {
		if strings.TrimSpace(d) != d || d == "" {
			t.Errorf("directorio mal recortado: %q", d)
		}
	}
}

// fixarPath deixa o PATH nun valor coñecido e devolve como restauralo, para
// que un test non lle cambie o entorno aos demais.
func fixarPath(t *testing.T, valor string) func() {
	t.Helper()
	anterior, había := os.LookupEnv("PATH")
	os.Setenv("PATH", valor)
	return func() {
		if había {
			os.Setenv("PATH", anterior)
		} else {
			os.Unsetenv("PATH")
		}
	}
}
