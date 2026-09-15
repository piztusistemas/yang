package main

import (
	"strings"
	"testing"
)

func TestTipografiaCorpo(t *testing.T) {
	casos := []struct {
		nome string
		in   *TipografiaOpts
		want string
	}{
		{"nil", nil, ""},
		{"todo por defecto", &TipografiaOpts{}, ""},
		{"só tamaño", &TipografiaOpts{TamanoPt: 13}, "\\fontsize{13pt}{15.6pt}\\selectfont\n"},
		{"tamaño + interliñado", &TipografiaOpts{TamanoPt: 12, Interlinado: 1.5}, "\\fontsize{12pt}{21.6pt}\\selectfont\n"},
		{"só interliñado", &TipografiaOpts{Interlinado: 2}, "\\linespread{2}\\selectfont\n"},
	}
	for _, c := range casos {
		if got := tipografiaCorpo(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", c.nome, got, c.want)
		}
	}
}

func TestTipografiaPreambulo(t *testing.T) {
	if got := tipografiaPreambulo(nil, "pdflatex"); got != "" {
		t.Errorf("nil: got %q", got)
	}
	if got := tipografiaPreambulo(&TipografiaOpts{Fonte: "inventada"}, "pdflatex"); got != "" {
		t.Errorf("preset descoñecido: got %q", got)
	}
	if got := tipografiaPreambulo(&TipografiaOpts{Fonte: "serif"}, "pdflatex"); !strings.Contains(got, "mathptmx") {
		t.Errorf("pdflatex serif debería cargar mathptmx: %q", got)
	}
	if got := tipografiaPreambulo(&TipografiaOpts{Fonte: "serif"}, "xelatex"); !strings.Contains(got, "\\setmainfont{TeX Gyre Termes}") {
		t.Errorf("xelatex serif debería usar \\setmainfont: %q", got)
	}
	if got := tipografiaPreambulo(&TipografiaOpts{Fonte: "sans"}, "pdflatex"); !strings.Contains(got, "helvet") {
		t.Errorf("pdflatex sans debería cargar helvet: %q", got)
	}

	// Fonte do sistema (nome de familia calquera): \setmainfont só con
	// motor Unicode; nada con pdflatex.
	if got := tipografiaPreambulo(&TipografiaOpts{Fonte: "DejaVu Serif"}, "xelatex"); got != "\\setmainfont{DejaVu Serif}\n" {
		t.Errorf("fonte do sistema en xelatex: %q", got)
	}
	if got := tipografiaPreambulo(&TipografiaOpts{Fonte: "DejaVu Serif"}, "pdflatex"); got != "" {
		t.Errorf("fonte do sistema en pdflatex debería dar '': %q", got)
	}
	// Limpeza contra inxección de LaTeX no nome.
	if got := tipografiaPreambulo(&TipografiaOpts{Fonte: "X}\\input{/etc/passwd}{"}, "xelatex"); strings.ContainsAny(got, "\\}{") && !strings.HasPrefix(got, "\\setmainfont{") {
		t.Errorf("nome de fonte sen sanear: %q", got)
	}
}

func TestEsFonteDoSistema(t *testing.T) {
	casos := map[string]bool{"": false, "serif": false, "palatino": false, "DejaVu Sans": true, "Cambria": true}
	for f, want := range casos {
		if got := esFonteDoSistema(f); got != want {
			t.Errorf("esFonteDoSistema(%q) = %v, want %v", f, got, want)
		}
	}
}
