// createBlocksEditor - versión Blockly do editor de exames por bloques.
// MESMO contrato ca o antigo blocks.js (getValue/setValue/focus/destroy),
// así que main.js non ten por que cambiar (ver plan): isto é un ficheiro
// NOVO, aínda non conectado dende main.js ata confirmar paridade (Fase 6).
//
// Arquitectura (ver o plan completo): o texto .matex segue sendo a única
// fonte de verdade. O workspace Blockly é unha vista de edición
// transitoria - setValue reconstrúeo dende cero (docToWorkspace),
// getValue lée o estado actual e volve pasalo por documentToText
// (workspaceToDoc + blocks-serialize.js xa existente, sen tocar).
//
// Exercicios coma "obxectos" independentes (coma os sprites de Scratch,
// ver blocks-exercises-panel.js): o workspace Blockly (único) SÓ contén os
// bloques do exercicio ACTIVO (exercisesData[activeIndex]) - o resto dos
// exercicios do exame viven coma datos soltos en `exercisesData` (arrays
// de {elements}, mesma forma cós exercicios de blocks-serialize.js) ata
// que se abren. A ventá "🗂️ Exercicios" (blocks-exercises-panel.js) é a
// única forma de trocar de exercicio activo, engadir un novo, eliminar,
// duplicar ou reordear. getValue() combina sempre o exercicio activo (lido
// EN VIVO do workspace) cos demais (lidos de exercisesData) - así o
// roundtrip a texto .matex non cambia cara afora aínda que só se vexa un
// exercicio de cada vez.
import * as Blockly from 'blockly/core'
import * as BlocklyMsgGl from 'blockly/msg/gl'
import * as BlocklyMsgEs from 'blockly/msg/es'
import * as BlocklyMsgEn from 'blockly/msg/en'
import * as BlocklyMsgPt from 'blockly/msg/pt'
import { registerBlocks } from './blockly/blocks.js'
import { buildToolbox } from './blockly/toolbox.js'
import { matexeTheme } from './blockly/theme.js'
import { buildExerciseBlock, docToWorkspace, workspaceToDoc, EXERCICIO_TYPE } from './blockly/adapter.js'
import { registerEditor, unregisterEditor } from './blockly/editor-registry.js'
import { registerContextMenuItems } from './blockly/context-menu.js'
import { elementMeta, ELEMENT_TYPES, elementTypeForBlock } from './blocks-meta.js'
import { parseDocument, documentToText, parseElements, elementsToText } from './blocks-serialize.js'
import { confirmar } from './dialogs.js'
import { openContentModal } from './blocks-modal.js'
import { openExerciseAIModal, openExamAIModal } from './blocks-ai-library-modals.js'
import { openExercisesPanel, closeExercisesPanel } from './blocks-exercises-panel.js'
import { createEditor } from './editor.js'

// BLOCKLY_MSGS: mensaxes PROPIAS de Blockly (menú contextual nativo
// "Duplicar"/"Eliminar N bloques"/"Deshabilitar", tooltips do zoom e da
// papeleira...). Non veñen dos nosos idiomas/*.json - son as de Blockly, e
// hai que escoller o módulo á man. Ata agora fixábase sempre `gl`, así que
// ese menú quedaba en galego aínda con Yang en castelán/inglés/portugués.
// gl segue a ser o fallback, coma en t() (ver main.js).
const BLOCKLY_MSGS = { gl: BlocklyMsgGl, es: BlocklyMsgEs, en: BlocklyMsgEn, pt: BlocklyMsgPt }

let localeSet = null
function ensureLocale(idioma) {
  const code = BLOCKLY_MSGS[idioma] ? idioma : 'gl'
  if (localeSet === code) return
  localeSet = code
  Blockly.setLocale(BLOCKLY_MSGS[code])
}

let defaultContextMenuRegistered = false
// Blockly.ContextMenuItems.registerDefaultOptions() dá de balde, para
// calquera bloque, "Duplicar"/"Eliminar N bloques"/"Deshabilitar" (+
// "Limpar bloques"/"Eliminar todo" a nivel de workspace) - cobre "Duplicar
// exercicio"/"Eliminar exercicio"/"Eliminar anaco" do editor antigo sen
// código propio. Rexistro global (coma registerBlocks), idempotente coma
// eles - pero a propia Blockly xa rexistra estes atallos por conta propia
// en canto se inxecta un primeiro workspace (non queda claro exactamente
// onde, non está documentado), así que un try/catch cobre tanto "aínda
// non rexistrado" coma "xa rexistrado por Blockly" sen ter que adiviñar a
// orde exacta.
function ensureDefaultContextMenu() {
  if (defaultContextMenuRegistered) return
  defaultContextMenuRegistered = true
  try {
    Blockly.ContextMenuItems.registerDefaultOptions()
  } catch (err) {
    if (!String(err.message).includes('already registered')) throw err
  }
}

// showSimpleMenu: mini menú contextual xenérico para elementos HTML soltos
// que non teñen menú contextual propio de Blockly (ese vive só dentro do SVG
// do lenzo, ver o WORKSPACE-scoped item máis abaixo e blockly/context-menu.js)
// - usado só polo botón "🗂️ Exercicio X de N" (botón dereito -> "Ver
// código", ver openExamCodeView). Un só nivel, sen submenús. Singleton a
// nivel de módulo (coma `panel` en blocks-exercises-panel.js): só ten
// sentido un aberto de cada vez.
let openSimpleMenu = null
function closeSimpleMenu() {
  if (!openSimpleMenu) return
  openSimpleMenu.remove()
  openSimpleMenu = null
  document.removeEventListener('mousedown', handleSimpleMenuOutsideClick, true)
  document.removeEventListener('keydown', handleSimpleMenuEscape, true)
}
function handleSimpleMenuOutsideClick(e) {
  if (openSimpleMenu && !openSimpleMenu.contains(e.target)) closeSimpleMenu()
}
function handleSimpleMenuEscape(e) {
  if (e.key === 'Escape') closeSimpleMenu()
}
function showSimpleMenu(x, y, items) {
  closeSimpleMenu()
  const menu = document.createElement('div')
  menu.className = 'blocks-simple-menu'
  menu.style.left = x + 'px'
  menu.style.top = y + 'px'
  for (const item of items) {
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.className = 'blocks-simple-menu__item'
    btn.innerHTML = item.label
    btn.addEventListener('click', () => { closeSimpleMenu(); item.onClick() })
    menu.appendChild(btn)
  }
  document.body.appendChild(menu)
  openSimpleMenu = menu
  // Diferido (non no mesmo tick): o propio "contextmenu" que abriu este
  // menú tamén dispara un "mousedown" antes en WebKitGTK/Chrome - sen isto
  // pecharíase a si mesmo no intre de abrir.
  setTimeout(() => {
    document.addEventListener('mousedown', handleSimpleMenuOutsideClick, true)
    document.addEventListener('keydown', handleSimpleMenuEscape, true)
  }, 0)
}

export function createBlocksEditor(container, {
  initialDoc, uploadImage, canUploadImage, generateWithAI, generateExerciseWithAI,
  generateExamWithAI, saveToLibrary, listLibrary, deleteFromLibrary, abrirAsistente,
  onToggleFullWidth, onGenerateResult, onDetachExerciseAI, onDetachExamAI,
  xeradorIAAberta, focarXeradorIA, t: tIn, idioma,
} = {}) {
  const t = tIn || ((clave, fallback, vars) => {
    let val = fallback ?? clave
    if (vars) for (const k in vars) val = val.replaceAll(`{${k}}`, vars[k])
    return val
  })

  ensureLocale(idioma)
  registerBlocks(t)
  ensureDefaultContextMenu()
  registerContextMenuItems()

  const ELEMENT_META = elementMeta(t)

  container.innerHTML = ''
  container.classList.add('blocks-root')

  const toolbar = document.createElement('div')
  toolbar.className = 'blocks-toolbar'

  // "🗂️ Exercicio X de N": único punto de entrada á ventá emerxente de
  // exercicios (ver blocks-exercises-panel.js) - substitúe ao vello botón
  // "+ Engadir exercicio" (que engadía un bloque directamente no lenzo
  // compartido; agora só hai un exercicio no lenzo de cada vez, así que
  // engadir tamén pasa pola ventá, coma "engadir sprite" en Scratch).
  const exercisesBtn = document.createElement('button')
  exercisesBtn.type = 'button'
  exercisesBtn.className = 'blocks-toolbar__exercises'
  exercisesBtn.innerHTML = '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="5.5" width="14" height="10" rx="1.6"/><path d="M3 8.2h14"/></svg><span></span>'
  exercisesBtn.addEventListener('click', showExercisesPanel)
  // Botón dereito sobre "🗂️ Exercicio X de N" -> "📄 Ver código": mesma idea
  // ca o menú contextual do propio lenzo (ver embaixo), pero para o exame
  // ENTEIRO (todos os exercicios), non só o activo - ver openExamCodeView.
  exercisesBtn.addEventListener('contextmenu', (e) => {
    e.preventDefault()
    showSimpleMenu(e.clientX, e.clientY, [
      { label: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" style="width:13px;height:13px;vertical-align:-2px;margin-right:6px;"><path d="M6 2.5h5.5L15 6v11a.5.5 0 0 1-.5.5h-8a.5.5 0 0 1-.5-.5v-14a.5.5 0 0 1 .5-.5z"/><path d="M11.5 2.5V6H15"/></svg>' + t('blocks.codeView.menuItem', 'Ver código'), onClick: openExamCodeView },
    ])
  })
  toolbar.appendChild(exercisesBtn)

  // aiSparkleSVG: icona compartida dos botóns de IA (degradado --ai-1->
  // --ai-2, ver blocks.css) - un <linearGradient> por botón (ids propios)
  // porque cada un vive no seu <svg>, os defs non se comparten entre eles.
  const aiSparkleSVG = (gradId) =>
    `<svg viewBox="0 0 20 20" fill="url(#${gradId})"><defs><linearGradient id="${gradId}" x1="0" y1="0" x2="20" y2="20"><stop offset="0" stop-color="var(--ai-1)"/><stop offset="1" stop-color="var(--ai-2)"/></linearGradient></defs><path d="M10 2.5l1.6 4.4L16 8.5l-4.4 1.6L10 14.5l-1.6-4.4L4 8.5l4.4-1.6z"/></svg>`

  if (typeof generateExerciseWithAI === 'function') {
    const aiBtn = document.createElement('button')
    aiBtn.type = 'button'
    aiBtn.className = 'blocks-toolbar__ai'
    aiBtn.innerHTML = aiSparkleSVG('aiGradExercicio') + '<span></span>'
    aiBtn.querySelector('span').textContent = t('blocks.exerciseWithAI', 'Exercicio con IA')
    aiBtn.addEventListener('click', async () => {
      // Se o xerador xa está desacoplado noutra xanela, non abrir tamén a
      // modal integrada: só traer esa xanela ao foco.
      if (xeradorIAAberta && await xeradorIAAberta()) { focarXeradorIA && focarXeradorIA('exercicio'); return }
      openExerciseAIModal({
        t,
        generateExerciseWithAI,
        onCreateExercise: (elements) => addExerciseWithElements(elements),
        onReplaceExercise: (index, elements) => replaceExerciseElements(index, elements),
        onGenerateResult,
        onDesacoplar: onDetachExerciseAI,
      })
    })
    toolbar.appendChild(aiBtn)
  }

  if (typeof generateExamWithAI === 'function') {
    const examBtn = document.createElement('button')
    examBtn.type = 'button'
    examBtn.className = 'blocks-toolbar__ai'
    examBtn.innerHTML = aiSparkleSVG('aiGradExame') + '<span></span>'
    examBtn.querySelector('span').textContent = t('blocks.examWithAI', 'Exame completo con IA')
    examBtn.addEventListener('click', async () => {
      if (xeradorIAAberta && await xeradorIAAberta()) { focarXeradorIA && focarXeradorIA('exame'); return }
      openExamAIModal({
        t,
        generateExamWithAI,
        onCreateExercises: (listaExercicios) => addExercisesWithElements(listaExercicios),
        onReplaceExercises: (start, count, listaExercicios) => replaceExercisesRange(start, count, listaExercicios),
        onGenerateResult,
        onDesacoplar: onDetachExamAI,
      })
    })
    toolbar.appendChild(examBtn)
  }
  // "⤢ Ampliar"/"⤡ Repregar" (resultado desacoplado, modo dúas xanelas):
  // un só botón que troca de icona/texto coma un interruptor, en vez de
  // dous botóns amosados/agochados por separado - menos DOM, mesmo efecto
  // visual. Agochado por defecto (display:none): main.js só o amosa
  // (setFullWidthToggleVisible) mentres o panel de código de fondo existe
  // realmente (resultadoDesacoplado && editMode === 'blocks', ver
  // actualizarPanelFondo) - fóra dese caso non hai nada que ampliar/
  // repregar. Vive AQUÍ (non nun botón fixo de main.js) porque é o editor
  // de bloques quen ten sentido ampliar (o de código xa ocupa sempre o
  // 100% do seu propio lado, ver mostrarCodigoDeFondo).
  let fullWidth = false
  const fullWidthBtn = document.createElement('button')
  fullWidthBtn.type = 'button'
  fullWidthBtn.className = 'blocks-toolbar__fullwidth'
  fullWidthBtn.style.display = 'none'
  fullWidthBtn.style.marginLeft = 'auto'
  function updateFullWidthBtn() {
    fullWidthBtn.textContent = fullWidth
      ? t('blocks.fullWidth.collapse', '⤡ Repregar')
      : t('blocks.fullWidth.expand', '⤢ Ampliar')
    fullWidthBtn.title = fullWidth
      ? t('blocks.fullWidth.collapse.title', 'Volver á partición normal (amosa de novo o código de fondo)')
      : t('blocks.fullWidth.expand.title', 'Ocupar todo o ancho (agocha o código de fondo)')
  }
  updateFullWidthBtn()
  fullWidthBtn.addEventListener('click', () => {
    fullWidth = !fullWidth
    updateFullWidthBtn()
    if (typeof onToggleFullWidth === 'function') onToggleFullWidth(fullWidth)
  })
  toolbar.appendChild(fullWidthBtn)

  // Nota: a Biblioteca (📚 listar/inserir) do editor antigo xa non tiña
  // ningunha entrada activa na UI (ver blocks-ai-library-modals.js) - só se
  // porta o "💾 Gardar na biblioteca" (menú contextual de bloque, ver
  // blockly/context-menu.js).

  const workspaceDiv = document.createElement('div')
  workspaceDiv.className = 'blocks-canvas'
  // Estilos fixados por JS (non só a regra .blocks-canvas de blocks.css,
  // pensada orixinalmente para unha lista HTML normal con scroll nativo,
  // ver blocks.css): Blockly xestiona o seu propio panning/zoom/scrollbars
  // dentro do SVG, así que o contedor que o aloxa precisa overflow:hidden e
  // sen padding propio (un padding aquí desprazaría os cálculos internos de
  // Blockly, que mide directamente as dimensións do seu contedor).
  workspaceDiv.style.flex = '1'
  workspaceDiv.style.minHeight = '0'
  workspaceDiv.style.position = 'relative'
  workspaceDiv.style.overflow = 'hidden'
  workspaceDiv.style.padding = '0'
  // isolation:isolate forza un contexto de apilamento propio para todo o
  // SVG de Blockly (lenzo, barras de desprazamento, superficie de
  // arrastre...): en WebKitGTK (o webview de Linux) observouse que estas
  // pezas do SVG - promovidas á súa propia capa de composición por Blockly
  // internamente para arrastrar bloques con fluidez - poden pintarse POR
  // ENRIBA doutro contido con z-index maior (p.ex. as ventás modais
  // .overlay, ver style.css) en vez de respectar a orde normal de
  // apilamento, dando barras de desprazamento fóra de lugar tapando
  // modais. isolation:isolate obriga a que TODO o que hai dentro do SVG
  // quede contido dentro da propia franxa de apilamento de workspaceDiv,
  // sen poder escapar por riba de elementos posteriores no DOM coma
  // .overlay - non se observou este problema en Chrome/Firefox, pero a
  // regra non lles afecta (só cambia como se resolven conflitos que aí
  // nunca se dan).
  workspaceDiv.style.isolation = 'isolate'
  workspaceDiv.style.zIndex = '0'

  container.appendChild(toolbar)
  container.appendChild(workspaceDiv)

  const workspace = Blockly.inject(workspaceDiv, {
    toolbox: buildToolbox(t),
    theme: matexeTheme,
    renderer: 'zelos',
    trashcan: true,
    zoom: { controls: true, wheel: true, startScale: 0.9 },
    grid: { spacing: 20, length: 3, colour: '#e3e3e0', snap: true },
  })

  // ResizeObserver, non só o resize da FIESTRA: Blockly.inject() mide o
  // tamaño de workspaceDiv no intre exacto en que se chama - se aínda non
  // ten a súa medida final do layout flexbox (.blocks-root/.blocks-canvas)
  // nese intre, o SVG queda coa medida vella (moitas veces cuase 0) para
  // sempre, aínda que window nunca cambie de tamaño. Isto pasa de forma
  // fiable en WebKitGTK (o webview de Linux) aínda que nunca en Chrome/
  // Firefox - confirmado comparando o MESMO HTML/JS en Firefox (perfecto)
  // fronte á app empaquetada (bloques amosábanse comprimidos a case nada).
  // ResizeObserver dispárase en canto o propio contedor cambia de tamaño
  // por calquera motivo (layout, non só a fiestra), así que arranxa isto
  // sen depender de adiviñar cando remata o layout inicial.
  const resizeObserver = new ResizeObserver(() => Blockly.svgResize(workspace))
  resizeObserver.observe(workspaceDiv)

  // O lixo NATIVO de Blockly só serve para amosar os bloques eliminados
  // recentemente (Trashcan.prototype.click: "hasContents() &&
  // openFlyout()") - premelo cando está baleiro (nada eliminado aínda
  // nesta sesión) non fai NADA, coma se estivese roto. Real report:
  // profesorado esperaba que premer o lixo baleirase o exercicio activo
  // dun golpe. "Eliminar N bloques" xa existe co botón dereito do lenzo
  // (Blockly.ContextMenuItems.registerDefaultOptions, ver
  // ensureDefaultContextMenu arriba) - isto reutiliza ESA mesma idea (con
  // confirmación) enganchada ao clic do lixo, en vez de reimplementar o
  // borrado a man. trashcan.click substitúese só nesta instancia (non no
  // prototipo), así que non afecta outros workspaces Blockly da páxina.
  if (workspace.trashcan) {
    const trashcan = workspace.trashcan
    const nativeClick = trashcan.click.bind(trashcan)
    trashcan.click = async () => {
      if (typeof trashcan.hasContents === 'function' && trashcan.hasContents()) {
        nativeClick()
        return
      }
      const topBlocks = workspace.getTopBlocks(false)
      if (topBlocks.length === 0) return
      const msg = t('blocks.clearAll.confirm', 'Eliminar TODOS os bloques deste exercicio?')
      if (!await confirmar(msg)) return
      Blockly.Events.setGroup(true)
      try {
        for (const block of topBlocks) block.dispose(false, true)
      } finally {
        Blockly.Events.setGroup(false)
      }
    }
  }

  // legacy replica doc.legacy do editor antigo (ver blocks-serialize.js):
  // un documento vello (sen <EX>) escríbese SEN envolver mentres non se
  // toque a ESTRUTURA (engadir/quitar/mover bloques, engadir/eliminar/
  // duplicar/reordear EXERCICIOS) - editar só o contido dun campo
  // (BLOCK_CHANGE) non conta, por iso o listener de abaixo escoita
  // CREATE/DELETE/MOVE pero non CHANGE.
  let legacy = false
  // suppressTracking evita que a reconstrución programática do workspace
  // (docToWorkspace, chamada por reloadWorkspaceFrom) dispare o propio
  // listener estrutural (que interpretaría os connect() internos coma
  // edicións do usuario) nin o auto-apertura de modal para "bloque novo
  // baleiro".
  let suppressTracking = false

  // exercisesData: UN {elements} por exercicio do exame, MESMA orde ca no
  // documento - fonte de verdade para calquera exercicio que non estea
  // activo agora mesmo (o activo lése en vivo do workspace, ver
  // flushActive/getValue). activeIndex: índice do exercicio aberto no
  // workspace, ou null se non hai ningún (exame baleiro).
  let exercisesData = []
  let activeIndex = null

  // flushActive garda o estado ACTUAL do workspace de volta en
  // exercisesData[activeIndex] - hai que chamala ANTES de calquera
  // operación que mute exercisesData ou troque de exercicio activo, senón
  // perderíanse as edicións feitas no exercicio que estaba aberto.
  function flushActive() {
    if (activeIndex === null) return
    const doc = workspaceToDoc(workspace)
    exercisesData[activeIndex] = doc.exercises[0] || { elements: [] }
  }

  // adoptOrphanBlocks: un anaco SOLTO no lenzo (fóra de calquera bloque
  // "Exercicio") non existe para o documento - workspaceToDoc só le a pila
  // ANACOS dun exercicio, así que ao gardar/xerar desaparecería en silencio.
  // Nun exame NOVO (aínda sen ningún exercicio, activeIndex===null) o
  // resultado é peor: o documento sae baleiro de todo e Xerar falla con "o
  // documento está baleiro" (app.go/latexdoc.go) aínda tendo bloques á
  // vista - report real: soltar unha imaxe nun documento novo e premer
  // Xerar. Por iso, en canto un anaco queda solto, adóptase: se non hai
  // ningún exercicio créase un, e a pila solta engánchase ao final da do
  // exercicio activo (na orde vertical en que estean no lenzo).
  //
  // suppressTracking mentres dura: os connect() de aquí non son edicións do
  // usuario (senón reabrirían a modal de "bloque novo"); legacy=false márcase
  // á man porque si é un cambio estrutural do documento.
  function adoptOrphanBlocks() {
    const orphans = workspace
      .getTopBlocks(true)
      .filter((block) => ELEMENT_TYPES.includes(elementTypeForBlock(block.type)))
    if (orphans.length === 0) return false

    suppressTracking = true
    try {
      let exBlock = workspace.getTopBlocks(false).find((b) => b.type === EXERCICIO_TYPE)
      if (!exBlock) {
        exBlock = buildExerciseBlock(workspace, [])
        exercisesData.push({ elements: [] })
        activeIndex = exercisesData.length - 1
        updateExercisesButton()
      }
      for (const orphan of orphans) {
        let last = exBlock.getInputTargetBlock('ANACOS')
        while (last && last.getNextBlock()) last = last.getNextBlock()
        const connection = last ? last.nextConnection : exBlock.getInput('ANACOS').connection
        connection.connect(orphan.previousConnection)
      }
    } finally {
      suppressTracking = false
    }
    legacy = false
    return true
  }

  // reloadWorkspaceFrom recarga o workspace dende exercisesData[index] SEN
  // gardar nada antes (chamar flushActive() antes se fai falla conservar o
  // que había) - se index non existe en exercisesData (exame baleiro tralo
  // eliminar o único exercicio), o workspace queda baleiro e activeIndex a
  // null.
  function reloadWorkspaceFrom(index) {
    const data = exercisesData[index]
    suppressTracking = true
    docToWorkspace({ exercises: data ? [data] : [] }, workspace)
    suppressTracking = false
    activeIndex = data ? index : null
    Blockly.svgResize(workspace)
    centerViewOnActiveExercise()
    updateExercisesButton()
  }

  // centerViewOnActiveExercise: pedido explícito - ao abrir un .matex (ou
  // trocar de exercicio, ver reloadWorkspaceFrom) os bloques aparecían
  // arriba á esquerda (onde os deixa docToWorkspace, ver adapter.js);
  // agora quedan centrados horizontalmente e cun 5% da alto visible por
  // riba do punto medio vertical (VERTICAL_OFFSET_RATIO - axeitado a 5%
  // tralo primeiro intento con 25%, que quedaba demasiado arriba).
  // centerOnBlock() xa centra perfectamente (conta escala/métricas por nós
  // - blockOnly=false por defecto, centra a PILA enteira de anacos, non só
  // o bloque "Exercicio"); o axuste é un scroll adicional despois: reducir
  // scrollY despraza o contido cara ARRIBA na pantalla (ver o comentario
  // de scrollY en Blockly, workspace_svg.d.ts - "as the canvas moves up,
  // this value becomes more negative"). Non se engancha a ningún resize
  // xeral (só se chama aquí, en reloadWorkspaceFrom) - recentrar cada vez
  // que a xanela cambia de tamaño mentres se edita sería máis molesto ca
  // útil.
  const VERTICAL_OFFSET_RATIO = 0.05
  function centerViewOnActiveExercise() {
    const exBlock = workspace.getTopBlocks(false)[0]
    if (!exBlock) return
    workspace.centerOnBlock(exBlock.id)
    const metrics = workspace.getMetrics()
    if (metrics) workspace.scroll(workspace.scrollX, workspace.scrollY - metrics.viewHeight * VERTICAL_OFFSET_RATIO)
  }

  function updateExercisesButton() {
    const n = exercisesData.length
    exercisesBtn.querySelector('span').textContent = n === 0
      ? t('blocks.exercises.buttonEmpty', 'Sen exercicios')
      : t('blocks.exercises.button', 'Exercicio {i} de {n}', { i: (activeIndex ?? 0) + 1, n })
  }

  function loadDoc(text) {
    exitCodeView()
    const doc = parseDocument(text || '')
    exercisesData = doc.exercises.map((ex) => ({ elements: ex.elements }))
    legacy = doc.legacy
    activeIndex = null
    reloadWorkspaceFrom(0)
  }

  function openEditorFor(block, { isNew }) {
    const type = elementTypeForBlock(block.type)
    const meta = ELEMENT_META[type]
    if (!meta) return
    openContentModal({
      type,
      meta,
      content: block.getFieldValue('CONTENT') ?? '',
      isNew,
      t,
      uploadImage,
      canUploadImage,
      generateWithAI,
      abrirAsistente,
      onChange: (newContent) => block.setFieldValue(newContent, 'CONTENT'),
      onSave: (newContent) => block.setFieldValue(newContent, 'CONTENT'),
      onCancel: () => {
        // Cancelar un anaco RECÉN creado bórrao (non queda un bloque
        // baleiro solto) - healStack=true reconecta o anterior co
        // seguinte, coma o antigo removeElement. Un anaco xa existente
        // (isNew=false) simplemente non muda ao cancelar.
        if (isNew && block.workspace) block.dispose(true)
      },
    })
  }

  // ---------- "Ver código" (exercicio activo / exame enteiro) ----------
  // Substitúe temporalmente o lenzo Blockly (workspaceDiv) por un
  // CodeMirror (createEditor, ver editor.js - así herda tamén o seu zoom
  // Ctrl+/Ctrl-) cun toolbar propio (título + 🤖 Asistente + volver a
  // bloques). Accesible por menú contextual: botón dereito no lenzo
  // (WORKSPACE-scoped, ver blockly/context-menu.js) para SÓ o exercicio
  // activo; botón dereito sobre "🗂️ Exercicio X de N" (arriba) para o exame
  // ENTEIRO. codeView non null mentres estea aberta unha destas vistas.
  let codeView = null // { scope: 'exercise' | 'exam', index, api, root }

  // syncCodeViewToData volca o texto ACTUAL do CodeMirror da vista de código
  // (se hai unha aberta) de volta en exercisesData - chamada tanto por
  // exitCodeView() coma por getValue() (para que gardar/xerar mentres se
  // está a editar código non perda o que aínda non se aplicou "Volver a
  // bloques"). Non toca o workspace nin activeIndex - só os datos.
  function syncCodeViewToData() {
    if (!codeView) return
    const text = codeView.api.getValue()
    if (codeView.scope === 'exercise') {
      exercisesData[codeView.index] = { elements: parseElements(text) }
    } else {
      const doc = parseDocument(text)
      exercisesData = doc.exercises.map((ex) => ({ elements: ex.elements }))
      legacy = doc.legacy
    }
  }

  // exitCodeView pecha calquera vista de código aberta, gardando primeiro o
  // que houbese (syncCodeViewToData) e recargando o workspace dende os datos
  // xa actualizados - chamada tamén (coma garda, sen custo se non hai
  // ningunha aberta) ao principio de calquera operación que mute
  // exercisesData/activeIndex por outra vía (trocar de exercicio, engadir,
  // eliminar...), para que esas operacións nunca traballen con datos
  // desactualizados nin deixen a vista de código orfa.
  function exitCodeView() {
    if (!codeView) return
    syncCodeViewToData()
    const { scope, index, api, root } = codeView
    codeView = null
    api.view.destroy()
    root.remove()
    workspaceDiv.classList.remove('hidden')
    const target = scope === 'exercise' ? index : Math.min(activeIndex ?? 0, exercisesData.length - 1)
    reloadWorkspaceFrom(target)
  }

  function buildCodeView({ scope, index, title, text }) {
    workspaceDiv.classList.add('hidden')

    const root = document.createElement('div')
    root.className = 'blocks-code-view'

    const bar = document.createElement('div')
    bar.className = 'blocks-code-view__toolbar'

    const titleEl = document.createElement('span')
    titleEl.className = 'blocks-code-view__title'
    titleEl.textContent = title
    bar.appendChild(titleEl)

    // 🤖 Asistente: mesma instancia có editor de Código principal e cós
    // modais de anaco (ver abrirAsistente, inxectado por main.js) - opera
    // sobre o texto ENTEIRO desta vista (documento .matex, exercicio solto
    // ou exame completo, os dous casos son "documento" para o asistente).
    if (typeof abrirAsistente === 'function') {
      const assistantBtn = document.createElement('button')
      assistantBtn.type = 'button'
      assistantBtn.className = 'blocks-code-view__assistant'
      assistantBtn.innerHTML = '<svg viewBox="0 0 20 20" fill="currentColor"><path d="M10 2.5l1.6 4.4L16 8.5l-4.4 1.6L10 14.5l-1.6-4.4L4 8.5l4.4-1.6z"/></svg><span></span>'
      assistantBtn.querySelector('span').textContent = t('blocks.codeView.assistant', 'Asistente')
      assistantBtn.addEventListener('click', () => {
        abrirAsistente({
          tipo: 'documento',
          title: t('assistant.codeTitle', 'Asistente — Código'),
          getContent: () => api.getValue(),
          setContent: (texto) => api.setValue(texto),
        })
      })
      bar.appendChild(assistantBtn)
    }

    const backBtn = document.createElement('button')
    backBtn.type = 'button'
    backBtn.className = 'blocks-code-view__back'
    backBtn.innerHTML = '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="6" height="6" rx="1.2"/><rect x="11" y="3" width="6" height="6" rx="1.2"/><rect x="3" y="11" width="6" height="6" rx="1.2"/><rect x="11" y="11" width="6" height="6" rx="1.2"/></svg><span></span>'
    backBtn.querySelector('span').textContent = t('blocks.codeView.back', 'Ver bloques')
    backBtn.addEventListener('click', () => exitCodeView())
    bar.appendChild(backBtn)

    root.appendChild(bar)

    const editorDiv = document.createElement('div')
    editorDiv.className = 'blocks-code-view__editor'
    root.appendChild(editorDiv)

    container.appendChild(root)

    const api = createEditor(editorDiv, { initialDoc: text })
    codeView = { scope, index, api, root }
    api.focus()
  }

  // openExerciseCodeView: menú contextual do lenzo (WORKSPACE, ver
  // registerEditor/blockly/context-menu.js) - SÓ o exercicio activo.
  function openExerciseCodeView() {
    exitCodeView() // por se xa había outra vista de código aberta
    flushActive()
    if (activeIndex === null) return
    buildCodeView({
      scope: 'exercise',
      index: activeIndex,
      title: t('blocks.codeView.exerciseTitle', '📄 Código — exercicio {n}', { n: activeIndex + 1 }),
      text: elementsToText(exercisesData[activeIndex].elements),
    })
  }

  // openExamCodeView: menú contextual do botón "🗂️ Exercicios" (arriba) -
  // TODOS os exercicios do exame, mesmo formato ca getValue()/documentToText.
  function openExamCodeView() {
    exitCodeView()
    flushActive()
    buildCodeView({
      scope: 'exam',
      index: null,
      title: t('blocks.codeView.examTitle', '📄 Código — todos os exercicios'),
      text: documentToText({ exercises: exercisesData, legacy }),
    })
  }

  registerEditor(workspace, {
    onEditContent: (block) => openEditorFor(block, { isNew: false }),
    // onViewExerciseCode/canViewExerciseCode: menú contextual do lenzo
    // (WORKSPACE-scoped, ver blockly/context-menu.js) -> "📄 Ver código".
    onViewExerciseCode: () => openExerciseCodeView(),
    canViewExerciseCode: () => activeIndex !== null,
    // t/saveToLibrary: resoltos aquí (non pasados directamente a
    // blockly/context-menu.js) porque ese rexistro é global e só sabe do
    // bloque premido - precisa consultar editor-registry.js igual có
    // ContentField, ver ese ficheiro.
    t,
    saveToLibrary,
  })

  workspace.addChangeListener((event) => {
    if (suppressTracking) return
    if (event.type === Blockly.Events.BLOCK_CREATE) {
      legacy = false
      const blockId = event.blockId
      // Diferido: BLOCK_CREATE dispárase no intre de crear o bloque (p.ex.
      // ao arrastralo fóra do flyout), antes de que quede conectado no
      // sitio definitivo - agardar un tick dá tempo a que o drop remate
      // (ou a que se cancele: se se soltou de volta no flyout, o bloque xa
      // non existe cando isto se executa, e simplemente non se fai nada).
      queueMicrotask(() => {
        const block = workspace.getBlockById(blockId)
        if (!block) return
        const type = elementTypeForBlock(block.type)
        if (!ELEMENT_TYPES.includes(type)) return
        if (block.getFieldValue('CONTENT')) return // xa ten contido (p.ex. pegado) - non reabrir
        openEditorFor(block, { isNew: true })
      })
      scheduleAdopt()
    } else if (event.type === Blockly.Events.BLOCK_DELETE || event.type === Blockly.Events.BLOCK_MOVE) {
      legacy = false
      if (event.type === Blockly.Events.BLOCK_MOVE) scheduleAdopt()
    }
  })

  // scheduleAdopt: adopta os anacos soltos (ver adoptOrphanBlocks) en canto
  // repousan no lenzo, para que o profesorado VEXA o bloque encaixar no
  // exercicio en vez de descubrir máis tarde que non contaba. Diferido polo
  // mesmo motivo ca a apertura da modal de arriba (o BLOCK_CREATE do flyout
  // chega antes de que o bloque estea no seu sitio) e saltado mentres dure
  // un arrastre: adoptar a metade dun drag pelexaría co usuario, e o
  // BLOCK_MOVE do final do arrastre volve pasar por aquí de todos os xeitos.
  function scheduleAdopt() {
    queueMicrotask(() => {
      if (workspace.isDragging?.()) return
      adoptOrphanBlocks()
    })
  }

  // ---------- Xestión de exercicios (ventá "🗂️ Exercicios") ----------
  // Cada función de abaixo mantén o mesmo protocolo: flushActive() ANTES
  // de tocar exercisesData (para non perder edicións do exercicio aberto),
  // mutar exercisesData, marcar legacy=false (calquera cambio na lista de
  // exercicios "sobe" o documento ao formato <EX>, coma calquera outro
  // cambio estrutural), e só recargar o workspace (reloadWorkspaceFrom)
  // cando o exercicio ACTIVO puido cambiar de contido ou desaparecer.

  // exitCodeView() ao principio de cada operación de abaixo: por se se
  // invocan (dende a ventá "🗂️ Exercicios", que segue accesible por riba
  // dunha vista de código aberta, ver openExercisesPanel/showExercisesPanel)
  // mentres se está a editar código - senón flushActive() lería o workspace
  // ESTANCADO (o lenzo queda oculto e sen tocar mentres dura a vista de
  // código) e perderíanse as edicións feitas aí. Sen custo se non hai
  // ningunha vista de código aberta.
  function switchTo(index) {
    exitCodeView()
    if (index === activeIndex) return
    flushActive()
    reloadWorkspaceFrom(index)
  }

  function addExercise() {
    exitCodeView()
    flushActive()
    exercisesData.push({ elements: [] })
    legacy = false
    reloadWorkspaceFrom(exercisesData.length - 1)
  }

  function addExerciseWithElements(elements) {
    exitCodeView()
    flushActive()
    exercisesData.push({ elements })
    legacy = false
    const novoIndex = exercisesData.length - 1
    reloadWorkspaceFrom(novoIndex)
    return novoIndex
  }

  function addExercisesWithElements(listaExercicios) {
    if (!listaExercicios || listaExercicios.length === 0) return null
    exitCodeView()
    flushActive()
    const startIndex = exercisesData.length
    for (const elements of listaExercicios) exercisesData.push({ elements })
    legacy = false
    reloadWorkspaceFrom(startIndex)
    return { start: startIndex, count: listaExercicios.length }
  }

  // replaceExerciseElements / replaceExercisesRange: usados pola modal
  // "Exercicio/Exame con IA" cando se preme "Xerar" máis dunha vez sen
  // pechar - substitúen o que creou a xeración anterior en vez de acumular.
  // Se o índice/rango xa non é válido (o profesorado borrou eses exercicios
  // á man entre medias) recórrese a engadir, para non tocar outro exercicio.
  function replaceExerciseElements(index, elements) {
    if (index == null || !exercisesData[index]) return addExerciseWithElements(elements)
    exitCodeView()
    flushActive()
    exercisesData[index] = { elements }
    legacy = false
    reloadWorkspaceFrom(index)
    return index
  }

  function replaceExercisesRange(start, count, listaExercicios) {
    if (!listaExercicios || listaExercicios.length === 0) return { start, count: 0 }
    if (start == null || start < 0 || start + count > exercisesData.length) {
      return addExercisesWithElements(listaExercicios)
    }
    exitCodeView()
    flushActive()
    const nuevos = listaExercicios.map((elements) => ({ elements }))
    exercisesData.splice(start, count, ...nuevos)
    legacy = false
    reloadWorkspaceFrom(start)
    return { start, count: nuevos.length }
  }

  function deleteExercise(index) {
    exitCodeView()
    flushActive()
    const wasActiveIndex = activeIndex
    exercisesData.splice(index, 1)
    legacy = false
    if (index === wasActiveIndex) {
      reloadWorkspaceFrom(Math.min(index, exercisesData.length - 1))
    } else if (wasActiveIndex !== null) {
      activeIndex = index < wasActiveIndex ? wasActiveIndex - 1 : wasActiveIndex
      updateExercisesButton()
    }
  }

  function duplicateExercise(index) {
    exitCodeView()
    flushActive()
    const source = exercisesData[index] || { elements: [] }
    const clone = { elements: source.elements.map((e) => ({ type: e.type, content: e.content })) }
    exercisesData.splice(index + 1, 0, clone)
    legacy = false
    reloadWorkspaceFrom(index + 1) // abre a copia, coma "duplicar sprite" en Scratch
  }

  function reorderExercises(from, to) {
    if (from === to) return
    exitCodeView()
    flushActive()
    const wasActiveIndex = activeIndex
    const [item] = exercisesData.splice(from, 1)
    exercisesData.splice(to, 0, item)
    legacy = false
    if (wasActiveIndex === from) activeIndex = to
    else if (wasActiveIndex !== null && from < wasActiveIndex && wasActiveIndex <= to) activeIndex -= 1
    else if (wasActiveIndex !== null && to <= wasActiveIndex && wasActiveIndex < from) activeIndex += 1
    updateExercisesButton()
  }

  function showExercisesPanel() {
    openExercisesPanel({
      t,
      count: exercisesData.length,
      activeIndex,
      onSwitch: (i) => { switchTo(i); closeExercisesPanel() },
      onAdd: () => { addExercise(); closeExercisesPanel() },
      // Eliminar/duplicar/reordear NON pechan a ventá (coma en Scratch: o
      // profesorado adoita facer varios destes cambios seguidos) - só a
      // redebuxan co estado novo, chamando showExercisesPanel() de novo.
      onDelete: (i) => { deleteExercise(i); showExercisesPanel() },
      onDuplicate: (i) => { duplicateExercise(i); showExercisesPanel() },
      onReorder: (from, to) => { reorderExercises(from, to); showExercisesPanel() },
    })
  }

  loadDoc(initialDoc)
  window.addEventListener('resize', () => Blockly.svgResize(workspace))

  return {
    getValue: () => {
      // syncCodeViewToData primeiro: se hai unha vista de código aberta
      // (ver "Ver código" arriba), o seu texto AÍNDA non aplicado a
      // exercisesData ("Volver a bloques" non se premeu) ten que contar
      // igualmente - senón Gardar/Xerar mentres se edita código perdería eses
      // cambios. Combina o exercicio activo (lido EN VIVO do workspace, pode
      // diferir do que hai en exercisesData[activeIndex] se aínda non se
      // chamou a flushActive - PERO só cando non hai vista de código aberta,
      // que é cando o workspace deixa de ser a fonte de verdade dese
      // exercicio) cos demais (xa gardados en exercisesData) - así getValue()
      // sempre devolve o exame ENTEIRO, aínda que o lenzo só amose un
      // exercicio (ou o seu código) de cada vez.
      syncCodeViewToData()
      // Rede de seguridade: se algún anaco quedou solto no lenzo e ningún
      // evento o adoptou aínda (ver adoptOrphanBlocks/scheduleAdopt),
      // adóptase AGORA, xusto antes de serializar - así gardar/xerar nunca
      // perde un bloque que está á vista. Só cando o lenzo é a fonte de
      // verdade: con vista de código aberta, o workspace está estancado.
      if (!codeView) adoptOrphanBlocks()
      const merged = exercisesData.map((data, i) => {
        if (i !== activeIndex || codeView) return data
        return workspaceToDoc(workspace).exercises[0] || { elements: [] }
      })
      return documentToText({ exercises: merged, legacy })
    },
    setValue: (text) => loadDoc(text),
    // add/replace*: expostas para o xerador con IA DESACOPLADO (xanela á
    // parte, ver xanelaxeradoria.go / main.js): a xanela principal escoita o
    // evento yang:xerador-ia-* e chámaas para inserir o que xerou a outra
    // xanela. Dentro da modal integrada úsanse como closures (ver arriba).
    addExerciseWithElements,
    replaceExerciseElements,
    addExercisesWithElements,
    replaceExercisesRange,
    focus: () => {
      if (codeView) { codeView.api.focus(); return }
      const first = workspace.getTopBlocks(true)[0]
      first?.select?.()
    },
    // undo/redo: chamables dende un botón visible (ver #btnUndo/#btnRedo,
    // main.js). Se hai unha vista "Ver código" aberta, desfacer/refacer
    // opera sobre ESE CodeMirror (mesmo criterio ca focus() enriba - é o
    // que se está a editar realmente nese intre); se non, sobre o propio
    // workspace Blockly (undo/redo de bloques xa vén de fábrica en
    // Blockly.WorkspaceSvg, undo(false)/undo(true) - seguro de chamar sen
    // nada que desfacer/refacer, non fai nada).
    undo: () => { if (codeView) codeView.api.undo(); else workspace.undo(false) },
    redo: () => { if (codeView) codeView.api.redo(); else workspace.undo(true) },
    destroy: () => {
      if (codeView) { codeView.api.view.destroy(); codeView.root.remove(); codeView = null }
      resizeObserver.disconnect()
      unregisterEditor(workspace)
      workspace.dispose()
      container.innerHTML = ''
      container.classList.remove('blocks-root')
      // A ventá "🗂️ Exercicios" é un overlay compartido fóra de `container`
      // (mesmo patrón cás modais de blocks-ai-library-modals.js) - se quedou
      // aberta (p.ex. tralo eliminar/duplicar, que a redebuxan en vez de
      // pechala), hai que pechala explicitamente aquí: senón sobreviviría a
      // este editor, amosando datos dun workspace xa destruído.
      closeExercisesPanel()
    },
    // setFullWidthToggleVisible: chamada por main.js dende
    // actualizarPanelFondo() cada vez que resultadoDesacoplado/editMode
    // cambian - amosa/agocha "⤢ Ampliar"/"⤡ Repregar" (ver arriba). Se se
    // agocha estando en estado "ampliado", vólvese a "normal" á vez
    // (chamando onToggleFullWidth(false)) para non deixar o panel de
    // código de fondo agochado por un estado que xa non ten sentido (ex.
    // trocar a modo Código, ou acoplar o resultado de novo).
    setFullWidthToggleVisible: (visible) => {
      if (!visible && fullWidth) {
        fullWidth = false
        updateFullWidthBtn()
        if (typeof onToggleFullWidth === 'function') onToggleFullWidth(false)
      }
      fullWidthBtn.style.display = visible ? '' : 'none'
    },
    // Escotilla só para tests (blocks-editor.test.js) - non forma parte do
    // contrato estable (ver plan: getValue/setValue/focus/destroy é o único
    // que main.js pode asumir). Deixa inspeccionar/manipular bloques
    // directamente sen ter que simular clics en coordenadas SVG.
    _workspaceForTests: workspace,
    // Mesma idea, para a vista "Ver código" (ver createEditor, editor.js) -
    // deixa ler/escribir directamente o CodeMirror da vista aberta (ou null
    // se non hai ningunha) sen ter que simular tecleo real dentro del.
    _codeViewApiForTests: () => (codeView ? codeView.api : null),
  }
}
