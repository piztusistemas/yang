//go:build windows

package cas

import (
	"os/exec"
	"syscall"
)

// ocultarConsola evita que cmd abra a súa propia xanela de consola visible
// en Windows - ver execwindow_windows.go (paquete main) para o porqué
// completo. Duplicado aquí (non exportado dende main) porque cas é un
// paquete á parte usado polos propios procesos Maxima/pdflatex/pdftoppm
// deste ficheiro (process.go, tags.go).
func ocultarConsola(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
