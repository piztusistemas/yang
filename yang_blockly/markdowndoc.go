package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"matexe-wails/cas"

	"golang.org/x/net/html"

)

var markdownEscapeRe = regexp.MustCompile("[\\\\`*_\\[\\]]")

// escapeMarkdownText backslash-escapes the handful of Markdown special
// characters that show up in ordinary exam text (*_`[]) - mirrors
// escapeOutsideMath (app.go) for the LaTeX export. Text already protected
// via rawRegistry.Protect (the $...$/![]() blocks ToTeX/plot/tikz already
// rendered) is a plain alphanumeric token at this point, so this is safe to
// apply blindly.
func escapeMarkdownText(s string) string {
	return markdownEscapeRe.ReplaceAllStringFunc(s, func(m string) string { return "\\" + m })
}

// preprocessPseudoTagsMarkdown mirrors preprocessPseudoTags (app.go) for the
// Markdown export: <TAB>/<BP> never have a closing tag, so they must be
// substituted (and protected from escapeMarkdownText) before the text
// reaches the HTML parser. No LaTeX equivalent makes sense here - TAB
// becomes plain spaces, BP becomes a horizontal rule as the closest thing
// Markdown has to a forced page break.
func preprocessPseudoTagsMarkdown(s string, reg *rawRegistry) string {
	s = tabRe.ReplaceAllString(s, reg.Protect("    "))
	s = bpRe.ReplaceAllString(s, reg.Protect("\n\n---\n\n"))
	return s
}

// markdownConverter walks a parsed HTML fragment (same parseFragment used by
// latexConverter, latexdoc.go) and renders it as Markdown source.
// Deliberately not merged with latexConverter into one parametrised type
// despite the structural overlap: escaping rules, image syntax and the lack
// of a two-column layout diverge enough that two small parallel converters
// stay easier to follow than one with format branches sprinkled through it.
type markdownConverter struct {
	baseDir     string // directory of the open .matex file, for <IMG> lookups
	buildDir    string // directory the .md is being written to (images/ subfolder goes here)
	unknownTags map[string]bool
	warnings    []string
}

func newMarkdownConverter(baseDir, buildDir string) *markdownConverter {
	return &markdownConverter{baseDir: baseDir, buildDir: buildDir, unknownTags: map[string]bool{}}
}

func (c *markdownConverter) render(n *html.Node) string {
	switch n.Type {
	case html.TextNode:
		return escapeMarkdownText(n.Data)
	case html.DocumentNode:
		return c.renderChildren(n)
	case html.ElementNode:
		return c.renderElement(n)
	default:
		return ""
	}
}

func (c *markdownConverter) renderChildren(n *html.Node) string {
	var b strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		b.WriteString(c.render(ch))
	}
	return b.String()
}

var markdownHeadingCmd = map[string]string{"h1": "#", "h2": "##", "h3": "###", "h4": "####"}

func (c *markdownConverter) renderElement(n *html.Node) string {
	tag := strings.ToLower(n.Data)
	switch tag {
	case "head", "style", "script", "link", "meta", "title":
		return "" // no Markdown meaning, drop entirely (incl. children)
	case "html", "body", "span", "font", "ex", "resp", "sol":
		// "ex" is the block editor's structural exercise marker
		// (blocks-serialize.js); "resp"/"sol" are the table editor's answer/
		// solution markers (table-serialize.js). None carry CAS/Markdown
		// meaning - by the time the document reaches here cas.ParseText has
		// already dropped or unwrapped them per RenderMode, so this is just
		// a safety net against a stray one from a hand-edited .matex.
		return c.renderChildren(n)
	case "a":
		return c.renderLink(n)
	case "div":
		// Markdown has no side-by-side layout equivalent to latexConverter's
		// izq/der minipages - flatten in document order, known limitation.
		return c.renderChildren(n) + "\n"
	case "h1", "h2", "h3", "h4":
		return "\n" + markdownHeadingCmd[tag] + " " + strings.TrimSpace(c.renderChildren(n)) + "\n\n"
	case "b", "strong":
		return "**" + c.renderChildren(n) + "**"
	case "i", "em":
		return "*" + c.renderChildren(n) + "*"
	case "u":
		// Standard Markdown has no underline - raw inline HTML is valid in
		// virtually every renderer that matters here (same call as "center").
		return "<u>" + c.renderChildren(n) + "</u>"
	case "small", "big":
		// Markdown non ten tamaño de letra e o HTML cru non sobrevive ao
		// paso por Pandoc a .docx/.odt - consérvase o TEXTO, que é o que
		// importa, sen marcar a etiqueta coma "sen soporte" (o aviso xa se
		// dá no PDF se fai falla, aquí non hai nada mellor que facer).
		return c.renderChildren(n)
	case "code", "tt", "kbd", "samp":
		return "`" + c.renderChildren(n) + "`"
	case "blockquote":
		// Prefixo "> " en cada liña, que é o que Markdown entende por cita.
		var b strings.Builder
		for _, liña := range strings.Split(strings.TrimSpace(c.renderChildren(n)), "\n") {
			b.WriteString("> " + liña + "\n")
		}
		return "\n" + b.String() + "\n"
	case "br":
		return "  \n" // trailing double-space = forced line break in Markdown
	case "hr":
		// "---" nunha liña propia é a regra horizontal estándar en Markdown -
		// mesmo marcador que xa usa preprocessPseudoTagsMarkdown para <BP>
		// (separador entre variantes), por consistencia.
		return "\n\n---\n\n"
	case "p":
		return c.renderChildren(n) + "\n\n"
	case "center":
		return "\n<div align=\"center\">\n\n" + c.renderChildren(n) + "\n\n</div>\n"
	case "ul", "ol":
		var b strings.Builder
		i := 0
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type == html.ElementNode && strings.ToLower(ch.Data) == "li" {
				i++
				marker := "-"
				if tag == "ol" {
					marker = fmt.Sprintf("%d.", i)
				}
				b.WriteString(marker + " " + strings.TrimSpace(c.renderChildren(ch)) + "\n")
			}
		}
		b.WriteString("\n")
		return b.String()
	case "table":
		return c.renderTable(n)
	case "img":
		return c.renderImg(n)
	default:
		c.unknownTags[tag] = true
		return c.renderChildren(n)
	}
}

func (c *markdownConverter) renderTable(n *html.Node) string {
	var rows [][]string
	var walkRows func(*html.Node)
	walkRows = func(node *html.Node) {
		for ch := node.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type != html.ElementNode {
				continue
			}
			switch strings.ToLower(ch.Data) {
			case "tr":
				var cells []string
				for cell := ch.FirstChild; cell != nil; cell = cell.NextSibling {
					if cell.Type == html.ElementNode {
						name := strings.ToLower(cell.Data)
						if name == "td" || name == "th" {
							// Pipe tables are one line per row - collapse any
							// line breaks a cell's own content introduced.
							cells = append(cells, strings.TrimSpace(strings.ReplaceAll(c.renderChildren(cell), "\n", " ")))
						}
					}
				}
				if len(cells) > 0 {
					rows = append(rows, cells)
				}
			case "thead", "tbody", "tfoot":
				walkRows(ch)
			}
		}
	}
	walkRows(n)
	if len(rows) == 0 {
		return ""
	}
	cols := len(rows[0])
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}

	var b strings.Builder
	b.WriteString("\n")
	for i, row := range rows {
		for len(row) < cols {
			row = append(row, "")
		}
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
		if i == 0 {
			sep := make([]string, cols)
			for j := range sep {
				sep[j] = "---"
			}
			b.WriteString("| " + strings.Join(sep, " | ") + " |\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

// renderImg mirrors latexConverter.renderImg (latexdoc.go) for uploaded
// <IMG src="..."> references, but copies into buildDir/images (not
// buildDir directly) to land next to what plot()/tikz() already write there
// in FormatMarkdown, and emits Markdown's ![]() instead of \includegraphics.
func (c *markdownConverter) renderImg(n *html.Node) string {
	src := attr(n, "src")
	if src == "" || strings.HasPrefix(src, "data:") {
		return ""
	}
	if c.baseDir == "" {
		c.warnings = append(c.warnings, fmt.Sprintf("imaxe %q omitida (abre o ficheiro dende disco para incluí-la)", src))
		return ""
	}
	data, err := os.ReadFile(filepath.Join(c.baseDir, src))
	if err != nil {
		c.warnings = append(c.warnings, fmt.Sprintf("non se atopou a imaxe %q", src))
		return ""
	}
	imgDir := filepath.Join(c.buildDir, "images")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		c.warnings = append(c.warnings, fmt.Sprintf("non se puido crear o cartafol de imaxes: %v", err))
		return ""
	}
	dest := filepath.Join(imgDir, filepath.Base(src))
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		c.warnings = append(c.warnings, fmt.Sprintf("non se puido copiar a imaxe %q: %v", src, err))
		return ""
	}
	// Markdown's ![]() non admite width/height: se se deron (ver
	// imgIncludeOptions en latexdoc.go, mesmos atributos), emítese o <img>
	// HTML cru en vez de ![]() - practicamente todos os renderizadores de
	// Markdown (incluído GitHub) admiten HTML incrustado, e así non se perde
	// o tamaño escollido no editor.
	width, height := attr(n, "width"), attr(n, "height")
	if width == "" && height == "" {
		return fmt.Sprintf("\n![](images/%s)\n", filepath.Base(src))
	}
	var sizeAttrs strings.Builder
	if width != "" {
		fmt.Fprintf(&sizeAttrs, ` width="%s"`, width)
	}
	if height != "" {
		fmt.Fprintf(&sizeAttrs, ` height="%s"`, height)
	}
	return fmt.Sprintf("\n<img src=\"images/%s\"%s>\n", filepath.Base(src), sizeAttrs.String())
}

// renderLink handles <A href="..."> the same way renderImg above handles
// <IMG src>: an external URL (http(s)://, mailto:) becomes Markdown's
// [text](url) unchanged; a local file next to the .matex source (e.g. a
// PDF the explorer's "⬆️ Subir ficheiro" button uploaded, see explorer.go's
// SubirFicheiroCartafol) is copied into buildDir/attachments and linked
// with a path relative to the exported file. For a plain .md export
// (ExportMarkdown, buildDir = the REAL folder next to the saved .md) that
// attachment genuinely travels alongside and keeps working; for .docx/.odt
// (exportViaPandoc, buildDir = a throwaway tmpDir Pandoc packages into ONE
// file and then discards) the link only survives as long as the exported
// document stays next to that file - same known limitation as PDF export's
// SavePDFDialog (see renderLink, latexdoc.go). isExternalURL is defined
// there too, shared (same package).
func (c *markdownConverter) renderLink(n *html.Node) string {
	href := attr(n, "href")
	text := c.renderChildren(n)
	if href == "" {
		return text
	}
	if text == "" {
		text = escapeMarkdownText(href)
	}
	if isExternalURL(href) {
		return fmt.Sprintf("[%s](%s)", text, href)
	}
	if c.baseDir == "" {
		c.warnings = append(c.warnings, fmt.Sprintf("ligazón %q omitida (abre o ficheiro dende disco para incluí-la)", href))
		return text
	}
	data, err := os.ReadFile(filepath.Join(c.baseDir, href))
	if err != nil {
		c.warnings = append(c.warnings, fmt.Sprintf("non se atopou o ficheiro ligado %q", href))
		return text
	}
	attDir := filepath.Join(c.buildDir, "attachments")
	if err := os.MkdirAll(attDir, 0o755); err != nil {
		c.warnings = append(c.warnings, fmt.Sprintf("non se puido crear o cartafol de anexos: %v", err))
		return text
	}
	dest := filepath.Join(attDir, filepath.Base(href))
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		c.warnings = append(c.warnings, fmt.Sprintf("non se puido copiar o ficheiro ligado %q: %v", href, err))
		return text
	}
	return fmt.Sprintf("[%s](attachments/%s)", text, filepath.Base(href))
}

// GenerateMarkdownRequest mirrors GeneratePDFRequest (latexdoc.go).
type GenerateMarkdownRequest struct {
	Source     string `json:"source"`
	CodeIni    string `json:"codeIni"`
	Seed       int    `json:"seed"`
	Iterations int    `json:"iterations"`
	BaseDir    string `json:"baseDir"`
	// NomeDoc: igual ca en GeneratePDFRequest (latexdoc.go) - só para as
	// variables automáticas da plantilla activa.
	NomeDoc string `json:"nomeDoc"`
	// Modo: igual ca en GenerateRequest (app.go) - ""/"enunciados",
	// "resposta" ou "solucions". Ver cas.Maxima.RenderMode.
	Modo string `json:"modo"`
}

type GenerateMarkdownResult struct {
	Path     string   `json:"path"`
	Warnings []string `json:"warnings"`
}

// generateMarkdownBody runs the same per-variant Maxima loop as GeneratePDF
// (latexdoc.go) but in cas.FormatMarkdown mode, and returns the resulting
// Markdown body as a string - the shared core behind ExportMarkdown (writes
// it straight to a .md file) and ExportDocx/ExportOdt (docdoc.go: feeds it
// to pandoc instead). outDir is where plot()/tikz() (cas/tags.go) write
// real image files instead of embedding base64 (m.Format == FormatMarkdown)
// - an "images" subdirectory must already exist under it.
func (a *App) generateMarkdownBody(req GenerateMarkdownRequest, outDir string) (body string, warnings []string, err error) {
	if strings.TrimSpace(req.Source) == "" {
		return "", nil, fmt.Errorf("o documento está baleiro")
	}
	if req.Iterations < 1 {
		req.Iterations = 1
	}
	if strings.TrimSpace(a.maximaPath) == "" {
		return "", nil, fmt.Errorf("non se atopou Maxima. Configura a ruta en Opcións")
	}

	codeIni := req.CodeIni
	if strings.TrimSpace(codeIni) == "" {
		codeIni = a.settings.CodeIni
	}
	if strings.TrimSpace(codeIni) == "" {
		codeIni = defaultCodeIni
	}
	codeIniLines := splitLines(codeIni)

	if err := os.MkdirAll(filepath.Join(outDir, "images"), 0o755); err != nil {
		return "", nil, err
	}

	sess, err := cas.Open(a.maximaPath)
	if err != nil {
		return "", nil, fmt.Errorf("non se puido iniciar Maxima (%s): %w", a.maximaPath, err)
	}
	defer a.trackMaximaSession(sess)()
	m := cas.NewMaxima(sess)
	defer m.Close()
	m.SetOutDir(outDir)
	m.Format = cas.FormatMarkdown
	m.SetTimeout(a.settings.TimeoutResolto())
	m.ForzarDecimal = a.settings.Decimal
	m.RenderMode = req.Modo

	reg := newRawRegistry()
	m.ProtectRaw = reg.Protect

	if _, err := m.Send("linel:1024;display2d:false;simp:true;", false); err != nil {
		return "", nil, err
	}

	conv := newMarkdownConverter(req.BaseDir, outDir)

	var out strings.Builder
	for i := 0; i < req.Iterations; i++ {
		variantSeed := req.Seed + i

		if _, err := m.Send("reset();kill(all)", true); err != nil {
			return "", nil, err
		}
		// SetDecimais TEN que ir despois de kill(all), non antes do bucle:
		// kill(all) borra TODA definición de usuario, incluída a función
		// matexe_arredondar que SetDecimais acaba de definir.
		if err := m.SetDecimais(a.settings.DecimaisResolto()); err != nil {
			return "", nil, err
		}
		for _, line := range codeIniLines {
			if _, err := m.Send(line, false); err != nil {
				warnings = append(warnings,
					fmt.Sprintf("variante %d, codeini %q: %v", i+1, line, err))
			}
		}
		if err := m.SetSeed(variantSeed); err != nil {
			return "", nil, err
		}

		rawMd, err := m.ParseText(preprocessPseudoTagsMarkdown(req.Source, reg))
		if err != nil {
			return "", nil, fmt.Errorf("variante %d (semente %d): %w", i+1, variantSeed, err)
		}

		nodes, err := parseFragment(rawMd)
		if err != nil {
			return "", nil, fmt.Errorf("variante %d: erro analizando HTML: %w", i+1, err)
		}
		if i > 0 {
			out.WriteString("\n\n---\n\n")
		}
		for _, n := range nodes {
			out.WriteString(conv.render(n))
		}
	}

	warnings = append(warnings, conv.warnings...)
	for t := range conv.unknownTags {
		warnings = append(warnings, "etiqueta HTML sen soporte en Markdown (mantívose o contido): <"+t+">")
	}

	corpo := reg.Resolve(strings.TrimSpace(out.String())) + "\n"

	// A plantilla activa (se ten parte Markdown) envolve o corpo coa mesma
	// cabeceira/pé que xa aplica ao PDF - ver plantillas.go. Afecta por
	// igual a ExportMarkdown e a ExportDocx/ExportOdt, que pasan por aquí.
	plantilla := a.plantillaActiva()
	if err := copiarImaxesPlantillaMarkdown(plantilla, outDir); err != nil {
		warnings = append(warnings, fmt.Sprintf("non se puideron copiar as imaxes da plantilla: %v", err))
	}
	doc, avisosPlantilla := documentoMarkdownConPlantilla(corpo, plantilla, req.NomeDoc)
	warnings = append(warnings, avisosPlantilla...)
	return doc, warnings, nil
}

// ExportMarkdown shows a native "save as" dialog for a .md file, then runs
// generateMarkdownBody and writes the result straight to the chosen path
// plus an "images/" subdirectory next to it for any <PLOT>/<TIKZ>/uploaded
// <IMG> referenced. Unlike GeneratePDF there's no compile step, so no temp
// dir is needed: the Maxima session's outDir IS the export destination.
func (a *App) ExportMarkdown(req GenerateMarkdownRequest, defaultName string) (GenerateMarkdownResult, error) {
	result := GenerateMarkdownResult{}

	dir, base := splitDialogDefault(defaultName)
	path, err := saveFileDialogCompat("Exportar Markdown", dir, base, "Markdown (*.md)", "*.md")
	if err != nil || path == "" {
		return result, err
	}

	body, warnings, err := a.generateMarkdownBody(req, filepath.Dir(path))
	result.Warnings = warnings
	if err != nil {
		return result, err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return result, err
	}
	result.Path = path
	return result, nil
}
