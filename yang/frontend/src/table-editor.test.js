// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest'

// confirmar() amosa unha modal e resolve ao premer un botón - nos tests
// dámola sempre por aceptada.
vi.mock('./dialogs.js', () => ({ confirmar: () => Promise.resolve(true) }))

import { createTableEditor } from './table-editor.js'
import { parseDocument } from './table-serialize.js'

function mount() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const ed = createTableEditor(host, { initialDoc: '' })
  return { host, ed }
}

beforeEach(() => { document.body.innerHTML = '' })

describe('createTableEditor — contrato básico', () => {
  it('arranca baleiro e getValue devolve ""', () => {
    const { ed } = mount()
    expect(ed.getValue()).toBe('')
    expect(ed.getModo()).toBe('enunciados')
  })

  it('setValue + getValue fai roundtrip dun documento de táboa', () => {
    const { ed } = mount()
    mountExemplo(ed)
    const once = ed.getValue()
    const reparsed = parseDocument(once)
    expect(reparsed.legacy).toBe(false)
    expect(reparsed.model.exercicios).toHaveLength(1)
    expect(once).toContain('<HIDE>')
    ed.setValue(once)
    expect(ed.getValue()).toBe(once) // idempotente
  })

  it('«＋ exercicio» engade unha fila', () => {
    const { host, ed } = mount()
    btn(host, '.table-toolbar__add').click()
    expect(ed._modelForTests().exercicios).toHaveLength(1)
    expect(host.querySelectorAll('.table-row:not(.table-row--head)').length).toBe(1)
  })

  it('engadir variable e cambiar o seu modo actualiza o campo de valor', () => {
    const { host, ed } = mount()
    btn(host, '.table-toolbar__add').click()
    btn(host, '.table-var-add').click()
    const modoSel = host.querySelector('.table-var-modo')
    expect(modoSel).toBeTruthy()
    // por defecto "aleatoria" -> hai un <select> de xerador
    expect(host.querySelector('.table-var-gen')).toBeTruthy()
    modoSel.value = 'fixa'
    modoSel.dispatchEvent(new Event('change'))
    expect(host.querySelector('.table-var-gen')).toBeNull()
    expect(ed._modelForTests().exercicios[0].variables[0].modo).toBe('fixa')
  })

  it('as lapelas cambian getModo, as columnas visibles e disparan onModoChange', () => {
    const { host, ed } = mount()
    const cb = vi.fn()
    ed.onModoChange(cb)
    btn(host, '.table-toolbar__add').click()

    expect(host.querySelector('.table-c-resp')).toBeNull()
    tab(host, 'resposta').click()
    expect(ed.getModo()).toBe('resposta')
    expect(cb).toHaveBeenCalledWith('resposta')
    expect(host.querySelector('.table-c-resp')).toBeTruthy()
    expect(host.querySelector('.table-c-sol')).toBeNull()

    tab(host, 'solucions').click()
    expect(host.querySelector('.table-c-sol')).toBeTruthy()
  })

  it('a lapela Test amosa o seu propio panel', () => {
    const { host } = mount()
    const tabTest = tab(host, 'proba')
    expect(tabTest.textContent).toBe('Test')
    tabTest.click()
    expect(host.querySelector('.table-probes__run')).toBeTruthy()
    expect(host.querySelector('.table-grid')).toBeNull()
  })

  it('o botón de Asistente segue visible en TODAS as lapelas, incluída Test', () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const ed = createTableEditor(host, {
      initialDoc: '',
      abrirAsistente: () => {},
      generateExerciseWithAI: async () => [],
      generateExamWithAI: async () => [],
    })
    const asistente = [...host.querySelectorAll('.table-toolbar__ai')]
      .find((b) => b.textContent.includes('Asistente'))
    expect(asistente).toBeTruthy()
    for (const m of ['enunciados', 'resposta', 'solucions', 'proba']) {
      ed.setModo(m)
      expect(asistente.style.display).toBe('')
    }
  })

  it('amosa os dous botóns de IA só se se pasan os xeradores', () => {
    const host1 = document.createElement('div')
    document.body.appendChild(host1)
    createTableEditor(host1, { initialDoc: '' })
    expect(host1.querySelectorAll('.table-toolbar__ai').length).toBe(0)

    const host2 = document.createElement('div')
    document.body.appendChild(host2)
    createTableEditor(host2, {
      initialDoc: '',
      generateExerciseWithAI: async () => [],
      generateExamWithAI: async () => [],
    })
    // dous botóns de IA de xeración (exercicio / exame); o de "Asistente" non
    // se conta porque non se pasou abrirAsistente.
    expect(host2.querySelectorAll('.table-toolbar__ai').length).toBe(2)
  })

  it('exercicioDesdeTaboa: cada campo da IA vai á súa columna', () => {
    const { ed } = mount()
    const ex = ed._exercicioDesdeTaboaForTests({
      enunciado: 'Resolve o sistema con <MAT>a</MAT> = {a}.',
      variables: [
        { nome: 'a', modo: 'aleatoria', xer: 'z0' },
        { nome: 'detA', modo: 'auto', expr: '1-a' },
        { nome: 'g', modo: 'fixa', valor: '9.8' },
      ],
      resultado: 'x = <EVAL>x0</EVAL>, y = <EVAL>y0</EVAL>.',
      resolucion: '<p>O determinante é <EVAL>detA</EVAL>.</p><ul><li>Se a≠1…</li></ul>',
    })
    expect(ex.enunciado).toBe('Resolve o sistema con <MAT>a</MAT> = {a}.')
    expect(ex.variables.map((v) => [v.nome, v.modo])).toEqual([['a', 'aleatoria'], ['detA', 'auto'], ['g', 'fixa']])
    expect(ex.variables[1].expr).toBe('1-a')
    expect(ex.variables[2].valor).toBe('9.8')
    expect(ex.resposta).toBe('x = {x0}, y = {y0}.') // <EVAL>id</EVAL> -> token
    expect(ex.solucion).toContain('Se a≠1')
    // <EVAL>id</EVAL> convértese a token editable
    expect(ex.solucion).toContain('{detA}')
  })

  it('exercicioDesdeTaboa: variable "rango" e campos ausentes', () => {
    const { ed } = mount()
    const ex = ed._exercicioDesdeTaboaForTests({
      enunciado: 'Suma {b} + 3.',
      variables: [{ nome: 'b', modo: 'aleatoria', xer: 'rango', min: '2', max: '9' }, { modo: 'fixa' }],
      resultado: '', resolucion: '',
    })
    expect(ex.variables).toHaveLength(1) // a variable sen nome descártase
    expect(ex.variables[0]).toMatchObject({ nome: 'b', modo: 'aleatoria', xer: 'rango', min: '2', max: '9', paso: '1' })
    expect(ex.resposta).toBe('')
    expect(ex.solucion).toBe('')
  })

  it('os botóns de xerar-con-IA ocúltanse na lapela Test', () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const ed = createTableEditor(host, {
      initialDoc: '',
      generateExerciseWithAI: async () => [],
      generateExamWithAI: async () => [],
    })
    ed.setModo('proba')
    for (const b of host.querySelectorAll('.table-toolbar__ai')) {
      expect(b.style.display).toBe('none')
    }
    ed.setModo('enunciados')
    for (const b of host.querySelectorAll('.table-toolbar__ai')) {
      expect(b.style.display).toBe('')
    }
  })

  it('inserir un token {var} mete <EVAL> no enunciado', () => {
    const { host, ed } = mount()
    btn(host, '.table-toolbar__add').click()
    // dálle nome a unha variable
    btn(host, '.table-var-add').click()
    const nome = host.querySelector('.table-var-nome')
    nome.value = 'x'
    nome.dispatchEvent(new Event('input'))
    // re-render tras cambiar algo estrutural non ocorre ao teclear o nome;
    // forzamos un render pedindo os chips (aparecen só se hai nomes)
    ed._renderForTests()
    const chip = host.querySelector('.table-token')
    expect(chip.textContent).toBe('{x}')
    chip.click()
    expect(ed._modelForTests().exercicios[0].enunciado).toContain('{x}')
    expect(ed.getValue()).toContain('<EVAL>x</EVAL>')
  })

  it('non hai botón «Cargar exemplo»', () => {
    const { host } = mount()
    const nomes = [...host.querySelectorAll('.table-tbtn')].map((b) => b.textContent)
    expect(nomes.some((x) => x.toLowerCase().includes('exemplo'))).toBe(false)
  })

  it('undo desfai a última operación estrutural', () => {
    const { host, ed } = mount()
    btn(host, '.table-toolbar__add').click()
    btn(host, '.table-toolbar__add').click()
    expect(ed._modelForTests().exercicios).toHaveLength(2)
    ed.undo()
    expect(ed._modelForTests().exercicios).toHaveLength(1)
    ed.redo()
    expect(ed._modelForTests().exercicios).toHaveLength(2)
  })
})

// ---- helpers ----
function btn(host, sel) {
  const b = host.querySelector(sel)
  if (!b) throw new Error('non se atopou ' + sel)
  return b
}
function tab(host, modo) {
  return [...host.querySelectorAll('.table-tab')].find((b) => b.dataset.modo === modo)
}
function mountExemplo(ed) {
  ed.setValue('')
  // constrúe o exemplo a través do seu propio botón sería async; máis
  // directo: pásalle un documento xa serializado por table-serialize e
  // comproba o roundtrip.
  const json = '{"v":1,"exercicios":[{"id":"e1","titulo":"","enunciado":"Suma {a}+{b}.","variables":[{"nome":"a","modo":"aleatoria","xer":"n1"},{"nome":"b","modo":"fixa","valor":"3"}],"resposta":"Total {a}+{b}.","solucion":""}]}'
  const src = [
    '<!--yang:tabla:1 ' + btoa(json) + ' -->',
    '<EX>',
    '<HIDE>a: n1()$ b: 3$</HIDE>',
    'Suma <EVAL>a</EVAL>+<EVAL>b</EVAL>.',
    '<RESP>Total <EVAL>a</EVAL>+<EVAL>b</EVAL>.</RESP>',
    '</EX>',
    '',
  ].join('\n')
  ed.setValue(src)
  return ed.getValue()
}
