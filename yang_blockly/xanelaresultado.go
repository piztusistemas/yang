package main

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"matexe-wails/internal/api"
)

var errXeraAntes = errors.New("xera antes de exportar")

// errSenAPIInterna sinala que a.clienteAPI é nil — iniciarAPI (apiservidor.go)
// non conseguiu arrincar o servidor (caso raro: fallo ao crear un socket
// Unix nun directorio temporal). Sen el, o modo dúas xanelas non ten como
// comunicar editor e xanela de resultado.
var errSenAPIInterna = errors.New("a API interna de Yang non está dispoñible")

// xanelaresultado.go implementa o "modo dúas xanelas": ademais da xanela
// única integrada (editor+resultado, o modo por defecto e o único que
// arrinca así), o profesorado pode DESACOPLAR o resultado (botón
// "Desacoplar" na cabeceira da previsualización) para telo nunha xanela á
// parte (Imprimir/PDF/TeX/MD/DOCX/ODT), arrastrable a un segundo monitor, e
// ACOPLALO de volta cando queira (botón "Acoplar" nesa xanela, ou a X do
// xestor de xanelas) - decisión de sesión, non un axuste persistente (Yang
// arrinca sempre en xanela única).
//
// DESEÑO (Wails v3): editor e xanela de resultado falan a través da API
// interna de Yang (yang/internal/api, endpoints "/resultado", ver
// yang/docs/api-yang.md §11) - mesmo camiño que xa usaría un módulo
// externo, só que aquí quen chama tamén é o propio proceso de Yang (ver
// clienteapi.go). O estado en si non vive en App: vive no Servidor da API
// (internal/api/resultado.go). Wails v3 permite varias WebviewWindow no
// MESMO proceso (ver AbrirXanelaResultado) - non fai falla relanzar un
// segundo proceso coma no deseño anterior con Wails v2.
const nomeXanelaResultado = "results"

// snapshotResultado é o que ve o frontend da xanela de resultado — mesma
// forma que api.SnapshotResultado, exposta aos bindings coma un tipo propio
// de main para non filtrar o paquete api ao JS.
type snapshotResultado struct {
	Version    int      `json:"version"`
	PageImages []string `json:"pageImages"`
	Warnings   []string `json:"warnings"`
	HasPDF     bool     `json:"hasPdf"`
	HasTex     bool     `json:"hasTex"`
}

func snapshotDesdeAPI(s api.SnapshotResultado) snapshotResultado {
	return snapshotResultado{
		Version: s.Version, PageImages: s.PageImages, Warnings: s.Warnings,
		HasPDF: s.HasPDF, HasTex: s.HasTex,
	}
}

// PublicarEstadoResultado chámao o frontend do EDITOR despois de cada
// Xerar/PDF con éxito (run()/generatePDF() en main.js) - publica o
// necesario a través da API interna de Yang (POST /api/v1/resultado) para
// que a xanela de resultado (se está aberta) poida amosalo e, se llo piden,
// exportar MD/DOCX/ODT sen volver preguntarlle nada ao editor. Chamada
// barata e inofensiva aínda sen modo dúas xanelas activo (o evento
// simplemente non ten quen o escoite).
func (a *App) PublicarEstadoResultado(req GenerateMarkdownRequest, nomeBase string, pageImages []string, warnings []string, pdfBase64, texSource string) {
	if a.clienteAPI == nil {
		return
	}
	snap, err := a.clienteAPI.PublicarResultado(context.Background(), api.PublicarResultadoRequest{
		Source: req.Source, CodeIni: req.CodeIni, Seed: req.Seed, Iterations: req.Iterations,
		NomeBase: nomeBase, PageImages: pageImages, Warnings: warnings,
		PDFBase64: pdfBase64, TexSource: texSource,
	})
	if err != nil {
		println("Yang: non se puido publicar o estado do resultado:", err.Error())
		return
	}
	// application.Get() é nil nun test que chama este binding directamente
	// sen pasar por todo o ciclo de vida de Wails (ver TestGenerate,
	// app_test.go, e comprobarActualizacionYang en app.go para o mesmo
	// patrón) - Emit sobre unha app inexistente petaría.
	if app := application.Get(); app != nil {
		app.Event.Emit("yang:resultado-actualizado", snapshotDesdeAPI(snap))
	}
}

// GetEstadoResultado é o que chama a xanela de resultado ao montar
// (hidratación inicial) - le a través da API interna (GET
// /api/v1/resultado).
func (a *App) GetEstadoResultado() snapshotResultado {
	if a.clienteAPI == nil {
		return snapshotResultado{}
	}
	snap, err := a.clienteAPI.Resultado(context.Background())
	if err != nil {
		return snapshotResultado{}
	}
	return snapshotDesdeAPI(snap)
}

// accionResultado é a resposta común ás AccionResultado* - warnings sempre
// que o backend as dea (ex. imaxe non atopada), path só se de verdade se
// gardou algo (o profesorado puido cancelar o diálogo "Gardar como").
type accionResultado struct {
	Path     string   `json:"path"`
	Warnings []string `json:"warnings"`
}

// AccionResultadoPDF/Tex len os bytes xa xerados a través da API interna
// (GET /api/v1/resultado/pdf|tex) - non recompilan, o editor xa xerou estes
// bytes en run()/generatePDF() (ver PublicarEstadoResultado), recompilar de
// novo co MESMO seed daría exactamente o mesmo resultado, só máis lento.
func (a *App) AccionResultadoPDF() (accionResultado, error) {
	if a.clienteAPI == nil {
		return accionResultado{}, errSenAPIInterna
	}
	res, err := a.clienteAPI.ResultadoPDF(context.Background())
	if err != nil {
		return accionResultado{}, errXeraAntes
	}
	path, err := a.SavePDFDialog(res.NomeBase+".pdf", res.PDFBase64)
	return accionResultado{Path: path}, err
}

func (a *App) AccionResultadoTex() (accionResultado, error) {
	if a.clienteAPI == nil {
		return accionResultado{}, errSenAPIInterna
	}
	res, err := a.clienteAPI.ResultadoTex(context.Background())
	if err != nil {
		return accionResultado{}, errXeraAntes
	}
	path, err := a.SaveTexDialog(res.NomeBase+".tex", res.TexSource)
	return accionResultado{Path: path}, err
}

// AccionResultadoMD/Docx/Odt SI recompilan (cas.FormatMarkdown, nunca se
// executou durante un Xerar/PDF normal - cas.FormatLatex) - len o
// documento fonte a través da API interna (GET /api/v1/resultado/fonte) e
// reutilizan ExportMarkdown/ExportDocx/ExportOdt, mesma lóxica que xa usan
// dende a xanela única, só que chamadas dende aquí no canto de directamente
// do editor.
func (a *App) AccionResultadoMD() (accionResultado, error) {
	fonte, err := a.resultadoFonte()
	if err != nil {
		return accionResultado{}, err
	}
	result, err := a.ExportMarkdown(GenerateMarkdownRequest{
		Source: fonte.Source, CodeIni: fonte.CodeIni, Seed: fonte.Seed, Iterations: fonte.Iterations,
		NomeDoc: fonte.NomeBase,
	}, fonte.NomeBase+".md")
	return accionResultado{Path: result.Path, Warnings: result.Warnings}, err
}

func (a *App) AccionResultadoDocx() (accionResultado, error) {
	fonte, err := a.resultadoFonte()
	if err != nil {
		return accionResultado{}, err
	}
	result, err := a.ExportDocx(GenerateMarkdownRequest{
		Source: fonte.Source, CodeIni: fonte.CodeIni, Seed: fonte.Seed, Iterations: fonte.Iterations,
		NomeDoc: fonte.NomeBase,
	}, fonte.NomeBase+".docx")
	return accionResultado{Path: result.Path, Warnings: result.Warnings}, err
}

func (a *App) AccionResultadoOdt() (accionResultado, error) {
	fonte, err := a.resultadoFonte()
	if err != nil {
		return accionResultado{}, err
	}
	result, err := a.ExportOdt(GenerateMarkdownRequest{
		Source: fonte.Source, CodeIni: fonte.CodeIni, Seed: fonte.Seed, Iterations: fonte.Iterations,
		NomeDoc: fonte.NomeBase,
	}, fonte.NomeBase+".odt")
	return accionResultado{Path: result.Path, Warnings: result.Warnings}, err
}

// resultadoFonte é o núcleo compartido por AccionResultadoMD/Docx/Odt: pide
// o documento fonte gardado á API interna, traducindo calquera fallo
// (servidor non dispoñible, ou aínda sen publicar nada) a errXeraAntes.
func (a *App) resultadoFonte() (api.ResultadoFonteResponse, error) {
	if a.clienteAPI == nil {
		return api.ResultadoFonteResponse{}, errSenAPIInterna
	}
	fonte, err := a.clienteAPI.ResultadoFonte(context.Background())
	if err != nil {
		return api.ResultadoFonteResponse{}, errXeraAntes
	}
	return fonte, nil
}

// AbrirXanelaResultado (re)abre a xanela de resultado coma toplevel
// independente - dous WebviewWindow do MESMO proceso, iso é o que permite
// v3. Idempotente: se xa está aberta, só a trae ao foco. Chamado dende o
// botón "Desacoplar" da xanela principal, e tamén dispoñible coma botón
// manual "Reabrir xanela de resultado" por se o profesorado a pechou sen
// querer.
func (a *App) AbrirXanelaResultado() error {
	app := application.Get()
	if w, ok := app.Window.GetByName(nomeXanelaResultado); ok {
		w.Show()
		w.Focus()
		return nil
	}

	// A xanela de resultado NON arrinca maximizada: o obxectivo do modo
	// dúas xanelas é ter as dúas visibles á vez (nun segundo monitor, ou a
	// carón da outra) - se arrincase maximizada tapando a principal, o
	// profesorado tería que desmaximizala igualmente antes de poder
	// arrastrala a onde lle interese.
	w := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             nomeXanelaResultado,
		Title:            "Yang — Resultado",
		URL:              "/?resultado=1",
		Width:            900,
		Height:           700,
		BackgroundColour: application.NewRGB(23, 24, 28),
		// HiddenOnTaskbar: equivalente en Windows ao truco GTK de
		// xanela_gtk_linux.go/ocultarXanelaDaBarraDeTarefas - Wails xa o
		// implementa internamente (WS_EX_TOOLWINDOW), non fai falla código
		// propio. Ignorado sen efecto en Linux/macOS (ver xanela_other.go).
		Windows: application.WindowsWindow{HiddenOnTaskbar: true},
		// ZoomControlEnabled: en Windows delega directamente no propio
		// WebView2 (CoreWebView2Settings.IsZoomControlEnabled) - dálle á
		// xanela de resultado o mesmo Ctrl+/Ctrl-/Ctrl+0/Ctrl+roda que
		// calquera páxina en Edge/Chrome, sen código propio ningún.
		ZoomControlEnabled: true,
		// Minimise/MaximiseButtonState: por defecto deberían amosarse
		// (ButtonEnabled é o valor cero), pero un real report en Windows
		// atopou só o botón de pechar na xanela desacoplada - fixándoos
		// explícitos evita depender de calquera comportamento por defecto
		// da plataforma nesta versión beta de Wails v3.
		MinimiseButtonState: application.ButtonEnabled,
		MaximiseButtonState: application.ButtonEnabled,
	})

	// Peche (botón "Acoplar" na xanela de resultado, OU a X do xestor de
	// xanelas - mesmo camiño de volta para os dous) -> avisar ao editor.
	w.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		application.Get().Event.Emit("yang:resultado-pechado")
	})

	// Fóra da barra de tarefas - ver xanela_gtk_linux.go. Sen isto, o modo
	// dúas xanelas amosaría dous "aplicativos" distintos na barra, moi
	// incómodo; a xanela de resultado é auxiliar (coma unha paleta
	// flotante), non precisa a súa propia entrada.
	ocultarXanelaDaBarraDeTarefas(w)

	return nil
}

// DockResultado é o binding que chama o botón "Acoplar" da xanela de
// resultado - simplemente pecha o toplevel; OnWindowEvent(WindowClosing)
// enriba xa fai o resto (emitir yang:resultado-pechado). Un só camiño de
// retorno, tanto para o botón coma para a X do xestor de xanelas.
func (a *App) DockResultado() {
	if w, ok := application.Get().Window.GetByName(nomeXanelaResultado); ok {
		w.Close()
	}
}

func (a *App) XanelaResultadoAberta() bool {
	_, ok := application.Get().Window.GetByName(nomeXanelaResultado)
	return ok
}

func init() {
	application.RegisterEvent[snapshotResultado]("yang:resultado-actualizado")
	// application.Void (non "any") é o tipo correcto para un evento sen
	// datos - con "any", Emit sen dato provoca un nil pointer dereference
	// interno que mata toda a app.
	application.RegisterEvent[application.Void]("yang:resultado-pechado")
}
