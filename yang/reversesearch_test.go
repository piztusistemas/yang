package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// TestInsertOffsetMarkers is a pure-Go, fast check (no Maxima/pdflatex
// needed) that every Matexe tag in the source gets a "%MATEXOFF:<offset>"
// marker at the right UTF-16 offset (NOT a byte offset - the source
// deliberately has accented characters, "á"/"é", BEFORE both tags: a byte
// offset would be wrong there, since "á"/"é" are 2 bytes in UTF-8 but 1
// UTF-16 code unit - exactly the mismatch that made the very first version
// of this feature silently fail to highlight anything in a real, accented
// Galician document), and that the original text is never modified - only
// markers get spliced in.
func TestInsertOffsetMarkers(t *testing.T) {
	reg := newRawRegistry()
	source := "Están aquí: <MAT>1+1</MAT> e tamén <EVAL>2+2</EVAL> fin"
	marked := insertOffsetMarkers(source, reg)
	resolved := reg.Resolve(marked)

	wantMatOffset := utf16OffsetOf(source, strings.Index(source, "<MAT>"))
	wantEvalOffset := utf16OffsetOf(source, strings.Index(source, "<EVAL>"))

	if !strings.Contains(resolved, fmt.Sprintf("%%MATEXOFF:%d", wantMatOffset)) {
		t.Errorf("falta o marcador de <MAT> (offset UTF-16 %d) en:\n%s", wantMatOffset, resolved)
	}
	if !strings.Contains(resolved, fmt.Sprintf("%%MATEXOFF:%d", wantEvalOffset)) {
		t.Errorf("falta o marcador de <EVAL> (offset UTF-16 %d) en:\n%s", wantEvalOffset, resolved)
	}
	// Os marcadores só se ENGADEN antes de cada etiqueta, nunca substitúen
	// nada - as propias etiquetas (co seu contido) teñen que seguir
	// aparecendo intactas (aínda que xa non contiguas ao resto do texto,
	// por teren un marcador xusto antes).
	for _, tag := range []string{"<MAT>1+1</MAT>", "<EVAL>2+2</EVAL>"} {
		if !strings.Contains(resolved, tag) {
			t.Errorf("a etiqueta %q xa non aparece intacta en:\n%s", tag, resolved)
		}
	}
}

// TestUTF16OffsetOf pins the exact bug this fixed: a byte offset and a
// UTF-16 offset diverge as soon as a multi-byte UTF-8 character (any
// Galician/Spanish accent) appears before the position being measured.
func TestUTF16OffsetOf(t *testing.T) {
	source := "Número: X" // "ú" is 2 bytes in UTF-8, 1 UTF-16 code unit
	byteOffset := strings.Index(source, "X")
	if got, want := utf16OffsetOf(source, byteOffset), 8; got != want {
		t.Errorf("utf16OffsetOf(%q, %d) = %d, want %d", source, byteOffset, got, want)
	}
	if byteOffset == 8 {
		t.Fatal("erro no propio test: o byte offset debería diferir do UTF-16, non ser igual")
	}
}

// TestOffsetFromTexLine cobre o caso real: SyncTeX adoita devolver a liña
// COA PRIMEIRA CONTIDO despois do marcador (unha liña só-comentario non
// xera ningún glifo), non a propia liña do comentario - offsetFromTexLine
// ten que atopalo igual escaneando cara atrás en calquera dos dous casos.
func TestOffsetFromTexLine(t *testing.T) {
	dir := t.TempDir()
	texPath := filepath.Join(dir, "doc.tex")
	content := "liña1\nliña2\n%MATEXOFF:99\nliña4 con contido\nliña5\n"
	if err := os.WriteFile(texPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if got, err := offsetFromTexLine(texPath, 4); err != nil || got != 99 {
		t.Errorf("offsetFromTexLine(4) = %d, %v; want 99, <nil>", got, err)
	}
	if got, err := offsetFromTexLine(texPath, 3); err != nil || got != 99 {
		t.Errorf("offsetFromTexLine(3) = %d, %v; want 99, <nil>", got, err)
	}
	if _, err := offsetFromTexLine(texPath, 2); err == nil {
		t.Error("esperaba un erro cando non hai marcador antes desa liña")
	}
}

// TestReverseSearch exercita o pipeline completo de punta a punta:
// GeneratePDF (co -synctex=1 + insertOffsetMarkers novos) compila un
// documento con acentos ANTES da etiqueta (mesmo caso ca TestInsertOffset-
// Markers - reproduce o informe real, "Un número aleatorio: <EVAL>..."),
// e despois ReverseSearch recibe a posición EXACTA do PDF que o propio
// SyncTeX di que corresponde a esa etiqueta (vía "synctex view", a busca
// FORWARD - a dirección inversa da que proba ReverseSearch) e ten que
// resolver de volta ao mesmo offset UTF-16.
func TestReverseSearch(t *testing.T) {
	// configIllado (plantillas_test.go): sen isto a proba compilaría coa
	// PLANTILLA que teña activa quen a executa (plantillas.go), e o
	// resultado dependería da súa configuración persoal en vez do código.
	configIllado(t)
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" {
		t.Skip("maxima not found on this machine, skipping")
	}
	if !commandExists("pdflatex") {
		t.Skip("pdflatex not found on this machine, skipping")
	}
	if !commandExists("synctex") {
		t.Skip("synctex not found on this machine, skipping")
	}

	source := "Número: <MAT>1+1</MAT> resultado, é <EVAL>2+2</EVAL> fin.\n"
	wantOffset := utf16OffsetOf(source, strings.Index(source, "<MAT>"))

	result, err := a.GeneratePDF(GeneratePDFRequest{Source: source, Seed: 1, Iterations: 1})
	if err != nil {
		t.Fatalf("GeneratePDF: %v\nlog:\n%s", err, result.Log)
	}
	if len(result.PageImages) == 0 {
		t.Fatal("esperaba polo menos unha imaxe de páxina")
	}

	a.buildMu.Lock()
	lb := a.lastBuild
	a.buildMu.Unlock()
	if lb == nil {
		t.Fatal("esperaba que a.lastBuild quedase gardado despois dun GeneratePDF exitoso")
	}

	texBytes, err := os.ReadFile(lb.texPath)
	if err != nil {
		t.Fatal(err)
	}
	wantMarker := fmt.Sprintf("%%MATEXOFF:%d", wantOffset)
	markerLine := -1
	for i, l := range strings.Split(string(texBytes), "\n") {
		if strings.TrimSpace(l) == wantMarker {
			markerLine = i + 1 // synctex conta liñas dende 1
			break
		}
	}
	if markerLine == -1 {
		t.Fatalf("marcador %q non atopado en doc.tex:\n%s", wantMarker, texBytes)
	}

	pdfPath := filepath.Join(lb.dir, "doc.pdf")
	viewOut, err := exec.Command("synctex", "view", "-i", fmt.Sprintf("%d:1:doc.tex", markerLine), "-o", pdfPath).CombinedOutput()
	if err != nil {
		t.Fatalf("synctex view: %v: %s", err, viewOut)
	}
	page, xPt, yPt := parseSynctexViewOutputForTest(t, string(viewOut))
	if page == 0 {
		t.Fatalf("non se puido extraer a páxina da saída de synctex view:\n%s", viewOut)
	}

	// puntos PDF -> píxeles, coa mesma DPI ca ReverseSearch espera do
	// clic.
	xPx := xPt * pdfPreviewDPI / 72
	yPx := yPt * pdfPreviewDPI / 72

	got, err := a.ReverseSearch(page, xPx, yPx)
	if err != nil {
		t.Fatalf("ReverseSearch: %v", err)
	}
	if got.Offset != wantOffset {
		t.Errorf("ReverseSearch(%d, %.2f, %.2f) = offset %d; want %d (liña do marcador %d)",
			page, xPx, yPx, got.Offset, wantOffset, markerLine)
	}

	// A comprobación final que máis importa: o offset devolto ten que
	// funcionar tal cal en JavaScript (text.slice(offset)) - reproducindo
	// exactamente o que fai main.js, non só validando o número en abstracto.
	if !strings.HasPrefix(utf16Slice(source, got.Offset), "<MAT>1+1</MAT>") {
		t.Errorf("source[offset UTF-16 %d:] non empeza pola etiqueta <MAT>: %q", got.Offset, utf16Slice(source, got.Offset))
	}
}

// utf16Slice imita text.slice(offset) en JavaScript (índices en unidades
// UTF-16), para poder comprobar en Go que un offset devolto por
// ReverseSearch cae exactamente onde JavaScript o interpretaría.
func utf16Slice(s string, utf16Offset int) string {
	units := utf16.Encode([]rune(s))
	if utf16Offset > len(units) {
		return ""
	}
	return string(utf16.Decode(units[utf16Offset:]))
}

func parseSynctexViewOutputForTest(t *testing.T, out string) (page int, x, y float64) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "Page:"):
			fmt.Sscanf(line, "Page:%d", &page)
		case strings.HasPrefix(line, "x:"):
			fmt.Sscanf(line, "x:%f", &x)
		case strings.HasPrefix(line, "y:"):
			fmt.Sscanf(line, "y:%f", &y)
		}
	}
	return
}
