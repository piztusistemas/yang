package main

import (
	"encoding/base64"
	"os"
	"path/filepath"

	"matexe-wails/internal/api"
)

// nucleoAPI adapta *App á interface api.Nucleo (yang/internal/api/tipos.go)
// — un tipo á parte en vez de facer que o propio *App a implemente
// directamente porque App xa ten métodos co mesmo nome (ListarBiblioteca,
// GardarNaBiblioteca, EliminarDaBiblioteca) con sinaturas distintas
// (traballan con main.BibliotecaItem, non api.BibliotecaItem) — dous
// métodos co mesmo nome no mesmo tipo non compila. Un tipo novo cos seus
// propios métodos evita a colisión sen tocar os xa existentes que usa a
// GUI.
type nucleoAPI struct{ app *App }

func (n *nucleoAPI) Xerar(req api.XerarRequest) (api.XerarResult, error) {
	res, err := n.app.Generate(GenerateRequest{
		Source: req.Source, CodeIni: req.CodeIni, Seed: req.Seed, Iterations: req.Iterations,
	})
	return api.XerarResult{HTML: res.HTML, Warnings: res.Warnings}, err
}

func (n *nucleoAPI) PDF(req api.XerarRequest) (api.PDFResult, error) {
	// BaseDir queda baleiro a propósito: sen ficheiro local aberto que
	// definir como referencia para <IMG src="...">, ver
	// yang/docs/api-yang.md §6.
	res, err := n.app.GeneratePDF(GeneratePDFRequest{
		Source: req.Source, CodeIni: req.CodeIni, Seed: req.Seed, Iterations: req.Iterations,
	})
	return api.PDFResult{
		PDFBase64: res.PDFBase64, LatexSource: res.LatexSource,
		PageImages: res.PageImages, Warnings: res.Warnings, Log: res.Log,
	}, err
}

func (n *nucleoAPI) Markdown(req api.XerarRequest) (api.MarkdownResult, error) {
	tmpDir, err := os.MkdirTemp("", "yang-api-md-*")
	if err != nil {
		return api.MarkdownResult{}, err
	}
	defer os.RemoveAll(tmpDir)
	body, warnings, err := n.app.generateMarkdownBody(GenerateMarkdownRequest{
		Source: req.Source, CodeIni: req.CodeIni, Seed: req.Seed, Iterations: req.Iterations,
	}, tmpDir)
	return api.MarkdownResult{Markdown: body, Warnings: warnings}, err
}

func (n *nucleoAPI) Docx(req api.XerarRequest) (api.DocResult, error) {
	return n.app.exportarDocAPI(req, "docx", referenceDocx)
}

func (n *nucleoAPI) Odt(req api.XerarRequest) (api.DocResult, error) {
	return n.app.exportarDocAPI(req, "odt", referenceOdt)
}

// exportarDocAPI é o equivalente de ExportDocx/ExportOdt (docdoc.go) para a
// API: en vez dun diálogo "gardar como", exportViaPandoc escribe a un path
// nun directorio temporal que se le e bórrase decontado — o chamador
// remoto recibe os bytes na resposta, nunca un path do sistema de
// ficheiros de Yang (ver yang/docs/api-yang.md §7.1).
func (a *App) exportarDocAPI(req api.XerarRequest, format string, reference []byte) (api.DocResult, error) {
	tmpDir, err := os.MkdirTemp("", "yang-api-"+format+"-*")
	if err != nil {
		return api.DocResult{}, err
	}
	defer os.RemoveAll(tmpDir)

	outPath := filepath.Join(tmpDir, "doc."+format)
	res, err := a.exportViaPandoc(GenerateMarkdownRequest{
		Source: req.Source, CodeIni: req.CodeIni, Seed: req.Seed, Iterations: req.Iterations,
	}, outPath, format, reference)
	if err != nil {
		return api.DocResult{Warnings: res.Warnings}, err
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		return api.DocResult{Warnings: res.Warnings}, err
	}
	return api.DocResult{ConteudoBase64: base64.StdEncoding.EncodeToString(data), Warnings: res.Warnings}, nil
}

func (n *nucleoAPI) ListarBiblioteca() ([]api.BibliotecaItem, error) {
	items, err := n.app.ListarBiblioteca()
	if err != nil {
		return nil, err
	}
	out := make([]api.BibliotecaItem, len(items))
	for i, it := range items {
		out[i] = api.BibliotecaItem{ID: it.ID, Nome: it.Nome, Tipo: it.Tipo, Contido: it.Contido, Creado: it.Creado}
	}
	return out, nil
}

func (n *nucleoAPI) GardarNaBiblioteca(nome, tipo, contido string) (api.BibliotecaItem, error) {
	it, err := n.app.GardarNaBiblioteca(nome, tipo, contido)
	return api.BibliotecaItem{ID: it.ID, Nome: it.Nome, Tipo: it.Tipo, Contido: it.Contido, Creado: it.Creado}, err
}

func (n *nucleoAPI) EliminarDaBiblioteca(id string) error {
	return n.app.EliminarDaBiblioteca(id)
}
