// Páxina de proba visual SÓ para desenvolvemento (ver blockly-preview.html
// na raíz de frontend/) - non forma parte da app, non se importa dende
// main.js. Serve para ver o aspecto real (tema + renderer zelos) nun
// navegador antes de conectar todo isto ao editor de verdade.
import * as Blockly from 'blockly/core'
import * as BlocklyMsgGl from 'blockly/msg/gl'
import { registerBlocks } from './blocks.js'
import { buildToolbox } from './toolbox.js'
import { matexeTheme } from './theme.js'
import { docToWorkspace } from './adapter.js'
import { parseDocument } from '../blocks-serialize.js'

Blockly.setLocale(BlocklyMsgGl)
registerBlocks()

const workspace = Blockly.inject('blocklyDiv', {
  toolbox: buildToolbox(),
  theme: matexeTheme,
  renderer: 'zelos',
  trashcan: true,
  zoom: { controls: true, wheel: true },
  grid: { spacing: 20, length: 3, colour: '#eee', snap: true },
})

const SAMPLE = `<EX>Exercicio 1: calcula <EVAL>f(x):=x^2+1; f(3)</EVAL> e representa a función.
<PLOT>plot2d(x^2+1,[x,-3,3])</PLOT></EX>

<EX>Exercicio 2: debuxa un triángulo con TikZ.
<TIKZ>\\draw (0,0) -- (2,0) -- (1,1.5) -- cycle;</TIKZ>
Inclúe tamén unha imaxe: <IMG src="logo.png"></EX>
`

docToWorkspace(parseDocument(SAMPLE), workspace)
Blockly.svgResize(workspace)
