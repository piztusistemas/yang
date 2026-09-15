package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// versionInstalador é a versión deste instalador (non confundir coa de Yang
// en si — yang/modulo.json → "version" — aínda que hoxe se publiquen sempre
// xuntas). Ver App.Version en app.go. Mesmo patrón que installer/main.go.
//go:embed VERSION
var versionInstalador string

func main() {
	// En macOS, un .app aberto con dobre clic recibe de launchd un PATH
	// mínimo que non inclúe /opt/homebrew/bin: sen isto, `brew` non se
	// atoparía e o paso de dependencias fallaría sempre (no-op fóra de
	// macOS, ver macos_darwin.go).
	refrescarPathMacOS()

	if saiuOK, permisoDenegado := intentarElevar(); saiuOK {
		return
	} else {
		app := NewApp()
		app.permisoDenegado = permisoDenegado

		err := wails.Run(&options.App{
			Title:            "Yang — Instalador",
			Width:            1024,
			Height:           720,
			WindowStartState: options.Maximised,
			AssetServer: &assetserver.Options{
				Assets: assets,
			},
			BackgroundColour: &options.RGBA{R: 245, G: 245, B: 244, A: 1}, // --bg-main
			OnStartup:        app.startup,
			Bind: []interface{}{
				app,
			},
			Linux: &linux.Options{
				Icon:        appIcon,
				ProgramName: "Yang Installer",
			},
		})

		if err != nil {
			println("Error:", err.Error())
		}
	}
}
