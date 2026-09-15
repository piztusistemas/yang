package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ModeloPDF is one PDF file the profesorado attaches as an optional model/
// reference when generating an exam with AI (see XerarExameIA): the exam's
// text is extracted and given to the AI as context to produce a DERIVATIVE
// work (same topic/format/difficulty), never a literal copy.
type ModeloPDF struct {
	FileName string `json:"fileName"`
	DataB64  string `json:"dataB64"`
}

// limiteTextoModeloPDF caps how much text of each attached PDF goes into the
// prompt - abonda para un exame/documento de referencia normal, e evita que
// un PDF xigante (por erro, ex. un libro enteiro) dispare o contexto/custo
// da chamada á IA sen máis aviso.
const limiteTextoModeloPDF = 20000 // runas por documento

// textoDeModelosPDF decodes and extracts the text of each attached PDF (via
// pdftotext, part of poppler-utils - already a hard dependency of Yang, see
// installer.go/instalarLatex) and concatenates them, each headed by its file
// name so the IA can tell several attached documents apart. Devolve "" (sen
// erro) se non hai modelos.
func textoDeModelosPDF(modelos []ModeloPDF) (string, error) {
	if len(modelos) == 0 {
		return "", nil
	}
	if !commandExists("pdftotext") {
		return "", fmt.Errorf("non se atopou pdftotext (paquete poppler-utils) para ler o PDF modelo")
	}

	var partes []string
	for _, m := range modelos {
		nome := strings.TrimSpace(m.FileName)
		if nome == "" {
			nome = "documento.pdf"
		}
		texto, err := extraerTextoPDF(m.DataB64)
		if err != nil {
			return "", fmt.Errorf("lendo %q: %w", nome, err)
		}
		texto = strings.TrimSpace(texto)
		if texto == "" {
			continue
		}
		partes = append(partes, "["+nome+"]\n"+truncarRunas(texto, limiteTextoModeloPDF))
	}
	return strings.Join(partes, "\n\n"), nil
}

// extraerTextoPDF decodes a base64 PDF into a temp file and runs pdftotext
// -layout on it (mantén a disposición de columnas/táboas mellor có modo por
// defecto, útil para exames con enunciados numerados ou en columnas).
func extraerTextoPDF(dataB64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return "", fmt.Errorf("PDF non válido: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "yang-pdf-modelo-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	pdfPath := filepath.Join(tmpDir, "modelo.pdf")
	if err := os.WriteFile(pdfPath, data, 0o644); err != nil {
		return "", err
	}

	cmd := exec.Command("pdftotext", "-layout", pdfPath, "-")
	ocultarConsola(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext: %w", err)
	}
	return string(out), nil
}

// truncarRunas corta `s` a como moito `limite` runas, avisando ao final se
// tivo que facelo (para que a IA e o profesorado saiban que o documento
// achegado era máis longo có que se lle mandou).
func truncarRunas(s string, limite int) string {
	r := []rune(s)
	if len(r) <= limite {
		return s
	}
	return string(r[:limite]) + "\n[...documento truncado, era máis longo...]"
}
