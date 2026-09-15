package cas

import (
	"os/exec"
	"strings"
	"testing"
)

// TestTikzLatexMode doesn't need a live Maxima session at all: tikz() never
// calls m.Send in FormatLatex mode, it just wraps the raw code. No
// ProtectRaw set, so protect() is a no-op and the raw LaTeX is directly
// assertable. Also locks in the \begin{center}/\end{center} wrapper (real
// report: a TikZ diagram with no whitespace around it in the source - the
// block editor concatenates blocks with none - rendered inline and
// overlapped the surrounding text, since a bare tikzpicture has no implicit
// \par the way center's trivlist does).
func TestTikzLatexMode(t *testing.T) {
	m := &Maxima{Format: FormatLatex}
	out, err := m.processTag("TIKZ", "\\draw (0,0) -- (1,1);")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\\begin{center}\\begin{tikzpicture}") || !strings.Contains(out, "\\draw (0,0) -- (1,1);") || !strings.Contains(out, "\\end{tikzpicture}\\end{center}") {
		t.Fatalf("unexpected output: %q", out)
	}
	// A circuitikz drawing's symbols/labels overrun the tikzpicture bounding
	// box, so beyond center's own \par there has to be an explicit vertical
	// gap on each side or the diagram prints over the adjacent text.
	if !strings.Contains(out, "\\par\\addvspace{\\baselineskip}%\n\\begin{center}") || !strings.Contains(out, "\\end{center}\\par\\addvspace{\\baselineskip}%") {
		t.Fatalf("missing vertical padding around the tikzpicture: %q", out)
	}
}

// TestTikzLatexMalformadoDaAviso: un <TIKZ> que non compila (un \node sen
// "{...}", o erro habitual do contido da IA) NON tumba o documento - no seu
// sitio vai unha caixa de aviso. Necesita pdflatex (+ circuitikz para o
// prevoo); sen eles pásase o código tal cal e este test sáltase.
func TestTikzLatexMalformadoDaAviso(t *testing.T) {
	if !commandExists("pdflatex") {
		t.Skip("pdflatex non atopado, sáltase")
	}
	if !tikzPreamboloUsable() {
		t.Skip("o preámbulo do prevoo (tikz+circuitikz) non compila neste equipo")
	}
	m := &Maxima{Format: FormatLatex}
	out, err := m.processTag("TIKZ", `\draw (0,1) circle (0.35); \node at (0,1) R_1;`)
	if err != nil {
		t.Fatalf("un <TIKZ> roto non debe devolver erro: %v", err)
	}
	if strings.Contains(out, "\\begin{tikzpicture}") {
		t.Errorf("un <TIKZ> roto non se debe inserir tal cal: %q", out)
	}
	if !strings.Contains(out, "Aviso:") {
		t.Errorf("esperábase a caixa de aviso: %q", out)
	}

	// Un <TIKZ> correcto SI se insire.
	ok, err := m.processTag("TIKZ", `\draw (0,0) rectangle (1,1);`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ok, "\\begin{tikzpicture}") {
		t.Errorf("un <TIKZ> válido debe inserirse: %q", ok)
	}
}

// TestTikzHTMLMode doesn't need Maxima either (same reason as above) - only
// pdflatex + poppler-utils (pdftoppm), which is a nice side effect of the
// design: TIKZ tests don't require Maxima to be installed.
func TestTikzHTMLMode(t *testing.T) {
	if !commandExists("pdflatex") || !commandExists("pdftoppm") {
		t.Skip("pdflatex/pdftoppm not found on this machine, skipping")
	}
	m := &Maxima{Format: FormatHTML}
	out, err := m.processTag("TIKZ", "\\draw (0,0) rectangle (1,1);")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, `<img src="data:image/png;base64,`) {
		t.Fatalf("expected a data-URI <img>, got: %q", out[:min(80, len(out))])
	}
}

// TestBareRelationalSymbol locks in specialCaseSymbol's short-circuit for a
// lone comparison operator wrapped in <MAT>/<EVAL> (real report: the IA
// assistant wrote <MAT>>=</MAT> to reference "the inequality has a ≥" in
// prose) - Maxima always rejects this ("is not a prefix operator"), so it
// must never reach m.Send at all. No live Maxima session needed: a nil
// m.sess would panic if this path fell through to ToTeX.
func TestBareRelationalSymbol(t *testing.T) {
	m := &Maxima{Format: FormatLatex}
	cases := map[string]string{
		">=": `\geq`, "<=": `\leq`, "!=": `\neq`, "#": `\neq`,
		"=": "=", "<": "<", ">": ">", "≥": `\geq`, "≤": `\leq`, "≠": `\neq`,
	}
	for in, want := range cases {
		out, err := m.processTag("MAT", in)
		if err != nil {
			t.Errorf("MAT %q: unexpected error: %v", in, err)
			continue
		}
		if out != "$"+want+"$" {
			t.Errorf("MAT %q: got %q, want %q", in, out, "$"+want+"$")
		}
	}
	// A real inequality (operands on both sides) must still go through
	// Maxima/ToTeX as before - only the truly bare operator short-circuits.
	if _, ok := bareRelationalSymbol("x >= 3"); ok {
		t.Errorf("bareRelationalSymbol(%q) should not match a full expression", "x >= 3")
	}
}

// TestIntervalNotationSymbol locks in specialCaseSymbol's short-circuit for
// interval notation (real report: <MAT>(-inf, -2]</MAT> and
// <MAT>(3, inf)</MAT>) - mismatched brackets are a hard Maxima syntax
// error, and matched round parens with an infinite bound silently
// evaluates to just the second element (confirmed empirically), so both
// shapes are intercepted before reaching Maxima.
func TestIntervalNotationSymbol(t *testing.T) {
	m := &Maxima{Format: FormatLatex}
	cases := map[string]string{
		"(-inf, -2]":  `(-\infty, -2]`,
		"(3, inf)":    `(3, \infty)`,
		"(-inf, inf)": `(-\infty, \infty)`,
	}
	for in, want := range cases {
		out, err := m.processTag("MAT", in)
		if err != nil {
			t.Errorf("MAT %q: unexpected error: %v", in, err)
			continue
		}
		if out != "$"+want+"$" {
			t.Errorf("MAT %q: got %q, want %q", in, out, "$"+want+"$")
		}
	}
	// "[a, b]" (matched square brackets, no infinity) is a legitimate
	// Maxima list literal - must NOT be intercepted.
	if _, ok := intervalNotationSymbol("[a, b]"); ok {
		t.Errorf("intervalNotationSymbol(%q) should leave a valid Maxima list alone", "[a, b]")
	}
	// "(a, b)" with no infinity bound could be a real two-statement Maxima
	// expression - must NOT be intercepted either.
	if _, ok := intervalNotationSymbol("(a, b)"); ok {
		t.Errorf("intervalNotationSymbol(%q) should leave a plain paren pair alone", "(a, b)")
	}
}

// TestMathProseAsText locks in the three real reports of this shape: a
// label with a full expression ending in a dangling "=" ("dom(f) = "), a
// chained symmetric-line equation ("(x-1)/2 = (y+1)/1 = z/2"), and the
// same chain with commas INSIDE nested lists ("PM = [...] = [...]" - the
// case that broke a naive "no comma anywhere" heuristic, see
// countTopLevelEquals). All three must render as plain literal text
// instead of ever reaching Maxima (which rejects all three).
func TestMathProseAsText(t *testing.T) {
	m := &Maxima{Format: FormatLatex}
	cases := map[string]string{
		"dom(f) =":                                 `dom(f) =`,
		"(x - 1)/2 = (y + 1)/1 = z/2":              `(x - 1)/2 = (y + 1)/1 = z/2`,
		"PM = [3 - 1, 0 - 2, 2 - 3] = [2, -2, -1]": `PM = [3 - 1, 0 - 2, 2 - 3] = [2, -2, -1]`,
	}
	for in, want := range cases {
		out, err := m.processTag("MAT", in)
		if err != nil {
			t.Errorf("MAT %q: unexpected error: %v", in, err)
			continue
		}
		if out != want {
			t.Errorf("MAT %q: got %q, want %q", in, out, want)
		}
	}

	// Un só "=" en Maxima válido (dentro dunha chamada real, con comas de
	// separación de argumentos) NON debe interceptarse.
	if _, ok := mathProseAsText("ev(f(x),x=3,y=2)"); ok {
		t.Errorf(`mathProseAsText("ev(f(x),x=3,y=2)") should leave a valid multi-arg call alone`)
	}
	// Unha soa relación (non encadeada, non remata en "=") tampouco.
	if _, ok := mathProseAsText("x <= 5"); ok {
		t.Errorf(`mathProseAsText("x <= 5") should leave a single valid inequality alone`)
	}
}

// TestDegreeSymbolAsText locks in the real report: an IA-generated worked
// solution for complex numbers in polar form wrote r[θ°]-style notation
// directly inside <MAT> - "z[1] = 2[60°]" crashed with "incorrect syntax"
// (Maxima has no polar-form literal and no notion of "°" at all), and
// "z[2] = (sqrt(2))[135°]" crashed with "° is not an infix operator". Both
// have only ONE top-level "=" (mathProseAsText's chain check needs >= 2),
// so containsDegreeSymbol is what has to catch them.
func TestDegreeSymbolAsText(t *testing.T) {
	m := &Maxima{Format: FormatLatex}
	cases := map[string]string{
		"z[1] = 2[60°]":          "z[1] = 2[60\\textdegree{}]",
		"z[2] = (sqrt(2))[135°]": "z[2] = (sqrt(2))[135\\textdegree{}]",
	}
	for in, want := range cases {
		out, err := m.processTag("MAT", in)
		if err != nil {
			t.Errorf("MAT %q: unexpected error: %v", in, err)
			continue
		}
		if out != want {
			t.Errorf("MAT %q: got %q, want %q", in, out, want)
		}
	}
	if _, ok := mathProseAsText("60°"); !ok {
		t.Errorf(`mathProseAsText("60°") should catch a bare degree value too`)
	}
	if _, ok := mathProseAsText("2 * 3"); ok {
		t.Errorf(`mathProseAsText("2 * 3") should leave a plain expression without "°" alone`)
	}
}

// TestPureNumericArithmetic locks in the real report: <MAT>548 + 375</MAT>
// printed "923" (the simplified result) instead of "548 + 375" as written -
// Maxima's simplifier collapses numeric-literal arithmetic at parse time,
// which quoting (autoQuoteLimit's trick) doesn't block, so this is
// intercepted the same way as bareRelationalSymbol/intervalNotationSymbol
// instead of ever reaching Maxima.
func TestPureNumericArithmetic(t *testing.T) {
	m := &Maxima{Format: FormatLatex}
	cases := map[string]string{
		"548 + 375":    `548 + 375`,
		"12.5 - 3 + 1": `12.5 - 3 + 1`,
		"10 - -5":      `10 - -5`,
	}
	for in, want := range cases {
		out, err := m.processTag("MAT", in)
		if err != nil {
			t.Errorf("MAT %q: unexpected error: %v", in, err)
			continue
		}
		if out != "$"+want+"$" {
			t.Errorf("MAT %q: got %q, want %q", in, out, "$"+want+"$")
		}
	}

	// Un só literal (nada que "des-simplificar", sen operador) non debe
	// interceptarse - segue a Maxima coma antes.
	if _, ok := pureNumericArithmetic("923"); ok {
		t.Errorf(`pureNumericArithmetic("923") should leave a lone literal alone`)
	}
	// Aritmética con variables non é "pure numeric" - debe seguir a Maxima.
	if _, ok := pureNumericArithmetic("548 + x"); ok {
		t.Errorf(`pureNumericArithmetic("548 + x") should leave a real expression alone`)
	}
}

// TestPureNumericArithmeticFactorisation locks in the second real report:
// <MAT>48 = 2^4 * 3</MAT> printed "48 = 48" instead of the factorisation as
// written - same root cause as TestPureNumericArithmetic, but for * and ^
// (widened later) and for a "lhs = rhs" factorisation, not just a bare
// expression.
func TestPureNumericArithmeticFactorisation(t *testing.T) {
	m := &Maxima{Format: FormatLatex}
	cases := map[string]string{
		"48 = 2^4 * 3":  `48 = 2^{4} \cdot 3`,
		"2^3 * 3":       `2^{3} \cdot 3`,
		"2^4 * 3^2 * 5": `2^{4} \cdot 3^{2} \cdot 5`,
	}
	for in, want := range cases {
		out, err := m.processTag("MAT", in)
		if err != nil {
			t.Errorf("MAT %q: unexpected error: %v", in, err)
			continue
		}
		if out != "$"+want+"$" {
			t.Errorf("MAT %q: got %q, want %q", in, out, "$"+want+"$")
		}
	}

	// "5 = 5": nin lado ten operador ningún, non hai nada que preservar -
	// segue a Maxima coma antes.
	if _, ok := pureNumericArithmetic("5 = 5"); ok {
		t.Errorf(`pureNumericArithmetic("5 = 5") should leave a bare equality alone`)
	}
}

// TestNormalizeLatexSubscripts locks in the real report: <MAT>R_{eq}</MAT>
// (and variants like "I_t = V / R_{eq}", "P_{tot} = V * I_t") fail with
// Maxima's "incorrect syntax: { is not an infix operator", because "{" is
// always a SET literal to Maxima, never a subscript grouping like in LaTeX.
// normalizeLatexSubscripts must strip the braces to a plain identifier
// Maxima accepts ("R_eq") while leaving anything with a non-alphanumeric
// subscript (genuine multi-level LaTeX Maxima has no equivalent for)
// untouched.
func TestStripNestedValueTags(t *testing.T) {
	cases := map[string]string{
		// Report real: exercicio de matrices na Táboa, "{a}"/"{b}" declaradas.
		"A = matrix([<EVAL>a</EVAL>, <EVAL>b</EVAL>], [1, 2])": "A = matrix([a, b], [1, 2])",
		"2*<MAT>x</MAT> + 1":        "2*x + 1",
		"<EVAL><MAT>k</MAT></EVAL>": "k",
		"sen etiquetas aniñadas":    "sen etiquetas aniñadas",
	}
	for in, want := range cases {
		if got := stripNestedValueTags(in); got != want {
			t.Errorf("stripNestedValueTags(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeLatexSubscripts(t *testing.T) {
	cases := map[string]string{
		"R_{eq}":             "R_eq",
		"R_{eq} = R_1 + R_2": "R_eq = R_1 + R_2",
		"I_t = V / R_{eq}":   "I_t = V / R_eq",
		"P_{tot} = V * I_t":  "P_tot = V * I_t",
		"x_{i,j}":            "x_{i,j}",   // non tocado: subíndice non alfanumérico
		"x_{i+1}":            "x_{i+1}",   // idem
		"R_1 + R_2":          "R_1 + R_2", // xa válido, sen chaves
	}
	for in, want := range cases {
		if got := normalizeLatexSubscripts(in); got != want {
			t.Errorf("normalizeLatexSubscripts(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestAutoQuoteLimit é o test de regresión para "<MAT>limit(...)</MAT>
// amosa un número solto (3) en vez da notación do límite" - só limit() sen
// comiña se autocorrixe (ver comentario de autoQuoteLimit); diff/integrate/
// outras funcións, ou un limit() xa citado a man, quedan tal cal.
func TestAutoQuoteLimit(t *testing.T) {
	cases := map[string]string{
		"limit(sqrt(x^2+6*x)-x,x,inf)": "'limit(sqrt(x^2+6*x)-x,x,inf)",
		"limit (f(x), x, 0)":           "'limit (f(x), x, 0)",
		"'limit(f(x),x,inf)":           "'limit(f(x),x,inf)",  // xa citado, non dobra
		"diff(x^2,x)":                  "diff(x^2,x)",         // fóra de alcance, non tocado
		"x + limit(f(x),x,0)":          "x + limit(f(x),x,0)", // limit() non está ao principio
	}
	for in, want := range cases {
		if got := autoQuoteLimit(in); got != want {
			t.Errorf("autoQuoteLimit(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestAutoQuoteFormula: unha fórmula alxébrica só con nomes de variables
// cítase (para verse coma fórmula dentro de <RESP>/<SOL>); calquera cousa
// cunha chamada a función, sen operador, só con números, ou xa citada,
// queda tal cal.
func TestAutoQuoteFormula(t *testing.T) {
	cases := map[string]string{
		"V_1/R_1":     "'(V_1/R_1)",
		"F/m":         "'(F/m)",
		"(1/2)*m*v^2": "'((1/2)*m*v^2)",
		" a + b ":     "'(a + b)",
		"diff(x^2,x)": "diff(x^2,x)", // chamada a función: cálculo real
		"sqrt(2*g*h)": "sqrt(2*g*h)", // idem
		"'(V_1/R_1)":  "'(V_1/R_1)",  // xa citada
		"x":           "x",           // sen operador
		"220/13.2":    "220/13.2",    // só números -> pureNumericArithmetic
		"a = b":       "a = b",       // ten "=", non é fórmula pura
		"[a, b]":      "[a, b]",      // corchetes: fóra
	}
	for in, want := range cases {
		if got := autoQuoteFormula(in); got != want {
			t.Errorf("autoQuoteFormula(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMatEnCorreccionAmosaSimbolos: dentro dun <SOL>, <MAT>V_1/R_1</MAT> ten
// que verse coma a FÓRMULA (V_{1}\over R_{1}) aínda que V_1 e R_1 estean
// ligadas a números - se non, o boletín amosa "16.67 = 16.67". <EVAL> na
// mesma liña si dá o número.
func TestMatEnCorreccionAmosaSimbolos(t *testing.T) {
	maximaPath, err := exec.LookPath("maxima")
	if err != nil {
		t.Skip("maxima not found on this machine, skipping")
	}
	sess, err := Open(maximaPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sess.Close()
	m := NewMaxima(sess)
	defer m.Close()
	m.Format = FormatLatex
	m.RenderMode = "solucions" // fai que <SOL> se renderice

	if _, err := m.Send("V_1: 220$ R_1: 13.2$", true); err != nil {
		t.Fatal(err)
	}

	out, err := m.ProcessTag("SOL", "<p>I = <MAT>V_1/R_1</MAT> = <EVAL>V_1/R_1</EVAL> A</p>")
	if err != nil {
		t.Fatalf("ProcessTag(SOL): %v", err)
	}
	if !strings.Contains(out, "V_{1}") || !strings.Contains(out, "R_{1}") {
		t.Errorf("<MAT> nun <SOL> debería amosar a fórmula simbólica, got: %q", out)
	}
	if !strings.Contains(out, "16.6") {
		t.Errorf("<EVAL> na mesma liña debería dar o número, got: %q", out)
	}

	// Fóra de <RESP>/<SOL> o <MAT> segue substituíndo (comportamento de sempre).
	m.enCorreccion = false
	plain, err := m.ProcessTag("MAT", "V_1/R_1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "V_{1}") {
		t.Errorf("fóra de <SOL>, <MAT>V_1/R_1</MAT> debe dar o número, non a fórmula: %q", plain)
	}
}

// TestMatConEtiquetasAniñadas: report real. Un exercicio de matrices feito na
// Táboa con "<MAT>A = matrix([{a}, {b}], [1, 2])</MAT>" e a,b como variables
// declaradas: o "{a}" expandíase a "<EVAL>a</EVAL>" DENTRO do <MAT> e Maxima
// rebentaba con "incorrect syntax: < is not a prefix operator". Agora
// stripNestedValueTags desenvolve o <EVAL> ao nome espido antes de chegar a
// Maxima, e este resólveo co binding do <HIDE>.
func TestMatConEtiquetasAniñadas(t *testing.T) {
	maximaPath, err := exec.LookPath("maxima")
	if err != nil {
		t.Skip("maxima not found on this machine, skipping")
	}
	sess, err := Open(maximaPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sess.Close()
	m := NewMaxima(sess)
	defer m.Close()
	m.Format = FormatLatex

	if _, err := m.Send("a: 3$ b: 5$", true); err != nil {
		t.Fatal(err)
	}
	out, err := m.ProcessTag("MAT", "A = matrix([<EVAL>a</EVAL>, <EVAL>b</EVAL>], [1, 2])")
	if err != nil {
		t.Fatalf("ProcessTag(MAT) con etiquetas aniñadas: %v", err)
	}
	if strings.Contains(out, "incorrect syntax") || strings.Contains(out, "prefix operator") {
		t.Fatalf("seguiu chegando o <EVAL> literal a Maxima: %q", out)
	}
	if !strings.Contains(out, "3") || !strings.Contains(out, "5") {
		t.Errorf("esperábase a matriz cos valores ligados (3, 5): %q", out)
	}
}

// TestCountTopLevelEquals cobre directamente o contador que soluciona o
// fallo do heurístico anterior baseado en "sen comas en ningures".
func TestCountTopLevelEquals(t *testing.T) {
	cases := map[string]int{
		"ev(f(x),x=3,y=2)":                         0,
		"(x - 1)/2 = (y + 1)/1 = z/2":              2,
		"PM = [3 - 1, 0 - 2, 2 - 3] = [2, -2, -1]": 2,
		"x <= 5":   1,
		"dom(f) =": 1,
	}
	for in, want := range cases {
		if got := countTopLevelEquals(in); got != want {
			t.Errorf("countTopLevelEquals(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestMaximaErrorFrom locks in both error shapes maximaErrorFrom must catch:
// the "-- an error." runtime block, and a bare parser rejection (no such
// block) - the latter is what a <PLOT>/<HIDE> with a LaTeX-style single
// quoted title hits, and it used to slip straight through.
func TestMaximaErrorFrom(t *testing.T) {
	if got := maximaErrorFrom("(%o7) done\n"); got != "" {
		t.Errorf("clean output: got %q, want \"\"", got)
	}
	runtimeErr := "draw: unknown option axes\n -- an error. To debug this try: debugmode(true);\n(%i8) "
	if got := maximaErrorFrom(runtimeErr); got != "draw: unknown option axes" {
		t.Errorf("runtime error: got %q", got)
	}
	syntaxErr := "incorrect syntax: e is not an infix operator\n,5],[title, 'f(x) e \n                  ^\n\n(%i2) "
	got := maximaErrorFrom(syntaxErr)
	if !strings.HasPrefix(got, "incorrect syntax: e is not an infix operator") {
		t.Errorf("syntax error: got %q", got)
	}
	if strings.Contains(got, "(%i2)") {
		t.Errorf("syntax error: prompt echo leaked into message: %q", got)
	}
}

// TestRespSolRenderMode: as etiquetas <RESP>/<SOL> da interface de táboa
// inclúense ou descártanse segundo m.RenderMode, sen tocar Maxima cando o
// contido non leva outras etiquetas (é o caso probado aquí).
func TestRespSolRenderMode(t *testing.T) {
	casos := []struct {
		modo       string
		kind       string
		wantOutput string
	}{
		{"", "RESP", ""},
		{"", "SOL", ""},
		{"enunciados", "RESP", ""},
		{"enunciados", "SOL", ""},
		{"resposta", "RESP", "abc"},
		{"resposta", "SOL", ""},
		// Regra estrita: en "solucions" SÓ aparece <SOL> (a <RESP> non se
		// repite á parte).
		{"solucions", "RESP", ""},
		{"solucions", "SOL", "abc"},
		{"proba", "RESP", "abc"},
		{"proba", "SOL", "abc"},
	}
	for _, c := range casos {
		m := &Maxima{RenderMode: c.modo}
		got, err := m.processTag(c.kind, "abc")
		if err != nil {
			t.Fatalf("modo=%q kind=%q: erro inesperado: %v", c.modo, c.kind, err)
		}
		if got != c.wantOutput {
			t.Errorf("modo=%q kind=%q: got %q, want %q", c.modo, c.kind, got, c.wantOutput)
		}
	}
}

// TestRespSolParseTextStripping: nun documento completo, ParseText aplica a
// regra estrita das lapelas - o texto de arredor non se toca en ningún caso.
func TestRespSolParseTextStripping(t *testing.T) {
	doc := "Pregunta. <RESP>Resposta: 42.</RESP> fin <SOL>Paso 1... logo 42.</SOL>"

	comprobar := func(modo string, querenResp, querenSol bool) {
		m := &Maxima{RenderMode: modo}
		got, err := m.ParseText(doc)
		if err != nil {
			t.Fatalf("modo %q: %v", modo, err)
		}
		if !strings.Contains(got, "Pregunta.") || !strings.Contains(got, "fin") {
			t.Errorf("modo %q: o texto de arredor perdeuse: %q", modo, got)
		}
		if strings.Contains(got, "Resposta: 42.") != querenResp {
			t.Errorf("modo %q: <RESP> visible=%v, esperado=%v: %q", modo, !querenResp, querenResp, got)
		}
		if strings.Contains(got, "Paso 1") != querenSol {
			t.Errorf("modo %q: <SOL> visible=%v, esperado=%v: %q", modo, !querenSol, querenSol, got)
		}
	}

	comprobar("enunciados", false, false)
	comprobar("resposta", true, false)
	comprobar("solucions", false, true) // estrito: SÓ <SOL>
	comprobar("proba", true, true)
}

// TestTikzAmsmathNoPrevoo: o prevoo compila o fragmento nun documento á
// parte, e ese documento TEN que traer os mesmos paquetes ca o boletín. Caso
// real: un esquema de circuíto cun "\text{ V}" nunha etiqueta de nodo daba
// "Undefined control sequence" no prevoo (faltaba amsmath) e o exercicio, que
// levaba tempo saíndo ben, pasou a amosar a caixa de aviso.
func TestTikzAmsmathNoPrevoo(t *testing.T) {
	if !commandExists("pdflatex") {
		t.Skip("pdflatex non atopado, sáltase")
	}
	if !tikzPreamboloUsable() {
		t.Skip("o preámbulo do prevoo non compila neste equipo")
	}
	m := &Maxima{Format: FormatLatex}
	code := `\draw[thick] (0,0) rectangle (1.2,0.6); \node at (0.6,0.3) {{$R_1$}}; ` +
		`\node[left] at (-0.3,0.3) {{$24\text{{ V}}$}}; \node[above] at (0.6,0.6) {{Fusíbel}};`
	out, err := m.processTag("TIKZ", code)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\\begin{tikzpicture}") {
		t.Errorf("un debuxo con \\text (amsmath) e acentos debe compilar no prevoo, non dar aviso: %q", out)
	}
}

// TestTikzExpandeEtiquetasAniñadas: a IA mete <EVAL>/<MAT> dentro do <TIKZ>
// para poñer os valores aleatorios nas etiquetas dos compoñentes. Antes iso
// chegaba literal a pdflatex e o esquema saía co texto "<EVAL>R_1</EVAL>"
// impreso enriba do debuxo (report real). Ten que quedar resolto a TeX.
func TestTikzExpandeEtiquetasAniñadas(t *testing.T) {
	maximaPath, err := exec.LookPath("maxima")
	if err != nil {
		t.Skip("maxima not found on this machine, skipping")
	}
	sess, err := Open(maximaPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer sess.Close()
	m := NewMaxima(sess)
	defer m.Close()
	m.Format = FormatLatex

	code := `<HIDE>r_1: 47$</HIDE>\draw[thick] (0,0) rectangle (1.2,0.6); ` +
		`\node at (0.6,0.3) {$R_1$}; \node[above] at (0.6,0.6) {$<EVAL>r_1</EVAL>\,\Omega$};`
	out, err := m.processTag("TIKZ", code)
	if err != nil {
		t.Fatalf("processTag(TIKZ): %v", err)
	}
	if strings.Contains(out, "<EVAL>") || strings.Contains(out, "<HIDE>") {
		t.Errorf("quedaron etiquetas sen resolver no debuxo: %q", out)
	}
	if !strings.Contains(out, "\\ensuremath{47}") {
		t.Errorf("esperábase o valor de r_1 resolto a TeX: %q", out)
	}
	if !strings.Contains(out, "\\begin{tikzpicture}") {
		t.Errorf("o debuxo xa resolto debe compilar e inserirse: %q", out)
	}
}
