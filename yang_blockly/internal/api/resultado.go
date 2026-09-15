package api

import "sync"

// Este ficheiro implementa o grupo de endpoints "resultado" — mecanismo
// INTERNO de Yang para o modo dúas xanelas (editor ↔ xanela de resultado
// desacoplada, ver yang/xanelaresultado.go), non parte do catálogo público
// para módulos externos (ver yang/docs/api-yang.md §2 e §11). Vive no mesmo
// servidor/socket/token que o resto da API porque é o mesmo proceso e o
// mesmo transporte — só cambia que quen o consome hoxe é sempre o propio
// Yang (a través de yang/clienteapi.go), nunca outro módulo.

// SnapshotResultado é o que ve o frontend da xanela de resultado — un
// chisco máis simple que o estado completo (sen pdf/tex/fonte, que só fan
// falla nos endpoints ResultadoPDF/ResultadoTex/ResultadoFonte).
type SnapshotResultado struct {
	Version    int      `json:"version"`
	PageImages []string `json:"pageImages"`
	Warnings   []string `json:"warnings"`
	HasPDF     bool     `json:"hasPdf"`
	HasTex     bool     `json:"hasTex"`
}

// PublicarResultadoRequest é o corpo de POST /api/v1/resultado — o que o
// editor de Yang garda despois de cada Xerar/PDF con éxito, para que a
// xanela de resultado (se está aberta) poida amosalo e exportar MD/DOCX/ODT
// sen volver preguntarlle nada ao editor.
type PublicarResultadoRequest struct {
	Source     string   `json:"source"`
	CodeIni    string   `json:"codeIni"`
	Seed       int      `json:"seed"`
	Iterations int      `json:"iterations"`
	NomeBase   string   `json:"nomeBase"`
	PageImages []string `json:"pageImages"`
	Warnings   []string `json:"warnings"`
	PDFBase64  string   `json:"pdfBase64"`
	TexSource  string   `json:"texSource"`
}

// ResultadoPDFResponse é a resposta de GET /api/v1/resultado/pdf — os bytes
// xa xerados (non recompila) máis o nome suxerido para o diálogo "gardar
// como".
type ResultadoPDFResponse struct {
	PDFBase64 string `json:"pdfBase64"`
	NomeBase  string `json:"nomeBase"`
}

// ResultadoTexResponse é a resposta de GET /api/v1/resultado/tex.
type ResultadoTexResponse struct {
	TexSource string `json:"texSource"`
	NomeBase  string `json:"nomeBase"`
}

// ResultadoFonteResponse é a resposta de GET /api/v1/resultado/fonte — o
// documento fonte e a semente que xeraron o resultado actual, para que
// AccionResultadoMD/Docx/Odt (xanelaresultado.go) poidan recompilar
// chamando ExportMarkdown/ExportDocx/ExportOdt coma sempre — só que agora
// req/nomeBase chegan a través desta API, non dun campo compartido lido
// directamente.
type ResultadoFonteResponse struct {
	Source     string `json:"source"`
	CodeIni    string `json:"codeIni"`
	Seed       int    `json:"seed"`
	Iterations int    `json:"iterations"`
	NomeBase   string `json:"nomeBase"`
}

// estadoResultado garda a última xeración (Xerar/PDF) publicada polo
// editor — vive dentro do Servidor porque agora é a propia API quen ten a
// autoridade sobre este estado: tanto o editor coma a xanela de resultado
// len/escriben a través dela, nunca dun campo en memoria accedido
// directamente.
type estadoResultado struct {
	mu         sync.Mutex
	version    int
	source     string
	codeIni    string
	seed       int
	iterations int
	nomeBase   string
	pages      []string
	warnings   []string
	pdf        string
	tex        string
}

func (e *estadoResultado) publicar(req PublicarResultadoRequest) SnapshotResultado {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.version++
	e.source = req.Source
	e.codeIni = req.CodeIni
	e.seed = req.Seed
	e.iterations = req.Iterations
	e.nomeBase = req.NomeBase
	e.pages = req.PageImages
	e.warnings = req.Warnings
	e.pdf = req.PDFBase64
	e.tex = req.TexSource
	return e.snapshot()
}

// snapshot asume que e.mu xa está tomado polo chamador.
func (e *estadoResultado) snapshot() SnapshotResultado {
	return SnapshotResultado{
		Version: e.version, PageImages: e.pages, Warnings: e.warnings,
		HasPDF: e.pdf != "", HasTex: e.tex != "",
	}
}

func (e *estadoResultado) Snapshot() SnapshotResultado {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshot()
}

// PDF devolve o PDF xa xerado e o nome suxerido; ok=false se aínda non se
// publicou ningún (AccionResultadoPDF/Tex non recompilan, ver
// yang/xanelaresultado.go).
func (e *estadoResultado) PDF() (pdf, nomeBase string, ok bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pdf, e.nomeBase, e.pdf != ""
}

func (e *estadoResultado) Tex() (tex, nomeBase string, ok bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tex, e.nomeBase, e.tex != ""
}

func (e *estadoResultado) Fonte() (ResultadoFonteResponse, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.source == "" {
		return ResultadoFonteResponse{}, false
	}
	return ResultadoFonteResponse{
		Source: e.source, CodeIni: e.codeIni, Seed: e.seed,
		Iterations: e.iterations, NomeBase: e.nomeBase,
	}, true
}
