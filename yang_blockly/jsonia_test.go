package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// O caso real que rompía a xeración dun exame enteiro: o prompt pide
// "min": "6" e o modelo escribe "min": 6.
func TestExercicioTaboaAceptaNumerosOndeSePediuTexto(t *testing.T) {
	raw := `[{
	  "enunciado": "<p>Unha resistencia de {r} Ω conéctase a {v} V.</p>",
	  "variables": [
	    {"nome": "v", "modo": "aleatoria", "xer": "rango", "min": 6, "max": 24, "paso": 3},
	    {"nome": "r", "modo": "fixa", "valor": 100},
	    {"nome": "i", "modo": "auto", "expr": "v/r"}
	  ],
	  "resultado": "<EVAL>i</EVAL> A",
	  "resolucion": "<p>I = V/R</p>"
	}]`
	var exs []ExercicioTaboa
	if err := json.Unmarshal([]byte(raw), &exs); err != nil {
		t.Fatalf("non debería fallar: %v", err)
	}
	if len(exs) != 1 || len(exs[0].Variables) != 3 {
		t.Fatalf("estrutura inesperada: %+v", exs)
	}
	v := exs[0].Variables[0]
	if v.Min != "6" || v.Max != "24" || v.Paso != "3" {
		t.Errorf("rango mal lido: min=%q max=%q paso=%q", v.Min, v.Max, v.Paso)
	}
	if exs[0].Variables[1].Valor != "100" {
		t.Errorf("valor fixo mal lido: %q", exs[0].Variables[1].Valor)
	}
}

func TestTextoIAToleraListasNullEObxectos(t *testing.T) {
	var ex ExercicioTaboa
	raw := `{
	  "enunciado": ["<p>Primeira parte.</p>", "<p>Segunda parte.</p>"],
	  "variables": null,
	  "resultado": null,
	  "resolucion": true
	}`
	if err := json.Unmarshal([]byte(raw), &ex); err != nil {
		t.Fatalf("non debería fallar: %v", err)
	}
	if ex.Enunciado != "<p>Primeira parte.</p>\n<p>Segunda parte.</p>" {
		t.Errorf("lista non unida: %q", ex.Enunciado)
	}
	if ex.Resultado != "" {
		t.Errorf("null debería quedar baleiro: %q", ex.Resultado)
	}
	if ex.Resolucion != "true" {
		t.Errorf("booleano coma texto: %q", ex.Resolucion)
	}
	if ex.Variables != nil {
		t.Errorf("variables null deberían ser nil: %+v", ex.Variables)
	}
}

func TestVariablesIAAceptaObxectoSoltoEMapa(t *testing.T) {
	var ex ExercicioTaboa
	if err := json.Unmarshal([]byte(`{"variables": {"nome":"a","modo":"aleatoria","xer":"z0"}}`), &ex); err != nil {
		t.Fatalf("obxecto solto: %v", err)
	}
	if len(ex.Variables) != 1 || ex.Variables[0].Nome != "a" {
		t.Fatalf("obxecto solto mal lido: %+v", ex.Variables)
	}

	var ex2 ExercicioTaboa
	if err := json.Unmarshal([]byte(`{"variables": {"b": {"modo":"fixa","valor":2}, "a": {"modo":"aleatoria","xer":"n1"}}}`), &ex2); err != nil {
		t.Fatalf("mapa: %v", err)
	}
	if len(ex2.Variables) != 2 || ex2.Variables[0].Nome != "a" || ex2.Variables[1].Nome != "b" {
		t.Fatalf("mapa mal lido (espérase ordenado por nome): %+v", ex2.Variables)
	}
}

func TestTextoIASegueFallandoConJSONRoto(t *testing.T) {
	var ex ExercicioTaboa
	if err := json.Unmarshal([]byte(`{"enunciado": <p>sen comiñas</p>}`), &ex); err == nil {
		t.Error("un JSON realmente roto ten que seguir dando erro")
	}
}

func TestListaTextosIA(t *testing.T) {
	var l listaTextosIA
	if err := json.Unmarshal([]byte(`["un", 2, "  ", null, ["tres", "catro"]]`), &l); err != nil {
		t.Fatalf("non debería fallar: %v", err)
	}
	if len(l) != 3 || l[0] != "un" || l[1] != "2" || l[2] != "tres\ncatro" {
		t.Fatalf("guión mal lido: %#v", l)
	}
}

func TestBooleanoIA(t *testing.T) {
	casos := map[string]bool{
		`true`: true, `false`: false, `"true"`: true, `"Si"`: true,
		`"non"`: false, `1`: true, `0`: false, `null`: false,
	}
	for entrada, agardado := range casos {
		var b booleanoIA
		if err := json.Unmarshal([]byte(entrada), &b); err != nil {
			t.Fatalf("%s: %v", entrada, err)
		}
		if bool(b) != agardado {
			t.Errorf("%s -> %v, agardábase %v", entrada, bool(b), agardado)
		}
	}
}

// TestRepararEscapesJSON: o report que motivou isto - a IA devolveu un
// exercicio cun <TIKZ> e escribiu "\draw" sen dobrar a barra, así que o
// JSON petaba con "invalid character 'd' in string escape code" e caía a
// xeración enteira do exame.
func TestRepararEscapesJSON(t *testing.T) {
	roto := `[{"enunciado":"<TIKZ>\draw (0,0) -- (1,1); \node at (0.5,0.5) {$R = <EVAL>r</EVAL>\,\Omega$};</TIKZ>"}]`
	if json.Valid([]byte(roto)) {
		t.Fatal("o caso de proba xa era JSON válido; non proba nada")
	}
	reparado := repararEscapesJSON(roto)
	var v []struct {
		Enunciado string `json:"enunciado"`
	}
	if err := json.Unmarshal([]byte(reparado), &v); err != nil {
		t.Fatalf("tras reparar debía parsear: %v\n%s", err, reparado)
	}
	if len(v) != 1 || !strings.Contains(v[0].Enunciado, `\draw`) || !strings.Contains(v[0].Enunciado, `\Omega`) {
		t.Fatalf("o contido TikZ non sobreviviu á reparación: %q", v[0].Enunciado)
	}

	// Un salto de liña cru dentro dun string tamén se arranxa.
	conSalto := "{\"a\":\"liña1\nliña2\"}"
	var m map[string]string
	if err := json.Unmarshal([]byte(repararEscapesJSON(conSalto)), &m); err != nil {
		t.Fatalf("salto de liña cru non reparado: %v", err)
	}
	if m["a"] != "liña1\nliña2" {
		t.Errorf("salto de liña mal reparado: %q", m["a"])
	}

	// Un JSON que xa era válido non se toca (os escapes correctos consérvanse).
	bo := `{"x":"a\\b\tc\"d","y":"ñ"}`
	if got := repararEscapesJSON(bo); got != bo {
		t.Errorf("un JSON xa válido non se debe cambiar:\n  in : %s\n  out: %s", bo, got)
	}
}
