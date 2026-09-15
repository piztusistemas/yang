package cas

import "strings"

var latexEscaper = strings.NewReplacer(
	`\`, `\textbackslash{}`,
	`&`, `\&`,
	`%`, `\%`,
	`$`, `\$`,
	`#`, `\#`,
	`_`, `\_`,
	`{`, `\{`,
	`}`, `\}`,
	`~`, `\textasciitilde{}`,
	`^`, `\textasciicircum{}`,
)

// EscapeLatex escapes LaTeX's special characters in plain text (content
// that is *not* already math/TeX source, e.g. error messages or ordinary
// document text).
func EscapeLatex(s string) string {
	return latexEscaper.Replace(s)
}

func escapeLatex(s string) string { return EscapeLatex(s) }

// collapseWhitespace flattens every run of whitespace into a single space
// and trims the ends, so the result cannot contain a blank line.
//
// This exists for one specific, document-killing failure. Maxima reports a
// syntax error over several lines, and the line that carries the "^" caret
// pointing at the offending token is followed by an EMPTY one:
//
//	sintaxis no correcta: Missing )
//	arredondar(ev(x^2+1;
//	                  ^
//	                                 ← this blank line
//	(%i96)
//
// An empty line is a \par to TeX, and the argument of \textcolor is not
// \long, so pdflatex aborts with "! Paragraph ended before \@textcolor was
// complete." Because compileLatex runs with -halt-on-error, that ends the
// run right there: a single mistyped <EVAL> in ONE exercise made the ENTIRE
// exam fail to compile (reproduced with testdata/bloques.matex), instead of
// just showing that one error in red where it belongs. Same class of bug as
// the one mathTextSymbols documents further down for stray Unicode glyphs.
//
// Nothing of value is lost by flattening: the caret's column alignment only
// means anything in a monospaced terminal, and this text is about to be
// typeset as an ordinary proportional-font paragraph, where LaTeX collapses
// runs of spaces anyway.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// mathTextSymbols maps Unicode math/relational/Greek characters that
// commonly show up as PLAIN PROSE (typed by hand or by the IA assistant,
// see gemini_client.go's chuletaMatexe: "letras gregas soltas... son HTML
// válido") to a LaTeX command that actually renders them, instead of the
// raw codepoint. Found empirically running real documents through pdflatex:
// Computer Modern/Latin Modern's TEXT font (used for ordinary paragraph
// text) has none of these glyphs, so passing them through as-is either
// aborts the whole PDF with a fatal "Unicode character ... not set up for
// use with LaTeX" (pdflatex, -halt-on-error - the entire exam fails to
// generate, not just one exercise) or silently drops the glyph with a
// "Missing character" warning (xelatex/lualatex, whose fontspec default
// font has the same gap). Routing them through \ensuremath{...} sidesteps
// the text-font gap entirely - amsmath/amssymb (loaded in both LaTeX
// preambles, see latexPreambleTemplatePDF/Unicode) supply these commands
// via the MATH font, which both engines already have, so this fixes both
// engines uniformly instead of duplicating an engine-specific fixup.
var mathTextSymbols = map[rune]string{
	'−': `\ensuremath{-}`, // U+2212 MINUS SIGN (not the ASCII hyphen)
	'×': `\ensuremath{\times}`,
	'÷': `\ensuremath{\div}`,
	'±': `\ensuremath{\pm}`,
	'≈': `\ensuremath{\approx}`,
	'≤': `\ensuremath{\leq}`,
	'≥': `\ensuremath{\geq}`,
	'≠': `\ensuremath{\neq}`,
	'∞': `\ensuremath{\infty}`,
	'∑': `\ensuremath{\sum}`,
	'√': `\ensuremath{\surd}`,
	'∪': `\ensuremath{\cup}`,
	'∩': `\ensuremath{\cap}`,
	'∈': `\ensuremath{\in}`,
	'∉': `\ensuremath{\notin}`,
	'⊂': `\ensuremath{\subset}`,
	'⊆': `\ensuremath{\subseteq}`,
	'∅': `\ensuremath{\emptyset}`,
	'∖': `\ensuremath{\setminus}`,
	'ℝ': `\ensuremath{\mathbb{R}}`,
	'ℕ': `\ensuremath{\mathbb{N}}`,
	'ℤ': `\ensuremath{\mathbb{Z}}`,
	'ℚ': `\ensuremath{\mathbb{Q}}`,
	'ℂ': `\ensuremath{\mathbb{C}}`,
	'→': `\ensuremath{\rightarrow}`,
	'←': `\ensuremath{\leftarrow}`,
	'↔': `\ensuremath{\leftrightarrow}`,
	'⇒': `\ensuremath{\Rightarrow}`,
	'⇐': `\ensuremath{\Leftarrow}`,
	'⇔': `\ensuremath{\Leftrightarrow}`,
	'°': `\textdegree{}`,
	'α': `\ensuremath{\alpha}`, 'β': `\ensuremath{\beta}`, 'γ': `\ensuremath{\gamma}`,
	'δ': `\ensuremath{\delta}`, 'ε': `\ensuremath{\varepsilon}`, 'ζ': `\ensuremath{\zeta}`,
	'η': `\ensuremath{\eta}`, 'θ': `\ensuremath{\theta}`, 'ι': `\ensuremath{\iota}`,
	'κ': `\ensuremath{\kappa}`, 'λ': `\ensuremath{\lambda}`, 'μ': `\ensuremath{\mu}`,
	'ν': `\ensuremath{\nu}`, 'ξ': `\ensuremath{\xi}`, 'π': `\ensuremath{\pi}`,
	'ρ': `\ensuremath{\rho}`, 'σ': `\ensuremath{\sigma}`, 'τ': `\ensuremath{\tau}`,
	'υ': `\ensuremath{\upsilon}`, 'φ': `\ensuremath{\varphi}`, 'χ': `\ensuremath{\chi}`,
	'ψ': `\ensuremath{\psi}`, 'ω': `\ensuremath{\omega}`,
	'Γ': `\ensuremath{\Gamma}`, 'Δ': `\ensuremath{\Delta}`, 'Θ': `\ensuremath{\Theta}`,
	'Λ': `\ensuremath{\Lambda}`, 'Ξ': `\ensuremath{\Xi}`, 'Π': `\ensuremath{\Pi}`,
	'Σ': `\ensuremath{\Sigma}`, 'Φ': `\ensuremath{\Phi}`, 'Ψ': `\ensuremath{\Psi}`,
	'Ω': `\ensuremath{\Omega}`,
}

// EscapeLatexProse is EscapeLatex plus the mathTextSymbols substitution
// above - the one to use for actual document body text (see
// escapeOutsideMath in latexdoc.go). Kept separate from EscapeLatex itself
// (used as-is for short diagnostic strings, e.g. maxima.go's inline Maxima
// error messages, where these symbols essentially never appear and pulling
// in \ensuremath commands would be pointless).
func EscapeLatexProse(s string) string {
	var b strings.Builder
	for _, r := range s {
		if repl, ok := mathTextSymbols[r]; ok {
			b.WriteString(repl)
			continue
		}
		b.WriteString(latexEscaper.Replace(string(r)))
	}
	return b.String()
}
