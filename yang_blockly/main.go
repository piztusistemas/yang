package main

import (
	"embed"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/linux/icon.png
var appIcon []byte

// nomeXanelaEditor é o Name da xanela principal. A de resultado
// (nomeXanelaResultado, xanelaresultado.go) créase baixo demanda - Wails v3
// permite varias WebviewWindow no MESMO proceso, así que xa non fai falla
// relanzar un segundo proceso deste binario (ver historial deste ficheiro:
// deseño usado mentres o proxecto estivo en Wails v2).
const nomeXanelaEditor = "editor"

func main() {
	// Antes de NewApp(): loadSettings() → findMaxima() (settings.go) xa
	// depende do PATH, así que hai que telo completo ANTES. En macOS isto é
	// imprescindible — unha app lanzada dende o Finder recibe de launchd un
	// PATH mínimo que non inclúe Homebrew nin MacTeX, e sen esta chamada
	// Maxima/pdflatex/pdftoppm/pandoc darían "non atopado" aínda estando
	// instalados (ver pathrefresh_darwin.go). En Windows relé o PATH do
	// Rexistro (pathrefresh_windows.go); en Linux non fai nada.
	refrescarPathDendeRexistro()

	app := NewApp()

	wailsApp := application.New(application.Options{
		Name:        "Yang",
		Description: "Editor de exames de Yang (matemáticas, Maxima + LaTeX)",
		Services: []application.Service{
			application.NewService(app),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Icon: appIcon,
		Linux: application.LinuxOptions{
			ProgramName: "Yang",
		},
	})

	// Xanela principal ("editor") - maximizada dende o arranque.
	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             nomeXanelaEditor,
		Title:            "Yang",
		Width:            1024,
		Height:           768,
		StartState:       application.WindowStateMaximised,
		BackgroundColour: application.NewRGB(23, 24, 28),
		URL:              "/",
		// ZoomControlEnabled: mesmo Ctrl+/Ctrl-/Ctrl+0 ca na xanela de
		// resultado (ver xanelaresultado.go) - por consistencia entre as
		// dúas xanelas.
		ZoomControlEnabled: true,
	})

	if err := wailsApp.Run(); err != nil {
		println("Error:", err.Error())
	}
}
