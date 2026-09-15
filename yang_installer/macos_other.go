//go:build !darwin

package main

import "fmt"

// Talóns para Linux e Windows. Os camiños de macOS de install.go compílanse
// en todas as plataformas aínda que só se executen nun Mac (a decisión
// tómaa runtime.GOOS en tempo de execución), así que estes símbolos teñen
// que existir sempre. Ver macos_darwin.go para o que fan de verdade.

// refrescarPathMacOS: en Linux o PATH da sesión xa serve, e en Windows o
// equivalente (relelo do Rexistro despois dun `winget install`) faino Yang,
// non o instalador.
func refrescarPathMacOS() {}

func instalarDependenciasMacOS(log func(string)) error {
	return fmt.Errorf("instalación con Homebrew só dispoñible en macOS")
}

func desinstalarDependenciasMacOS(log func(string)) error {
	return fmt.Errorf("desinstalación con Homebrew só dispoñible en macOS")
}

func createShortcutsMacOS() error {
	return fmt.Errorf("bundles .app só existen en macOS")
}

// prepararBundleMacOS: en Linux e Windows non hai bundle nin corentena de
// Gatekeeper que quitar, así que doInstall non ten nada que facer aquí.
func prepararBundleMacOS(log func(string)) error { return nil }

func desinstalarAccesosDirectosMacOS() {}
