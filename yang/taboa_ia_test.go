package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// servidorIAFalso: un provedor "gemini" de mentira que devolve o texto que
// lle dea `resposta(peticion)`, e garda o que lle chegou. Mesmo truco ca
// TestAsistenteIA (app_test.go).
func servidorIAFalso(t *testing.T, resposta func(corpo string) string) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	corpos := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		corpos = append(corpos, string(body))
		mu.Unlock()
		texto, _ := json.Marshal(resposta(string(body)))
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":`+string(texto)+`}]}}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &corpos
}

func algunCorpoConten(corpos []string, sub string) bool {
	for _, c := range corpos {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// O fluxo completo por lotes: IniciarExameTaboa pide SÓ o guión (unha
// chamada curta) e cada XerarLoteExameTaboa desenvolve o seu anaco, así que
// un exame de 5 saen en 1 + 3 chamadas pequenas no canto dunha longa.
func TestExamePorLotes(t *testing.T) {
	srv, corpos := servidorIAFalso(t, func(corpo string) string {
		if strings.Contains(corpo, "GUIÓN dun exame") {
			return `["derivada dunha polinómica", "recta tanxente", "máximos e mínimos",
			         "problema de optimización", "gráfica a partir da derivada"]`
		}
		// 2ª pasada: resposta final + resolución dun exercicio xa enunciado.
		if strings.Contains(corpo, "RESPOSTA E RESOLUCIÓN") {
			return `{"resultado":"<p><EVAL>a</EVAL> m</p>","resolucion":"<ol><li><b>Paso:</b> <MAT>x</MAT> = <EVAL>a</EVAL> m</li></ol>"}`
		}
		// 1ª pasada dun lote: só enunciado + variables (o número de
		// exercicios vai no prompt, aquí abonda con dous e que se recorten).
		return `[{"enunciado":"<p>A</p>","variables":[{"nome":"a","modo":"aleatoria","xer":"rango","min":1,"max":9,"paso":1}]},
		         {"enunciado":"<p>B</p>","variables":[]}]`
	})

	a := &App{settings: Settings{IAProvedor: "gemini", IAAPIKey: "clave", IAModel: "m", IABaseURL: srv.URL}}
	plan, err := a.IniciarExameTaboa("derivadas", 5, nil)
	if err != nil {
		t.Fatalf("IniciarExameTaboa: %v", err)
	}
	if plan.Sesion == "" || len(plan.Guion) != 5 {
		t.Fatalf("plan inesperado: %+v", plan)
	}

	// Lote de 2 exercicios.
	exs, err := a.XerarLoteExameTaboa(plan.Sesion, 0, 2)
	if err != nil {
		t.Fatalf("XerarLoteExameTaboa: %v", err)
	}
	if len(exs) != 2 || exs[0].Variables[0].Min != "1" {
		t.Fatalf("lote inesperado: %+v", exs)
	}
	// A 2ª pasada encheu resposta final e resolución de cada exercicio do lote.
	if exs[0].Resultado.Trim() == "" || exs[0].Resolucion.Trim() == "" ||
		exs[1].Resultado.Trim() == "" || exs[1].Resolucion.Trim() == "" {
		t.Fatalf("a 2ª pasada non encheu resultado/resolución: %+v", exs)
	}

	// Último lote: só queda 1 exercicio no guión aínda que se pidan 2, e o
	// que sobre da resposta da IA recórtase.
	exs, err = a.XerarLoteExameTaboa(plan.Sesion, 4, 2)
	if err != nil {
		t.Fatalf("último lote: %v", err)
	}
	if len(exs) != 1 {
		t.Fatalf("o último lote debería traer 1 exercicio, trouxo %d", len(exs))
	}

	// O prompt do lote leva o guión enteiro e di cales tocan (búscase entre
	// todos os corpos: agora tamén hai chamadas da 2ª pasada polo medio).
	if !algunCorpoConten(*corpos, "DESENVOLVE SÓ o exercicio 5") {
		t.Errorf("ningún lote indicou que exercicio tocaba:\n%s", strings.Join(*corpos, "\n---\n"))
	}
	if !algunCorpoConten(*corpos, "recta tanxente") {
		t.Errorf("ningún lote levaba o guión completo:\n%s", strings.Join(*corpos, "\n---\n"))
	}

	// Fóra do guión: erro claro, non un exercicio inventado.
	if _, err := a.XerarLoteExameTaboa(plan.Sesion, 9, 1); err == nil {
		t.Error("un lote fóra do guión ten que dar erro")
	}
	// Sesión descoñecida (Yang reiniciado, sesión caducada).
	if _, err := a.XerarLoteExameTaboa("non-existe", 0, 1); err == nil {
		t.Error("unha sesión descoñecida ten que dar erro")
	}
}

// Con "indefinido" (0) é o guión quen decide cantos exercicios hai, e o tope
// de 20 respéctase igual ca en XerarExameTaboa.
func TestIniciarExameTaboaIndefinidoESaturacion(t *testing.T) {
	srv, _ := servidorIAFalso(t, func(string) string {
		partes := make([]string, 25)
		for i := range partes {
			partes[i] = `"exercicio"`
		}
		return "[" + strings.Join(partes, ",") + "]"
	})
	a := &App{settings: Settings{IAProvedor: "gemini", IAAPIKey: "clave", IAModel: "m", IABaseURL: srv.URL}}

	plan, err := a.IniciarExameTaboa("trigonometría", 0, nil)
	if err != nil {
		t.Fatalf("IniciarExameTaboa: %v", err)
	}
	if len(plan.Guion) != maxExerciciosPlan {
		t.Errorf("guión de %d; o tope é %d", len(plan.Guion), maxExerciciosPlan)
	}

	plan, err = a.IniciarExameTaboa("trigonometría", 4, nil)
	if err != nil {
		t.Fatalf("IniciarExameTaboa(4): %v", err)
	}
	if len(plan.Guion) != 4 {
		t.Errorf("pedíronse 4 exercicios e o guión trae %d", len(plan.Guion))
	}
}

func TestIniciarExameTaboaSenTemaNinModelo(t *testing.T) {
	a := &App{}
	if _, err := a.IniciarExameTaboa("   ", 5, nil); err == nil {
		t.Error("sen tema nin PDF modelo ten que dar erro antes de chamar á IA")
	}
}

// parseRespostaResolucionTaboa: mesma tolerancia ca o resto de parsers de IA
// - JSON envolto nunha frase e "resolucion" partida en lista de pasos.
func TestParseRespostaResolucionTaboa(t *testing.T) {
	raw := "Aquí tes: {\"resultado\": \"<p><EVAL>a*b</EVAL> m^2</p>\", " +
		"\"resolucion\": [\"<li>paso 1</li>\", \"<li>paso 2</li>\"]} listo."
	res, sol, err := parseRespostaResolucionTaboa(raw)
	if err != nil {
		t.Fatalf("parseRespostaResolucionTaboa: %v", err)
	}
	if res != "<p><EVAL>a*b</EVAL> m^2</p>" {
		t.Errorf("resultado inesperado: %q", res)
	}
	if !strings.Contains(sol, "paso 1") || !strings.Contains(sol, "paso 2") {
		t.Errorf("a resolución en lista non se uniu: %q", sol)
	}
}

// completarExercicioTaboa: se a 2ª chamada á IA falla, o exercicio devólvese
// TAL CAL (a xeración non se tomba por iso).
func TestCompletarExercicioTaboaFalloNonTomba(t *testing.T) {
	ex := ExercicioTaboa{Enunciado: "<p>Área dun rectángulo</p>", Resultado: "previo", Resolucion: "previo"}
	a := &App{} // sen settings de IA -> chamarIA falla de contado
	got := a.completarExercicioTaboa(ex, "")
	if got.Resultado.Trim() != "previo" || got.Resolucion.Trim() != "previo" {
		t.Errorf("cun fallo de IA debe conservarse o exercicio: %+v", got)
	}
}
