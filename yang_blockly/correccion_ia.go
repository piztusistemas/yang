package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// correccion_ia.go: xeración con IA da CORRECCIÓN dun exercicio - o resultado
// final (como fórmulas) e a resolución paso a paso - a partir SÓ do enunciado
// e das variables xa declaradas. É o motor do botón "Reorganizar práctica"
// (frontend/src/reorganizar.js): a diferenza de XerarExercicioTaboa
// (taboa_ia.go), que inventa un exercicio enteiro, aquí o enunciado xa está
// FIXO e só se deriva del a corrección, apartado a apartado.
//
// A clave da fiabilidade: a IA devolve FÓRMULAS Maxima, non números. O
// orquestrador de JS mételas no <HIDE> como res_1, res_2... e tanto <RESP>
// coma <SOL> as referencian, así que Maxima calcula os valores e non poden
// discrepar. Despois valídase todo en varias sementes (ProbarExercicios) e,
// se algo dá inf/indeterminado/erro, os diagnósticos vólvense mandar aquí en
// Diagnosticos para unha rolda de arranxo.

// CorreccionRequest é o que manda reorganizar.js por cada exercicio.
type CorreccionRequest struct {
	Enunciado    string          `json:"enunciado"`    // texto do enunciado (pode levar <EVAL>/<MAT>); NON se cambia
	Variables    []string        `json:"variables"`    // liñas Maxima do <HIDE> (table-serialize varToMaxima), en orde
	Resolucion   string      `json:"resolucion"`   // resolución previa, se a hai (punto de partida a mellorar)
	Diagnosticos []ProbaDiag `json:"diagnosticos"` // fallas da validación anterior, para o arranxo (ver proba.go)
}

// ResultadoApartado: un apartado do resultado final, coma FÓRMULA (nunca o
// número xa calculado).
type ResultadoApartado struct {
	Etiqueta string `json:"etiqueta"`
	Formula  string `json:"formula"`
}

// CorreccionExercicio é a resposta xa parseada e saneada. SenFormula==true
// (cun Motivo) significa "este exercicio non ten resultado calculable"
// (demostración, resposta aberta...): o orquestrador déixao intacto.
type CorreccionExercicio struct {
	Resultados []ResultadoApartado `json:"resultados"`
	Resolucion string              `json:"resolucion"`
	SenFormula bool                `json:"senFormula"`
	Motivo     string              `json:"motivo"`
}

// XerarCorreccionTaboa: unha chamada á IA por exercicio. Ámbito pequeno a
// propósito (un exercicio, non o exame enteiro) - é o que fai a xeración
// fiable e permite reintentar só o que falla.
func (a *App) XerarCorreccionTaboa(req CorreccionRequest) (CorreccionExercicio, error) {
	if strings.TrimSpace(req.Enunciado) == "" {
		return CorreccionExercicio{}, fmt.Errorf("o exercicio non ten enunciado que corrixir")
	}
	raw, err := chamarIA(a.settings, systemPromptCorreccion, construirPeticionCorreccion(req))
	if err != nil {
		return CorreccionExercicio{}, err
	}
	return parseCorreccionResposta(raw)
}

// construirPeticionCorreccion arma a mensaxe de usuario: enunciado fixo,
// variables dispoñibles, resolución previa (se a hai) e diagnósticos da
// rolda anterior (se é un reintento de arranxo).
func construirPeticionCorreccion(req CorreccionRequest) string {
	var b strings.Builder
	b.WriteString("ENUNCIADO (non o cambies):\n")
	b.WriteString(strings.TrimSpace(req.Enunciado))
	b.WriteString("\n\n")

	var vars []string
	for _, v := range req.Variables {
		if s := strings.TrimSpace(v); s != "" {
			vars = append(vars, s)
		}
	}
	if len(vars) > 0 {
		b.WriteString("VARIABLES XA DECLARADAS (usa estes nomes nas fórmulas):\n")
		for _, v := range vars {
			b.WriteString("  " + v + "\n")
		}
	} else {
		b.WriteString("VARIABLES XA DECLARADAS: (ningunha)\n")
	}
	b.WriteString("\n")

	if s := strings.TrimSpace(req.Resolucion); s != "" {
		b.WriteString("RESOLUCIÓN ACTUAL (consérvaa se está ben, arránxaa se ten fallos):\n")
		b.WriteString(s)
		b.WriteString("\n\n")
	}

	if len(req.Diagnosticos) > 0 {
		b.WriteString("A ÚLTIMA VERSIÓN FALLOU A VALIDACIÓN. Cambia as fórmulas para que NON volva pasar:\n")
		for _, d := range req.Diagnosticos {
			linea := fmt.Sprintf("  - %s", d.Problema)
			if d.Expr != "" {
				linea += fmt.Sprintf(": na expresión %q", d.Expr)
			}
			if d.Valor != "" {
				linea += fmt.Sprintf(" deu %q", d.Valor)
			}
			linea += fmt.Sprintf(" (semente %d)\n", d.Seed)
			b.WriteString(linea)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// parseCorreccionResposta le o obxecto JSON da IA e sanéao. Función pura (á
// parte de XerarCorreccionTaboa) para poder probala sen tocar a rede, mesmo
// criterio ca parseAsistenteResposta (app.go) e parsearPlantillaIA
// (plantillas_ia.go).
func parseCorreccionResposta(raw string) (CorreccionExercicio, error) {
	// Campos textoIA/booleanoIA (jsonia.go): a IA escribe ás veces os
	// escalares no seu tipo natural ("formula": 12, "sen_formula": "true") e
	// con `string`/`bool` puros iso tiraba coa corrección enteira.
	var resp struct {
		Resultados []struct {
			Etiqueta textoIA `json:"etiqueta"`
			Formula  textoIA `json:"formula"`
		} `json:"resultados"`
		Resolucion textoIA    `json:"resolucion"`
		SenFormula booleanoIA `json:"sen_formula"`
		Motivo     textoIA    `json:"motivo"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(limparValadosMarkdown(raw))), &resp); err != nil {
		return CorreccionExercicio{}, formatoInesperadoErr(raw, err)
	}

	out := CorreccionExercicio{
		Resolucion: resp.Resolucion.Trim(),
		SenFormula: bool(resp.SenFormula),
		Motivo:     resp.Motivo.Trim(),
	}
	for _, r := range resp.Resultados {
		f := limparFormulaResultado(r.Formula.String())
		if f == "" {
			continue
		}
		out.Resultados = append(out.Resultados, ResultadoApartado{
			Etiqueta: r.Etiqueta.Trim(),
			Formula:  f,
		})
	}

	if out.SenFormula {
		// "sen fórmula" é unha resposta válida e completa por si soa.
		return out, nil
	}
	if len(out.Resultados) == 0 && out.Resolucion == "" {
		return CorreccionExercicio{}, fmt.Errorf("a IA non devolveu nin resultados nin resolución")
	}
	return out, nil
}

// limparFormulaResultado quita espazos e calquera "=" solto ao final: unha
// fórmula rematada en "=" rompe SEMPRE o parser de Maxima (regra do
// chuletaMatexe), e a IA ás veces esvara. Non toca "=" internos.
func limparFormulaResultado(s string) string {
	s = strings.TrimSpace(s)
	for strings.HasSuffix(s, "=") {
		s = strings.TrimSpace(strings.TrimSuffix(s, "="))
	}
	return s
}
