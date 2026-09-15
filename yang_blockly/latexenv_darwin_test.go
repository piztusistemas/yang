//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestXerarEstiloGnuplotTikZ comproba o arranxo do que depende que se poida
// compilar CALQUERA PDF en macOS.
//
// O preámbulo que xera Yang inclúe sempre \usepackage{gnuplot-lua-tikz}, e
// ese paquete non existe en macOS: non o trae TeX Live/MacTeX (kpsewhich non
// o atopa nunha instalación completa) nin a botella de gnuplot de Homebrew
// (que só instala gnuplot-tikz.lua, o terminal). Antes disto, un Mac cos
// tres compoñentes perfectamente instalados fallaba en todos os PDFs con
// "File `gnuplot-lua-tikz.sty' not found".
func TestXerarEstiloGnuplotTikZ(t *testing.T) {
	if !commandExists("gnuplot") {
		t.Skip("gnuplot non instalado neste equipo")
	}

	dir, err := xerarEstiloGnuplotTikZ()
	if err != nil {
		t.Fatalf("xerarEstiloGnuplotTikZ: %v", err)
	}

	// O .sty é o que importa; os outros dous ficheiros son os que el mesmo
	// carga por dentro, así que teñen que estar tamén.
	for _, nome := range []string{
		"gnuplot-lua-tikz.sty",
		"gnuplot-lua-tikz-common.tex",
		"t-gnuplot-lua-tikz.tex",
	} {
		ruta := filepath.Join(dir, nome)
		info, err := os.Stat(ruta)
		if err != nil {
			t.Errorf("falta %s: %v", nome, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s está baleiro", nome)
		}
	}
}

// TestLatexEnvExtraApuntaAoEstilo: de nada serve xerar o .sty se despois non
// se lle di ao motor de LaTeX onde está. compileLatex (latexdoc.go) engade
// ao ambiente o que devolva isto.
func TestLatexEnvExtraApuntaAoEstilo(t *testing.T) {
	if !commandExists("gnuplot") {
		t.Skip("gnuplot non instalado neste equipo")
	}
	if _, err := xerarEstiloGnuplotTikZ(); err != nil {
		t.Skipf("non se puido preparar o estilo: %v", err)
	}

	extra := latexEnvExtra()
	if len(extra) == 0 {
		t.Fatal("esperaba un TEXINPUTS co cartafol do estilo, non obtiven nada")
	}

	var texinputs string
	for _, v := range extra {
		if despois, ok := strings.CutPrefix(v, "TEXINPUTS="); ok {
			texinputs = despois
		}
	}
	if texinputs == "" {
		t.Fatalf("esperaba TEXINPUTS en %v", extra)
	}

	dir, err := dirEstiloGnuplotTikZ()
	if err != nil {
		t.Fatalf("dirEstiloGnuplotTikZ: %v", err)
	}
	if !strings.Contains(texinputs, dir) {
		t.Errorf("TEXINPUTS=%q non menciona %q", texinputs, dir)
	}
	// "//" é a marca de kpathsea para busca recursiva; sen ela pdflatex non
	// baixaría aos subcartafoles.
	if !strings.Contains(texinputs, dir+"//") {
		t.Errorf("TEXINPUTS=%q non leva o sufixo // de busca recursiva", texinputs)
	}
	// O separador final sen nada despois é o que fai que se ENGADAN os
	// camiños por defecto do sistema en vez de substituílos: sen el,
	// pdflatex deixaría de atopar tikz, amsmath e todo o demais.
	if !strings.HasSuffix(texinputs, string(os.PathListSeparator)) {
		t.Errorf("TEXINPUTS=%q non remata en %q: substituiría os camiños do sistema en vez de engadirse a eles",
			texinputs, string(os.PathListSeparator))
	}
}
