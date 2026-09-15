package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ── GET /api/v1/whoami ───────────────────────────────────────────────────

func (s *Servidor) handleWhoami(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, http.StatusOK, map[string]string{
		"servizo":     "yang",
		"api_version": VersionAPI,
		"version":     s.versionYang,
	})
}

// ── /xerar, /pdf, /markdown, /docx, /odt comparten o mesmo corpo ────────

func decodificarXerar(w http.ResponseWriter, r *http.Request) (XerarRequest, bool) {
	var req XerarRequest
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		escribirErro(w, http.StatusBadRequest, "corpo_invalido", "corpo JSON non válido: "+err.Error())
		return req, false
	}
	if strings.TrimSpace(req.Source) == "" {
		escribirErro(w, http.StatusBadRequest, "documento_baleiro", "o documento está baleiro")
		return req, false
	}
	return req, true
}

func (s *Servidor) handleXerar(w http.ResponseWriter, r *http.Request) {
	req, ok := decodificarXerar(w, r)
	if !ok {
		return
	}
	res, err := s.nucleo.Xerar(req)
	if err != nil {
		escribirErro(w, http.StatusBadRequest, "erro_xeracion", err.Error())
		return
	}
	escribirJSON(w, http.StatusOK, res)
}

func (s *Servidor) handlePDF(w http.ResponseWriter, r *http.Request) {
	req, ok := decodificarXerar(w, r)
	if !ok {
		return
	}
	res, err := s.nucleo.PDF(req)
	if err != nil {
		escribirErro(w, http.StatusBadRequest, "erro_pdf", err.Error())
		return
	}
	escribirJSON(w, http.StatusOK, res)
}

func (s *Servidor) handleMarkdown(w http.ResponseWriter, r *http.Request) {
	req, ok := decodificarXerar(w, r)
	if !ok {
		return
	}
	res, err := s.nucleo.Markdown(req)
	if err != nil {
		escribirErro(w, http.StatusBadRequest, "erro_markdown", err.Error())
		return
	}
	escribirJSON(w, http.StatusOK, res)
}

func (s *Servidor) handleDocx(w http.ResponseWriter, r *http.Request) {
	req, ok := decodificarXerar(w, r)
	if !ok {
		return
	}
	res, err := s.nucleo.Docx(req)
	if err != nil {
		escribirErro(w, http.StatusBadRequest, "erro_docx", err.Error())
		return
	}
	escribirJSON(w, http.StatusOK, res)
}

func (s *Servidor) handleOdt(w http.ResponseWriter, r *http.Request) {
	req, ok := decodificarXerar(w, r)
	if !ok {
		return
	}
	res, err := s.nucleo.Odt(req)
	if err != nil {
		escribirErro(w, http.StatusBadRequest, "erro_odt", err.Error())
		return
	}
	escribirJSON(w, http.StatusOK, res)
}

// ── /biblioteca ───────────────────────────────────────────────────────

func (s *Servidor) handleBibliotecaListar(w http.ResponseWriter, r *http.Request) {
	items, err := s.nucleo.ListarBiblioteca()
	if err != nil {
		escribirErro(w, http.StatusInternalServerError, "erro_biblioteca", err.Error())
		return
	}
	escribirJSON(w, http.StatusOK, items)
}

func (s *Servidor) handleBibliotecaGardar(w http.ResponseWriter, r *http.Request) {
	var peticion peticionBiblioteca
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&peticion); err != nil {
		escribirErro(w, http.StatusBadRequest, "corpo_invalido", "corpo JSON non válido: "+err.Error())
		return
	}
	item, err := s.nucleo.GardarNaBiblioteca(peticion.Nome, peticion.Tipo, peticion.Contido)
	if err != nil {
		escribirErro(w, http.StatusBadRequest, "erro_biblioteca", err.Error())
		return
	}
	escribirJSON(w, http.StatusOK, item)
}

// handleBibliotecaEliminar responde 204 mesmo se o id xa non existía —
// mesmo criterio ca EliminarDaBiblioteca en main (biblioteca.go): un
// no-op silencioso, non un 404, xa que o estado final ("non está na
// biblioteca") é o mesmo que pedía o chamador.
func (s *Servidor) handleBibliotecaEliminar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		escribirErro(w, http.StatusBadRequest, "id_baleiro", "falta o id na ruta")
		return
	}
	if err := s.nucleo.EliminarDaBiblioteca(id); err != nil {
		escribirErro(w, http.StatusInternalServerError, "erro_biblioteca", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── /resultado — mecanismo interno, ver resultado.go ─────────────────────

func (s *Servidor) handlePublicarResultado(w http.ResponseWriter, r *http.Request) {
	var req PublicarResultadoRequest
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		escribirErro(w, http.StatusBadRequest, "corpo_invalido", "corpo JSON non válido: "+err.Error())
		return
	}
	escribirJSON(w, http.StatusOK, s.resultado.publicar(req))
}

func (s *Servidor) handleGetResultado(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, http.StatusOK, s.resultado.Snapshot())
}

func (s *Servidor) handleResultadoPDF(w http.ResponseWriter, r *http.Request) {
	pdf, nomeBase, ok := s.resultado.PDF()
	if !ok {
		escribirErro(w, http.StatusBadRequest, "resultado_sen_pdf", "aínda non se xerou ningún PDF")
		return
	}
	escribirJSON(w, http.StatusOK, ResultadoPDFResponse{PDFBase64: pdf, NomeBase: nomeBase})
}

func (s *Servidor) handleResultadoTex(w http.ResponseWriter, r *http.Request) {
	tex, nomeBase, ok := s.resultado.Tex()
	if !ok {
		escribirErro(w, http.StatusBadRequest, "resultado_sen_tex", "aínda non se xerou ningún PDF")
		return
	}
	escribirJSON(w, http.StatusOK, ResultadoTexResponse{TexSource: tex, NomeBase: nomeBase})
}

func (s *Servidor) handleResultadoFonte(w http.ResponseWriter, r *http.Request) {
	fonte, ok := s.resultado.Fonte()
	if !ok {
		escribirErro(w, http.StatusBadRequest, "resultado_sen_fonte", "aínda non se xerou nada")
		return
	}
	escribirJSON(w, http.StatusOK, fonte)
}
