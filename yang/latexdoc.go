package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"matexe-wails/cas"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// rawRegistry lets any stage of the pipeline (Maxima's math/plot output,
// the <TAB>/<BP> pseudo-tag substitution) mark a chunk of text as "this is
// already final LaTeX, don't escape it" without the escaper needing to
// recognise every possible shape of protected content. Protect() swaps the
// chunk for an opaque alphanumeric token - safe to pass through both the
// HTML parser (ordinary text, no special meaning) and EscapeLatex (nothing
// in it needs escaping) - and Resolve() swaps the tokens back for the real
// LaTeX once the whole document has been rendered.
type rawRegistry struct {
	blocks map[string]string
	n      int
}

func newRawRegistry() *rawRegistry {
	return &rawRegistry{blocks: map[string]string{}}
}

func (r *rawRegistry) Protect(s string) string {
	r.n++
	token := fmt.Sprintf("MATEXERAWBLOCK%dZZ", r.n)
	r.blocks[token] = s
	return token
}

func (r *rawRegistry) Resolve(s string) string {
	for token, real := range r.blocks {
		s = strings.ReplaceAll(s, token, real)
	}
	return s
}

// preprocessPseudoTags handles Matexe's non-standard, tag-less markers.
// <TAB> and <BP> never have a closing tag (Main.pas substituted them with
// plain stringReplace, not real tag parsing), so they'd confuse a real
// HTML parser if left in place - replace them with their LaTeX equivalent
// via the same protected-token mechanism the cas package uses for math and
// plots, so escapeOutsideMath doesn't mangle them either.
var (
	tabRe = regexp.MustCompile(`(?i)<TAB>`)
	bpRe  = regexp.MustCompile(`(?i)<BP>`)
)

func preprocessPseudoTags(s string, reg *rawRegistry) string {
	s = tabRe.ReplaceAllString(s, reg.Protect(`\quad{}`))
	s = bpRe.ReplaceAllString(s, reg.Protect(`\clearpage{}`))
	return s
}

// escapeOutsideMath LaTeX-escapes text nodes encountered while walking the
// parsed fragment. Anything already protected via rawRegistry.Protect is a
// plain alphanumeric token at this point, so a blind escape is safe -
// there's nothing left in the string that still needs shielding.
func escapeOutsideMath(s string) string {
	return cas.EscapeLatexProse(s)
}

// ambienteMalRe recoñece un \begin ou \end que NON vai seguido (opcionalmente
// tras espazos ou saltos de liña) dun "{". \b evita tocar \begingroup /
// \endinput / \endcsname e demais. O grupo {..\{}? é OPCIONAL: se casa,
// remata en "{" (ambiente ben formado, déixase intacto); se non, o \begin /
// \end vai solto.
var ambienteMalRe = regexp.MustCompile(`\\(begin|end)\b([ \t\r\n]*\{)?`)

// sanearAmbientesLatex neutraliza os \begin / \end que quedan sen unha
// contorna "{nome}" válida. É SEMPRE contido malformado da IA: confunde a
// sintaxe LaTeX \begin{env} coas etiquetas <TAG> de Matexe e escribe cousas
// como "\begin<SISTEMA>" ou "\begin{cases}" solto nun texto. LaTeX aborta con
// «! LaTeX Error: Environment <undefined» e non sae ningún PDF (report
// habitual ao xerar exames coa IA). Aquí déixase o \begin/\end coma texto
// literal para que o resto do documento compile, e devólvese un aviso para
// que o profesorado saiba que ese exercicio hai que revisalo/rexeneralo.
// Aplícase ao corpo XA resolto (reg.Resolve), así que os \begin{...} propios
// de Yang (center, tabular, tikzpicture, cases de <SISTEMA>...) van sempre
// con "{" e non se tocan.
func sanearAmbientesLatex(body string) (string, []string) {
	n := 0
	out := ambienteMalRe.ReplaceAllStringFunc(body, func(m string) string {
		if strings.HasSuffix(m, "{") {
			return m // \begin{ / \end{ - ben formado
		}
		n++
		return `\textbackslash{}` + strings.TrimPrefix(m, `\`)
	})
	if n == 0 {
		return body, nil
	}
	return out, []string{fmt.Sprintf(
		"atopáronse %d usos de \\begin/\\end sen ambiente válido (adoita ser contido da IA que mestura \\begin{...} coas etiquetas <...>): deixáronse coma texto para que o PDF compile. Revisa ese exercicio ou rexenérao.", n)}
}

// condicionalLatexRe recoñece as primitivas de fluxo condicional de TeX
// (\iffalse, \iftrue, \ifnum, \ifx, \ifdim..., \else, \fi) que poidan
// quedar SOLTAS no corpo: contido da IA, texto copiado a man ou un <TEX> a
// medias. \newif non casa (empeza por \new).
var condicionalLatexRe = regexp.MustCompile(`\\(if[a-zA-Z@]*|else|fi)\b`)

// sanearCondicionaisLatex neutraliza \if.../\else/\fi soltos SÓ cando están
// DESEQUILIBRADOS (tantos "\if..." coma "\fi" -> déixase, pode ser un <TEX>
// lexítimo). Un só "\iffalse" ou "\fi" de máis fai que xelatex/pdflatex
// aborte TODO o documento con "Incomplete \iffalse; all text was ignored
// after line N" e non saia ningún PDF (report real, exame xerado coa IA).
// Mesma filosofía ca sanearAmbientesLatex: convértense en texto para que o
// resto compile, e avísase.
func sanearCondicionaisLatex(body string) (string, []string) {
	var abre, pecha int
	for _, m := range condicionalLatexRe.FindAllString(body, -1) {
		switch {
		case strings.HasPrefix(m, `\if`):
			abre++
		case m == `\fi`:
			pecha++
		}
	}
	if abre == pecha {
		return body, nil
	}
	n := 0
	out := condicionalLatexRe.ReplaceAllStringFunc(body, func(m string) string {
		n++
		return `\textbackslash{}` + strings.TrimPrefix(m, `\`)
	})
	return out, []string{fmt.Sprintf(
		"atopáronse %d primitivas condicionais de TeX (\\if.../\\fi) sen equilibrar no corpo (adoita ser contido da IA ou un <TEX> a medias): deixáronse coma texto para que o PDF compile. Revisa ese exercicio ou rexenérao.", n)}
}

var headingCmd = map[string]string{
	"h1": `\LARGE`, "h2": `\Large`, "h3": `\large`, "h4": `\normalsize`,
}

// latexConverter walks a parsed HTML fragment and renders it as LaTeX body
// source. Tags with no LaTeX meaning (head/style/script/link) are dropped
// entirely; anything not explicitly handled falls back to "keep the
// children, drop the tag" so unrecognised markup degrades gracefully
// instead of vanishing or crashing the build.
type latexConverter struct {
	baseDir     string // directory of the open .matex file, for <IMG> lookups
	buildDir    string // where referenced local images get copied to
	unknownTags map[string]bool
	warnings    []string
}

func newLatexConverter(baseDir, buildDir string) *latexConverter {
	return &latexConverter{baseDir: baseDir, buildDir: buildDir, unknownTags: map[string]bool{}}
}

func (c *latexConverter) render(n *html.Node) string {
	switch n.Type {
	case html.TextNode:
		return escapeOutsideMath(n.Data)
	case html.DocumentNode:
		return c.renderChildren(n)
	case html.ElementNode:
		return c.renderElement(n)
	default:
		return ""
	}
}

func (c *latexConverter) renderChildren(n *html.Node) string {
	var b strings.Builder
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		b.WriteString(c.render(ch))
	}
	return b.String()
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

func (c *latexConverter) renderElement(n *html.Node) string {
	tag := strings.ToLower(n.Data)
	switch tag {
	case "head", "style", "script", "link", "meta", "title":
		return "" // no LaTeX meaning, drop entirely (incl. children)
	case "html", "body", "span", "font", "ex", "resp", "sol":
		// "ex" is the block editor's structural exercise marker
		// (blocks-serialize.js); "resp"/"sol" are the table editor's answer/
		// solution markers (table-serialize.js). None have CAS/LaTeX meaning,
		// only their children matter - and cas.ParseText already dropped or
		// unwrapped resp/sol per RenderMode before we get here, so this just
		// keeps a stray one (hand-edited .matex) from being flagged as an
		// unsupported tag. Listed explicitly (not left to fall into default:).
		return c.renderChildren(n)
	case "a":
		return c.renderLink(n)
	case "div":
		return c.renderDiv(n)
	case "h1", "h2", "h3", "h4":
		return "\n{" + headingCmd[tag] + `\bfseries ` + c.renderChildren(n) + `}\par\medskip` + "\n"
	case "b", "strong":
		return `\textbf{` + c.renderChildren(n) + `}`
	case "i", "em":
		return `\textit{` + c.renderChildren(n) + `}`
	case "u":
		return `\underline{` + c.renderChildren(n) + `}`
	case "small":
		// Letra pequena (aclaracións, notas ao pé dun enunciado, "2 puntos"
		// a carón dunha pregunta). Sen este caso caía no default: e, aínda
		// que o texto se conservaba, saltaba o aviso "etiqueta HTML sen
		// soporte en PDF" en calquera documento que a usase - report real.
		// {\small ...} é LaTeX de base, non fai falla paquete ningún.
		return `{\small ` + c.renderChildren(n) + `}`
	case "big":
		return `{\large ` + c.renderChildren(n) + `}`
	case "code", "tt", "kbd", "samp":
		// Monoespazada. \texttt (non un entorno verbatim): os fillos xa
		// veñen escapados de cas.EscapeLatexProse, e verbatim non admite
		// contido xa procesado.
		return `\texttt{` + c.renderChildren(n) + `}`
	case "blockquote":
		return "\n\\begin{quote}\n" + c.renderChildren(n) + "\n\\end{quote}\n"
	case "sub":
		// Subíndice matemático (ex.: "lim<sub>x → ∞</sub>" para lim_{x→∞},
		// ou "H<sub>2</sub>O") - sen este caso caía no default: (etiqueta
		// descoñecida, só se conservan os fillos) e o subíndice desaparecía
		// por completo, deixando "limx → ∞" en liña recta sen apilar. $_{...}$
		// funciona sen base explícita en TeX (subíndice "solto" pegado ao que
		// veña antes) - non fai falla \ensuremath arredor: os fillos xa
		// escapan símbolos matemáticos (→, ∞...) coma \ensuremath{...} (ver
		// mathTextSymbols en cas/escape.go), que dentro de $...$ simplemente
		// se expande, sen entrar en conflito.
		return `$_{` + c.renderChildren(n) + `}$`
	case "sup":
		// Superíndice - mesmo razoamento ca "sub" enriba.
		return `$^{` + c.renderChildren(n) + `}$`
	case "br":
		// \par instead of \\ - \\ errors out ("no line here to end") if
		// it's the first thing in a paragraph (e.g. right after a
		// heading), which happens constantly in loosely-structured
		// Matexe documents. \par is a safe no-op in that situation.
		return "\\par\n"
	case "hr":
		// Regra horizontal visible (a diferenza de <br>, que só salta de
		// liña) - \rule{\linewidth}{0.4pt} debuxa unha liña de ancho
		// completo. \par antes/despois pola mesma razón ca <br>: evita que
		// "\rule" quede pegado a texto sen rematar o parágrafo primeiro.
		return "\\par\\noindent\\rule{\\linewidth}{0.4pt}\\par\n"
	case "p":
		return c.renderChildren(n) + `\par\medskip` + "\n"
	case "center":
		return "\n\\begin{center}\n" + c.renderChildren(n) + "\n\\end{center}\n"
	case "ul", "ol":
		env := "itemize"
		opts := ""
		if tag == "ol" {
			env = "enumerate"
			// type="a"/"A"/"i"/"I" e start=N (convención de exames: os
			// apartados adoitan ir "a)", "b)"...). Sen isto <ol type="a">
			// saía coma "1." "2." Precisa enumitem (ver preámbulo).
			opts = enumerateOpts(attr(n, "type"), attr(n, "start"))
		}
		var b strings.Builder
		b.WriteString("\n\\begin{" + env + "}" + opts + "\n")
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			if ch.Type == html.ElementNode && strings.ToLower(ch.Data) == "li" {
				b.WriteString(`\item ` + c.renderChildren(ch) + "\n")
			}
		}
		b.WriteString("\\end{" + env + "}\n")
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

// enumerateOpts traduce os atributos type/start dun <ol> á opción de
// enumitem (\begin{enumerate}[...]). type="a" -> "a)", "A" -> "A)", "i" ->
// "i)", "I" -> "I)"; calquera outra cousa (ou nada) deixa a numeración por
// defecto ("1."). start=N engade ", start=N". Devolve "" cando non hai nada
// que cambiar, así os <ol> normais renderízanse exactamente coma antes.
func enumerateOpts(tipo, start string) string {
	var partes []string
	switch strings.TrimSpace(tipo) {
	case "a":
		partes = append(partes, `label=\alph*)`)
	case "A":
		partes = append(partes, `label=\Alph*)`)
	case "i":
		partes = append(partes, `label=\roman*)`)
	case "I":
		partes = append(partes, `label=\Roman*)`)
	}
	if n, err := strconv.Atoi(strings.TrimSpace(start)); err == nil && n > 0 {
		partes = append(partes, fmt.Sprintf("start=%d", n))
	}
	if len(partes) == 0 {
		return ""
	}
	return "[" + strings.Join(partes, ", ") + "]"
}

// renderDiv special-cases the "izq"/"der" id convention Matexe exam
// headers commonly use for a two-column layout (name/course/date on the
// left, a logo/photo box on the right) - normally arranged with CSS
// floats we have no access to, so it's reproduced with side-by-side
// minipages instead. Anything else just flattens like before: we can't
// tell "these child divs should stack" from "these should sit side by
// side" without actually parsing CSS, and guessing wrong (e.g. for the
// name/course/date divs *inside* "izq" itself) would make things worse,
// not better.
func (c *latexConverter) renderDiv(n *html.Node) string {
	switch strings.ToLower(attr(n, "id")) {
	case "izq":
		return "\\begin{minipage}[t]{0.62\\linewidth}\n" + c.renderChildren(n) + "\n\\end{minipage}%\n"
	case "der":
		return "\\hfill\\begin{minipage}[t]{0.32\\linewidth}\n" + c.renderChildren(n) + "\n\\end{minipage}\n"
	default:
		return c.renderChildren(n) + "\n"
	}
}

func (c *latexConverter) renderTable(n *html.Node) string {
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
							cells = append(cells, c.renderChildren(cell))
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
	// p{width} columns wrap long text instead of running off the page -
	// plain "l" columns don't wrap at all, which matters since Matexe
	// documents use <TABLE> mainly to flow running text into side-by-side
	// columns (see test.matex's "Distribución del texto en columnas").
	colSpec := strings.Repeat(fmt.Sprintf(`p{%.3f\linewidth}`, 0.94/float64(cols)), cols)
	b.WriteString("\n\\begin{tabular}{" + colSpec + "}\n")
	for _, row := range rows {
		for len(row) < cols {
			row = append(row, "")
		}
		b.WriteString(strings.Join(row, " & ") + ` \\` + "\n")
	}
	b.WriteString("\\end{tabular}\n")
	return b.String()
}

// renderImg handles <IMG src="..."> references to static files alongside
// the .matex source (e.g. a school logo) - <PLOT>-generated images never
// reach here, those already became \includegraphics during tag parsing.
//
// width/height are optional HTML attributes on the <img> tag (same ones the
// block editor's "Inserir imaxe" control writes, see blocks.js) that let a
// header logo be sized precisely instead of always defaulting to 30% of the
// text width - see imgIncludeOptions.
func (c *latexConverter) renderImg(n *html.Node) string {
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
	dest := filepath.Join(c.buildDir, filepath.Base(src))
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		c.warnings = append(c.warnings, fmt.Sprintf("non se puido copiar a imaxe %q: %v", src, err))
		return ""
	}

	opts, standalone := imgIncludeOptions(attr(n, "width"), attr(n, "height"))
	include := `\includegraphics[` + opts + `]{` + filepath.Base(src) + `}`
	if standalone {
		// Sen width/height explícitos: comportamento orixinal, sen cambios
		// (centrado, 30% do ancho do texto) - compatibilidade cara atrás con
		// calquera <IMG> xa existente nun .matex previo a este cambio.
		return `\begin{center}` + include + `\end{center}`
	}
	// Con tamaño explícito (normalmente unha imaxe de cabeceira dentro dunha
	// <table>, ver o exemplo do editor): non se forza \begin{center}, para
	// non engadir salto vertical extra dentro dunha cela de táboa.
	return include
}

// renderLink handles <A href="..."> - either an external URL (http(s)://,
// mailto:) or a local file next to the .matex source (e.g. a PDF the
// explorer's "⬆️ Subir ficheiro" button just uploaded, see explorer.go's
// SubirFicheiroCartafol) - turned into a real clickable \href in the
// generated PDF (see \usepackage[hidelinks]{hyperref} in the preamble
// templates above; hidelinks keeps the link functional without the
// coloured box/text hyperref draws by default, which reads oddly on a
// printed exam). A LOCAL href is copied into buildDir at compile time, same
// as renderImg above, so pdflatex can find it while building THIS PDF -
// but unlike an embedded image, the linked file itself is never packed
// INTO the PDF: once the finished PDF is saved elsewhere (SavePDFDialog,
// app.go, "Gardar como PDF"), only the .pdf itself travels, so a local link
// only keeps working while the PDF stays next to that file (e.g. inside
// the SAME project folder the explorer uploaded it into) - an external URL
// has no such limitation. href isn't LaTeX-escaped (unlike the visible
// text, already escaped via renderChildren -> escapeOutsideMath): hyperref
// handles %/#/&/_ etc. INSIDE \href's URL argument itself via its own
// active-character machinery, and double-escaping here would break that.
func (c *latexConverter) renderLink(n *html.Node) string {
	href := attr(n, "href")
	text := c.renderChildren(n)
	if href == "" {
		return text
	}
	if text == "" {
		text = escapeOutsideMath(href) // fallback: bare URL/filename as visible text
	}
	if isExternalURL(href) {
		return `\href{` + href + `}{` + text + `}`
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
	dest := filepath.Join(c.buildDir, filepath.Base(href))
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		c.warnings = append(c.warnings, fmt.Sprintf("non se puido copiar o ficheiro ligado %q: %v", href, err))
		return text
	}
	return `\href{` + filepath.Base(href) + `}{` + text + `}`
}

// isExternalURL reports whether href is an absolute URL (works unchanged
// wherever the PDF ends up) rather than a local file path relative to the
// .matex source (see renderLink).
func isExternalURL(href string) bool {
	lower := strings.ToLower(href)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:")
}

// imgIncludeOptions traduce os atributos HTML width/height (CSS length: un
// número solto significa px, coma en HTML plano, ou pode levar unidade
// explícita px/cm/mm/in/pt/%) ás opcións de \includegraphics. standalone
// indica "non se deu ningún tamaño", para manter o comportamento orixinal.
// Con só un dos dous dáse keepaspectratio (non deformar); cos dous, estírase
// exactamente a esa caixa - mesma semántica que <img width height> en HTML.
func imgIncludeOptions(width, height string) (opts string, standalone bool) {
	w := cssLengthToLatex(width)
	h := cssLengthToLatex(height)
	switch {
	case w == "" && h == "":
		return `width=0.3\textwidth`, true
	case w != "" && h != "":
		return "width=" + w + ",height=" + h, false
	case w != "":
		return "width=" + w + ",keepaspectratio", false
	default:
		return "height=" + h + ",keepaspectratio", false
	}
}

var cssLengthRe = regexp.MustCompile(`^([0-9]*\.?[0-9]+)\s*(px|cm|mm|in|pt|%)?$`)

// cssLengthToLatex converte unha lonxitude estilo CSS (a que se atopa nun
// atributo width/height de HTML) a unha lonxitude \includegraphics de
// LaTeX. "" (ausente ou non recoñecida) significa "sen especificar".
func cssLengthToLatex(v string) string {
	m := cssLengthRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return ""
	}
	n, _ := strconv.ParseFloat(m[1], 64)
	switch m[2] {
	case "%":
		return fmt.Sprintf("%.4g\\textwidth", n/100)
	case "cm", "mm", "in", "pt":
		return fmt.Sprintf("%g%s", n, m[2])
	default: // px, ou número solto (mesmo por defecto ca en HTML/CSS)
		return fmt.Sprintf("%.4gcm", n/96*2.54)
	}
}

// parseFragment parses an HTML snippet as if it were the contents of
// <body>, so top-level text/tags don't need a full <html><body> wrapper.
func parseFragment(s string) ([]*html.Node, error) {
	context := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	return html.ParseFragment(strings.NewReader(s), context)
}

// latexPreambleTemplatePDF é para pdflatex (o motor por defecto): precisa
// inputenc/fontenc explícitos para UTF-8. inputenc xa soporta
// \DeclareUnicodeCharacter de fábrica dende hai anos - engádense aquí os
// símbolos matemáticos Unicode que máis doado aparecen soltos nun enunciado
// (escritos a man ou pola IA) fóra de <MAT>/<EVAL>, onde a fonte Computer
// Modern estándar non ten o glifo dispoñible en modo texto (atopado
// probando de verdade: "≈" facía fallar pdflatex enteiro). xelatex/lualatex
// (latexPreambleTemplateUnicode) non precisan isto: xa son Unicode nativos.
// NOTA: o texto normal do documento (fóra de <TEX>/<TIKZ>) xa NON depende
// disto - escapeOutsideMath (cas.EscapeLatexProse) substitúe estes mesmos
// símbolos por comandos LaTeX antes de chegar aquí, e así funciona igual en
// pdflatex e en xelatex/lualatex. Isto queda coma rede de seguridade só
// para <TEX>/<TIKZ> (código LaTeX/TikZ cru, non pasa por EscapeLatexProse).
// latexUnicodeDeclarations son as declaracións \DeclareUnicodeCharacter que
// necesita pdflatex (inputenc) para non petar cun símbolo Unicode solto
// escrito fóra de <MAT>/<EVAL> - extraídas do preámbulo por defecto porque
// tamén fan falla ao compilar cunha PLANTILLA do profesorado (plantillas.go):
// esa trae o seu propio \documentclass, e Yang inxéctalle este mesmo bloque
// (máis os \usepackage que lle falten) para que un <PLOT>/<TIKZ>/símbolo raro
// siga compilando igual de ben ca co preámbulo de serie.
const latexUnicodeDeclarations = `\DeclareUnicodeCharacter{2212}{\ensuremath{-}}
\DeclareUnicodeCharacter{2248}{\ensuremath{\approx}}
\DeclareUnicodeCharacter{2264}{\ensuremath{\leq}}
\DeclareUnicodeCharacter{2265}{\ensuremath{\geq}}
\DeclareUnicodeCharacter{2260}{\ensuremath{\neq}}
\DeclareUnicodeCharacter{00B1}{\ensuremath{\pm}}
\DeclareUnicodeCharacter{00D7}{\ensuremath{\times}}
\DeclareUnicodeCharacter{00F7}{\ensuremath{\div}}
\DeclareUnicodeCharacter{00B0}{\textdegree}
\DeclareUnicodeCharacter{2192}{\ensuremath{\rightarrow}}
\DeclareUnicodeCharacter{2190}{\ensuremath{\leftarrow}}
\DeclareUnicodeCharacter{2194}{\ensuremath{\leftrightarrow}}
\DeclareUnicodeCharacter{21D2}{\ensuremath{\Rightarrow}}
\DeclareUnicodeCharacter{21D0}{\ensuremath{\Leftarrow}}
\DeclareUnicodeCharacter{21D4}{\ensuremath{\Leftrightarrow}}
\DeclareUnicodeCharacter{221E}{\ensuremath{\infty}}
\DeclareUnicodeCharacter{2211}{\ensuremath{\sum}}
\DeclareUnicodeCharacter{221A}{\ensuremath{\surd}}
\DeclareUnicodeCharacter{222A}{\ensuremath{\cup}}
\DeclareUnicodeCharacter{2229}{\ensuremath{\cap}}
\DeclareUnicodeCharacter{2208}{\ensuremath{\in}}
\DeclareUnicodeCharacter{2209}{\ensuremath{\notin}}
\DeclareUnicodeCharacter{2282}{\ensuremath{\subset}}
\DeclareUnicodeCharacter{2286}{\ensuremath{\subseteq}}
\DeclareUnicodeCharacter{2205}{\ensuremath{\emptyset}}
\DeclareUnicodeCharacter{2216}{\ensuremath{\setminus}}
\DeclareUnicodeCharacter{211D}{\ensuremath{\mathbb{R}}}
\DeclareUnicodeCharacter{2115}{\ensuremath{\mathbb{N}}}
\DeclareUnicodeCharacter{2124}{\ensuremath{\mathbb{Z}}}
\DeclareUnicodeCharacter{211A}{\ensuremath{\mathbb{Q}}}
\DeclareUnicodeCharacter{2102}{\ensuremath{\mathbb{C}}}
\DeclareUnicodeCharacter{03B1}{\ensuremath{\alpha}}
\DeclareUnicodeCharacter{03B2}{\ensuremath{\beta}}
\DeclareUnicodeCharacter{03B3}{\ensuremath{\gamma}}
\DeclareUnicodeCharacter{03B4}{\ensuremath{\delta}}
\DeclareUnicodeCharacter{03B5}{\ensuremath{\varepsilon}}
\DeclareUnicodeCharacter{03B6}{\ensuremath{\zeta}}
\DeclareUnicodeCharacter{03B7}{\ensuremath{\eta}}
\DeclareUnicodeCharacter{03B8}{\ensuremath{\theta}}
\DeclareUnicodeCharacter{03B9}{\ensuremath{\iota}}
\DeclareUnicodeCharacter{03BA}{\ensuremath{\kappa}}
\DeclareUnicodeCharacter{03BB}{\ensuremath{\lambda}}
\DeclareUnicodeCharacter{03BC}{\ensuremath{\mu}}
\DeclareUnicodeCharacter{03BD}{\ensuremath{\nu}}
\DeclareUnicodeCharacter{03BE}{\ensuremath{\xi}}
\DeclareUnicodeCharacter{03C0}{\ensuremath{\pi}}
\DeclareUnicodeCharacter{03C1}{\ensuremath{\rho}}
\DeclareUnicodeCharacter{03C3}{\ensuremath{\sigma}}
\DeclareUnicodeCharacter{03C4}{\ensuremath{\tau}}
\DeclareUnicodeCharacter{03C5}{\ensuremath{\upsilon}}
\DeclareUnicodeCharacter{03C6}{\ensuremath{\varphi}}
\DeclareUnicodeCharacter{03C7}{\ensuremath{\chi}}
\DeclareUnicodeCharacter{03C8}{\ensuremath{\psi}}
\DeclareUnicodeCharacter{03C9}{\ensuremath{\omega}}
\DeclareUnicodeCharacter{0393}{\ensuremath{\Gamma}}
\DeclareUnicodeCharacter{0394}{\ensuremath{\Delta}}
\DeclareUnicodeCharacter{0398}{\ensuremath{\Theta}}
\DeclareUnicodeCharacter{039B}{\ensuremath{\Lambda}}
\DeclareUnicodeCharacter{039E}{\ensuremath{\Xi}}
\DeclareUnicodeCharacter{03A0}{\ensuremath{\Pi}}
\DeclareUnicodeCharacter{03A3}{\ensuremath{\Sigma}}
\DeclareUnicodeCharacter{03A6}{\ensuremath{\Phi}}
\DeclareUnicodeCharacter{03A8}{\ensuremath{\Psi}}
`

// O documento standalone co que cas rasteriza/valida un <TIKZ> ten que
// aceptar os mesmos símbolos Unicode soltos ca o boletín; sen isto, un
// debuxo con "≈" ou "±" nunha etiqueta compilaría no PDF final pero o prevoo
// de cas rexeitaríao. Ver cas.DeclaracionsUnicode / tikzDocStandalone.
func init() { cas.DeclaracionsUnicode = latexUnicodeDeclarations }

// tikzBabelLine carga a libraría "babel" de TikZ. Con babel (galician/
// spanish, ver babelLine) certos caracteres pasan a ser ACTIVOS - entre
// eles "<" e ">" - e dentro dun tikzpicture rebentan calquera "\draw[->]"
// ou "\draw[<->]" con "Argument of \language@active@arg> has an extra }"
// (report real nun <TIKZ> de vectores; o prevoo de cas non o collía porque
// tikzDocStandalone non carga babel). A libraría apaga os shorthands de
// babel ao entrar nun tikzpicture e restáuraos ao saír; é inofensiva se
// babel non se cargou (kpsewhich non atopou o .ldf), así que vai sempre.
const tikzBabelLine = "\\usetikzlibrary{babel}\n"

const latexPreambleTemplatePDF = `\documentclass[12pt]{article}
\usepackage[utf8]{inputenc}
\usepackage[T1]{fontenc}
` + latexUnicodeDeclarations + `%s\usepackage{amsmath,amssymb}
\usepackage{graphicx}
\usepackage{tikz}
\usepackage{gnuplot-lua-tikz}
\usepackage{circuitikz}
` + tikzBabelLine + `\usepackage{xcolor}
\usepackage{enumitem}
\usepackage[margin=2cm]{geometry}
\usepackage[hidelinks]{hyperref}
\setlength{\parindent}{0pt}
\begin{document}
`

// latexPreambleTemplateUnicode é para xelatex/lualatex: xa traballan en
// UTF-8 de fábrica, e inputenc/fontenc dan erro con eles ("You can't use
// `inputenc' with XeTeX") - fontspec é o paquete de fontes equivalente.
const latexPreambleTemplateUnicode = `\documentclass[12pt]{article}
\usepackage{fontspec}
%s\usepackage{amsmath,amssymb}
\usepackage{graphicx}
\usepackage{tikz}
\usepackage{gnuplot-lua-tikz}
\usepackage{circuitikz}
` + tikzBabelLine + `\usepackage{xcolor}
\usepackage{enumitem}
\usepackage[margin=2cm]{geometry}
\usepackage[hidelinks]{hyperref}
\setlength{\parindent}{0pt}
\begin{document}
`

const latexPostamble = "\n\\end{document}\n"

// preambuloPorDefecto devolve o preámbulo de serie que corresponde ao motor
// (aínda coa marca %s da liña de babel sen encher, ver babelLine). Existe
// para que plantillas.go poida reutilizar EXACTAMENTE o mesmo preámbulo ao
// envolver unha plantilla-fragmento (unha que só trae cabeceira e pé, sen
// \documentclass propio) - se se duplicase alá, calquera paquete engadido
// aquí deixaría de chegar a esas plantillas.
func preambuloPorDefecto(engine string) string {
	if engineUnicodeNativo(engine) {
		return latexPreambleTemplateUnicode
	}
	return latexPreambleTemplatePDF
}

// engineUnicodeNativo di se `engine` xa traballa en UTF-8/fontes do sistema
// de fábrica (xelatex, lualatex) fronte ao pdflatex por defecto, que
// necesita inputenc/fontenc - determina que preámbulo usar.
func engineUnicodeNativo(engine string) bool {
	return engine == "xelatex" || engine == "lualatex"
}

// babelLine picks a babel language the installed TeX Live can actually
// find (galician.ldf and even spanish.ldf require the texlive-lang-*
// packages, which a base/minimal install won't have - only english.ldf is
// always guaranteed to exist). Skipping babel entirely when nothing fits
// still produces a perfectly good PDF, just without language-aware
// hyphenation.
//
// "shorthands=off": galician/spanish babel fan ACTIVOS varios caracteres
// (<, >, ", ~, ...) para atallos tipográficos («», ñ...). Nun exame eses
// atallos non se usan (o contido xa vén en UTF-8 con «, ñ... de verdade) e
// en troques rebentan a compilación en canto aparece un "<", ">", "->" ou
// "<=" no contido - dentro dun <TIKZ>, dun <TEX>, ou nun texto que a IA non
// escapou - con erros crípticos ("Argument of \language@active@arg> has an
// extra }", "Incomplete \iffalse"). Apagalos de raíz é máis seguro e non
// perde nada visible. (A libraría tikz "babel", ver tikzBabelLine, xa
// cubría o caso dos tikzpicture; isto cobre todo o demais.)
func babelLine() string {
	for _, lang := range []string{"galician", "spanish"} {
		if kpsewhichFound(lang + ".ldf") {
			return `\usepackage[` + lang + `,shorthands=off]{babel}` + "\n"
		}
	}
	return ""
}

// babelListquotFix neutraliza un segundo xeito (independente de
// "shorthands=off", babelLine enriba) de que babel-galician/babel-spanish
// rompan calquera <ol>/<ul> de dous ou máis elementos con "Incomplete
// \iffalse; all text was ignored after line N" - SEN que faga falla
// <TIKZ> nin ningún carácter activo (<, >...) no medio: abonda con DOUS
// "\item" en calquera lista, mesmo "\item $x+y=1$" repetido dúas veces
// (report real, reproducido illado con "gauss_jordan.matex" - unha
// "resolución paso a paso" con varios pasos numerados abonda).
//
// Motivo (galician.ldf/spanish.ldf, función gl@enumdef/\gl@itemize):
// \labelenumN e \labelitemN redefínense para chamaren \gl@listquot (ou
// es@listquot), pensado para acentos de peche de cita cando un \item
// continúa un diálogo dentro dun ambiente "quoting" - Yang nunca usa
// "quoting", pero \gl@listquot chámase IGUALMENTE en cada \item, e a súa
// interacción coas modificacións de \item de enumitem (\usepackage
// {enumitem} enriba, necesario para <ol type="a">) é o que desincroniza o
// contador de condicionais de TeX e xera o \iffalse fantasma.
//
// A neutralización ten que ir DESPOIS de \begin{document} (non no
// preámbulo): babel volve activar \gl@listquot ao seleccionar o idioma,
// nun hook que se executa XUSTO AO EMPEZAR O DOCUMENTO - calquera
// \renewcommand no preámbulo queda pisado por ese hook e non serve de
// nada (comprobado). \@ifundefined fai que isto sexa inofensivo cando
// babel non cargou galego/castelán (inglés, ou sen babel ningún): non
// toca nada que non exista. \gl@listquot/es@listquot non levan
// argumentos e non se usan para máis nada no documento (a numeración en
// si - "1.", "a)"...) - queda intacta, só desaparece o efecto colateral
// que rompía a compilación.
//
// Insírese en documentoLatexConPlantilla (plantillas.go), canda
// tipografiaCorpo - así chega XUSTO tras \begin{document} nos tres casos
// (preámbulo por defecto, plantilla fragmento e plantilla con
// \documentclass propio), non só cando Yang pon o seu propio preámbulo.
const babelListquotFix = `\makeatletter
\@ifundefined{gl@listquot}{}{\renewcommand{\gl@listquot}{}}
\@ifundefined{es@listquot}{}{\renewcommand{\es@listquot}{}}
\makeatother
`

func kpsewhichFound(file string) bool {
	if !commandExists("kpsewhich") {
		return false
	}
	cmd := exec.Command("kpsewhich", file)
	ocultarConsola(cmd)
	return cmd.Run() == nil
}

// motoresParaProbar devolve, en orde, os motores LaTeX candidatos para
// compilar un documento: primeiro o preferido (o escollido en Opcións),
// despois os outros dous coma fallback transparente - só os que estean
// realmente instalados no sistema. Isto é o que permite a compileLatexAuto
// recuperarse dun motor mal escollido (ou non instalado) sen que o usuario
// teña que decatarse: se o preferido non chega a xerar PDF, próbanse os
// demais antes de darse por vencido.
func motoresParaProbar(preferido string) []string {
	todos := []string{"pdflatex", "xelatex", "lualatex"}
	orde := todos
	if preferido != "" {
		orde = append([]string{preferido}, todos...)
	}
	var dispoñibles []string
	xaProbado := map[string]bool{}
	for _, m := range orde {
		if xaProbado[m] || !commandExists(m) {
			continue
		}
		xaProbado[m] = true
		dispoñibles = append(dispoñibles, m)
	}
	return dispoñibles
}

// compileLatexAuto compila body co motor preferido e, se falla (erro de
// compilación, non un motor ausente - iso xa o corta motoresParaProbar),
// reintenta transparentemente cos demais motores instalados no sistema.
// Devolve o resultado do primeiro que compile con éxito; usedEngine di cal
// foi realmente, para que a chamante poida avisar se non coincide co
// configurado. Se ningún compila, devolve o erro do motor preferido (o máis
// relevante para diagnosticar), non o do último intento.
func compileLatexAuto(dir, body, preferido string, plantilla *PlantillaLatex, nomeDoc string, tip *TipografiaOpts) (pdf []byte, texSource, log, usedEngine string, avisos []string, err error) {
	candidatos := motoresParaProbar(preferido)
	if len(candidatos) == 0 {
		pacote := "texlive-latex-base"
		switch preferido {
		case "xelatex":
			pacote = "texlive-xetex"
		case "lualatex":
			pacote = "texlive-luatex"
		}
		return nil, "", "", "", nil, fmt.Errorf("non se atopou %s. Instala unha distribución LaTeX (p.ex. 'sudo apt install %s')", preferido, pacote)
	}

	var primeiroErro error
	var primeiroLog, primeiroTex string
	var primeirosAvisos []string
	for i, engine := range candidatos {
		p, tex, l, av, e := compileLatex(dir, body, engine, plantilla, nomeDoc, tip)
		if e == nil {
			return p, tex, l, engine, av, nil
		}
		if i == 0 {
			primeiroErro, primeiroLog, primeiroTex, primeirosAvisos = e, l, tex, av
		}
	}
	return nil, primeiroTex, primeiroLog, "", primeirosAvisos, primeiroErro
}

// compileLatex writes body wrapped in the document template to dir/doc.tex
// and compiles it with pdflatex, returning the resulting PDF bytes, the full
// .tex source that was compiled (so it can be offered for export as-is -
// see GeneratePDFResult.LatexSource), and the compiler's combined
// stdout/stderr log.
func compileLatex(dir, body, engine string, plantilla *PlantillaLatex, nomeDoc string, tip *TipografiaOpts) (pdf []byte, texSource string, log string, avisos []string, err error) {
	if strings.TrimSpace(engine) == "" {
		engine = "pdflatex"
	}
	// As imaxes da plantilla (logotipos) van a carón do doc.tex: así o
	// \includegraphics{logo.png} que escribiu o profesorado atópaas sen
	// depender de onde estea gardado o .matex. Ver copiarImaxesPlantilla.
	if err := copiarImaxesPlantilla(plantilla, dir); err != nil {
		return nil, "", "", nil, fmt.Errorf("non se puideron copiar as imaxes da plantilla: %w", err)
	}
	texPath := filepath.Join(dir, "doc.tex")
	full, avisosPlantilla, err := documentoLatexConPlantilla(body, engine, plantilla, nomeDoc, tip)
	if err != nil {
		return nil, "", "", nil, err
	}
	avisos = avisosPlantilla
	if err := os.WriteFile(texPath, []byte(full), 0o644); err != nil {
		return nil, "", "", avisos, err
	}

	var out bytes.Buffer
	// -synctex=1: necesario para o Ctrl+clic "reverse search" na
	// previsualización (ver reversesearch.go) - xera doc.synctex.gz xunto
	// co PDF, sen custo perceptible de tempo de compilación.
	cmd := exec.Command(engine, "-interaction=nonstopmode", "-halt-on-error", "-synctex=1", "doc.tex")
	cmd.Dir = dir
	// gnuplot-lua-tikz.sty (require un <PLOT> con saída TikZ) non o atopan
	// nin MiKTeX en Windows (vive dentro do cartafol de Maxima, ver
	// latexenv_windows.go) nin MacTeX en macOS (alí nin sequera existe: hai
	// que xeralo con gnuplot, ver latexenv_darwin.go); nos dous casos
	// indícaselles onde está vía TEXINPUTS. nil/sen efecto en Linux, onde o
	// paquete do sistema xa o deixa nun sitio estándar.
	if extra := latexEnvExtra(); extra != nil {
		cmd.Env = append(os.Environ(), extra...)
	}
	ocultarConsola(cmd)
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	log = out.String()

	data, readErr := os.ReadFile(filepath.Join(dir, "doc.pdf"))
	if readErr != nil {
		if runErr != nil {
			// A liña real do erro (a que empeza por "!") queda en `log`, mais
			// `log` nunca chega ao frontend cando GeneratePDF devolve erro: o
			// binding Wails descarta todo o GeneratePDFResult e só entrega o
			// error - "pdflatex fallou: exit status 1" a secas, sen dicir
			// por qué. Métese aquí para que a causa real si chegue.
			if pista := pistaLatexAccionable(log); pista != "" {
				// A pista vai DIANTE do erro cru: é o que ten que ler
				// primeiro quen o vexa, non o final dunha mensaxe longa.
				return nil, full, log, avisos, fmt.Errorf("%s: %s", pista, extraerErroPDFLatex(log))
			}
			if detalle := extraerErroPDFLatex(log); detalle != "" {
				return nil, full, log, avisos, fmt.Errorf("%s fallou: %s", engine, detalle)
			}
			return nil, full, log, avisos, fmt.Errorf("%s fallou: %w", engine, runErr)
		}
		return nil, full, log, avisos, readErr
	}
	return data, full, log, avisos, nil
}

// fonteQueFaltaRe/paqueteQueFaltaRe recoñecen os dous fallos que máis se
// dan cunha PLANTILLA do profesorado (plantillas.go), sobre todo se a
// escribiu a IA a partir dun modelo bonito: pide unha fonte do sistema ou
// un paquete de LaTeX que ese equipo non ten. O erro cru de LaTeX non lle
// di a ninguén que facer; estas dúas pistas si.
var fonteQueFaltaRe = regexp.MustCompile(`The font "([^"]+)" cannot be found`)
var paqueteQueFaltaRe = regexp.MustCompile("File `([A-Za-z0-9@_.-]+)\\.sty' not found")

// ambienteIndefinidoRe: "! LaTeX Error: Environment foobar undefined." -
// case típico de contido da IA que escribe \begin{algo} cun ambiente que non
// existe (ou que non ten sentido nese sitio). sanearAmbientesLatex xa
// desactiva os \begin/\end SEN "{", pero un \begin{align}/\begin{aligned}/…
// mal colocado aínda chega aquí.
var ambienteIndefinidoRe = regexp.MustCompile(`Environment (\S+) undefined`)

// pistaLatexAccionable traduce un log de LaTeX a unha frase que diga QUE
// hai que facer, ou "" se non recoñece o caso (entón queda o erro cru de
// sempre, que para un erro de sintaxe é máis útil ca calquera paráfrase).
func pistaLatexAccionable(log string) string {
	if m := fonteQueFaltaRe.FindStringSubmatch(log); m != nil {
		return fmt.Sprintf("a plantilla pide a fonte «%s», que non está instalada neste equipo: instálaa, ou cambia a fonte (\\setmainfont/\\setsansfont/\\setmonofont) no editor de plantillas", m[1])
	}
	if m := paqueteQueFaltaRe.FindStringSubmatch(log); m != nil {
		return fmt.Sprintf("falta o paquete de LaTeX «%s»: instálao (p.ex. 'sudo tlmgr install %s') ou quítao da plantilla", m[1], m[1])
	}
	if m := ambienteIndefinidoRe.FindStringSubmatch(log); m != nil {
		return fmt.Sprintf("o documento usa un ambiente LaTeX que non existe («%s»); adoita vir da IA ao mesturar \\begin{...} coas etiquetas <...>. Rexenera ese exercicio, ou na súa resolución usa <SISTEMA> para os sistemas e <MAT>matrix(...)</MAT> para as matrices en vez de \\begin{...}", strings.Trim(m[1], `.`))
	}
	return ""
}

// extraerErroPDFLatex busca as liñas de erro estándar de pdflatex (as que
// empezan por "!", ex. "! Undefined control sequence.", "! Package ...
// Error: ...") máis a liña "l.NN ..." que indica ONDE - mesmo patrón que xa
// usa cas.tikz para os seus propios avisos.
//
// A liña "l.NN" non sempre vai xusto debaixo do "!": erros coma "! Missing
// \endgroup inserted." ou "! Missing $ inserted." meten primeiro o que TeX
// inseriu ("<inserted text>", "<template>", "<recently read>"...) e só
// despois a localización. Buscando só na liña seguinte quedaba fóra
// precisamente nos erros que máis falla fan de localizar, e o profesorado
// recibía un "! Missing \endgroup inserted." pelado, sen liña ningunha.
// Por iso ollamos unhas cantas liñas cara adiante, ata a primeira "l.NN" ou
// ata o vindeiro "!" (que xa é outro erro distinto).
const maxLiñasAtaLocalizacion = 8

// maxLiñasContinuacion limita canto se colle das liñas "(paquete) ..." que
// seguen a un erro de paquete: as primeiras din o problema, o resto adoita
// ser a lea xenérica de LaTeX ("this may be but usually is not a bug...").
const maxLiñasContinuacion = 4

// continuacionPaquete devolve o texto dunha liña de continuación de erro de
// paquete ("(fontspec)      The font ... cannot be found"), ou "" se a liña
// non o é. O formato é sempre (nome-do-paquete) + recheo de espazos.
var continuacionRe = regexp.MustCompile(`^\(([A-Za-z0-9@_.-]+)\)\s+(\S.*)$`)

func continuacionPaquete(liña string) string {
	m := continuacionRe.FindStringSubmatch(strings.TrimRight(liña, "\r"))
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[2])
}

func extraerErroPDFLatex(log string) string {
	liñas := strings.Split(log, "\n")
	var atopadas []string
	for i, l := range liñas {
		if !strings.HasPrefix(strings.TrimSpace(l), "!") {
			continue
		}
		erro := strings.TrimSpace(l)
		// Continuación "(paquete)   ...": os erros de paquete parten a
		// explicación en varias liñas prefixadas co seu nome, e a PRIMEIRA
		// (a que empeza por "!") pode quedar baleira despois dos dous
		// puntos. Report real: "! Package fontspec Error:" a secas, cando o
		// que fai falla saber ("The font "Fira Sans" cannot be found") ía
		// nas liñas de abaixo. Únense nunha soa frase, colapsando o
		// recheo de espazos co que LaTeX as aliña.
		for j := i + 1; j < len(liñas) && j <= i+maxLiñasContinuacion; j++ {
			resto := continuacionPaquete(liñas[j])
			if resto == "" {
				break
			}
			erro = strings.TrimSpace(erro) + " " + resto
		}
		atopadas = append(atopadas, erro)
		for j := i + 1; j < len(liñas) && j <= i+maxLiñasAtaLocalizacion; j++ {
			seguinte := strings.TrimSpace(liñas[j])
			if strings.HasPrefix(seguinte, "!") {
				break
			}
			if strings.HasPrefix(seguinte, "l.") {
				atopadas = append(atopadas, seguinte)
				break
			}
		}
	}
	return strings.Join(atopadas, " | ")
}

// pdfPageImages rasterizes every page of a compiled PDF to PNG via pdftoppm
// (poppler-utils - the same tool cas.tikz already relies on for its HTML
// preview), so the webview can show a faithful print preview: WebKitGTK's
// embedded webview (what Wails uses on Linux) has no reliable native PDF
// renderer to point an <iframe>/<embed> at, but any webview can show <img>
// tags. Returns one data: URI per page, in page order.
func pdfPageImages(dir, pdfPath string) ([]string, error) {
	prefix := filepath.Join(dir, "page")
	cmd := exec.Command("pdftoppm", "-png", "-r", "100", pdfPath, prefix)
	ocultarConsola(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm: %w: %s", err, out)
	}

	matches, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return nil, err
	}
	// pdftoppm's own numeric suffix (page-1.png, page-2.png, ...) doesn't
	// zero-pad below 10 pages, so a plain string sort would put page-10.png
	// before page-2.png - sort by the parsed page number instead.
	pageNumRe := regexp.MustCompile(`-(\d+)\.png$`)
	pageNum := func(name string) int {
		m := pageNumRe.FindStringSubmatch(name)
		if m == nil {
			return 0
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}
	sort.Slice(matches, func(i, j int) bool { return pageNum(matches[i]) < pageNum(matches[j]) })

	images := make([]string, 0, len(matches))
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			return nil, err
		}
		images = append(images, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(data))
	}
	return images, nil
}

// GeneratePDFRequest mirrors GenerateRequest, plus BaseDir - the directory
// of the currently open .matex file, needed to resolve <IMG src="..."> to
// static files (e.g. a school logo) that must be copied next to doc.tex.
type GeneratePDFRequest struct {
	Source     string `json:"source"`
	CodeIni    string `json:"codeIni"`
	Seed       int    `json:"seed"`
	Iterations int    `json:"iterations"`
	BaseDir    string `json:"baseDir"`
	// NomeDoc é o nome do documento aberto (sen extensión), só para encher
	// as variables automáticas {{TITULO}}/{{FICHEIRO}} dunha plantilla -
	// ver plantillas.go. Baleiro cando non hai ficheiro gardado aínda.
	NomeDoc string `json:"nomeDoc"`
	// Modo: igual ca en GenerateRequest (app.go) - ""/"enunciados",
	// "resposta" ou "solucions". Ver cas.Maxima.RenderMode.
	Modo string `json:"modo"`
	// Tipografia: axustes globais de tamaño de letra / interliñado / fonte
	// escollidos no botón "Axustes do texto" da barra de previsualización.
	// nil (ou todo a cero) = tipografía por defecto do documento/plantilla.
	Tipografia *TipografiaOpts `json:"tipografia"`
}

// TipografiaOpts leva os tres axustes globais do botón "Axustes do texto"
// (frontend/src/main.js). Aplícanse SÓ á saída PDF (é o que se
// previsualiza); a plantilla, se ten \documentclass propio, segue mandando
// na súa fonte (só se lle aplica o tamaño/interliñado).
type TipografiaOpts struct {
	TamanoPt    float64 `json:"tamanoPt"`    // corpo do texto en pt; 0 = por defecto
	Interlinado float64 `json:"interlinado"` // factor de \linespread; 0 = 1.0 (sinxelo)
	Fonte       string  `json:"fonte"`       // id dun preset (ver presetsFonte); "" = por defecto
}

// presetsFonte: id -> paquete a cargar en pdflatex / nome de fonte para
// \setmainfont en xelatex+lualatex. Só familias moi estendidas (viven en
// texlive-latex-recommended / texlive-fonts-recommended); se faltan, o aviso
// de "instalar paquete de LaTeX" (paquetelatex.go) xa o cobre.
var presetsFonte = map[string]struct{ pdflatex, unicode string }{
	"serif":    {"\\usepackage{mathptmx}", "TeX Gyre Termes"},
	"sans":     {"\\usepackage[scaled]{helvet}\n\\renewcommand\\familydefault{\\sfdefault}", "TeX Gyre Heros"},
	"palatino": {"\\usepackage{mathpazo}", "TeX Gyre Pagella"},
}

// esFonteDoSistema di se `f` é un nome de familia do sistema (fc-list), é
// dicir: non baleiro e non un dos presets. Estas fontes SÓ as pode usar
// xelatex/lualatex - GeneratePDF forza o motor cando aparece unha.
func esFonteDoSistema(f string) bool {
	f = strings.TrimSpace(f)
	if f == "" {
		return false
	}
	_, ok := presetsFonte[f]
	return !ok
}

// fontNameSafeRe limpa o nome de familia que chega do frontend antes de
// metelo nun \setmainfont{...}: só letras, díxitos, espazos e puntuación
// inofensiva (os nomes de fonte reais están dentro diso). Evita inxección de
// LaTeX a través do valor do selector.
var fontNameSafeRe = regexp.MustCompile(`[^\p{L}\p{N} .,'&()+_-]`)

// tipografiaPreambulo: liña(s) que van no PREÁMBULO (carga da familia de
// fonte). "" se non hai fonte escollida, ou se é unha fonte do sistema pero
// o motor non é Unicode (nese caso GeneratePDF xa debería telo forzado).
func tipografiaPreambulo(t *TipografiaOpts, engine string) string {
	if t == nil || strings.TrimSpace(t.Fonte) == "" {
		return ""
	}
	f := strings.TrimSpace(t.Fonte)
	if p, ok := presetsFonte[f]; ok {
		if engineUnicodeNativo(engine) {
			if p.unicode == "" {
				return ""
			}
			return "\\setmainfont{" + p.unicode + "}\n"
		}
		return p.pdflatex + "\n"
	}
	// Fonte do sistema: só fontspec (xelatex/lualatex).
	if !engineUnicodeNativo(engine) {
		return ""
	}
	nome := strings.TrimSpace(fontNameSafeRe.ReplaceAllString(f, ""))
	if nome == "" {
		return ""
	}
	return "\\setmainfont{" + nome + "}\n"
}

// tipografiaCorpo: comandos que van XUSTO DESPOIS de \begin{document}
// (tamaño e interliñado). "" se ambos son os de por defecto.
func tipografiaCorpo(t *TipografiaOpts) string {
	if t == nil || (t.TamanoPt <= 0 && t.Interlinado <= 0) {
		return ""
	}
	inter := t.Interlinado
	if inter <= 0 {
		inter = 1.0
	}
	if t.TamanoPt > 0 {
		// baseline = tamaño * 1.2 (factor estándar de TeX) * interliñado.
		return fmt.Sprintf("\\fontsize{%spt}{%spt}\\selectfont\n",
			trimFloat(t.TamanoPt), trimFloat(t.TamanoPt*1.2*inter))
	}
	return fmt.Sprintf("\\linespread{%s}\\selectfont\n", trimFloat(inter))
}

// trimFloat formatea un float a 2 decimais como moito e sen ceros de cola
// ("13", "1.5", "15.6") - evita que 12*1.2*1.5 saia como
// "21.599999999999998pt" no .tex.
func trimFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

type GeneratePDFResult struct {
	PDFBase64 string `json:"pdfBase64"`
	// LatexSource is the exact .tex source pdflatex compiled (preamble +
	// body + \end{document}) - plain text, no base64 needed. Lets the user
	// export/edit the LaTeX directly instead of only the compiled PDF.
	LatexSource string `json:"latexSource"`
	// PageImages holds one data: URI (PNG) per PDF page, for previewing the
	// compiled document in the webview - see pdfPageImages. Empty if
	// pdftoppm isn't installed; the PDF itself still generates fine, it
	// just can't be shown page-by-page in the preview pane.
	PageImages []string `json:"pageImages"`
	Warnings   []string `json:"warnings"`
	Log        string   `json:"log"` // pdflatex's own output, for troubleshooting a failed compile
}

// GeneratePDF runs the same per-variant Maxima loop as Generate, but in
// cas.FormatLatex mode: <MAT>/<EVAL> become real $...$ TeX (Maxima already
// produces it via tex(), we just stop wrapping it for MathJax), <PLOT>
// images are referenced with \includegraphics, and the surrounding HTML
// markup is converted to LaTeX via latexConverter before being compiled
// with pdflatex.
func (a *App) GeneratePDF(req GeneratePDFRequest) (GeneratePDFResult, error) {
	result := GeneratePDFResult{}

	if strings.TrimSpace(req.Source) == "" {
		return result, fmt.Errorf("o documento está baleiro")
	}
	if req.Iterations < 1 {
		req.Iterations = 1
	}
	// A plantilla activa (se hai) decide a maqueta do documento e pode
	// esixir un motor concreto (unha que use fontspec precisa xelatex) -
	// ver plantillas.go.
	plantilla := a.plantillaActiva()
	engine := strings.TrimSpace(motorParaPlantilla(plantilla, a.settings.LatexEngine))
	if engine == "" {
		engine = "pdflatex"
	}
	// Unha fonte do sistema escollida en "Axustes do texto" só a pode usar
	// fontspec: fórzase xelatex aínda que Opcións teña pdflatex. (Se hai
	// plantilla que xa esixe xelatex/lualatex, respéctase o seu.)
	if req.Tipografia != nil && esFonteDoSistema(req.Tipografia.Fonte) && !engineUnicodeNativo(engine) {
		engine = "xelatex"
	}
	if len(motoresParaProbar(engine)) == 0 {
		pacote := "texlive-latex-base"
		switch engine {
		case "xelatex":
			pacote = "texlive-xetex"
		case "lualatex":
			pacote = "texlive-luatex"
		}
		return result, fmt.Errorf("non se atopou %s. Instala unha distribución LaTeX (p.ex. 'sudo apt install %s')", engine, pacote)
	}
	if strings.TrimSpace(a.maximaPath) == "" {
		return result, fmt.Errorf("non se atopou Maxima. Configura a ruta en Opcións")
	}

	codeIni := req.CodeIni
	if strings.TrimSpace(codeIni) == "" {
		codeIni = a.settings.CodeIni
	}
	if strings.TrimSpace(codeIni) == "" {
		codeIni = defaultCodeIni
	}
	codeIniLines := splitLines(codeIni)

	tmpDir, err := os.MkdirTemp("", "matexe-pdf-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(tmpDir)
	if err := os.MkdirAll(filepath.Join(tmpDir, "images"), 0o755); err != nil {
		return result, err
	}

	sess, err := cas.Open(a.maximaPath)
	if err != nil {
		return result, fmt.Errorf("non se puido iniciar Maxima (%s): %w", a.maximaPath, err)
	}
	defer a.trackMaximaSession(sess)()
	m := cas.NewMaxima(sess)
	defer m.Close()
	m.SetOutDir(tmpDir)
	m.Format = cas.FormatLatex
	m.SetTimeout(a.settings.TimeoutResolto())
	m.ForzarDecimal = a.settings.Decimal
	m.RenderMode = req.Modo

	reg := newRawRegistry()
	m.ProtectRaw = reg.Protect

	if _, err := m.Send("linel:1024;display2d:false;simp:true;", false); err != nil {
		return result, err
	}

	conv := newLatexConverter(req.BaseDir, tmpDir)

	// insertOffsetMarkers vai FÓRA do bucle de variantes a propósito: os
	// offsets que calcula son sobre req.Source (o mesmo en todas as
	// variantes, só cambia a semente), calculalo unha soa vez abonda - ver
	// reversesearch.go.
	sourceWithMarkers := insertOffsetMarkers(req.Source, reg)

	var body strings.Builder
	for i := 0; i < req.Iterations; i++ {
		variantSeed := req.Seed + i

		if _, err := m.Send("reset();kill(all)", true); err != nil {
			return result, err
		}
		// SetDecimais TEN que ir despois de kill(all), non antes do bucle:
		// kill(all) borra TODA definición de usuario, incluída a función
		// matexe_arredondar que SetDecimais acaba de definir - sen
		// redefinila aquí, ToTeX devolvería "matexe_arredondar(...)" sen
		// avaliar en cada <MAT>/<EVAL> de cada variante.
		if err := m.SetDecimais(a.settings.DecimaisResolto()); err != nil {
			return result, err
		}
		for _, line := range codeIniLines {
			if _, err := m.Send(line, false); err != nil {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("variante %d, codeini %q: %v", i+1, line, err))
			}
		}
		if err := m.SetSeed(variantSeed); err != nil {
			return result, err
		}

		rawLatex, err := m.ParseText(preprocessPseudoTags(sourceWithMarkers, reg))
		if err != nil {
			return result, fmt.Errorf("variante %d (semente %d): %w", i+1, variantSeed, err)
		}

		nodes, err := parseFragment(rawLatex)
		if err != nil {
			return result, fmt.Errorf("variante %d: erro analizando HTML: %w", i+1, err)
		}
		if i > 0 {
			body.WriteString("\\clearpage\n")
		}
		for _, n := range nodes {
			body.WriteString(conv.render(n))
		}
	}

	result.Warnings = append(result.Warnings, conv.warnings...)
	for t := range conv.unknownTags {
		result.Warnings = append(result.Warnings, "etiqueta HTML sen soporte en PDF (mantívose o contido): <"+t+">")
	}

	corpoLatex, avisosAmbiente := sanearAmbientesLatex(reg.Resolve(body.String()))
	result.Warnings = append(result.Warnings, avisosAmbiente...)
	corpoLatex, avisosCond := sanearCondicionaisLatex(corpoLatex)
	result.Warnings = append(result.Warnings, avisosCond...)

	pdf, texSource, log, usedEngine, avisosPlantilla, err := compileLatexAuto(tmpDir, corpoLatex, engine, plantilla, req.NomeDoc, req.Tipografia)
	result.Warnings = append(result.Warnings, avisosPlantilla...)
	result.Log = log
	result.LatexSource = texSource
	if err != nil {
		// Se o que faltou foi un paquete de LaTeX, déixase anotado para que
		// o frontend poida ofrecer instalalo (ver paquetelatex.go): o log
		// non chega ao frontend cando GeneratePDF devolve erro.
		anotarPaqueteQueFalta(log)
		return result, fmt.Errorf("erro ao compilar PDF: %w", err)
	}
	limparPaqueteQueFalta()
	if usedEngine != engine {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s non conseguiu compilar este documento; usouse %s automaticamente. Podes cambiar o motor por defecto en Opcións.", engine, usedEngine))
	}
	result.PDFBase64 = base64.StdEncoding.EncodeToString(pdf)

	if commandExists("pdftoppm") {
		images, imgErr := pdfPageImages(tmpDir, filepath.Join(tmpDir, "doc.pdf"))
		if imgErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("non se puido xerar a vista previa das páxinas: %v", imgErr))
		} else {
			result.PageImages = images
		}
	} else {
		result.Warnings = append(result.Warnings, "non se atopou pdftoppm (paquete poppler-utils): sen vista previa páxina a páxina")
	}

	// Gardar doc.tex/doc.pdf/doc.synctex.gz para o Ctrl+clic "reverse
	// search" (ver reversesearch.go) - tmpDir bórrase co defer de arriba en
	// canto esta función remata, así que sen isto non quedaría nada que
	// consultar despois. Non fatal se falla: só se perde o Ctrl+clic desta
	// xeración, o PDF/previsualización xa se devolveron igual.
	if err := a.rememberBuildForReverseSearch(tmpDir); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("non se puido gardar a compilación para Ctrl+clic: %v", err))
	}

	return result, nil
}

// SavePDFDialog shows a native "save as" dialog and writes base64-encoded
// PDF bytes (as returned by GeneratePDF) to the chosen path.
func (a *App) SavePDFDialog(defaultName, pdfBase64 string) (string, error) {
	dir, base := splitDialogDefault(defaultName)
	path, err := saveFileDialogCompat("Exportar PDF", dir, base, "PDF (*.pdf)", "*.pdf")
	if err != nil || path == "" {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(pdfBase64)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// SaveTexDialog shows a native "save as" dialog and writes the .tex source
// (as returned by GeneratePDF's LatexSource) to the chosen path - plain
// text, unlike SavePDFDialog there's no base64 decoding step.
func (a *App) SaveTexDialog(defaultName, content string) (string, error) {
	dir, base := splitDialogDefault(defaultName)
	path, err := saveFileDialogCompat("Exportar código LaTeX", dir, base, "LaTeX (*.tex)", "*.tex")
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
