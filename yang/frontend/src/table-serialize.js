// Conversión texto .matex <-> modelo de TÁBOA (interface de folla de cálculo,
// alternativa a Blockly). Lóxica pura, sen DOM, para poder probala illada -
// mesmo criterio ca blocks-serialize.js.
//
// A diferenza do editor de bloques, a táboa precisa ESTRUTURA que o .matex
// non ten: unha variable ten nome + modo (fixa / aleatoria / automática), e
// cada exercicio pode ter unha "resposta final" e unha "solución" separadas
// do enunciado. Esa estrutura gárdase como un MODELO JSON incrustado nun
// comentario HTML na primeira liña do documento:
//
//   <!--yang:tabla:1 <base64(JSON)> -->
//   <EX>… corpo real expandido …</EX>
//
// O comentario é invisible en todas as saídas (preview HTML, PDF via
// latexdoc.go -> CommentNode -> "", Pandoc, API). O corpo <EX>…</EX> que vén
// despois é .matex normal e corrente: HIDE coas variables, o enunciado con
// <EVAL> onde había {tokens}, e <RESP>/<SOL> coa corrección. É dicir: o
// documento SEMPRE compila aínda que se abra noutra vista; o comentario só
// serve para reconstruír a táboa exacta ao reabrir.
//
// Se un .matex NON leva o comentario (ficheiro antigo, ou editado en Blockly
// / texto), parseDocument devolve legacy:true e fai unha importación "o
// mellor posible": parte por <EX>, colle o <HIDE> inicial como variables
// (inferindo o modo da sintaxe) e o resto como enunciado. En canto a táboa
// se edita, o chamador marca dirty e a partir de aí getValue() xa escribe o
// comentario - o documento "sobe" ao formato táboa, coma o <EX> de
// blocks-serialize.js.

// XERADORES: nomes de función Maxima que codeini_default.ini define para
// valores aleatorios (naturais / enteiros / racionais, pequenos/medianos/
// grandes). "rango" non é un deles - é a opción "de X a Y" que a táboa
// compila a random(...).
export const XERADORES = ['n0', 'n1', 'n2', 'z0', 'z1', 'z2', 'q0', 'q1', 'q2']
export const MODOS_VARIABLE = ['fixa', 'aleatoria', 'auto']

// MODELO_VERSION: o número que vai no comentario incrustado (yang:tabla:N) e
// no campo `v` do modelo. A v2 engade `resultados` a cada exercicio (ver
// reorganizar.js / docs/tabla.md): a lista estruturada {etiqueta, nome,
// formula} que é a ÚNICA fonte de verdade do resultado final. `resposta` e
// `solucion` pasan a derivarse dela. Un documento v1 (sen `resultados`)
// lese igual - normalizeModel só lle engade unha lista baleira.
export const MODELO_VERSION = 2
// RES_PREFIX: prefixo dos nomes de variable auto que reorganizar.js xera
// para cada apartado do resultado (res_1, res_2...). Exportado para que o
// orquestrador poida evitar colisións cos nomes que xa usa o docente.
export const RES_PREFIX = 'res_'

const EMBED_RE = /^﻿?\s*<!--\s*yang:tabla:(\d+)\s+([A-Za-z0-9+/=]+)\s*-->\r?\n?/
const EX_RE = /<EX>([\s\S]*?)<\/EX>/g
const HIDE_RE = /<HIDE>([\s\S]*?)<\/HIDE>/
const RESP_RE = /<RESP>([\s\S]*?)<\/RESP>/
const SOL_RE = /<SOL>([\s\S]*?)<\/SOL>/
const IDENT_RE = /^[A-Za-z_]\w*$/

let nextId = 1
function freshId() {
  return 'e' + nextId++
}

// ---------- base64 <-> texto UTF-8 (browser + Node/vitest) ----------
// TextEncoder/TextDecoder + btoa/atob están dispoñibles nos dous entornos;
// o paso por String.fromCharCode/charCodeAt byte a byte é o que fai que
// btoa/atob (que só falan Latin-1) non estraguen os acentos do JSON.
function b64Encode(str) {
  const bytes = new TextEncoder().encode(str)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin)
}
function b64Decode(b64) {
  const bin = atob(b64)
  const bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0))
  return new TextDecoder().decode(bytes)
}

// ---------- modelo: creación / normalización ----------
export function makeVariable(nome = '', modo = 'aleatoria') {
  const v = { nome, modo }
  if (modo === 'fixa') v.valor = ''
  else if (modo === 'auto') v.expr = ''
  else { v.xer = 'n1' } // aleatoria por defecto
  return v
}

export function makeExercise() {
  return {
    id: freshId(),
    titulo: '',
    enunciado: '',
    variables: [],
    // resultados: [{etiqueta, nome, formula}] - a corrección estruturada
    // (ver reorganizar.js). Baleira ata que se reorganiza a práctica.
    resultados: [],
    resposta: '',
    solucion: '',
  }
}

export function makeResultado(etiqueta = '', nome = '', formula = '') {
  return { etiqueta, nome, formula }
}

export function emptyModel() {
  return { v: MODELO_VERSION, exercicios: [] }
}

// normalizeModel garante a forma esperada aínda que o JSON incrustado veña
// dunha versión futura, editado a man, ou incompleto - nunca lanza.
export function normalizeModel(raw) {
  const model = { v: MODELO_VERSION, exercicios: [] }
  const lista = raw && Array.isArray(raw.exercicios) ? raw.exercicios : []
  for (const ex of lista) {
    model.exercicios.push({
      id: typeof ex.id === 'string' && ex.id ? ex.id : freshId(),
      titulo: str(ex.titulo),
      enunciado: str(ex.enunciado),
      resultados: (Array.isArray(ex.resultados) ? ex.resultados : []).map(normalizeResultado).filter(Boolean),
      // (map pásalle (r, i) a normalizeResultado; o índice só se usa se falta o nome)
      resposta: str(ex.resposta),
      solucion: str(ex.solucion),
      variables: (Array.isArray(ex.variables) ? ex.variables : []).map(normalizeVar).filter(Boolean),
    })
  }
  return model
}

// normalizeResultado sanea unha entrada de `resultados` (pode vir dun
// comentario editado a man ou dunha versión futura): precisa formula; se non
// trae un `nome` identificador válido, asígnaselle un (res_1, res_2...)
// segundo a posición. Devolve null se non hai formula - unha entrada baleira
// non merece unha liña `res_n: $` no <HIDE>.
function normalizeResultado(r, i) {
  if (!r || typeof r !== 'object') return null
  const formula = str(r.formula).trim()
  if (!formula) return null
  const nomeRaw = str(r.nome).trim()
  const nome = IDENT_RE.test(nomeRaw) ? nomeRaw : RES_PREFIX + (i + 1)
  return { etiqueta: str(r.etiqueta).trim(), nome, formula }
}

function normalizeVar(v) {
  if (!v || typeof v !== 'object') return null
  const modo = MODOS_VARIABLE.includes(v.modo) ? v.modo : 'aleatoria'
  const out = { nome: str(v.nome), modo }
  if (modo === 'fixa') {
    out.valor = str(v.valor)
  } else if (modo === 'auto') {
    out.expr = str(v.expr)
    if (v.defOp === ':=') out.defOp = ':='
  } else {
    // aleatoria: ou un xerador con nome, ou "rango" (de min a max, paso).
    if (v.xer === 'rango') {
      out.xer = 'rango'
      out.min = str(v.min) || '1'
      out.max = str(v.max) || '10'
      out.paso = str(v.paso) || '1'
    } else {
      out.xer = XERADORES.includes(v.xer) ? v.xer : 'n1'
    }
  }
  return out
}

function str(x) {
  return x == null ? '' : String(x)
}

// ---------- {tokens} <-> <EVAL> ----------
// BLOQUES_CODIGO son as etiquetas cuxo INTERIOR non é prosa senón código
// (TikZ/LaTeX/Maxima), onde "{...}" é sintaxe da linguaxe e NUNCA un token de
// variable. Tratalo coma prosa era un destrozo silencioso: nun debuxo
// "\node at (0,1) {$R_1$};" o "{$R_1$}" convertíase en "<EVAL>$R_1$</EVAL>"
// e "\mathrm{W}" en "\mathrm<EVAL>W</EVAL>", así que o esquema deixaba de
// compilar (report real: dous boletíns de electricidade seguidos, o que
// dobrara as chaves saía ben e o que non, non). Para meter o valor dunha
// variable nun debuxo escríbese o <EVAL> a man - cas resólveo dentro do
// <TIKZ> antes de compilar (ver expandirTagsTikz en cas/tags.go).
const BLOQUES_CODIGO = '<(TIKZ|TEX|PLOT|MAT|EVAL|HIDE|SISTEMA)>[\\s\\S]*?<\\/\\1>'

// Na volta (evalToTokens) protéxense SÓ os bloques CONTEDORES: dentro deles
// un <EVAL> forma parte do código e ten que quedar como <EVAL>. Un <MAT> ou
// un <EVAL> soltos na prosa, en cambio, son precisamente o que hai que
// tokenizar, así que non entran nesta lista.
const BLOQUES_CONTEDORES = '<(TIKZ|TEX|PLOT|SISTEMA)>[\\s\\S]*?<\\/\\1>'

// MAXIMA_BLOQUES: das etiquetas de BLOQUES_CODIGO, as que levan dentro
// código de MAXIMA (non LaTeX/TikZ). Aquí un token "{a}" dunha variable
// declarada NON se pode volver "<EVAL>a</EVAL>": cas non expande etiquetas
// aniñadas dentro dun <MAT>/<EVAL>, así que ese "<EVAL>" iría LITERAL a
// Maxima e rebentaría a compilación (report real: "<MAT>A = matrix([{a},
// {b}], [1, 2])</MAT>" -> "incorrect syntax: < is not a prefix operator").
// Como o <HIDE> do exercicio xa liga esas variables na mesma sesión de
// Maxima, abonda con deixar o NOME ESPIDO ("{a}" -> "a") e Maxima
// substitúeo polo seu valor ao facer tex(). As de LaTeX/TikZ (TEX/PLOT/
// TIKZ) seguen co <EVAL>, que cas si resolve antes de compilar.
const MAXIMA_BLOQUES = new Set(['MAT', 'EVAL', 'HIDE', 'SISTEMA'])

// porSegmentos parte o texto en anacos de prosa e anacos de código (as
// etiquetas de BLOQUES_CODIGO, coa súa etiqueta incluída) e aplica a cada
// clase a súa función.
function porSegmentos(text, patron, enProsa, enCodigo) {
  const s = str(text)
  const re = new RegExp(patron, 'g')
  let out = ''
  let last = 0
  let m
  while ((m = re.exec(s)) !== null) {
    out += enProsa(s.slice(last, m.index)) + enCodigo(m[0])
    last = m.index + m[0].length
  }
  return out + enProsa(s.slice(last))
}

// tokensToEval: "área de {b}" -> "área de <EVAL>b</EVAL>". {{ e }} son
// chaves literais. Un token pode ser calquera cousa non baleira sen chaves
// nin saltos de liña dentro (normalmente un nome de variable, pero
// admítese unha expresión curta tipo {a+b} por comodidade).
//
// `nomes` é a lista de variables/resultados declarados no exercicio, e SÓ se
// usa dentro dos bloques de código, onde a regra é moito máis restritiva (ver
// tokensEnCodigo). Na prosa non cambia nada: alí toda chave é un token.
export function tokensToEval(text, nomes) {
  const declarados = new Set((nomes || []).map((n) => str(n).trim()).filter(Boolean))
  return porSegmentos(text, BLOQUES_CODIGO, tokensToEvalProsa, (bloque) => {
    const tag = (/^<([A-Za-z]+)>/.exec(bloque) || [])[1]
    return MAXIMA_BLOQUES.has(tag)
      ? tokensEnMaxima(bloque, declarados)
      : tokensEnCodigo(bloque, declarados)
  })
}

function tokensToEvalProsa(text) {
  return str(text).replace(/\{\{|\}\}|\{([^{}\n]+)\}/g, (m, expr) => {
    if (m === '{{') return '{'
    if (m === '}}') return '}'
    return `<EVAL>${expr.trim()}</EVAL>`
  })
}

// tokensEnCodigo: dentro dun <TIKZ>/<TEX>/... unha chave é, case sempre,
// sintaxe da linguaxe ("\node ... {$R_1$};", "\mathrm{W}"), non un token. Pero
// non SEMPRE: un circuíto que ten que amosar a resistencia da copia concreta
// escríbese "{$R_1={r1}\,\Omega$}", e ese {r1} si hai que resolvelo (se non,
// no debuxo aparece a letra "r1" onde debía ir o número).
//
// A regra que distingue os dous casos é a única que non adiviña: convértese a
// <EVAL> só o que sexa un identificador simple E estea DECLARADO como variable
// ou resultado dese exercicio. "{W}" de "\mathrm{W}", "{$+$}" ou
// "{Conexión en serie}" non o están, así que quedan como chaves de LaTeX.
//
// O desescapado "{{" -> "{" faise igual ca na prosa: os documentos xa
// gardados (e a IA, á que se lle pediu así durante un tempo) traen o TikZ coas
// chaves dobradas e teñen que seguir saíndo igual. Así valen os dous patróns.
function tokensEnCodigo(text, declarados) {
  return str(text).replace(/\{\{|\}\}|\{([^{}\n]+)\}/g, (m, expr) => {
    if (m === '{{') return '{'
    if (m === '}}') return '}'
    const t = expr.trim()
    return IDENT_RE.test(t) && declarados.has(t) ? `<EVAL>${t}</EVAL>` : m
  })
}

// tokensEnMaxima: coma tokensEnCodigo pero para os bloques de código MAXIMA
// (ver MAXIMA_BLOQUES). Unha chave que sexa un identificador simple E estea
// DECLARADA substitúese polo NOME ESPIDO (o <HIDE> do exercicio xa o liga na
// mesma sesión, así que Maxima o resolve ao facer tex()); calquera outra
// cousa - un "{1, 2, 3}" que é un conxunto de Maxima, unha expresión, unha
// variable non declarada - queda igual. "{{"/"}}" desescápanse coma na prosa.
// Aquí NUNCA se emite "<EVAL>": iría literal a Maxima dentro do <MAT>/<EVAL>
// e rompería a compilación (ver MAXIMA_BLOQUES).
function tokensEnMaxima(text, declarados) {
  return str(text).replace(/\{\{|\}\}|\{([^{}\n]+)\}/g, (m, expr) => {
    if (m === '{{') return '{'
    if (m === '}}') return '}'
    const t = expr.trim()
    return IDENT_RE.test(t) && declarados.has(t) ? t : m
  })
}

// evalToTokens: a inversa, para a importación legacy. Só converte a {nome}
// os <EVAL> cun identificador simple dentro (o caso normal dun enunciado
// feito coa táboa); un <EVAL> cunha expresión complexa déixase tal cal
// (segue funcionando, só que non se pode "tokenizar" para editar como
// texto). As chaves que xa houbese no texto escúdanse a {{ }}.
//
// Os bloques de código pasan intactos, pola mesma razón ca en tokensToEval:
// dobrar as chaves dun \node de TikZ non aporta nada (tokensToEval xa non
// as toca) e o <EVAL> que leve dentro un debuxo ten que seguir sendo un
// <EVAL>, non converterse nun token que logo se perdería.
export function evalToTokens(text) {
  return porSegmentos(
    text,
    BLOQUES_CONTEDORES,
    (t) =>
      t
        .replace(/([{}])/g, '$1$1')
        .replace(/<EVAL>([\s\S]*?)<\/EVAL>/g, (m, inner) => {
          const t2 = inner.trim()
          return IDENT_RE.test(t2) ? `{${t2}}` : m
        }),
    (t) => t,
  )
}

// ---------- variable -> liña Maxima (dentro do <HIDE>) ----------
export function varToMaxima(v) {
  const nome = str(v.nome).trim()
  if (!nome) return ''
  if (v.modo === 'fixa') {
    const val = str(v.valor).trim()
    return val ? `${nome}: ${val}$` : ''
  }
  if (v.modo === 'auto') {
    const ex = str(v.expr).trim()
    if (!ex) return ''
    // defOp ":=" consérvase para as definicións de función importadas dun
    // .matex legacy (ex. f(x):=x^2) - o resto das "auto" son asignacións
    // normais cun ":".
    return v.defOp === ':=' || nome.includes('(') ? `${nome} := ${ex}$` : `${nome}: ${ex}$`
  }
  // aleatoria
  if (v.xer === 'rango') {
    const min = str(v.min).trim() || '1'
    const max = str(v.max).trim() || '10'
    const paso = str(v.paso).trim() || '1'
    if (paso === '1') {
      const n = numOrNull(max) != null && numOrNull(min) != null
        ? String(numOrNull(max) - numOrNull(min) + 1)
        : `((${max})-(${min})+1)`
      return `${nome}: (${min}) + random(${n})$`
    }
    return `${nome}: (${min}) + (${paso})*random(floor(((${max})-(${min}))/(${paso}))+1)$`
  }
  const xer = XERADORES.includes(v.xer) ? v.xer : 'n1'
  return `${nome}: ${xer}()$`
}

function numOrNull(s) {
  const n = Number(s)
  return Number.isFinite(n) && String(s).trim() !== '' ? n : null
}

// ---------- resultado estruturado -> liña Maxima / HTML ----------
// resultadoToMaxima: unha entrada de `resultados` compílase a unha liña auto
// máis do <HIDE> (res_1: base*altura/2$), colocada DESPOIS das variables do
// docente para que poida usalas. É o mecanismo que fai que <RESP> e <SOL>
// nunca discrepen: os dous avalían este mesmo binding.
export function resultadoToMaxima(r) {
  const nome = str(r && r.nome).trim()
  const formula = str(r && r.formula).trim()
  return nome && formula ? `${nome}: ${formula}$` : ''
}

// respostaDesdeResultados: `resposta` construída a partir da lista
// estruturada. Cada apartado: «etiqueta: <MAT>fórmula</MAT> = {res_n}». Usa
// a MESMA convención de tokens {nome} ca o resto da táboa (así a cela
// "Resultado final" amosa e edita coma calquera outra, e modelToBody
// convérteos a <EVAL> co seu tokensToEval de sempre). As chaves que houbese
// na etiqueta escúdanse a {{ }} para que tokensToEval as respecte.
export function respostaDesdeResultados(resultados) {
  return (resultados || [])
    .map((r) => {
      const nome = str(r && r.nome).trim()
      const formula = str(r && r.formula).trim()
      if (!nome || !formula) return ''
      const et = str(r && r.etiqueta).trim().replace(/{/g, '{{').replace(/}/g, '}}')
      const pref = et ? `<b>${et}:</b> ` : ''
      return `<p>${pref}<MAT>${formula}</MAT> = {${nome}}</p>`
    })
    .filter(Boolean)
    .join('\n')
}

// ---------- modelo -> corpo .matex (sen o comentario) ----------
export function modelToBody(model) {
  const exs = (model.exercicios || []).map((ex) => {
    const partes = []
    const hideVars = (ex.variables || []).map(varToMaxima).filter(Boolean)
    const hideRes = (ex.resultados || []).map(resultadoToMaxima).filter(Boolean)
    const hide = [...hideVars, ...hideRes].join(' ')
    if (hide) partes.push(`<HIDE>${hide}</HIDE>`)
    // Nomes declarados no exercicio: o que decide, dentro dun <TIKZ>, se
    // "{r1}" é a variable r1 ou unhas chaves de LaTeX (ver tokensEnCodigo).
    const nomes = [
      ...(ex.variables || []).map((v) => str(v.nome)),
      ...(ex.resultados || []).map((r) => str(r.nome)),
    ]
    const enun = tokensToEval(ex.enunciado || '', nomes).trim()
    if (enun) partes.push(enun)

    // <RESP>: a `resposta` escrita (a man ou reconstruída) manda; se está
    // baleira pero hai resultados estruturados, constrúese un por defecto.
    // Nos dous casos pasa por tokensToEval ({res_n} -> <EVAL>res_n</EVAL>).
    let resp = str(ex.resposta).trim()
    if (!resp && (ex.resultados || []).length) resp = respostaDesdeResultados(ex.resultados)
    if (resp) partes.push(`<RESP>${tokensToEval(resp, nomes).trim()}</RESP>`)

    if (str(ex.solucion).trim()) partes.push(`<SOL>${tokensToEval(ex.solucion, nomes).trim()}</SOL>`)
    return `<EX>\n${partes.join('\n')}\n</EX>`
  })
  return exs.join('\n\n') + (exs.length ? '\n' : '')
}

export function buildEmbed(model) {
  const limpo = {
    v: MODELO_VERSION,
    exercicios: (model.exercicios || []).map((ex) => ({
      id: ex.id,
      titulo: ex.titulo || '',
      enunciado: ex.enunciado || '',
      variables: ex.variables || [],
      resultados: (ex.resultados || []).map((r) => ({
        etiqueta: str(r.etiqueta), nome: str(r.nome), formula: str(r.formula),
      })),
      resposta: ex.resposta || '',
      solucion: ex.solucion || '',
    })),
  }
  return `<!--yang:tabla:${MODELO_VERSION} ${b64Encode(JSON.stringify(limpo))} -->`
}

// documentToText: modelo -> .matex canónico (comentario + corpo). Un modelo
// sen exercicios produce "" (ficheiro en branco, coma "Novo").
export function documentToText(model) {
  if (!model || !(model.exercicios || []).length) return ''
  return buildEmbed(model) + '\n' + modelToBody(model)
}

// ---------- .matex -> modelo ----------
export function parseEmbed(text) {
  const m = EMBED_RE.exec(text || '')
  if (!m) return null
  try {
    return normalizeModel(JSON.parse(b64Decode(m[2])))
  } catch {
    return null // comentario corrupto: cae na importación legacy
  }
}

// parseDocument: { model, legacy }. legacy:true significa "este .matex non
// foi feito coa táboa" - o chamador debe conservar o texto orixinal
// mentres non se edite (mesmo contrato ca blocks-serialize.js).
export function parseDocument(text) {
  const incrustado = parseEmbed(text)
  if (incrustado) return { model: incrustado, legacy: false }
  if (str(text).trim() === '') return { model: emptyModel(), legacy: false }
  return { model: legacyImport(text), legacy: true }
}

function legacyImport(text) {
  const bloques = [...str(text).matchAll(EX_RE)]
  if (bloques.length > 0 && soBrancoFora(text, bloques)) {
    return {
      v: 1,
      exercicios: bloques.map((b) => exercicioDesdeLegacy(b[1])),
    }
  }
  // Documento solto sen <EX>: un só exercicio cun enunciado opaco (o texto
  // enteiro, sen tokenizar - máis seguro). Editable como unha caixa grande.
  const ex = makeExercise()
  ex.enunciado = str(text)
  return { v: 1, exercicios: [ex] }
}

function soBrancoFora(text, matches) {
  let last = 0
  for (const m of matches) {
    if (!/^\s*$/.test(text.slice(last, m.index))) return false
    last = m.index + m[0].length
  }
  return /^\s*$/.test(text.slice(last))
}

function exercicioDesdeLegacy(inner) {
  const ex = makeExercise()
  let resto = inner

  const hide = HIDE_RE.exec(resto)
  if (hide) {
    ex.variables = hideParaVariables(hide[1])
    resto = resto.slice(0, hide.index) + resto.slice(hide.index + hide[0].length)
  }
  const resp = RESP_RE.exec(resto)
  if (resp) {
    ex.resposta = evalToTokens(resp[1]).trim()
    resto = resto.slice(0, resp.index) + resto.slice(resp.index + resp[0].length)
  }
  const sol = SOL_RE.exec(resto)
  if (sol) {
    ex.solucion = evalToTokens(sol[1]).trim()
    resto = resto.slice(0, sol.index) + resto.slice(sol.index + sol[0].length)
  }
  ex.enunciado = evalToTokens(resto).trim()
  return ex
}

// hideParaVariables: "a: 3$ b: n1()$ area: a*b/2" -> [{nome,modo,...}]. As
// pezas sepáranse por ; ou $ (coma en Maxima). Clasifícase o lado dereito:
//   número literal          -> fixa
//   xer con nome, ex. n1()  -> aleatoria (xerador)
//   min + random(k)         -> aleatoria (rango)
//   calquera outra cousa    -> auto (expresión)
// Unha peza sen "nome:" recoñecible cóntase como auto anónima (consérvase o
// texto para non perder nada ao reabrir).
export function hideParaVariables(hide) {
  const vars = []
  for (const bruto of str(hide).split(/[;$]/)) {
    const peza = bruto.trim()
    if (!peza) continue
    const asg = /^([A-Za-z_]\w*(?:\([^)]*\))?)\s*(:=|:)\s*([\s\S]+)$/.exec(peza)
    if (!asg) {
      vars.push({ nome: '', modo: 'auto', expr: peza })
      continue
    }
    const nome = asg[1]
    const funcdef = asg[2] === ':=' || nome.includes('(')
    const rhs = asg[3].trim()
    if (funcdef) {
      vars.push({ nome, modo: 'auto', expr: rhs, defOp: ':=' })
      continue
    }
    if (/^-?\d+(?:\.\d+)?$/.test(rhs)) {
      vars.push({ nome, modo: 'fixa', valor: rhs })
      continue
    }
    const xer = /^([nzq][012])\s*\(\s*\)$/.exec(rhs)
    if (xer) {
      vars.push({ nome, modo: 'aleatoria', xer: xer[1] })
      continue
    }
    const rango = /^\(?\s*(-?\d+)\s*\)?\s*\+\s*random\(\s*(\d+)\s*\)$/.exec(rhs)
    if (rango) {
      const min = parseInt(rango[1], 10)
      const conta = parseInt(rango[2], 10)
      vars.push({ nome, modo: 'aleatoria', xer: 'rango', min: String(min), max: String(min + conta - 1), paso: '1' })
      continue
    }
    vars.push({ nome, modo: 'auto', expr: rhs })
  }
  return vars
}

