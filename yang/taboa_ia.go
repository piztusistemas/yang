package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Xeración con IA para a INTERFACE DE TÁBOA (frontend/src/table-editor.js).
//
// A diferenza de XerarExercicioIA/XerarExameIA (app.go), que devolven unha
// lista de "anacos" pensada para o editor de bloques e que a táboa tiña que
// repartir a ollo nos seus campos, aquí a IA devolve xa a estrutura da
// táboa: enunciado / variables / resultado final / resolución paso a paso.
// Así cada lapela ("Enunciados", "+ resultado final", "+ resolución") amosa
// exactamente o que lle toca, sen heurísticas.

// VariableTaboa reproduce a forma dunha variable do modelo da táboa
// (table-serialize.js): nome + modo, e segundo o modo un dos grupos de
// campos.
// Todos os campos son textoIA (jsonia.go) e non `string`: a IA escribe moitas
// veces os números coma números ("min": 6, non "min": "6") e antes iso
// abortaba a xeración enteira cun erro de unmarshal.
type VariableTaboa struct {
	Nome  textoIA `json:"nome"`
	Modo  textoIA `json:"modo"` // "fixa" | "aleatoria" | "auto"
	Valor textoIA `json:"valor,omitempty"`
	Expr  textoIA `json:"expr,omitempty"`
	Xer   textoIA `json:"xer,omitempty"` // n0..n2 | z0..z2 | q0..q2 | "rango"
	Min   textoIA `json:"min,omitempty"`
	Max   textoIA `json:"max,omitempty"`
	Paso  textoIA `json:"paso,omitempty"`
}

// ExercicioTaboa é un exercicio xa listo para a táboa.
type ExercicioTaboa struct {
	Enunciado  textoIA     `json:"enunciado"`
	Variables  variablesIA `json:"variables"`
	Resultado  textoIA     `json:"resultado"`  // -> columna "Resultado final" (<RESP>)
	Resolucion textoIA     `json:"resolucion"` // -> columna "Resolución" (<SOL>)
}

// systemPromptTaboa: cantos <= 0 -> exame indefinido; == 1 -> un exercicio;
// > 1 -> exactamente ese número.
func systemPromptTaboa(cantos int) string {
	base := basePromptTaboa()
	switch {
	case cantos <= 0:
		return base + `

Crea un EXAME COMPLETO sobre o tema indicado (varios exercicios, variados e
con dificultade progresiva cando teña sentido). Cantos exercicios: decídelo
ti segundo o que pida o tema ou o documento modelo; non infles nin recortes.`
	case cantos == 1:
		return base + `

Crea UN só exercicio sobre o que se pide. O array terá un único elemento.`
	default:
		return base + fmt.Sprintf(`

Crea EXACTAMENTE %d exercicios distintos sobre o tema indicado, variados e,
cando teña sentido, en dificultade progresiva. O array terá %d elementos.`, cantos, cantos)
	}
}

// regrasResolucionTaboa: como ten que quedar escrita a "resolucion" dun
// exercicio de táboa. Compártese entre a 2ª pasada da xeración
// (systemPromptRespostaResolucionTaboa) e o botón de rexenerar a resolución
// dun exercicio solto - así as dúas producen exactamente o mesmo formato.
const regrasResolucionTaboa = `
COMO ESCRIBIR "resolucion" (é o que máis falla - coídao):
- Unha lista <ol> cun <li> por paso, en orde. Se o enunciado ten apartados,
  usa <ol type="a"> para que saian "a)", "b)"...
- Cada <li> ABRE cunha frase curta en <b> que di QUE se fai nese paso e por
  que ("<b>Illamos a aceleración da 2ª lei de Newton:</b>", "<b>Aplicamos a
  lei de Ohm:</b>"). Son os comentarios do que se está a facer.
- A seguir vai a fórmula e o valor NA MESMA LIÑA, con este patrón:
  "nome = <MAT>fórmula</MAT> = <EVAL>mesma fórmula</EVAL> unidade".
  Exemplo: "<p>I = <MAT>V_1/R_1</MAT> = <EVAL>V_1/R_1</EVAL> A</p>".
  * <MAT> leva a FÓRMULA cos NOMES das variables declaradas (V_1/R_1, F/m,
    (1/2)*m*v^2...): Yang amosaa coma fórmula (V₁/R₁), non coma un número.
  * <EVAL> leva a MESMA expresión: Yang substitúe os valores e amosa o
    número. NUNCA escribas ti o número.
  * O "nome =" e o "=" entre as etiquetas van coma texto normal, FÓRA das
    etiquetas. NUNCA metas un "=" dentro de <MAT>/<EVAL>.
- As UNIDADES (m, s, m/s^2, N, J, Ω, V...) van SEMPRE como texto normal XUSTO
  detrás do <EVAL>, NUNCA dentro. Toda magnitude física, nos pasos
  intermedios e no resultado, leva a súa unidade correcta.
- O ÚLTIMO <li> enuncia o resultado final ("<b>Resultado:</b> v =
  <EVAL>...</EVAL> m/s") co mesmo valor e unidade que "resultado".`

// basePromptTaboa é a parte común a TODAS as chamadas da PRIMEIRA pasada
// (exercicio solto, exame dunha soa chamada e cada lote da xeración por
// lotes): aquí a IA só escribe o ENUNCIADO e as VARIABLES. A resposta final
// e a resolución paso a paso pídense despois, nunha segunda chamada por
// exercicio (systemPromptRespostaResolucionTaboa / completarExercicioTaboa),
// porque mesturalo todo nun só JSON fai que a IA descoide os pasos e as
// unidades e ás veces cole o resultado dentro do enunciado.
func basePromptTaboa() string {
	return chuletaMatexe + `

Vas xerar contido para a INTERFACE DE TÁBOA de Yang. Devolve un ARRAY JSON
onde cada elemento é un exercicio con EXACTAMENTE estes DOUS campos:

{
  "enunciado":  "SÓ a pregunta e os datos coñecidos (en <MAT> ou como {nome}). NUNCA o resultado, NUNCA os pasos, NUNCA a palabra Solución/Resolución.",
  "variables":  [{"nome":"a","modo":"aleatoria","xer":"z0"}, {"nome":"k","modo":"auto","expr":"2*a+1"}]
}

A RESPOSTA FINAL e a RESOLUCIÓN paso a paso NON van aquí: pídense despois,
noutra chamada, a partir deste enunciado. Aquí concéntrate en que o
enunciado quede completo, ben formulado e SEN filtrar nada da solución.

Cada elemento de "variables" ten "nome" e "modo":
  - "fixa": un valor numérico en "valor".
  - "aleatoria": un xerador en "xer" - n0/n1/n2 (naturais peq./med./grandes),
    z0/z1/z2 (enteiros), q0/q1/q2 (fraccións), ou "rango" con "min", "max" e
    "paso".
  - "auto": unha expresión Maxima en "expr", calculada a partir doutras
    variables xa declaradas.
Se algún dato do enunciado ten que cambiar entre copias, decláralo como
"aleatoria" e refírete a el con {nome} no enunciado. Os cálculos intermedios
van como "auto". Non declares "variables" que non uses.

O token {nome} vale SÓ na prosa do enunciado. Dentro dun <TIKZ> (ou dun
<TEX>/<PLOT>) as chaves son sintaxe de LaTeX, así que alí o valor dunha
variable escríbese con <EVAL>:
- CORRECTO:   <TIKZ>... \node at (2,3.6) {$R_1 = <EVAL>r1</EVAL>\,\Omega$}; ...</TIKZ>
- INCORRECTO: <TIKZ>... \node at (2,3.6) {$R_1 = {r1}\,\Omega$}; ...</TIKZ>

REGRAS ESTRITAS:
- "enunciado" é HTML simple (<p>, <b>, <ul>...); TODO o código matemático vai
  en <MAT>/<EVAL> (nunca LaTeX cru, nunca "$...$").
- "enunciado" NON pode conter o resultado, nin os pasos, nin a solución.
- Devolve SÓ o array JSON: sen valados markdown, sen texto antes nin despois.
- En galego, salvo que se pida outro idioma.`
}

// systemPromptRespostaResolucionTaboa é a SEGUNDA pasada: o enunciado xa está
// feito e FIXO, e aquí só se pide a RESPOSTA FINAL por apartados e a
// RESOLUCIÓN paso a paso normalizada, nun obxecto JSON de dúas claves. Mesma
// idea ca correccion_ia.go: unha chamada pequena e enfocada sae mellor ca
// unha grande que ten que facelo todo.
const systemPromptRespostaResolucionTaboa = chuletaMatexe + `

TAREFA: dáseche a RESPOSTA E RESOLUCIÓN dun exercicio que hai que completar.
Recibes o ENUNCIADO (NON o cambies, NON o repitas) e a lista de VARIABLES xa
declaradas. Devolve UN obxecto JSON con EXACTAMENTE estas dúas claves:

{
  "resultado":  "A resposta FINAL a CADA apartado do enunciado: curta, directa, SEN desenvolvemento nin pasos. Un <p> (ou un <li>) por apartado, coa súa etiqueta e a súa unidade. Pode levar <EVAL>/<MAT>.",
  "resolucion": "O desenvolvemento PASO A PASO ata ese resultado."
}
` + regrasResolucionTaboa + `

- "resultado" NON leva pasos nin explicacións; "resolucion" NON se limita a
  dar o número, ten que amosar o camiño ata el.
- Se recibes un "INTENTO ANTERIOR" cun erro de compilación, ESE resultado/
  resolución NON compilaron - a túa versión nova ten que evitar
  especificamente o que causou ESE erro (revisa <TIKZ>, apóstrofos, barras
  invertidas e "=" ao final dentro de <MAT>/<EVAL>), non repetir o mesmo
  patrón con outras palabras.
- Usa SÓ os nomes das variables dadas (e, se cómpre, cálculos sobre elas).
- Respecta as regras de <MAT>/<EVAL> de enriba (nada de LaTeX cru, nada de
  "=" ao final dunha etiqueta, nada de apóstrofos, unidades SEMPRE fóra).
- Devolve SÓ ese obxecto JSON: sen valados markdown, sen texto antes nin
  despois. En galego, salvo que o enunciado estea noutro idioma.`

// descricionVariableTaboa: unha liña lexible por variable para a 2ª pasada
// (só ten que saber que nomes hai e que representan, non a súa forma Maxima).
func descricionVariableTaboa(v VariableTaboa) string {
	nome := v.Nome.Trim()
	switch v.Modo.Trim() {
	case "fixa":
		return fmt.Sprintf("%s = %s (valor fixo)", nome, v.Valor.Trim())
	case "auto":
		return fmt.Sprintf("%s = %s (calculada a partir doutras)", nome, v.Expr.Trim())
	default:
		switch xer := v.Xer.Trim(); xer {
		case "rango":
			return fmt.Sprintf("%s (aleatoria de %s a %s, paso %s)", nome, v.Min.Trim(), v.Max.Trim(), v.Paso.Trim())
		case "":
			return fmt.Sprintf("%s (aleatoria)", nome)
		default:
			return fmt.Sprintf("%s (aleatoria, xerador %s)", nome, xer)
		}
	}
}

// completarExercicioTaboa fai a 2ª pasada dun exercicio: enche Resultado e
// Resolucion a partir do enunciado xa xerado. Se a chamada á IA falla, ou a
// resposta non ven no formato agardado, DEVOLVE O EXERCICIO TAL CAL (sen
// tumbar a xeración enteira) - o profesorado sempre pode premer o botón de
// rexenerar a resolución nese exercicio.
//
// contextoErro: "" na xeración normal; en autorreparación (RexenerarExerci
// cioTaboa, autoreparar.go) vén co resultado/resolución que NON compilaron
// a última vez máis o erro literal, para que esta 2ª pasada non regenere ás
// cegas - se o problema estaba no propio texto da resolución (ex. un <TIKZ>
// mal pechado), a IA precisa velo para non repetilo.
func (a *App) completarExercicioTaboa(ex ExercicioTaboa, contextoErro string) ExercicioTaboa {
	enun := ex.Enunciado.Trim()
	if enun == "" {
		return ex
	}

	var b strings.Builder
	b.WriteString("ENUNCIADO (non o cambies):\n")
	b.WriteString(enun)
	b.WriteString("\n\nVARIABLES XA DECLARADAS (usa estes nomes):\n")
	if len(ex.Variables) == 0 {
		b.WriteString("  (ningunha)\n")
	} else {
		for _, v := range ex.Variables {
			if v.Nome.Trim() == "" {
				continue
			}
			b.WriteString("  - " + descricionVariableTaboa(v) + "\n")
		}
	}
	if contextoErro != "" {
		b.WriteString("\n\n")
		b.WriteString(contextoErro)
	}

	raw, err := chamarIA(a.settings, systemPromptRespostaResolucionTaboa, b.String())
	if err != nil {
		return ex
	}
	resultado, resolucion, err := parseRespostaResolucionTaboa(raw)
	if err != nil {
		return ex
	}
	if resultado != "" {
		ex.Resultado = textoIA(resultado)
	}
	if resolucion != "" {
		ex.Resolucion = textoIA(resolucion)
	}
	return ex
}

// completarExerciciosTaboa aplica a 2ª pasada a unha lista, con paralelismo
// acoutado (unha chamada á IA por exercicio). Mantén a orde de `exs`.
func (a *App) completarExerciciosTaboa(exs []ExercicioTaboa) []ExercicioTaboa {
	const traballadores = 3
	sem := make(chan struct{}, traballadores)
	var wg sync.WaitGroup
	for i := range exs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			exs[i] = a.completarExercicioTaboa(exs[i], "")
		}(i)
	}
	wg.Wait()
	return exs
}

// parseRespostaResolucionTaboa le o obxecto {resultado, resolucion} da 2ª
// pasada, coa mesma tolerancia ca parseCorreccionResposta (a IA escribe ás
// veces os campos coma listas, ou envolve o JSON nunha frase).
func parseRespostaResolucionTaboa(raw string) (string, string, error) {
	var resp struct {
		Resultado  textoIA `json:"resultado"`
		Resolucion textoIA `json:"resolucion"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(limparValadosMarkdown(raw))), &resp); err != nil {
		return "", "", formatoInesperadoErr(raw, err)
	}
	return resp.Resultado.Trim(), resp.Resolucion.Trim(), nil
}

// XerarExercicioTaboa: un exercicio para a táboa a partir dunha descrición
// (e/ou PDF modelo).
func (a *App) XerarExercicioTaboa(peticion string, modelos []ModeloPDF) (ExercicioTaboa, error) {
	exs, err := a.xerarTaboa(strings.TrimSpace(peticion), 1, modelos,
		"escribe que exercicio queres ou xunta un PDF modelo")
	if err != nil {
		return ExercicioTaboa{}, err
	}
	return exs[0], nil
}

// XerarExameTaboa: un exame enteiro para a táboa. numExercicios <= 0 =
// indefinido (decide a IA); satúrase a 20 igual ca XerarExameIA.
func (a *App) XerarExameTaboa(tema string, numExercicios int, modelos []ModeloPDF) ([]ExercicioTaboa, error) {
	if numExercicios < 0 {
		numExercicios = 0
	}
	if numExercicios > 20 {
		numExercicios = 20
	}
	return a.xerarTaboa(strings.TrimSpace(tema), numExercicios, modelos,
		"escribe o tema do exame ou xunta un PDF modelo")
}

func (a *App) xerarTaboa(peticion string, cantos int, modelos []ModeloPDF, erroBaleiro string) ([]ExercicioTaboa, error) {
	if peticion == "" && len(modelos) == 0 {
		return nil, fmt.Errorf("%s", erroBaleiro)
	}

	textoModelos, err := textoDeModelosPDF(modelos)
	if err != nil {
		return nil, err
	}
	sistema := systemPromptTaboa(cantos)
	if textoModelos != "" {
		sistema += instrucionModelosPDF
		if peticion == "" {
			peticion = "Crea o contido derivado do(s) documento(s) modelo achegado(s) a continuación."
		}
		peticion += "\n\n--- DOCUMENTO(S) MODELO ---\n\n" + textoModelos
	}

	// A IA falla moito ao escapar o JSON cando un exercicio leva un <TIKZ>
	// ("\draw" sen dobrar a barra); chamarIAJSON repara os escapes e, se non
	// abonda, rechama ata 2 veces máis antes de darse por vencida.
	var exs []ExercicioTaboa
	if err := chamarIAJSON(a.settings, sistema, peticion, extractJSONArray, &exs); err != nil {
		return nil, err
	}
	if len(exs) == 0 {
		return nil, fmt.Errorf("a IA non devolveu ningún exercicio")
	}
	// 2ª pasada: encher resposta final + resolución paso a paso de cada
	// exercicio a partir do seu enunciado (ver completarExercicioTaboa).
	return a.completarExerciciosTaboa(exs), nil
}

// =========================================================================
//  XERACIÓN DO EXAME POR LOTES (varias chamadas curtas, non unha longa)
// =========================================================================
//
// Por que: pedir "un exame de 10 exercicios" nunha soa chamada obriga á IA a
// escribir de vez os 10 enunciados + 10 resultados + 10 resolucións. Iso é
// moito texto de saída, e o texto de saída é O QUE TARDA (os modelos escriben
// palabra a palabra). Ademais é todo ou nada: se a resposta chega cortada ou
// cun campo no formato que non era, pérdense OS DEZ.
//
// Aquí faise coma xa se fai a corrección (correccion_ia.go): moitas chamadas
// pequenas.
//   1. IniciarExameTaboa pide só o GUIÓN: unha liña por exercicio. É unha
//      resposta curtísima (chega en segundos) e xa nos di cantos exercicios
//      hai cando o profesorado escolleu "indefinido".
//   2. XerarLoteExameTaboa desenvolve UN ANACO dese guión (un ou dous
//      exercicios). O frontend lanza varios lotes á vez e vai metendo na
//      táboa os que van chegando, así que:
//        - o tempo total é o do lote máis lento, non a suma de todos;
//        - o que xa chegou queda posto aínda que outro lote falle;
//        - reintentar só volve pedir os que faltan, non o exame enteiro.
//
// O guión gárdase nunha sesión no backend para non ter que reenviar por cada
// lote o tema e, sobre todo, o texto dos PDF modelo (que ademais só se
// extrae unha vez).

// PlanExameTaboa é o que devolve IniciarExameTaboa: o identificador da sesión
// e o guión (unha liña por exercicio, na orde do exame).
type PlanExameTaboa struct {
	Sesion string   `json:"sesion"`
	Guion  []string `json:"guion"`
}

type sesionExameTaboa struct {
	tema         string
	textoModelos string
	guion        []string
	creada       time.Time
}

const (
	vidaSesionExame   = 2 * time.Hour // as sesións vellas bótanse ao crear unha nova
	maxSesionsExame   = 20
	maxExerciciosPlan = 20 // mesmo tope ca XerarExameTaboa
)

var (
	sesionsExameMu      sync.Mutex
	sesionsExame        = map[string]*sesionExameTaboa{}
	contadorSesionExame int
)

func gardarSesionExame(s *sesionExameTaboa) string {
	sesionsExameMu.Lock()
	defer sesionsExameMu.Unlock()

	// Limpeza: caducadas e, se aínda hai demasiadas, as máis vellas.
	agora := time.Now()
	for id, vella := range sesionsExame {
		if agora.Sub(vella.creada) > vidaSesionExame {
			delete(sesionsExame, id)
		}
	}
	for len(sesionsExame) >= maxSesionsExame {
		masVella, cando := "", time.Time{}
		for id, v := range sesionsExame {
			if masVella == "" || v.creada.Before(cando) {
				masVella, cando = id, v.creada
			}
		}
		delete(sesionsExame, masVella)
	}

	contadorSesionExame++
	id := fmt.Sprintf("exame-%d-%d", agora.UnixNano(), contadorSesionExame)
	s.creada = agora
	sesionsExame[id] = s
	return id
}

func lerSesionExame(id string) (*sesionExameTaboa, error) {
	sesionsExameMu.Lock()
	defer sesionsExameMu.Unlock()
	s := sesionsExame[strings.TrimSpace(id)]
	if s == nil {
		return nil, fmt.Errorf("caducou a sesión de xeración do exame; volve premer \"Xerar\"")
	}
	return s, nil
}

// systemPromptGuionExame: prompt CURTO a propósito - aquí non fai falla nada
// de Matexe/Maxima (chuletaMatexe), porque non se escribe aínda ningunha
// fórmula: só a lista de que vai en cada exercicio.
func systemPromptGuionExame(cantos int) string {
	base := `Es profesorado que prepara o GUIÓN dun exame (aínda non os enunciados).

Devolve SÓ un array JSON de cadeas de texto: unha cadea por exercicio, na
orde do exame. Cada cadea di NUNHA LIÑA (máximo 25 palabras) que se pide
nese exercicio e con que datos ou nivel; NON escribas o enunciado completo,
nin fórmulas, nin a solución.

Os exercicios teñen que ser distintos entre si, cubrir o tema e, cando teña
sentido, ir de menos a máis dificultade. Sen valados markdown, sen
numeración dentro das cadeas, sen texto antes nin despois do array. En
galego, salvo que se pida outro idioma.`
	if cantos > 0 {
		return base + fmt.Sprintf("\n\nO array terá EXACTAMENTE %d cadeas.", cantos)
	}
	return base + `

Decide ti cantos exercicios fan falla segundo o tema ou o documento modelo
(entre 3 e 12); non infles nin recortes.`
}

// IniciarExameTaboa é o paso 1: devolve o guión do exame e abre unha sesión
// para que despois XerarLoteExameTaboa desenvolva os exercicios en anacos.
// numExercicios <= 0 = indefinido (decide a IA cantos).
func (a *App) IniciarExameTaboa(tema string, numExercicios int, modelos []ModeloPDF) (PlanExameTaboa, error) {
	tema = strings.TrimSpace(tema)
	if tema == "" && len(modelos) == 0 {
		return PlanExameTaboa{}, fmt.Errorf("escribe o tema do exame ou xunta un PDF modelo")
	}
	if numExercicios < 0 {
		numExercicios = 0
	}
	if numExercicios > maxExerciciosPlan {
		numExercicios = maxExerciciosPlan
	}

	textoModelos, err := textoDeModelosPDF(modelos)
	if err != nil {
		return PlanExameTaboa{}, err
	}

	sistema := systemPromptGuionExame(numExercicios)
	peticion := tema
	if textoModelos != "" {
		sistema += instrucionModelosPDF
		if peticion == "" {
			peticion = "Fai o guión dun exame derivado do(s) documento(s) modelo achegado(s) a continuación."
		}
		peticion += "\n\n--- DOCUMENTO(S) MODELO ---\n\n" + textoModelos
	}

	var guion listaTextosIA
	if err := chamarIAJSON(a.settings, sistema, peticion, extractJSONArray, &guion); err != nil {
		return PlanExameTaboa{}, err
	}
	if len(guion) == 0 {
		return PlanExameTaboa{}, fmt.Errorf("a IA non devolveu ningún exercicio")
	}
	if numExercicios > 0 && len(guion) > numExercicios {
		guion = guion[:numExercicios] // se se pasa, quedamos cos que se pediron
	}
	if len(guion) > maxExerciciosPlan {
		guion = guion[:maxExerciciosPlan]
	}

	id := gardarSesionExame(&sesionExameTaboa{
		tema:         tema,
		textoModelos: textoModelos,
		guion:        guion,
	})
	return PlanExameTaboa{Sesion: id, Guion: guion}, nil
}

// XerarLoteExameTaboa é o paso 2: desenvolve os exercicios [desde,
// desde+cantos) do guión da sesión. Pódense pedir varios lotes á vez (o
// frontend faino) - cada un é unha chamada independente á IA.
func (a *App) XerarLoteExameTaboa(sesion string, desde, cantos int) ([]ExercicioTaboa, error) {
	s, err := lerSesionExame(sesion)
	if err != nil {
		return nil, err
	}
	if desde < 0 {
		desde = 0
	}
	if cantos <= 0 {
		cantos = 1
	}
	if desde >= len(s.guion) {
		return nil, fmt.Errorf("o lote pedido (%d) queda fóra do guión (%d exercicios)", desde+1, len(s.guion))
	}
	if desde+cantos > len(s.guion) {
		cantos = len(s.guion) - desde
	}

	sistema := basePromptTaboa() + fmt.Sprintf(`

Vas desenvolver SÓ UN ANACO dun exame que xa está planificado. Dáseche o
guión completo (para que saibas que hai antes e despois e non repitas nada) e
os números dos exercicios que che tocan: crea EXACTAMENTE %d exercicio(s), os
indicados e na orde do guión. NON desenvolvas os demais nin cambies o que di
o guión de cada un. O array terá %d elementos.`, cantos, cantos)

	var b strings.Builder
	if s.tema != "" {
		b.WriteString("TEMA DO EXAME: " + s.tema + "\n\n")
	}
	b.WriteString("GUIÓN COMPLETO DO EXAME:\n")
	for i, g := range s.guion {
		b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, g))
	}
	if cantos == 1 {
		b.WriteString(fmt.Sprintf("\nDESENVOLVE SÓ o exercicio %d.\n", desde+1))
	} else {
		b.WriteString(fmt.Sprintf("\nDESENVOLVE SÓ os exercicios %d a %d (nesa orde).\n", desde+1, desde+cantos))
	}
	if s.textoModelos != "" {
		sistema += instrucionModelosPDF
		b.WriteString("\n--- DOCUMENTO(S) MODELO ---\n\n" + s.textoModelos)
	}

	var exs []ExercicioTaboa
	if err := chamarIAJSON(a.settings, sistema, b.String(), extractJSONArray, &exs); err != nil {
		return nil, err
	}
	if len(exs) == 0 {
		return nil, fmt.Errorf("a IA non devolveu ningún exercicio")
	}
	if len(exs) > cantos {
		exs = exs[:cantos] // se se pasa de listo, quedamos cos que tocaban
	}
	// 2ª pasada por exercicio deste lote (resposta final + resolución).
	return a.completarExerciciosTaboa(exs), nil
}
