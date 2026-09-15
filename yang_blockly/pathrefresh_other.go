//go:build !windows && !darwin

package main

// refrescarPathDendeRexistro non fai nada en Linux: o PATH cos que arrinca
// Yang xa contén todo o que precisa. Un .desktop lanzado polo escritorio
// herda o PATH da sesión (que xa inclúe /usr/bin, onde apt/dnf/pacman poñen
// maxima, pdflatex, pdftoppm e pandoc), e pkexec herda o do chamador.
//
// Ver pathrefresh_windows.go (relé o PATH do Rexistro despois dun `winget
// install`) e pathrefresh_darwin.go (engade Homebrew/MacTeX, que unha app
// lanzada dende Finder NON ve) para as plataformas onde si fai falla.
func refrescarPathDendeRexistro() {}
