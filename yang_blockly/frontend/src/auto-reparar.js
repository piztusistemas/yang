// auto-reparar.js: orquestrador da AUTOREPARACIÓN por exercicio dun exame
// da interface de Táboa. Lóxica pura, sen DOM, para poder probala illada -
// mesmo criterio ca reorganizar.js / table-serialize.js.
//
// Que fai: parte dun .matex de táboa xa xerado que NON compila e, sen
// rexenerar o exame enteiro:
//   1. pregúntalle ao backend cales exercicios fallan e con que erro
//      (DiagnosticarCompilacionExercicios);
//   2. por cada un que falla, pídelle á IA que o rexenere pasándolle o erro
//      (RexenerarExercicioTaboa) e substitúeo NO MODELO (non toca os demais);
//   3. reconstrúe o .matex e volve diagnosticar, ata `maxRechamadas` roldas;
//   4. os que sigan sen compilar déixanse tal cal (o chamador pode envolvelos
//      nunha caixa de aviso) e márcanse coma FALLA no informe.
//
// O bucle vive aquí (mesmo patrón ca reorganizarPractica); as tres pezas que
// precisan Maxima/LaTeX/IA veñen inxectadas en `opts` para poder probar.

import { parseDocument, documentToText } from './table-serialize.js'

export const ESTADO_REP = {
  OK: 'ok', // compilou á primeira, non se tocou
  REPARADO: 'reparado', // fallaba e quedou compilando tras rexenerar
  FALLA: 'falla', // esgotáronse as roldas e segue sen compilar
  ERRO: 'erro', // a chamada á IA fallou
}

function emit(cb, inf) {
  if (typeof cb === 'function') {
    try { cb({ ...inf }) } catch { /* o log non debe tumbar a operación */ }
  }
}

function mensaxeErro(err) {
  return err && err.message ? err.message : String(err)
}

// exercicioParaBackend adapta un exercicio do modelo da táboa á forma
// ExercicioTaboa que espera RexenerarExercicioTaboa (taboa_ia.go): enunciado
// + variables + resultado final + resolución.
function exercicioParaBackend(ex) {
  return {
    enunciado: ex.enunciado || '',
    variables: ex.variables || [],
    resultado: ex.resposta || '',
    resolucion: ex.solucion || '',
  }
}

// aplicarExercicio mete a versión arranxada que devolveu o backend
// (ExercicioTaboa) de volta no exercicio do modelo. Baléirase `resultados`
// (a lista estruturada de reorganizar.js): a `resposta`/`solucion` que dá o
// backend xa referencia as variables declaradas directamente, coma na
// xeración normal (ver xerarTaboa / completarExercicioTaboa).
function aplicarExercicio(ex, r) {
  if (!r) return
  if (r.enunciado != null && String(r.enunciado).trim() !== '') ex.enunciado = String(r.enunciado)
  if (Array.isArray(r.variables)) ex.variables = r.variables
  if (r.resultado != null) ex.resposta = String(r.resultado)
  if (r.resolucion != null) ex.solucion = String(r.resolucion)
  ex.resultados = []
}

function escaparHTML(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

// marcarComoNonCompila substitúe un exercicio que non se puido reparar por
// unha caixa de aviso VISIBLE que si compila: así o resto do exame sae. O
// contido orixinal consérvase escapado (as etiquetas <MAT>/<EVAL>... vense
// coma texto, non se procesan), para que o profesorado non perda o traballo
// e poida arranxalo a man ou volver premer «Xerar».
export function marcarComoNonCompila(ex, numero, erro) {
  const orixinal = String(ex.enunciado || '').trim()
  ex.enunciado =
    `<p><b>⚠ Exercicio ${numero}: non se puido xerar unha versión que compile.</b>` +
    (erro ? ` <small>(${escaparHTML(erro)})</small>` : '') +
    ' Edítao ou volve premer «Xerar».</p>' +
    (orixinal
      ? `<p style="white-space:pre-wrap;color:#a00"><small>${escaparHTML(orixinal)}</small></p>`
      : '')
  ex.variables = []
  ex.resposta = ''
  ex.solucion = ''
  ex.resultados = []
}

// autorepararExame: punto de entrada. Devolve
//   { texto, informe: [{ indice, titulo, estado, intentos, erro }], cambiou }
// sen tocar `sourceText` - o chamador decide se aplica `texto` ao editor.
//
// opts:
//   diagnosticar(source) -> Promise<[{indice, ok, erro}]>  (binding DiagnosticarCompilacionExercicios)
//   rexenerar({exercicio, erro, intento}) -> Promise<ExercicioTaboa>  (binding RexenerarExercicioTaboa)
//   maxRechamadas   máximo de roldas de rexeneración (Settings.MaxRechamadasIA), por defecto 2
//   onProgreso(inf) callback por cada exercicio ao cambiar de estado
export async function autorepararExame(sourceText, opts = {}) {
  const {
    diagnosticar,
    rexenerar,
    maxRechamadas = 2,
    onProgreso = null,
  } = opts

  if (typeof diagnosticar !== 'function' || typeof rexenerar !== 'function') {
    throw new Error('autorepararExame: faltan os bindings diagnosticar/rexenerar')
  }

  const { model } = parseDocument(sourceText || '')
  if (!model.exercicios.length) {
    return { texto: sourceText || '', informe: [], cambiou: false, senExercicios: true }
  }
  const roldasMax = Math.max(0, Math.floor(maxRechamadas) || 0)
  // Instantánea do enunciado orixinal de cada exercicio: se ao final non se
  // consegue arranxar, a caixa de aviso amosa o que ESCRIBIU o profesorado /
  // pediu a IA de primeiras, non o último intento fallido.
  const enunciadosOrixinais = model.exercicios.map((ex) => String(ex.enunciado || ''))

  const informe = model.exercicios.map((ex, i) => ({
    indice: i,
    titulo: ex.titulo || '',
    estado: ESTADO_REP.OK, // provisional; baixa a REPARADO/FALLA se algún día falla
    intentos: 0,
    erro: '',
  }))
  let cambiou = false

  // pendentes: índices (no modelo COMPLETO) que aínda hai que comprobar.
  // Empeza con todos, pero a partir da 1ª rolda só leva os que fallaron -
  // un exercicio que xa compilou OK non se volve tocar, así que non fai
  // falla recompilalo (Maxima + LaTeX) en cada rolda seguinte só para
  // confirmar outra vez o que xa se sabía. Antes diagnosticábase SEMPRE o
  // exame enteiro en cada rolda (documentToText(model)) - nun exame de 15
  // exercicios cos que 14 xa ían ben, isto eran 14 compilacións de balde
  // por rolda, e con maxRechamadas=2 son 3 roldas: o groso da lentitude
  // percibida non era o tamaño da petición á IA, era isto.
  let pendentes = model.exercicios.map((_, i) => i)

  for (let ronda = 0; ronda <= roldasMax && pendentes.length; ronda++) {
    // Sub-documento só cos exercicios pendentes, na mesma orde - os índices
    // que devolve `diagnosticar` son relativos A EL (0..pendentes.length-1),
    // así que hai que traducilos de volta ao índice real con pendentes[k].
    const subModel = { v: model.v, exercicios: pendentes.map((i) => model.exercicios[i]) }
    let infosSub
    try {
      infosSub = await diagnosticar(documentToText(subModel))
    } catch (err) {
      // Sen diagnóstico non hai nada que reparar: devólvese o que haxa.
      return { texto: documentToText(model), informe, cambiou, erroDiagnostico: mensaxeErro(err) }
    }
    const fallos = (infosSub || [])
      .filter((x) => x && x.ok === false && pendentes[x.indice] != null)
      .map((x) => ({ indice: pendentes[x.indice], erro: x.erro }))
    if (!fallos.length) break

    if (ronda >= roldasMax) {
      for (const f of fallos) {
        const inf = informe[f.indice]
        const ex = model.exercicios[f.indice]
        if (!inf || !ex) continue
        inf.estado = ESTADO_REP.FALLA
        inf.erro = f.erro || inf.erro
        ex.enunciado = enunciadosOrixinais[f.indice] || ex.enunciado
        marcarComoNonCompila(ex, f.indice + 1, inf.erro)
        cambiou = true
        emit(onProgreso, inf)
      }
      break
    }

    for (const f of fallos) {
      const inf = informe[f.indice]
      const ex = model.exercicios[f.indice]
      if (!inf || !ex) continue
      inf.erro = f.erro || ''
      try {
        const arranxado = await rexenerar({
          exercicio: exercicioParaBackend(ex),
          erro: f.erro || '',
          intento: ronda + 1,
        })
        aplicarExercicio(ex, arranxado)
        inf.estado = ESTADO_REP.REPARADO
        inf.intentos = ronda + 1
        cambiou = true
      } catch (err) {
        inf.estado = ESTADO_REP.ERRO
        inf.erro = mensaxeErro(err)
      }
      emit(onProgreso, inf)
    }
    // Só os que seguían a fallar (agora rexenerados) precisan volver
    // comprobarse a próxima rolda; os "ERRO" (a IA caeu) tamén, por se a
    // próxima rolda vai mellor.
    pendentes = fallos.map((f) => f.indice)
  }

  return { texto: documentToText(model), informe, cambiou }
}

// resumoInforme: reconto por estado, para a liña de estado da modal e os tests.
export function resumoInforme(informe) {
  const r = { ok: 0, reparado: 0, falla: 0, erro: 0, total: (informe || []).length }
  for (const inf of informe || []) if (r[inf.estado] != null) r[inf.estado]++
  return r
}
