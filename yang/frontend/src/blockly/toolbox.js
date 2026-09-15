// Toolbox JSON (paleta categorizada) - substitúe a paleta lateral manual do
// antigo blocks.js. Categorías = mesmos catro grupos temáticos ca antes
// (elementGroups en blocks-meta.js), cada unha coa cor do primeiro tipo que
// contén (así a categoría xa adianta a cor dos bloques que ten dentro).
import { elementGroups, blockTypeForElement } from '../blocks-meta.js'
import { CATEGORY_COLOURS } from './theme.js'

export function buildToolbox(t) {
  return {
    kind: 'categoryToolbox',
    contents: elementGroups(t).map((group, i) => ({
      kind: 'category',
      name: group.title,
      // colour: pinta a franxa/icona de cor a carón do nome da categoría
      // (Blockly.toolbox.ToolboxCategory) coa MESMA cor cós bloques que
      // contén (ver CATEGORY_COLOURS, blockly/theme.js) - así Texto/
      // Variables e cálculos/Matemáticas/Multimedia amosan de vez o seu
      // código de cor, coma en Scratch.
      colour: CATEGORY_COLOURS[i],
      contents: group.types.map((type) => ({
        kind: 'block',
        type: blockTypeForElement(type),
      })),
    })),
  }
}
