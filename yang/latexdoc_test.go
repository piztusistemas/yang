package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLatexConverterIgnoresExTag is the regression test for the block
// editor's structural <EX> tag (blocks-serialize.js): it must render like
// span/a/font (children only, no wrapper) and must NOT be recorded as an
// unsupported tag, or every PDF export of a block-editor document would
// carry a spurious "etiqueta HTML sen soporte" warning.
func TestLatexConverterIgnoresExTag(t *testing.T) {
	nodes, err := parseFragment("<EX>ola</EX>")
	if err != nil {
		t.Fatal(err)
	}
	conv := newLatexConverter("", "")
	var out strings.Builder
	for _, n := range nodes {
		out.WriteString(conv.render(n))
	}
	if !strings.Contains(out.String(), "ola") {
		t.Fatalf("expected content to pass through, got: %q", out.String())
	}
	if conv.unknownTags["ex"] {
		t.Errorf("<EX> should not be flagged as an unsupported tag")
	}
}

// TestLatexConverterOrderedListType: <ol type="a"> ten que dar apartados
// "a)", "b)"... (convención de exames), non "1." "2.". Report real: os
// boletíns saían cos apartados numerados. <ol> normal non cambia.
func TestLatexConverterOrderedListType(t *testing.T) {
	render := func(html string) string {
		nodes, err := parseFragment(html)
		if err != nil {
			t.Fatal(err)
		}
		conv := newLatexConverter("", "")
		var out strings.Builder
		for _, n := range nodes {
			out.WriteString(conv.render(n))
		}
		return out.String()
	}

	if got := render(`<ol type="a"><li>un</li><li>dous</li></ol>`); !strings.Contains(got, `\begin{enumerate}[label=\alph*)]`) {
		t.Errorf(`<ol type="a"> non xerou a etiqueta \alph*): %q`, got)
	}
	if got := render(`<ol type="I" start="3"><li>un</li></ol>`); !strings.Contains(got, `\begin{enumerate}[label=\Roman*), start=3]`) {
		t.Errorf(`<ol type="I" start="3"> mal traducido: %q`, got)
	}
	if got := render(`<ol><li>un</li></ol>`); !strings.Contains(got, "\\begin{enumerate}\n") {
		t.Errorf("un <ol> normal non debe levar opcións: %q", got)
	}

	if got := enumerateOpts("", ""); got != "" {
		t.Errorf(`enumerateOpts("","") = %q, want ""`, got)
	}
	if got := enumerateOpts("a", ""); got != `[label=\alph*)]` {
		t.Errorf(`enumerateOpts("a","") = %q`, got)
	}
}

// TestLatexConverterRendersHr is the regression test for "⚠ etiqueta HTML
// sen soporte en PDF: <hr>" showing up on any document with a horizontal
// rule: <hr> must render as an actual visible line (not silently vanish,
// which is what the default case would do - <hr> is a void element, no
// children to pass through) and must NOT be recorded as an unsupported tag.
func TestLatexConverterRendersHr(t *testing.T) {
	nodes, err := parseFragment("<p>antes</p><hr><p>despois</p>")
	if err != nil {
		t.Fatal(err)
	}
	conv := newLatexConverter("", "")
	var out strings.Builder
	for _, n := range nodes {
		out.WriteString(conv.render(n))
	}
	if !strings.Contains(out.String(), `\rule{\linewidth}`) {
		t.Errorf("expected a \\rule (horizontal line), got: %q", out.String())
	}
	if conv.unknownTags["hr"] {
		t.Errorf("<hr> should not be flagged as an unsupported tag")
	}
}

// TestLatexConverterRendersLinkExternal locks in <A href="https://...">
// becoming a real clickable \href in the generated PDF (see
// \usepackage[hidelinks]{hyperref} in the preamble templates) instead of
// silently dropping to plain text (the old "a" behaviour, grouped with
// span/font as "children only, no wrapper") - no baseDir/buildDir needed,
// an external URL never touches disk.
func TestLatexConverterRendersLinkExternal(t *testing.T) {
	nodes, err := parseFragment(`ver <A href="https://example.org/guia.pdf">a guía</A>`)
	if err != nil {
		t.Fatal(err)
	}
	conv := newLatexConverter("", "")
	var out strings.Builder
	for _, n := range nodes {
		out.WriteString(conv.render(n))
	}
	want := `\href{https://example.org/guia.pdf}{a guía}`
	if !strings.Contains(out.String(), want) {
		t.Errorf("expected %q, got: %q", want, out.String())
	}
	if conv.unknownTags["a"] {
		t.Errorf("<a> should not be flagged as an unsupported tag")
	}
}

// TestLatexConverterRendersLinkLocal locks in a LOCAL <A href="..."> (a
// PDF uploaded via the explorer's "⬆️ Subir ficheiro" button, see
// explorer.go's SubirFicheiroCartafol) being found next to baseDir, copied
// into buildDir (same treatment as renderImg), and linked by its bare
// filename - mirrors how <IMG src="..."> already works.
func TestLatexConverterRendersLinkLocal(t *testing.T) {
	baseDir := t.TempDir()
	buildDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "modelo.pdf"), []byte("%PDF-fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	nodes, err := parseFragment(`<A href="modelo.pdf">o modelo</A>`)
	if err != nil {
		t.Fatal(err)
	}
	conv := newLatexConverter(baseDir, buildDir)
	var out strings.Builder
	for _, n := range nodes {
		out.WriteString(conv.render(n))
	}
	want := `\href{modelo.pdf}{o modelo}`
	if !strings.Contains(out.String(), want) {
		t.Errorf("expected %q, got: %q", want, out.String())
	}
	if _, err := os.Stat(filepath.Join(buildDir, "modelo.pdf")); err != nil {
		t.Errorf("expected modelo.pdf copied into buildDir: %v", err)
	}
	if len(conv.warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", conv.warnings)
	}
}

// TestLatexConverterRendersLinkMissingFile locks in the warning path (same
// criterion as renderImg's "non se atopou a imaxe"): a dangling local
// href must not abort the whole document, just fall back to the link's
// plain text and record a warning.
func TestLatexConverterRendersLinkMissingFile(t *testing.T) {
	baseDir := t.TempDir()
	nodes, err := parseFragment(`<A href="non-existe.pdf">o modelo</A>`)
	if err != nil {
		t.Fatal(err)
	}
	conv := newLatexConverter(baseDir, t.TempDir())
	var out strings.Builder
	for _, n := range nodes {
		out.WriteString(conv.render(n))
	}
	if strings.Contains(out.String(), `\href`) {
		t.Errorf("expected no \\href for a missing file, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "o modelo") {
		t.Errorf("expected the link text to survive as plain text, got: %q", out.String())
	}
	if len(conv.warnings) == 0 {
		t.Errorf("expected a warning about the missing file")
	}
}

// TestLatexConverterRendersSubSup is the regression test for "limx → ∞"
// showing up flat/unstacked in the PDF instead of a proper subscript: <sub>/
// <sup> fell into the default case (unknown tag, children-only) before this
// fix, so "lim<sub>x → ∞</sub>" lost its subscript entirely. Must render as
// $_{...}$/$^{...}$ and must NOT be recorded as an unsupported tag.
func TestLatexConverterRendersSubSup(t *testing.T) {
	nodes, err := parseFragment("lim<sub>x → ∞</sub> e x<sup>2</sup>")
	if err != nil {
		t.Fatal(err)
	}
	conv := newLatexConverter("", "")
	var out strings.Builder
	for _, n := range nodes {
		out.WriteString(conv.render(n))
	}
	got := out.String()
	if !strings.Contains(got, `$_{x `) {
		t.Errorf("expected a $_{...} subscript, got: %q", got)
	}
	if !strings.Contains(got, `$^{2}$`) {
		t.Errorf("expected a $^{...} superscript, got: %q", got)
	}
	if conv.unknownTags["sub"] || conv.unknownTags["sup"] {
		t.Errorf("<sub>/<sup> should not be flagged as unsupported tags")
	}
}

// TestImgIncludeOptions covers the width/height sizing the block editor's
// "Inserir imaxe" control (blocks.js) adds to <IMG>: no size at all keeps
// the original centred/30%-width default (standalone=true), one dimension
// keeps the aspect ratio, and both dimensions together stretch to that
// exact box - same semantics as plain HTML <img width height>.
func TestImgIncludeOptions(t *testing.T) {
	cases := []struct {
		name           string
		width, height  string
		wantOpts       string
		wantStandalone bool
	}{
		{"none", "", "", `width=0.3\textwidth`, true},
		{"width only, px", "120", "", "width=3.175cm,keepaspectratio", false},
		{"height only, cm unit", "", "4cm", "height=4cm,keepaspectratio", false},
		{"both, mixed units", "5cm", "60mm", "width=5cm,height=60mm", false},
		{"percent width", "50%", "", `width=0.5\textwidth,keepaspectratio`, false},
		{"unparseable falls back to standalone", "auto", "", `width=0.3\textwidth`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts, standalone := imgIncludeOptions(c.width, c.height)
			if opts != c.wantOpts || standalone != c.wantStandalone {
				t.Errorf("imgIncludeOptions(%q, %q) = (%q, %v), want (%q, %v)",
					c.width, c.height, opts, standalone, c.wantOpts, c.wantStandalone)
			}
		})
	}
}

// TestMotoresParaProbarPreferidoInexistente is the regression test for the
// "usuario ten mal seleccionado o compilador" case: if the configured
// engine doesn't exist as a command at all, motoresParaProbar must still
// offer whatever real engines ARE installed (as fallback candidates),
// instead of returning a list that only ever contains the broken choice.
func TestMotoresParaProbarPreferidoInexistente(t *testing.T) {
	candidatos := motoresParaProbar("motor-que-non-existe-xyz")
	for _, m := range candidatos {
		if m == "motor-que-non-existe-xyz" {
			t.Fatalf("motoresParaProbar non debería devolver un motor que non existe: %v", candidatos)
		}
	}
	// Se hai polo menos un motor LaTeX real instalado nesta máquina, ten
	// que aparecer na lista (é o que permite o fallback).
	for _, m := range []string{"pdflatex", "xelatex", "lualatex"} {
		if commandExists(m) {
			found := false
			for _, c := range candidatos {
				if c == m {
					found = true
				}
			}
			if !found {
				t.Errorf("%s está instalado pero non aparece entre os candidatos: %v", m, candidatos)
			}
		}
	}
}

// TestCompileLatexAutoFallback exercises the transparent-fallback path
// itself: with a preferred engine that can't run at all, compileLatexAuto
// must still produce a PDF using whatever real engine is installed, and
// report that engine back via usedEngine so the caller can warn the user.
func TestCompileLatexAutoFallback(t *testing.T) {
	if !commandExists("pdflatex") {
		t.Skip("pdflatex not found on this machine, skipping")
	}
	dir := t.TempDir()
	pdf, _, log, usedEngine, _, err := compileLatexAuto(dir, "Ola mundo", "motor-que-non-existe-xyz", nil, "", nil)
	if err != nil {
		t.Fatalf("compileLatexAuto debería recuperarse co fallback, pero fallou: %v\nlog:\n%s", err, log)
	}
	if usedEngine == "" || usedEngine == "motor-que-non-existe-xyz" {
		t.Errorf("usedEngine debería ser un motor real instalado, foi %q", usedEngine)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("output doesn't look like a PDF (%d bytes)", len(pdf))
	}
}

// TestGeneratePDF exercises App.GeneratePDF - the exact code path the
// compiled app runs for "PDF (LaTeX)" - against the real 2013 sample
// documents, compiling all the way to actual PDF bytes with pdflatex.
func TestGeneratePDF(t *testing.T) {
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

	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("reading testdata: %v", err)
	}

	// test.matex references logo.png relative to this dir in the original
	// tree - point BaseDir there so the <IMG> path gets exercised too.
	baseDir := "/home/usuario/Descargas/matexe.2013/files.maxima/test.out"

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

			result, err := a.GeneratePDF(GeneratePDFRequest{
				Source:     string(src),
				Seed:       7,
				Iterations: 2,
				BaseDir:    baseDir,
			})
			if err != nil {
				t.Fatalf("GeneratePDF failed: %v\nlog:\n%s", err, result.Log)
			}
			if len(result.Warnings) > 0 {
				t.Logf("warnings: %v", result.Warnings)
			}

			pdf, decErr := base64.StdEncoding.DecodeString(result.PDFBase64)
			if decErr != nil {
				t.Fatalf("decoding PDF: %v", decErr)
			}
			if !bytes.HasPrefix(pdf, []byte("%PDF")) {
				t.Fatalf("output doesn't look like a PDF (%d bytes)", len(pdf))
			}
			t.Logf("%s: %d bytes of PDF generated", e.Name(), len(pdf))

			// Preview page images (pdfPageImages) back the block editor's
			// print preview - every compile should produce at least one.
			if len(result.PageImages) == 0 {
				t.Errorf("expected at least one page image, got none (warnings: %v)", result.Warnings)
			}
			for i, img := range result.PageImages {
				if !strings.HasPrefix(img, "data:image/png;base64,") {
					t.Errorf("page %d: expected a PNG data URI, got prefix %q", i, img[:min(40, len(img))])
				}
			}
			t.Logf("%s: %d page image(s)", e.Name(), len(result.PageImages))

			// Save it somewhere persistent (this run's t.TempDir) so it can
			// be eyeballed manually if needed.
			out := filepath.Join(t.TempDir(), e.Name()+".pdf")
			if err := os.WriteFile(out, pdf, 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("saved to %s", out)
		})
	}
}

// TestGeneratePDFBoletinConFormulaSimbolica reproduce o boletín do report:
// no modo "solucions", <MAT>V_1/R_1</MAT> ten que saír coma fórmula (V₁/R₁)
// e non coma "16,67 = 16,67", e <ol type="a"> coma "a)". Compila de verdade.
func TestGeneratePDFBoletinConFormulaSimbolica(t *testing.T) {
	configIllado(t)
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" || !commandExists("pdflatex") {
		t.Skip("maxima/pdflatex non atopados, sáltase")
	}

	src := `<HIDE>V_1: 220$ R_1: 13.2$ res_1: V_1/R_1$</HIDE>
<p>Calcula a intensidade.</p>
<SOL><ol type="a"><li><p><b>Intensidade:</b> Aplicamos a lei de Ohm:</p>
<p>I = <MAT>V_1/R_1</MAT> = <EVAL>res_1</EVAL> A</p></li></ol></SOL>`

	result, err := a.GeneratePDF(GeneratePDFRequest{Source: src, Seed: 1, Iterations: 1, Modo: "solucions"})
	if err != nil {
		t.Fatalf("GeneratePDF: %v\nlog:\n%s", err, result.Log)
	}
	tex := result.LatexSource
	// A fórmula simbólica: tex(V_1/R_1) citado dá "{{V_{1}}\over{R_{1}}}".
	if !strings.Contains(tex, "V_{1}") || !strings.Contains(tex, "R_{1}") {
		t.Errorf("a <MAT> do <SOL> non saíu simbólica:\n%s", tex)
	}
	// O <EVAL> na mesma liña si dá o número.
	if !strings.Contains(tex, "16.6") && !strings.Contains(tex, "16,6") {
		t.Errorf("o <EVAL> non deu o valor numérico:\n%s", tex)
	}
	// <ol type="a"> -> enumitem con \alph*).
	if !strings.Contains(tex, `\alph*)`) {
		t.Errorf(`<ol type="a"> non se traduciu a apartados "a)":\n%s`, tex)
	}
}

// TestPreamblesCarganTikzBabel: os dous preámbulos por defecto cargan a
// libraría "babel" de TikZ DESPOIS de tikz. Sen ela, con babel activo un
// "\draw[->]" nun <TIKZ> peta con "Argument of \language@active@arg> has
// an extra }" (report real).
func TestPreamblesCarganTikzBabel(t *testing.T) {
	for _, engine := range []string{"pdflatex", "xelatex"} {
		p := preambuloPorDefecto(engine)
		iTikz := strings.Index(p, `\usepackage{tikz}`)
		iBabel := strings.Index(p, `\usetikzlibrary{babel}`)
		if iTikz == -1 || iBabel == -1 {
			t.Fatalf("%s: falta \\usepackage{tikz} (%d) ou \\usetikzlibrary{babel} (%d)", engine, iTikz, iBabel)
		}
		if iBabel < iTikz {
			t.Errorf("%s: \\usetikzlibrary{babel} ten que ir despois de cargar tikz", engine)
		}
	}
}

// TestGeneratePDFTikzFrechaConBabel: un <TIKZ> cunha frecha ("\draw[->]")
// compila aínda con babel activo (galician/spanish fan "<"/">" activos).
func TestGeneratePDFTikzFrechaConBabel(t *testing.T) {
	configIllado(t)
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" || !commandExists("pdflatex") {
		t.Skip("maxima/pdflatex non atopados, sáltase")
	}

	src := "<p>Vector:</p>\n<TIKZ>\\draw[thick, ->] (0,0) -- (2,1); \\draw[<->] (0,-1) -- (2,-1);</TIKZ>\n<p>Feito.</p>"
	result, err := a.GeneratePDF(GeneratePDFRequest{Source: src, Seed: 1, Iterations: 1})
	if err != nil {
		t.Fatalf("GeneratePDF cun <TIKZ> de frechas non debería fallar: %v\nlog:\n%s", err, result.Log)
	}
	pdf, _ := base64.StdEncoding.DecodeString(result.PDFBase64)
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("non saíu un PDF (%d bytes)", len(pdf))
	}
}

// TestGeneratePDFAmbienteMalformadoNonTomba: un \begin solto (sen "{"), o
// erro habitual do contido da IA que confunde \begin{env} con <TAG>, xa non
// tumba a xeración - o PDF sae igual e queda un aviso.
func TestGeneratePDFAmbienteMalformadoNonTomba(t *testing.T) {
	configIllado(t)
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" || !commandExists("pdflatex") {
		t.Skip("maxima/pdflatex non atopados, sáltase")
	}

	// A IA mete LaTeX cru nun <TEX> confundindo \begin{env} con etiquetas:
	// "\begin<...>" chega SEN escapar ao corpo e petaba a compilación con
	// "Environment <undefined".
	src := "<p>Resolve:</p>\n<TEX>\\begin<center>x+y=1</center></TEX>\n<p>Feito.</p>"
	result, err := a.GeneratePDF(GeneratePDFRequest{Source: src, Seed: 1, Iterations: 1})
	if err != nil {
		t.Fatalf("GeneratePDF debería compilar pese ao \\begin solto: %v\nlog:\n%s", err, result.Log)
	}
	pdf, _ := base64.StdEncoding.DecodeString(result.PDFBase64)
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("non saíu un PDF (%d bytes)", len(pdf))
	}
	got := strings.Join(result.Warnings, " | ")
	if !strings.Contains(got, `\begin/\end`) {
		t.Errorf("esperábase un aviso sobre \\begin/\\end sen ambiente; avisos: %v", result.Warnings)
	}
}

// TestExtraerErroPDFLatexLocalizacionTrasInsertedText: os erros do tipo
// "Missing ... inserted" meten "<inserted text>" entre o "!" e a liña
// "l.NN", así que a localización non vai pegada ao erro. Log real de
// xelatex - sen isto, o profesorado recibía o erro sen dicir en que liña.
func TestExtraerErroPDFLatexLocalizacionTrasInsertedText(t *testing.T) {
	log := `Overfull \hbox (1.0pt too wide) in paragraph at lines 4--5
! Missing \endgroup inserted.
<inserted text> 
                \endgroup 
l.42 \end{tabular}
                  
No pages of output.`
	got := extraerErroPDFLatex(log)
	if !strings.Contains(got, `! Missing \endgroup inserted.`) {
		t.Errorf("falta a liña do erro: %q", got)
	}
	if !strings.Contains(got, "l.42") {
		t.Errorf("falta a localización l.42 (é o que fai o erro diagnosticable): %q", got)
	}
}

// TestExtraerErroPDFLatexNonMesturaErros: cada "!" queda coa SÚA
// localización; se un erro non ten "l.NN" antes do vindeiro "!", non se lle
// pode pegar a do erro seguinte.
func TestExtraerErroPDFLatexNonMesturaErros(t *testing.T) {
	log := `! Undefined control sequence.
l.10 \descoñecido
! Missing $ inserted.
<inserted text> 
                $
l.20 \end{document}`
	got := extraerErroPDFLatex(log)
	want := `! Undefined control sequence. | l.10 \descoñecido | ! Missing $ inserted. | l.20 \end{document}`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// TestExtraerErroPDFLatexSenLocalizacion: hai erros que nunca chegan a dar
// "l.NN" (o ficheiro rematou mentres se lía unha macro, por exemplo). Aí
// devólvese só a liña do erro, non a primeira "l." que apareza páxinas
// despois.
func TestExtraerErroPDFLatexSenLocalizacion(t *testing.T) {
	log := `! File ended while scanning use of \align.
<inserted text> 
                \par 
<*> doc.tex
          
` + strings.Repeat("algunha liña de recheo\n", 10) + `l.99 non me corresponde`
	got := extraerErroPDFLatex(log)
	if strings.Contains(got, "l.99") {
		t.Errorf("colleu unha localización allea moi afastada: %q", got)
	}
}

// TestLatexConverterRendersSmall é a regresión de "⚠ etiqueta HTML sen
// soporte en PDF (mantívose o contido): <small>" - report real ao compilar
// un documento con letra pequena. As catro etiquetas de aquí teñen
// equivalente LaTeX de base (sen paquetes extra), así que teñen que
// renderizar de verdade e NON quedar marcadas coma sen soporte.
func TestLatexConverterRendersSmall(t *testing.T) {
	casos := []struct{ html, agardado string }{
		{"<small>2 puntos</small>", `{\small 2 puntos}`},
		{"<big>título</big>", `{\large título}`},
		{"<code>plot2d</code>", `\texttt{plot2d}`},
		{"<kbd>Ctrl</kbd>", `\texttt{Ctrl}`},
		{"<blockquote>citado</blockquote>", `\begin{quote}`},
	}
	for _, c := range casos {
		nodes, err := parseFragment(c.html)
		if err != nil {
			t.Fatal(err)
		}
		conv := newLatexConverter("", "")
		var out strings.Builder
		for _, n := range nodes {
			out.WriteString(conv.render(n))
		}
		if !strings.Contains(out.String(), c.agardado) {
			t.Errorf("%s -> %q, esperaba que contivese %q", c.html, out.String(), c.agardado)
		}
		if len(conv.unknownTags) > 0 {
			t.Errorf("%s marcou etiquetas sen soporte: %v", c.html, conv.unknownTags)
		}
	}
}

// TestExtraerErroPDFLatexContinuacion: os erros de PAQUETE parten a
// explicación en liñas "(paquete)  ..." e deixan a primeira baleira tras os
// dous puntos. Report real: o profesorado recibía "! Package fontspec
// Error:" a secas, sen o motivo. Teñen que vir xuntos nunha soa frase.
func TestExtraerErroPDFLatexContinuacion(t *testing.T) {
	log := `This is XeTeX
(/usr/local/texlive/2026/texmf-dist/tex/latex/base/article.cls)
! Package fontspec Error: 
(fontspec)                The font "Fira Sans" cannot be found; this may be
(fontspec)                but usually is not a fontspec bug.

l.12 \setsansfont{Fira Sans}
`
	got := extraerErroPDFLatex(log)
	if !strings.Contains(got, `The font "Fira Sans" cannot be found`) {
		t.Errorf("perdeuse o motivo real do erro: %q", got)
	}
	if !strings.Contains(got, "l.12") {
		t.Errorf("perdeuse a localización: %q", got)
	}
}

// TestPistaLatexAccionable: os dous fallos típicos dunha plantilla escrita
// pola IA (fonte ou paquete que ese equipo non ten) teñen que traducirse a
// unha frase que diga que facer; calquera outro erro non se toca.
func TestPistaLatexAccionable(t *testing.T) {
	fonte := pistaLatexAccionable(`(fontspec)  The font "Fira Sans" cannot be found; this may be`)
	if !strings.Contains(fonte, "Fira Sans") || !strings.Contains(fonte, "non está instalada") {
		t.Errorf("pista de fonte pouco útil: %q", fonte)
	}
	paquete := pistaLatexAccionable("! LaTeX Error: File `tcolorbox.sty' not found.")
	if !strings.Contains(paquete, "tcolorbox") || !strings.Contains(paquete, "tlmgr install tcolorbox") {
		t.Errorf("pista de paquete pouco útil: %q", paquete)
	}
	if outro := pistaLatexAccionable("! Undefined control sequence.\nl.4 \\foo"); outro != "" {
		t.Errorf("un erro non recoñecido non debe inventar pista: %q", outro)
	}
	amb := pistaLatexAccionable("! LaTeX Error: Environment cases undefined.\nl.45 \\begin{cases}")
	if !strings.Contains(amb, "cases") || !strings.Contains(amb, "<SISTEMA>") {
		t.Errorf("pista de ambiente indefinido pouco útil: %q", amb)
	}
}

func TestSanearAmbientesLatex(t *testing.T) {
	// \begin/\end sen "{": neutralízanse e avísase.
	in := `Antes \begin<SISTEMA>\nx+y=1\n\end<SISTEMA> despois \begin  fin`
	out, avisos := sanearAmbientesLatex(in)
	if strings.Contains(out, `\begin<`) || strings.Contains(out, `\end<`) {
		t.Errorf("quedou un \\begin/\\end solto: %q", out)
	}
	if !strings.Contains(out, `\textbackslash{}begin`) || !strings.Contains(out, `\textbackslash{}end`) {
		t.Errorf("non se neutralizou coma texto: %q", out)
	}
	if len(avisos) != 1 || !strings.Contains(avisos[0], "3") {
		t.Errorf("aviso inesperado (esperábanse 3 casos): %v", avisos)
	}

	// Os \begin{...} ben formados (os que emite o propio Yang) non se tocan,
	// nin \begingroup/\endinput e demais palabras que empezan por begin/end.
	ok := "\\begin{center}\\begin{tabular}{cc}a&b\\end{tabular}\\end{center}\n\\begingroup\\endinput"
	out2, avisos2 := sanearAmbientesLatex(ok)
	if out2 != ok || avisos2 != nil {
		t.Errorf("un corpo válido non debe cambiar:\n want %q\n got  %q\n avisos %v", ok, out2, avisos2)
	}

	// \begin{cases} solto (ambiente que existe pero mal colocado): déixase
	// pasar - non é o traballo desta función, que só quita os que non teñen
	// "{". Compróbase que NON se rompe ese caso.
	if out3, _ := sanearAmbientesLatex(`\begin{cases}x\end{cases}`); out3 != `\begin{cases}x\end{cases}` {
		t.Errorf("\\begin{cases} ben formado non se debe tocar: %q", out3)
	}
}

func TestSanearCondicionaisLatex(t *testing.T) {
	// Un "\iffalse" solto (o report "Incomplete \iffalse; all text was
	// ignored after line N"): desequilibrado -> neutralízase e avísase.
	in := `texto normal \iffalse escondido \node ... fin`
	out, avisos := sanearCondicionaisLatex(in)
	if strings.Contains(out, `\iffalse`) {
		t.Errorf("quedou un \\iffalse solto: %q", out)
	}
	if !strings.Contains(out, `\textbackslash{}iffalse`) {
		t.Errorf("non se neutralizou coma texto: %q", out)
	}
	if len(avisos) != 1 || !strings.Contains(avisos[0], "1") {
		t.Errorf("aviso inesperado: %v", avisos)
	}

	// Un "\fi" de máis, sen "\if..." diante: tamén desequilibrado.
	if out2, av2 := sanearCondicionaisLatex(`resultado \fi máis texto`); !strings.Contains(out2, `\textbackslash{}fi`) || len(av2) != 1 {
		t.Errorf("un \\fi orfo non se neutralizou: %q %v", out2, av2)
	}

	// Un condicional EQUILIBRADO (p.ex. dentro dun <TEX> lexítimo): non se toca.
	ok := `\ifmmode x \else y \fi`
	if out3, av3 := sanearCondicionaisLatex(ok); out3 != ok || av3 != nil {
		t.Errorf("un condicional equilibrado non se debe tocar:\n want %q\n got  %q\n avisos %v", ok, out3, av3)
	}

	// Sen condicionais ningún: identidade.
	if out4, av4 := sanearCondicionaisLatex(`só texto e $x^2$`); out4 != `só texto e $x^2$` || av4 != nil {
		t.Errorf("un corpo sen condicionais non debe cambiar: %q %v", out4, av4)
	}
}
