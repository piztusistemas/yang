//go:build !windows && !darwin

package main

// latexEnvExtra non fai falla en Linux: o paquete de sistema "gnuplot" xa
// instala gnuplot-lua-tikz.sty baixo /usr/share/texmf, onde
// pdflatex/xelatex/lualatex xa miran por defecto.
//
// É a única das tres plataformas onde isto sae de balde: en Windows o .sty
// vén dentro do cartafol de Maxima e hai que apuntarlle TEXINPUTS (ver
// latexenv_windows.go), e en macOS non existe en ningures e hai que xeralo
// con gnuplot (ver latexenv_darwin.go).
func latexEnvExtra() []string { return nil }

// prepararGnuplotTikZ non ten nada que preparar en Linux, polo mesmo motivo.
func prepararGnuplotTikZ(log func(string)) {}
