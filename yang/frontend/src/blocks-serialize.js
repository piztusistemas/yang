// Conversión texto .matex <-> lista de exercicios, para o editor de bloques
// (estilo Snap!/Scratch). É lóxica pura (sen DOM) para poder probala illada.
//
// O documento .matex sempre viaxa como unha soa string, tanto no frontend
// coma no backend Go (cas.Maxima.ParseText, cas/tags.go) — non existe
// ningún AST intermedio. Este módulo é o único sitio onde ese texto se
// converte a unha estrutura editable por bloques e viceversa; o resto da
// app (Generate, SaveFile, GeneratePDF...) segue a falar sempre en texto.
//
// Modelo: cada BLOQUE do lenzo é un EXERCICIO enteiro (non un anaco solto
// coma na primeira versión deste editor). Un exercicio contén unha lista
// ordenada de ANACOS (text/formula/image-plot/image-tikz/variables/image-upload).
//
// <EX>...</EX> é unha etiqueta puramente ESTRUTURAL que marca o límite dun
// exercicio: non ten significado ningún para Maxima nin para cas/tags.go
// (que nunca a busca), así que pasa inofensiva tanto en HTML (o navegador
// ignora etiquetas descoñecidas e amosa os fillos) coma en LaTeX
// (latexdoc.go's renderElement ten un caso explícito de paso directo para
// "ex", ver latexdoc.go). É dicir: <EX> existe só para que este ficheiro
// saiba onde recortar ao reabrir un documento — non aparece nunca no HTML
// nin no PDF xerados.
//
// Deliberadamente NON se soportan as etiquetas FILE/COND: existen só como
// resaltado "orfo" en editor.js (atallo Ctrl+F incluído), pero
// cas/tags.go nunca as implementou no parser real de Maxima — replicalas
// aquí crearía anacos que logo non farían nada ao xerar.
export const ELEMENT_TAG_NAMES = {
  formula: 'EVAL',
  'image-plot': 'PLOT',
  'image-tikz': 'TIKZ',
  variables: 'HIDE',
}

// Mesma alternación que en cas/tags.go, pero só para os tipos que o editor
// de bloques ofrece (EVAL/PLOT/TIKZ/HIDE) máis IMG (sen equivalente CAS, é
// HTML puro xa soportado por latexdoc.go's renderImg). MAT/TEX quedan fóra a
// propósito: un documento antigo que as use non se rompe (o texto entre elas
// vira un anaco "text" opaco, coma calquera outro HTML), pero o despregable
// de bloques non as ofrece como opción nova. HIDE si se ofrece (grupo
// "Variables e cálculos" da paleta): cas/tags.go xa a executa en silencio
// (process_HIDE, sen amosar resultado), é exactamente o que fai falla para
// definir/calcular variables sen que apareza nada no exame.
const ELEMENT_RE =
  /<EVAL>([\s\S]*?)<\/EVAL>|<PLOT>([\s\S]*?)<\/PLOT>|<TIKZ>([\s\S]*?)<\/TIKZ>|<HIDE>([\s\S]*?)<\/HIDE>|<IMG\s+src="([^"]*)"\s*\/?>/g

const EX_RE = /<EX>([\s\S]*?)<\/EX>/g

let nextId = 1

export function makeElement(type, content, id) {
  return { id: id ?? nextId++, type, content }
}

export function makeExercise(elements, id) {
  return { id: id ?? nextId++, elements }
}

// parseElements percorre o texto interior dun exercicio de esquerda a
// dereita: cada match dunha etiqueta coñecida vira un anaco tipado; o
// texto entre matches (por complexo que sexa o HTML, incluídas etiquetas
// vellas coma MAT/HIDE/TEX) vira un anaco "text" opaco, sen intentar
// interpretalo. Isto garante que abrir calquera exercicio existente en
// modo bloques non perde contido nunca.
export function parseElements(text) {
  const elements = []
  let lastIndex = 0

  for (const m of text.matchAll(ELEMENT_RE)) {
    if (m.index > lastIndex) {
      elements.push(makeElement('text', text.slice(lastIndex, m.index)))
    }
    // Se m.index === lastIndex, dúas etiquetas van seguidas sen texto
    // entre elas — non crear un anaco "text" baleiro no medio.

    const [, ev, plot, tikz, hide, img] = m
    const type =
      ev !== undefined ? 'formula' :
      plot !== undefined ? 'image-plot' :
      tikz !== undefined ? 'image-tikz' :
      hide !== undefined ? 'variables' : 'image-upload'
    const content = ev ?? plot ?? tikz ?? hide ?? img

    elements.push(makeElement(type, content))
    lastIndex = m.index + m[0].length
  }

  if (lastIndex < text.length) {
    elements.push(makeElement('text', text.slice(lastIndex)))
  }

  return elements
}

// elementsToText é a inversa exacta de parseElements: elementsToText(parseElements(t)) === t
// byte a byte para texto xa producido por este módulo.
export function elementsToText(elements) {
  return elements
    .map((e) => {
      if (e.type === 'text') return e.content
      if (e.type === 'image-upload') return `<IMG src="${e.content}">`
      return `<${ELEMENT_TAG_NAMES[e.type]}>${e.content}</${ELEMENT_TAG_NAMES[e.type]}>`
    })
    .join('')
}

// parseDocument é o parser de nivel superior: texto -> lista de exercicios.
//
// `legacy: true` marca "este documento non foi (aínda) editado co formato
// <EX> deste editor" — cobre tanto ficheiros .matex anteriores a esta
// versión coma calquera documento con contido solto fóra de <EX> (que non
// se pode recortar en exercicios sen adiviñar e arriscar perder algo). En
// calquera dos dous casos o documento enteiro vira un só exercicio cun
// único anaco "text" opaco: nada se perde, e segue sendo editable coma na
// versión anterior deste editor (unha caixa de texto grande).
export function parseDocument(text) {
  if (text.trim() === '') return { exercises: [], legacy: false }

  const matches = [...text.matchAll(EX_RE)]
  if (matches.length > 0 && onlyWhitespaceOutside(text, matches)) {
    return {
      exercises: matches.map((m) => makeExercise(parseElements(m[1]))),
      legacy: false,
    }
  }
  return { exercises: [makeExercise([makeElement('text', text)])], legacy: true }
}

function onlyWhitespaceOutside(text, matches) {
  let last = 0
  for (const m of matches) {
    if (!/^\s*$/.test(text.slice(last, m.index))) return false
    last = m.index + m[0].length
  }
  return /^\s*$/.test(text.slice(last))
}

// documentToText é a inversa de parseDocument, con dúas asimetrías
// deliberadas:
//
// 1. Un documento "legacy" (non tocado aínda por este editor) escríbese
//    exactamente coma entrou — sen envolver en <EX> — para que abrir e
//    pechar un .matex antigo sen editar exercicios (engadir/mover/
//    eliminar/duplicar/arrastrar) non lle cambie nin un byte.
// 2. En canto se toca a lista de exercicios, o chamador (blocks.js) debe
//    marcar `doc.legacy = false` — a partir dese momento o documento
//    "sobe" ao formato <EX> permanentemente, aínda que volva quedar cun
//    só exercicio. Editar texto *dentro* dun anaco non cambia `legacy`.
export function documentToText(doc) {
  if (doc.legacy && doc.exercises.length === 1) {
    return elementsToText(doc.exercises[0].elements)
  }
  if (doc.exercises.length === 0) return ''
  // Sen espazo en branco fixo dentro de <EX>...</EX> (só entre exercicios):
  // se se engadise aquí, un roundtrip parseDocument -> documentToText o
  // capturaría coma parte dun anaco "text" e volvería engadir outra copia
  // no seguinte documentToText, rompendo a idempotencia.
  return doc.exercises
    .map((ex) => `<EX>${elementsToText(ex.elements)}</EX>`)
    .join('\n\n') + '\n'
}
