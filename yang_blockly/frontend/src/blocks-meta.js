// Metadatos dos tipos de anaco (texto/fórmula/variables/gráfico/TikZ/imaxe):
// icona, etiqueta, contido por defecto e nota de axuda. Extraído do antigo
// blocks.js (editor Snap!/Scratch feito á man) para que tanto o novo editor
// Blockly (blockly/blocks.js, blockly/toolbox.js) coma a modal de contido
// (blocks-modal.js) compartan a MESMA fonte, en vez de duplicar iconas/
// labels en dous sitios. Non depende de Blockly nin do DOM - lóxica pura.
//
// elementMeta(t) mantense coa mesma sinatura ca antes (t opcional, por
// defecto devolve sempre o fallback) para que blocks-serialize.test.js e
// calquera outro test existente que a importase siga a funcionar sen
// cambios.
export function elementMeta(t) {
  const tr = t || ((clave, fallback, vars) => {
    let val = fallback ?? clave
    if (vars) for (const k in vars) val = val.replaceAll(`{${k}}`, vars[k])
    return val
  })
  return {
    text: {
      label: tr('blocks.element.text.label', 'Enunciado'), ico: '📝',
      def: tr('blocks.element.text.def', 'Escribe aquí o enunciado…'),
    },
    variables: {
      label: tr('blocks.element.variables.label', 'Variable / cálculo'), ico: '🔢',
      def: 'a: 1',
      note: tr('blocks.element.variables.note', 'Execútase en silencio (Maxima), sen amosar nada no exame. Varias expresións sepáranse con ; ou $.'),
    },
    formula: {
      label: tr('blocks.element.formula.label', 'Fórmula'), ico: '🧮',
      def: 'f(3)',
      note: tr('blocks.element.formula.note', 'Maxima avalía a expresión e o resultado aparece no exame.'),
    },
    'image-plot': {
      label: tr('blocks.element.imagePlot.label', 'Gráfico'), ico: '📈',
      def: 'plot2d(f(x),[x,-2,2])',
      note: tr('blocks.element.imagePlot.note', 'Xera unha imaxe co gráfico da función.'),
    },
    'image-tikz': {
      label: tr('blocks.element.imageTikz.label', 'Debuxo TikZ'), ico: '✏️',
      def: '\\draw (0,0) -- (1,1) -- (1,0) -- cycle;',
      note: tr('blocks.element.imageTikz.note', 'Código TikZ: debúxase con LaTeX/pgf, non con Maxima.'),
    },
    'image-upload': {
      label: tr('blocks.element.imageUpload.label', 'Imaxe'), ico: '🖼️',
      def: '',
      note: tr('blocks.element.imageUpload.note', 'Imaxe do disco, inserida no exame.'),
    },
  }
}

// ELEMENT_TYPES: orde estable dos 6 tipos de anaco - usada para xerar os
// bloques Blockly (blockly/blocks.js) e a paleta/toolbox (blockly/toolbox.js)
// sen ter que enumerar os tipos á man en varios sitios.
export const ELEMENT_TYPES = ['text', 'variables', 'formula', 'image-plot', 'image-tikz', 'image-upload']

// Agrupamento temático da paleta/toolbox - mesmos catro grupos ca no antigo
// blocks.js (ELEMENT_GROUPS).
export function elementGroups(t) {
  const tr = t || ((clave, fallback) => fallback ?? clave)
  return [
    { title: tr('blocks.group.text', 'Texto'), types: ['text'] },
    { title: tr('blocks.group.variables', 'Cálculos'), types: ['variables'] },
    { title: tr('blocks.group.math', 'Matemáticas'), types: ['formula', 'image-plot', 'image-tikz'] },
    { title: tr('blocks.group.multimedia', 'Imaxes'), types: ['image-upload'] },
  ]
}

// blockTypeForElement/elementTypeForBlock: tradución entre o `type` de
// elemento usado por blocks-serialize.js ("image-plot", con guión) e o nome
// de tipo de bloque Blockly rexistrado (blockly/blocks.js) - Blockly non
// admite "-" nun nome de tipo de bloque, así que se substitúe por "_".
export function blockTypeForElement(elementType) {
  return 'matexe_' + elementType.replace(/-/g, '_')
}

export function elementTypeForBlock(blockType) {
  const withoutPrefix = blockType.replace(/^matexe_/, '')
  // Só "image_plot"/"image_tikz"/"image_upload" levan "-" no type orixinal;
  // o resto (text/variables/formula) non ten "_" que trocar, así que
  // replaceAll é seguro nos dous sentidos.
  if (withoutPrefix.startsWith('image_')) return withoutPrefix.replace('_', '-')
  return withoutPrefix
}
