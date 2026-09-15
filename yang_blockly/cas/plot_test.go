package cas

import (
	"os/exec"
	"strings"
	"testing"
)

// TestPlotLatexUsesTikzForPlot2d locks in the "gráficos deseñados" fix: a
// <PLOT> with plot2d/plot3d must render as \input{...} of a TikZ file (the
// document's own font, see cas/tags.go plot()), NOT the old
// \includegraphics{...pdf} - real report: gnuplot's bitmap font next to
// pdflatex's own text read as "pasted in", not designed.
func TestPlotLatexUsesTikzForPlot2d(t *testing.T) {
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
	m.SetOutDir(t.TempDir())

	out, err := m.processTag("PLOT", "plot2d(x^2,[x,-2,2])")
	if err != nil {
		t.Fatalf("processTag PLOT: %v", err)
	}
	if !strings.Contains(out, "\\input{images/") || !strings.Contains(out, ".tex}") {
		t.Errorf("esperaba \\input{images/....tex}, got: %q", out)
	}
	if strings.Contains(out, "\\includegraphics") {
		t.Errorf("plot2d non debería usar \\includegraphics (ese é o camiño draw2d/pdf), got: %q", out)
	}
}

// TestPlotLatexKeepsPdfForDraw2d is the non-regression guard for the other
// half of plot(): Maxima's draw package only allows a fixed terminal list
// (confirmed by a real "illegal terminal specification: tikz" error) - "tikz"
// isn't in it, so draw2d/draw3d must keep working exactly as before (pdf +
// \includegraphics), not break when isDraw is detected.
func TestPlotLatexKeepsPdfForDraw2d(t *testing.T) {
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
	m.SetOutDir(t.TempDir())

	out, err := m.processTag("PLOT", "draw2d(explicit(x^2,x,-2,2))")
	if err != nil {
		t.Fatalf("processTag PLOT: %v", err)
	}
	if !strings.Contains(out, "\\includegraphics") || !strings.Contains(out, ".pdf}") {
		t.Errorf("esperaba \\includegraphics{....pdf}, got: %q", out)
	}
}
