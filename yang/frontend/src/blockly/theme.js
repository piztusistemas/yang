// Tema visual Matexe: cores por categoría de anaco (mesmo agrupamento que a
// paleta, ver blocks-meta.js) + fondo/scrollbar. As cores en si van aquí
// (non en blocks.js, que só referencia os NOMES de estilo) para que
// retocar o aspecto sexa cambiar un sitio só. O aspecto "redondeado"/lúdico
// en si (forma dos bloques, non a cor) vén do renderer 'zelos' (ver
// blocks-editor.js, Blockly.inject({ renderer: 'zelos', theme: matexeTheme })).
//
// Paleta aliñada co rediseño "SaaS moderno" de Yang (canvas "Yang SaaS
// Redesign", aprobado polo usuario) - violeta/amber/emerald/sky, mesma
// familia de ton cós tokens oklch() de style.css/blocks.css. Van en HEX
// (non oklch()) porque Blockly.utils.colour (usado internamente para
// contraste/hover) non entende oklch(); son o equivalente visual das
// variables --primary/--amber/etc., non unha copia literal.
//   Texto                 -> LOOKS      (violeta) - "di/pensa", coma un enunciado
//   Variables e cálculos  -> VARIABLES  (amber)
//   Matemáticas           -> OPERATORS  (emerald) - operadores/expresións
//   Multimedia (imaxe)    -> SENSING    (sky)
//   Exercicio (contedor)  -> EVENTS     (violeta escuro) - inicia un "script"
import * as Blockly from 'blockly/core'

const LOOKS = { primary: '#8B6EF0', secondary: '#7857DB', tertiary: '#6544BE' }
const VARIABLES = { primary: '#F2A93E', secondary: '#DB8F1F', tertiary: '#B8740D' }
const OPERATORS = { primary: '#2FB673', secondary: '#1F9A5D', tertiary: '#187E4B' }
const SENSING = { primary: '#4FA3D1', secondary: '#3E8DBC', tertiary: '#327194' }
const EVENTS = { primary: '#7458E8', secondary: '#6144CE', tertiary: '#4F35AE' }

// CATEGORY_COLOURS: unha cor por grupo temático da paleta/toolbox, MESMA
// orde ca elementGroups() en blocks-meta.js (Texto/Variables e cálculos/
// Matemáticas/Multimedia) - blockly/toolbox.js reutilíza este array para
// pintar a franxa de cor de cada categoría, así que retocar unha paleta
// retoca a outra á vez.
export const CATEGORY_COLOURS = [LOOKS.primary, VARIABLES.primary, OPERATORS.primary, SENSING.primary]

export const matexeTheme = Blockly.Theme.defineTheme('matexe', {
  base: Blockly.Themes.Zelos,
  blockStyles: {
    matexe_exercicio_style: {
      colourPrimary: EVENTS.primary,
      colourSecondary: EVENTS.secondary,
      colourTertiary: EVENTS.tertiary,
    },
    matexe_text_style: {
      colourPrimary: LOOKS.primary,
      colourSecondary: LOOKS.secondary,
      colourTertiary: LOOKS.tertiary,
    },
    matexe_variables_style: {
      colourPrimary: VARIABLES.primary,
      colourSecondary: VARIABLES.secondary,
      colourTertiary: VARIABLES.tertiary,
    },
    matexe_formula_style: {
      colourPrimary: OPERATORS.primary,
      colourSecondary: OPERATORS.secondary,
      colourTertiary: OPERATORS.tertiary,
    },
    matexe_image_plot_style: {
      colourPrimary: OPERATORS.primary,
      colourSecondary: OPERATORS.secondary,
      colourTertiary: OPERATORS.tertiary,
    },
    matexe_image_tikz_style: {
      colourPrimary: OPERATORS.primary,
      colourSecondary: OPERATORS.secondary,
      colourTertiary: OPERATORS.tertiary,
    },
    matexe_image_upload_style: {
      colourPrimary: SENSING.primary,
      colourSecondary: SENSING.secondary,
      colourTertiary: SENSING.tertiary,
    },
  },
  componentStyles: {
    // Equivalentes hex de --surface-sunken/--surface/--text-dark/
    // --border-strong (style.css) - fondo frío neutro, non cálido coma antes.
    workspaceBackgroundColour: '#f5f6f8',
    toolboxBackgroundColour: '#ffffff',
    toolboxForegroundColour: '#2b2d33',
    flyoutBackgroundColour: '#f7f8f9',
    flyoutForegroundColour: '#2b2d33',
    flyoutOpacity: 1,
    scrollbarColour: '#cdd0d6',
    // Equivalente hex de --primary (violeta-indigo) para o marcador de
    // inserción e o cursor - mesmo acento ca no resto da app.
    insertionMarkerColour: '#5B4FE0',
    insertionMarkerOpacity: 0.4,
    cursorColour: '#5B4FE0',
  },
})
