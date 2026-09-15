//go:build !(linux && gtk3)

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// ocultarXanelaDaBarraDeTarefas non fai nada nesta plataforma/build.
//
// En Linux con "-tags gtk3" (xanela_gtk_linux.go) fai falla un truco GTK
// explícito para agochar a xanela de resultado da barra de tarefas. En
// Windows non fai falla: AbrirXanelaResultado (xanelaresultado.go) xa pide
// iso na propia creación da xanela con
// application.WebviewWindowOptions{Windows: application.WindowsWindow{
// HiddenOnTaskbar: true}} - Wails aplícao internamente coa mesma técnica
// (WS_EX_TOOLWINDOW), sen precisar código propio aquí. En macOS e en Linux
// sen "-tags gtk3" isto queda sen efecto (a xanela de resultado aparece na
// barra de tarefas coma calquera outra) - non impide usar Yang, só é menos
// elegante.
func ocultarXanelaDaBarraDeTarefas(w *application.WebviewWindow) {}
