//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"syscall"
)

// gnuplotTexmfWindows/registrarGnuplotTexmfMiKTeX: mesmo arranxo ca en
// yang_blockly (latexenv_windows.go) - módulo Go á parte, sen import
// compartido posible, así que se duplica aquí (mesmo patrón xa usado para
// outras utilidades pequenas específicas de Windows neste proxecto).
//
// gnuplot-lua-tikz.sty (require calquera <PLOT> con saída TikZ) non é un
// paquete MiKTeX - `mpm --install=gnuplot-lua-tikz` devolve "The requested
// package is unknown" (comprobado en real). En Windows vive só dentro do
// propio cartafol de instalación de Maxima
// (<instalación>\gnuplot\share\texmf\...), empaquetado xunto co gnuplot
// que trae o instalador oficial (o mesmo que usa o winget "MaximaTeam.
// Maxima"). Chamado ao final de instalarDependenciasWindows, cando Maxima
// e MiKTeX xa deberían estar os dous instalados.
func gnuplotTexmfWindows() string {
	var patrons []string
	for _, base := range []string{
		`C:\`,
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"),
	} {
		if base == "" {
			continue
		}
		patrons = append(patrons, filepath.Join(base, "maxima-*", "gnuplot", "share", "texmf"))
		patrons = append(patrons, filepath.Join(base, "Maxima-*", "gnuplot", "share", "texmf"))
	}
	var atopados []string
	for _, patron := range patrons {
		coincidencias, _ := filepath.Glob(patron)
		atopados = append(atopados, coincidencias...)
	}
	if len(atopados) == 0 {
		return ""
	}
	sort.Strings(atopados)
	return atopados[len(atopados)-1]
}

// registrarGnuplotTexmfMiKTeX rexistra (idempotente, comprobado en real) o
// texmf de gnuplot coma "TEXMF root" adicional de MiKTeX
// (`initexmf --register-root` + `--update-fndb`), para que pdflatex atope
// gnuplot-lua-tikz.sty sen depender de TEXINPUTS (yang/latexenv_windows.go
// ten esoutro arranxo coma rede de seguridade se isto non chegou a correr).
func registrarGnuplotTexmfMiKTeX(log func(string)) {
	dir := gnuplotTexmfWindows()
	if dir == "" {
		return // Maxima non atopado - nada que rexistrar
	}
	if !commandExists("initexmf") {
		return // MiKTeX non instalado
	}
	log("🔧 Rexistrando gnuplot-lua-tikz.sty en MiKTeX...")
	if err := runWithLog(comandoOculto("initexmf", "--register-root="+dir), log); err != nil {
		log("⚠️  non se puido rexistrar o texmf de gnuplot en MiKTeX: " + err.Error())
		return
	}
	if err := runWithLog(comandoOculto("initexmf", "--update-fndb"), log); err != nil {
		log("⚠️  non se puido actualizar a base de ficheiros de MiKTeX: " + err.Error())
	}
}

// comandoOculto é exec.Command + ocultar a xanela de consola (ver
// ocultarConsolaWindows) - initexmf/winget son procesos de consola, e este
// instalador tamén é unha app GUI sen consola propia da que herdar unha.
func comandoOculto(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	ocultarConsolaWindows(cmd)
	return cmd
}

func ocultarConsolaWindows(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
