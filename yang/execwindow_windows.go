//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// ocultarConsola evita que cmd abra a súa propia xanela de consola visible
// en Windows - real report: abrir un .matex (que dispara Xerar/GeneratePDF
// decontado) facía aparecer varias "pantallas negras" (unha por cada
// maxima.bat/pdflatex/pdftoppm/pandoc lanzado, ver cas/process.go,
// cas/tags.go, latexdoc.go, docdoc.go). Yang en si é unha app GUI sen
// consola propia (ldflags -H windowsgui) - calquera fillo de consola
// (maxima.bat, pdflatex.exe...) abre entón a SÚA, xa que non hai ningunha
// que herdar. CREATE_NO_WINDOW dille a CreateProcess que non cree consola
// ningunha para ese proceso - stdin/stdout/stderr seguen funcionando igual
// (xa van redirixidos a pipes/buffers, nunca a esa consola). Sen efecto
// sobre procesos GUI (ex. relanzar o propio yang.exe en ReiniciarYang,
// actualizacion.go): eses non teñen consola que amosar en primeiro lugar.
func ocultarConsola(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
