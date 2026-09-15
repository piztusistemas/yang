package main

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReferenceTemplatesAreValidArchives comproba que referenceDocx e
// referenceOdt (embebidos con go:embed) son de verdade ficheiros ZIP válidos
// co ficheiro de estilos que exportViaPandoc precisa - se algún se corrompe
// nunha edición manual futura, isto falla no acto en vez de só ao premer o
// botón DOCX/ODT na app.
func TestReferenceTemplatesAreValidArchives(t *testing.T) {
	cases := []struct {
		name        string
		data        []byte
		stylesEntry string
	}{
		{"reference.docx", referenceDocx, "word/styles.xml"},
		{"reference.odt", referenceOdt, "styles.xml"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := zip.NewReader(bytes.NewReader(c.data), int64(len(c.data)))
			if err != nil {
				t.Fatalf("%s non é un ZIP válido: %v", c.name, err)
			}
			found := false
			for _, f := range r.File {
				if f.Name == c.stylesEntry {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s non contén %s", c.name, c.stylesEntry)
			}
		})
	}
}

// TestExportViaPandoc exercises App.exportViaPandoc (the dialog-free core
// behind ExportDocx/ExportOdt) against mini.matex, all the way through
// Maxima + Pandoc to real .docx/.odt bytes.
func TestExportViaPandoc(t *testing.T) {
	// configIllado (plantillas_test.go): sen isto a proba compilaría coa
	// PLANTILLA que teña activa quen a executa (plantillas.go), e o
	// resultado dependería da súa configuración persoal en vez do código.
	configIllado(t)
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" {
		t.Skip("maxima not found on this machine, skipping")
	}
	if !commandExists("pandoc") {
		t.Skip("pandoc not found on this machine, skipping")
	}

	source, err := os.ReadFile(filepath.Join("testdata", "mini.matex"))
	if err != nil {
		t.Fatalf("reading testdata/mini.matex: %v", err)
	}
	req := GenerateMarkdownRequest{Source: string(source), Iterations: 1}

	for _, format := range []string{"docx", "odt"} {
		t.Run(format, func(t *testing.T) {
			reference := referenceDocx
			if format == "odt" {
				reference = referenceOdt
			}
			outPath := filepath.Join(t.TempDir(), "exame."+format)
			result, err := a.exportViaPandoc(req, outPath, format, reference)
			if err != nil {
				t.Fatalf("exportViaPandoc: %v", err)
			}
			if result.Path != outPath {
				t.Errorf("Path = %q, quería %q", result.Path, outPath)
			}
			data, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatalf("reading output: %v", err)
			}
			// .docx/.odt son ambos ficheiros ZIP (sinatura "PK").
			if !bytes.HasPrefix(data, []byte("PK")) {
				t.Fatalf("a saída non semella un %s (%d bytes)", format, len(data))
			}
		})
	}
}
