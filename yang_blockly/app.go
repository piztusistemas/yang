package main

import (
	_ "embed"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"matexe-wails/cas"
	"matexe-wails/internal/api"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// codeini_default.ini gets sent to Maxima ONE LINE AT A TIME (splitLines +
// a Send() per line, below) - not as a single block. A multi-line
// "/* ... */" comment breaks this: each fragment line is a separate
// synchronous round-trip, and Maxima won't answer any of them until the
// comment actually closes, so the whole session deadlocks (found the hard
// way - a multi-line comment here once hung every single generation, not
// just whatever it was documenting). Keep any comment in this file to one
// line.
//
//go:embed codeini_default.ini
var defaultCodeIni string

// App is the Go-side backend exposed to the JS frontend (frontend/src/main.js)
// via Wails bindings.
type App struct {
	maximaPath string
	settings   Settings

	// idiomaPiztu is the language code (gl/es/en/pt) the professorado has set
	// for Piztu itself, learned from PIZTU_CONTEXTO (contexto.go) — "" if
	// Yang ran without that context (standalone install, no Piztu). Only a
	// DEFAULT for Yang's own language when settings.Idioma is still unset —
	// see IdiomaPiztu() and frontend/src/main.js.
	idiomaPiztu string

	// maximaSession points at whatever Maxima session Generate/GeneratePDF/
	// ExportMarkdown currently has open, so KillMaxima can reach it from a
	// separate frontend call while the one running it is blocked (e.g.
	// stuck inside cas.Session.rawSend with no timeout). Guarded by
	// maximaMu since it's written from whichever of those goroutines is
	// running, read from the KillMaxima call.
	maximaMu      sync.Mutex
	maximaSession *cas.Session

	// actualizacion é a última foto de se hai unha versión nova de Yang
	// publicada en GitHub (ver comprobarActualizacionYang, actualizacion.go).
	// Consultada en fondo ao arrincar; o botón "i" da barra pinta fondo azul
	// se hai actualización (evento actualizacion_yang_cambiada).
	actualizacionMu sync.Mutex
	actualizacion   EstadoActualizacionYang

	// buildMu/lastBuild lembran os ficheiros da última compilación exitosa
	// (doc.tex/doc.pdf/doc.synctex.gz), para o Ctrl+clic "reverse search"
	// na previsualización - ver reversesearch.go.
	buildMu   sync.Mutex
	lastBuild *lastBuild

	// api é o servidor da API interna de Yang (yang/internal/api,
	// yang/docs/api-yang.md) que expón Xerar/PDF/Markdown/Docx/Odt/
	// Biblioteca a calquera proceso local, e tamén o estado do modo dúas
	// xanelas (editor ↔ xanela de resultado, ver xanelaresultado.go e
	// yang/docs/api-yang.md §11). Non-nil tras ServiceStartup salvo fallo
	// raro ao crear un socket (ver iniciarAPI, apiservidor.go).
	api *api.Servidor
	// apiTmpDir/apiTmpDirPropio: onde vive o socket da API e se ese
	// directorio o creou Yang el mesmo (execución sen Piztu) - nese caso hai
	// que borralo en pecharAPI; se é o tmp_dir compartido de Piztu, non.
	apiTmpDir       string
	apiTmpDirPropio bool
	// clienteAPI fala coa API interna sobre o seu propio socket (ver
	// clienteapi.go) - é o que usan PublicarEstadoResultado/
	// GetEstadoResultado/AccionResultado* en vez de ler un campo compartido
	// en memoria directamente: mesmo camiño que xa usaría un módulo
	// externo, só que aquí quen chama tamén é o propio Yang (ver
	// xanelaresultado.go).
	clienteAPI *clienteAPIInterno

	// taoContexto/taoContextoActiva/clienteTao: presentes só cando Tao lanzou
	// Yang para editar unha tarefa de sesión (TAO_CONTEXTO, ver
	// contexto_tao.go, tao/yang_launch.go). clienteTao fala co API local de
	// Tao (docs/api-tao.md) para empuxar o Markdown xerado en canto está
	// listo — ver tarefa_tao.go, EnviarATarefaTao.
	taoContexto       contextoTao
	taoContextoActiva bool
	clienteTao        *clienteTao

	// estadoXeradorIA garda (coma texto JSON) o estado do formulario das
	// modais "Exercicio/Exame con IA" no intre de "Desacoplar", para que a
	// xanela do xerador que se abre a continuación (ver xanelaxeradoria.go)
	// arrinque co mesmo prompt. Escríbeo GardarEstadoXeradorIA desde a xanela
	// principal e léeo LerEstadoXeradorIA desde a desacoplada — as dúas
	// chamadas veñen do mesmo proceso, non fai falla candado.
	estadoXeradorIA string
}

func NewApp() *App {
	return &App{}
}

// startup mantense coma alias fino de compatibilidade só para os tests
// existentes (app_test.go, exam_check_test.go), que chaman a.startup(...)
// directamente sen pasar por todo o ciclo de vida de Wails — deliberadamente
// NON toca a rede nin arrinca a API interna (ver ServiceStartup, que si o
// fai): un test que chame só startup() non debe abrir sockets nin disparar
// chamadas HTTP de fondo.
func (a *App) startup(ctx context.Context) {
	a.settings = loadSettings()
	a.maximaPath = a.settings.MaximaPath
	if pctx, ok := lerContextoLanzamento(); ok {
		a.idiomaPiztu = pctx.Idioma
	}
}

// ServiceStartup é o hook de ciclo de vida que Wails v3 chama unha soa vez
// ao arrincar (interface application.ServiceStartup) - equivalente ao
// OnStartup(ctx) de v2. Xa non fai falla gardar ctx (v3 non o precisa para
// as chamadas a runtime/diálogos, ver SaveFileDialog etc.) nin detectar "son
// a xanela de resultado" (agora sempre é o mesmo proceso "editor" - a
// xanela de resultado créase baixo demanda, ver AbrirXanelaResultado).
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	// En Windows, o PATH que herda Yang ao arrincar (dende un acceso
	// directo/Explorer) é unha instantánea de cando comezou a sesión de
	// escritorio - se MiKTeX/Pandoc/Maxima se instalaron DESPOIS diso (ex.:
	// co seu propio instalador, non co botón de Yang), Yang seguiría sen
	// velos ata pechar sesión. Refrescar aquí, antes de calquera detección
	// (CheckLatexDeps/CheckDocDeps/findMaxima máis abaixo e as que chama o
	// frontend decontado despois), evita ese falso "non atopado" - ver
	// pathrefresh_windows.go. No-op noutras plataformas.
	refrescarPathDendeRexistro()

	a.startup(ctx)
	go a.comprobarActualizacionYang()
	a.iniciarAPI()
	if tctx, ok := lerContextoTao(); ok {
		a.taoContexto = tctx
		a.taoContextoActiva = true
		a.clienteTao = novoClienteTao(tctx)
	}
	return nil
}

// ServiceShutdown é o hook de ciclo de vida que Wails v3 chama ao pechar
// Yang (interface application.ServiceShutdown) — pecha a API interna para
// non deixar socket/ficheiro de descubrimento vellos tras un peche limpo
// (ver pecharAPI, apiservidor.go).
func (a *App) ServiceShutdown() error {
	a.pecharAPI()
	return nil
}

// IdiomaPiztu devolve o código de idioma (gl/es/en/pt) que o profesorado ten
// escollido en ⚙ Aula do propio Piztu, ou "" se Yang arrincou sen ese
// contexto (executable á man, sen Piztu instalado). O frontend só o usa
// coma valor por defecto cando o profesorado aínda non escolleu un idioma
// propio en Opcións (Settings.Idioma == "") — unha elección xa feita en
// Yang sempre gaña, ver frontend/src/main.js.
func (a *App) IdiomaPiztu() string {
	return a.idiomaPiztu
}

// comprobarActualizacionYang consulta as Releases de piztutao/modulos en
// GitHub (ver actualizacion.go) e avisa o frontend (evento
// actualizacion_yang_cambiada) para que o botón "i" amose fondo azul.
func (a *App) comprobarActualizacionYang() {
	estado := ComprobarActualizacionYang()
	a.actualizacionMu.Lock()
	a.actualizacion = estado
	a.actualizacionMu.Unlock()
	// application.Get() só é non-nil cando application.New()/Run() xa
	// correu polo ciclo de vida real de Wails - nun test que chama
	// a.startup(context.Background()) directamente (ver TestGenerate,
	// app_test.go) segue sendo nil, e Emit sobre unha app inexistente
	// petaría.
	if app := application.Get(); app != nil {
		app.Event.Emit("actualizacion_yang_cambiada", estado)
	}
}

// GetActualizacionYang devolve a última foto de se hai actualización de Yang
// dispoñible, sen tocar a rede (← diálogo de información ao abrirse).
func (a *App) GetActualizacionYang() EstadoActualizacionYang {
	a.actualizacionMu.Lock()
	defer a.actualizacionMu.Unlock()
	return a.actualizacion
}

// AplicarActualizacionYang descarga a última versión de Yang, substitúe o
// executable en execución e arrinca xa a nova copia (← botón "Actualizar"
// no diálogo de información). O frontend é quen peche esta xanela despois.
func (a *App) AplicarActualizacionYang() error {
	estado := a.GetActualizacionYang()
	if !estado.Disponible {
		return fmt.Errorf("non hai ningunha actualización dispoñible")
	}
	if err := AplicarActualizacionYangEnCaliente(estado.URLDescarga, estado.URLChecksum, estado.VersionRemota); err != nil {
		return err
	}
	return ReiniciarYang()
}

// trackMaximaSession registers sess as the currently running Maxima session
// and returns a func that un-registers it - call it as `defer
// a.trackMaximaSession(sess)()` right after a successful cas.Open, before
// the defer m.Close(). Only clears the field if it's still sess, so an
// overlapping session (a second Xerar started before the first finished)
// can't accidentally wipe out the one KillMaxima should actually target.
func (a *App) trackMaximaSession(sess *cas.Session) func() {
	a.maximaMu.Lock()
	a.maximaSession = sess
	a.maximaMu.Unlock()
	return func() {
		a.maximaMu.Lock()
		if a.maximaSession == sess {
			a.maximaSession = nil
		}
		a.maximaMu.Unlock()
	}
}

// KillMaxima forcibly kills whatever Maxima process Generate/GeneratePDF/
// ExportMarkdown currently has running, to recover from a session that's
// wedged (e.g. an expression that sends Maxima into an infinite loop) rather
// than leaving Xerar looking permanently frozen. This doesn't cancel the
// in-flight call synchronously - it just frees its blocked read, so that
// call still returns (with an error) right after.
func (a *App) KillMaxima() error {
	a.maximaMu.Lock()
	sess := a.maximaSession
	a.maximaMu.Unlock()
	if sess == nil {
		return fmt.Errorf("non hai ningunha sesión de Maxima en marcha")
	}
	return sess.Kill()
}

// GenerateRequest mirrors the inputs TMainForm.generarMatex used to read
// from the UI fields (semilla, iteraciones) plus the document text itself.
type GenerateRequest struct {
	Source     string `json:"source"`     // .matex document text
	CodeIni    string `json:"codeIni"`    // Maxima preamble; empty = use the bundled default
	Seed       int    `json:"seed"`
	Iterations int    `json:"iterations"`
	// Modo controla as etiquetas <RESP>/<SOL> da interface de táboa:
	// ""/"enunciados" (folla para o alumnado), "resposta", "solucions".
	// Ver cas.Maxima.RenderMode. Baleiro = comportamento de sempre.
	Modo string `json:"modo"`
}

// GenerateResult carries back the produced HTML plus any per-variant
// warnings (e.g. a tag that failed to evaluate) so the UI can surface them
// without aborting the whole run.
type GenerateResult struct {
	HTML     string   `json:"html"`
	Warnings []string `json:"warnings"`
}

// Generate ports TMainForm.generarMatex: for each iteration it resets the
// Maxima session, runs the codeini preamble, sets a new seed, and parses
// the source document's Matexe tags, concatenating every variant's HTML.
func (a *App) Generate(req GenerateRequest) (GenerateResult, error) {
	result := GenerateResult{}

	if strings.TrimSpace(req.Source) == "" {
		return result, fmt.Errorf("o documento está baleiro")
	}
	if req.Iterations < 1 {
		req.Iterations = 1
	}

	// Priority: per-call override > saved user setting > embedded default.
	codeIni := req.CodeIni
	if strings.TrimSpace(codeIni) == "" {
		codeIni = a.settings.CodeIni
	}
	if strings.TrimSpace(codeIni) == "" {
		codeIni = defaultCodeIni
	}
	codeIniLines := splitLines(codeIni)

	if strings.TrimSpace(a.maximaPath) == "" {
		return result, fmt.Errorf("non se atopou Maxima. Configura a ruta en Opcións")
	}

	tmpDir, err := os.MkdirTemp("", "matexe-gen-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(tmpDir)

	sess, err := cas.Open(a.maximaPath)
	if err != nil {
		return result, fmt.Errorf("non se puido iniciar Maxima (%s): %w", a.maximaPath, err)
	}
	defer a.trackMaximaSession(sess)()
	m := cas.NewMaxima(sess)
	defer m.Close()
	m.SetOutDir(tmpDir)
	m.SetTimeout(a.settings.TimeoutResolto())
	m.ForzarDecimal = a.settings.Decimal
	m.RenderMode = req.Modo

	if _, err := m.Send("linel:1024;display2d:false;simp:true;", false); err != nil {
		return result, err
	}

	var body strings.Builder
	for i := 0; i < req.Iterations; i++ {
		variantSeed := req.Seed + i

		if _, err := m.Send("reset();kill(all)", true); err != nil {
			return result, err
		}
		// SetDecimais TEN que ir despois de kill(all), non antes do bucle:
		// kill(all) borra TODA definición de usuario, incluída a función
		// matexe_arredondar que SetDecimais acaba de definir.
		if err := m.SetDecimais(a.settings.DecimaisResolto()); err != nil {
			return result, err
		}
		for _, line := range codeIniLines {
			if _, err := m.Send(line, false); err != nil {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("variante %d, codeini %q: %v", i+1, line, err))
			}
		}
		if err := m.SetSeed(variantSeed); err != nil {
			return result, err
		}

		html, err := m.ParseText(req.Source)
		if err != nil {
			return result, fmt.Errorf("variante %d (semente %d): %w", i+1, variantSeed, err)
		}
		if i > 0 {
			body.WriteString(`<div style="page-break-before:always;border-top:2px dashed #ccc;margin-top:2rem;padding-top:2rem"></div>` + "\n")
		}
		fmt.Fprintf(&body, "<!-- variante semente=%d -->\n%s\n", variantSeed, html)
	}

	result.HTML = wrapHTML(body.String())
	return result, nil
}

func wrapHTML(body string) string {
	return `<!doctype html><html><head><meta charset="utf-8">
<script src="https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js"></script>
<style>body{font-family:sans-serif;max-width:900px;margin:1.5rem auto;padding:0 1rem}</style>
</head><body>
` + body + `</body></html>`
}

func splitLines(s string) []string {
	s = strings.TrimPrefix(s, "\uFEFF")
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// OpenFileDialog shows a native "open" dialog filtered to .matex files and
// returns the chosen path plus its contents.
func (a *App) OpenFileDialog() (map[string]string, error) {
	path, err := application.Get().Dialog.OpenFile().
		SetTitle("Abrir ficheiro .matex").
		AddFilter("Yang (*.matex)", "*.matex").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return map[string]string{"path": path, "content": string(content)}, nil
}

// splitDialogDefault separa un "nome suxerido" para un diálogo de gardar en
// directorio + nome de ficheiro - necesario porque DefaultFilename NON
// acepta unha ruta completa: en Linux mapea directamente a
// gtk_file_chooser_set_current_name(), que a documentación de GTK di
// explicitamente que espera só un nome de ficheiro, sen barras - pasarlle
// unha ruta completa (ex. "/home/prof/exames/exame1.pdf", que é o que
// facían antes SavePDFDialog/SaveTexDialog/ExportMarkdown/SaveFileDialog
// cando xa había un ficheiro aberto) rompe o selector (atopado probando de
// verdade: o selector amosaba/tentaba gardar coa ruta completa coma se
// fose o nome, en vez de abrir nesa carpeta cun nome normal para editar).
// O directorio vai en DefaultDirectory (se aínda existe - se non, déixase
// baleiro para que o selector use o seu propio por defecto en vez de
// fallar enteiro, que é o que fai runtime.SaveFileDialog se
// DefaultDirectory non existe).
func splitDialogDefault(name string) (dir, base string) {
	base = filepath.Base(name)
	d := filepath.Dir(name)
	if d == "" || d == "." {
		return "", base
	}
	if info, err := os.Stat(d); err != nil || !info.IsDir() {
		return "", base
	}
	return d, base
}

// saveFileDialogCompat constrúe un diálogo "gardar" coa mesma semántica que
// tiña runtime.SaveFileDialog en v2: dir só se fixa se non está baleiro (ver
// splitDialogDefault - baleiro deixa que o selector use o seu propio
// directorio por defecto en vez de fallar enteiro). Usado por
// SaveFileDialog/ExportHTMLDialog aquí e por SavePDFDialog/SaveTexDialog
// (latexdoc.go) e as exportacións MD/DOCX/ODT (markdowndoc.go/docdoc.go).
func saveFileDialogCompat(title, dir, base, filterName, filterPattern string) (string, error) {
	d := application.Get().Dialog.SaveFile().
		SetMessage(title).
		SetFilename(base).
		AddFilter(filterName, filterPattern)
	if dir != "" {
		d = d.SetDirectory(dir)
	}
	return d.PromptForSingleSelection()
}

// SaveFileDialog shows a native "save as" dialog and writes content to the
// chosen path.
func (a *App) SaveFileDialog(defaultName, content string) (string, error) {
	dir, base := splitDialogDefault(defaultName)
	path, err := saveFileDialogCompat("Gardar ficheiro .matex", dir, base, "Yang (*.matex)", "*.matex")
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// SaveFile writes content to an already-known path (Ctrl+S on a file that
// was already opened or saved once).
func (a *App) SaveFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// SaveImageRequest carries an image the user picked from disk via the block
// editor's file input, base64-encoded (the caller already stripped the
// "data:...;base64," prefix), plus the directory of the currently
// open/saved .matex file.
type SaveImageRequest struct {
	BaseDir  string `json:"baseDir"`
	FileName string `json:"fileName"`
	DataB64  string `json:"dataB64"`
}

// SaveUploadedImage copies an uploaded image into an "assets" subdirectory
// next to the open .matex file and returns the relative path to use as
// <IMG src="...">. This is the write-side counterpart of renderImg
// (latexdoc.go), which already resolves <IMG src="..."> relative to
// baseDir - an image inserted here round-trips through GeneratePDF exactly
// like one placed on disk by hand.
func (a *App) SaveUploadedImage(req SaveImageRequest) (string, error) {
	if strings.TrimSpace(req.BaseDir) == "" {
		return "", fmt.Errorf("garda o documento antes de engadir imaxes")
	}
	data, err := base64.StdEncoding.DecodeString(req.DataB64)
	if err != nil {
		return "", fmt.Errorf("imaxe non válida: %w", err)
	}
	assetsDir := filepath.Join(req.BaseDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return "", err
	}

	name := dedupeFileName(assetsDir, req.FileName, "imaxe")
	if err := os.WriteFile(filepath.Join(assetsDir, name), data, 0o644); err != nil {
		return "", err
	}
	return "assets/" + name, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// dedupeFileName devolve un nome de ficheiro seguro para escribir en dir:
// se fileName xa existe aí, engade "-1", "-2"... antes da extensión ata
// atopar un que non colida (mesmo criterio ca SaveUploadedImage orixinal,
// agora compartido tamén por SubirFicheiroCartafol, explorer.go). fallback
// substitúe un nome baleiro (fileName sen parte antes da extensión, p.ex.
// ".png" solto).
func dedupeFileName(dir, fileName, fallback string) string {
	ext := filepath.Ext(fileName)
	base := strings.TrimSuffix(filepath.Base(fileName), ext)
	if base == "" {
		base = fallback
	}
	name := base + ext
	for i := 1; fileExists(filepath.Join(dir, name)); i++ {
		name = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
	return name
}

// XerarContidoIA pide á IA configurada (calquera provedor, ver ia_client.go)
// contido para un anaco de exercicio a partir dunha petición en linguaxe
// natural — botón ✨ do modal de bloques (blocks.js). `tipo` é o mesmo
// "type" que usa blocks-serialize.js (formula/variables/image-plot/
// image-tikz/text); determina o prompt de sistema (ver systemPromptPara,
// ia_client.go) para que o resultado se poida meter directamente no
// textarea do bloque, sen envoltorios.
func (a *App) XerarContidoIA(tipo, peticion string) (string, error) {
	if strings.TrimSpace(peticion) == "" {
		return "", fmt.Errorf("escribe que queres xerar")
	}
	return chamarIA(a.settings, systemPromptPara(tipo), peticion)
}

// AsistenteIARequest é o que manda o frontend nun turno de conversa co
// asistente integrado (icona 🤖): Tipo escolle o prompt de sistema
// (systemPromptAsistente, ia_client.go - "documento" para o editor de
// Código enteiro, ou o mesmo "type" de blocks-serialize.js para un anaco
// do editor de bloques), Contido é o texto ACTUAL dese documento/anaco
// (baleiro se aínda non hai nada escrito), Peticion é o que escribiu o
// profesorado nesta mensaxe.
type AsistenteIARequest struct {
	Tipo     string `json:"tipo"`
	Contido  string `json:"contido"`
	Peticion string `json:"peticion"`
}

// AsistenteIAResponse é a resposta xa clasificada: Accion "responder"
// significa que Texto é unha explicación en prosa e nada cambiou; Accion
// "editar" significa que Texto é unha explicación breve (opcional) e
// Contido é a proposta de contido novo completo - o frontend amosa un
// botón "Aplicar" e NUNCA substitúe nada por conta propia.
type AsistenteIAResponse struct {
	Accion  string `json:"accion"`
	Texto   string `json:"texto"`
	Contido string `json:"contido"`
}

// AsistenteIA atende un turno do asistente de IA integrado - editor de
// Código (icona 🤖 antes de MAT na barra de etiquetas) e cada modal de
// anaco do editor de bloques (Enunciado/Variable/Fórmula/Gráfico/TikZ, non
// "Imaxe": é un ficheiro do disco, non hai contido textual que editar).
// Reutiliza chamarIA (mesmos 3 provedores ca XerarContidoIA/
// XerarExercicioIA); o que cambia é o prompt (systemPromptAsistente, pensado
// para decidir "pregunta ou cambio" en vez de xerar sempre dende cero) e que
// aquí SI se manda o contido actual, para que a IA poida explicalo ou
// modificalo en vez de ignoralo.
func (a *App) AsistenteIA(req AsistenteIARequest) (AsistenteIAResponse, error) {
	if strings.TrimSpace(req.Peticion) == "" {
		return AsistenteIAResponse{}, fmt.Errorf("escribe unha pregunta ou instrución")
	}
	userMsg := req.Peticion
	if strings.TrimSpace(req.Contido) != "" {
		userMsg = "CONTIDO ACTUAL:\n" + req.Contido + "\n\nPETICIÓN: " + req.Peticion
	}
	raw, err := chamarIA(a.settings, systemPromptAsistente(req.Tipo), userMsg)
	if err != nil {
		return AsistenteIAResponse{}, err
	}
	return parseAsistenteResposta(raw), nil
}

// parseAsistenteResposta clasifica a resposta crúa da IA en "editar" (se
// atopa as marcas <<<EDICION>>>...<<<FIN_EDICION>>>, ver asistenteEdicionRe)
// ou "responder" (calquera outra cousa, tal cal). Función pura, á parte de
// AsistenteIA, para poder probar o parseo sen ter que chamar de verdade a
// ningún provedor de IA.
func parseAsistenteResposta(raw string) AsistenteIAResponse {
	if m := asistenteEdicionRe.FindStringSubmatchIndex(raw); m != nil {
		contido := raw[m[2]:m[3]]
		var partes []string
		if antes := strings.TrimSpace(raw[:m[0]]); antes != "" {
			partes = append(partes, antes)
		}
		if despois := strings.TrimSpace(raw[m[1]:]); despois != "" {
			partes = append(partes, despois)
		}
		return AsistenteIAResponse{Accion: "editar", Texto: strings.Join(partes, "\n"), Contido: contido}
	}
	return AsistenteIAResponse{Accion: "responder", Texto: strings.TrimSpace(raw)}
}

// XerarExercicioIA pide á IA un EXERCICIO enteiro (varios anacos xa listos
// para makeExercise/makeElement en blocks.js) a partir dunha descrición en
// linguaxe natural - o profesorado nunca escolle un tipo de bloque nin sabe
// que existen etiquetas Matexe, só describe o exercicio. Un "type" que a IA
// devolva fóra dos coñecidos degrada a "text" en vez de fallar: mellor un
// anaco mal clasificado (o profesorado aínda pode cambialo á man) que
// perder contido enteiro por un capricho de formato da IA.
//
// modelos é opcional (mesmo criterio ca XerarExameIA): un ou varios PDF que
// o profesorado xunta coma referencia - o seu texto extráese (pdftext.go) e
// engádese á petición coa instrución de crear unha obra DERIVADA, non unha
// copia. Con modelos xuntados xa non fai falla escribir a descrición á man.
func (a *App) XerarExercicioIA(peticion string, modelos []ModeloPDF) ([]ExercicioAnaco, error) {
	peticion = strings.TrimSpace(peticion)
	if peticion == "" && len(modelos) == 0 {
		return nil, fmt.Errorf("escribe que exercicio queres ou xunta un PDF modelo")
	}

	textoModelos, err := textoDeModelosPDF(modelos)
	if err != nil {
		return nil, err
	}
	sistema := systemPromptExercicio
	if textoModelos != "" {
		sistema += instrucionModelosPDF
		if peticion == "" {
			peticion = "Crea un exercicio derivado do(s) documento(s) modelo achegado(s) a continuación."
		}
		peticion += "\n\n--- DOCUMENTO(S) MODELO ---\n\n" + textoModelos
	}

	raw, err := chamarIA(a.settings, sistema, peticion)
	if err != nil {
		return nil, err
	}
	var anacos []ExercicioAnaco
	if err := json.Unmarshal([]byte(extractJSONArray(raw)), &anacos); err != nil {
		return nil, formatoInesperadoErr(raw, err)
	}
	if len(anacos) == 0 {
		return nil, fmt.Errorf("a IA non devolveu ningún anaco")
	}
	for i := range anacos {
		if !tiposAnacoValidos[anacos[i].Type] {
			anacos[i].Type = "text"
		}
	}
	return anacos, nil
}

// XerarExameIA pide á IA un EXAME COMPLETO: varios exercicios (cada un xa na
// mesma forma que devolve XerarExercicioIA) a partir dun tema e dun número
// de exercicios desexado - o profesorado escribe só o tema, Yang crea
// todos os exercicios de vez, cada un editable despois coma calquera outro
// creado á man. numExercicios fóra de rango satúrase (1 mínimo, 20 máximo:
// máis exercicios adoita esgotar o límite de resposta da IA e acabar en
// JSON truncado) en vez de fallar.
//
// modelos é opcional: un ou varios PDF (ex. un exame anterior) que o
// profesorado xunta coma referencia - o seu texto extráese (ver pdftext.go,
// pdftotext/poppler-utils) e engádese á petición coa instrución explícita de
// crear unha obra DERIVADA (mesmo tema/formato/dificultade), non unha copia.
// Con modelos xuntados xa non fai falla escribir un tema á man.
//
// numExercicios <= 0 significa INDEFINIDO (casa "A IA decide cantos" na
// modal): non se lle impón número ningún nin, polo tanto, tope de 20 - ese
// tope existe só para protexer unha petición EXPLÍCITA de moitos exercicios
// (a resposta acaba cortada polo límite de saída do modelo, ver
// formatoInesperadoErr), pero non ten sentido aplicarllo a un número que a
// propia IA escolle mirando o que pide o tema ou o documento modelo.
func (a *App) XerarExameIA(tema string, numExercicios int, modelos []ModeloPDF) ([][]ExercicioAnaco, error) {
	tema = strings.TrimSpace(tema)
	if tema == "" && len(modelos) == 0 {
		return nil, fmt.Errorf("escribe o tema do exame ou xunta un PDF modelo")
	}
	if numExercicios < 0 {
		numExercicios = 0
	}
	if numExercicios > 20 {
		numExercicios = 20
	}

	textoModelos, err := textoDeModelosPDF(modelos)
	if err != nil {
		return nil, err
	}
	sistema := systemPromptExame(numExercicios)
	peticion := tema
	if textoModelos != "" {
		sistema += instrucionModelosPDF
		if peticion == "" {
			peticion = "Crea un exame derivado do(s) documento(s) modelo achegado(s) a continuación."
		}
		peticion += "\n\n--- DOCUMENTO(S) MODELO ---\n\n" + textoModelos
	}

	raw, err := chamarIA(a.settings, sistema, peticion)
	if err != nil {
		return nil, err
	}
	var exercicios [][]ExercicioAnaco
	if err := json.Unmarshal([]byte(extractJSONArray(raw)), &exercicios); err != nil {
		return nil, formatoInesperadoErr(raw, err)
	}
	if len(exercicios) == 0 {
		return nil, fmt.Errorf("a IA non devolveu ningún exercicio")
	}
	for i := range exercicios {
		for j := range exercicios[i] {
			if !tiposAnacoValidos[exercicios[i][j].Type] {
				exercicios[i][j].Type = "text"
			}
		}
	}
	return exercicios, nil
}

// extractJSONArray scans raw for the first top-level JSON array ("[" ...
// matching "]", tracking nesting depth and skipping brackets inside quoted
// strings) and returns just that slice, discarding anything before/after
// it. Real report: XerarExercicioIA/XerarExameIA's json.Unmarshal often
// failed with an opaque "a IA devolveu un formato inesperado" - turned out
// the IA frequently wraps its JSON answer in a sentence ("Aquí tes o
// exame:\n[...]\nEspero que che sirva!") that limparValadosMarkdown
// (ia_client.go) doesn't catch (it only strips a code fence that starts at
// the very beginning of the string) - json.Unmarshal saw the whole mixed
// string and rejected it outright, even though the JSON itself was
// perfectly valid. Falls back to raw unchanged when no balanced array is
// found (no "[" at all, or one that never closes - most often a genuinely
// TRUNCATED response, cut short by the model's own output-length limit) so
// the caller's error message (formatoInesperadoErr) still has the full
// response to show a preview of and to run its own truncation heuristic on.
func extractJSONArray(raw string) string {
	start := strings.IndexByte(raw, '[')
	if start == -1 {
		return raw
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		c := raw[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return raw[start : i+1]
			}
		}
	}
	return raw // desequilibrado (case corrente: cortado a metade) - devolve tal cal
}

// formatoInesperadoErr replaces the old unconditional "a IA devolveu un
// formato inesperado, téntao de novo" (real complaint: no way to tell WHY
// it failed or what to actually try differently) with the underlying JSON
// parse error, a heuristic guess at the most common real cause (a response
// cut short by the model's own output-length limit - genuinely frequent for
// XerarExameIA with many exercises requested, since none of the 3 provider
// paths in ia_client.go raise their max-output-tokens above the provider
// default), and a preview of what the IA actually sent back, so a
// professor - or whoever ends up debugging this - has something concrete to
// act on instead of just "try again" with no idea what might change.
func formatoInesperadoErr(raw string, parseErr error) error {
	raw = strings.TrimSpace(raw)
	preview := raw
	const maxPreview = 400
	if len(preview) > maxPreview {
		preview = preview[:maxPreview] + "…"
	}
	if preview == "" {
		preview = "(baleira)"
	}
	hint := "Téntao de novo; se persiste, proba outro modelo ou provedor de IA (Opcións)."
	if looksTruncatedJSON(raw) {
		hint = "Parece que a resposta quedou CORTADA a metade (chegou ao límite de lonxitude do modelo) - sobe o \"Máximo de tokens\" en Opcións (0 = o máximo que dea o modelo), escolle un modelo con máis marxe de resposta, ou pide menos exercicios de vez."
	}
	return fmt.Errorf("a IA devolveu algo que non se puido interpretar coma JSON (%v). %s\n\nComezo da resposta:\n%s", parseErr, hint, preview)
}

// looksTruncatedJSON is formatoInesperadoErr's heuristic for "this looks
// like a response the model's own output-length limit cut off mid-way",
// not a genuinely different/malformed answer: a well-formed JSON array or
// object always ends with its own closing bracket, once whitespace is
// trimmed - anything else (raw is empty, or ends on a comma/quote/digit/
// bare word from a value that never got to close) is what a cut-off stream
// looks like.
func looksTruncatedJSON(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	last := raw[len(raw)-1]
	return last != ']' && last != '}'
}

// ExportHTMLDialog shows a native "save as" dialog filtered to .html files
// and writes the generated preview content - mirrors saving the .out/*.html
// file that TMainForm.generarMatex produced.
func (a *App) ExportHTMLDialog(defaultName, content string) (string, error) {
	dir, base := splitDialogDefault(defaultName)
	path, err := saveFileDialogCompat("Exportar HTML xerado", dir, base, "HTML (*.html)", "*.html")
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
