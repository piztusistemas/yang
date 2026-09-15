import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import {
  parseDocument,
  documentToText,
  parseElements,
  elementsToText,
  makeExercise,
  makeElement,
} from './blocks-serialize.js'

const testdataDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'testdata')

function readTestdata(name) {
  return readFileSync(join(testdataDir, name), 'utf8')
}

describe('parseDocument / documentToText — documentos legacy (sen <EX>)', () => {
  for (const name of ['mini.matex', 'test.matex', 'modelo.examen.matex']) {
    it(`reproduce ${name} byte a byte`, () => {
      const original = readTestdata(name)
      const doc = parseDocument(original)
      expect(doc.legacy).toBe(true)
      expect(doc.exercises).toHaveLength(1)
      expect(documentToText(doc)).toBe(original)
    })
  }

  it('documento baleiro produce lista de exercicios baleira', () => {
    const doc = parseDocument('')
    expect(doc).toEqual({ exercises: [], legacy: false })
    expect(documentToText(doc)).toBe('')
  })

  it('texto solto sen <EX> vira un só exercicio cun anaco text opaco', () => {
    const doc = parseDocument('<h1>Ola</h1><EVAL>1+1</EVAL>')
    expect(doc.legacy).toBe(true)
    expect(doc.exercises).toHaveLength(1)
    expect(doc.exercises[0].elements).toEqual([
      { id: doc.exercises[0].elements[0].id, type: 'text', content: '<h1>Ola</h1><EVAL>1+1</EVAL>' },
    ])
  })
})

describe('parseDocument — documentos co formato <EX>', () => {
  it('recoñece varios exercicios en orde', () => {
    const text = '<EX><EVAL>1+1</EVAL></EX>\n\n<EX><PLOT>plot2d(x,[x,-1,1])</PLOT></EX>\n'
    const doc = parseDocument(text)
    expect(doc.legacy).toBe(false)
    expect(doc.exercises).toHaveLength(2)
    expect(doc.exercises[0].elements).toMatchObject([{ type: 'formula', content: '1+1' }])
    expect(doc.exercises[1].elements).toMatchObject([{ type: 'image-plot', content: 'plot2d(x,[x,-1,1])' }])
  })

  it('contido non-branco fóra de <EX> fai caer a fallback legacy', () => {
    const text = '<EX><EVAL>1+1</EVAL></EX>\nsolto\n<EX><EVAL>2+2</EVAL></EX>'
    const doc = parseDocument(text)
    expect(doc.legacy).toBe(true)
    expect(doc.exercises).toHaveLength(1)
    expect(doc.exercises[0].elements[0].type).toBe('text')
  })

  it('roundtrip é idempotente para un documento construído a man', () => {
    const doc = {
      legacy: false,
      exercises: [
        makeExercise([makeElement('text', 'Calcula: '), makeElement('formula', 'f(3)')]),
        makeExercise([makeElement('image-tikz', '\\draw (0,0) -- (1,1);')]),
      ],
    }
    const text = documentToText(doc)
    const reparsed = parseDocument(text)
    expect(documentToText(reparsed)).toBe(text)
    expect(reparsed.exercises).toHaveLength(2)
  })
})

describe('parseElements / elementsToText', () => {
  it('mapea cada tipo á súa etiqueta', () => {
    const text = 'Enunciado: <EVAL>f(3)</EVAL> <PLOT>plot2d(x,[x,-1,1])</PLOT> <TIKZ>\\draw (0,0) circle (1);</TIKZ> <IMG src="logo.png">'
    const elements = parseElements(text)
    expect(elements.map((e) => e.type)).toEqual(['text', 'formula', 'text', 'image-plot', 'text', 'image-tikz', 'text', 'image-upload'])
    expect(elements.find((e) => e.type === 'image-upload').content).toBe('logo.png')
    expect(elementsToText(elements)).toBe(text)
  })

  it('etiquetas adxacentes non xeran anaco text baleiro entre elas', () => {
    const elements = parseElements('<EVAL>a</EVAL><PLOT>b</PLOT>')
    expect(elements.map((e) => e.type)).toEqual(['formula', 'image-plot'])
  })

  it('etiquetas vellas MAT/TEX viran texto opaco, non fórmula', () => {
    const text = '<MAT>a</MAT><TEX>\\alpha</TEX>'
    const elements = parseElements(text)
    expect(elements).toHaveLength(1)
    expect(elements[0].type).toBe('text')
    expect(elements[0].content).toBe(text)
  })

  it('HIDE mapea a variables (grupo "Variables e cálculos")', () => {
    const text = 'Antes <HIDE>a:1; b:2</HIDE> despois'
    const elements = parseElements(text)
    expect(elements.map((e) => e.type)).toEqual(['text', 'variables', 'text'])
    expect(elements[1].content).toBe('a:1; b:2')
    expect(elementsToText(elements)).toBe(text)
  })

  it('texto sen ningunha etiqueta produce un só anaco text', () => {
    const elements = parseElements('<h1>Ola</h1><p>sen tags</p>')
    expect(elements).toEqual([{ id: elements[0].id, type: 'text', content: '<h1>Ola</h1><p>sen tags</p>' }])
  })
})
