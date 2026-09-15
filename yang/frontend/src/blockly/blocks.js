// Definicións de bloques Blockly para a linguaxe Matexe: un bloque
// contedor (matexe_exercicio, un exercicio enteiro) e un bloque por tipo de
// anaco (matexe_text/variables/formula/image_plot/image_tikz/image_upload).
// Non hai xerador de código (Blockly.Generator) - a tradución bloque<->texto
// .matex faise en blockly/adapter.js + blocks-serialize.js xa existente (ver
// o plan: a estrutura é un contedor plano con fillos tipados, non expresións
// aniñadas, así que un percorrido directo abonda e evita duplicar lóxica).
// Ver blockly/content-field.js para o campo custom que abre a modal.
//
// Contido: cada bloque-anaco leva o seu contido nun ContentField chamado
// CONTENT - garda o valor coma calquera campo (getFieldValue/setFieldValue,
// é o que usa adapter.js), pero premelo abre a modal grande xa existente
// (textarea, autopechado, IA, asistente - ver blocks-modal.js) en vez de
// editar inline: o contido adoita ser demasiado longo/complexo para caber
// nun bloque.
import * as Blockly from 'blockly/core'
import { ContentField } from './content-field.js'
import { elementMeta, ELEMENT_TYPES, blockTypeForElement } from '../blocks-meta.js'

// ANACO_CHECK: tipo de conexión compartido por todos os bloques-anaco e
// polo input ANACOS do bloque exercicio - así só un anaco pode encaixar
// dentro dun exercicio (non outro exercicio, nin un anaco solto no
// workspace conectándose a outra cousa).
const ANACO_CHECK = 'anaco'

// Nome do blockStyle (definido en blockly/theme.js, non aquí) para cada
// bloque - así a cor en si vive nun sitio só e retocar o tema non implica
// tocar as definicións de bloque.
const EXERCICIO_STYLE = 'matexe_exercicio_style'
function styleForElement(type) {
  return 'matexe_' + type.replace(/-/g, '_') + '_style'
}

let registered = false

// registerBlocks é idempotente (chamable varias veces, p.ex. en tests que
// creen workspaces independentes) - Blockly.Blocks é un rexistro global por
// módulo, non por workspace.
export function registerBlocks(t) {
  if (registered) return
  registered = true

  const meta = elementMeta(t)

  Blockly.Blocks['matexe_exercicio'] = {
    init() {
      this.appendDummyInput()
        .appendField('📋')
        .appendField(Blockly.Msg['MATEXE_EXERCICIO'] || 'Exercicio')
      this.appendStatementInput('ANACOS').setCheck(ANACO_CHECK)
      this.setStyle(EXERCICIO_STYLE)
      this.setDeletable(true)
      this.setMovable(true)
      // Sen previousStatement/nextStatement a propósito: cada exercicio é
      // un "script" independente no workspace (coma en Scratch), non se
      // encadea con outro exercicio - reordénanse arrastrando a súa
      // posición vertical (ver blockly/adapter.js workspaceToDoc, que le
      // getTopBlocks(true) - ordenados por posición).
    },
  }

  for (const type of ELEMENT_TYPES) {
    const info = meta[type]
    Blockly.Blocks[blockTypeForElement(type)] = {
      init() {
        this.appendDummyInput()
          .appendField(info.ico)
          .appendField(info.label)
        this.appendDummyInput('CONTENT_ROW')
          .appendField(new ContentField(info.def || ''), 'CONTENT')
        this.setPreviousStatement(true, ANACO_CHECK)
        this.setNextStatement(true, ANACO_CHECK)
        this.setStyle(styleForElement(type))
        if (info.note) this.setTooltip(info.note)
      },
    }
  }
}
