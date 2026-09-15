package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"matexe-wails/cas"
)

// configIllado illa TODA a configuración de Yang (settings.json,
// biblioteca.json, plantillas.json e o cartafol de imaxes das plantillas)
// nun temporal. XDG_CONFIG_HOME abonda en Linux, pero os.UserConfigDir
// ignórao en macOS (alí é $HOME/Library/Application Support), así que hai
// que fixar as dúas variables para que a proba non escriba na
// configuración real de quen a executa.
func configIllado(t *testing.T) {
	t.Helper()
	// A CACHÉ (a diferenza da configuración) séguese compartindo coa real,
	// e a propósito: en macOS o gnuplot-lua-tikz.sty que precisa CALQUERA
	// compilación vive en os.UserCacheDir()/yang (xérase unha vez, ver
	// latexenv_darwin.go) e tamén colga de HOME - cun HOME falso e baleiro,
	// pdflatex non atoparía o .sty e non compilaría nada. Ligar o cartafol
	// de caché ao real deixa iso funcionando sen sacar a configuración do
	// temporal.
	cacheReal, errCache := os.UserCacheDir()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	if errCache != nil {
		return
	}
	novaCache, err := os.UserCacheDir()
	if err != nil || novaCache == cacheReal {
		return
	}
	if err := os.MkdirAll(filepath.Dir(novaCache), 0o755); err == nil {
		_ = os.Symlink(cacheReal, novaCache)
	}
}

func TestPlantillasRoundTrip(t *testing.T) {
	configIllado(t)
	a := &App{}

	// A biblioteca arranca coas plantillas de exemplo sementadas.
	estado, err := a.ListarPlantillas()
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(estado.Plantillas) != len(plantillasDeFabrica()) {
		t.Fatalf("esperaba as plantillas de fábrica sementadas, atopei %d", len(estado.Plantillas))
	}
	if estado.Activa != "" {
		t.Errorf("por defecto non debería haber plantilla activa, hai %q", estado.Activa)
	}

	if _, err := a.GardarPlantilla(PlantillaLatex{Nome: "", Latex: "{{CORPO}}"}); err == nil {
		t.Error("esperaba erro sen nome")
	}
	if _, err := a.GardarPlantilla(PlantillaLatex{Nome: "Sen corpo", Latex: `\section{Ola}`}); err == nil {
		t.Error("esperaba erro sen o marcador {{CORPO}}")
	}
	if _, err := a.GardarPlantilla(PlantillaLatex{Nome: "MD sen corpo", Latex: "{{CORPO}}", Markdown: "# Ola"}); err == nil {
		t.Error("esperaba erro co Markdown sen {{CORPO}}")
	}

	nova, err := a.GardarPlantilla(PlantillaLatex{
		Nome:      "Circular",
		Latex:     "{{CENTRO}}\n\n{{CORPO}}\n",
		Variables: []PlantillaVar{{Nome: "centro educativo", Etiqueta: "Centro", Valor: "IES Xistral"}},
	})
	if err != nil {
		t.Fatalf("gardar: %v", err)
	}
	if nova.ID == "" {
		t.Fatal("esperaba unha ID")
	}
	if len(nova.Variables) != 1 || nova.Variables[0].Nome != "CENTRO_EDUCATIVO" {
		t.Errorf("o nome da variable debería normalizarse: %+v", nova.Variables)
	}

	// Editar (mesma ID) actualiza en vez de duplicar.
	nova.Nome = "Circular do centro"
	if _, err := a.GardarPlantilla(nova); err != nil {
		t.Fatalf("actualizar: %v", err)
	}
	estado, _ = a.ListarPlantillas()
	if len(estado.Plantillas) != len(plantillasDeFabrica())+1 {
		t.Fatalf("actualizar non debería crear outra plantilla: %d", len(estado.Plantillas))
	}

	// Activar, e lembrar por ficheiro.
	if err := a.EscollerPlantilla(nova.ID, ""); err != nil {
		t.Fatalf("escoller: %v", err)
	}
	if p := a.plantillaActiva(); p == nil || p.ID != nova.ID {
		t.Fatalf("plantillaActiva non devolveu a escollida: %+v", p)
	}

	if err := a.EliminarPlantilla(nova.ID); err != nil {
		t.Fatalf("eliminar: %v", err)
	}
	if p := a.plantillaActiva(); p != nil {
		t.Errorf("ao borrar a plantilla activa debe quedar ningunha, quedou %+v", p)
	}
}

// TestPlantillaPorFicheiro: cada documento lembra a súa plantilla, e abrir
// un que nunca tivo ningunha non desfai a escolla actual.
func TestPlantillaPorFicheiro(t *testing.T) {
	configIllado(t)
	a := &App{}
	p1, err := a.GardarPlantilla(PlantillaLatex{Nome: "Exame", Latex: "{{CORPO}}"})
	if err != nil {
		t.Fatalf("gardar: %v", err)
	}

	// O ficheiro ten que existir de verdade: podarPorFicheiro limpa as
	// entradas de documentos borrados.
	doc := t.TempDir() + "/exame.matex"
	if err := writeFileParaTest(doc); err != nil {
		t.Fatal(err)
	}
	if err := a.EscollerPlantilla(p1.ID, doc); err != nil {
		t.Fatalf("escoller: %v", err)
	}
	if err := a.EscollerPlantilla("", ""); err != nil {
		t.Fatalf("desactivar: %v", err)
	}

	id, err := a.PlantillaParaFicheiro(doc)
	if err != nil {
		t.Fatalf("plantillaParaFicheiro: %v", err)
	}
	if id != p1.ID {
		t.Errorf("esperaba recuperar %q para %s, obtiven %q", p1.ID, doc, id)
	}
	if p := a.plantillaActiva(); p == nil || p.ID != p1.ID {
		t.Error("abrir o documento debería activar a súa plantilla")
	}

	outro := t.TempDir() + "/outro.matex"
	if err := writeFileParaTest(outro); err != nil {
		t.Fatal(err)
	}
	id, _ = a.PlantillaParaFicheiro(outro)
	if id != p1.ID {
		t.Errorf("un documento sen plantilla lembrada debe manter a activa, deu %q", id)
	}
}

func TestSubstituirVariables(t *testing.T) {
	p := &PlantillaLatex{Variables: []PlantillaVar{
		{Nome: "CENTRO", Valor: "IES Xistral"},
		{Nome: "TITULO", Valor: "Exame de setembro"},
	}}
	auto := map[string]string{"DATA": "01/09/2026", "TITULO": "exame"}

	out, avisos := substituirVariables("{{CENTRO}} - {{DATA}} - {{TITULO}} - {{CORPO}} - {{NADA}}", p, auto, "", nil)
	if want := "IES Xistral - 01/09/2026 - Exame de setembro - {{CORPO}} - "; out != want {
		t.Errorf("substitución incorrecta:\n  obtido: %q\n  esperado: %q", out, want)
	}
	if len(avisos) != 1 || !strings.Contains(avisos[0], "NADA") {
		t.Errorf("esperaba un aviso por {{NADA}}, obtiven %v", avisos)
	}

	// En LaTeX, unha variable sen valor (definida ou non) ten que deixar un
	// \mbox{}: unha liña baleira antes dun \\ mata a compilación enteira.
	p.Variables = append(p.Variables, PlantillaVar{Nome: "PE", Valor: "   "})
	out, _ = substituirVariables("{{PE}}|{{NADA}}", p, auto, baleiroLatex, nil)
	if out != `\mbox{}|\mbox{}` {
		t.Errorf("unha variable baleira en LaTeX ten que dar \\mbox{}: %q", out)
	}

	// Un grupo LaTeX normal non é unha variable.
	out, _ = substituirVariables(`\frac{{a+b}}{2}`, p, auto, baleiroLatex, nil)
	if out != `\frac{{a+b}}{2}` {
		t.Errorf("non debería tocar código LaTeX normal: %q", out)
	}

	// Camiño LaTeX: o VALOR dunha variable con caracteres especiais escápase
	// (senón "unidade_2" -> "Missing $ inserted"). A maqueta non se toca.
	out, _ = substituirVariables(`\bfseries {{TITULO}} & {{DATA}}`, &PlantillaLatex{
		Variables: []PlantillaVar{{Nome: "TITULO", Valor: "unidade_2"}},
	}, map[string]string{"DATA": "01/09/2026"}, baleiroLatex, cas.EscapeLatexProse)
	if want := `\bfseries unidade\_2 & 01/09/2026`; out != want {
		t.Errorf("o valor da variable debía ir escapado:\n  obtido: %q\n  esperado: %q", out, want)
	}
}

func TestDocumentoLatexConPlantillaFragmento(t *testing.T) {
	p := &PlantillaLatex{
		Nome:      "Cabeceira",
		Latex:     "\\begin{center}{{CENTRO}}\\end{center}\n\n{{CORPO}}\n",
		Variables: []PlantillaVar{{Nome: "CENTRO", Valor: "IES Xistral"}},
	}
	doc, _, err := documentoLatexConPlantilla("CORPO XERADO", "pdflatex", p, "exame", nil)
	if err != nil {
		t.Fatalf("compoñer: %v", err)
	}
	if !strings.Contains(doc, `\documentclass`) || !strings.Contains(doc, `\begin{document}`) {
		t.Error("un fragmento ten que ir envolto no preámbulo por defecto")
	}
	if !strings.Contains(doc, "IES Xistral") || !strings.Contains(doc, "CORPO XERADO") {
		t.Error("faltan a variable ou o corpo no documento composto")
	}
	if !strings.Contains(doc, `\usepackage{gnuplot-lua-tikz}`) {
		t.Error("o preámbulo por defecto ten que seguir traendo os paquetes de Matexe")
	}
	if strings.Contains(doc, marcadorCorpo) {
		t.Error("quedou un {{CORPO}} sen substituír")
	}
}

func TestDocumentoLatexConPlantillaCompleta(t *testing.T) {
	p := &PlantillaLatex{
		Nome: "Orzamento",
		Latex: `\documentclass[11pt]{article}
\usepackage[margin=3cm]{geometry}
\usepackage{graphicx}
\begin{document}
{{CORPO}}
\end{document}
`,
	}
	doc, _, err := documentoLatexConPlantilla("CORPO", "pdflatex", p, "orzamento", nil)
	if err != nil {
		t.Fatalf("compoñer: %v", err)
	}
	if strings.Count(doc, `\documentclass`) != 1 {
		t.Error("unha plantilla completa manda: non se lle pode engadir outro \\documentclass")
	}
	if !strings.Contains(doc, `[margin=3cm]{geometry}`) {
		t.Error("perdéronse as marxes propias da plantilla")
	}
	if strings.Count(doc, `\usepackage{graphicx}`) != 1 {
		t.Error("graphicx xa estaba declarado: non se debe duplicar (Option clash)")
	}
	if !strings.Contains(doc, `\usepackage{gnuplot-lua-tikz}`) {
		t.Error("os paquetes que faltan (gnuplot-lua-tikz) teñen que inxectarse")
	}
	if !strings.Contains(doc, `\DeclareUnicodeCharacter`) {
		t.Error("con pdflatex tamén hai que inxectar as declaracións Unicode")
	}
	// Todo o inxectado ten que quedar ANTES do \begin{document}.
	if strings.Index(doc, `\usepackage{gnuplot-lua-tikz}`) > strings.Index(doc, `\begin{document}`) {
		t.Error("o preámbulo inxectado quedou despois de \\begin{document}")
	}
}

// Cun motor Unicode (xelatex/lualatex) NON se pode inxectar inputenc: é
// erro duro nese motor. Debe ir fontspec no seu lugar.
func TestInxeccionSegundoMotor(t *testing.T) {
	base := "\\documentclass{article}\n\\begin{document}\n{{CORPO}}\n\\end{document}\n"
	p := &PlantillaLatex{Nome: "x", Latex: base}

	pdf, _, _ := documentoLatexConPlantilla("c", "pdflatex", p, "", nil)
	if !strings.Contains(pdf, `\usepackage[utf8]{inputenc}`) {
		t.Error("con pdflatex fai falla inputenc")
	}
	xe, _, _ := documentoLatexConPlantilla("c", "xelatex", p, "", nil)
	if strings.Contains(xe, "inputenc") {
		t.Error("con xelatex NON se pode inxectar inputenc")
	}
	if !strings.Contains(xe, `\usepackage{fontspec}`) {
		t.Error("con xelatex fai falla fontspec")
	}
}

func TestDocumentoMarkdownConPlantilla(t *testing.T) {
	corpo := "Exercicio 1"
	if out, _ := documentoMarkdownConPlantilla(corpo, nil, ""); out != corpo {
		t.Errorf("sen plantilla o corpo non se toca: %q", out)
	}
	p := &PlantillaLatex{Latex: "{{CORPO}}"} // sen parte Markdown
	if out, _ := documentoMarkdownConPlantilla(corpo, p, ""); out != corpo {
		t.Errorf("sen parte Markdown o corpo non se toca: %q", out)
	}
	p.Markdown = "# {{TITULO}}\n\n{{CORPO}}\n"
	out, _ := documentoMarkdownConPlantilla(corpo, p, "Exame de setembro")
	if out != "# Exame de setembro\n\nExercicio 1\n" {
		t.Errorf("composición Markdown incorrecta: %q", out)
	}
}

func TestNormalizarNomeVar(t *testing.T) {
	casos := map[string]string{
		"centro educativo": "CENTRO_EDUCATIVO",
		"  Curso  ":        "CURSO",
		"n.º de orzamento": "N_DE_ORZAMENTO",
		"{{raro}}":         "RARO",
		"":                 "",
	}
	for entrada, esperado := range casos {
		if got := normalizarNomeVar(entrada); got != esperado {
			t.Errorf("normalizarNomeVar(%q) = %q, esperaba %q", entrada, got, esperado)
		}
	}
}

func TestParsearPlantillaIA(t *testing.T) {
	raw := "Aquí tes a plantilla:\n```json\n{\"nome\":\"Circular\",\"latex\":\"\\\\section{Ola}\\n{{CORPO}}\",\"markdown\":\"\",\"variables\":[{\"nome\":\"centro\",\"etiqueta\":\"Centro\",\"valor\":\"\"}]}\n```\nEspero que che sirva!"
	p, err := parsearPlantillaIA(raw)
	if err != nil {
		t.Fatalf("parsear: %v", err)
	}
	if p.Nome != "Circular" || !strings.Contains(p.Latex, marcadorCorpo) {
		t.Errorf("plantilla mal parseada: %+v", p)
	}
	if len(p.Variables) != 1 || p.Variables[0].Nome != "CENTRO" {
		t.Errorf("variables mal normalizadas: %+v", p.Variables)
	}

	// Sen {{CORPO}}: recupérase engadíndoo en vez de tirar a resposta.
	p, err = parsearPlantillaIA(`{"nome":"X","latex":"\\documentclass{article}\n\\begin{document}\nOla\n\\end{document}"}`)
	if err != nil {
		t.Fatalf("parsear sen marcador: %v", err)
	}
	if !strings.Contains(p.Latex, marcadorCorpo) {
		t.Errorf("debería engadirse {{CORPO}}: %q", p.Latex)
	}
	if strings.Index(p.Latex, marcadorCorpo) > strings.Index(p.Latex, `\end{document}`) {
		t.Error("{{CORPO}} ten que quedar DENTRO do documento")
	}

	if _, err := parsearPlantillaIA("non hai json ningún aquí"); err == nil {
		t.Error("esperaba erro cunha resposta sen JSON")
	}
}

func TestTextoVisibleHTML(t *testing.T) {
	html := `<html><head><style>p{color:red}</style><script>var x=1</script></head>
	<body><h1>Instituto</h1><p>Modelo de exame</p></body></html>`
	out := textoVisibleHTML(html)
	if !strings.Contains(out, "Instituto") || !strings.Contains(out, "Modelo de exame") {
		t.Errorf("perdeuse texto visible: %q", out)
	}
	if strings.Contains(out, "color:red") || strings.Contains(out, "var x") {
		t.Errorf("style/script non deberían aparecer: %q", out)
	}
}

// writeFileParaTest crea un ficheiro baleiro (as probas de PorFicheiro
// precisan que a ruta exista de verdade, ver podarPorFicheiro).
func writeFileParaTest(path string) error {
	return os.WriteFile(path, []byte("<EX></EX>\n"), 0o644)
}

// TestPrevisualizarPlantillasDeFabrica compila DE VERDADE cada plantilla de
// exemplo (pdflatex + o documento de demostración) - é a única forma de
// saber que o LaTeX que Yang sementa non peta na cara do profesorado a
// primeira vez que preme "Vista previa", e de paso cobre a inxección de
// paquetes na plantilla completa (Orzamento, que trae \documentclass
// propio).
func TestPrevisualizarPlantillasDeFabrica(t *testing.T) {
	if !commandExists("pdflatex") {
		t.Skip("sen pdflatex neste equipo")
	}
	configIllado(t)
	a := &App{}
	for _, p := range plantillasDeFabrica() {
		t.Run(p.Nome, func(t *testing.T) {
			res, err := a.PrevisualizarPlantilla(p)
			if err != nil {
				t.Fatalf("non compilou: %v\nlog:\n%s", err, res.Log)
			}
			if res.PDFBase64 == "" {
				t.Error("compilou sen devolver PDF")
			}
			if !strings.Contains(res.LatexSource, `\usepackage{gnuplot-lua-tikz}`) {
				t.Error("faltan os paquetes de Matexe no .tex final")
			}
			if strings.Contains(res.LatexSource, marcadorCorpo) {
				t.Error("quedou un {{CORPO}} sen substituír no .tex final")
			}
		})
	}
}

// TestGeneratePDFConPlantillaActiva é a proba de integración de todo o
// camiño: escoller unha plantilla e premer "Xerar" ten que dar un PDF co
// documento DENTRO da maqueta (Maxima + pdflatex reais, coma na app).
// Tamén comproba o que máis doado é romper ao tocar isto: que as etiquetas
// de Matexe (aquí un <EVAL>) sigan avaliándose igual dentro dunha plantilla.
func TestGeneratePDFConPlantillaActiva(t *testing.T) {
	configIllado(t)
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" {
		t.Skip("sen Maxima neste equipo")
	}
	if !commandExists("pdflatex") {
		t.Skip("sen pdflatex neste equipo")
	}

	p, err := a.GardarPlantilla(PlantillaLatex{
		Nome: "Cabeceira do centro",
		Latex: `\begin{center}\Large {{CENTRO}} \\ \normalsize {{MATERIA}} \end{center}
\hrule
{{CORPO}}
\vfill
\footnotesize Xerado o {{DATA}} para {{TITULO}}.
`,
		Markdown:  "# {{CENTRO}}\n\n{{CORPO}}\n",
		Variables: []PlantillaVar{{Nome: "CENTRO", Valor: "IES Xistral"}, {Nome: "MATERIA", Valor: "Matemáticas"}},
	})
	if err != nil {
		t.Fatalf("gardar plantilla: %v", err)
	}

	fonte := "<p>Suma: <EVAL>2+3</EVAL></p>"
	req := GeneratePDFRequest{Source: fonte, Seed: 1, Iterations: 1, NomeDoc: "exame-setembro"}

	// Sen plantilla activa: o documento sae coma sempre.
	sen, err := a.GeneratePDF(req)
	if err != nil {
		t.Fatalf("xerar sen plantilla: %v\nlog:\n%s", err, sen.Log)
	}
	if strings.Contains(sen.LatexSource, "IES Xistral") {
		t.Error("sen plantilla activa non debería aparecer a cabeceira")
	}

	if err := a.EscollerPlantilla(p.ID, ""); err != nil {
		t.Fatalf("escoller: %v", err)
	}
	con, err := a.GeneratePDF(req)
	if err != nil {
		t.Fatalf("xerar con plantilla: %v\nlog:\n%s", err, con.Log)
	}
	pdf, err := base64.StdEncoding.DecodeString(con.PDFBase64)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("non saíu un PDF válido (%v)", err)
	}
	for _, agardado := range []string{"IES Xistral", "Matemáticas", "exame-setembro"} {
		if !strings.Contains(con.LatexSource, agardado) {
			t.Errorf("falta %q no .tex final", agardado)
		}
	}
	if !strings.Contains(con.LatexSource, "$5$") {
		t.Errorf("o <EVAL> ten que seguir avaliándose dentro da plantilla; .tex:\n%s", con.LatexSource)
	}
	if strings.Contains(con.LatexSource, marcadorCorpo) {
		t.Error("quedou {{CORPO}} sen substituír")
	}
}

// TestMarkdownConPlantillaActiva cobre a outra metade: .md/.docx/.odt non
// pasan por LaTeX (generateMarkdownBody, markdowndoc.go), así que a
// plantilla ten que aplicárselles pola súa parte Markdown.
func TestMarkdownConPlantillaActiva(t *testing.T) {
	configIllado(t)
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" {
		t.Skip("sen Maxima neste equipo")
	}

	p, err := a.GardarPlantilla(PlantillaLatex{
		Nome:      "Boletín",
		Latex:     "{{CORPO}}",
		Markdown:  "# {{CENTRO}}\n\n{{CORPO}}\n\n---\n{{DATA}}\n",
		Variables: []PlantillaVar{{Nome: "CENTRO", Valor: "IES Xistral"}},
	})
	if err != nil {
		t.Fatalf("gardar: %v", err)
	}
	req := GenerateMarkdownRequest{Source: "<p>Suma: <EVAL>2+3</EVAL></p>", Seed: 1, Iterations: 1}

	corpo, _, err := a.generateMarkdownBody(req, t.TempDir())
	if err != nil {
		t.Fatalf("markdown sen plantilla: %v", err)
	}
	if strings.Contains(corpo, "IES Xistral") {
		t.Error("sen plantilla activa non debería aparecer a cabeceira")
	}

	if err := a.EscollerPlantilla(p.ID, ""); err != nil {
		t.Fatalf("escoller: %v", err)
	}
	con, _, err := a.generateMarkdownBody(req, t.TempDir())
	if err != nil {
		t.Fatalf("markdown con plantilla: %v", err)
	}
	if !strings.HasPrefix(con, "# IES Xistral") {
		t.Errorf("falta a cabeceira Markdown da plantilla:\n%s", con)
	}
	if !strings.Contains(con, "5") {
		t.Errorf("perdeuse o corpo xerado:\n%s", con)
	}
	if strings.Contains(con, marcadorCorpo) {
		t.Error("quedou {{CORPO}} sen substituír")
	}
}

// TestRepararBarrasLatex cobre o fallo real que rompeu unha plantilla
// xerada dende unha URL: o modelo sub-escapou os \\ do JSON e chegaron coma
// unha soa barra ("\[0.2em]"), que en LaTeX abre modo matemático.
func TestRepararBarrasLatex(t *testing.T) {
	casos := []struct{ entrada, esperado string }{
		{`{\Large {{EMPRESA}} }\[0.2em]`, `{\Large {{EMPRESA}} }\\[0.2em]`},
		{`\footnotesize\bfseries FACTURA\[-0.2ex]`, `\footnotesize\bfseries FACTURA\\[-0.2ex]`},
		{"unha liña\\\noutra liña", "unha liña\\\\\noutra liña"},
		// Xa correcto: non se toca.
		{`fila un\\[4pt]`, `fila un\\[4pt]`},
		{"fila un\\\\\nfila dous", "fila un\\\\\nfila dous"},
		// Display math de verdade (pecha con \]) queda intacto.
		{`\[ x^2 + 1 \]`, `\[ x^2 + 1 \]`},
		// Comandos normais non se tocan.
		{`\textbf{ola} \begin{center}`, `\textbf{ola} \begin{center}`},
	}
	for _, c := range casos {
		if got := repararBarrasLatex(c.entrada); got != c.esperado {
			t.Errorf("repararBarrasLatex(%q)\n  = %q\n  esperaba %q", c.entrada, got, c.esperado)
		}
	}
}

// TestMotorDeducidoDaIA: unha plantilla con fontspec só compila con
// xelatex/lualatex. Se a IA non o marca no campo "motor", dedúcese do
// código - senón, quen teña pdflatex en Opcións vería fallar a plantilla
// cun erro que non menciona a causa.
func TestMotorDeducidoDaIA(t *testing.T) {
	con, err := parsearPlantillaIA(`{"nome":"X","latex":"\\documentclass{article}\n\\usepackage{fontspec}\n\\begin{document}\n{{CORPO}}\n\\end{document}"}`)
	if err != nil {
		t.Fatalf("parsear: %v", err)
	}
	if con.Motor != "xelatex" {
		t.Errorf("con fontspec o motor debería ser xelatex, foi %q", con.Motor)
	}

	sen, err := parsearPlantillaIA(`{"nome":"X","latex":"\\section{Ola}\n{{CORPO}}"}`)
	if err != nil {
		t.Fatalf("parsear: %v", err)
	}
	if sen.Motor != "" {
		t.Errorf("sen fontspec non se debe forzar motor, foi %q", sen.Motor)
	}

	// Un motor que xa veña da IA mándase el.
	explicito, err := parsearPlantillaIA(`{"nome":"X","motor":"lualatex","latex":"\\usepackage{fontspec}\n{{CORPO}}"}`)
	if err != nil {
		t.Fatalf("parsear: %v", err)
	}
	if explicito.Motor != "lualatex" {
		t.Errorf("o motor explícito da IA non se pode pisar, foi %q", explicito.Motor)
	}
}

// TestPlantillasDeFabricaNovasChegorAQuenXaTiñaYang é a proba de que se
// poden REPARTIR plantillas: unha que se engada a plantillas_fabrica.go
// nunha versión nova ten que aparecerlle tamén a quen xa tiña Yang
// instalado (biblioteca xa sementada), sen pisar o que teña editado nin
// resucitar o que borrase.
func TestPlantillasDeFabricaNovasChegorAQuenXaTiñaYang(t *testing.T) {
	configIllado(t)
	a := &App{}

	// Estado "instalación vella": xa sementada, pero só cunha das de
	// fábrica, e ademais EDITADA polo profesorado.
	fabrica := plantillasDeFabrica()
	editada := fabrica[0]
	editada.Latex = "O MEU DESEÑO PROPIO\n{{CORPO}}\n"
	editada.Nome = "O meu exame"
	if err := saveAlmacenPlantillas(almacenPlantillas{
		Plantillas: []PlantillaLatex{editada},
		Sementado:  true,
	}); err != nil {
		t.Fatal(err)
	}

	estado, err := a.ListarPlantillas()
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	// As tres de fábrica MÁIS a copia propia que saca repararFabricaEditadas
	// da que estaba editada en sitio. Antes eran só tres, porque o traballo
	// do profesorado ocupaba unha das IDs de fábrica - que é precisamente o
	// que xa non pasa (ver repararFabricaEditadas, plantillas.go).
	if len(estado.Plantillas) != len(fabrica)+1 {
		t.Fatalf("esperaba %d plantillas (as %d de fábrica + a copia do traballo editado), atopei %d",
			len(fabrica)+1, len(fabrica), len(estado.Plantillas))
	}
	// A garantía de sempre segue en pé: o traballo do profesorado non se
	// perde nunha actualización. O que cambia é onde vive - nunha plantilla
	// propia, non pisando a de fábrica.
	traballo := false
	for _, p := range estado.Plantillas {
		if p.Nome == "O meu exame" && strings.Contains(p.Latex, "O MEU DESEÑO PROPIO") {
			traballo = true
			if p.DeFabrica {
				t.Error("o traballo do profesorado non pode quedar marcado coma de fábrica")
			}
			if p.ID == editada.ID {
				t.Error("tiña que mudar de ID para deixar libre a de fábrica")
			}
		}
	}
	if !traballo {
		t.Error("unha actualización non pode facer desaparecer unha plantilla xa editada")
	}

	// Borrar unha de fábrica ten que ser definitivo.
	borrada := fabrica[1].ID
	if err := a.EliminarPlantilla(borrada); err != nil {
		t.Fatalf("eliminar: %v", err)
	}
	estado, err = a.ListarPlantillas() // "reinicio": volve pasar por sementarSeFai
	if err != nil {
		t.Fatalf("listar tras eliminar: %v", err)
	}
	for _, p := range estado.Plantillas {
		if p.ID == borrada {
			t.Errorf("unha plantilla de fábrica borrada non pode volver soa: %q", p.Nome)
		}
	}
	// -1 pola borrada, +1 pola copia propia da reparación (que segue aí:
	// borrar unha de fábrica non pode levar por diante o traballo propio).
	if len(estado.Plantillas) != len(fabrica) {
		t.Errorf("esperaba %d tras eliminar, atopei %d", len(fabrica), len(estado.Plantillas))
	}
}

// TestEditarDeFabricaBifurca cobre o arranxo de bifurcarDeFabrica: gardar
// por riba dunha plantilla de fábrica NON a pisa. Antes si, e iso deixaba a
// ID de fábrica ocupada para sempre (sementarSeFai só repón as que faltan),
// así que esa plantilla xa nunca recibía as melloras dunha versión nova de
// Yang - e ademais gardaba datos privados baixo unha entrada marcada coma
// "de exemplo".
func TestEditarDeFabricaBifurca(t *testing.T) {
	configIllado(t)
	a := &App{}

	estado, err := a.ListarPlantillas() // sementa as de fábrica
	if err != nil {
		t.Fatal(err)
	}
	orixinal := estado.Plantillas[0]
	if !orixinal.DeFabrica {
		t.Fatalf("esperaba unha de fábrica, dáme %+v", orixinal.Nome)
	}
	latexFabrica := orixinal.Latex

	// O documento e a escolla apuntan á de fábrica, coma quen leva tempo
	// traballando con ela.
	doc := filepath.Join(t.TempDir(), "meu.matex")
	if err := os.WriteFile(doc, []byte("ola"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.EscollerPlantilla(orixinal.ID, doc); err != nil {
		t.Fatal(err)
	}

	// Editar e gardar, coma quen lle mete os datos do seu centro.
	editada := orixinal
	editada.Latex = "DATOS PRIVADOS DO MEU CENTRO\n{{CORPO}}\n"
	gardada, err := a.GardarPlantilla(editada)
	if err != nil {
		t.Fatalf("gardar: %v", err)
	}

	if gardada.ID == orixinal.ID {
		t.Error("gardar sobre unha de fábrica ten que crear unha copia, non pisala")
	}
	if gardada.DeFabrica {
		t.Error("a copia do profesorado non pode quedar marcada coma de fábrica")
	}

	estado, err = a.ListarPlantillas()
	if err != nil {
		t.Fatal(err)
	}
	var fabricaAgora, miña *PlantillaLatex
	for i := range estado.Plantillas {
		switch estado.Plantillas[i].ID {
		case orixinal.ID:
			fabricaAgora = &estado.Plantillas[i]
		case gardada.ID:
			miña = &estado.Plantillas[i]
		}
	}
	if fabricaAgora == nil {
		t.Fatal("a de fábrica ten que sobrevivir á edición")
	}
	if fabricaAgora.Latex != latexFabrica {
		t.Error("a de fábrica quedou modificada; tiña que seguir intacta")
	}
	if strings.Contains(fabricaAgora.Latex, "DATOS PRIVADOS") {
		t.Error("datos privados filtrados na plantilla marcada coma de exemplo")
	}
	if miña == nil || !strings.Contains(miña.Latex, "DATOS PRIVADOS DO MEU CENTRO") {
		t.Fatal("a copia ten que levar o que se escribiu")
	}

	// A bifurcación herda a escolla: o traballo segue a aplicarse ao mesmo
	// documento, que é o que espera quen acaba de premer "Gardar".
	if estado.Activa != gardada.ID {
		t.Errorf("a activa tiña que pasar á copia, quedou en %q", estado.Activa)
	}
	id, err := a.PlantillaParaFicheiro(doc)
	if err != nil {
		t.Fatal(err)
	}
	if id != gardada.ID {
		t.Errorf("o documento tiña que quedar apuntando á copia, apunta a %q", id)
	}

	// Editar OUTRA VEZ a copia xa non bifurca: só a primeira vez.
	miña.Latex = "SEGUNDA VOLTA\n{{CORPO}}\n"
	outra, err := a.GardarPlantilla(*miña)
	if err != nil {
		t.Fatal(err)
	}
	if outra.ID != gardada.ID {
		t.Error("editar unha plantilla propia non pode seguir creando copias")
	}
}

// TestRepararFabricaEditadaAnterior cobre a reparación retroactiva: un
// plantillas.json que xa vén estropeado dunha versión anterior de Yang
// (unha de fábrica pisada en sitio) arránxase só ao cargalo, sen perder
// nada e sen pedirlle nada ao profesorado.
func TestRepararFabricaEditadaAnterior(t *testing.T) {
	configIllado(t)
	a := &App{}

	fabrica := plantillasDeFabrica()
	orixinal := fabrica[0]

	// Estado herdado: a de fábrica pisada, aínda marcada DeFabrica, e todo
	// (activa + documento) apuntando a ela.
	estropeada := orixinal
	estropeada.Nome = "Exame do meu centro"
	estropeada.Latex = "CENTRO PRIVADO\n{{CORPO}}\n"
	doc := "/tmp/vello.matex"
	if err := saveAlmacenPlantillas(almacenPlantillas{
		Plantillas:  []PlantillaLatex{estropeada, fabrica[1], fabrica[2]},
		Activa:      orixinal.ID,
		PorFicheiro: map[string]string{doc: orixinal.ID},
		Sementado:   true,
	}); err != nil {
		t.Fatal(err)
	}

	estado, err := a.ListarPlantillas() // pasa por repararFabricaEditadas
	if err != nil {
		t.Fatal(err)
	}

	var deFabrica, propia *PlantillaLatex
	for i := range estado.Plantillas {
		switch {
		case estado.Plantillas[i].ID == orixinal.ID:
			deFabrica = &estado.Plantillas[i]
		case estado.Plantillas[i].Nome == "Exame do meu centro":
			propia = &estado.Plantillas[i]
		}
	}
	if deFabrica == nil {
		t.Fatal("a de fábrica ten que seguir existindo coa súa ID")
	}
	if deFabrica.Latex != orixinal.Latex || deFabrica.Nome != orixinal.Nome {
		t.Errorf("a de fábrica tiña que volver ao orixinal, quedou en %q", deFabrica.Nome)
	}
	if strings.Contains(deFabrica.Latex, "CENTRO PRIVADO") {
		t.Error("datos privados aínda baixo a entrada marcada de fábrica")
	}
	if propia == nil {
		t.Fatal("o traballo editado tiña que sobrevivir nunha plantilla propia")
	}
	if propia.DeFabrica || !strings.Contains(propia.Latex, "CENTRO PRIVADO") {
		t.Errorf("a copia propia non conserva o traballo: %+v", propia.Nome)
	}
	if estado.Activa != propia.ID {
		t.Error("a activa tiña que pasar á copia propia")
	}
	id, err := a.PlantillaParaFicheiro(doc)
	if err != nil {
		t.Fatal(err)
	}
	if id != propia.ID {
		t.Error("o documento tiña que quedar coa copia propia")
	}

	// Idempotente: cargar outra vez non pode seguir creando copias.
	antes := len(estado.Plantillas)
	estado, err = a.ListarPlantillas()
	if err != nil {
		t.Fatal(err)
	}
	if len(estado.Plantillas) != antes {
		t.Errorf("a reparación repetiuse: %d -> %d plantillas", antes, len(estado.Plantillas))
	}
}
