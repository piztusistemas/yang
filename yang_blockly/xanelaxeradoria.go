package main

import (
	"encoding/json"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// xanelaxeradoria.go implementa o "desacople" das modais de xeración con IA
// ("Exercicio con IA" / "Exame completo con IA"), co MESMO patrón que a
// xanela de resultado (ver xanelaresultado.go): unha WebviewWindow toplevel
// independente (Wails v3 permite varias no mesmo proceso) que carga o mesmo
// index.html cun parámetro de URL (?xerador=exercicio|exame), arrastrable a
// un segundo monitor.
//
// A xanela desacoplada chama os bindings de IA (XerarExercicioIA/
// XerarExameIA) ela mesma e, co resultado, chama a EntregarXeracionIA para
// que a XANELA PRINCIPAL (a que ten o documento aberto) insira o
// exercicio/exame no editor de bloques e recompile a previsualización. A
// comunicación vai por eventos emitidos desde Go (mesmo precedente ca
// yang:resultado-actualizado): non hai Events.Emit desde JS neste proxecto.
const nomeXanelaXeradorIA = "xerador-ia"

// anacoIA é un anaco (type/content) tal e como o serializa o frontend
// (blocks-serialize.js) - mesma forma que ExercicioAnaco pero cun nome
// propio para non mesturar co tipo que devolven os bindings de IA.
type anacoIA struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// xeracionIA é o que manda a xanela desacoplada a EntregarXeracionIA e o que
// recibe a xanela principal nos eventos yang:xerador-ia-exercicio/-exame.
//   - Kind: "exercicio" | "exame"
//   - Op:   "create" (primeira "Xerar") | "replace" (as seguintes, sen
//     pechar: substitúen o que creou a anterior, non acumulan)
//   - Elements:   só para Kind == "exercicio"
//   - Exercicios: só para Kind == "exame"
type xeracionIA struct {
	Kind       string      `json:"kind"`
	Op         string      `json:"op"`
	Elements   []anacoIA   `json:"elements"`
	Exercicios [][]anacoIA `json:"exercicios"`
}

// AbrirXanelaXeradorIA (re)abre a xanela do xerador con IA coma toplevel
// independente. Idempotente: se xa está aberta só a trae ao foco (mesmo
// criterio ca AbrirXanelaResultado). `modo` decide que formulario amosa a
// páxina (?xerador=exercicio|exame); calquera outro valor cae a "exercicio".
func (a *App) AbrirXanelaXeradorIA(modo string) error {
	if modo != "exercicio" && modo != "exame" {
		modo = "exercicio"
	}
	app := application.Get()
	if w, ok := app.Window.GetByName(nomeXanelaXeradorIA); ok {
		w.Show()
		w.Focus()
		return nil
	}

	w := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             nomeXanelaXeradorIA,
		Title:            "Yang — Xerador con IA",
		URL:              "/?xerador=" + modo,
		Width:            640,
		Height:           720,
		BackgroundColour: application.NewRGB(23, 24, 28),
		// Fóra da barra de tarefas en Windows (WS_EX_TOOLWINDOW) - a xanela é
		// auxiliar, coma unha paleta flotante; mesmo criterio ca a de
		// resultado. Ignorado sen efecto noutras plataformas.
		Windows:            application.WindowsWindow{HiddenOnTaskbar: true},
		ZoomControlEnabled: true,
		// Fixar os botóns de minimizar/maximizar explícitos: un real report
		// en Windows atopou só o de pechar na xanela de resultado sen isto.
		MinimiseButtonState: application.ButtonEnabled,
		MaximiseButtonState: application.ButtonEnabled,
	})

	// Peche (botón "Acoplar" da propia xanela OU a X do xestor de xanelas -
	// mesmo camiño de volta) -> avisar á xanela principal para que esqueza o
	// exercicio/exame que estaba a "substituír".
	w.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		application.Get().Event.Emit("yang:xerador-ia-pechado")
	})

	// Fóra da barra de tarefas tamén en Linux (truco GTK) - ver
	// xanela_gtk_linux.go / xanela_other.go.
	ocultarXanelaDaBarraDeTarefas(w)

	return nil
}

// AcoplarXeradorIA pecha o toplevel do xerador - OnWindowEvent(WindowClosing)
// xa fai o resto (emitir yang:xerador-ia-pechado). Un só camiño de volta,
// tanto para o botón "Acoplar" coma para a X do xestor de xanelas.
func (a *App) AcoplarXeradorIA() {
	if w, ok := application.Get().Window.GetByName(nomeXanelaXeradorIA); ok {
		w.Close()
	}
}

// XanelaXeradorIAAberta dío á xanela principal: cando está aberta, premer
// "Exercicio/Exame con IA" na barra só trae a desacoplada ao foco no canto
// de abrir tamén a modal integrada.
func (a *App) XanelaXeradorIAAberta() bool {
	_, ok := application.Get().Window.GetByName(nomeXanelaXeradorIA)
	return ok
}

// GardarEstadoXeradorIA / LerEstadoXeradorIA traspasan o estado do
// formulario (texto do prompt, número de exercicios, casa "Indefinido") da
// modal integrada á xanela desacoplada que se abre a continuación. Texto JSON
// opaco para Go - só o frontend sabe a súa forma.
func (a *App) GardarEstadoXeradorIA(estadoJSON string) {
	a.estadoXeradorIA = estadoJSON
}

func (a *App) LerEstadoXeradorIA() string {
	return a.estadoXeradorIA
}

// EntregarXeracionIA recibe da xanela desacoplada o exercicio/exame recén
// xerado e reemíteo coma evento para que a xanela principal o insira no
// documento e recompile a previsualización.
func (a *App) EntregarXeracionIA(payloadJSON string) error {
	var p xeracionIA
	if err := json.Unmarshal([]byte(payloadJSON), &p); err != nil {
		return err
	}
	app := application.Get()
	if app == nil {
		return nil
	}
	if p.Kind == "exame" {
		app.Event.Emit("yang:xerador-ia-exame", p)
	} else {
		app.Event.Emit("yang:xerador-ia-exercicio", p)
	}
	return nil
}

func init() {
	application.RegisterEvent[xeracionIA]("yang:xerador-ia-exercicio")
	application.RegisterEvent[xeracionIA]("yang:xerador-ia-exame")
	// application.Void (non "any") para un evento sen datos - con "any", Emit
	// sen dato dispara un nil pointer dereference interno que mata a app (ver
	// a mesma nota en xanelaresultado.go).
	application.RegisterEvent[application.Void]("yang:xerador-ia-pechado")
}
