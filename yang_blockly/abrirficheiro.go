package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

// AbrirFicheiro abre `ruta` coa aplicación por defecto do sistema operativo
// (o PDF co lector de PDF instalado, o .tex/.md co editor de texto...). O
// frontend chámao dende o botón "Abrir" que aparece despois de gardar
// (GeneratePDF/SavePDFDialog/SaveTexDialog/ExportMarkdown/ExportDocx/
// ExportOdt - ver main.js), para que o profesorado non teña que ir buscar o
// ficheiro no explorador á man.
//
// cmd.Start() e non cmd.Run(): só lanza o visor e segue - un PDF ou un Word
// aberto pode quedar minutos na pantalla, non ten sentido esperar a que se
// peche.
func (a *App) AbrirFicheiro(ruta string) error {
	if ruta == "" {
		return fmt.Errorf("ruta baleira")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", ruta)
	case "windows":
		// "start" é un comando interno de cmd.exe, non un executable - o ""
		// é o título de xanela que start espera coma primeiro argumento
		// cando o que segue (a ruta) pode levar espazos ou comiñas.
		cmd = exec.Command("cmd", "/c", "start", "", ruta)
	default: // linux e outros unix: xdg-open (parte de xdg-utils, case sempre presente en escritorio)
		cmd = exec.Command("xdg-open", ruta)
	}
	ocultarConsola(cmd) // execwindow_windows.go/execwindow_other.go - sen "pantalla negra" en Windows

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("non se puido abrir %q: %w", ruta, err)
	}
	return nil
}
