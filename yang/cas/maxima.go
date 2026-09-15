package cas

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

var (
	oLabelRe = regexp.MustCompile(`\(%o\d+\)`)
	iLabelRe = regexp.MustCompile(`\(%i\d+\)`)
)

// OutputFormat picks how ToTeX/plot wrap their results: as HTML fragments
// for the MathJax preview, as raw LaTeX source for a .tex document, or as
// Markdown source for a .md export.
type OutputFormat int

const (
	FormatHTML     OutputFormat = iota // <span class="math">\(...\)</span>, <img src="data:...">
	FormatLatex                        // $...$, \includegraphics{...}
	FormatMarkdown                     // $...$ (same math as LaTeX), ![](images/...)
)

// Maxima wraps a Session with the higher level operations ported from
// TMaxima in maxima.pas: CAS_send and CAS_toTeX.
type Maxima struct {
	sess      *Session
	outDir    string
	plotCount int
	Format    OutputFormat

	// ProtectRaw, when set, marks a chunk this package already rendered as
	// final LaTeX ($...$, \includegraphics{...}) so a later HTML-aware
	// escaping pass over the surrounding document text leaves it alone
	// instead of mangling its backslashes/braces. Only meaningful when
	// Format == FormatLatex; nil is a safe no-op for FormatHTML callers.
	ProtectRaw func(string) string

	// Decimais, when > 0, makes ToTeX round every floating-point leaf of
	// the expression to this many decimal places before converting to TeX
	// (see matexeArredondarDef) - <= 0 leaves Maxima's own unlimited
	// display precision alone. Set via SetDecimais, never directly (it
	// needs to define the Maxima-side helper function first).
	Decimais int

	// ForzarDecimal, when true, makes ToTeX wrap EVERY <MAT>/<EVAL> result
	// in matexe_decimal(...) by default (same effect as a teacher calling
	// it explicitly on every line, see matexeDecimalDef) - the "Amosar
	// resultados en decimal" toggle in Options (Settings.Decimal,
	// settings.go). A line that already calls matexe_decimal or
	// matexe_fraccion explicitly is left alone (see matexeFraccionDef),
	// so a single exercise can still ask for the opposite of the
	// document's default. Plain Go field, no session round-trip needed -
	// unlike Decimais/SetDecimais it doesn't define anything Maxima-side
	// on its own, so it survives kill(all) with no extra handling.
	ForzarDecimal bool

	// RenderMode decide se <RESP> (resposta final) e <SOL> (solución
	// completa) se inclúen no resultado ou se descartan en silencio - é o
	// que fai que as lapelas da interface de táboa (Enunciados / +Resposta /
	// +Solución / Probas) xeren unha cousa ou outra co MESMO .matex. Valores:
	// Regra ESTRITA (pedida así): cada lapela amosa unha soa cousa ademais
	// do enunciado.
	//   ""/"enunciados" -> nin <RESP> nin <SOL> (folla para o alumnado)
	//   "resposta"      -> SÓ <RESP> (o resultado final de cada pregunta)
	//   "solucions"     -> SÓ <SOL> (a solución paso a paso, que xa chega ao
	//                      resultado; a <RESP> NON se repite á parte)
	//   "proba"         -> <RESP> e <SOL> (a lapela Test necesita avaliar
	//                      todo para detectar infinitos/indeterminacións)
	// Cando se descarta, NON se manda nada a Maxima: a etiqueta é como se
	// non estivese, sen efectos secundarios.
	RenderMode string

	// enCorreccion vale true SÓ mentres se procesa o contido dun <RESP> ou
	// <SOL> (as etiquetas de corrección da táboa). Nese contexto un <MAT> cos
	// nomes das variables do exercicio (p.ex. <MAT>V_1/R_1</MAT>) quérese ver
	// coma a FÓRMULA («V₁/R₁»), non coma o número que sae de substituír os
	// valores aleatorios - así que autoQuoteFormula cítao. Fóra de <RESP>/<SOL>
	// o comportamento de <MAT> non cambia. Restáurase sempre (defer).
	enCorreccion bool
}

// ProcessTag expón processTag para que a lapela "Test" (proba.go) avalíe
// cada etiqueta EXACTAMENTE polo mesmo camiño que a xeración real. Sen isto,
// probar cun m.Send() cru marcaba como erro cousas que cas renderiza como
// texto sen avaliar - unha cadea de igualdades "a = b = c", unha etiqueta
// rematada en "=", notación con "°"... (ver mathProseAsText).
func (m *Maxima) ProcessTag(kind, content string) (string, error) {
	return m.processTag(kind, content)
}

// mostrarCorreccion di se unha etiqueta de corrección (kind = "RESP" ou
// "SOL") debe renderizarse co RenderMode actual. Ver o campo RenderMode.
func (m *Maxima) mostrarCorreccion(kind string) bool {
	switch m.RenderMode {
	case "resposta":
		return kind == "RESP"
	case "solucions":
		return kind == "SOL"
	case "proba":
		return true
	default: // "", "enunciados"
		return false
	}
}

// matexeArredondarDef defines matexe_arredondar(e,n): recursively rounds
// every floating-point leaf of a Maxima expression to n decimal places,
// rebuilding the same operator/argument structure around it (equations,
// sums, lists, matrices...). Exact values (fractions, integers, symbols)
// are left untouched - only literal floats get rounded (32/9 stays exact,
// 3.555555555555556 becomes 3.56 for n=2) - which is exactly the case that
// needs fixing: a long machine-precision tail only ever shows up once a
// calculation touches a literal decimal somewhere in the chain.
const matexeArredondarDef = `matexe_arredondar(e,n):=if atom(e) then (if floatnump(e) then float(round(e*10^n)/10^n) else e) else apply(op(e),map(lambda([x],matexe_arredondar(x,n)),args(e)))$`

// matexeDecimalDef defines matexe_decimal(e): an explicit "quero decimal,
// non fracción exacta" escape hatch for a single <MAT>/<EVAL> expression.
// Recursively converts every NON-INTEGER exact rational subexpression to a
// float (23/20 -> 1.15) - but deliberately leaves plain integers alone (4
// stays 4, never 4.0): Maxima's own float() would flatten those too, which
// is exactly the report that prompted this ("<MAT>4</MAT> -> 4.0 nun PDF,
// queda raro" para calquera exercicio onde 4 é o resultado natural, non
// unha fracción que precisase converterse). Already-float leaves (4.5) and
// symbols pass through untouched either way.
//
// ratnump(e) is checked BEFORE atom(e), unlike matexeArredondarDef's
// floatnump check - not the same shape by accident: a float literal IS
// atomic in Maxima, so matexeArredondarDef's "atom(e) then check-and-round"
// naturally reaches it, but a non-integer RATIO is NOT atomic (23/20 is
// internally (23)*(20)^(-1), i.e. mtimes/mexpt of two small integers) - an
// atom-first version of this function would recurse straight past the
// ratio into its integer parts and rebuild it completely unchanged
// (found the hard way: matexe_decimal(230*5/1000) silently stayed 23/20).
// Testing ratnump(e) first catches the ratio as a whole, before recursing
// into pieces that individually look like plain integers.
//
// The N-decimal rounding on top comes for free: ToTeX already wraps EVERY
// <MAT>/<EVAL> result in matexe_arredondar(..., Decimais) below, which
// rounds floatnump leaves - exactly what matexe_decimal just produced.
//
// Deliberately NOT the default for every result (needs to be called
// explicitly, or via ForzarDecimal below): matexe_arredondar leaves exact
// fractions untouched on purpose (see its own comment) because some
// exercises need that - testdata/test.matex's "Simplificación de
// fraccións" is <MAT>a/b</MAT> = <EVAL>a/b</EVAL>, the whole point is the
// exact reduced fraction. matexe_decimal is opt-in, called explicitly only
// where a decimal is actually wanted, e.g. a physics worksheet:
// <MAT>matexe_decimal(230*5/1000)</MAT> -> 1.15, not 23/20 - and now
// <MAT>matexe_decimal(4)</MAT> stays 4, not 4.0.
const matexeDecimalDef = `matexe_decimal(e):=if ratnump(e) then (if integerp(e) then e else float(e)) elseif atom(e) then e else apply(op(e),map(lambda([x],matexe_decimal(x)),args(e)))$`

// matexeFraccionDef defines matexe_fraccion(e): the identity function -
// evaluating it does nothing at all. Its only job is as a MARKER that
// ToTeX's auto-wrap (see ForzarDecimal) recognises by name in the raw
// expression text and skips: when a document has "Amosar resultados en
// decimal" on but one specific exercise needs the exact fraction anyway
// (e.g. "Simplificación de fraccións"), the teacher wraps it explicitly -
// <MAT>matexe_fraccion(a/b)</MAT> - and that line is left alone instead of
// being auto-floated. Defined unconditionally (like matexe_decimal) so it
// works as documentation/opt-out even in documents that never turn
// ForzarDecimal on.
const matexeFraccionDef = `matexe_fraccion(e):=e$`

// SetDecimais sets how many decimal places ToTeX should round
// floating-point results to, defining the Maxima-side helpers (see
// matexeArredondarDef/matexeDecimalDef/matexeFraccionDef above).
// matexe_decimal/matexe_fraccion are always defined (matexe_decimal is
// just an alias for Maxima's own float(), useful regardless of rounding);
// matexe_arredondar (the rounding itself) is only defined when n > 0 -
// n <= 0 disables rounding for subsequent ToTeX calls.
func (m *Maxima) SetDecimais(n int) error {
	m.Decimais = n
	if _, err := m.Send(matexeDecimalDef, false); err != nil {
		return err
	}
	if _, err := m.Send(matexeFraccionDef, false); err != nil {
		return err
	}
	if n <= 0 {
		return nil
	}
	_, err := m.Send(matexeArredondarDef, false)
	return err
}

func (m *Maxima) protect(s string) string {
	if m.ProtectRaw == nil {
		return s
	}
	return m.ProtectRaw(s)
}

func NewMaxima(sess *Session) *Maxima {
	return &Maxima{sess: sess}
}

func (m *Maxima) Close() error { return m.sess.Close() }

// SetTimeout bounds how long any single command (Send/ToTeX) waits for
// Maxima before the session is killed automatically - see Session.SetTimeout.
func (m *Maxima) SetTimeout(d time.Duration) { m.sess.SetTimeout(d) }

// Send sends a line to Maxima and returns the evaluated result as plain
// text. Mirrors TMaxima.CAS_send: extracts the value between our command's
// own (%oN) label and the following (%iM) prompt, ignoring the sentinel's
// own (%oM) id_fin_de_lectura output that comes after it.
func (m *Maxima) Send(line string, forceEval bool) (string, error) {
	line = strings.TrimRight(strings.TrimSpace(line), ";$\r\n")
	if line == "" {
		return "", nil
	}
	if strings.ContainsAny(line, ";$") {
		line = line + ";" // compound statement, leave as-is
	} else if forceEval {
		line = fmt.Sprintf("ev(%s,eval,simp);", line)
	} else {
		line = line + ";"
	}

	raw, err := m.sess.rawSend(line)
	if err != nil {
		return "", err
	}
	return extractLastValue(raw), nil
}

// extractLastValue implements the (%o.../(%i... scraping from
// TMaxima.CAS_send: value = text between the second-to-last "(%oN)" label
// (our command's own output) and the following "(%iM)" prompt.
func extractLastValue(raw string) string {
	oIdx := oLabelRe.FindAllStringIndex(raw, -1)
	if len(oIdx) < 2 {
		return strings.TrimSpace(raw)
	}
	ours := oIdx[len(oIdx)-2] // last one is the sentinel's own (%oM) id_fin_de_lectura
	valueStart := ours[1]

	iIdx := iLabelRe.FindAllStringIndex(raw, -1)
	valueEnd := len(raw)
	for _, m := range iIdx {
		if m[0] > valueStart {
			valueEnd = m[0]
			break
		}
	}
	return strings.TrimSpace(raw[valueStart:valueEnd])
}

// MarcaErroInline é o estilo co que ToTeX pinta, no propio sitio do
// resultado, o texto de erro que devolveu Maxima cando unha expresión non se
// puido converter a fórmula (o equivalente HTML do \textcolor{red} do modo
// LaTeX). Está exportado porque hai dous consumidores fóra deste paquete que
// necesitan RECOÑECER esa marca no HTML xa xerado - a lapela "Test"
// (clasificarValor en proba.go) e as probas de xeración - e unha cadea
// literal copiada en tres sitios deixaría de casar en canto se tocase a cor.
const MarcaErroInline = "background-color:#fdd"

// ToTeX evaluates (or just converts, if forceEval is false) an expression
// and returns its LaTeX representation wrapped for the page, mirroring
// TMaxima.CAS_toTeX. Maxima's tex() prints "$$...$$" as a side effect
// *before* its own (%oN) => done label, so here we take everything up to
// the first "(%oN)" label instead of the last one.
func (m *Maxima) ToTeX(line string, forceEval bool) (string, error) {
	value, hasMath, err := m.toTeXValue(line, forceEval)
	if err != nil {
		return "", err
	}

	if hasMath {
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, `\lim`) {
			// \lim_{x\to a} only stacks its subscript below "lim" (the
			// notation a maths teacher expects) in \displaystyle - in the
			// inline/textstyle math this function always wraps into below
			// ("$...$"/"\(...\)"), LaTeX, MathJax and KaTeX all render it
			// beside "lim" instead, like a plain subscript. Real report: a
			// lone <MAT>limit(...)</MAT> showed "x->2" next to "lim" rather
			// than under it. <SISTEMA> doesn't need this - it already wraps
			// in display ("$$...$$") mode.
			value = `\displaystyle ` + value
		}
	}

	if m.Format == FormatLatex || m.Format == FormatMarkdown {
		if hasMath {
			return m.protect("$" + value + "$"), nil
		}
		// Maxima error output: keep it visible but out of math mode, and
		// LaTeX-escape it since it's arbitrary error text, not TeX. Markdown
		// has no better fallback for this (rare) case either - \textcolor is
		// LaTeX, but any Markdown renderer that lets $...$ through generally
		// tolerates stray LaTeX commands too, degrading to plain text.
		//
		// collapseWhitespace is what keeps ONE bad exercise from taking down
		// the WHOLE exam - see its own comment. Maxima's syntax errors are
		// several lines long and include a blank one, which \textcolor
		// cannot survive.
		return m.protect(`\textcolor{red}{\small ` + escapeLatex(collapseWhitespace(value)) + `}`), nil
	}

	if hasMath {
		return `<span class="math">\(` + value + `\)</span>`, nil
	}
	return fmt.Sprintf(`<span class="math" style="%s">%s</span>`, MarcaErroInline, html.EscapeString(value)), nil
}

// toTeXValue is ToTeX's core, split out so <SISTEMA> (renderSistema, in
// tags.go) can get the bare TeX for several lines and combine them under one
// shared brace, instead of each line coming back already wrapped in its own
// "$...$"/"\(...\)". Returns the raw TeX (or, if hasMath is false, Maxima's
// cleaned-up error text) with none of ToTeX's page-ready wrapping applied
// yet.
func (m *Maxima) toTeXValue(line string, forceEval bool) (value string, hasMath bool, err error) {
	line = strings.TrimRight(strings.TrimSpace(line), ";$\r\n")

	// Function definitions (f(x):=...) must be sent first, then only the
	// name is converted to TeX - same special case as CAS_toTeX.
	if p := strings.Index(line, ":="); p >= 0 {
		if _, err := m.Send(line, true); err != nil {
			return "", false, err
		}
		line = line[p+2:]
	}

	inner := line
	if forceEval {
		inner = fmt.Sprintf("ev(%s,eval)", line)
	}
	// ForzarDecimal: auto-wrap coma se cada <MAT>/<EVAL> chamase
	// matexe_decimal(...) explicitamente - salvo que o profesorado xa
	// escribise matexe_decimal(...) ou matexe_fraccion(...) el mesmo nesa
	// mesma liña, caso en que gaña sempre a escolla explícita (ver
	// matexeFraccionDef). A comprobación mira ao `line` orixinal (antes de
	// engadir ev()), non a `inner`, para non falsos-negativos por mor do
	// "ev(...)" que forceEval acaba de engadir por fóra.
	if m.ForzarDecimal && !strings.Contains(line, "matexe_decimal(") && !strings.Contains(line, "matexe_fraccion(") {
		inner = fmt.Sprintf("matexe_decimal(%s)", inner)
	}
	if m.Decimais > 0 {
		inner = fmt.Sprintf("matexe_arredondar(%s,%d)", inner, m.Decimais)
	}
	texCmd := fmt.Sprintf("tex(%s);", inner)

	raw, err := m.sess.rawSend(texCmd)
	if err != nil {
		return "", false, err
	}

	value = raw
	// Strip a leading "(%iN) " echo if present.
	if loc := iLabelRe.FindStringIndex(value); loc != nil && loc[0] == 0 {
		value = value[loc[1]:]
	}
	// Cut at the first "(%oN)" label - that's tex()'s own "done" return value.
	if loc := oLabelRe.FindStringIndex(value); loc != nil {
		value = value[:loc[0]]
	}
	value = strings.TrimSpace(value)

	hasMath = strings.Contains(value, "$$")
	if hasMath {
		value = strings.Replace(value, "$$", "", 1)
		value = strings.Replace(value, "$$", "", 1)
		value = strings.TrimSpace(value)
	} else {
		value = cleanMaximaSyntaxError(value)
	}
	return value, hasMath, nil
}

// cleanMaximaSyntaxError turns a raw "incorrect syntax" reader error from
// Maxima - internal Lisp-reader debug noise (every space in the offending
// line spelled out as the literal word "Space", a caret pointer line, a
// trailing "(%iN)" prompt echo) - into one short, legible line. A teacher
// looking at a generated exam has no reason to know what "Space=Space(6..."
// means; this is exactly the kind of dump ToTeX falls back to showing
// verbatim when a <MAT>/<EVAL> expression fails to even parse (most often:
// content generated by the IA assistant, or typed by hand, that ends in a
// stray "=" - explicitly forbidden in the IA's system prompt, see
// gemini_client.go's chuletaMatexe, but not something the model always
// avoids). Anything that doesn't look like this exact shape is returned
// untouched - better an ugly-but-honest message than a wrongly "cleaned"
// one for an error shape this function doesn't actually recognise.
func cleanMaximaSyntaxError(raw string) string {
	lines := strings.SplitN(raw, "\n", 2)
	first := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(first, "incorrect syntax") {
		return raw
	}
	msg := first
	if len(lines) > 1 {
		// The offending line is Maxima's own echo of the bad input with
		// every space spelled out as "Space" - the caret/(%iN) lines that
		// follow are positional/prompt noise, meaningless without the
		// original column, so they're dropped rather than decoded.
		for _, l := range strings.Split(lines[1], "\n") {
			if strings.Contains(l, "Space") {
				msg += ": " + strings.ReplaceAll(strings.TrimSpace(l), "Space", " ")
				break
			}
		}
	}
	if strings.HasSuffix(strings.TrimSpace(msg), "=") {
		msg += " (non pode rematar nun '=' solto dentro de <MAT>/<EVAL>)"
	}
	return msg
}

// SetSeed mirrors TMaxima.setSeed: seeds Maxima's PRNG so a run is
// reproducible for a given seed.
func (m *Maxima) SetSeed(seed int) error {
	if _, err := m.Send(fmt.Sprintf("matexe_seed:%d", seed), false); err != nil {
		return err
	}
	_, err := m.Send(fmt.Sprintf("set_random_state(make_random_state(%d))", seed), false)
	return err
}
