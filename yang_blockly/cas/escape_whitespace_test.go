package cas

import (
	"strings"
	"testing"
)

// TestCollapseWhitespaceMataOsParrafos fixa a razón de existir de
// collapseWhitespace: unha mensaxe de erro de Maxima non pode conter unha
// liña en branco cando acaba dentro de \textcolor{red}{...}, porque ese
// \par aborta a compilación de TODO o exame (ver o comentario da función).
func TestCollapseWhitespaceMataOsParrafos(t *testing.T) {
	// Erro real de Maxima, tal cal o devolve (reproducido con
	// testdata/bloques.matex): fíxate na liña baleira antes de "(%i96)".
	erroReal := "sintaxis no correcta: Missing )\n" +
		"arredondar(ev(x^2+1;\n" +
		"                  ^\n" +
		"\n" +
		"(%i96)"

	got := collapseWhitespace(erroReal)

	if strings.Contains(got, "\n") {
		t.Errorf("quedou un salto de liña: %q", got)
	}
	if strings.Contains(got, "  ") {
		t.Errorf("quedaron espazos consecutivos: %q", got)
	}
	// O texto do erro ten que seguir aí: colapsar non é descartar.
	for _, agulla := range []string{"sintaxis no correcta", "Missing )", "arredondar(ev(x^2+1;", "(%i96)"} {
		if !strings.Contains(got, agulla) {
			t.Errorf("perdeuse %q da mensaxe de erro: %q", agulla, got)
		}
	}
}

func TestCollapseWhitespaceCasosSimples(t *testing.T) {
	casos := []struct{ entrada, esperado string }{
		{"", ""},
		{"   ", ""},
		{"ola", "ola"},
		{"  ola  ", "ola"},
		{"ola\n\nmundo", "ola mundo"},
		{"ola\t\tmundo", "ola mundo"},
		{"\n\nola\n\n", "ola"},
		{"a\r\n\r\nb", "a b"},
	}
	for _, c := range casos {
		if got := collapseWhitespace(c.entrada); got != c.esperado {
			t.Errorf("collapseWhitespace(%q) = %q, esperaba %q", c.entrada, got, c.esperado)
		}
	}
}
