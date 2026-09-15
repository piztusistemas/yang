//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// refrescarPathDendeRexistro relé o PATH (sistema + usuario) directamente do
// Rexistro de Windows e fusiónao no PATH do proceso actual.
//
// Por que fai falla: `winget install` (chamado por InstallMaxima/InstallLatex/
// InstallPandoc, installer.go) pode engadir o novo programa ao PATH escribindo
// directamente nas claves de Rexistro de Environment - pero Windows só
// propaga eses cambios a PROCESOS NOVOS (via WM_SETTINGCHANGE aos que
// escoiten, tipicamente o Explorer). Yang é un proceso longa duración: o seu
// os.Getenv("PATH") queda "conxelado" dende que arrincou, así que sen isto o
// botón "Instalar" parecería fallar (Maxima/pdflatex instalado pero non
// atopado) ata que o profesorado pechase e reabrise Yang - o contrario de
// "sinxelo". Chamada só dende os camiños "windows" de InstallMaxima/
// InstallLatex/InstallPandoc, xusto antes de volver comprobar se o programa
// xa se atopa.
func refrescarPathDendeRexistro() {
	sistema := lerPathDeRexistro(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`)
	usuario := lerPathDeRexistro(registry.CURRENT_USER, `Environment`)

	novo := os.Getenv("PATH")
	if sistema != "" {
		novo = sistema + ";" + novo
	}
	if usuario != "" {
		novo = usuario + ";" + novo
	}
	os.Setenv("PATH", novo)
}

func lerPathDeRexistro(raiz registry.Key, ruta string) string {
	k, err := registry.OpenKey(raiz, ruta, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	valor, _, err := k.GetStringValue("Path")
	if err != nil {
		return ""
	}
	return valor
}
