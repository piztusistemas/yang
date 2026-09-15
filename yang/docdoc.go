package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

)

// referenceDocx/referenceOdt son plantillas de estilos "estilo exame
// clásico" (tipografía serif, título e Título 1 con liña separadora,
// marxes de 2,5cm en A4, táboas con grade) - xeradas unha vez a partir do
// reference.docx/reference.odt por defecto de Pandoc e retocadas á man
// (ver word/styles.xml e styles.xml dentro de cada .docx/.odt, son ficheiros
// ZIP). Pandoc só usa os ESTILOS destes ficheiros, non o seu contido, coa
// opción --reference-doc.
//
//go:embed reference.docx
var referenceDocx []byte

//go:embed reference.odt
var referenceOdt []byte

// GenerateDocResult mirrors GenerateMarkdownResult (markdowndoc.go).
type GenerateDocResult struct {
	Path     string   `json:"path"`
	Warnings []string `json:"warnings"`
}

// exportViaPandoc é o núcleo compartido por ExportDocx e ExportOdt: xera o
// corpo en Markdown coa mesma pasada por Maxima que ExportMarkdown
// (generateMarkdownBody, markdowndoc.go), e convérteo co Pandoc instalado no
// sistema ao formato pedido, aplicando reference como plantilla de estilos.
// Un directorio temporal abonda - a diferenza do .md exportado, o .docx/.odt
// final leva as imaxes incrustadas dentro do propio ficheiro (Pandoc
// empaquétaas), non precisa dun cartafol "images/" á beira.
func (a *App) exportViaPandoc(req GenerateMarkdownRequest, path, format string, reference []byte) (GenerateDocResult, error) {
	result := GenerateDocResult{}

	if !commandExists("pandoc") {
		return result, fmt.Errorf("non se atopou Pandoc. Instálao con 'sudo apt install pandoc'")
	}

	tmpDir, err := os.MkdirTemp("", "matexe-"+format+"-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(tmpDir)

	body, warnings, err := a.generateMarkdownBody(req, tmpDir)
	result.Warnings = warnings
	if err != nil {
		return result, err
	}

	mdPath := filepath.Join(tmpDir, "doc.md")
	if err := os.WriteFile(mdPath, []byte(body), 0o644); err != nil {
		return result, err
	}
	refPath := filepath.Join(tmpDir, "reference."+format)
	if err := os.WriteFile(refPath, reference, 0o644); err != nil {
		return result, err
	}

	cmd := exec.Command("pandoc", mdPath,
		"-f", "markdown",
		"-o", path,
		"--reference-doc="+refPath,
		"--resource-path="+tmpDir,
	)
	ocultarConsola(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return result, fmt.Errorf("pandoc fallou: %w: %s", err, out)
	}

	result.Path = path
	return result, nil
}

// ExportDocx amosa un diálogo "Gardar como" para un .docx, xera o exame e
// convérteo con Pandoc usando referenceDocx como plantilla de estilos - ver
// exportViaPandoc.
func (a *App) ExportDocx(req GenerateMarkdownRequest, defaultName string) (GenerateDocResult, error) {
	result := GenerateDocResult{}
	dir, base := splitDialogDefault(defaultName)
	path, err := saveFileDialogCompat("Exportar Word (.docx)", dir, base, "Word (*.docx)", "*.docx")
	if err != nil || path == "" {
		return result, err
	}
	return a.exportViaPandoc(req, path, "docx", referenceDocx)
}

// ExportOdt amosa un diálogo "Gardar como" para un .odt, xera o exame e
// convérteo con Pandoc usando referenceOdt como plantilla de estilos - ver
// exportViaPandoc.
func (a *App) ExportOdt(req GenerateMarkdownRequest, defaultName string) (GenerateDocResult, error) {
	result := GenerateDocResult{}
	dir, base := splitDialogDefault(defaultName)
	path, err := saveFileDialogCompat("Exportar OpenDocument (.odt)", dir, base, "OpenDocument (*.odt)", "*.odt")
	if err != nil || path == "" {
		return result, err
	}
	return a.exportViaPandoc(req, path, "odt", referenceOdt)
}
