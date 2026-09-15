// Package api é o servidor HTTP sobre socket Unix que expón as funcións de
// Yang (xerar, exportar, biblioteca) a calquera proceso local — a porta de
// entrada documentada en yang/docs/api-yang.md. É o inverso do patrón de
// piztu/internal/api: alí Piztu é o servidor e os módulos son clientes;
// aquí Yang é o servidor.
//
// Nucleo é a única dependencia deste paquete cara ao resto de Yang: como
// *App vive en package main (Go non permite que un paquete interno o
// importe), quen adapta App a esta interface é main/apinucleo.go, non este
// paquete.
package api

// XerarRequest é o corpo común a /xerar, /pdf, /markdown, /docx e /odt —
// mesmos catro campos que xa len Generate/GeneratePDF/generateMarkdownBody
// en main (GenerateRequest/GeneratePDFRequest/GenerateMarkdownRequest),
// unificados nun só tipo porque a API non ten "baseDir": sen ficheiro local
// aberto do que resolver <IMG src="..."> relativos (ver
// yang/docs/api-yang.md §6).
type XerarRequest struct {
	Source     string `json:"source"`
	CodeIni    string `json:"codeIni"`
	Seed       int    `json:"seed"`
	Iterations int    `json:"iterations"`
}

// XerarResult é a resposta de POST /api/v1/xerar.
type XerarResult struct {
	HTML     string   `json:"html"`
	Warnings []string `json:"warnings"`
}

// PDFResult é a resposta de POST /api/v1/pdf — mesma forma que
// GeneratePDFResult en main.
type PDFResult struct {
	PDFBase64   string   `json:"pdfBase64"`
	LatexSource string   `json:"latexSource"`
	PageImages  []string `json:"pageImages"`
	Warnings    []string `json:"warnings"`
	Log         string   `json:"log"`
}

// MarkdownResult é a resposta de POST /api/v1/markdown — contido, non un
// path (ver yang/docs/api-yang.md §7.1: un chamador remoto non ten por que
// compartir sistema de ficheiros con Yang).
type MarkdownResult struct {
	Markdown string   `json:"markdown"`
	Warnings []string `json:"warnings"`
}

// DocResult é a resposta de POST /api/v1/docx e /api/v1/odt — o ficheiro
// binario en base64, mesmo criterio ca MarkdownResult.
type DocResult struct {
	ConteudoBase64 string   `json:"conteudoBase64"`
	Warnings       []string `json:"warnings"`
}

// BibliotecaItem mirra o tipo homónimo en main (biblioteca.go).
type BibliotecaItem struct {
	ID      string `json:"id"`
	Nome    string `json:"nome"`
	Tipo    string `json:"tipo"`
	Contido string `json:"contido"`
	Creado  string `json:"creado"`
}

// peticionBiblioteca é o corpo de POST /api/v1/biblioteca.
type peticionBiblioteca struct {
	Nome    string `json:"nome"`
	Tipo    string `json:"tipo"`
	Contido string `json:"contido"`
}

// Nucleo é o que este servidor precisa de Yang — implementado por
// main.nucleoAPI (yang/apinucleo.go), un adaptador fino sobre *App. Cada
// método chama directamente á función xa existente que usa a GUI (Generate,
// GeneratePDF, generateMarkdownBody...), coa mesma configuración persoal do
// profesorado (a.settings) — ver yang/docs/api-yang.md §7.2.
type Nucleo interface {
	Xerar(req XerarRequest) (XerarResult, error)
	PDF(req XerarRequest) (PDFResult, error)
	Markdown(req XerarRequest) (MarkdownResult, error)
	Docx(req XerarRequest) (DocResult, error)
	Odt(req XerarRequest) (DocResult, error)

	ListarBiblioteca() ([]BibliotecaItem, error)
	GardarNaBiblioteca(nome, tipo, contido string) (BibliotecaItem, error)
	EliminarDaBiblioteca(id string) error
}
