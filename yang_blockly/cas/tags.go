package cas

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// tagPattern mirrors the alternation regex built in Tcas.parse_text from
// cas.pas: (?s) makes '.' match newlines too, since a tag's content can
// span multiple lines. TIKZ has no Pascal ancestor - it's new, added for
// the block editor's "gráfico TikZ" element type (see blocks-serialize.js).
// <RESP>/<SOL> son as etiquetas de corrección da interface de táboa
// (resposta final / solución paso a paso). Van ao final da alternación para
// non mover os índices de grupo dos que xa había. Renderízanse segundo
// m.RenderMode, regra ESTRITA (ver mostrarCorreccion en maxima.go): en
// "enunciados" descártanse as dúas, en "resposta" só <RESP>, en "solucions"
// só <SOL>. Cando se descarta, non se toca Maxima.
var tagPattern = regexp.MustCompile(`(?s)<MAT>(.*?)</MAT>|<EVAL>(.*?)</EVAL>|<HIDE>(.*?)</HIDE>|<PLOT>(.*?)</PLOT>|<TEX>(.*?)</TEX>|<TIKZ>(.*?)</TIKZ>|<SISTEMA>(.*?)</SISTEMA>|<RESP>(.*?)</RESP>|<SOL>(.*?)</SOL>`)

// TagPattern exposes tagPattern to other packages that need to scan a
// .matex source for tag boundaries WITHOUT evaluating them - e.g.
// insertOffsetMarkers (reversesearch.go) needs the exact byte offset of
// every tag in the PRISTINE source (before ParseText runs), for the
// Ctrl+clic-on-a-result "reverse search" feature. Kept as a single
// definition (this var) so a caller of TagPattern can never drift from
// what ParseText itself actually matches.
var TagPattern = tagPattern

// OutDir is where generated images (<PLOT>) are written, relative to the
// final HTML file - mirrors Tcas.pathDocs.
func (m *Maxima) SetOutDir(dir string) { m.outDir = dir }

// ParseText walks textToParse left to right, sends every Matexe tag
// (<MAT>/<EVAL>/<HIDE>/<PLOT>/<TEX>/<TIKZ>/<SISTEMA>) it finds to Maxima in
// order, and splices the result back in. Ports Tcas.parse_text +
// Tcas.processTag.
func (m *Maxima) ParseText(text string) (string, error) {
	matches := tagPattern.FindAllStringSubmatchIndex(text, -1)
	var out strings.Builder
	last := 0

	for _, idx := range matches {
		out.WriteString(text[last:idx[0]])
		last = idx[1]

		kind, content := tagEnMatch(text, idx)
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}

		repl, err := m.processTag(kind, content)
		if err != nil {
			return "", fmt.Errorf("<%s>%s</%s>: %w", kind, content, kind, err)
		}
		out.WriteString(repl)
	}
	out.WriteString(text[last:])
	return out.String(), nil
}

// tagEnMatch decodifica un match de tagPattern (grupos por parellas, na
// mesma orde ca a alternación) no seu nome de etiqueta e no seu contido cru.
// Está separado porque hai DOUS percorridos distintos sobre o mesmo patrón:
// ParseText (o do documento) e expandirTagsTikz (o de dentro dun debuxo).
func tagEnMatch(text string, idx []int) (kind, content string) {
	for i, nome := range []string{"MAT", "EVAL", "HIDE", "PLOT", "TEX", "TIKZ", "SISTEMA", "RESP", "SOL"} {
		if g := 2 + 2*i; idx[g] != -1 {
			return nome, text[idx[g]:idx[g+1]]
		}
	}
	return "", ""
}

// processTag ports Tcas.process_MAT / process_EVAL / process_HIDE /
// process_PLOT / process_TEX.
func (m *Maxima) processTag(kind, content string) (string, error) {
	switch kind {
	case "MAT":
		content = stripNestedValueTags(content)
		content = normalizeLatexSubscripts(content)
		if sym, ok := specialCaseSymbol(content); ok {
			return m.renderMathSymbol(sym), nil
		}
		if txt, ok := mathProseAsText(content); ok {
			return m.renderLiteralText(txt), nil
		}
		if expr, ok := pureNumericArithmetic(content); ok {
			return m.renderMathSymbol(expr), nil
		}
		// Dentro dun <RESP>/<SOL>, unha fórmula alxébrica cos nomes das
		// variables (V_1/R_1, F/m, (1/2)*m*v^2...) cítase para que se vexa
		// coma fórmula e non coma o número de substituír os valores. Ver
		// autoQuoteFormula e Maxima.enCorreccion.
		if m.enCorreccion {
			content = autoQuoteFormula(content)
		}
		// Unevaluated, un-simplified expression -> TeX.
		return m.ToTeX(autoQuoteLimit(content), false)
	case "EVAL":
		content = stripNestedValueTags(content)
		content = normalizeLatexSubscripts(content)
		if sym, ok := specialCaseSymbol(content); ok {
			return m.renderMathSymbol(sym), nil
		}
		if txt, ok := mathProseAsText(content); ok {
			return m.renderLiteralText(txt), nil
		}
		// Evaluated expression -> TeX.
		return m.ToTeX(content, true)
	case "HIDE":
		content = normalizeLatexSubscripts(content)
		// Executed silently in the CAS session, nothing shown - but a
		// Maxima-level error (bad syntax, unknown option...) must still
		// surface: silently swallowing it would leave later tags that
		// depend on this one's variables failing in a much more confusing
		// way (or worse, plotting/showing stale values from a previous
		// run). See maximaErrorFrom.
		raw, err := m.Send(content, true)
		if err != nil {
			return "", err
		}
		if msg := maximaErrorFrom(raw); msg != "" {
			return "", fmt.Errorf("erro de Maxima: %s", msg)
		}
		return "", nil
	case "PLOT":
		return m.plot(content)
	case "TEX":
		// Content may itself contain tags (parsed first), then wrapped
		// as raw TeX like Tcas.CAS_TeX. It's already valid TeX source
		// either way, so both formats just wrap it as inline math.
		inner, err := m.ParseText(content)
		if err != nil {
			return "", err
		}
		if m.Format == FormatLatex {
			return m.protect("$" + inner + "$"), nil
		}
		return fmt.Sprintf(`<span class="math">\(%s\)</span>`, inner), nil
	case "TIKZ":
		// Malia que a chuleta da IA prohibe expresamente meter etiquetas
		// dentro dun <TIKZ> (ver chuletaMatexe en ia_client.go), a IA faino
		// igual sempre que o debuxo ten que amosar os valores aleatorios do
		// exercicio ("$R_1 = <EVAL>r_1</EVAL>\,\Omega$" nunha etiqueta de
		// nodo). Antes ía tal cal a pdflatex e o esquema saía co literal
		// "<EVAL>...</EVAL>" impreso e superposto (report real). Resólvense
		// aquí a TeX en bruto ANTES de compilar; ademais de arranxar o
		// debuxo, isto fai que un circuíto poida levar de verdade os valores
		// aleatorios da variante.
		code, err := m.expandirTagsTikz(content)
		if err != nil {
			return m.tikzAviso("Non se puido debuxar o esquema TikZ: " + err.Error()), nil
		}
		return m.tikz(code)
	case "SISTEMA":
		return m.renderSistema(content)
	case "RESP", "SOL":
		// Etiquetas de corrección da interface de táboa. Se o RenderMode
		// actual non as amosa, descártanse SEN mandar nada a Maxima (efecto
		// cero, coma se non estivesen). Se as amosa, o interior pode levar
		// á súa vez <EVAL>/<MAT>/<HIDE>... así que se procesa recursivamente
		// igual ca <TEX>.
		if !m.mostrarCorreccion(kind) {
			return "", nil
		}
		prev := m.enCorreccion
		m.enCorreccion = true
		defer func() { m.enCorreccion = prev }()
		return m.ParseText(content)
	default:
		return "", fmt.Errorf("unsupported tag <%s>", kind)
	}
}

// renderSistema groups several equations (one per line, plain Maxima
// source - no individual <MAT> wrapper needed) under one shared brace, real
// report: three separate <MAT>eq</MAT> tags, each in its own <p>, render as
// three unrelated lines instead of the "system of equations" notation
// (brace + stacked equations) a maths teacher expects. amsmath's "cases"
// environment already draws exactly that (left brace, left-aligned rows,
// no equation numbering) - loaded in both LaTeX preambles (see
// latexPreambleTemplatePDF/Unicode in latexdoc.go) and understood natively
// by MathJax (HTML preview) and by Pandoc's LaTeX-math reader (Markdown ->
// DOCX/ODT, see docdoc.go), so one wrapping covers every export format.
// Each line goes through toTeXValue the same way a lone <MAT> would
// (unevaluated - a system is shown as posed, not solved) - kept separate
// from MAT's own specialCaseSymbol/mathProseAsText short-circuits since
// those exist for edge cases (a bare "<MAT>>=</MAT>", interval notation...)
// that don't apply to actual equations.
func (m *Maxima) renderSistema(content string) (string, error) {
	var rows []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = normalizeLatexSubscripts(line)
		value, hasMath, err := m.toTeXValue(autoQuoteLimit(line), false)
		if err != nil {
			return "", err
		}
		if !hasMath {
			// Maxima-level error on this one line (bad syntax...) - surface
			// it the same way ParseText surfaces any other tag error,
			// instead of silently dropping the row from the system.
			return "", fmt.Errorf("erro de Maxima en %q: %s", line, value)
		}
		rows = append(rows, value)
	}
	if len(rows) == 0 {
		return "", nil
	}
	inner := "\\begin{cases}\n" + strings.Join(rows, " \\\\\n") + "\n\\end{cases}"

	if m.Format == FormatLatex || m.Format == FormatMarkdown {
		return m.protect("$$" + inner + "$$"), nil
	}
	return `<span class="math">\[` + inner + `\]</span>`, nil
}

// bareRelationalSymbols catches the single most common mistake found
// empirically in IA-generated (and occasionally hand-typed) content: a bare
// comparison operator, no operands, wrapped in <MAT>/<EVAL> to reference it
// in prose (e.g. "a desigualdade ten <MAT>>=</MAT>"). Maxima's parser
// always rejects this ("is not a prefix operator") since there's nothing to
// evaluate - it was never a Maxima expression to begin with, just notation.
// Explicitly forbidding it in the IA's system prompt (see gemini_client.go's
// chuletaMatexe) didn't reliably stop it from recurring, so it's handled
// here directly instead: recognised before ever reaching Maxima and
// rendered as the plain math symbol, regardless of where the mistake came
// from.
var bareRelationalSymbols = map[string]string{
	"=": "=", "==": "=",
	"!=": `\neq`, "#": `\neq`, "≠": `\neq`,
	"<": "<", ">": ">",
	"<=": `\leq`, "≤": `\leq`,
	">=": `\geq`, "≥": `\geq`,
}

// bareRelationalSymbol reports whether content (already trimmed by the
// caller, ParseText) is exactly one of bareRelationalSymbols - nothing
// else, no operands - and if so returns its LaTeX math rendering.
func bareRelationalSymbol(content string) (string, bool) {
	sym, ok := bareRelationalSymbols[content]
	return sym, ok
}

// intervalRe matches "(A, B)" / "[A, B]" / "(A, B]" / "[A, B)" - interval
// notation for a solution set, another very common IA/hand-typed mistake
// wrapped in <MAT>/<EVAL> (e.g. "<MAT>(-inf, -2]</MAT>"). Bounds must be
// paren/bracket-free (no nested "f(x)"-style calls) - anything more complex
// falls through to the normal Maxima path unchanged, same as before.
var intervalRe = regexp.MustCompile(`^([(\[])\s*([^,()\[\]]+?)\s*,\s*([^,()\[\]]+?)\s*([)\]])$`)

// infinityWordRe recognises an interval bound that's some spelling of
// infinity ("inf", "-inf", "+infinity", "∞", "-∞"...) - the strongest
// signal that a "(A, B)"-shaped tag is interval notation and not a
// legitimate two-statement Maxima expression (which round parens with a
// comma otherwise are: valid syntax, but SILENTLY evaluates to just B,
// discarding A - confirmed empirically with `tex((3, inf))` printing only
// "∞" - worse than a visible error, so this is intercepted proactively
// rather than only reacting to a parse failure like bareRelationalSymbol).
var infinityWordRe = regexp.MustCompile(`(?i)^([+\-−]?)\s*(?:inf|infinity|∞)$`)

// intervalNotationSymbol reports whether content is interval notation (see
// intervalRe) that Maxima would either reject outright (mismatched
// bracket types - never valid Maxima syntax) or silently misinterpret
// (matched round parens - only intercepted when a bound is recognisably
// "infinity", to avoid ever touching a real two-statement Maxima
// expression a teacher intentionally wrote). "[A, B]" alone (matched
// square brackets, no infinity) is left untouched: that's already a valid
// Maxima list, and Maxima's own tex([A,B]) renders it exactly like a
// closed interval anyway.
func intervalNotationSymbol(content string) (string, bool) {
	mm := intervalRe.FindStringSubmatch(content)
	if mm == nil {
		return "", false
	}
	open, lower, upper, closeCh := mm[1], mm[2], mm[3], mm[4]
	mismatched := (open == "(" && closeCh == "]") || (open == "[" && closeCh == ")")
	if !mismatched {
		if !(open == "(" && closeCh == ")") {
			return "", false // "[A, B]" (matched square brackets): already valid Maxima, leave it
		}
		if !infinityWordRe.MatchString(strings.TrimSpace(lower)) && !infinityWordRe.MatchString(strings.TrimSpace(upper)) {
			return "", false // matched "(A, B)", neither bound is infinity - leave Maxima to handle it
		}
	}
	return open + renderIntervalBound(lower) + ", " + renderIntervalBound(upper) + closeCh, true
}

// renderIntervalBound renders a single interval bound as LaTeX math: an
// infinity spelling becomes \infty (with its sign, if any), anything else
// passes through as-is - bounds here are expected to already be simple
// literals ("-2", "3"), not expressions needing further evaluation.
func renderIntervalBound(bound string) string {
	bound = strings.TrimSpace(bound)
	if mm := infinityWordRe.FindStringSubmatch(bound); mm != nil {
		return mm[1] + `\infty`
	}
	return bound
}

// nestedValueTagRe matches a Matexe VALUE tag (<MAT>/<EVAL>) left nested
// INSIDE another <MAT>/<EVAL>'s content. ParseText never recurses into a
// <MAT>/<EVAL> body (unlike <TIKZ>/<TEX>/<RESP>/<SOL>), so such a tag would
// otherwise be shipped verbatim to Maxima and fail as "incorrect syntax: <
// is not a prefix operator" - real report from a matrix exercise built in
// the table editor, where "{a}" tokens inside "<MAT>A = matrix([{a},
// {b}], ...)</MAT>" expand to "<EVAL>a</EVAL>". Both tags just carry a plain
// Maxima sub-expression, which inside another expression is exactly what
// should be spliced in raw, so the wrapper is unwrapped to its content -
// the bare symbol then evaluates against the session's own <HIDE> bindings
// like any other. The frontend (table-serialize.js) already avoids emitting
// these; this is the backend safety net for hand-written / block-editor /
// legacy .matex.
var nestedValueTagRe = regexp.MustCompile(`(?s)<(?:MAT|EVAL)>(.*?)</(?:MAT|EVAL)>`)

// stripNestedValueTags unwraps every nested <MAT>/<EVAL> in s to its inner
// content (see nestedValueTagRe), repeating until stable so a double
// nesting ("<EVAL><MAT>x</MAT></EVAL>") collapses too. Bounded iteration so
// a pathological input can't spin here.
func stripNestedValueTags(s string) string {
	for i := 0; i < 5 && nestedValueTagRe.MatchString(s); i++ {
		s = nestedValueTagRe.ReplaceAllString(s, "$1")
	}
	return s
}

// latexSubscriptRe matches LaTeX-style subscript braces on a plain
// alphanumeric subscript: "R_{eq}", "V_{1}", "I_{total}". Real report:
// <MAT>R_{eq}</MAT> (from prose like "la resistencia equivalente R_{eq}")
// fails with Maxima's "incorrect syntax: { is not an infix operator" -
// unlike LaTeX, Maxima's "{" always opens a SET literal, never a grouping
// for "_", so "R_{eq}" parses as the identifier "R_" immediately followed
// by the set "{eq}" with nothing joining them. Maxima's own identifier
// syntax already allows the underscore directly ("R_eq" is one valid
// symbol name, and Maxima's tex() renders it back with a real LaTeX
// subscript, "R_{eq}") - so the fix is to normalize the braces away before
// the content ever reaches Maxima, not to special-case it out to plain
// text like bareRelationalSymbol/intervalNotationSymbol: a normalized
// "R_eq" is still a real, evaluable Maxima expression (e.g. "R_{eq} =
// R_{1} + R_{2}" keeps working as an equation, not just a lone label).
// Left deliberately narrow to a single alphanumeric run: a comma or
// operator inside the braces ("x_{i,j}", "x_{i+1}") is genuine multi-level
// LaTeX notation Maxima has no equivalent for anyway, so it's left
// untouched and falls through to the same syntax error as before (not a
// regression - just not newly fixed).
var latexSubscriptRe = regexp.MustCompile(`_\{([A-Za-z0-9]+)\}`)

// normalizeLatexSubscripts strips LaTeX subscript braces (see
// latexSubscriptRe) from MAT/EVAL/HIDE content - the three tag kinds whose
// content is Maxima source. Left out of PLOT/TEX/TIKZ: PLOT content can
// contain quoted string literals (plot titles/legends) where blindly
// rewriting "_{...}" would silently change the displayed text instead of
// fixing a syntax error, and TEX/TIKZ content is raw LaTeX/TikZ source
// where "_{...}" is already correct, valid syntax that must not be touched.
func normalizeLatexSubscripts(content string) string {
	return latexSubscriptRe.ReplaceAllString(content, "_$1")
}

// limitCallRe matches a bare (unquoted) limit(...) call at the very start
// of a <MAT> expression.
var limitCallRe = regexp.MustCompile(`^limit\s*\(`)

// autoQuoteLimit prefixes a bare limit(...) call with a quote (') so Maxima
// treats it as an inert "noun" form instead of evaluating it immediately.
// Real report: <MAT>limit(sqrt(x^2+6*x)-x,x,inf)</MAT> rendered as a bare
// "3" instead of the limit notation - <MAT>'s own promise ("Mostra a
// expresión SEN avaliar", the tagbar tooltip) doesn't hold for limit():
// Maxima executes any unquoted function call as soon as it parses it,
// regardless of ToTeX's forceEval - only the noun-form quote keeps it
// symbolic, and tex() already knows how to typeset a quoted limit as
// \lim_{x\to a}{...} (verified against real Maxima output). Deliberately
// scoped to limit() only, not diff/integrate/sum/product: those normally
// SHOULD show their computed formula inside <MAT> (e.g. diff(x^2,x) -> "2x"
// is exactly what's wanted) - limit() is the one case that visibly
// contradicts <MAT>'s own contract by collapsing to a lone number. A
// professor who already writes 'limit(...) by hand is unaffected (the
// leading quote means the string no longer starts with "limit(").
func autoQuoteLimit(content string) string {
	if limitCallRe.MatchString(content) {
		return "'" + content
	}
	return content
}

// autoQuoteFormula cita ('( ... )') unha fórmula alxébrica SÓ CON NOMES DE
// VARIABLES e operadores, para que <MAT> a amose coma fórmula e non coma o
// número que sae de substituír os valores aleatorios do exercicio
// (tex(V_1/R_1) -> "16.67"; tex('(V_1/R_1)) -> "V_1/R_1"). Report habitual ao
// xerar boletíns: a resolución da IA di "<MAT>V_1/R_1</MAT> = <EVAL>res_1</EVAL>"
// e no PDF saía "16,67 = 16,67".
//
// Sae SEN TOCAR (comportamento actual) se: xa vai citada; ten unha chamada a
// función ("nome(" - diff/sqrt/matrix/... SI deben calcular dentro de <MAT>);
// ten algo que non sexa identificador/número/operador/paréntese; non ten
// ningún operador (un identificador solo non colapsa a nada); ou non ten
// ningunha letra (só números -> xa o colle pureNumericArithmetic).
var (
	chamadaFuncionRe    = regexp.MustCompile(`[A-Za-z_]\w*\s*\(`)
	formulaAlxebraicaRe = regexp.MustCompile(`^[A-Za-z0-9_.\s+\-*/^()]+$`)
	temLetraRe          = regexp.MustCompile(`[A-Za-z]`)
)

func autoQuoteFormula(content string) string {
	t := strings.TrimSpace(content)
	if t == "" || strings.HasPrefix(t, "'") {
		return content
	}
	if chamadaFuncionRe.MatchString(t) {
		return content
	}
	if !formulaAlxebraicaRe.MatchString(t) {
		return content
	}
	if !strings.ContainsAny(t, "+-*/^") || !temLetraRe.MatchString(t) {
		return content
	}
	return "'(" + t + ")"
}

// pureNumericArithmeticRe matches a chain of plain number literals joined by
// +, -, * or ^, no variables (e.g. "548 + 375", "12.5 - 3 + 1", "2^3 * 3",
// "2^4 * 3^2 * 5" - a prime factorisation, as posed by a factor-decomposition
// exercise). Originally +/- only; widened after a second real report:
// <MAT>48 = 2^4 * 3</MAT> printed "48 = 48" instead of the factorisation as
// written, same root cause as the first report below applied to * and ^ too.
// Real report #1: <MAT>548 + 375</MAT> printed "923" instead of the sum as
// written - unlike limit() (autoQuoteLimit), where quoting the call keeps it
// symbolic, numeric-literal arithmetic is collapsed by Maxima's SIMPLIFIER
// at parse time, before "evaluation" (what a leading quote blocks) even
// applies - confirmed empirically: tex('(548+375)) still prints "923". A
// single literal ("923" alone) deliberately falls through to the normal
// Maxima/ToTeX path unchanged (see pureNumericArithmeticHasOperator) -
// there's nothing to "un-simplify" and no operator to strip the quote's
// effect from.
var pureNumericArithmeticRe = regexp.MustCompile(`^-?\d+(?:\.\d+)?(?:\s*[+\-*^]\s*-?\d+(?:\.\d+)?)*$`)

// pureNumericArithmeticHasOperator reports whether s (already matched by
// pureNumericArithmeticRe) contains an actual operator, as opposed to being
// a single bare literal - needed because pureNumericArithmeticRe alone must
// also accept a bare literal now (a "=" side like the "48" in
// "48 = 2^4 * 3" is legitimately just one literal). Skips index 0: a leading
// "-" there is a sign, not an operator.
func pureNumericArithmeticHasOperator(s string) bool {
	if s == "" {
		return false
	}
	return strings.ContainsAny(s[1:], "+-*^")
}

// exponentDigitsRe wraps an exponent in braces for LaTeX: bare "2^34" would
// otherwise typeset as "3" superscript followed by a baseline "4" (LaTeX's
// "^" only grabs the single next token) - "2^{34}" is what's actually
// meant. Single-digit exponents ("2^3") already render fine without braces,
// but bracing them too is harmless, so this applies unconditionally.
var exponentDigitsRe = regexp.MustCompile(`\^(-?\d+(?:\.\d+)?)`)

// renderPureNumericArithmetic turns a matched expression into LaTeX source:
// braces multi-digit exponents (exponentDigitsRe) and swaps "*" for "\cdot"
// (the conventional symbol for a factor decomposition, e.g. "2^3 \cdot 3"
// instead of a bare asterisk) - digits/+/-/./whitespace need no escaping.
func renderPureNumericArithmetic(expr string) string {
	expr = exponentDigitsRe.ReplaceAllString(expr, `^{$1}`)
	return strings.ReplaceAll(expr, "*", `\cdot`)
}

// pureNumericArithmetic reports whether content (already trimmed by the
// caller, ParseText) is pure numeric arithmetic (see pureNumericArithmeticRe)
// - either standalone, or as one "=" comparing two such sides (e.g.
// "48 = 2^4 * 3", a factorisation as posed) - returning its LaTeX rendering
// (see renderPureNumericArithmetic). MAT-only (see processTag): EVAL must
// keep evaluating, that's its whole point, so this is never consulted from
// the EVAL branch.
func pureNumericArithmetic(content string) (string, bool) {
	if parts := strings.SplitN(content, "=", 2); len(parts) == 2 {
		lhs, rhs := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if !pureNumericArithmeticRe.MatchString(lhs) || !pureNumericArithmeticRe.MatchString(rhs) {
			return "", false
		}
		if !pureNumericArithmeticHasOperator(lhs) && !pureNumericArithmeticHasOperator(rhs) {
			return "", false // "5 = 5": no factorisation on either side, nothing to preserve
		}
		return renderPureNumericArithmetic(lhs) + " = " + renderPureNumericArithmetic(rhs), true
	}
	if !pureNumericArithmeticRe.MatchString(content) || !pureNumericArithmeticHasOperator(content) {
		return "", false
	}
	return renderPureNumericArithmetic(content), true
}

// specialCaseSymbol tries every known "this was never meant to be Maxima
// source" shape (see bareRelationalSymbol/intervalNotationSymbol) before a
// MAT/EVAL tag's content reaches Maxima at all.
func specialCaseSymbol(content string) (string, bool) {
	if sym, ok := bareRelationalSymbol(content); ok {
		return sym, true
	}
	return intervalNotationSymbol(content)
}

// renderMathSymbol wraps a symbol already known to be valid LaTeX math
// source (bareRelationalSymbol's output) the same way ToTeX wraps a
// successful Maxima result, without a round-trip through Maxima at all.
func (m *Maxima) renderMathSymbol(sym string) string {
	if m.Format == FormatLatex || m.Format == FormatMarkdown {
		return m.protect("$" + sym + "$")
	}
	return `<span class="math">\(` + sym + `\)</span>`
}

// countTopLevelEquals counts "=" characters that sit OUTSIDE any
// (), [], {} nesting - i.e. genuinely chaining/qualifying the whole
// expression, not glued inside a nested list or function-call argument. A
// valid multi-argument call like "ev(f(x),x=3,y=2)" keeps every "=" one
// level deep inside the outer "(...)", so it reports zero and is correctly
// left alone; "PM = [3 - 1, 0 - 2, 2 - 3] = [2, -2, -1]" (real report -
// note the "=" signs are top-level even though the content also has
// commas, just nested ones inside the "[...]" lists) reports 2. Doesn't
// bother distinguishing ">="/"<="/"!="/"==" from a bare "=" - a false
// positive here only means legitimate content that already uses one of
// those operators TWICE at the top level (rare) renders as plain text
// instead of being evaluated, not a wrong result or a crash.
func countTopLevelEquals(content string) int {
	depth := 0
	count := 0
	for _, r := range content {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case '=':
			if depth == 0 {
				count++
			}
		}
	}
	return count
}

// mathProseAsText catches content that reads as mathematical NOTATION
// rather than a single evaluable Maxima expression: a chain of relations
// (countTopLevelEquals >= 2, e.g. "(x-1)/2 = (y+1)/1 = z/2", the standard
// shorthand for a line's symmetric equations in 3D geometry), a label with
// a dangling "=" at the end (e.g. "dom(f) = ", the general form of the
// trailing-"=" rule already forbidden in gemini_client.go's chuletaMatexe -
// bareRelationalSymbol only catches a BARE operator with nothing else, this
// catches the same mistake with a real expression glued in front, which
// keeps recurring despite the explicit prompt rule), or a degree symbol
// anywhere in content (containsDegreeSymbol - polar-form notation like
// "2[60°]", never valid Maxima). Maxima's parser always rejects all three
// ("a=b=c" parses as "(a=b)=c", a LOGICAL comparison where an ALGEBRAIC
// value was expected; a trailing "=" is simply incomplete; "°" isn't a
// token Maxima's reader knows at all, "... is not an infix operator" or an
// outright "incorrect syntax"). None of the three has one single "correct"
// evaluation to fall back to the way bareRelationalSymbol/
// intervalNotationSymbol do, so all render as plain literal text instead -
// always safe, since it's exactly what the content would have looked like
// as ordinary prose outside any tag to begin with.
func mathProseAsText(content string) (string, bool) {
	if _, ok := bareRelationalSymbols[content]; ok {
		return "", false // xa ten a súa propia renderización (só o símbolo)
	}
	if containsDegreeSymbol(content) || strings.HasSuffix(content, "=") || countTopLevelEquals(content) >= 2 {
		return content, true
	}
	return "", false
}

// containsDegreeSymbol reports whether content contains the "°" (degree)
// character - never valid Maxima syntax (it's used nowhere else in this
// codebase except prose escaping, see EscapeLatexProse in escape.go, and
// Maxima's reader has no notion of a degree sign at all: "60°" tokenizes as
// the number 60 followed by an unrecognised character). Real report: an
// IA-generated worked solution for complex numbers in polar form wrote
// r[θ°]-style notation directly inside <MAT> ("z[1] = 2[60°]",
// "(sqrt(2))[135°]", "2^6[360°]") - Maxima has no polar-form literal syntax
// at all, this was always meant as notation to read, never something to
// evaluate, and choked with "incorrect syntax" / "° is not an infix
// operator" depending on where in the expression the "°" landed.
func containsDegreeSymbol(content string) bool {
	return strings.ContainsRune(content, '°')
}

// renderLiteralText renders content that never was (or turned out not to
// be) valid Maxima source as ordinary prose instead - same treatment as
// document text outside any tag (EscapeLatexProse: LaTeX-escaped, with the
// same Unicode math-symbol substitutions as escapeOutsideMath in
// latexdoc.go), just reached from inside a MAT/EVAL tag instead of from a
// text node.
func (m *Maxima) renderLiteralText(text string) string {
	if m.Format == FormatLatex || m.Format == FormatMarkdown {
		return m.protect(EscapeLatexProse(text))
	}
	return html.EscapeString(text)
}

// maximaErrorFrom detects a Maxima-level error in a Send response - Maxima
// reports these on its own console output ("draw: unknown option axes \n --
// an error. To debug this try: debugmode(true);"), not as a failed command,
// so Session.rawSend/Send see it as ordinary (successful) output. Returns
// the error message, or "" if the response doesn't look like an error.
//
// Two shapes are caught:
//   - a runtime error, ending in "-- an error." (everything before it);
//   - a PARSER rejection ("incorrect syntax: X is not an infix operator"),
//     which Maxima prints as plain reader output with NO "-- an error."
//     block. The common trigger is a title/label written with LaTeX-style
//     single quotes ([title, 'f(x) e f”(x)']) instead of a Maxima string
//     ([title, "f(x) e f'(x)"]): Maxima reads 'f(x) as quote(f(x)), then
//     trips on the next bare word. Without this it slips through and the
//     failure only surfaces later - a <PLOT> that never wrote its image, a
//     <HIDE> whose variables stayed unset.
func maximaErrorFrom(raw string) string {
	if idx := strings.Index(raw, "-- an error."); idx != -1 {
		return strings.TrimSpace(raw[:idx])
	}
	if idx := strings.Index(raw, "incorrect syntax"); idx != -1 {
		var out []string
		for _, l := range strings.Split(raw[idx:], "\n") {
			l = strings.TrimRight(l, " \r")
			if strings.TrimSpace(l) == "" || iLabelRe.MatchString(l) {
				break // stop at the blank line / next (%iN) prompt echo
			}
			out = append(out, l)
			if len(out) == 3 {
				break // message + offending fragment + caret is enough
			}
		}
		return strings.TrimSpace(strings.Join(out, "\n"))
	}
	return ""
}

// plot ports Tcas.process_PLOT + TMaxima.CAS_Plot: routes Maxima's
// draw2d/draw3d output through gnuplot and returns an <img>/\includegraphics/
// \input reference. LaTeX output for plot2d/plot3d uses gnuplot's "tikz"
// terminal (via its Lua script driver, confirmed present alongside this
// gnuplot/Maxima version) - real vector TikZ drawn with the DOCUMENT's own
// font/size, not gnuplot's own bitmap font, so axis numbers/labels read as
// part of the page instead of a pasted-in image (real complaint: Yang's
// generated plots "didn't look designed" next to hand-styled references).
// draw2d/draw3d can't target "tikz" (Maxima's draw package only allows a
// fixed terminal list - pdf/pdfcairo/epslatex/svg/... - confirmed by a real
// "illegal terminal specification: tikz" error) so those keep "pdf": still a
// real VECTOR file, just without the font-matching upgrade (out of scope
// here - epslatex would need an extra eps->pdf conversion step). HTML/
// Markdown previews still need a raster (no <img>/![]() PDF/TikZ support),
// so those keep "pngcairo" - gnuplot's modern anti-aliased PNG terminal.
// isDraw/ext/term/gnuplot_preamble all confirmed against this Maxima/gnuplot
// installation with direct `maxima -b` runs before wiring this in.

// wxPlotReplacer normaliza os nomes de wxMaxima (wxplot2d/wxdraw2d/…) ás
// súas versións "de verdade". As wx* aceptan a chamada sen queixarse pero
// están pensadas para a saída embebida de wxMaxima: IGNORAN
// gnuplot_out_file, así que non escriben ningún ficheiro e Maxima non
// devolve "-- an error." - o \input/\includegraphics que xera plot() abaixo
// apuntaría a algo que non existe e o fallo só saía moito máis tarde coma
// un críptico "File `images/img_N.tex' not found" de pdflatex/xelatex. Un
// profesor que copie o plot dende wxMaxima tráese o "wx" sen decatarse.
var wxPlotReplacer = strings.NewReplacer(
	"wxplot2d(", "plot2d(",
	"wxplot3d(", "plot3d(",
	"wxdraw2d(", "draw2d(",
	"wxdraw3d(", "draw3d(",
)

func (m *Maxima) plot(expr string) (string, error) {
	m.plotCount++
	expr = wxPlotReplacer.Replace(expr)
	isDraw := strings.Contains(expr, "draw2d(") || strings.Contains(expr, "draw3d(")
	ext, term := "png", "pngcairo"
	if m.Format == FormatLatex {
		if isDraw {
			ext, term = "pdf", "pdf"
		} else {
			ext, term = "tex", "tikz"
		}
	}
	filename := fmt.Sprintf("img_%d.%s", m.plotCount, ext)
	imgDir := filepath.Join(m.outDir, "images")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return "", err
	}
	fullPath := filepath.ToSlash(filepath.Join(imgDir, filename))
	baseNoExt := strings.TrimSuffix(fullPath, "."+ext)

	for _, opt := range []string{
		`set_plot_option([plot_format, gnuplot])`,
		fmt.Sprintf(`set_plot_option([gnuplot_term, %s])`, term),
		fmt.Sprintf(`set_plot_option([gnuplot_out_file,"%s"])`, fullPath),
		// Muted grid/border + the app's own accent as the default line
		// colour (see style.css --border/--border-strong/--primary) - a
		// gnuplot "set" command applied before drawing, same mechanism for
		// plot2d/plot3d/draw2d/draw3d and for every output format (raster
		// previews get the same restyle, not just the vector PDF/TikZ path).
		// "unset key" (no legend box): the reference look has none anyway,
		// and it removes one source of raw-LaTeX-breaking label text (see
		// ylabel/xlabel below).
		`set_plot_option([gnuplot_preamble, "set grid lc rgb '#e3e5e9' lw 0.5; set border lc rgb '#a8acb5' lw 0.8; set linetype 1 lc rgb '#5b4fe0' lw 2; unset key"])`,
		// plot2d/plot3d default the Y axis label to the plotted EXPRESSION
		// ITSELF (confirmed: plot2d(x^2+1,...) sets ylabel to "x^2+1") - with
		// the "tikz" terminal that text is written as a literal LaTeX \node,
		// not rasterised/vector-drawn like the old pdf/pdfcairo terminals
		// did, so "^"/"_" outside math mode is a hard pdflatex/xelatex error
		// ("Missing $ inserted") - a real failure hit in latexdoc_test.go
		// before adding these two. "unset ylabel" in gnuplot_preamble above
		// does NOT work (Maxima's own ylabel default is applied AFTER the
		// preamble, overriding it) - has to be these two plot_options
		// instead, confirmed by direct `maxima -b` testing.
		`set_plot_option([ylabel, ""])`,
		`set_plot_option([xlabel, ""])`,
	} {
		if _, err := m.Send(opt, false); err != nil {
			return "", err
		}
	}

	line := expr
	switch {
	case strings.Contains(line, "draw2d("):
		line = strings.Replace(line, "draw2d(",
			fmt.Sprintf(`draw2d(terminal='%s, file_name="%s",`, term, baseNoExt), 1)
	case strings.Contains(line, "draw3d("):
		line = strings.Replace(line, "draw3d(",
			fmt.Sprintf(`draw3d(terminal='%s, file_name="%s",`, term, baseNoExt), 1)
	}
	// Maxima answers a bad draw2d/draw3d call (e.g. an option that belongs to
	// plot2d, not draw2d - "unknown option axes") as plain error text on its
	// own console output, not as a Go error: Send only fails on a
	// communication problem. Left unchecked, no PNG ever gets written and
	// the failure only surfaces much later, as a baffling "file not found"
	// from pdflatex - check here instead, right where the real cause is.
	raw, err := m.Send(line, true)
	if err != nil {
		return "", err
	}
	if msg := maximaErrorFrom(raw); msg != "" {
		return "", fmt.Errorf("erro de Maxima ao debuxar: %s", msg)
	}

	// Rede de seguridade: se aquí non hai ficheiro, o plot non xerou nada
	// aínda que Maxima non se queixase (unha variante wx* que se nos
	// escapase, contour_plot/implicit_plot sen o seu load(), gnuplot que
	// fallou en silencio…). Fallar agora, coa expresión á vista, en vez de
	// devolver un \input/\includegraphics colgando que rompe a compilación
	// do PDF moito despois cun erro que non sinala a causa. O ramo HTML de
	// abaixo xa fai a súa propia comprobación vía os.ReadFile.
	if m.Format == FormatLatex || m.Format == FormatMarkdown {
		if _, statErr := os.Stat(fullPath); statErr != nil {
			return "", fmt.Errorf("a expresión de <PLOT> non xerou ningunha imaxe: %s", expr)
		}
	}

	if m.Format == FormatLatex {
		// LaTeX needs a real file on disk, referenced relative to the
		// .tex file being compiled - leave it where it is, in images/.
		if ext == "tex" {
			// TikZ output (plot2d/plot3d): \input, not \includegraphics -
			// gnuplot's tikz terminal already writes a full tikzpicture, and
			// \resizebox scales that vector content to the same width the
			// includegraphics branch below uses (\input alone has no width
			// option). Requires \usepackage{gnuplot-lua-tikz} in the
			// preamble (latexdoc.go) - defines \gpcolor/\gpsetlinetype/etc.
			return m.protect(fmt.Sprintf("\\begin{center}\\resizebox{0.55\\textwidth}{!}{\\input{images/%s}}\\end{center}", filename)), nil
		}
		return m.protect(fmt.Sprintf("\\begin{center}\\includegraphics[width=0.55\\textwidth]{images/%s}\\end{center}", filename)), nil
	}
	if m.Format == FormatMarkdown {
		// Markdown export: same idea as LaTeX (real file, referenced
		// relative to the .md file, already sitting in images/), just with
		// Markdown's own image syntax instead of \includegraphics. Alt text
		// is a plain fixed label, not expr: Maxima plot calls are full of
		// literal []/() that would break the ![alt](url) syntax if dropped
		// in raw (html.EscapeString, used by the HTML branch below, doesn't
		// help here - Markdown's special characters aren't HTML's).
		return m.protect(fmt.Sprintf("![gráfico](images/%s)", filename)), nil
	}

	// Embed the PNG directly so the resulting HTML is self-contained -
	// convenient for previewing in the webview without an asset server,
	// and for the final exported/printed document.
	png, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("reading generated plot %s: %w", fullPath, err)
	}
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	return fmt.Sprintf(`<img src="%s" alt="%s">`, dataURI, html.EscapeString(expr)), nil
}

// tikz ports the new <TIKZ> tag: raw TikZ source. In LaTeX mode it's
// wrapped in a tikzpicture environment INSIDE \begin{center}...\end{center}
// - matching plot()/renderImg() below, and not just cosmetic: a bare
// tikzpicture is valid INLINE LaTeX content (no implicit \par), so a
// diagram wedged between two blocks with no whitespace between them (the
// block editor concatenates blocks with none, see blocks-serialize.js)
// rendered mid-paragraph, at whatever height the drawing turned out to be,
// overlapping the surrounding text instead of getting its own line (real
// report). \begin{center}, being list-based (trivlist), forces \par both
// entering and leaving regardless of what's textually adjacent to it, the
// same guarantee plot()/renderImg() already relied on - now tikzpicture
// gets it too. The compiled document already loads the tikz package (see
// latexPreambleTemplate in latexdoc.go) so pdflatex renders it natively, no
// raster step needed. In HTML preview mode a browser can't render TikZ, so
// a throwaway standalone LaTeX doc containing only the tikzpicture is
// compiled with pdflatex and
// rasterized with pdftoppm (part of poppler-utils - much more commonly
// preinstalled than a full ImageMagick/Ghostscript chain), then embedded
// as a data: URI exactly like plot() does for gnuplot output above.
// Missing tools or a bad TikZ snippet degrade to a visible inline warning
// instead of aborting the whole generation - ParseText's caller has no
// other channel to surface a per-tag warning in HTML mode.
func (m *Maxima) tikz(code string) (string, error) {
	if m.Format == FormatLatex {
		// PREVOO: compílase o fragmento nun documento standalone á parte
		// (coas mesmas librarías ca o documento final). Se falla, NON se
		// insire no boletín - tumbaría o PDF ENTEIRO cun erro cru de
		// xelatex; en troques déixase unha caixa de aviso no seu sitio
		// (mesma filosofía ca o modo HTML aquí embaixo: un debuxo roto non
		// pode botar abaixo todo o exame). O fallo típico é contido da IA
		// que mestura sintaxe TikZ coas etiquetas <...>: "\node at (0,1)
		// <MAT>...", un \node sen "{...}", un compoñente circuitikz que non
		// existe...
		if commandExists("pdflatex") {
			if err := tikzCompilaProba(code); err != nil {
				return m.tikzAviso("Erro compilando o debuxo TikZ (revisa a sintaxe)."), nil
			}
		}
		// \begin{center} alone forces a \par on both sides, but that is not
		// enough for a circuitikz drawing (circuit symbols, node labels and
		// current arrows routinely stick out past the tikzpicture's nominal
		// bounding box), so the diagram still ended up printed on top of the
		// line above or below it (real report). Add an explicit vertical gap
		// on each side. \addvspace (not \vspace) so two drawings in a row, or
		// a drawing next to another centred block, don't stack a double gap.
		return m.protect("\\par\\addvspace{\\baselineskip}%\n" +
			"\\begin{center}\\begin{tikzpicture}\n" + code + "\n\\end{tikzpicture}\\end{center}" +
			"\\par\\addvspace{\\baselineskip}%\n"), nil
	}
	if !commandExists("pdflatex") {
		return m.tikzAviso("Non se puido xerar o debuxo TikZ: non se atopou pdflatex."), nil
	}
	if !commandExists("pdftoppm") {
		return m.tikzAviso("Non se puido xerar o debuxo TikZ: non se atopou pdftoppm (paquete poppler-utils)."), nil
	}

	tmpDir, err := os.MkdirTemp("", "matexe-tikz-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	doc := tikzDocStandalone(code)
	if err := os.WriteFile(filepath.Join(tmpDir, "doc.tex"), []byte(doc), 0o644); err != nil {
		return "", err
	}

	cmd := exec.Command("pdflatex", "-interaction=nonstopmode", "-halt-on-error", "doc.tex")
	cmd.Dir = tmpDir
	ocultarConsola(cmd)
	if err := cmd.Run(); err != nil {
		return m.tikzAviso("Erro compilando o debuxo TikZ (revisa a sintaxe)."), nil
	}

	pngPath := filepath.Join(tmpDir, "doc.png")
	conv := exec.Command("pdftoppm", "-png", "-r", "150", "-singlefile",
		filepath.Join(tmpDir, "doc.pdf"), filepath.Join(tmpDir, "doc"))
	ocultarConsola(conv)
	if out, err := conv.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pdftoppm: %w: %s", err, out)
	}

	png, err := os.ReadFile(pngPath)
	if err != nil {
		return "", fmt.Errorf("reading generated tikz png %s: %w", pngPath, err)
	}

	if m.Format == FormatMarkdown {
		// Same treatment as plot(): a real file in images/, referenced by
		// relative path, instead of an embedded data: URI. Shares
		// m.plotCount with plot() so filenames never collide when a
		// document mixes <PLOT> and <TIKZ> blocks.
		m.plotCount++
		filename := fmt.Sprintf("img_%d.png", m.plotCount)
		imgDir := filepath.Join(m.outDir, "images")
		if err := os.MkdirAll(imgDir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(imgDir, filename), png, 0o644); err != nil {
			return "", fmt.Errorf("writing tikz png %s: %w", filename, err)
		}
		return m.protect(fmt.Sprintf("![tikz](images/%s)", filename)), nil
	}

	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	return fmt.Sprintf(`<img src="%s" alt="tikz">`, dataURI), nil
}

// avisoTikzLatex é a caixa que substitúe un <TIKZ> que non compila, para que
// o resto do boletín si saia. Vai centrada e cos mesmos \par\addvspace ca o
// debuxo real, para non descolocar o texto de arredor.
const avisoTikzLatex = "\\par\\addvspace{\\baselineskip}%\n" +
	"\\begin{center}\\fbox{\\parbox{0.8\\linewidth}{\\centering\\small " +
	"\\textbf{Aviso:} non se puido debuxar o esquema deste exercicio " +
	"(revisa a sintaxe TikZ).}}\\end{center}" +
	"\\par\\addvspace{\\baselineskip}%\n"

// tikzChkCache: o mesmo fragmento TikZ compílase igual en todas as variantes
// (o contido de <TIKZ> non pasa por Maxima, é literal), así que o resultado
// do prevoo cachéase pola sinatura do código - un só pdflatex por debuxo
// distinto, non un por variante. Acoutado para non medrar sen fin nun
// proceso longo.
var (
	tikzChkMu    sync.Mutex
	tikzChkCache = map[string]bool{} // sha1(code) -> compila

	tikzBaseOnce sync.Once
	tikzBaseOK   bool // un debuxo trivial compila neste equipo co mesmo preámbulo
)

// tikzPreamboloUsable comproba UNHA VEZ que un tikzpicture trivial compila co
// preámbulo do prevoo (tikz + circuitikz). Se non (falta circuitikz, TeX
// roto...), o prevoo non pode xulgar nada: tikzCompilaProba pasa todo coma
// bo, e vólvese ao comportamento de antes (o erro cru de LaTeX).
func tikzPreamboloUsable() bool {
	tikzBaseOnce.Do(func() {
		tikzBaseOK = tikzCompilaProbaReal(`\draw (0,0) -- (1,1);`) == nil
	})
	return tikzBaseOK
}

func tikzCompilaProba(code string) error {
	if !tikzPreamboloUsable() {
		return nil
	}

	key := fmt.Sprintf("%x", sha1.Sum([]byte(code)))
	tikzChkMu.Lock()
	if ok, hai := tikzChkCache[key]; hai {
		tikzChkMu.Unlock()
		if ok {
			return nil
		}
		return fmt.Errorf("o fragmento TikZ non compila")
	}
	tikzChkMu.Unlock()

	err := tikzCompilaProbaReal(code)

	tikzChkMu.Lock()
	if len(tikzChkCache) >= 256 {
		tikzChkCache = map[string]bool{}
	}
	tikzChkCache[key] = err == nil
	tikzChkMu.Unlock()
	return err
}

func tikzCompilaProbaReal(code string) error {
	tmpDir, err := os.MkdirTemp("", "matexe-tikzchk-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	doc := tikzDocStandalone(code)
	if err := os.WriteFile(filepath.Join(tmpDir, "chk.tex"), []byte(doc), 0o644); err != nil {
		return err
	}
	cmd := exec.Command("pdflatex", "-interaction=nonstopmode", "-halt-on-error", "chk.tex")
	cmd.Dir = tmpDir
	ocultarConsola(cmd)
	return cmd.Run()
}

// DeclaracionsUnicode recibe (dende o paquete principal, ver latexdoc.go) o
// mesmo bloque de \DeclareUnicodeCharacter que leva o preámbulo do boletín,
// para que o documento standalone de tikzDocStandalone acepte exactamente os
// mesmos símbolos Unicode soltos ca o documento final. Se queda baleiro
// (tests do paquete cas illados) só se perde ese extra: o resto do preámbulo
// vai igual.
var DeclaracionsUnicode string

// tikzDocStandalone monta o documento dun só debuxo que se usa DÚAS veces:
// para rasterizar o <TIKZ> na vista previa HTML, e para o prevoo que decide
// se o fragmento entra no boletín LaTeX ou se substitúe por avisoTikzLatex.
//
// CRÍTICO: os paquetes teñen que ser os MESMOS que os do preámbulo real
// (latexPreambleTemplatePDF/Unicode en latexdoc.go). Un preámbulo máis pobre
// aquí non "protexe" nada: fai que o prevoo rexeite debuxos perfectamente
// válidos no boletín. Caso real: un esquema de circuíto con
// "\node[left] at (-0.3,1.5) {$24\text{ V}}$}" fallaba con "Undefined control
// sequence" porque faltaba amsmath (de onde vén \text), e o exercicio - que
// antes saía ben - pasou a amosar a caixa de aviso.
func tikzDocStandalone(code string) string {
	return "\\documentclass[border=2pt]{standalone}\n" +
		"\\usepackage[utf8]{inputenc}\n" +
		"\\usepackage[T1]{fontenc}\n" +
		DeclaracionsUnicode +
		"\\usepackage{amsmath,amssymb}\n" +
		"\\usepackage{graphicx}\n" +
		"\\usepackage{tikz}\n" +
		"\\usepackage{circuitikz}\n" +
		"\\usepackage{xcolor}\n" +
		"\\begin{document}\n" +
		"\\begin{tikzpicture}\n" + code + "\n\\end{tikzpicture}\n" +
		"\\end{document}\n"
}

// tikzAviso é a caixa/parágrafo que ocupa o sitio dun <TIKZ> que non se pode
// debuxar. Filosofía de sempre: un debuxo roto NON pode botar abaixo o exame
// enteiro, así que nunca devolve erro - só marca o oco.
func (m *Maxima) tikzAviso(motivo string) string {
	if m.Format == FormatLatex {
		return m.protect(avisoTikzLatex)
	}
	return `<p class="matexe-warning">⚠ ` + html.EscapeString(motivo) + `</p>`
}

// expandirTagsTikz resolve as etiquetas Matexe que a IA deixa DENTRO dun
// <TIKZ> antes de que o fragmento chegue a pdflatex. Sen isto o código ía
// literal ao debuxo e saía impreso "<EVAL>r_1</EVAL>" no medio do esquema.
//
//   - <MAT>/<EVAL> -> o TeX en bruto, envolto en \ensuremath{...} para que
//     valla igual dentro dun "{$...$}" ca nunha etiqueta de nodo en modo
//     texto ("{<EVAL>r_1</EVAL> ohmios}").
//   - <HIDE> -> execútase (pode definir as variables que usan os <EVAL> de
//     máis adiante) e non escribe nada.
//   - <TEX> -> o seu contido é xa LaTeX; expándese recursivamente e insírese
//     sen envolver.
//   - <PLOT>/<TIKZ>/<SISTEMA>/<RESP>/<SOL> -> descártanse: unha imaxe, un
//     debuxo aniñado ou un bloque "cases" non caben dentro dun tikzpicture,
//     e deixalos pasar tal cal era xustamente o que rompía a compilación.
func (m *Maxima) expandirTagsTikz(code string) (string, error) {
	matches := tagPattern.FindAllStringSubmatchIndex(code, -1)
	if len(matches) == 0 {
		// Camiño normal (TikZ puro): nin unha soa chamada a Maxima, e o
		// fragmento sae byte a byte igual ca antes deste cambio.
		return code, nil
	}

	var out strings.Builder
	last := 0
	for _, idx := range matches {
		out.WriteString(code[last:idx[0]])
		last = idx[1]

		kind, content := tagEnMatch(code, idx)
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}

		switch kind {
		case "MAT", "EVAL":
			tex, err := m.texEnBrutoTikz(kind, content)
			if err != nil {
				return "", fmt.Errorf("<%s>%s</%s>: %w", kind, content, kind, err)
			}
			out.WriteString(tex)
		case "HIDE":
			if _, err := m.processTag("HIDE", content); err != nil {
				return "", err
			}
		case "TEX":
			inner, err := m.expandirTagsTikz(content)
			if err != nil {
				return "", err
			}
			out.WriteString(inner)
		}
	}
	out.WriteString(code[last:])
	return out.String(), nil
}

// texEnBrutoTikz é o <MAT>/<EVAL> de processTag pero devolvendo o TeX SEN o
// envoltorio de páxina ("$...$" en LaTeX, "<span class=math>" en HTML), que
// dentro dunha etiqueta de nodo TikZ non pega. Mantén os mesmos atallos ca
// processTag (símbolo especial, prosa, aritmética puramente numérica) para
// que "<MAT>>=</MAT>" ou "<EVAL>2+3</EVAL>" se comporten igual aquí ca fóra.
func (m *Maxima) texEnBrutoTikz(kind, content string) (string, error) {
	content = normalizeLatexSubscripts(content)
	if sym, ok := specialCaseSymbol(content); ok {
		return "\\ensuremath{" + sym + "}", nil
	}
	if txt, ok := mathProseAsText(content); ok {
		return EscapeLatexProse(txt), nil
	}
	forceEval := kind == "EVAL"
	if !forceEval {
		if expr, ok := pureNumericArithmetic(content); ok {
			return "\\ensuremath{" + expr + "}", nil
		}
		if m.enCorreccion {
			content = autoQuoteFormula(content)
		}
		content = autoQuoteLimit(content)
	}

	value, hasMath, err := m.toTeXValue(content, forceEval)
	if err != nil {
		return "", err
	}
	if !hasMath {
		// Maxima devolveu texto de erro no canto dunha fórmula. Fóra dun
		// debuxo iso píntase en vermello no sitio; aquí non hai onde, e
		// meter o texto do erro no tikzpicture rompería a compilación - así
		// que sobe coma erro e o <TIKZ> enteiro pasa a ser a caixa de aviso.
		return "", fmt.Errorf("%s", collapseWhitespace(value))
	}
	return "\\ensuremath{" + strings.TrimSpace(value) + "}", nil
}
