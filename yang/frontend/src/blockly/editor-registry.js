// Rexistro workspace -> API do editor que o creou.
//
// Por que fai falla: Blockly.Blocks é un rexistro GLOBAL por módulo (ver
// blocks.js, registerBlocks() só executa unha vez - "idempotente"), pero
// createBlocksEditor pode instanciarse varias veces (reabrir o editor,
// tests...), cada vez cun workspace novo e o seu propio uploadImage/
// generateWithAI/abrirAsistente inxectados por main.js. Un campo (ver
// content-field.js) só sabe o seu bloque e o workspace dese bloque - non
// pode "capturar" en pechadura o callback do editor concreto que o creou,
// porque init() do tipo de bloque execútase unha soa vez para TODOS os
// workspaces. Este rexistro (workspace.id -> API) é a indirección que
// resolve iso: cada createBlocksEditor rexístrase ao nacer e dá de baixa ao
// destruírse (destroy()).
const editors = new Map()

export function registerEditor(workspace, api) {
  editors.set(workspace.id, api)
}

export function unregisterEditor(workspace) {
  editors.delete(workspace.id)
}

export function getEditor(workspace) {
  return editors.get(workspace.id)
}
