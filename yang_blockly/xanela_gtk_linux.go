//go:build linux && gtk3

package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>

// ocultarIdle corre no fío principal de GTK (agendado por g_idle_add, ver
// abaixo) - recibe o GtkWindow concreto (non percorre todas as toplevel:
// dende a migración a Wails v3 este proceso ten DÚAS xanelas - editor e
// resultado - e só a de resultado debe ocultarse) e márcaa "skip taskbar":
// mecanismo estándar de GTK/X11 para dicir "esta xanela é auxiliar, non a
// amoses coma unha aplicación á parte na barra de tarefas" - respectado
// pola inmensa maioría dos xestores de xanelas en Linux (GNOME, KDE,
// XFCE...), independentemente de que agrupen ou non por WM_CLASS.
static gboolean ocultarIdle(gpointer data) {
    GtkWindow *w = GTK_WINDOW(data);
    gtk_window_set_skip_taskbar_hint(w, TRUE);
    gtk_window_set_skip_pager_hint(w, TRUE);
    return FALSE; // FALSE = non repetir, correr unha soa vez
}

// ocultarDaBarraDeTarefas é o único punto de entrada seguro dende Go: GTK
// esixe que todas as súas chamadas corran no MESMO fío que executa
// gtk_main() (o fío principal), pero AbrirXanelaResultado (onde a
// chamamos) pode correr noutra goroutine - chamar GTK directamente dende
// aí sería un uso multi-fío inseguro (podería petar ou corromper estado).
// g_idle_add() SI é seguro de chamar dende calquera fío (é un mecanismo de
// GLib pensado precisamente para isto): só AGENDA ocultarIdle para correr
// máis tarde no fío principal, cando o propio bucle de eventos de GTK teña
// un intre libre - practicamente inmediato, xa que ese bucle está activo.
static void ocultarDaBarraDeTarefas(GtkWindow *w) {
    g_idle_add(ocultarIdle, w);
}
*/
import "C"

import (
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ocultarXanelaDaBarraDeTarefas chámase só sobre a xanela de RESULTADO
// (modo dúas xanelas, ver xanelaresultado.go, AbrirXanelaResultado) - a
// xanela EDITOR queda coma está (esa SI ten que verse na barra de tarefas,
// é "a aplicación"; a de resultado é auxiliar, coma unha paleta de
// ferramentas flotante). ProgramName (main.go) xa fai que as dúas
// compartan WM_CLASS, o que abondaría nalgúns escritorios para agrupalas
// baixo unha soa icona - pero "skip taskbar" é máis fiable: funciona sexa
// cal sexa a política de agrupamento do escritorio do profesorado.
func ocultarXanelaDaBarraDeTarefas(w *application.WebviewWindow) {
	native := w.NativeWindow()
	if native == nil {
		return
	}
	C.ocultarDaBarraDeTarefas((*C.GtkWindow)(unsafe.Pointer(native)))
}
