// reorganizar.js: orquestrador do botón «⟳ Reorganizar práctica» do
// Asistente (ver assistant.js). Lóxica pura + chamadas a bindings, SEN DOM,
// para poder probala illada - mesmo criterio ca table-serialize.js.
//
// Que fai: parte dunha práctica xa xerada (interface de Táboa OU de Código)
// e reparte a información de forma FIABLE:
//
//   1. os enunciados non se tocan;
//   2. por cada exercicio, pídelle á IA (XerarCorreccionTaboa) o resultado
//      final coma FÓRMULAS Maxima e a resolución paso a paso;
//   3. as fórmulas escríbense no <HIDE> como res_1, res_2... e tanto <RESP>
//      coma <SOL> as referencian -> é Maxima quen calcula, non poden discrepar;
//   4. valídase todo en varias sementes (ProbarExercicios); se algo dá
//      inf / indeterminado / división por cero / erro de Maxima, os
//      diagnósticos vólvense mandar á IA para unha rolda de arranxo, ata
//      `roldas` veces;
//   5. só se aplica cando o profesorado preme «Aplicar» (patrón do asistente).
//
// O nº de sementes é configurable (Opcións) e, se `adaptar`, axústase ás
// características de cada exercicio (raíces, denominadores con variable...).

import {
  parseDocument, documentToText, varToMaxima, tokensToEval,
  respostaDesdeResultados, RES_PREFIX,
} from './table-serialize.js'

// Estado de cada exercicio no informe final.
export const ESTADO = {
  OK: 'ok',                 // xerado e validado sen fallos
  REPARADO: 'reparado',     // validado tras unha ou máis roldas de arranxo
  FALLA: 'falla',           // esgotáronse as roldas e segue dando valores incoherentes
  SEN_FORMULA: 'sen_formula', // a IA di que non ten resultado calculable (demostración...)
  SEN_CAMBIOS: 'sen_cambios', // saltado (sen enunciado, ou fóra do ámbito pedido)
  ERRO: 'erro',             // a chamada á IA fallou
}

// sementesRecomendadas: adapta o nº de sementes de validación ás
// características do exercicio. `base` é ReorganizarSementesResolto (Opcións).
//   - sen variables aleatorias -> 3 (non hai nada que barrer);
//   - división / fracción por algo que non é un número -> x2 (risco de /0);
//   - raíces, logaritmos, arcos -> +15 (risco de dominio);
//   - tanxentes e derivadas -> +15;
//   - solve/roots -> +10;
//   - moitas variables aleatorias (>3) -> +10.
// Tope duro en 200 (o mesmo ca a lapela Test / proba.go).
export function sementesRecomendadas(ex, base) {
  const b = Math.max(1, Math.floor(base) || 25)
  const variables = ex.variables || []
  const aleatorias = variables.filter((v) => v.modo === 'aleatoria')
  if (aleatorias.length === 0) return Math.min(b, 3)

  const texto = [
    ...variables.map((v) => v.expr || ''),
    ...(ex.resultados || []).map((r) => r.formula || ''),
    ex.solucion || '',
  ].join(' ')

  let n = b
  const fraccionaria = aleatorias.some((v) => /^q[012]$/.test(v.xer)) || /\/\s*[a-zA-Z(]/.test(texto)
  if (fraccionaria) n = Math.round(n * 2)
  if (/\b(sqrt|log|ln|asin|acos|acsc|asec)\s*\(/.test(texto)) n += 15
  if (/\b(tan|cot|sec|csc|diff|integrate)\s*\(/.test(texto)) n += 15
  if (/\b(solve|roots|realroots|linsolve|allroots)\s*\(/.test(texto)) n += 10
  if (aleatorias.length > 3) n += 10
  return Math.min(n, 200)
}

// peticionDe monta a CorreccionRequest que espera XerarCorreccionTaboa
// (correccion_ia.go): enunciado en claro (<EVAL>, non {tokens}), as liñas
// Maxima das variables, a resolución previa e os diagnósticos da rolda
// anterior (baleiros na primeira pasada).
function peticionDe(ex, diagnosticos) {
  return {
    enunciado: tokensToEval(ex.enunciado || '', [
      ...(ex.variables || []).map((v) => v && v.nome),
      ...(ex.resultados || []).map((r) => r && r.nome),
    ]).trim(),
    variables: (ex.variables || []).map(varToMaxima).filter(Boolean),
    resolucion: String(ex.solucion || '').trim(),
    diagnosticos: diagnosticos || [],
  }
}

// correccionAModelo escribe ex.resultados / ex.resposta / ex.solucion a
// partir do que devolveu XerarCorreccionTaboa. `resposta` reconstrúese
// SEMPRE desde os resultados (fonte única); `solucion` é a que dá a IA (xa
// referencia res_1...). Muta e devolve o mesmo `ex`.
export function correccionAModelo(ex, correccion) {
  if (!correccion || correccion.senFormula) return ex

  // Nome dos apartados: res_1, res_2... salvo que o docente xa teña unha
  // variable que empece por "res_" seguida de díxitos - nese caso úsase
  // "q_res_" e reescríbense as referencias na resolución que dea a IA.
  let pref = RES_PREFIX
  const colisiona = (ex.variables || []).some((v) => new RegExp('^' + RES_PREFIX + '\\d').test(String(v.nome || '').trim()))
  let sol = String(correccion.resolucion || '').trim()
  if (colisiona) {
    pref = 'q_' + RES_PREFIX
    sol = sol.replace(new RegExp('\\b' + RES_PREFIX + '(\\d+)\\b', 'g'), pref + '$1')
  }

  const resultados = (correccion.resultados || [])
    .map((r, i) => ({
      etiqueta: String((r && r.etiqueta) || '').trim(),
      nome: pref + (i + 1),
      formula: String((r && r.formula) || '').trim(),
    }))
    .filter((r) => r.formula)

  ex.resultados = resultados
  if (resultados.length) ex.resposta = respostaDesdeResultados(resultados)

  if (sol) {
    ex.solucion = sol
  } else if (resultados.length && !String(ex.solucion || '').trim()) {
    // Rede de seguridade: sen resolución da IA, polo menos amosar o valor.
    ex.solucion = resultados
      .map((r) => `<p>${r.etiqueta ? `<b>${r.etiqueta}:</b> ` : ''}<EVAL>${r.nome}</EVAL></p>`)
      .join('\n')
  }
  return ex
}

function emit(cb, inf) {
  if (typeof cb === 'function') { try { cb({ ...inf }) } catch { /* o log non debe tumbar a operación */ } }
}

// reorganizarPractica: o punto de entrada. Devolve
//   { texto, informe: [{ indice, titulo, estado, roldas, sementes, diagnosticos, motivo }] }
// sen tocar `sourceText` - o chamador decide se aplica `texto`.
//
// opts:
//   xerarCorreccion(req)  -> Promise<CorreccionExercicio>   (binding XerarCorreccionTaboa)
//   probar(req)           -> Promise<ProbaResult>           (binding ProbarExercicios); opcional
//   codeIni               cadea de preámbulo Maxima (opcional)
//   sementes              base de sementes (ReorganizarSementesResolto)
//   roldas                máximo de roldas de arranxo (ReorganizarRoldasResolto)
//   adaptar               true -> sementesRecomendadas por exercicio
//   soIndice              nº de exercicio a reorganizar en solitario, ou null (todos)
//   seedBase              primeira semente da validación (por defecto 1)
//   onProgreso(inf)       callback por cada exercicio ao chegar a un estado
export async function reorganizarPractica(sourceText, opts = {}) {
  const {
    xerarCorreccion,
    probar = null,
    codeIni = '',
    sementes = 25,
    roldas = 3,
    adaptar = true,
    soIndice = null,
    seedBase = 1,
    onProgreso = null,
  } = opts

  if (typeof xerarCorreccion !== 'function') {
    throw new Error('reorganizarPractica: falta xerarCorreccion')
  }

  const { model } = parseDocument(sourceText || '')
  if (!model.exercicios.length) {
    return { texto: sourceText || '', informe: [], senExercicios: true }
  }
  const roldasMax = Math.max(1, Math.floor(roldas) || 3)

  const enAmbito = (i) => soIndice == null || i === soIndice
  const informe = model.exercicios.map((ex, i) => ({
    indice: i,
    titulo: ex.titulo || '',
    estado: ESTADO.SEN_CAMBIOS,
    roldas: 0,
    sementes: 0,
    diagnosticos: [],
    motivo: '',
  }))

  // ---- 1) primeira corrección ----
  for (let i = 0; i < model.exercicios.length; i++) {
    if (!enAmbito(i)) continue
    const ex = model.exercicios[i]
    const inf = informe[i]
    if (!String(ex.enunciado || '').trim()) {
      inf.motivo = 'sen enunciado'
      emit(onProgreso, inf)
      continue
    }
    try {
      const c = await xerarCorreccion(peticionDe(ex, []))
      if (c && c.senFormula) {
        inf.estado = ESTADO.SEN_FORMULA
        inf.motivo = (c.motivo || '').trim()
        emit(onProgreso, inf)
        continue
      }
      correccionAModelo(ex, c)
      inf.estado = ESTADO.OK // provisional ata validar
      inf.roldas = 1
    } catch (err) {
      inf.estado = ESTADO.ERRO
      inf.motivo = String(err && err.message ? err.message : err)
      emit(onProgreso, inf)
    }
  }

  // ---- 2) validar + arranxar ----
  // `intento` 0 = validación inicial (aínda pode arranxar); 1..roldasMax-1 =
  // valida e arranxa; roldasMax = última validación, xa sen máis arranxos.
  // => ata `roldasMax` roldas de arranxo reais.
  const validado = new Set()
  for (let intento = 0; probar && intento <= roldasMax; intento++) {
    let quedanFallos = false
    for (let i = 0; i < model.exercicios.length; i++) {
      if (!enAmbito(i) || validado.has(i)) continue
      const inf = informe[i]
      if (inf.estado !== ESTADO.OK && inf.estado !== ESTADO.REPARADO) continue
      const ex = model.exercicios[i]
      if (!(ex.resultados || []).length) { validado.add(i); continue }

      const nSeeds = adaptar ? sementesRecomendadas(ex, sementes) : Math.max(1, Math.floor(sementes) || 25)
      let e
      try {
        const sub = documentToText({ v: 2, exercicios: [ex] })
        const r = await probar({ source: sub, seedBase, nSeeds, codeIni })
        e = (r && r.exercicios ? r.exercicios : [])[0]
      } catch (err) {
        inf.motivo = inf.motivo || ('non se puido validar: ' + String(err && err.message ? err.message : err))
        emit(onProgreso, inf)
        validado.add(i) // sen validación non ten sentido reintentar
        continue
      }

      const diags = (e && e.diagnosticos) || []
      inf.sementes = (e && e.nProbas) || nSeeds
      if (!diags.length) {
        validado.add(i)
        emit(onProgreso, inf)
        continue
      }

      quedanFallos = true
      inf.diagnosticos = diags
      if (intento >= roldasMax) {
        inf.estado = ESTADO.FALLA
        emit(onProgreso, inf)
        continue
      }
      try {
        const c = await xerarCorreccion(peticionDe(ex, diags))
        if (c && c.senFormula) {
          inf.estado = ESTADO.SEN_FORMULA
          inf.motivo = (c.motivo || '').trim()
          validado.add(i)
          emit(onProgreso, inf)
          continue
        }
        correccionAModelo(ex, c)
        inf.estado = ESTADO.REPARADO
        inf.roldas = intento + 1
      } catch (err) {
        inf.estado = ESTADO.FALLA
        inf.motivo = String(err && err.message ? err.message : err)
        emit(onProgreso, inf)
      }
    }
    if (!quedanFallos) break
  }

  return { texto: documentToText(model), informe }
}

// resumoInforme: reconto por estado, para a cabeceira do log e os tests.
export function resumoInforme(informe) {
  const r = { ok: 0, reparado: 0, falla: 0, sen_formula: 0, sen_cambios: 0, erro: 0, total: (informe || []).length }
  for (const inf of informe || []) if (r[inf.estado] != null) r[inf.estado]++
  return r
}
