//go:build !bindings

package main

import (
	"os"
	"os/exec"
	"runtime"
)

// intentarElevar reexecuta o proceso como root vía pkexec cando fai falta
// (Linux, non root). Se saiuOK é true, xa se reexecutou con éxito e main()
// debe saír sen abrir xanela ningunha (a instancia root xestionou todo). Se
// non, permisoDenegado indica se pkexec fallou/foi cancelado (para amosar a
// pantalla de instrucións no frontend en vez do asistente). Porte directo de
// installer/elevate_normal.go.
//
// En macOS NON se eleva, e non é unha limitación senón o deseño correcto:
// alí Yang instálase en /Applications/Yang.app, que o usuario administrador
// habitual dun Mac xa pode escribir, e as dependencias veñen de Homebrew,
// que se NÉGA a executarse coma root. Elevar todo o proceso rompería
// precisamente o paso de instalar Maxima e LaTeX. O único que precisa
// permisos de administrador é MacTeX (é un .pkg) e pídeos el só, cun
// diálogo nativo, a través de SUDO_ASKPASS - ver macos_darwin.go. Ademais,
// unha GUI executada con sudo nin sequera podería conectar co WindowServer
// do usuario. A escalada de privilexios só ten sentido en Linux (a máquina
// real do profesor), onde o destino é /opt/piztu.
//
// En Windows NON fai falla nada disto: o propio executable leva incrustado
// un manifesto con requestedExecutionLevel="requireAdministrator" (ver
// build/windows/wails.exe.manifest) - Windows xa amosa o diálogo de UAC e
// eleva o proceso ANTES sequera de que main() empece a executarse. Cando
// este código corre en Windows, xa estamos elevados por definición (se o
// usuario cancelase o UAC, Windows nin sequera chega a lanzar o proceso).
// os.Getuid() sempre devolve -1 en Windows (non existe ese concepto), así
// que había que sacalo explicitamente da condición de abaixo para non caer
// no camiño de pkexec (que non existe en Windows e fallaría sempre,
// amosando "permiso denegado" aínda estando xa elevados).
func intentarElevar() (saiuOK, permisoDenegado bool) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" || os.Getuid() == 0 {
		return false, false
	}

	// pkexec non propaga DISPLAY por defecto: usamos "env" para pasalo explicitamente
	exe, err := os.Executable()
	if err == nil {
		cmd := exec.Command("pkexec", "env",
			"DISPLAY="+os.Getenv("DISPLAY"),
			"XAUTHORITY="+os.Getenv("XAUTHORITY"),
			exe,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if cmd.Run() == nil {
			return true, false // a instancia root xestionou todo
		}
	}
	// pkexec non dispoñible ou cancelado: seguimos igualmente, pero o
	// frontend só amosará a pantalla de instrucións (ver App.PermisoDenegado).
	return false, true
}
