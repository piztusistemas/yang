// @vitest-environment jsdom
//
// Proba de integración DOM real de createBlocksEditor (Blockly.inject
// completo, non un workspace headless coma en blockly/adapter.test.js) -
// jsdom non implementa xeometría SVG real (getBBox/getComputedTextLength/
// getScreenCTM/createSVGPoint, ver o plan), así que se stuban só neste
// ficheiro, en beforeAll, antes de importar Blockly.
import { describe, it, expect, beforeAll, afterEach } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const testdataDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'testdata')
function readTestdata(name) {
  return readFileSync(join(testdataDir, name), 'utf8')
}

let createBlocksEditor
let Blockly

beforeAll(async () => {
  SVGElement.prototype.getBBox = () => ({ x: 0, y: 0, width: 100, height: 20 })
  SVGElement.prototype.getComputedTextLength = () => 50
  SVGElement.prototype.getScreenCTM = () => ({ a: 1, b: 0, c: 0, d: 1, e: 0, f: 0 })
  SVGElement.prototype.createSVGPoint = () => ({ x: 0, y: 0, matrixTransform: () => ({ x: 0, y: 0 }) })
  // jsdom non implementa ResizeObserver (ver blocks-editor.js) - stub sen
  // op abonda para estes tests, que non dependen de que dispare de verdade.
  global.ResizeObserver = class { observe() {} disconnect() {} }
  ;({ createBlocksEditor } = await import('./blocks-editor.js'))
  Blockly = await import('blockly/core')
})

let container
afterEach(() => {
  if (container) container.remove()
  container = null
  // As modais de IA ("Exercicio/Exame con IA") xa non se pechan ao premer
  // "Xerar" (déixanse abertas para afinar o prompt) e o seu overlay está
  // cacheado a nivel de módulo: hai que agochalo a man entre probas, senón
  // `document.querySelector('.overlay:not(.hidden) .modal')` da proba
  // seguinte colle o modal vello.
  document.querySelectorAll('.overlay').forEach((o) => o.classList.add('hidden'))
})

// responderDialogo preme un botón do diálogo de src/dialogs.js: o lixo xa
// non usa window.confirm (en macOS o WKWebView de Wails devólveo sempre
// false sen preguntar, ver a cabeceira de dialogs.js). abrirDialogo() monta
// o overlay de maneira síncrona, así que xa está no DOM en canto se preme
// o lixo; despois de responder abonda un tic de microtarefas para que siga
// o `await` de trashcan.click.
async function responderDialogo(aceptar) {
  const overlay = document.querySelector('.overlay--dialog')
  expect(overlay).toBeTruthy()
  const botons = overlay.querySelectorAll('.modal-actions button')
  ;(aceptar ? overlay.querySelector('.modal-actions button.primary') : botons[0]).click()
  await Promise.resolve()
  await Promise.resolve()
}

function mount(opts) {
  container = document.createElement('div')
  container.style.height = '400px'
  document.body.appendChild(container)
  return createBlocksEditor(container, opts)
}

describe('createBlocksEditor (Blockly) — contrato getValue/setValue/focus/destroy', () => {
  it('reproduce un .matex legacy real byte a byte sen tocar nada', () => {
    const original = readTestdata('bloques.matex')
    const editor = mount({ initialDoc: original })
    expect(editor.getValue()).toBe(original)
    editor.destroy()
  })

  for (const name of ['mini.matex', 'test.matex', 'modelo.examen.matex']) {
    it(`reproduce ${name} (legacy, sen <EX>) byte a byte`, () => {
      const original = readTestdata(name)
      const editor = mount({ initialDoc: original })
      expect(editor.getValue()).toBe(original)
      editor.destroy()
    })
  }

  it('setValue substitúe o contido enteiro', () => {
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n' })
    editor.setValue('<EX><EVAL>2+2</EVAL></EX>\n')
    expect(editor.getValue()).toBe('<EX><EVAL>2+2</EVAL></EX>\n')
    editor.destroy()
  })

  it('"🗂️ Exercicios" -> "+" engade un exercicio baleiro e dá de alta <EX></EX>', () => {
    const editor = mount({ initialDoc: '' })
    const exercisesBtn = container.querySelector('.blocks-toolbar__exercises')
    expect(exercisesBtn).toBeTruthy()
    exercisesBtn.click()
    const addTile = document.querySelector('.overlay:not(.hidden) .exercise-tile--add')
    expect(addTile).toBeTruthy()
    addTile.click()
    expect(editor.getValue()).toBe('<EX></EX>\n')
    editor.destroy()
  })

  it('destroy() baleira o contedor e non deixa o editor rexistrado', () => {
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n' })
    editor.destroy()
    expect(container.innerHTML).toBe('')
    expect(container.classList.contains('blocks-root')).toBe(false)
  })

  it('premer o campo de contido dun bloque abre a modal, e "Gardar" actualiza o texto', () => {
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n' })
    const anacoBlock = editor._workspaceForTests.getTopBlocks(true)[0].getInputTargetBlock('ANACOS')
    // Simula o clic: showEditor_ é exactamente o método que Blockly chama
    // ao premer un campo editable (ver content-field.js).
    anacoBlock.getField('CONTENT').showEditor_()

    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    expect(modal).toBeTruthy()
    expect(modal.querySelector('h2').textContent).toContain('Fórmula')
    const textarea = modal.querySelector('textarea')
    expect(textarea.value).toBe('1+1')
    textarea.value = '2+2'
    modal.querySelector('.modal-actions .primary').click()

    expect(editor.getValue()).toBe('<EX><EVAL>2+2</EVAL></EX>\n')
    editor.destroy()
  })
})

describe('createBlocksEditor (Blockly) — exercicios coma "obxectos" (ventá 🗂️ Exercicios)', () => {
  const DOUS_EXERCICIOS = '<EX><EVAL>1+1</EVAL></EX>\n\n<EX><EVAL>2+2</EVAL></EX>\n'

  it('por defecto só se ve o PRIMEIRO exercicio no lenzo, aínda que o documento teña varios', () => {
    const editor = mount({ initialDoc: DOUS_EXERCICIOS })
    expect(editor._workspaceForTests.getTopBlocks(true).length).toBe(1)
    const anacoBlock = editor._workspaceForTests.getTopBlocks(true)[0].getInputTargetBlock('ANACOS')
    expect(anacoBlock.getFieldValue('CONTENT')).toBe('1+1')
    // getValue() segue a devolver o documento ENTEIRO (os dous exercicios),
    // aínda que o lenzo só amose o primeiro.
    expect(editor.getValue()).toBe(DOUS_EXERCICIOS)
    editor.destroy()
  })

  it('dobre clic nunha caixa da ventá troca de exercicio activo, conservando as edicións do anterior', () => {
    const editor = mount({ initialDoc: DOUS_EXERCICIOS })
    // Edita o exercicio 1 (activo) antes de trocar.
    editor._workspaceForTests.getTopBlocks(true)[0].getInputTargetBlock('ANACOS').setFieldValue('3+3', 'CONTENT')

    container.querySelector('.blocks-toolbar__exercises').click()
    const tiles = document.querySelectorAll('.overlay:not(.hidden) .exercise-tile:not(.exercise-tile--add)')
    expect(tiles.length).toBe(2)
    tiles[1].dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))

    // O lenzo agora amosa o exercicio 2.
    const anacoBlock = editor._workspaceForTests.getTopBlocks(true)[0].getInputTargetBlock('ANACOS')
    expect(anacoBlock.getFieldValue('CONTENT')).toBe('2+2')
    // ...e a edición do exercicio 1 non se perdeu.
    expect(editor.getValue()).toBe('<EX><EVAL>3+3</EVAL></EX>\n\n<EX><EVAL>2+2</EVAL></EX>\n')
    editor.destroy()
  })

  it('🗑 (eliminar) dende a ventá quita o exercicio de getValue()', () => {
    const editor = mount({ initialDoc: DOUS_EXERCICIOS })
    container.querySelector('.blocks-toolbar__exercises').click()
    const tiles = document.querySelectorAll('.overlay:not(.hidden) .exercise-tile:not(.exercise-tile--add)')
    tiles[1].querySelector('[aria-label="Eliminar"]').click()

    expect(editor.getValue()).toBe('<EX><EVAL>1+1</EVAL></EX>\n')
    editor.destroy()
  })

  it('premer o lixo baleiro (sen bloques eliminados aínda) pide confirmación e baleira o exercicio activo', async () => {
    const editor = mount({ initialDoc: DOUS_EXERCICIOS })
    const workspace = editor._workspaceForTests
    expect(workspace.getTopBlocks(false).length).toBe(1)

    workspace.trashcan.click()
    await responderDialogo(true)

    expect(workspace.getTopBlocks(false).length).toBe(0)
    // O outro exercicio (nunca aberto no lenzo) queda intacto.
    expect(editor.getValue()).toBe('<EX></EX>\n\n<EX><EVAL>2+2</EVAL></EX>\n')
    editor.destroy()
  })

  it('premer o lixo baleiro cancelando a confirmación non toca nada', async () => {
    const editor = mount({ initialDoc: DOUS_EXERCICIOS })
    const workspace = editor._workspaceForTests

    workspace.trashcan.click()
    await responderDialogo(false)

    expect(workspace.getTopBlocks(false).length).toBe(1)
    editor.destroy()
  })

  it('⧉ (duplicar) dende a ventá clona o contido do exercicio', () => {
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n' })
    container.querySelector('.blocks-toolbar__exercises').click()
    const tiles = document.querySelectorAll('.overlay:not(.hidden) .exercise-tile:not(.exercise-tile--add)')
    tiles[0].querySelector('[aria-label="Duplicar"]').click()

    expect(editor.getValue()).toBe('<EX><EVAL>1+1</EVAL></EX>\n\n<EX><EVAL>1+1</EVAL></EX>\n')
    editor.destroy()
  })
})

describe('createBlocksEditor (Blockly) — paridade funcional (IA de exercicio/exame, biblioteca)', () => {
  it('"✨ Exercicio con IA" engade un exercicio novo cos anacos devoltos', async () => {
    const generateExerciseWithAI = async (peticion) => {
      expect(peticion).toBe('derivada de x^2')
      return [{ type: 'text', content: 'Deriva a función' }, { type: 'formula', content: "diff(x^2,x)" }]
    }
    const editor = mount({ initialDoc: '', generateExerciseWithAI })
    container.querySelector('.blocks-toolbar__ai').click()

    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    modal.querySelector('textarea').value = 'derivada de x^2'
    modal.querySelector('.modal-actions .primary').click()
    await Promise.resolve()
    await Promise.resolve()

    expect(editor.getValue()).toBe('<EX>Deriva a función<EVAL>diff(x^2,x)</EVAL></EX>\n')
    editor.destroy()
  })

  it('"✨ Exame completo con IA" engade varios exercicios de vez', async () => {
    const generateExamWithAI = async (tema, num) => {
      expect(tema).toBe('trigonometría')
      return [[{ type: 'text', content: 'Ex1' }], [{ type: 'text', content: 'Ex2' }]]
    }
    const editor = mount({ initialDoc: '', generateExamWithAI })
    const btns = container.querySelectorAll('.blocks-toolbar__ai')
    btns[btns.length - 1].click()

    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    modal.querySelector('textarea').value = 'trigonometría'
    modal.querySelector('.modal-actions .primary').click()
    await Promise.resolve()
    await Promise.resolve()

    expect(editor.getValue()).toBe('<EX>Ex1</EX>\n\n<EX>Ex2</EX>\n')
    editor.destroy()
  })

  it('"✨ Exame completo con IA": por defecto mándase o número escrito na casa', async () => {
    let recibido = null
    const generateExamWithAI = async (tema, num) => {
      recibido = num
      return [[{ type: 'text', content: 'Ex1' }]]
    }
    const editor = mount({ initialDoc: '', generateExamWithAI })
    const btns = container.querySelectorAll('.blocks-toolbar__ai')
    btns[btns.length - 1].click()

    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    modal.querySelector('input[type="number"]').value = '7'
    modal.querySelector('textarea').value = 'trigonometría'
    modal.querySelector('.modal-actions .primary').click()
    await Promise.resolve()
    await Promise.resolve()

    expect(recibido).toBe(7)
    editor.destroy()
  })

  // "Indefinido" viaxa coma 0: é o que XerarExameIA (app.go) entende por
  // "decide ti cantos", sen o tope de 20 que si se aplica a un número
  // pedido á man.
  it('"✨ Exame completo con IA": marcar "Indefinido" manda 0 e desactiva o número', async () => {
    let recibido = null
    const generateExamWithAI = async (tema, num) => {
      recibido = num
      return [[{ type: 'text', content: 'Ex1' }], [{ type: 'text', content: 'Ex2' }], [{ type: 'text', content: 'Ex3' }]]
    }
    const editor = mount({ initialDoc: '', generateExamWithAI })
    const btns = container.querySelectorAll('.blocks-toolbar__ai')
    btns[btns.length - 1].click()

    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    const numInput = modal.querySelector('input[type="number"]')
    const indef = modal.querySelector('.modal-label-checkbox input[type="checkbox"]')
    expect(numInput.disabled).toBe(false)
    indef.checked = true
    indef.dispatchEvent(new Event('change'))
    expect(numInput.disabled).toBe(true)

    modal.querySelector('textarea').value = 'trigonometría'
    modal.querySelector('.modal-actions .primary').click()
    await Promise.resolve()
    await Promise.resolve()

    expect(recibido).toBe(0)
    expect(editor.getValue()).toBe('<EX>Ex1</EX>\n\n<EX>Ex2</EX>\n\n<EX>Ex3</EX>\n')
    editor.destroy()
  })

  it('"💾 Gardar exercicio na biblioteca" (menú contextual) chama saveToLibrary co texto do exercicio', () => {
    let gardado = null
    const saveToLibrary = async (nome, tipo, contido) => { gardado = { nome, tipo, contido } }
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n', saveToLibrary })
    const exBlock = editor._workspaceForTests.getTopBlocks(true)[0]

    const item = Blockly.ContextMenuRegistry.registry.getItem('matexeSaveExerciseToLibrary')
    expect(item.preconditionFn({ block: exBlock })).toBe('enabled')
    item.callback({ block: exBlock })

    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    expect(modal.querySelector('h2').textContent).toContain('exercicio')
    modal.querySelector('input[type="text"]').value = 'O meu exercicio'
    modal.querySelector('.modal-actions .primary').click()

    expect(gardado).toEqual({ nome: 'O meu exercicio', tipo: 'exercicio', contido: '<EVAL>1+1</EVAL>' })
    editor.destroy()
  })

  it('"💾 Gardar bloque na biblioteca" (menú contextual dun anaco) chama saveToLibrary só co seu contido', () => {
    let gardado = null
    const saveToLibrary = async (nome, tipo, contido) => { gardado = { nome, tipo, contido } }
    const editor = mount({ initialDoc: '<EX>Enunciado<EVAL>1+1</EVAL></EX>\n', saveToLibrary })
    const exBlock = editor._workspaceForTests.getTopBlocks(true)[0]
    const formulaBlock = exBlock.getInputTargetBlock('ANACOS').getNextBlock()

    const exItem = Blockly.ContextMenuRegistry.registry.getItem('matexeSaveExerciseToLibrary')
    const blockItem = Blockly.ContextMenuRegistry.registry.getItem('matexeSaveBlockToLibrary')
    // O exercicio NON debe ofrecer "gardar bloque", e o anaco NON debe
    // ofrecer "gardar exercicio" - cada un só o seu.
    expect(exItem.preconditionFn({ block: exBlock })).toBe('enabled')
    expect(blockItem.preconditionFn({ block: exBlock })).toBe('hidden')
    expect(blockItem.preconditionFn({ block: formulaBlock })).toBe('enabled')
    expect(exItem.preconditionFn({ block: formulaBlock })).toBe('hidden')

    blockItem.callback({ block: formulaBlock })
    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    modal.querySelector('input[type="text"]').value = 'A miña fórmula'
    modal.querySelector('.modal-actions .primary').click()

    expect(gardado).toEqual({ nome: 'A miña fórmula', tipo: 'bloque', contido: '<EVAL>1+1</EVAL>' })
    editor.destroy()
  })

  it('menú contextual de gardar na biblioteca queda agochado sen saveToLibrary', () => {
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n' })
    const exBlock = editor._workspaceForTests.getTopBlocks(true)[0]
    const item = Blockly.ContextMenuRegistry.registry.getItem('matexeSaveExerciseToLibrary')
    expect(item.preconditionFn({ block: exBlock })).toBe('hidden')
    editor.destroy()
  })
})

describe('createBlocksEditor (Blockly) — "Ver código" (menú contextual do lenzo / de "🗂️ Exercicios")', () => {
  const DOUS_EXERCICIOS = '<EX><EVAL>1+1</EVAL></EX>\n\n<EX><EVAL>2+2</EVAL></EX>\n'

  it('menú do lenzo agochado sen exercicio activo (exame baleiro)', () => {
    const editor = mount({ initialDoc: '' })
    const item = Blockly.ContextMenuRegistry.registry.getItem('matexeViewExerciseCode')
    expect(item.preconditionFn({ workspace: editor._workspaceForTests })).toBe('hidden')
    editor.destroy()
  })

  it('"Ver código" (lenzo) amosa o texto SÓ do exercicio activo, oculta o lenzo', () => {
    const editor = mount({ initialDoc: DOUS_EXERCICIOS })
    const item = Blockly.ContextMenuRegistry.registry.getItem('matexeViewExerciseCode')
    expect(item.preconditionFn({ workspace: editor._workspaceForTests })).toBe('enabled')
    item.callback({ workspace: editor._workspaceForTests })

    expect(container.querySelector('.blocks-canvas').classList.contains('hidden')).toBe(true)
    expect(container.querySelector('.blocks-code-view')).toBeTruthy()
    expect(editor._codeViewApiForTests().getValue()).toBe('<EVAL>1+1</EVAL>')
    editor.destroy()
  })

  it('editar o código do exercicio e premer "Volver a bloques" aplica o cambio ao lenzo', () => {
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n' })
    const item = Blockly.ContextMenuRegistry.registry.getItem('matexeViewExerciseCode')
    item.callback({ workspace: editor._workspaceForTests })

    editor._codeViewApiForTests().setValue('<EVAL>9+9</EVAL>')
    container.querySelector('.blocks-code-view__back').click()

    expect(container.querySelector('.blocks-code-view')).toBeFalsy()
    expect(container.querySelector('.blocks-canvas').classList.contains('hidden')).toBe(false)
    expect(editor.getValue()).toBe('<EX><EVAL>9+9</EVAL></EX>\n')
    editor.destroy()
  })

  it('getValue() reflicte o código aínda sen premer "Volver a bloques"', () => {
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n' })
    const item = Blockly.ContextMenuRegistry.registry.getItem('matexeViewExerciseCode')
    item.callback({ workspace: editor._workspaceForTests })
    editor._codeViewApiForTests().setValue('<EVAL>5+5</EVAL>')

    expect(editor.getValue()).toBe('<EX><EVAL>5+5</EVAL></EX>\n')
    // ...e a vista de código segue aberta (getValue non a pecha).
    expect(container.querySelector('.blocks-code-view')).toBeTruthy()
    editor.destroy()
  })

  it('botón dereito en "🗂️ Exercicios" abre un menú con "Ver código" para o exame ENTEIRO', () => {
    const editor = mount({ initialDoc: DOUS_EXERCICIOS })
    const exercisesBtn = container.querySelector('.blocks-toolbar__exercises')
    exercisesBtn.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, clientX: 10, clientY: 10 }))

    const menuItem = document.querySelector('.blocks-simple-menu__item')
    expect(menuItem).toBeTruthy()
    menuItem.click()

    expect(container.querySelector('.blocks-code-view')).toBeTruthy()
    expect(editor._codeViewApiForTests().getValue()).toBe(DOUS_EXERCICIOS)
    editor.destroy()
  })

  it('trocar de exercicio (ventá 🗂️ Exercicios) mentres se ve o código gárdao antes de trocar', () => {
    const editor = mount({ initialDoc: DOUS_EXERCICIOS })
    const item = Blockly.ContextMenuRegistry.registry.getItem('matexeViewExerciseCode')
    item.callback({ workspace: editor._workspaceForTests }) // ver código do exercicio 1
    editor._codeViewApiForTests().setValue('<EVAL>7+7</EVAL>')

    container.querySelector('.blocks-toolbar__exercises').click()
    const tiles = document.querySelectorAll('.overlay:not(.hidden) .exercise-tile:not(.exercise-tile--add)')
    tiles[1].dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))

    expect(container.querySelector('.blocks-code-view')).toBeFalsy()
    expect(editor.getValue()).toBe('<EX><EVAL>7+7</EVAL></EX>\n\n<EX><EVAL>2+2</EVAL></EX>\n')
    editor.destroy()
  })
})

describe('createBlocksEditor (Blockly) — anacos soltos fóra de calquera exercicio', () => {
  // soltarAnaco simula o resultado dun arrastre que acaba FÓRA do bloque
  // "Exercicio": un bloque-anaco de nivel superior, sen conectar a nada.
  function soltarAnaco(workspace, type, content) {
    const block = workspace.newBlock(type)
    block.initSvg()
    block.render()
    block.setFieldValue(content, 'CONTENT')
    return block
  }

  it('unha imaxe solta nun exame novo non se perde: getValue() dálle un exercicio', () => {
    const editor = mount({ initialDoc: '' })
    soltarAnaco(editor._workspaceForTests, 'matexe_image_upload', 'imaxes/foto.png')
    expect(editor.getValue()).toBe('<EX><IMG src="imaxes/foto.png"></EX>\n')
    editor.destroy()
  })

  it('un anaco solto cun exercicio xa aberto vai ao final da súa pila', () => {
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n' })
    soltarAnaco(editor._workspaceForTests, 'matexe_image_upload', 'logo.png')
    expect(editor.getValue()).toBe('<EX><EVAL>1+1</EVAL><IMG src="logo.png"></EX>\n')
    editor.destroy()
  })

  it('varios anacos soltos entran todos, e o lenzo queda cun só exercicio', () => {
    const editor = mount({ initialDoc: '' })
    const ws = editor._workspaceForTests
    soltarAnaco(ws, 'matexe_text', 'Enunciado')
    soltarAnaco(ws, 'matexe_formula', '2+2')
    expect(editor.getValue()).toBe('<EX>Enunciado<EVAL>2+2</EVAL></EX>\n')
    expect(ws.getTopBlocks(false).length).toBe(1)
    editor.destroy()
  })

  it('os demais exercicios do exame non se tocan ao adoptar un anaco solto', () => {
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n\n<EX><EVAL>2+2</EVAL></EX>\n' })
    soltarAnaco(editor._workspaceForTests, 'matexe_variables', 'a: 1')
    expect(editor.getValue()).toBe('<EX><EVAL>1+1</EVAL><HIDE>a: 1</HIDE></EX>\n\n<EX><EVAL>2+2</EVAL></EX>\n')
    editor.destroy()
  })
})
