//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// O problema que resolve este ficheiro é o mesmo ca en Windows
// (latexenv_windows.go), pero a causa e a solución son distintas.
//
// Calquera <PLOT> que saia en TikZ (plot(), cas/tags.go) xera un .tex que
// empeza por \usepackage{gnuplot-lua-tikz}. Ese paquete NON está en TeX Live
// (nin, polo tanto, en MacTeX/BasicTeX): `kpsewhich gnuplot-lua-tikz.sty`
// non devolve nada nunha instalación completa de MacTeX - comprobado neste
// mesmo Mac. Tampouco o trae o gnuplot de Homebrew: a botella de `gnuplot`
// instala share/gnuplot/<ver>/lua/gnuplot-tikz.lua (o terminal en si), pero
// ningún .sty - comprobado listando o contido da botella.
//
// En Debian isto non se nota porque o paquete "gnuplot" do sistema si deixa
// o .sty baixo /usr/share/texmf, onde kpathsea xa mira (de aí que
// latexenv_other.go non teña que facer nada). En Windows o .sty vén dentro
// do cartafol de Maxima e chega con apuntarlle TEXINPUTS.
//
// En macOS non existe en ningures: hai que XERALO. O propio gnuplot sábeo
// facer - `set terminal tikz createstyle` escribe gnuplot-lua-tikz.sty (e
// os dous .tex que o acompañan) no directorio actual. Faise unha soa vez,
// nun cartafol de caché do usuario, e apúntaselle TEXINPUTS igual ca en
// Windows.

// dirEstiloGnuplotTikZ é onde se garda o .sty xerado: dentro da caché do
// usuario (~/Library/Caches/yang/gnuplot-tikz en macOS), non canda o
// executable - Yang vive en /Applications/Yang.app, que non é escribible
// sen permisos de administrador, e ademais isto é contido rexenerable, que
// é exactamente para o que serve Caches.
func dirEstiloGnuplotTikZ() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "yang", "gnuplot-tikz"), nil
}

// xerarEstiloGnuplotTikZ pídelle a gnuplot que escriba gnuplot-lua-tikz.sty
// no cartafol de caché e devolve ese cartafol. Sobrescribe o que houbese:
// se se acaba de actualizar gnuplot, o .sty ten que corresponder á versión
// nova do terminal (o .sty e o gnuplot-tikz.lua van emparellados).
func xerarEstiloGnuplotTikZ() (string, error) {
	if !commandExists("gnuplot") {
		return "", fmt.Errorf("gnuplot non está instalado")
	}
	dir, err := dirEstiloGnuplotTikZ()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	// `set terminal tikz createstyle` escribe os ficheiros de estilo no
	// DIRECTORIO ACTUAL do proceso, non nun destino que se poida indicar por
	// parámetro - de aí cmd.Dir.
	cmd := exec.Command("gnuplot", "-e", "set terminal tikz createstyle")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("gnuplot non puido xerar o estilo TikZ (pode estar compilado sen soporte lua): %w: %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "gnuplot-lua-tikz.sty")); err != nil {
		return "", fmt.Errorf("gnuplot non escribiu gnuplot-lua-tikz.sty en %s", dir)
	}
	return dir, nil
}

// intentoXeracion garante que, se a xeración falla (gnuplot sen soporte lua,
// caché non escribible...), non se reintente en CADA compilación de PDF -
// só unha vez por execución de Yang. prepararGnuplotTikZ non pasa por aquí:
// esa é a vía explícita ("acabo de instalar gnuplot, próbao outra vez").
var intentoXeracion sync.Once

// latexEnvExtra devolve TEXINPUTS apuntando ao cartafol co .sty xerado, para
// que pdflatex/xelatex/lualatex o atopen (ver compileLatex, latexdoc.go).
// "//" ao final é a marca de kpathsea para "busca recursiva"; o separador
// final sen nada despois engade os camiños por defecto en vez de
// substituílos. nil se non se puido preparar o estilo - a compilación segue
// co ambiente normal e, se o documento realmente precisaba o paquete,
// fallará coa mensaxe habitual de LaTeX.
func latexEnvExtra() []string {
	dir, err := dirEstiloGnuplotTikZ()
	if err != nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, "gnuplot-lua-tikz.sty")); err != nil {
		// Aínda non está: pode ser o primeiro PDF con <PLOT> despois de ter
		// instalado gnuplot pola súa conta (sen pasar polo botón de Opcións,
		// que xa chama a prepararGnuplotTikZ).
		intentoXeracion.Do(func() { _, _ = xerarEstiloGnuplotTikZ() })
		if _, err := os.Stat(filepath.Join(dir, "gnuplot-lua-tikz.sty")); err != nil {
			return nil
		}
	}
	return []string{"TEXINPUTS=" + dir + "//" + string(os.PathListSeparator)}
}

// prepararGnuplotTikZ xera o estilo despois de instalar Maxima/gnuplot ou
// LaTeX dende Opcións (InstallMaxima/InstallLatex, installer.go). Chámase
// dende os dous porque a orde na que o profesorado prema os botóns non se
// pode dar por sabida, e é barato repetilo (unha chamada a gnuplot, décimas
// de segundo - a diferenza do initexmf de MiKTeX en Windows, que tarda
// segundos).
func prepararGnuplotTikZ(log func(string)) {
	if !commandExists("gnuplot") {
		return // aínda non instalado: xa se intentará no outro botón
	}
	log("🔧 Xerando gnuplot-lua-tikz.sty para os debuxos <PLOT>…")
	if _, err := xerarEstiloGnuplotTikZ(); err != nil {
		log("⚠️  " + err.Error())
	}
}
