// @vitest-environment jsdom
//
// Ficheiro á parte de blocks-editor.test.js (que muda moito estes días,
// ver "Ver código" alí) só para os botóns "↶ Desfacer"/"↷ Refacer" novos
// (undo()/redo() en blocks-editor.js) - mesmos stubs de xeometría SVG que
// precisa calquera test que inxecte Blockly de verdade en jsdom.
//
// Proba SÓ a PLOMBAXE (undo()/redo() chaman workspace.undo(false/true), ou
// codeView.api.undo()/redo() se hai unha vista "Ver código" aberta) - o
// funcionamento en si do undo/redo de Blockly xa é responsabilidade da
// propia librería (Blockly.WorkspaceSvg.undo), non hai que volver probalo
// aquí; reproducir a semántica exacta de agrupamento de eventos de Blockly
// nun test illado sería froxo e non aportaría nada novo.
import { describe, it, expect, vi, beforeAll, afterEach } from 'vitest'

let createBlocksEditor

beforeAll(async () => {
  SVGElement.prototype.getBBox = () => ({ x: 0, y: 0, width: 100, height: 20 })
  SVGElement.prototype.getComputedTextLength = () => 50
  SVGElement.prototype.getScreenCTM = () => ({ a: 1, b: 0, c: 0, d: 1, e: 0, f: 0 })
  SVGElement.prototype.createSVGPoint = () => ({ x: 0, y: 0, matrixTransform: () => ({ x: 0, y: 0 }) })
  global.ResizeObserver = class { observe() {} disconnect() {} }
  ;({ createBlocksEditor } = await import('./blocks-editor.js'))
})

let container
afterEach(() => {
  if (container) container.remove()
  container = null
})

function mount(opts) {
  container = document.createElement('div')
  container.style.height = '400px'
  document.body.appendChild(container)
  return createBlocksEditor(container, opts)
}

describe('createBlocksEditor — undo()/redo() (botóns ↶/↷, ver main.js)', () => {
  it('undo()/redo() chaman workspace.undo(false)/workspace.undo(true)', () => {
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n' })
    const workspace = editor._workspaceForTests
    const spy = vi.spyOn(workspace, 'undo')

    editor.undo()
    expect(spy).toHaveBeenLastCalledWith(false)
    editor.redo()
    expect(spy).toHaveBeenLastCalledWith(true)

    editor.destroy()
  })

  it('undo()/redo() sen nada que desfacer/refacer non fan nada nin fallan', () => {
    const editor = mount({ initialDoc: '<EX><EVAL>1+1</EVAL></EX>\n' })
    expect(() => editor.undo()).not.toThrow()
    expect(() => editor.redo()).not.toThrow()
    expect(editor.getValue()).toBe('<EX><EVAL>1+1</EVAL></EX>\n')
    editor.destroy()
  })
})
