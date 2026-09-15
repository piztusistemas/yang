package main

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"matexe-wails/cas"
)

// examCheckPorDefecto é o exame co que se usa habitualmente esta proba. NON
// está no repositorio (é un ficheiro de traballo), así que cando non estea
// a proba SÁLTASE en vez de fallar - antes facía t.Fatal e deixaba a suite
// enteira en vermello en canto o ficheiro se movía ou se borraba, que é
// exactamente o que pasou.
const examCheckPorDefecto = "/home/usuario/Descargas/exame-1eso-matematicas.matex"

// TestExamCheck é a proba a man para pasar UN .matex concreto polos dous
// camiños de saída (HTML e PDF) e ver se algo peta. A cobertura automática
// dos .matex do repositorio faina TestGenerate (app_test.go) e a súa xemelga
// de PDF en latexdoc_test.go, sobre testdata/ - esta é para probar un exame
// de verdade que aínda non está aí.
//
// Para apuntala a outro ficheiro:
//
//	YANG_EXAM_CHECK=/ruta/ao/exame.matex go test -tags gtk3 -run TestExamCheck -v .
func TestExamCheck(t *testing.T) {
	ruta := os.Getenv("YANG_EXAM_CHECK")
	if ruta == "" {
		ruta = examCheckPorDefecto
	}
	src, err := os.ReadFile(ruta)
	if err != nil {
		t.Skipf("sen exame que probar (%s): pon YANG_EXAM_CHECK=/ruta/ao/exame.matex", ruta)
	}

	// configIllado (plantillas_test.go): igual ca TestGenerate, para non
	// compilar coa PLANTILLA persoal de quen executa a proba.
	configIllado(t)
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" {
		t.Skip("needs maxima")
	}

	t.Run("HTML", func(t *testing.T) {
		result, err := a.Generate(GenerateRequest{Source: string(src), Seed: 3, Iterations: 2})
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		if len(result.Warnings) > 0 {
			t.Logf("warnings: %v", result.Warnings)
		}
		if strings.Contains(result.HTML, cas.MarcaErroInline) {
			t.Errorf("an error was captured inline in the HTML output - check the log")
		}
		out := t.TempDir() + "/exam-check.html"
		os.WriteFile(out, []byte(result.HTML), 0o644)
		t.Logf("saved to %s (%d bytes)", out, len(result.HTML))
	})

	if !commandExists("pdflatex") {
		t.Skip("needs pdflatex")
	}
	t.Run("PDF", func(t *testing.T) {
		result, err := a.GeneratePDF(GeneratePDFRequest{Source: string(src), Seed: 3, Iterations: 1})
		if err != nil {
			t.Fatalf("GeneratePDF failed: %v\nlog:\n%s", err, result.Log)
		}
		if len(result.Warnings) > 0 {
			t.Logf("warnings: %v", result.Warnings)
		}
		pdf, _ := base64.StdEncoding.DecodeString(result.PDFBase64)
		out := t.TempDir() + "/exam-check.pdf"
		if err := os.WriteFile(out, pdf, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("saved to %s (%d bytes)", out, len(pdf))
	})
}
