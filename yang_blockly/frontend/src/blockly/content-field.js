// ContentField: campo Blockly que garda o contido dun anaco (texto/fórmula/
// variables/...) coma valor de string normal (getFieldValue/setFieldValue
// funcionan igual ca calquera campo - ver adapter.js, que non sabe nada
// deste campo en particular), pero NON se edita inline: ao premelo abre a
// MESMA modal grande que o editor antigo (textarea, autopechado, IA,
// asistente - ver blocks-modal.js), porque o contido adoita ser demasiado
// longo/complexo para un campo dun bloque. No bloque amósase só un resumo
// truncado (getText_).
import * as Blockly from 'blockly/core'
import { getEditor } from './editor-registry.js'

const PREVIEW_MAX = 40

export class ContentField extends Blockly.Field {
  constructor(value, config) {
    super(value ?? '', null, config)
    this.SERIALIZABLE = true
  }

  static fromJson(options) {
    return new ContentField(options['text'], options)
  }

  // showEditor_ é o punto de extensión estándar de Blockly.Field para "que
  // pasa cando se preme/actívase este campo" (ver core/field.d.ts) - aquí,
  // en vez do editor inline por defecto, delega no editor propietario
  // deste workspace (rexistrado en editor-registry.js por
  // blocks-editor.js), pasándolle o bloque enteiro (non só o campo) porque
  // a modal precisa saber o TIPO de anaco (meta.label/nota/etc.), non só o
  // contido.
  showEditor_() {
    const block = this.getSourceBlock()
    if (!block) return
    const api = getEditor(block.workspace)
    if (api) api.onEditContent(block)
  }

  // getText_ substitúe o texto amosado no bloque (o VALOR real, devolto por
  // getValue()/getFieldValue, non cambia) - unha soa liña, recortada, para
  // que o bloque non medre desmesuradamente con contido longo.
  getText_() {
    const raw = String(this.getValue() ?? '').replace(/\s+/g, ' ').trim()
    if (!raw) return '(baleiro)'
    return raw.length > PREVIEW_MAX ? raw.slice(0, PREVIEW_MAX) + '…' : raw
  }
}

Blockly.fieldRegistry.register('field_matexe_content', ContentField)
