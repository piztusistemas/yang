// Roundtrip doc<->workspace (e, encadeado con blocks-serialize.js xa
// existente, texto<->workspace<->texto) contra un Blockly.Workspace
// HEADLESS (sen WorkspaceSvg, sen renderizado SVG) - evita a fricción
// coñecida de Blockly+jsdom (getBBox/getComputedTextLength ausentes no DOM
// simulado, ver o plan). Non fai falla ningún jsdom aquí: Block.init() só
// declara inputs/fields/connections, non toca SVG quen non ten
// initSvg/render definidos (workspace headless), por iso docToWorkspace usa
// "?." nesas chamadas.
import { describe, it, expect, beforeAll } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import * as Blockly from 'blockly/core'
import { registerBlocks } from './blocks.js'
import { docToWorkspace, workspaceToDoc } from './adapter.js'
import { parseDocument, documentToText, makeElement, makeExercise } from '../blocks-serialize.js'

const testdataDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'testdata')
function readTestdata(name) {
  return readFileSync(join(testdataDir, name), 'utf8')
}

beforeAll(() => {
  registerBlocks()
})

function roundtripText(text) {
  const workspace = new Blockly.Workspace()
  try {
    const original = parseDocument(text)
    docToWorkspace(original, workspace)
    // legacy pasa explícito: workspaceToDoc non o infire (é o editor quen
    // debe rastrexar se houbo cambio estrutural, ver adapter.js) - este
    // test simula "abrir e reler sen tocar nada", así que ten que
    // conservarse tal cal.
    return documentToText(workspaceToDoc(workspace, { legacy: original.legacy }))
  } finally {
    workspace.dispose()
  }
}

describe('docToWorkspace / workspaceToDoc — roundtrip completo vía texto', () => {
  for (const name of ['mini.matex', 'test.matex', 'modelo.examen.matex', 'bloques.matex']) {
    it(`reproduce ${name} byte a byte pasando por un workspace Blockly`, () => {
      const original = readTestdata(name)
      expect(roundtripText(original)).toBe(original)
    })
  }

  it('conserva varios exercicios con varios tipos de anaco, na orde orixinal', () => {
    const doc = {
      legacy: false,
      exercises: [
        makeExercise([
          makeElement('text', 'Enunciado 1'),
          makeElement('formula', 'f(3)'),
          makeElement('variables', 'a: 1'),
        ]),
        makeExercise([
          makeElement('image-plot', 'plot2d(x,[x,-1,1])'),
          makeElement('image-tikz', '\\draw (0,0) -- (1,1);'),
          makeElement('image-upload', 'foto.png'),
        ]),
      ],
    }
    const workspace = new Blockly.Workspace()
    docToWorkspace(doc, workspace)
    const back = workspaceToDoc(workspace)
    workspace.dispose()

    expect(back.exercises).toHaveLength(2)
    expect(back.exercises[0].elements.map((e) => [e.type, e.content])).toEqual([
      ['text', 'Enunciado 1'],
      ['formula', 'f(3)'],
      ['variables', 'a: 1'],
    ])
    expect(back.exercises[1].elements.map((e) => [e.type, e.content])).toEqual([
      ['image-plot', 'plot2d(x,[x,-1,1])'],
      ['image-tikz', '\\draw (0,0) -- (1,1);'],
      ['image-upload', 'foto.png'],
    ])
  })

  it('un exercicio baleiro (sen anacos) non rompe o roundtrip', () => {
    const doc = { legacy: false, exercises: [makeExercise([])] }
    const workspace = new Blockly.Workspace()
    docToWorkspace(doc, workspace)
    const back = workspaceToDoc(workspace)
    workspace.dispose()
    expect(back.exercises).toHaveLength(1)
    expect(back.exercises[0].elements).toHaveLength(0)
  })
})
