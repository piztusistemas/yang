package main

import (
	"context"
	"strings"
	"testing"
)

// TestProbarExercicios comproba que a lapela "Probas" caza os fallos
// matemáticos típicos: unha división por cero cando a semente fai coincidir
// dúas variables, e unha indeterminación (límite und). Precisa Maxima; sen
// el sáltase, coma exam_check_test.go.
func TestProbarExercicios(t *testing.T) {
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" {
		t.Skip("needs maxima")
	}

	src := strings.Join([]string{
		// ex 0: 1/(a-a) -> división por cero SEMPRE
		`<EX><HIDE>a: 5$</HIDE>Valor <EVAL>1/(a-a)</EVAL>.</EX>`,
		// ex 1: limpo
		`<EX><HIDE>b: 3$</HIDE>Dobre <EVAL>2*b</EVAL>.<RESP><EVAL>2*b</EVAL></RESP></EX>`,
		// ex 2: límite indeterminado
		`<EX>Límite <EVAL>limit(sin(x)/x - 1/x, x, 0)</EVAL>.</EX>`,
	}, "\n\n")

	res, err := a.ProbarExercicios(ProbaRequest{Source: src, SeedBase: 1, NSeeds: 4})
	if err != nil {
		t.Fatalf("ProbarExercicios: %v", err)
	}
	if len(res.Exercicios) != 3 {
		t.Fatalf("agardábanse 3 exercicios, hai %d", len(res.Exercicios))
	}

	if res.Exercicios[0].NFallos == 0 {
		t.Errorf("ex 0 (1/(a-a)) debería fallar en todas as sementes, NFallos=0")
	}
	if !temProblema(res.Exercicios[0].Diagnosticos, "division_cero", "erro_maxima", "indeterminado", "infinito") {
		t.Errorf("ex 0: diagnóstico inesperado: %+v", res.Exercicios[0].Diagnosticos)
	}

	if res.Exercicios[1].NFallos != 0 {
		t.Errorf("ex 1 (limpo) non debería ter fallos: %+v", res.Exercicios[1].Diagnosticos)
	}

	if res.Exercicios[2].NFallos == 0 {
		t.Errorf("ex 2 (límite und) debería fallar")
	}
}

// TestProbarExercicios_CadeaIgualdades: un exercicio cuxos <EVAL> son cadeas
// de igualdades escritas pola man ("I = V/R = 230/46 = 5") NON pode dar
// erro - cas renderízaas como texto, sen mandalas a Maxima (mathProseAsText).
// Regresión: antes probábase con m.Send() cru e Maxima devolvía "incorrect
// syntax: Found LOGICAL expression where ALGEBRAIC expression".
func TestProbarExercicios_CadeaIgualdades(t *testing.T) {
	a := &App{}
	a.startup(context.Background())
	if strings.TrimSpace(a.maximaPath) == "" {
		t.Skip("needs maxima")
	}
	src := strings.Join([]string{
		`<EX><HIDE>V: 230$ R: 46$</HIDE>`,
		`<EVAL>I = V/R = 230/46 = 5</EVAL>`,
		`<EVAL>P = V*I = 230*5 = 1150</EVAL>`,
		`<EVAL>E = 138 * 3.6 * 10^6 = 4.968 * 10^8</EVAL></EX>`,
	}, "\n")
	res, err := a.ProbarExercicios(ProbaRequest{Source: src, SeedBase: 1, NSeeds: 3})
	if err != nil {
		t.Fatalf("ProbarExercicios: %v", err)
	}
	if res.Exercicios[0].NFallos != 0 {
		t.Errorf("as cadeas de igualdades non deben marcarse como erro: %+v", res.Exercicios[0].Diagnosticos)
	}
}

func temProblema(ds []ProbaDiag, algún ...string) bool {
	set := map[string]bool{}
	for _, p := range algún {
		set[p] = true
	}
	for _, d := range ds {
		if set[d.Problema] {
			return true
		}
	}
	return false
}

// TestClassificar non precisa Maxima: comproba a heurística de clasificación
// sobre cadeas de saída típicas.
func TestClassificar(t *testing.T) {
	casos := []struct {
		val  string
		want string
	}{
		{"und", "indeterminado"},
		{"inf", "infinito"},
		{"minf", "infinito"},
		{"3/4", ""},
		{"", "baleiro"},
		{"expt: undefined: 0 to a negative exponent.", "indeterminado"},
		{"Division by zero", "division_cero"},
		{"2*%i+1", "complexo"},
		{"42", ""},
	}
	for _, c := range casos {
		if got := classificar(c.val, nil); got != c.want {
			t.Errorf("classificar(%q) = %q, want %q", c.val, got, c.want)
		}
	}
}
