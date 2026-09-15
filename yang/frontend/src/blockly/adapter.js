// Ponte doc <-> workspace Blockly. "doc" é a mesma estrutura que xa produce/
// consome blocks-serialize.js ({ exercises: [{ elements: [...] }] }) - o
// texto .matex segue sendo a única fonte de verdade (ver plan): este
// adaptador nunca fala de texto directamente, só de doc <-> bloques. O
// límite texto<->doc (parseDocument/documentToText) non se toca.
import { makeElement, makeExercise } from '../blocks-serialize.js'
import { blockTypeForElement, elementTypeForBlock } from '../blocks-meta.js'

export const EXERCICIO_TYPE = 'matexe_exercicio'
const EXERCISE_SPACING_Y = 140 // separación vertical inicial entre exercicios, só estética

// buildExerciseBlock crea UN bloque matexe_exercicio con `elements`
// (array de {type, content}) conectados en pila no seu input ANACOS, sen
// posicionalo (quen chama decide onde - ver docToWorkspace e
// blocks-editor.js appendExercise). Extraído á parte porque tanto a carga
// completa dun documento (docToWorkspace) coma engadir UN exercicio solto
// (IA de exercicio/exame) precisan exactamente esta mesma construción.
export function buildExerciseBlock(workspace, elements) {
  const exBlock = workspace.newBlock(EXERCICIO_TYPE)
  exBlock.initSvg?.()

  const anacosInput = exBlock.getInput('ANACOS')
  let previous = null
  for (const element of elements) {
    const elBlock = workspace.newBlock(blockTypeForElement(element.type))
    elBlock.initSvg?.()
    elBlock.setFieldValue(element.content ?? '', 'CONTENT')
    if (previous) {
      previous.nextConnection.connect(elBlock.previousConnection)
    } else {
      anacosInput.connection.connect(elBlock.previousConnection)
    }
    previous = elBlock
  }
  exBlock.render?.()
  return exBlock
}

// docToWorkspace baleira o workspace e reconstrúe un bloque matexe_exercicio
// por exercicio (sen conectalos entre si - cada un é un "script"
// independente, ver blockly/blocks.js) e, dentro de cada un, unha pila de
// bloques-anaco conectados no seu input ANACOS, na mesma orde que en doc.
export function docToWorkspace(doc, workspace) {
  workspace.clear()

  doc.exercises.forEach((exercise, idx) => {
    const exBlock = buildExerciseBlock(workspace, exercise.elements)
    if (typeof exBlock.moveBy === 'function') {
      exBlock.moveBy(0, idx * EXERCISE_SPACING_Y)
    }
  })
}

// workspaceToDoc percorre os bloques de nivel superior ORDENADOS pola súa
// posición (getTopBlocks(true) - de arriba a abaixo, que é como o
// profesorado os reordena arrastrando) e, para cada exercicio, a súa pila de
// anacos encadeados. Ids non se preservan a propósito (makeElement/
// makeExercise xéranos de novo): non fan falta para o roundtrip a texto
// (documentToText nunca os usa), e este adaptador sempre reconstrúe o doc
// enteiro dende cero, non fai un diff incremental.
//
// legacy (segundo parámetro): workspaceToDoc é unha función pura, sen
// memoria do que pasou antes - non pode saber por si mesma se o
// profesorado tocou a ESTRUTURA (engadir/quitar/reordenar bloques) ou só
// editou texto dentro dun anaco xa existente, distinción da que depende
// documentToText para decidir se envolve en <EX> ou non (ver
// blocks-serialize.js: "Editar texto *dentro* dun anaco non cambia
// legacy"). Por iso quen chama (o editor, non este adaptador) é quen leva
// esa conta e pasa o valor correcto - por defecto false (xa non-legacy),
// que é o seguro para calquera chamador que aínda non a rastrexe.
export function workspaceToDoc(workspace, { legacy = false } = {}) {
  const exercises = workspace
    .getTopBlocks(true)
    .filter((block) => block.type === EXERCICIO_TYPE && block.isEnabled())
    .map((exBlock) => {
      const elements = []
      let anacoBlock = exBlock.getInputTargetBlock('ANACOS')
      while (anacoBlock) {
        // isEnabled(): o menú contextual estándar de Blockly (ver
        // blocks-editor.js, registerDefaultOptions()) trae "Deshabilitar
        // bloque" de balde - sen este filtro, un bloque "deshabilitado"
        // sairía igual no texto xerado, o cal contradí o que calquera
        // usuario de Blockly espera dese menú.
        if (anacoBlock.isEnabled()) {
          elements.push(makeElement(elementTypeForBlock(anacoBlock.type), anacoBlock.getFieldValue('CONTENT') ?? ''))
        }
        anacoBlock = anacoBlock.getNextBlock()
      }
      return makeExercise(elements)
    })
  return { exercises, legacy }
}
