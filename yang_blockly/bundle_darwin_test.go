//go:build darwin

package main

import (
	"path/filepath"
	"testing"
)

// TestBundleQueContenA: de acertar mal aquí dependen dúas cousas serias -
// volver asinar o .app despois dunha actualización (sen iso a app non
// arrinca en Apple Silicon) e relanzalo con `open` en vez de executar o
// binario interno.
func TestBundleQueContenA(t *testing.T) {
	casos := []struct {
		nome     string
		exe      string
		esperado string
	}{
		{
			nome:     "instalación normal en /Applications",
			exe:      "/Applications/Yang.app/Contents/MacOS/yang",
			esperado: "/Applications/Yang.app",
		},
		{
			nome:     "bundle de desenvolvemento (task run)",
			exe:      "/Users/profe/yang_blockly/build/bin/yang.dev.app/Contents/MacOS/yang",
			esperado: "/Users/profe/yang_blockly/build/bin/yang.dev.app",
		},
		{
			nome:     "binario solto: non hai bundle que asinar",
			exe:      "/Users/profe/yang_blockly/build/bin/yang",
			esperado: "",
		},
		{
			// Mesma profundidade ca un bundle, pero sen ser un: non se pode
			// dar por bo só porque haxa tres niveis por riba.
			nome:     "ruta parecida pero sen .app",
			exe:      "/opt/piztu/modulos/Contents/MacOS/yang",
			esperado: "",
		},
		{
			nome:     "dentro do bundle pero fóra de Contents/MacOS",
			exe:      "/Applications/Yang.app/Contents/Resources/yang",
			esperado: "",
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := bundleQueContenA(c.exe); got != c.esperado {
				t.Errorf("bundleQueContenA(%q) = %q, esperaba %q", c.exe, got, c.esperado)
			}
		})
	}
}

// TestComandoRelanzarUsaOpenNoBundle: relanzar o binario interno en vez do
// .app deixaría un proceso que Launch Services non recoñece coma Yang (sen
// icona nin nome no Dock).
func TestComandoRelanzarUsaOpenNoBundle(t *testing.T) {
	cmd := comandoRelanzar("/Applications/Yang.app/Contents/MacOS/yang")
	if filepath.Base(cmd.Path) != "open" {
		t.Errorf("esperaba relanzar con open, obtiven %q", cmd.Path)
	}
	// -n é imprescindible: cando se chama isto a instancia vella aínda non
	// pechou, e sen -n `open` limitaríase a traela ao fronte sen arrincar a
	// versión nova.
	temN, temApp := false, false
	for _, a := range cmd.Args {
		if a == "-n" {
			temN = true
		}
		if a == "/Applications/Yang.app" {
			temApp = true
		}
	}
	if !temN {
		t.Errorf("falta -n en %v", cmd.Args)
	}
	if !temApp {
		t.Errorf("esperaba a ruta do .app en %v", cmd.Args)
	}
}

// TestComandoRelanzarBinarioSolto: fóra dun bundle (execución dende un
// terminal, `task build` sen empaquetar) hai que executar o binario tal cal.
func TestComandoRelanzarBinarioSolto(t *testing.T) {
	exe := "/Users/profe/yang_blockly/build/bin/yang"
	cmd := comandoRelanzar(exe)
	if cmd.Path != exe {
		t.Errorf("esperaba executar %q directamente, obtiven %q", exe, cmd.Path)
	}
}
