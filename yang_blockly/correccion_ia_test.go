package main

import (
	"strings"
	"testing"
)

func TestParseCorreccionRespostaOK(t *testing.T) {
	raw := "```json\n" + `{
	  "resultados": [
	    {"etiqueta": "Área", "formula": "base*altura/2"},
	    {"etiqueta": "Perímetro", "formula": "base+altura+hipo ="}
	  ],
	  "resolucion": "<p>Área = <EVAL>res_1</EVAL>.</p>",
	  "sen_formula": false,
	  "motivo": ""
	}` + "\n```"

	c, err := parseCorreccionResposta(raw)
	if err != nil {
		t.Fatalf("parseCorreccionResposta: %v", err)
	}
	if len(c.Resultados) != 2 {
		t.Fatalf("resultados = %d, quería 2", len(c.Resultados))
	}
	if c.Resultados[0].Formula != "base*altura/2" || c.Resultados[0].Etiqueta != "Área" {
		t.Errorf("resultado 0 inesperado: %+v", c.Resultados[0])
	}
	// o "=" solto ao final ten que quedar limpo (rompe o parser de Maxima).
	if c.Resultados[1].Formula != "base+altura+hipo" {
		t.Errorf("resultado 1 non se limpou o '=' final: %q", c.Resultados[1].Formula)
	}
	if c.SenFormula {
		t.Error("sen_formula debería ser false")
	}
}

func TestParseCorreccionRespostaSenFormula(t *testing.T) {
	raw := `Aquí tes a resposta:
	{"resultados": [], "resolucion": "", "sen_formula": true, "motivo": "é unha demostración"}`
	c, err := parseCorreccionResposta(raw)
	if err != nil {
		t.Fatalf("parseCorreccionResposta: %v", err)
	}
	if !c.SenFormula {
		t.Error("debería recoñecer sen_formula:true")
	}
	if c.Motivo != "é unha demostración" {
		t.Errorf("motivo = %q", c.Motivo)
	}
	if len(c.Resultados) != 0 {
		t.Errorf("non debería haber resultados, hai %d", len(c.Resultados))
	}
}

func TestParseCorreccionRespostaBaleiraEErro(t *testing.T) {
	// Nin resultados, nin resolución, nin sen_formula -> erro (a IA non
	// devolveu nada aproveitable).
	if _, err := parseCorreccionResposta(`{"resultados": [], "resolucion": "", "sen_formula": false}`); err == nil {
		t.Error("esperaba erro cunha resposta baleira")
	}
	// JSON roto -> erro con contexto (formatoInesperadoErr).
	if _, err := parseCorreccionResposta("non é json"); err == nil {
		t.Error("esperaba erro cun JSON non interpretable")
	}
}

func TestConstruirPeticionCorreccionInclueDiagnosticos(t *testing.T) {
	req := CorreccionRequest{
		Enunciado: "Calcula a área.",
		Variables: []string{"base: n1()$", "  ", "altura: n1()$"},
		Diagnosticos: []ProbaDiag{
			{Seed: 41, Expr: "1/(base-altura)", Valor: "inf", Problema: "division_cero"},
		},
	}
	msg := construirPeticionCorreccion(req)
	for _, frag := range []string{"Calcula a área.", "base: n1()$", "altura: n1()$", "division_cero", "semente 41", "FALLOU A VALIDACIÓN"} {
		if !strings.Contains(msg, frag) {
			t.Errorf("a petición non contén %q:\n%s", frag, msg)
		}
	}
	// a liña en branco das variables non debe aparecer coma bullet baleiro.
	if strings.Contains(msg, "  \n") {
		t.Error("colouse unha variable baleira na petición")
	}
}

func TestSystemPromptCorreccionRegras(t *testing.T) {
	for _, frag := range []string{"res_1, res_2", "NUNCA o número", "sen_formula", "obxecto JSON"} {
		if !strings.Contains(systemPromptCorreccion, frag) {
			t.Errorf("systemPromptCorreccion perdeu a regra %q", frag)
		}
	}
}
