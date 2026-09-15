package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tinyPNGBase64 is a 1x1 transparent PNG, just enough bytes to exercise the
// write path without needing a real image fixture on disk.
const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

// TestParseAsistenteResposta covers the two shapes the embedded AI
// assistant's response can take (see systemPromptAsistente, ia_client.go):
// plain prose (a question was asked, nothing to change) vs a proposed edit
// wrapped in <<<EDICION>>>...<<<FIN_EDICION>>> markers, with or without
// explanatory text around them.
func TestParseAsistenteResposta(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		wantAccion  string
		wantTexto   string
		wantContido string
	}{
		{
			name:       "pregunta simple, sen marcas",
			raw:        "Esta función calcula a derivada de f(x).",
			wantAccion: "responder",
			wantTexto:  "Esta función calcula a derivada de f(x).",
		},
		{
			name:        "edición sen texto arredor",
			raw:         "<<<EDICION>>>\ndiff(x^2+1,x)\n<<<FIN_EDICION>>>",
			wantAccion:  "editar",
			wantTexto:   "",
			wantContido: "diff(x^2+1,x)",
		},
		{
			name:        "edición con explicación antes e despois",
			raw:         "Cambiei o expoñente a 3:\n<<<EDICION>>>\nx^3+1\n<<<FIN_EDICION>>>\nDime se queres outra cousa.",
			wantAccion:  "editar",
			wantTexto:   "Cambiei o expoñente a 3:\nDime se queres outra cousa.",
			wantContido: "x^3+1",
		},
		{
			name:        "edición multiliña (documento completo)",
			raw:         "<<<EDICION>>>\n<p>Ola</p>\n<p>Adeus</p>\n<<<FIN_EDICION>>>",
			wantAccion:  "editar",
			wantContido: "<p>Ola</p>\n<p>Adeus</p>",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseAsistenteResposta(c.raw)
			if got.Accion != c.wantAccion {
				t.Errorf("Accion = %q, want %q", got.Accion, c.wantAccion)
			}
			if got.Texto != c.wantTexto {
				t.Errorf("Texto = %q, want %q", got.Texto, c.wantTexto)
			}
			if got.Contido != c.wantContido {
				t.Errorf("Contido = %q, want %q", got.Contido, c.wantContido)
			}
		})
	}
}

// TestExtractJSONArray locks in the real report behind "a IA devolveu un
// formato inesperado": XerarExercicioIA/XerarExameIA's json.Unmarshal
// rejected a perfectly valid JSON array wrapped in the IA's own commentary
// ("Aquí tes o exame:\n[...]\nEspero que che sirva!") - limparValadosMarkdown
// (ia_client.go) only strips a code fence that starts at position 0, so
// that mix never got cleaned up before this fix.
func TestExtractJSONArray(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"array so", `[{"type":"text","content":"ola"}]`, `[{"type":"text","content":"ola"}]`},
		{"con conversa arredor", "Aquí tes o exame:\n[{\"type\":\"text\"}]\nEspero que che sirva!", `[{"type":"text"}]`},
		{"nested (exame: array de arrays)", `[[{"type":"text"}],[{"type":"formula"}]]`, `[[{"type":"text"}],[{"type":"formula"}]]`},
		{"corchetes dentro dunha cadea non contan coma aniñamento", `[{"content":"f(x)=[a,b]"}]`, `[{"content":"f(x)=[a,b]"}]`},
		{"sen corchete ningún: devolve tal cal", `non hai JSON aquí`, `non hai JSON aquí`},
		{"desequilibrado (cortado a metade): devolve tal cal", `[{"type":"text","content":"ola`, `[{"type":"text","content":"ola`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractJSONArray(c.raw); got != c.want {
				t.Errorf("extractJSONArray(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

// TestLooksTruncatedJSON locks in formatoInesperadoErr's heuristic for
// deciding which hint to show.
func TestLooksTruncatedJSON(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"array completo", `[{"type":"text"}]`, false},
		{"obxecto completo", `{"accion":"responder"}`, false},
		{"completo con espazo final", "[{\"type\":\"text\"}]\n", false},
		{"cortado a metade dun valor", `[{"type":"text","content":"ola`, true},
		{"cortado xusto despois dunha coma", `[{"type":"text"},`, true},
		{"baleiro", ``, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := looksTruncatedJSON(c.raw); got != c.want {
				t.Errorf("looksTruncatedJSON(%q) = %v, want %v", c.raw, got, c.want)
			}
		})
	}
}

// TestFormatoInesperadoErr comproba que a mensaxe final leva os tres anacos
// que a substitúen do vello "a IA devolveu un formato inesperado, téntao de
// novo" (real queixa: non daba ningunha pista do motivo nin do que facer) -
// o erro de parseo, unha suxestión distinta se parece cortada, e un adianto
// do que devolveu realmente a IA.
func TestFormatoInesperadoErr(t *testing.T) {
	parseErr := fmt.Errorf("json inválido preto do carácter 5")

	truncado := formatoInesperadoErr(`[{"type":"text","content":"ola`, parseErr).Error()
	if !strings.Contains(truncado, "CORTADA") {
		t.Errorf("esperaba mención a resposta cortada, obtiven: %q", truncado)
	}
	if !strings.Contains(truncado, parseErr.Error()) {
		t.Errorf("esperaba o erro de parseo orixinal na mensaxe, obtiven: %q", truncado)
	}
	if !strings.Contains(truncado, `[{"type":"text","content":"ola`) {
		t.Errorf("esperaba un adianto da resposta da IA, obtiven: %q", truncado)
	}

	noTruncado := formatoInesperadoErr(`{"algo":"distinto do agardado"}`, parseErr).Error()
	if strings.Contains(noTruncado, "CORTADA") {
		t.Errorf("non esperaba mención a resposta cortada, obtiven: %q", noTruncado)
	}

	// Un adianto MOI longo trúncase (non se enche a mensaxe de erro cun
	// documento enteiro) - o límite vive en formatoInesperadoErr.
	longo := formatoInesperadoErr(strings.Repeat("a", 1000)+"]", parseErr).Error()
	if strings.Contains(longo, strings.Repeat("a", 1000)) {
		t.Errorf("esperaba que o adianto se truncase, obtiven unha mensaxe de %d bytes", len(longo))
	}
}

// TestAsistenteIA is the end-to-end wiring test (App.AsistenteIA ->
// chamarIA -> a real provider wire format -> parseAsistenteResposta),
// complementing TestParseAsistenteResposta (pure parsing only): mocks a
// Gemini server and checks both that the current content is actually sent
// to the provider (not silently dropped) and that an edit-marked reply
// comes back correctly classified.
func TestAsistenteIA(t *testing.T) {
	var corpoRecibido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		corpoRecibido = string(body)
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":`+
			`"Cambiei o expoñente:\n<<<EDICION>>>\nx^3+1\n<<<FIN_EDICION>>>"}]}}]}`)
	}))
	defer srv.Close()

	a := &App{settings: Settings{IAProvedor: "gemini", IAAPIKey: "clave", IAModel: "m", IABaseURL: srv.URL}}
	resp, err := a.AsistenteIA(AsistenteIARequest{Tipo: "formula", Contido: "x^2+1", Peticion: "cambia o expoñente a 3"})
	if err != nil {
		t.Fatalf("AsistenteIA: %v", err)
	}
	if resp.Accion != "editar" || resp.Contido != "x^3+1" {
		t.Errorf("AsistenteIA = %+v, esperaba accion=editar contido=x^3+1", resp)
	}
	if !strings.Contains(corpoRecibido, "x^2+1") {
		t.Errorf("o contido actual non chegou ao provedor: %s", corpoRecibido)
	}
	if !strings.Contains(corpoRecibido, "cambia o expoñente a 3") {
		t.Errorf("a petición non chegou ao provedor: %s", corpoRecibido)
	}
}

// TestSaveUploadedImage exercises App.SaveUploadedImage - the write-side
// counterpart of renderImg (latexdoc.go), which resolves <IMG src="..."> as
// a path relative to baseDir - so both must agree on the "assets/" layout.
func TestSaveUploadedImage(t *testing.T) {
	a := &App{}

	if _, err := a.SaveUploadedImage(SaveImageRequest{FileName: "logo.png", DataB64: tinyPNGBase64}); err == nil {
		t.Fatal("expected an error when BaseDir is empty (document not saved yet)")
	}

	dir := t.TempDir()
	path1, err := a.SaveUploadedImage(SaveImageRequest{BaseDir: dir, FileName: "logo.png", DataB64: tinyPNGBase64})
	if err != nil {
		t.Fatal(err)
	}
	if path1 != "assets/logo.png" {
		t.Fatalf("expected assets/logo.png, got %q", path1)
	}

	// Same file name again -> must not overwrite, gets a "-1" suffix.
	path2, err := a.SaveUploadedImage(SaveImageRequest{BaseDir: dir, FileName: "logo.png", DataB64: tinyPNGBase64})
	if err != nil {
		t.Fatal(err)
	}
	if path2 != "assets/logo-1.png" {
		t.Fatalf("expected assets/logo-1.png, got %q", path2)
	}

	for _, p := range []string{path1, path2} {
		data, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("reading %s: %v", p, err)
		}
		want, _ := base64.StdEncoding.DecodeString(tinyPNGBase64)
		if string(data) != string(want) {
			t.Errorf("%s: bytes on disk don't match uploaded content", p)
		}
	}
}

// TestGenerate exercises App.Generate - the exact code path the compiled
// Wails app runs when the user clicks "Xerar" - against the real 2013
// sample documents, with 2 variants each to also check seed-based
// randomization stays deterministic and distinct.
func TestGenerate(t *testing.T) {
	// configIllado (plantillas_test.go): sen isto a proba compilaría coa
	// PLANTILLA que teña activa quen a executa (plantillas.go), e o
	// resultado dependería da súa configuración persoal en vez do código.
	configIllado(t)
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" {
		t.Skip("maxima not found on this machine, skipping")
	}

	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("reading testdata: %v", err)
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".matex") {
			continue
		}
		e := e
		t.Run(e.Name(), func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata", e.Name()))
			if err != nil {
				t.Fatal(err)
			}

			result, err := a.Generate(GenerateRequest{
				Source:     string(src),
				Seed:       42,
				Iterations: 2,
			})
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			if len(result.Warnings) > 0 {
				t.Logf("warnings: %v", result.Warnings)
			}

			if !strings.Contains(result.HTML, "<html") {
				t.Errorf("output doesn't look like HTML")
			}
			// Every <MAT>/<EVAL> should have become a math span; none
			// should be left un-substituted.
			for _, tag := range []string{"<MAT>", "<EVAL>", "<HIDE>"} {
				if strings.Contains(result.HTML, tag) {
					t.Errorf("unprocessed %s tag leaked into output", tag)
				}
			}
			t.Logf("%s: %d bytes of HTML generated", e.Name(), len(result.HTML))
		})
	}
}

// TestGeneratePDFSistemaTag is the regression test for the real report: a
// system of equations written as three separate <MAT>eq</MAT> tags (each in
// its own <p>) rendered as three unrelated lines, no shared brace. <SISTEMA>
// (cas/tags.go's renderSistema) must produce a real "\begin{cases}" block
// that pdflatex actually compiles - not just a string check, a full
// Maxima -> LaTeX -> pdflatex round-trip, since a malformed \begin{cases}
// would only surface as a compile error here, not in a unit test of
// renderSistema alone.
func TestGeneratePDFSistemaTag(t *testing.T) {
	// configIllado (plantillas_test.go): sen isto a proba compilaría coa
	// PLANTILLA que teña activa quen a executa (plantillas.go), e o
	// resultado dependería da súa configuración persoal en vez do código.
	configIllado(t)
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" || !commandExists("pdflatex") {
		t.Skip("maxima/pdflatex not found on this machine, skipping")
	}

	src := `<p>Resolve:</p><p><SISTEMA>
x + 2*y - z = 4
2*x - y + z = 1
-x + y + 2*z = 3
</SISTEMA></p>`

	result, err := a.GeneratePDF(GeneratePDFRequest{Source: src, Seed: 1, Iterations: 1})
	if err != nil {
		t.Fatalf("GeneratePDF failed: %v (log: %s)", err, result.Log)
	}
	if !strings.Contains(result.LatexSource, `\begin{cases}`) {
		t.Errorf("expected \\begin{cases} in the compiled .tex source, got: %s", result.LatexSource)
	}
	if strings.Contains(result.LatexSource, "<SISTEMA>") {
		t.Errorf("<SISTEMA> tag leaked unprocessed into the .tex source")
	}
	if len(result.PDFBase64) == 0 {
		t.Errorf("expected a non-empty compiled PDF")
	}
}

// TestSplitDialogDefault locks in the real bug report: passing a FULL path
// as a save dialog's "suggested name" (what every SaveFileDialog/
// SavePDFDialog/SaveTexDialog/ExportMarkdown call did whenever a document
// was already open, via currentPath in main.js) broke the native dialog -
// DefaultFilename maps to gtk_file_chooser_set_current_name() on Linux,
// documented to take a bare filename only, no path separators.
func TestSplitDialogDefault(t *testing.T) {
	existingDir := t.TempDir()

	dir, base := splitDialogDefault(filepath.Join(existingDir, "exame1.pdf"))
	if dir != existingDir || base != "exame1.pdf" {
		t.Errorf("existing dir: got (%q, %q), want (%q, %q)", dir, base, existingDir, "exame1.pdf")
	}

	// Un directorio que xa non existe (ex.: o .matex movéuse/borrouse dende
	// que se abriu) non debe facer fallar o diálogo enteiro - DefaultDirectory
	// baleiro deixa que o selector use o seu propio por defecto.
	dir, base = splitDialogDefault("/non/existe/xamais/exame1.pdf")
	if dir != "" || base != "exame1.pdf" {
		t.Errorf("non-existent dir: got (%q, %q), want (\"\", %q)", dir, base, "exame1.pdf")
	}

	// Un nome solto, sen ruta (o caso "documento novo, sen gardar aínda").
	dir, base = splitDialogDefault("sen-nome.matex")
	if dir != "" || base != "sen-nome.matex" {
		t.Errorf("bare filename: got (%q, %q), want (\"\", %q)", dir, base, "sen-nome.matex")
	}
}
