// CodeMirror 6 editor: HTML syntax highlighting (the .matex format is HTML
// with embedded Matexe tags) plus a lightweight decorator that colour-codes
// the Matexe-specific tags themselves, echoing the colour scheme cas.pas
// used in its debug log (process_MAT/EVAL/HIDE -> red/aqua/gray).
import { EditorView, keymap, Decoration, ViewPlugin } from '@codemirror/view'
import { EditorState, RangeSetBuilder } from '@codemirror/state'
import { html } from '@codemirror/lang-html'
import { defaultKeymap, history, historyKeymap, indentWithTab, undo as cmUndo, redo as cmRedo } from '@codemirror/commands'
import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete'
import { syntaxHighlighting, defaultHighlightStyle } from '@codemirror/language'
import { oneDark } from '@codemirror/theme-one-dark'

const TAG_RE = /<(MAT|EVAL|HIDE|PLOT|TEX|FILE|COND|RESP|SOL)>[\s\S]*?<\/\1>/g

const tagMark = {
  MAT: Decoration.mark({ class: 'cm-matexe-mat' }),
  EVAL: Decoration.mark({ class: 'cm-matexe-eval' }),
  HIDE: Decoration.mark({ class: 'cm-matexe-hide' }),
  PLOT: Decoration.mark({ class: 'cm-matexe-plot' }),
  TEX: Decoration.mark({ class: 'cm-matexe-tex' }),
  FILE: Decoration.mark({ class: 'cm-matexe-file' }),
  COND: Decoration.mark({ class: 'cm-matexe-cond' }),
  // RESP/SOL: etiquetas de corrección da interface de táboa (resposta final
  // / solución). Reúsase a cor de HIDE (gris) - non se avalían nin se amosan
  // agás na lapela correspondente.
  RESP: Decoration.mark({ class: 'cm-matexe-hide' }),
  SOL: Decoration.mark({ class: 'cm-matexe-hide' }),
}

function buildTagDecorations(doc) {
  const text = doc.toString()
  const builder = new RangeSetBuilder()
  for (const m of text.matchAll(TAG_RE)) {
    builder.add(m.index, m.index + m[0].length, tagMark[m[1]])
  }
  return builder.finish()
}

const matexeTagHighlighter = ViewPlugin.fromClass(
  class {
    constructor(view) {
      this.decorations = buildTagDecorations(view.state.doc)
    }
    update(update) {
      if (update.docChanged) this.decorations = buildTagDecorations(update.state.doc)
    }
  },
  { decorations: (v) => v.decorations }
)

const matexeTheme = EditorView.theme({
  '&': { height: '100%', fontSize: '0.9rem' },
  '.cm-scroller': { fontFamily: '"Fira Code", Consolas, monospace', overflow: 'auto' },
  '.cm-matexe-mat': { backgroundColor: 'rgba(229, 83, 75, 0.28)' },
  '.cm-matexe-eval': { backgroundColor: 'rgba(80, 190, 210, 0.25)' },
  '.cm-matexe-hide': { backgroundColor: 'rgba(140, 140, 140, 0.25)' },
  '.cm-matexe-plot': { backgroundColor: 'rgba(90, 200, 120, 0.25)' },
  '.cm-matexe-tex': { backgroundColor: 'rgba(180, 130, 230, 0.25)' },
  '.cm-matexe-file': { backgroundColor: 'rgba(230, 180, 80, 0.25)' },
  '.cm-matexe-cond': { backgroundColor: 'rgba(230, 130, 180, 0.25)' },
})

function wrapTagCommand(tag) {
  return (view) => {
    const range = view.state.selection.main
    const selected = view.state.sliceDoc(range.from, range.to)
    const insert = `<${tag}>${selected}</${tag}>`
    const cursor = range.from + tag.length + 2
    view.dispatch({
      changes: { from: range.from, to: range.to, insert },
      selection: { anchor: cursor, head: cursor + selected.length },
    })
    view.focus()
    return true
  }
}

// createEditor mounts a CodeMirror instance into `container` and returns a
// small API matching what main.js needs (get/set the whole document), plus
// wires the Matexe tag shortcuts (Ctrl+M/E/H/P/T/F). App-level shortcuts
// (Ctrl+S/O/Enter) are handled by main.js's window-level keydown listener -
// keydown bubbles up from the editor's contenteditable DOM same as from any
// other element, so they don't need to be duplicated here.
export function createEditor(container, { initialDoc, onChange }) {
  // zoom: Ctrl+/Ctrl-/Ctrl+0 sobre CALQUERA CodeMirror creado con esta
  // función (editor de Código principal, "código de fondo" do modo bloques
  // desacoplado, e as ventás "Ver código" do editor de bloques, ver
  // blocks-editor.js) - mesmos incrementos/límites có zoom das páxinas de
  // previsualización (zoomScript, main.js), por consistencia. Estado por
  // instancia (non global): cada CodeMirror garda o seu propio nivel.
  // Aplícase directamente coma estilo inline sobre view.dom (o elemento
  // raíz .cm-editor) - gaña por especificidade á regra `&{fontSize:...}` do
  // tema (matexeTheme, abaixo), inxectada coma clase normal.
  let zoom = 1
  const BASE_FONT_REM = 0.9
  function applyZoom() {
    view.dom.style.fontSize = (BASE_FONT_REM * zoom) + 'rem'
  }
  const zoomHandler = EditorView.domEventHandlers({
    keydown: (event) => {
      if (!(event.ctrlKey || event.metaKey)) return false
      if (event.key === '+' || event.key === '=') zoom = Math.min(zoom * 1.2, 4)
      else if (event.key === '-' || event.key === '_') zoom = Math.max(zoom / 1.2, 0.3)
      else if (event.key === '0') zoom = 1
      else return false
      applyZoom()
      event.preventDefault()
      return true
    },
  })

  const tagKeymap = [
    { key: 'Mod-m', run: wrapTagCommand('MAT'), preventDefault: true },
    { key: 'Mod-e', run: wrapTagCommand('EVAL'), preventDefault: true },
    { key: 'Mod-h', run: wrapTagCommand('HIDE'), preventDefault: true },
    { key: 'Mod-p', run: wrapTagCommand('PLOT'), preventDefault: true },
    { key: 'Mod-t', run: wrapTagCommand('TEX'), preventDefault: true },
    { key: 'Mod-f', run: wrapTagCommand('FILE'), preventDefault: true },
  ]

  // onChangeExtension: só se engade se main.js pasa un `onChange` - usado
  // para o panel de código de fondo do modo dúas xanelas (mostrarCodigoDeFondo)
  // para saber cando o profesorado escribiu aí e volcalo aos bloques
  // (blocksEditor.setValue) sen ter que facer polling continuo.
  const onChangeExtension = onChange
    ? EditorView.updateListener.of((update) => {
        if (update.docChanged) onChange()
      })
    : []

  const state = EditorState.create({
    doc: initialDoc,
    extensions: [
      onChangeExtension,
      history(),
      html(),
      // Autopecha (), "", '', [], {} ao escribir o carácter de apertura -
      // "sen ter que escribir as aspas" a man, pedido explícito do usuario.
      closeBrackets(),
      syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
      matexeTagHighlighter,
      matexeTheme,
      oneDark,
      keymap.of([...tagKeymap, ...closeBracketsKeymap, indentWithTab, ...defaultKeymap, ...historyKeymap]),
      EditorView.lineWrapping,
      zoomHandler,
    ],
  })

  const view = new EditorView({ state, parent: container })

  return {
    view,
    getValue: () => view.state.doc.toString(),
    setValue: (text) => {
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } })
    },
    focus: () => view.focus(),
    // undo/redo: mesmos comandos ca historyKeymap (Ctrl+Z/Ctrl+Maiús+Z, xa
    // funcionais dende o teclado) mais chamables dende un botón visible
    // (ver #btnUndo/#btnRedo, main.js) - un clic non pasa polo keymap, así
    // que non hai risco de desfacer dúas veces. Seguros de chamar sen nada
    // que desfacer/refacer (non fan nada, cmUndo/cmRedo devolven false).
    undo: () => cmUndo(view),
    redo: () => cmRedo(view),
    // wrapTag: mesma acción cós atallos Ctrl+M/E/H/P/T (wrapTagCommand
    // arriba), pero chamable dende un botón visible - ver #matexeTagBar en
    // main.js. Acepta calquera etiqueta, non só as que teñen atallo de
    // teclado (usado tamén para TIKZ, sen atallo propio).
    wrapTag: (tag) => wrapTagCommand(tag)(view),
    // selectRange: selecciona [from, to) e desprázase para que quede
    // visible - usado polo "Ctrl+clic reverse search" (main.js) para
    // resaltar a etiqueta que xerou o resultado premido na previsualización.
    selectRange: (from, to) => {
      view.dispatch({ selection: { anchor: from, head: to }, scrollIntoView: true })
      view.focus()
    },
    // getSelectionOrAll/replaceRange: usados polo asistente de IA (icona 🤖,
    // ver assistant.js) para operar sobre a SELECCIÓN se hai unha (edición
    // máis cirúrxica dun anaco), ou sobre o documento enteiro se non a hai.
    // {from, to} devólvese xa fixado no momento da consulta (getSelection-
    // OrAll) para que replaceRange aplique sempre ao mesmo treito, aínda
    // que o cursor cambiase mentres se conversaba co asistente.
    getSelectionOrAll: () => {
      const { from, to } = view.state.selection.main
      if (from === to) return { text: view.state.doc.toString(), from: 0, to: view.state.doc.length }
      return { text: view.state.sliceDoc(from, to), from, to }
    },
    replaceRange: (from, to, text) => {
      view.dispatch({ changes: { from, to, insert: text } })
      view.focus()
    },
  }
}
