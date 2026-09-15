// Menú contextual custom: "💾 Gardar na biblioteca", para o bloque
// matexe_exercicio e para calquera bloque-anaco - equivalente aos botóns
// "💾" do editor antigo (só visibles se main.js inxectou saveToLibrary, ver
// blocks.js orixinal). O resto do menú (Duplicar/Eliminar/Deshabilitar...)
// xa o dá Blockly.ContextMenuItems.registerDefaultOptions() de balde (ver
// blocks-editor.js) - non hai que reimplementalo.
//
// Rexistro GLOBAL (coma registerBlocks) pero resolve t/saveToLibrary por
// EDITOR en tempo de clic, vía editor-registry.js (mesmo motivo ca
// content-field.js: un bloque só sabe o seu workspace).
import * as Blockly from 'blockly/core'
import { elementMeta, elementTypeForBlock } from '../blocks-meta.js'
import { elementsToText } from '../blocks-serialize.js'
import { getEditor } from './editor-registry.js'
import { openSaveLibraryModal } from '../blocks-ai-library-modals.js'

const EXERCICIO_TYPE = 'matexe_exercicio'

let registered = false

function readAnacoElement(block) {
  return { type: elementTypeForBlock(block.type), content: block.getFieldValue('CONTENT') ?? '' }
}

export function registerContextMenuItems() {
  if (registered) return
  registered = true

  Blockly.ContextMenuRegistry.registry.register({
    id: 'matexeSaveExerciseToLibrary',
    scopeType: Blockly.ContextMenuRegistry.ScopeType.BLOCK,
    weight: 100,
    preconditionFn: (scope) => {
      const block = scope.block
      if (!block || block.type !== EXERCICIO_TYPE) return 'hidden'
      const api = getEditor(block.workspace)
      return api && typeof api.saveToLibrary === 'function' ? 'enabled' : 'hidden'
    },
    displayText: (scope) => (getEditor(scope.block.workspace)?.t || ((k, f) => f))('blocks.exercise.saveToLibrary', 'Gardar este exercicio na biblioteca'),
    callback: (scope) => {
      const block = scope.block
      const api = getEditor(block.workspace)
      if (!api) return
      const elements = []
      let anacoBlock = block.getInputTargetBlock('ANACOS')
      while (anacoBlock) {
        if (anacoBlock.isEnabled()) elements.push(readAnacoElement(anacoBlock))
        anacoBlock = anacoBlock.getNextBlock()
      }
      openSaveLibraryModal({ t: api.t, tipo: 'exercicio', contido: elementsToText(elements), saveToLibrary: api.saveToLibrary })
    },
  })

  // Botón dereito no lenzo (zona baleira, non enriba dun bloque) -> "📄 Ver
  // código": substitúe o lenzo polo texto .matex do exercicio ACTIVO (ver
  // openExerciseCodeView/onViewExerciseCode, blocks-editor.js). O equivalente
  // para o exame ENTEIRO vive no botón "🗂️ Exercicios" da toolbar (menú
  // propio, non Blockly - ver showSimpleMenu/openExamCodeView).
  Blockly.ContextMenuRegistry.registry.register({
    id: 'matexeViewExerciseCode',
    scopeType: Blockly.ContextMenuRegistry.ScopeType.WORKSPACE,
    weight: 200,
    preconditionFn: (scope) => {
      const api = getEditor(scope.workspace)
      if (!api || typeof api.onViewExerciseCode !== 'function') return 'hidden'
      const canView = typeof api.canViewExerciseCode === 'function' ? api.canViewExerciseCode() : true
      return canView ? 'enabled' : 'hidden'
    },
    displayText: (scope) => (getEditor(scope.workspace)?.t || ((k, f) => f))('blocks.codeView.menuItem', '📄 Ver código'),
    callback: (scope) => { getEditor(scope.workspace)?.onViewExerciseCode?.() },
  })

  Blockly.ContextMenuRegistry.registry.register({
    id: 'matexeSaveBlockToLibrary',
    scopeType: Blockly.ContextMenuRegistry.ScopeType.BLOCK,
    weight: 100,
    preconditionFn: (scope) => {
      const block = scope.block
      if (!block || !elementMeta()[elementTypeForBlock(block.type)]) return 'hidden'
      const api = getEditor(block.workspace)
      return api && typeof api.saveToLibrary === 'function' ? 'enabled' : 'hidden'
    },
    displayText: (scope) => (getEditor(scope.block.workspace)?.t || ((k, f) => f))('blocks.element.saveToLibrary', 'Gardar este bloque na biblioteca'),
    callback: (scope) => {
      const block = scope.block
      const api = getEditor(block.workspace)
      if (!api) return
      openSaveLibraryModal({ t: api.t, tipo: 'bloque', contido: elementsToText([readAnacoElement(block)]), saveToLibrary: api.saveToLibrary })
    },
  })
}
