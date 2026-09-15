//go:build !windows

package main

import "os/exec"

// registrarGnuplotTexmfMiKTeX só ten sentido en Windows - ver
// gnuplottex_windows.go. En macOS o equivalente (xerar gnuplot-lua-tikz.sty
// con gnuplot, que nin MacTeX nin Homebrew traen) faino o propio Yang a
// primeira vez que compila un PDF con <PLOT>, e tamén dende Opcións →
// Instalar - ver yang_blockly/latexenv_darwin.go. En Linux non fai falla
// nada: o paquete "gnuplot" do sistema xa deixa o .sty baixo /usr/share/texmf.
func registrarGnuplotTexmfMiKTeX(log func(string)) {}

// comandoOculto é exec.Command tal cal fóra de Windows: só alí fai falla
// pedir explicitamente que non se abra unha xanela de consola para o proceso
// fillo (ver gnuplottex_windows.go, que é onde vive a versión real).
//
// Este talón faltaba: install.go chámao sen guarda de plataforma nos camiños
// de winget (wingetPackageInstalled, instalarDependenciasWindows,
// desinstalarDependenciasWindows) e, ao estar definido só no ficheiro
// //go:build windows, o instalador non compilaba NIN en Linux nin en macOS
// ("undefined: comandoOculto"). Ese código só se executa en Windows -
// runtime.GOOS decide antes de chegar - pero compílase en todas as
// plataformas, así que o símbolo ten que existir en todas elas.
func comandoOculto(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}
