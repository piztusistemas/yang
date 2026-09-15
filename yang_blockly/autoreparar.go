package main

import (
	"fmt"
	"strings"
	"sync"
)

// autoreparar.go: soporte de backend para a AUTOREPARACIÓN por exercicio da
// interface de Táboa (frontend/src/auto-reparar.js).
//
// Idea: cando un exame xerado coa IA non compila, non se rexenera o exame
// enteiro - illa que exercicio(s) fallan (DiagnosticarCompilacionExercicios)
// e pídeselle á IA que rexenere SÓ eses, pasándolle o erro de compilación
// (RexenerarExercicioTaboa), ata Settings.MaxRechamadasIAResolto veces. Os
// que sigan sen compilar déixanse cunha caixa de aviso visible para que o
// resto do exame se poida usar igual.
//
// O bucle en si vive no frontend (mesmo patrón ca reorganizar.js): parte o
// modelo da táboa, chama a estes dous bindings por exercicio, reconstrúe o
// .matex e recompila. Aquí só están as dúas operacións que precisan Maxima /
// LaTeX / a IA.

// InfoCompilacionExercicio: resultado de compilar UN exercicio en solitario.
type InfoCompilacionExercicio struct {
	Indice int    `json:"indice"`
	Ok     bool   `json:"ok"`
	Erro   string `json:"erro"` // "" se Ok; se non, a liña de erro de LaTeX/Maxima xa limpa
}

// DiagnosticarCompilacionExercicios compila CADA <EX>...</EX> do documento
// por separado (co mesmo motor, semente e modo ca a xeración real) e di
// cales fallan e con que erro. Faise en paralelo acoutado: un exame de 10
// exercicios son 10 compilacións, pero só se lanza isto cando a compilación
// completa xa fallou, así que o custo asúmese a cambio de saber EXACTAMENTE
// que exercicio arranxar.
//
// Se o documento non ten <EX> (documento solto / legacy) devólvese un único
// InfoCompilacionExercicio co índice 0.
func (a *App) DiagnosticarCompilacionExercicios(req GeneratePDFRequest) ([]InfoCompilacionExercicio, error) {
	chunks := extraerExercicios(req.Source)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("o documento non ten nada que compilar")
	}

	infos := make([]InfoCompilacionExercicio, len(chunks))
	const traballadores = 3
	sem := make(chan struct{}, traballadores)
	var wg sync.WaitGroup

	for i, inner := range chunks {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, inner string) {
			defer wg.Done()
			defer func() { <-sem }()

			sub := req
			sub.Source = "<EX>\n" + inner + "\n</EX>\n"
			sub.Iterations = 1 // unha variante abonda para saber se compila

			res, err := a.GeneratePDF(sub)
			info := InfoCompilacionExercicio{Indice: i, Ok: err == nil}
			if err != nil {
				info.Erro = mensaxeErroCompilacion(res.Log, err)
			}
			infos[i] = info
		}(i, inner)
	}
	wg.Wait()
	return infos, nil
}

// mensaxeErroCompilacion queda coa liña de erro máis útil: a de LaTeX
// (extraerErroPDFLatex) se a hai no log, se non a propia mensaxe de erro de
// Go (que xa leva o erro de Maxima cando o fallo é anterior á compilación).
func mensaxeErroCompilacion(log string, err error) string {
	if s := strings.TrimSpace(extraerErroPDFLatex(log)); s != "" {
		return s
	}
	if err != nil {
		return err.Error()
	}
	return "erro descoñecido de compilación"
}

// RexenerarRequest é a entrada de RexenerarExercicioTaboa: o exercicio que
// non compila (tal cal o modelo da táboa) máis a mensaxe de erro que
// devolveu a compilación.
type RexenerarRequest struct {
	Exercicio ExercicioTaboa `json:"exercicio"`
	Erro      string         `json:"erro"`
	// Intento: 1 = primeira rechamada, 2 = segunda... só para o texto do
	// prompt (que a IA saiba que xa se intentou antes).
	Intento int `json:"intento"`
}

// systemPromptRexenerarExercicio: a IA recibe un exercicio de táboa que NON
// compilou e o erro literal, e ten que devolver o MESMO exercicio (mesmo
// enunciado e mesma intención) coa sintaxe arranxada. Reutiliza
// basePromptTaboa (regras de <MAT>/<EVAL>/<TIKZ>...) e engade a tarefa.
func systemPromptRexenerarExercicio() string {
	return basePromptTaboa() + `

TAREFA ESPECIAL: o exercicio que recibes NON COMPILOU. Dáseche a mensaxe de
erro literal de LaTeX/Maxima. Devolve o MESMO exercicio (mesmo enunciado,
mesmos datos, mesma dificultade) pero coa causa do erro corrixida. Cambia o
MÍNIMO imprescindible: non reformules o enunciado nin cambies os números se
non é o que falla.

Erros típicos e como se arranxan:
- "Missing $ inserted" / "extra }": hai un "_", "^", "&", "%" ou "\" solto no
  texto (fóra de <MAT>/<EVAL>) - quítao ou escríbeo ben; ou hai unha
  expresión matemática que ten que ir dentro de <MAT>/<EVAL> e non o está.
- "Undefined control sequence" / "Environment ... undefined": hai LaTeX cru
  (\frac, \begin{...}, \sqrt...) onde debía haber sintaxe Maxima dentro de
  <MAT>/<EVAL>, ou HTML normal.
- "< is not a prefix operator" / "incorrect syntax": dentro dun <MAT>/<EVAL>
  hai algo que non é unha expresión Maxima completa (un "=" ao final, unha
  etiqueta <EVAL> aniñada, un apóstrofo, unha barra invertida).
- "\language@active@arg" / "Incomplete \iffalse": normalmente un <TIKZ> mal
  pechado ou con código que non é TikZ válido.

Devolve un ARRAY JSON cun ÚNICO elemento (o exercicio arranxado), coa mesma
forma de sempre ("enunciado" + "variables"). SÓ o array, sen texto arredor.`
}

// RexenerarExercicioTaboa pídelle á IA a versión arranxada dun exercicio
// que non compila. Devolve o exercicio xa coa 2ª pasada feita (resposta
// final + resolución), listo para substituír no modelo da táboa.
func (a *App) RexenerarExercicioTaboa(req RexenerarRequest) (ExercicioTaboa, error) {
	enun := req.Exercicio.Enunciado.Trim()
	if enun == "" {
		return ExercicioTaboa{}, fmt.Errorf("o exercicio non ten enunciado que arranxar")
	}
	erro := strings.TrimSpace(req.Erro)
	if erro == "" {
		erro = "(sen detalle; a compilación fallou nese exercicio)"
	}

	// resultadoAnterior/resolucionAnterior (se hai): o erro de compilación
	// pode vir de calquera dos tres campos do exercicio, non só do
	// enunciado - un <TIKZ> mal pechado escrito na RESOLUCIÓN adoita ser o
	// caso real (ver systemPromptRexenerarExercicio, "Incomplete \iffalse").
	// Antes só se mandaba o enunciado + o erro, así que cando o problema
	// estaba na resolución a IA non tiña xeito de sabelo: non a vía, e
	// autorreparación repetía o mesmo fallo ata esgotar as rechamadas.
	resultadoAnterior := req.Exercicio.Resultado.Trim()
	resolucionAnterior := req.Exercicio.Resolucion.Trim()

	var b strings.Builder
	b.WriteString("EXERCICIO QUE NON COMPILA:\n")
	b.WriteString(enun)
	b.WriteString("\n\nVARIABLES DECLARADAS:\n")
	if len(req.Exercicio.Variables) == 0 {
		b.WriteString("  (ningunha)\n")
	} else {
		for _, v := range req.Exercicio.Variables {
			if v.Nome.Trim() == "" {
				continue
			}
			b.WriteString("  - " + descricionVariableTaboa(v) + "\n")
		}
	}
	if resultadoAnterior != "" {
		b.WriteString("\nRESULTADO ACTUAL (pode ser aquí onde está o erro):\n")
		b.WriteString(resultadoAnterior)
	}
	if resolucionAnterior != "" {
		b.WriteString("\nRESOLUCIÓN ACTUAL (pode ser aquí onde está o erro):\n")
		b.WriteString(resolucionAnterior)
	}
	b.WriteString("\n\nERRO DE COMPILACIÓN:\n")
	b.WriteString(erro)
	if req.Intento > 1 {
		fmt.Fprintf(&b, "\n\n(Este é o intento %d: o arranxo anterior seguiu sen compilar. Proba unha solución distinta.)", req.Intento)
	}

	var exs []ExercicioTaboa
	if err := chamarIAJSON(a.settings, systemPromptRexenerarExercicio(), b.String(), extractJSONArray, &exs); err != nil {
		return ExercicioTaboa{}, err
	}
	if len(exs) == 0 || exs[0].Enunciado.Trim() == "" {
		return ExercicioTaboa{}, fmt.Errorf("a IA non devolveu un exercicio arranxado")
	}
	// 2ª pasada: refacer resposta final + resolución sobre o enunciado xa
	// arranxado (o mesmo camiño ca a xeración normal, ver taboa_ia.go) -
	// pero non ás cegas: pásaselle o resultado/resolución anteriores máis o
	// erro literal (contextoErro) para que non repita o mesmo fallo se o
	// enunciado non era realmente o problema.
	var contextoErro strings.Builder
	if resultadoAnterior != "" || resolucionAnterior != "" {
		contextoErro.WriteString("INTENTO ANTERIOR (fallou ao compilar - NON repitas o mesmo erro):\n")
		if resultadoAnterior != "" {
			contextoErro.WriteString("RESULTADO:\n" + resultadoAnterior + "\n")
		}
		if resolucionAnterior != "" {
			contextoErro.WriteString("RESOLUCIÓN:\n" + resolucionAnterior + "\n")
		}
		contextoErro.WriteString("\n")
	}
	contextoErro.WriteString("ERRO DE COMPILACIÓN:\n" + erro)
	return a.completarExercicioTaboa(exs[0], contextoErro.String()), nil
}
