import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import {
  parseDocument,
  documentToText,
  parseEmbed,
  buildEmbed,
  modelToBody,
  normalizeModel,
  makeExercise,
  makeVariable,
  makeResultado,
  varToMaxima,
  resultadoToMaxima,
  respostaDesdeResultados,
  tokensToEval,
  evalToTokens,
  hideParaVariables,
  MODELO_VERSION,
} from './table-serialize.js'

const testdataDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'testdata')
const readTestdata = (name) => readFileSync(join(testdataDir, name), 'utf8')

function modeloExemplo() {
  const ex = makeExercise()
  ex.enunciado = 'Área dun triángulo de base {base} e altura {altura}.'
  ex.variables = [
    { nome: 'base', modo: 'aleatoria', xer: 'rango', min: '1', max: '10', paso: '1' },
    { nome: 'altura', modo: 'aleatoria', xer: 'n1' },
    { nome: 'area', modo: 'auto', expr: 'base*altura/2' },
    { nome: 'g', modo: 'fixa', valor: '9.8' },
  ]
  ex.resposta = 'A área é {area} u².'
  ex.solucion = 'Área = base·altura/2 = {base}·{altura}/2 = {area}.'
  return { v: 1, exercicios: [ex] }
}

describe('documentToText / parseDocument — roundtrip co comentario incrustado', () => {
  it('modelo baleiro produce cadea baleira', () => {
    expect(documentToText({ v: 1, exercicios: [] })).toBe('')
    expect(parseDocument('')).toEqual({ model: { v: MODELO_VERSION, exercicios: [] }, legacy: false })
  })

  it('un documento xerado pola táboa reábrese como o mesmo modelo (legacy:false)', () => {
    const model = modeloExemplo()
    const texto = documentToText(model)
    const { model: reaberto, legacy } = parseDocument(texto)
    expect(legacy).toBe(false)
    expect(reaberto.exercicios).toHaveLength(1)
    const ex = reaberto.exercicios[0]
    expect(ex.enunciado).toBe('Área dun triángulo de base {base} e altura {altura}.')
    expect(ex.resposta).toBe('A área é {area} u².')
    expect(ex.variables.map((v) => v.nome)).toEqual(['base', 'altura', 'area', 'g'])
    expect(ex.variables[0]).toMatchObject({ modo: 'aleatoria', xer: 'rango', min: '1', max: '10' })
    expect(ex.variables[3]).toMatchObject({ modo: 'fixa', valor: '9.8' })
  })

  it('documentToText é idempotente', () => {
    const t1 = documentToText(modeloExemplo())
    const t2 = documentToText(parseDocument(t1).model)
    expect(t2).toBe(t1)
  })

  it('o comentario vai na primeira liña e o corpo compila (HIDE + EVAL + RESP + SOL)', () => {
    const texto = documentToText(modeloExemplo())
    expect(texto.startsWith('<!--yang:tabla:2 ')).toBe(true)
    expect(texto).toContain('<HIDE>')
    expect(texto).toContain('<EVAL>base</EVAL>')
    expect(texto).toContain('<RESP>')
    expect(texto).toContain('<SOL>')
  })

  it('parseEmbed devolve null se non hai comentario', () => {
    expect(parseEmbed('<EX>ola</EX>')).toBeNull()
  })

  it('parseEmbed tolera un comentario corrupto (cae a legacy)', () => {
    const roto = '<!--yang:tabla:1 ***nonbase64*** -->\n<EX>ola</EX>'
    const { legacy } = parseDocument(roto)
    expect(legacy).toBe(true)
  })
})

describe('tokensToEval / evalToTokens', () => {
  it('convierte {nome} en <EVAL> e escapa {{ }}', () => {
    expect(tokensToEval('a {x} b {{lit}} c {y+1}')).toBe('a <EVAL>x</EVAL> b {lit} c <EVAL>y+1</EVAL>')
  })
  it('evalToTokens só tokeniza identificadores simples', () => {
    expect(evalToTokens('a <EVAL>x</EVAL> b <EVAL>solve(eq,x)</EVAL>')).toBe('a {x} b <EVAL>solve(eq,x)</EVAL>')
  })
  it('roundtrip texto->eval->texto para tokens simples', () => {
    const original = 'base {b} altura {h}'
    expect(evalToTokens(tokensToEval(original))).toBe(original)
  })

  // Report real: dous boletíns de electricidade seguidos xerados pola IA. As
  // chaves de LaTeX dentro dun <TIKZ> convertíanse en <EVAL> ("\\mathrm{W}"
  // -> "\\mathrm<EVAL>W</EVAL>") e o esquema deixaba de compilar. Dentro dun
  // bloque de código as chaves son sintaxe, non tokens.
  it('non tokeniza as chaves dentro dun <TIKZ>', () => {
    const tikz = '<TIKZ>\\node at (0,1) {$R_1$}; \\node at (2,1) {$5\\,\\mathrm{W}$};</TIKZ>'
    expect(tokensToEval('Debuxo: ' + tikz)).toBe('Debuxo: ' + tikz)
  })

  it('tokeniza a prosa de arredor pero non o bloque de código', () => {
    expect(tokensToEval('R = {r1} <TIKZ>\\node {$R$};</TIKZ> fin {v}')).toBe(
      'R = <EVAL>r1</EVAL> <TIKZ>\\node {$R$};</TIKZ> fin <EVAL>v</EVAL>',
    )
  })

  // Compatibilidade: os documentos xa gardados traen o TikZ coas chaves
  // dobradas (era o único patrón que funcionaba antes), e teñen que seguir
  // saíndo igual de ben.
  it('segue desescapando {{ }} dentro dun <TIKZ>', () => {
    expect(tokensToEval('<TIKZ>\\node at (0,1) {{$R_1$}};</TIKZ>')).toBe(
      '<TIKZ>\\node at (0,1) {$R_1$};</TIKZ>',
    )
  })

  // Dentro dun bloque de código só se resolve o que É unha variable
  // declarada: se non, un circuíto amosaba a letra "r1" onde ía o número
  // (report real, exame elec_1). O resto de chaves son LaTeX e quedan.
  it('dentro dun <TIKZ> só tokeniza as variables declaradas', () => {
    const tikz =
      '<TIKZ>\\node at (0,1) {$R_1={r1}\\,\\Omega$}; \\node at (2,1) {$5\\,\\mathrm{W}$}; \\node at (3,1) {$+$};</TIKZ>'
    expect(tokensToEval(tikz, ['r1'])).toBe(
      '<TIKZ>\\node at (0,1) {$R_1=<EVAL>r1</EVAL>\\,\\Omega$}; \\node at (2,1) {$5\\,\\mathrm{W}$}; \\node at (3,1) {$+$};</TIKZ>',
    )
  })

  it('sen lista de variables non tokeniza nada dentro dun <TIKZ>', () => {
    const tikz = '<TIKZ>\\node at (0,1) {$R_1={r1}$};</TIKZ>'
    expect(tokensToEval(tikz)).toBe(tikz)
  })

  it('evalToTokens deixa intacto o interior dun <TIKZ>', () => {
    const body = 'val <EVAL>x</EVAL> <TIKZ>\\node {$<EVAL>r1</EVAL>$};</TIKZ>'
    expect(evalToTokens(body)).toBe('val {x} <TIKZ>\\node {$<EVAL>r1</EVAL>$};</TIKZ>')
  })

  // Report real: un exercicio de matrices na Táboa con
  // "<MAT>A = matrix([{a}, {b}], [1, 2])</MAT>" e a,b declaradas. O token
  // convertíase en "<EVAL>a</EVAL>" DENTRO do <MAT>; cas non expande
  // etiquetas aniñadas nun <MAT>, mandaba ese "<EVAL>" literal a Maxima e
  // rebentaba ("incorrect syntax: < is not a prefix operator"). Dentro dun
  // bloque Maxima o token ten que quedar como o NOME ESPIDO - o <HIDE> xa o
  // liga na mesma sesión.
  it('dentro dun <MAT>/<EVAL> o token dunha variable declarada queda como nome espido', () => {
    expect(tokensToEval('<MAT>A = matrix([{a}, {b}], [1, 2])</MAT>', ['a', 'b'])).toBe(
      '<MAT>A = matrix([a, b], [1, 2])</MAT>',
    )
    expect(tokensToEval('<EVAL>{a}+{b}</EVAL>', ['a', 'b'])).toBe('<EVAL>a+b</EVAL>')
    expect(tokensToEval('<SISTEMA>x + y = {a}\nx - y = {b}</SISTEMA>', ['a', 'b'])).toBe(
      '<SISTEMA>x + y = a\nx - y = b</SISTEMA>',
    )
  })

  it('dentro dun <MAT> unha chave non declarada (conxunto de Maxima) non se toca', () => {
    expect(tokensToEval('<MAT>elementp(x, {1, 2, 3})</MAT>', ['a'])).toBe(
      '<MAT>elementp(x, {1, 2, 3})</MAT>',
    )
  })

  it('o mesmo token segue indo a <EVAL> na prosa e nun <TIKZ>, só nun bloque Maxima queda espido', () => {
    expect(tokensToEval('vale {a} e <MAT>b = {a}+1</MAT>', ['a'])).toBe(
      'vale <EVAL>a</EVAL> e <MAT>b = a+1</MAT>',
    )
  })
})

describe('varToMaxima', () => {
  it('fixa', () => {
    expect(varToMaxima({ nome: 'g', modo: 'fixa', valor: '9.8' })).toBe('g: 9.8$')
  })
  it('auto (asignación e definición de función)', () => {
    expect(varToMaxima({ nome: 'area', modo: 'auto', expr: 'b*h/2' })).toBe('area: b*h/2$')
    expect(varToMaxima({ nome: 'f(x)', modo: 'auto', expr: 'x^2', defOp: ':=' })).toBe('f(x) := x^2$')
  })
  it('aleatoria con xerador con nome', () => {
    expect(varToMaxima({ nome: 'n', modo: 'aleatoria', xer: 'n1' })).toBe('n: n1()$')
  })
  it('aleatoria rango paso 1 (conta numérica pechada)', () => {
    expect(varToMaxima({ nome: 'b', modo: 'aleatoria', xer: 'rango', min: '2', max: '9', paso: '1' }))
      .toBe('b: (2) + random(8)$')
  })
  it('aleatoria rango con paso', () => {
    expect(varToMaxima({ nome: 'b', modo: 'aleatoria', xer: 'rango', min: '0', max: '100', paso: '5' }))
      .toBe('b: (0) + (5)*random(floor(((100)-(0))/(5))+1)$')
  })
  it('variable sen nome ignórase', () => {
    expect(varToMaxima({ nome: '', modo: 'fixa', valor: '1' })).toBe('')
  })
})

describe('hideParaVariables (importación legacy)', () => {
  it('clasifica cada lado dereito', () => {
    const vars = hideParaVariables('a: 3$ n: n1()$ b: (2) + random(8)$ area: a*b/2')
    expect(vars).toEqual([
      { nome: 'a', modo: 'fixa', valor: '3' },
      { nome: 'n', modo: 'aleatoria', xer: 'n1' },
      { nome: 'b', modo: 'aleatoria', xer: 'rango', min: '2', max: '9', paso: '1' },
      { nome: 'area', modo: 'auto', expr: 'a*b/2' },
    ])
  })
  it('recoñece definicións de función', () => {
    const [v] = hideParaVariables('f(x):=x^2+1')
    expect(v).toMatchObject({ nome: 'f(x)', modo: 'auto', defOp: ':=', expr: 'x^2+1' })
  })
})

describe('parseDocument — .matex legacy (sen comentario)', () => {
  it('un documento con <EX> importa un exercicio por bloque', () => {
    const texto = '<EX><HIDE>a: 3$ b: n1()</HIDE>Calcula <EVAL>a</EVAL>+<EVAL>b</EVAL>.<RESP><EVAL>a+b</EVAL></RESP></EX>\n\n<EX>Segundo exercicio.</EX>\n'
    const { model, legacy } = parseDocument(texto)
    expect(legacy).toBe(true)
    expect(model.exercicios).toHaveLength(2)
    expect(model.exercicios[0].enunciado).toBe('Calcula {a}+{b}.')
    expect(model.exercicios[0].variables.map((v) => v.nome)).toEqual(['a', 'b'])
    expect(model.exercicios[0].resposta).toBe('<EVAL>a+b</EVAL>')
    expect(model.exercicios[1].enunciado).toBe('Segundo exercicio.')
  })

  it('un .matex sen <EX> vira un só exercicio cun enunciado opaco', () => {
    const original = readTestdata('mini.matex')
    const { model, legacy } = parseDocument(original)
    expect(legacy).toBe(true)
    expect(model.exercicios).toHaveLength(1)
    expect(model.exercicios[0].enunciado).toContain('<EVAL>1+1</EVAL>')
  })

  it('tras importar legacy, documentToText+parseDocument xa é estable', () => {
    const original = readTestdata('bloques.matex')
    const { model } = parseDocument(original)
    const t1 = documentToText(model)
    const t2 = documentToText(parseDocument(t1).model)
    expect(t2).toBe(t1)
  })
})

describe('normalizeModel', () => {
  it('reconstrúe unha estrutura mínima a partir de lixo', () => {
    const m = normalizeModel({ exercicios: [{ enunciado: 5, variables: [{ modo: 'raro' }, null, 42] }] })
    expect(m.exercicios[0].enunciado).toBe('5')
    expect(m.exercicios[0].variables).toEqual([{ nome: '', modo: 'aleatoria', xer: 'n1' }])
  })
  it('makeVariable ten forma coherente por modo', () => {
    expect(makeVariable('x', 'fixa')).toEqual({ nome: 'x', modo: 'fixa', valor: '' })
    expect(makeVariable('x', 'auto')).toEqual({ nome: 'x', modo: 'auto', expr: '' })
    expect(makeVariable('x', 'aleatoria')).toEqual({ nome: 'x', modo: 'aleatoria', xer: 'n1' })
  })
})

describe('resultados (yang:tabla:2) — fonte única do resultado final', () => {
  function modeloConResultados() {
    const ex = makeExercise()
    ex.enunciado = 'Rectángulo de base {base} e altura {altura}.'
    ex.variables = [
      { nome: 'base', modo: 'aleatoria', xer: 'n1' },
      { nome: 'altura', modo: 'aleatoria', xer: 'n1' },
    ]
    ex.resultados = [
      makeResultado('Área', 'res_1', 'base*altura'),
      makeResultado('Perímetro', 'res_2', '2*base+2*altura'),
    ]
    return { v: 2, exercicios: [ex] }
  }

  it('makeExercise trae resultados: []', () => {
    expect(makeExercise().resultados).toEqual([])
  })

  it('as fórmulas van no <HIDE> como liñas auto, despois das variables', () => {
    const body = modelToBody(modeloConResultados())
    expect(body).toContain('base: n1()$ altura: n1()$ res_1: base*altura$ res_2: 2*base+2*altura$')
  })

  it('sen `resposta` escrita, constrúese un <RESP> por defecto desde resultados', () => {
    const body = modelToBody(modeloConResultados())
    expect(body).toContain('<RESP>')
    expect(body).toContain('<b>Área:</b> <MAT>base*altura</MAT> = <EVAL>res_1</EVAL>')
    expect(body).toContain('<b>Perímetro:</b> <MAT>2*base+2*altura</MAT> = <EVAL>res_2</EVAL>')
  })

  it('unha `resposta` escrita a man gaña sobre a reconstrución automática', () => {
    const m = modeloConResultados()
    m.exercicios[0].resposta = 'A área é {res_1}.'
    const body = modelToBody(m)
    expect(body).toContain('<RESP>A área é <EVAL>res_1</EVAL>.</RESP>')
    expect(body).not.toContain('<MAT>base*altura</MAT>')
  })

  it('roundtrip: documentToText -> parseDocument conserva resultados (legacy:false, v2)', () => {
    const texto = documentToText(modeloConResultados())
    expect(texto.startsWith('<!--yang:tabla:2 ')).toBe(true)
    const { model, legacy } = parseDocument(texto)
    expect(legacy).toBe(false)
    expect(model.exercicios[0].resultados).toEqual([
      { etiqueta: 'Área', nome: 'res_1', formula: 'base*altura' },
      { etiqueta: 'Perímetro', nome: 'res_2', formula: '2*base+2*altura' },
    ])
  })

  it('documentToText segue sendo idempotente con resultados', () => {
    const t1 = documentToText(modeloConResultados())
    const t2 = documentToText(parseDocument(t1).model)
    expect(t2).toBe(t1)
  })

  it('un comentario v1 (sen resultados) lese engadindo unha lista baleira', () => {
    const ex = makeExercise()
    ex.enunciado = 'Ola {a}.'
    ex.variables = [{ nome: 'a', modo: 'aleatoria', xer: 'n1' }]
    const v1 = { v: 1, exercicios: [{ id: ex.id, titulo: '', enunciado: ex.enunciado, variables: ex.variables, resposta: '', solucion: '' }] }
    // simula un embed v1: mesmo mecanismo ca parseEmbed pero forzando o JSON cru
    const m = normalizeModel(v1)
    expect(m.v).toBe(MODELO_VERSION)
    expect(m.exercicios[0].resultados).toEqual([])
  })

  it('normalizeResultado descarta entradas sen fórmula e completa o nome que falte', () => {
    const m = normalizeModel({ exercicios: [{ resultados: [
      { etiqueta: 'A', formula: 'x+1' },        // sen nome -> res_1
      { etiqueta: 'ruído', formula: '' },       // sen fórmula -> fóra
      { etiqueta: 'B', nome: 'y', formula: 'x*2' },
    ] }] })
    expect(m.exercicios[0].resultados).toEqual([
      { etiqueta: 'A', nome: 'res_1', formula: 'x+1' },
      { etiqueta: 'B', nome: 'y', formula: 'x*2' },
    ])
  })

  it('resultadoToMaxima / respostaDesdeResultados', () => {
    expect(resultadoToMaxima({ nome: 'res_1', formula: 'a/b' })).toBe('res_1: a/b$')
    expect(resultadoToMaxima({ nome: '', formula: 'a/b' })).toBe('')
    // respostaDesdeResultados usa a convención de tokens {nome} da táboa;
    // modelToBody é quen os converte a <EVAL> co seu tokensToEval.
    expect(respostaDesdeResultados([{ etiqueta: 'X', nome: 'res_1', formula: 'a+b' }]))
      .toBe('<p><b>X:</b> <MAT>a+b</MAT> = {res_1}</p>')
  })
})
