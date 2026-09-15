package main

import (
	"fmt"
	"regexp"
	"strings"

	"matexe-wails/cas"
)

// proba.go: lapela "Probas" da interface de táboa (frontend/src/table-probes.js).
// Executa cada exercicio do documento en varias sementes e comproba que
// TODAS as expresións (<EVAL>, <MAT>, e as de dentro de <RESP>/<SOL>) dan
// valores coherentes - sen infinitos, indeterminacións, divisións por cero
// nin erros de Maxima. É unha rede de seguridade antes de repartir un exame:
// un exercicio que só rompe cunha semente concreta é moi difícil de cazar á
// man probando "a ollo".
//
// Non substitúe a Generate/GeneratePDF: reutiliza o mesmo motor (cas.Maxima)
// pero avalía expresión a expresión para poder dicir EXACTAMENTE cal falla,
// con que semente e con que valor.

type ProbaRequest struct {
	Source   string `json:"source"`
	SeedBase int    `json:"seedBase"`
	NSeeds   int    `json:"nSeeds"`
	CodeIni  string `json:"codeIni"`
}

// ProbaDiag: un problema concreto atopado nunha semente concreta.
type ProbaDiag struct {
	Seed     int    `json:"seed"`
	Expr     string `json:"expr"`
	Valor    string `json:"valor"`
	Problema string `json:"problema"` // infinito|indeterminado|nan|division_cero|erro_maxima|baleiro|complexo|timeout
}

type ProbaExercicio struct {
	Indice       int         `json:"indice"`
	NProbas      int         `json:"nProbas"`
	NFallos      int         `json:"nFallos"` // sementes nas que apareceu polo menos un problema
	Diagnosticos []ProbaDiag `json:"diagnosticos"`
}

type ProbaResult struct {
	Exercicios []ProbaExercicio `json:"exercicios"`
}

var (
	exBloqueRe   = regexp.MustCompile(`(?s)<EX>(.*?)</EX>`)
	// Palabras "síntoma" con fronteira: para non confundir "inf" con
	// "\infty" nin "nan" con "\tan"/"significant". \infty conta (LaTeX de inf).
	palabraInfRe = regexp.MustCompile(`(?i)(^|[^a-z0-9_])(minf|infinity|infty|inf)($|[^a-z0-9_])`)
	palabraUndRe = regexp.MustCompile(`(?i)(^|[^a-z0-9_])(undefined|und|ind)($|[^a-z0-9_])`)
	palabraNanRe = regexp.MustCompile(`(?i)(^|[^a-z0-9_])nan($|[^a-z0-9_])`)
	imaxinariaRe = regexp.MustCompile(`(^|[^a-zA-Z0-9_])%i($|[^a-zA-Z0-9_])`)
	espazosRe    = regexp.MustCompile(`\s+`)
)

// ProbarExercicios é o binding que chama table-probes.js.
func (a *App) ProbarExercicios(req ProbaRequest) (ProbaResult, error) {
	res := ProbaResult{Exercicios: []ProbaExercicio{}}

	if strings.TrimSpace(a.maximaPath) == "" {
		return res, fmt.Errorf("non se atopou Maxima. Configura a ruta en Opcións")
	}
	chunks := extraerExercicios(req.Source)
	if len(chunks) == 0 {
		return res, fmt.Errorf("o documento non ten nada que probar")
	}

	nSeeds := req.NSeeds
	if nSeeds < 1 {
		nSeeds = 25
	}
	if nSeeds > 200 {
		nSeeds = 200
	}

	codeIni := req.CodeIni
	if strings.TrimSpace(codeIni) == "" {
		codeIni = a.settings.CodeIni
	}
	if strings.TrimSpace(codeIni) == "" {
		codeIni = defaultCodeIni
	}
	codeIniLines := splitLines(codeIni)

	sess, err := cas.Open(a.maximaPath)
	if err != nil {
		return res, fmt.Errorf("non se puido iniciar Maxima (%s): %w", a.maximaPath, err)
	}
	defer a.trackMaximaSession(sess)()
	m := cas.NewMaxima(sess)
	defer m.Close()
	m.SetTimeout(a.settings.TimeoutResolto())
	// "proba": as etiquetas <RESP>/<SOL> avalíanse (necesitamos comprobalas),
	// non se descartan coma na folla do alumnado. Ver cas.Maxima.RenderMode.
	m.RenderMode = "proba"

	if _, err := m.Send("linel:1024;display2d:false;simp:true;", false); err != nil {
		return res, err
	}

	for ei, chunk := range chunks {
		exRes := ProbaExercicio{Indice: ei, NProbas: nSeeds}
		vistos := map[string]bool{}

		for s := 0; s < nSeeds; s++ {
			seed := req.SeedBase + s

			if _, err := m.Send("reset();kill(all)", true); err != nil {
				return res, err
			}
			if err := m.SetDecimais(a.settings.DecimaisResolto()); err != nil {
				return res, err
			}
			for _, line := range codeIniLines {
				// Un erro de codeini xa se ve ao Xerar - aquí non se
				// interrompe a proba por iso.
				_, _ = m.Send(line, false)
			}
			if err := m.SetSeed(seed); err != nil {
				return res, err
			}

			problemaNestaSemente := false
			for _, d := range probarChunk(m, chunk, seed) {
				problemaNestaSemente = true
				key := d.Problema + "\x00" + d.Expr
				if !vistos[key] && len(exRes.Diagnosticos) < 8 {
					vistos[key] = true
					exRes.Diagnosticos = append(exRes.Diagnosticos, d)
				}
			}
			if problemaNestaSemente {
				exRes.NFallos++
			}
		}
		res.Exercicios = append(res.Exercicios, exRes)
	}
	return res, nil
}

// extraerExercicios devolve o texto interior de cada <EX>...</EX>. Se non hai
// ningún <EX> pero o documento ten contido, próbase o documento enteiro coma
// un só "exercicio" (caso legacy / documento solto).
func extraerExercicios(src string) []string {
	var out []string
	for _, m := range exBloqueRe.FindAllStringSubmatch(src, -1) {
		if strings.TrimSpace(m[1]) != "" {
			out = append(out, m[1])
		}
	}
	if len(out) == 0 && strings.TrimSpace(src) != "" {
		out = append(out, src)
	}
	return out
}

// grupoTag traduce un match de cas.TagPattern ao (kind, contido). Grupos:
// 1=MAT 2=EVAL 3=HIDE 4=PLOT 5=TEX 6=TIKZ 7=SISTEMA 8=RESP 9=SOL.
var nomesGrupoTag = []string{"MAT", "EVAL", "HIDE", "PLOT", "TEX", "TIKZ", "SISTEMA", "RESP", "SOL"}

func grupoTag(mt []string) (kind, contido string) {
	for i, k := range nomesGrupoTag {
		if mt[i+1] != "" {
			return k, mt[i+1]
		}
	}
	return "", ""
}

// probarChunk avalía as etiquetas dun exercicio EN ORDE (as <HIDE> definen
// variables que as <EVAL>/<RESP>/<SOL> posteriores usan) e devolve un
// ProbaDiag por cada resultado incoherente.
//
// CADA etiqueta pasa por m.ProcessTag - o MESMO camiño que a xeración real -
// NON por m.Send() cru: así unha cadea "I = V/R = 230/46 = 5" (que cas
// renderiza como texto, sen mandala a Maxima) non se marca como erro. As
// etiquetas que non producen un valor numérico (PLOT/TIKZ/TEX) sáltanse.
func probarChunk(m *cas.Maxima, chunk string, seed int) []ProbaDiag {
	var diags []ProbaDiag
	for _, mt := range cas.TagPattern.FindAllStringSubmatch(chunk, -1) {
		kind, contido := grupoTag(mt)
		if kind == "" || strings.TrimSpace(contido) == "" {
			continue
		}
		switch kind {
		case "PLOT", "TIKZ", "TEX":
			continue
		}
		val, err := m.ProcessTag(kind, contido)
		if kind == "HIDE" {
			// <HIDE> non produce saída (execútase en silencio): unha saída
			// baleira é o esperado, non un "resultado baleiro". Só conta se
			// ProcessTag devolve erro (sintaxe mala, variable sen definir
			// que rompe unha asignación posterior...).
			if err != nil {
				diags = append(diags, ProbaDiag{Seed: seed, Expr: resumo(contido), Problema: classificar("", err)})
			}
			continue
		}
		if p := classificar(val, err); p != "" {
			diags = append(diags, ProbaDiag{Seed: seed, Expr: resumo(contido), Valor: resumo(val), Problema: p})
		}
	}
	return diags
}

// classificar mira o que devolveu m.ProcessTag (o texto renderizado + o
// posible erro de Go) e decide se hai un resultado incoherente e de que
// tipo. Devolve "" se está ben.
//
// ProcessTag NON devolve erro de Go cando Maxima falla: mete a mensaxe de
// erro no propio texto de saída, marcada (fondo #fdd en HTML, \textcolor{red}
// en LaTeX). Por iso hai que mirar tanto sendErr coma o contido de `val`.
func classificar(val string, sendErr error) string {
	if sendErr != nil {
		if strings.Contains(strings.ToLower(sendErr.Error()), "timeout") {
			return "timeout"
		}
		if p := porPalabra(strings.ToLower(sendErr.Error())); p != "" {
			return p
		}
		return "erro_maxima"
	}
	v := strings.TrimSpace(val)
	if v == "" {
		return "baleiro"
	}
	low := strings.ToLower(v)

	// ProcessTag marcou isto como erro de Maxima (non é un valor normal).
	esErroMarcado := strings.Contains(low, cas.MarcaErroInline) ||
		strings.Contains(v, `\textcolor{red}`) ||
		strings.Contains(low, "incorrect syntax") ||
		strings.Contains(low, "has been generated") ||
		strings.Contains(low, "lisp error") ||
		strings.Contains(low, "-- an error")
	if esErroMarcado {
		if p := porPalabra(low); p != "" {
			return p
		}
		return "erro_maxima"
	}

	// Resultado "normal" pero incoherente.
	if p := porPalabra(low); p != "" {
		return p
	}
	if imaxinariaRe.MatchString(v) {
		return "complexo"
	}
	return ""
}

// porPalabra busca os síntomas concretos (infinito / indeterminación /
// división por cero / NaN) nunha cadea xa en minúsculas. "" se non hai.
func porPalabra(low string) string {
	switch {
	case strings.Contains(low, "division by zero"), strings.Contains(low, "divide by zero"),
		strings.Contains(low, "división por cero"), strings.Contains(low, "division by 0"):
		return "division_cero"
	case palabraInfRe.MatchString(low):
		return "infinito"
	case palabraUndRe.MatchString(low):
		return "indeterminado"
	case palabraNanRe.MatchString(low):
		return "nan"
	}
	return ""
}

func resumo(s string) string {
	s = strings.TrimSpace(espazosRe.ReplaceAllString(s, " "))
	if len(s) > 70 {
		return s[:69] + "…"
	}
	return s
}
