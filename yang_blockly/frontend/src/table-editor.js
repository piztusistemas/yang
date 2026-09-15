// createTableEditor - interface de edición tipo FOLLA DE CÁLCULO, alternativa
// a Blockly (blocks-editor.js). Mesmo contrato cara a main.js
// (getValue/setValue/focus/destroy/undo/redo), máis tres métodos propios das
// lapelas de visualización (getModo/setModo/onModoChange) e o
// setFullWidthToggleVisible que xa usa o modo dúas xanelas.
//
// A idea (ver docs/tabla.md): o profesorado ve unha táboa cunha fila por
// exercicio. Cada fila ten o enunciado, a lista de variables usadas (cada
// unha FIXA, ALEATORIA ou AUTOMÁTICA), e - segundo a lapela activa - a
// resposta final e/ou a solución. A lapela activa decide tamén QUE se xera á
// dereita (main.js pásalle o `modo` a GeneratePDF): folla para o alumnado,
// folla con resposta, ou folla con solucións. A cuarta lapela, "Probas",
// avalía cada exercicio en varias sementes buscando infinitos,
// indeterminacións e erros (ver table-probes.js / proba.go).
//
// O modelo (estrutura de exercicios/variables) vive nun JSON incrustado nun
// comentario da primeira liña do .matex - toda esa conversión está en
// table-serialize.js, aquí só se edita o modelo e se re-serializa.

import {
  parseDocument, documentToText, modelToBody,
  makeExercise, makeVariable,
  XERADORES, MODOS_VARIABLE,
} from './table-serialize.js'
import { openExerciseAIModal, openExamAIModal } from './blocks-ai-library-modals.js'
import { confirmar, avisar } from './dialogs.js'
import { createTableProbes } from './table-probes.js'

// LAPELAS: id interno (== `modo` que se manda ao backend) + clave i18n. O id
// 'proba' mantense por dentro (backend RenderMode, binding ProbarExercicios);
// só cambia a etiqueta visible a "Test".
const LAPELAS = [
  { id: 'enunciados', k: 'table.tab.enunciados', d: 'Enunciados' },
  { id: 'resposta', k: 'table.tab.resposta', d: 'Resultado' },
  { id: 'solucions', k: 'table.tab.solucions', d: 'Resolución' },
  { id: 'proba', k: 'table.tab.test', d: 'Test' },
]

// Opcións do despregable "modo" de cada variable. "rango" non é un xerador
// Maxima real (ver table-serialize.js) - é a opción "de X a Y".
function opcionsXerador(t) {
  return [
    { v: 'rango', label: t('table.gen.rango', 'de X a Y (paso)') },
    { v: 'n0', label: t('table.gen.n0', 'natural pequeno (1–9)') },
    { v: 'n1', label: t('table.gen.n1', 'natural mediano (10–99)') },
    { v: 'n2', label: t('table.gen.n2', 'natural grande (100–999)') },
    { v: 'z0', label: t('table.gen.z0', 'enteiro pequeno (±9)') },
    { v: 'z1', label: t('table.gen.z1', 'enteiro mediano (±99)') },
    { v: 'z2', label: t('table.gen.z2', 'enteiro grande (±999)') },
    { v: 'q0', label: t('table.gen.q0', 'fracción sinxela') },
    { v: 'q1', label: t('table.gen.q1', 'fracción mediana') },
    { v: 'q2', label: t('table.gen.q2', 'fracción grande') },
  ]
}

export function createTableEditor(container, {
  initialDoc = '', t: tIn, abrirAsistente, probarExercicios,
  generateExerciseWithAI, generateExamWithAI, crearReorganizar,
  startExamWithAI, generateExamChunkWithAI,
  onToggleFullWidth, onGenerateResult, onGenerateExercise,
} = {}) {
  const t = tIn || ((clave, fallback, vars) => {
    let val = fallback ?? clave
    if (vars) for (const k in vars) val = val.replaceAll(`{${k}}`, vars[k])
    return val
  })

  // ---------- estado ----------
  let model = { v: 1, exercicios: [] }
  let legacy = false
  let originalText = ''
  let dirty = false
  let modo = 'enunciados'
  let onModoChangeCb = null
  let codeView = null // <pre> de "Ver código" ou null
  let buscaAberta = false // barra de busca de "Ver código" (Ctrl+F) aberta?
  let buscaTexto = ''
  let buscaActual = 0 // índice (entre as marcas atopadas) do resultado activo
  let fullWidth = false
  const undoStack = []
  const redoStack = []

  // ---------- armazón DOM ----------
  container.innerHTML = ''
  container.classList.add('table-editor')

  const tabsEl = el('div', 'table-tabs')
  tabsEl.setAttribute('role', 'tablist')
  const legendEl = el('div', 'table-legend')
  const toolbarEl = el('div', 'table-toolbar')
  const scrollEl = el('div', 'table-scroll')
  container.append(tabsEl, legendEl, toolbarEl, scrollEl)

  // --- lapelas ---
  for (const lap of LAPELAS) {
    const b = el('button', 'table-tab')
    b.type = 'button'
    b.dataset.modo = lap.id
    b.setAttribute('role', 'tab')
    b.textContent = t(lap.k, lap.d)
    b.addEventListener('click', () => setModo(lap.id))
    tabsEl.appendChild(b)
  }

  // --- barra de ferramentas ---
  const btnAdd = toolBtn('＋ ' + t('table.addExercise', 'exercicio'), 'table-toolbar__add', () => {
    pushUndo()
    model.exercicios.push(makeExercise())
    markDirty(); render(); focarUltimoEnunciado()
  })
  // ✨ Exercicio con IA / ✨ Exame con IA - reutilizan as modais do editor de
  // bloques (blocks-ai-library-modals.js) con modoTaboa:true; a IA devolve xa
  // a estrutura da táboa e convértese con exercicioDesdeTaboa.
  const btnAiExercicio = typeof generateExerciseWithAI === 'function'
    ? aiToolBtn('tblAiEx', t('blocks.exerciseAI.title', 'Exercicio con IA'), abrirAsistenteExercicioIA)
    : null
  const btnAiExame = typeof generateExamWithAI === 'function'
    ? aiToolBtn('tblAiExam', t('blocks.examAI.title', 'Exame completo con IA'), abrirAsistenteExameIA)
    : null

  const btnAssistant = abrirAsistente ? toolBtn('', 'table-toolbar__ai', abrirAsistenteDocumento) : null
  if (btnAssistant) {
    btnAssistant.innerHTML = sparkSVG('tblAiDoc') + '<span></span>'
    btnAssistant.querySelector('span').textContent = t('table.assistant', 'Asistente')
  }
  const btnCode = toolBtn('📄 ' + t('table.viewCode', 'Ver código'), '', toggleCodeView)

  const btnFullWidth = toolBtn('⤢ ' + t('blocks.fullWidth.expand', 'Ampliar').replace('⤢ ', ''), 'table-toolbar__fullwidth', () => {
    fullWidth = !fullWidth
    updateFullWidthBtn()
    onToggleFullWidth && onToggleFullWidth(fullWidth)
  })
  btnFullWidth.style.display = 'none'
  btnFullWidth.style.marginLeft = 'auto'

  toolbarEl.append(btnAdd)
  if (btnAiExercicio) toolbarEl.append(btnAiExercicio)
  if (btnAiExame) toolbarEl.append(btnAiExame)
  if (btnAssistant) toolbarEl.append(btnAssistant)
  toolbarEl.append(btnCode, btnFullWidth)

  // ---------- carga inicial ----------
  loadDoc(initialDoc)

  // =========================================================================
  //  RENDER
  // =========================================================================
  function render() {
    // lapelas activas
    for (const b of tabsEl.children) b.classList.toggle('table-tab--active', b.dataset.modo === modo)
    const legenda = legendaDe(modo)
    legendEl.textContent = legenda
    legendEl.hidden = !legenda
    // Na lapela "Test" ocúltanse xerar-con-IA e "Ver código" (alí non pintan
    // nada), pero o ASISTENTE queda SEMPRE visible: nesa lapela é onde máis
    // se usa, para pedirlle que corrixa un exercicio que dá inf/indeterminado.
    const agocharEnTest = modo === 'proba' ? 'none' : ''
    btnCode.style.display = agocharEnTest
    btnAiExercicio && (btnAiExercicio.style.display = agocharEnTest)
    btnAiExame && (btnAiExame.style.display = agocharEnTest)
    btnAssistant && (btnAssistant.style.display = '')

    if (codeView) { renderCodeView(); return }
    scrollEl.innerHTML = ''

    if (modo === 'proba') {
      renderProbas()
      return
    }
    scrollEl.appendChild(buildGrid())
    ajustarTA()
  }

  // ajustarTA: axusta TODAS as caixas de texto ao seu contido. scrollHeight
  // só é fiable co elemento xa no DOM e visible - por iso vai tamén nun
  // requestAnimationFrame (cobre o caso de renderizar mentres #tableEditor
  // aínda está oculto, xusto antes de que switchMode o amose).
  function ajustarTA() {
    const pasada = () => scrollEl.querySelectorAll('.table-ta').forEach(autoGrow)
    pasada()
    requestAnimationFrame(pasada)
  }

  // "Enunciados"/"Resultado"/"Resolución" xa dín por si soas que xéranse -
  // a lapela deixou de precisar unha liña de lenda debaixo (menos altura,
  // e así o bloque de lapelas queda á par ca cabeceira de botóns do panel
  // de resultado, .pane-header-preview). "Test" si a mantén: non é obvio
  // polo nome do que fai (validar varias sementes, non xerar folla ningunha).
  function legendaDe(m) {
    return {
      proba: t('table.legend.proba', 'Comproba que as respostas dan valores coherentes en varias sementes (sen infinitos nin indeterminacións).'),
    }[m] || ''
  }

  function buildGrid() {
    const showResp = modo === 'resposta' || modo === 'solucions'
    const showSol = modo === 'solucions'

    const grid = el('div', 'table-grid')
    // cabeceira
    const head = el('div', 'table-row table-row--head')
    head.append(
      cell('table-c-num', '#'),
      cell('table-c-enun', t('table.col.enunciado', 'Enunciado')),
      cell('table-c-vars', t('table.col.variables', 'Variables')),
    )
    if (showResp) head.append(cell('table-c-resp', t('table.col.resposta', 'Resultado final')))
    if (showSol) head.append(cell('table-c-sol', t('table.col.solucion', 'Resolución')))
    head.append(cell('table-c-acts', ''))
    grid.appendChild(head)

    if (!model.exercicios.length) {
      const vac = el('div', 'table-empty')
      vac.textContent = t('table.empty', 'Aínda non hai exercicios. Preme «＋ exercicio» para engadir o primeiro.')
      grid.appendChild(vac)
      return grid
    }

    model.exercicios.forEach((ex, i) => grid.appendChild(buildExerciseRow(ex, i, { showResp, showSol })))
    return grid
  }

  function buildExerciseRow(ex, i, { showResp, showSol }) {
    const row = el('div', 'table-row')
    row.dataset.index = String(i)

    // Nº + reordenar (drag)
    const numC = cell('table-c-num')
    numC.draggable = true
    numC.textContent = String(i + 1)
    numC.title = t('table.reorder', 'Arrastra para reordenar')
    row.appendChild(numC)
    wireDrag(numC, row, i)

    // Enunciado
    const enunC = cell('table-c-enun')
    enunC.appendChild(textArea(ex.enunciado, t('table.enunciado.ph', 'Escribe o enunciado. Usa {nome} para inserir o valor dunha variable.'), (val) => { ex.enunciado = val; markDirty() }))
    enunC.appendChild(tokenChips(ex, (tok) => insertToken(enunC, ex, 'enunciado', tok)))
    row.appendChild(enunC)

    // Variables
    row.appendChild(buildVarsCell(ex, i))

    if (showResp) {
      const c = cell('table-c-resp')
      c.appendChild(textArea(ex.resposta, t('table.resposta.ph', 'Resultado final (ex.: A área é {area} u²).'), (val) => { ex.resposta = val; markDirty() }))
      c.appendChild(tokenChips(ex, (tok) => insertToken(c, ex, 'resposta', tok)))
      row.appendChild(c)
    }
    if (showSol) {
      const c = cell('table-c-sol')
      c.appendChild(textArea(ex.solucion, t('table.solucion.ph', 'Resolución: explicación paso a paso ata o resultado.'), (val) => { ex.solucion = val; markDirty() }))
      c.appendChild(tokenChips(ex, (tok) => insertToken(c, ex, 'solucion', tok)))
      row.appendChild(c)
    }

    // Accións
    const actsC = cell('table-c-acts')
    actsC.append(...[
      // ✓ "Aplicar": recomputa SÓ este exercicio á dereita. Mentres se edita
      // non se recompila nada (ver nota xunto a sourceDunExercicio); este
      // botón é o único disparo manual - non toca o resto da práctica.
      typeof onGenerateExercise === 'function'
        ? miniBtn('✓', t('table.previewOne', 'Aplicar: ver só este exercicio á dereita'), () => onGenerateExercise(sourceDunExercicio(ex)), 'mini-ok')
        : null,
      miniBtn('↑', t('table.up', 'Subir'), () => moveExercise(i, i - 1)),
      miniBtn('↓', t('table.down', 'Baixar'), () => moveExercise(i, i + 1)),
      miniBtn('⧉', t('table.duplicate', 'Duplicar'), () => duplicateExercise(i)),
      // ↻ IA: reenvía SÓ este exercicio á IA para (re)xerar a resposta final
      // por apartados e a resolución paso a paso de forma normalizada, e
      // valídaa en varias sementes (mesmo motor ca "Reorganizar práctica",
      // pero acoutado a esta fila). Non toca os demais exercicios.
      typeof crearReorganizar === 'function'
        ? miniBtn('↻', t('table.regenSol', 'Rexenerar a resolución paso a paso deste exercicio coa IA'), (e) => rexenerarResolucion(i, e.currentTarget), 'mini-ai')
        : null,
      abrirAsistente ? aiMiniBtn(() => abrirAsistenteExercicio(i)) : null,
      miniBtn('✕', t('table.delete', 'Eliminar'), () => deleteExercise(i), 'mini-danger'),
    ].filter(Boolean))
    row.appendChild(actsC)
    return row
  }

  function buildVarsCell(ex, exIndex) {
    const c = cell('table-c-vars')
    const list = el('div', 'table-varlist')
    ex.variables.forEach((v, vi) => list.appendChild(buildVarRow(ex, exIndex, v, vi)))
    c.appendChild(list)
    const add = el('button', 'table-var-add')
    add.type = 'button'
    add.textContent = '＋ ' + t('table.addVariable', 'variable')
    add.addEventListener('click', () => {
      pushUndo()
      ex.variables.push(makeVariable(sugerirNome(ex), 'aleatoria'))
      markDirty(); render()
    })
    c.appendChild(add)
    return c
  }

  function buildVarRow(ex, exIndex, v, vi) {
    const r = el('div', 'table-var')

    const nome = el('input', 'table-var-nome')
    nome.type = 'text'
    nome.value = v.nome || ''
    nome.placeholder = t('table.var.nome', 'nome')
    nome.spellcheck = false
    nome.addEventListener('input', () => { v.nome = nome.value.trim(); markDirty() })
    r.appendChild(nome)

    const modoSel = el('select', 'table-var-modo')
    for (const m of MODOS_VARIABLE) {
      const o = document.createElement('option')
      o.value = m
      o.textContent = { fixa: t('table.mode.fixa', 'fixa'), aleatoria: t('table.mode.aleatoria', 'aleatoria'), auto: t('table.mode.auto', 'automática') }[m]
      modoSel.appendChild(o)
    }
    modoSel.value = v.modo
    modoSel.addEventListener('change', () => {
      pushUndo()
      cambiarModoVar(v, modoSel.value)
      markDirty(); render()
    })
    r.appendChild(modoSel)

    r.appendChild(buildVarValueField(v))

    r.appendChild(miniBtn('✕', t('table.var.remove', 'Quitar variable'), () => {
      pushUndo()
      ex.variables.splice(vi, 1)
      markDirty(); render()
    }, 'mini-danger'))
    return r
  }

  function buildVarValueField(v) {
    const wrap = el('div', 'table-var-val')
    if (v.modo === 'fixa') {
      wrap.appendChild(inputBound('table-var-in table-var-in--grow', v.valor || '', t('table.var.valor', 'valor'), (x) => { v.valor = x; markDirty() }))
    } else if (v.modo === 'auto') {
      wrap.appendChild(inputBound('table-var-in table-var-in--grow table-var-in--code', v.expr || '', t('table.var.expr', 'expresión'), (x) => { v.expr = x; markDirty() }))
    } else {
      // aleatoria: xerador + (se rango) "de … a … paso …"
      const gen = el('select', 'table-var-gen')
      for (const op of opcionsXerador(t)) {
        const o = document.createElement('option')
        o.value = op.v; o.textContent = op.label
        gen.appendChild(o)
      }
      gen.value = v.xer === 'rango' ? 'rango' : (XERADORES.includes(v.xer) ? v.xer : 'n1')
      gen.addEventListener('change', () => {
        pushUndo()
        if (gen.value === 'rango') { v.xer = 'rango'; v.min = v.min || '1'; v.max = v.max || '10'; v.paso = v.paso || '1' }
        else { v.xer = gen.value; delete v.min; delete v.max; delete v.paso }
        markDirty(); render()
      })
      wrap.appendChild(gen)
      if (v.xer === 'rango') wrap.appendChild(buildRango(v))
    }
    return wrap
  }

  // buildRango: "de [min] a [max]  paso [paso]" con etiquetas pequenas no
  // canto de tres caixas cegas con placeholder.
  function buildRango(v) {
    const box = el('div', 'table-var-rango')
    const seg = (lab, valor, key) => {
      const s = el('span', 'table-var-rango__seg')
      const l = el('span', 'table-var-rango__lab')
      l.textContent = lab
      s.append(l, inputBound('table-var-num', valor, '', (x) => { v[key] = x; markDirty() }))
      return s
    }
    box.append(
      seg(t('table.rango.de', 'de'), v.min ?? '1', 'min'),
      seg(t('table.rango.a', 'a'), v.max ?? '10', 'max'),
      seg(t('table.rango.paso', 'paso'), v.paso ?? '1', 'paso'),
    )
    return box
  }

  function renderProbas() {
    const host = el('div', 'table-probes-host')
    scrollEl.appendChild(host)
    createTableProbes(host, {
      t,
      getSource: () => documentToText(commitModel()),
      probarExercicios,
      numExercicios: model.exercicios.length,
    })
  }

  // ---------- busca en "Ver código" (Ctrl+F) ----------
  function escaparHTMLCodigo(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  }

  // pintarLiñaConBusca: escapa a liña para HTML e, se hai busca activa,
  // envolve en <mark> as coincidencias (sen distinguir maiúsculas/
  // minúsculas). Escápanse os DOUS lados (liña e termo buscado) coa mesma
  // función antes de comparar, así unha busca por "<MAT>" segue atopando
  // "<MAT>" aínda que por dentro comparemos "&lt;MAT&gt;" contra "&lt;MAT&gt;".
  function pintarLiñaConBusca(liña) {
    const escapada = escaparHTMLCodigo(liña)
    const termo = buscaTexto.trim()
    if (!termo) return escapada
    const termoEscapado = escaparHTMLCodigo(termo).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const re = new RegExp(termoEscapado, 'gi')
    return escapada.replace(re, (m) => `<mark class="table-code__match">${m}</mark>`)
  }

  // actualizarBusca: marca cal das coincidencias xa pintadas é a "activa"
  // (buscaActual), actualiza o contador e desprázaa á vista.
  function actualizarBusca() {
    const marcas = scrollEl.querySelectorAll('mark.table-code__match')
    const contador = scrollEl.querySelector('.table-code__search-count')
    if (contador) {
      contador.textContent = marcas.length
        ? `${buscaActual + 1}/${marcas.length}`
        : (buscaTexto.trim() ? t('table.code.search.none', 'sen resultados') : '')
    }
    marcas.forEach((m, i) => m.classList.toggle('table-code__match--actual', i === buscaActual))
    marcas[buscaActual]?.scrollIntoView({ block: 'center' })
  }

  function irAResultadoBusca(delta) {
    const marcas = scrollEl.querySelectorAll('mark.table-code__match')
    if (!marcas.length) { buscaActual = 0; return }
    buscaActual = (buscaActual + delta + marcas.length) % marcas.length
    actualizarBusca()
  }

  function abrirBuscaCodigo() {
    if (!codeView) return
    buscaAberta = true
    renderCodeView()
  }

  function pecharBuscaCodigo() {
    buscaAberta = false
    buscaTexto = ''
    buscaActual = 0
    if (codeView) renderCodeView()
  }

  // Ctrl+F/Cmd+F só cando "Ver código" está aberto E esta táboa é a
  // interface visible neste momento (senón, o atallo tería que ir para
  // outra parte da app - ex. o editor de bloques). Escoita en `document`
  // porque o foco pode estar en calquera botón da barra, non só dentro do
  // <ol> (que non é focable).
  document.addEventListener('keydown', (e) => {
    if (!codeView || container.classList.contains('hidden')) return
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'f') {
      e.preventDefault()
      abrirBuscaCodigo()
    }
  })

  function renderCodeView() {
    scrollEl.innerHTML = ''
    const bar = el('div', 'table-code__bar')
    const tit = el('span', 'table-code__title')
    tit.textContent = t('table.code.title', 'Código .matex que xera a táboa (só lectura)')
    const btnBuscar = el('button', 'table-code__search-btn')
    btnBuscar.type = 'button'
    btnBuscar.title = t('table.code.search.title', 'Buscar no código (Ctrl+F)')
    btnBuscar.textContent = '🔍'
    btnBuscar.addEventListener('click', abrirBuscaCodigo)
    const back = el('button', 'table-code__back')
    back.type = 'button'
    back.textContent = '↩ ' + t('table.code.back', 'Volver á táboa')
    back.addEventListener('click', () => { codeView = null; pecharBuscaCodigo(); render() })
    const accions = el('div', 'table-code__bar-actions')
    accions.append(btnBuscar, back)
    bar.append(tit, accions)

    const search = el('div', 'table-code__search')
    search.hidden = !buscaAberta
    let inputBusca = null
    if (buscaAberta) {
      inputBusca = document.createElement('input')
      inputBusca.type = 'text'
      inputBusca.className = 'table-code__search-input'
      inputBusca.placeholder = t('table.code.search.placeholder', 'Buscar no código...')
      inputBusca.value = buscaTexto
      inputBusca.addEventListener('input', () => {
        buscaTexto = inputBusca.value
        buscaActual = 0
        renderCodeView()
      })
      inputBusca.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') { e.preventDefault(); pecharBuscaCodigo() }
        else if (e.key === 'Enter') { e.preventDefault(); irAResultadoBusca(e.shiftKey ? -1 : 1) }
      })
      const contador = el('span', 'table-code__search-count')
      const prev = el('button', 'table-code__search-nav')
      prev.type = 'button'
      prev.textContent = '▲'
      prev.title = t('table.code.search.prev', 'Resultado anterior (Maiús+Intro)')
      prev.addEventListener('click', () => irAResultadoBusca(-1))
      const next = el('button', 'table-code__search-nav')
      next.type = 'button'
      next.textContent = '▼'
      next.title = t('table.code.search.next', 'Resultado seguinte (Intro)')
      next.addEventListener('click', () => irAResultadoBusca(1))
      const pechar = el('button', 'table-code__search-close')
      pechar.type = 'button'
      pechar.textContent = '✕'
      pechar.title = t('table.code.search.close', 'Pechar busca (Esc)')
      pechar.addEventListener('click', pecharBuscaCodigo)
      search.append(inputBusca, contador, prev, next, pechar)
    }

    // Números de liña (pedido: os erros de compilación de LaTeX din "line
    // N" e sen numeración era moi complicado atopar que liña arranxar) - un
    // <li> por liña do .matex, así o navegador numera el só e a numeración
    // non se desincroniza se cambia o contido. NOTA: N aquí é a liña do
    // .matex, non a do LaTeX final que cita o erro de compilación (levan
    // cabeceira e conversión de Maxima/Pandoc por medio, non son a mesma
    // numeración) - vale para localizar o exercicio a ollo, non para saltar
    // directamente á liña exacta que citou pdflatex.
    const texto = modelToBody(model)
    if (!texto) {
      const pre = el('pre', 'table-code')
      pre.textContent = t('table.code.empty', '(sen exercicios)')
      scrollEl.append(bar, search, pre)
      return
    }
    const ol = el('ol', 'table-code')
    for (const liña of texto.split('\n')) {
      const li = document.createElement('li')
      li.innerHTML = pintarLiñaConBusca(liña)
      ol.appendChild(li)
    }
    scrollEl.append(bar, search, ol)
    if (buscaAberta) {
      actualizarBusca()
      // requestAnimationFrame: o <input> acaba de crearse neste render, e
      // rerenderízase enteiro en CADA tecla (buscaTexto cambia) - sen isto
      // o foco pérdese despois do primeiro carácter escrito.
      requestAnimationFrame(() => {
        inputBusca.focus()
        inputBusca.setSelectionRange(inputBusca.value.length, inputBusca.value.length)
      })
    }
  }

  // =========================================================================
  //  OPERACIÓNS
  // =========================================================================
  function moveExercise(from, to) {
    if (to < 0 || to >= model.exercicios.length || from === to) return
    pushUndo()
    const [x] = model.exercicios.splice(from, 1)
    model.exercicios.splice(to, 0, x)
    markDirty(); render()
  }
  function duplicateExercise(i) {
    pushUndo()
    const src = model.exercicios[i]
    const clone = JSON.parse(JSON.stringify(src))
    clone.id = makeExercise().id
    model.exercicios.splice(i + 1, 0, clone)
    markDirty(); render()
  }
  async function deleteExercise(i) {
    if (!await confirmar(t('table.delete.confirm', 'Eliminar o exercicio {n}?', { n: i + 1 }))) return
    pushUndo()
    model.exercicios.splice(i, 1)
    markDirty(); render()
  }

  function cambiarModoVar(v, novo) {
    v.modo = novo
    if (novo === 'fixa') { v.valor = v.valor ?? ''; delete v.expr; delete v.xer; delete v.min; delete v.max; delete v.paso; delete v.defOp }
    else if (novo === 'auto') { v.expr = v.expr ?? ''; delete v.valor; delete v.xer; delete v.min; delete v.max; delete v.paso }
    else { v.xer = v.xer && (v.xer === 'rango' || XERADORES.includes(v.xer)) ? v.xer : 'n1'; delete v.valor; delete v.expr; delete v.defOp }
  }

  function insertToken(cellEl, ex, campo, tok) {
    const ta = cellEl.querySelector('textarea')
    const ins = '{' + tok + '}'
    if (ta) {
      const s = ta.selectionStart ?? ta.value.length
      const e = ta.selectionEnd ?? ta.value.length
      ta.value = ta.value.slice(0, s) + ins + ta.value.slice(e)
      ta.focus()
      ta.selectionStart = ta.selectionEnd = s + ins.length
      autoGrow(ta)
      ex[campo] = ta.value
    } else {
      ex[campo] = (ex[campo] || '') + ins
    }
    markDirty()
  }

  // ---------- asistente ----------
  // reorganizarDoc: constrúe (se main.js pasou crearReorganizar) o bloque
  // `reorganizar` que activa a barra "Reorganizar práctica" do asistente.
  // Sempre traballa co documento ENTEIRO (documentToText); `soIndice` só
  // acouta que exercicio se reconstrúe.
  async function reorganizarDoc(soIndice) {
    if (typeof crearReorganizar !== 'function') return null
    return crearReorganizar({
      getSource: () => documentToText(commitModel()),
      aplicar: (txt) => { aplicarTextoComoModelo(txt); render() },
      soIndice,
    })
  }

  async function abrirAsistenteDocumento() {
    const reorganizar = await reorganizarDoc(null)
    abrirAsistente({
      tipo: 'documento',
      title: t('table.assistant.docTitle', 'Asistente — táboa completa'),
      getContent: () => modelToBody(model),
      setContent: (txt) => { aplicarTextoComoModelo(txt); render() },
      reorganizar,
    })
  }
  async function abrirAsistenteExercicio(i) {
    const reorganizar = await reorganizarDoc(i)
    abrirAsistente({
      tipo: 'documento',
      title: t('table.assistant.exTitle', 'Asistente — exercicio {n}', { n: i + 1 }),
      getContent: () => bodyDunExercicio(model.exercicios[i]),
      setContent: (txt) => {
        const doc = parseDocument(wrapEx(txt))
        const ex0 = doc.model.exercicios[0]
        if (ex0) { ex0.id = model.exercicios[i].id; model.exercicios[i] = ex0; markDirty(); render() }
      },
      reorganizar,
    })
  }
  function aplicarTextoComoModelo(txt) {
    const doc = parseDocument(hasEx(txt) ? txt : wrapEx(txt))
    model = doc.model
    markDirty()
  }

  // rexenerarResolucion: botón ↻ IA dunha fila. Pasa ESE exercicio (e só ese,
  // soIndice) polo mesmo motor ca "Reorganizar práctica": a IA devolve a
  // resposta final coma fórmulas e a resolución paso a paso normalizada, e
  // valídase en varias sementes. Ao rematar, reescríbese só ese exercicio no
  // modelo. `btn` é o propio botón, para o estado "traballando".
  async function rexenerarResolucion(i, btn) {
    const ex = model.exercicios[i]
    if (!ex) return
    if (!String(ex.enunciado || '').trim()) {
      avisar(t('table.regenSol.senEnunciado', 'Escribe primeiro o enunciado do exercicio {n}.', { n: i + 1 }))
      return
    }
    const reorg = await reorganizarDoc(i)
    if (!reorg) return

    const textoPrevio = btn ? btn.textContent : ''
    if (btn) { btn.disabled = true; btn.classList.add('table-mini--busy'); btn.textContent = '⏳' }
    try {
      const res = await reorg.correr({ ...reorg.defaults, onProgreso: () => {} })
      if (res && res.texto) reorg.aplicar(res.texto)
      const inf = ((res && res.informe) || []).find((x) => x.indice === i)
      if (inf && (inf.estado === 'falla' || inf.estado === 'erro')) {
        avisar(t('table.regenSol.fallaValidacion', 'Xerouse a resolución do exercicio {n}, pero a validación aínda atopa valores incoherentes (infinito/indeterminado). Revísao.', { n: i + 1 }))
      } else if (inf && inf.estado === 'sen_formula') {
        avisar(t('table.regenSol.senFormula', 'A IA considera que o exercicio {n} non ten un resultado calculable cunha fórmula (demostración, resposta aberta...): deixouse como estaba.', { n: i + 1 }))
      }
    } catch (err) {
      avisar(t('table.regenSol.erro', 'Non se puido rexenerar a resolución: {msg}', { msg: err && err.message ? err.message : String(err) }))
    } finally {
      if (btn) { btn.disabled = false; btn.classList.remove('table-mini--busy'); btn.textContent = textoPrevio }
    }
  }

  // ---------- xeración con IA (exercicio solto / exame completo) ----------
  // A IA devolve xa a estrutura da táboa (taboa_ia.go / XerarExercicioTaboa /
  // XerarExameTaboa): un obxecto {enunciado, variables, resultado, resolucion}
  // por exercicio. exercicioDesdeTaboa só o normaliza ao modelo interno. Así
  // cada lapela ("Enunciados" / "Resultado" / "Resolución") amosa
  // exactamente o que lle toca, sen heurísticas.
  const s = (x) => (x == null ? '' : String(x))

  // evalIdsToTokens: <EVAL>identificador</EVAL> -> {identificador} (editable
  // coma token). Deixa os <EVAL> con expresión, e non toca as chaves que xa
  // houbese ({detA} que puxese a IA queda tal cal).
  function evalIdsToTokens(text) {
    return s(text).replace(/<EVAL>([\s\S]*?)<\/EVAL>/g, (m, inner) => {
      const id = inner.trim()
      return /^[A-Za-z_]\w*$/.test(id) ? `{${id}}` : m
    })
  }

  function varDesdeIA(v) {
    if (!v || !s(v.nome).trim()) return null
    const modo = MODOS_VARIABLE.includes(v.modo) ? v.modo : 'aleatoria'
    const out = makeVariable(s(v.nome).trim(), modo)
    if (modo === 'fixa') out.valor = s(v.valor)
    else if (modo === 'auto') out.expr = s(v.expr)
    else if (v.xer === 'rango') {
      out.xer = 'rango'
      out.min = s(v.min) || '1'
      out.max = s(v.max) || '10'
      out.paso = s(v.paso) || '1'
    } else {
      out.xer = XERADORES.includes(v.xer) ? v.xer : 'n1'
    }
    return out
  }

  function exercicioDesdeTaboa(raw, id) {
    const ex = makeExercise()
    if (id) ex.id = id
    raw = raw || {}
    ex.enunciado = evalIdsToTokens(raw.enunciado).trim()
    ex.resposta = evalIdsToTokens(raw.resultado).trim()
    ex.solucion = evalIdsToTokens(raw.resolucion).trim()
    for (const v of raw.variables || []) {
      const nv = varDesdeIA(v)
      if (nv) ex.variables.push(nv)
    }
    return ex
  }

  function abrirAsistenteExercicioIA() {
    openExerciseAIModal({
      t,
      modoTaboa: true,
      generateExerciseWithAI,
      onGenerateResult,
      onCreateExercise: (raw) => {
        pushUndo()
        model.exercicios.push(exercicioDesdeTaboa(raw))
        markDirty(); render()
        return model.exercicios.length - 1
      },
      onReplaceExercise: (index, raw) => {
        pushUndo()
        if (index == null || !model.exercicios[index]) {
          model.exercicios.push(exercicioDesdeTaboa(raw))
          markDirty(); render()
          return model.exercicios.length - 1
        }
        model.exercicios[index] = exercicioDesdeTaboa(raw, model.exercicios[index].id)
        markDirty(); render()
        return index
      },
    })
  }

  function abrirAsistenteExameIA() {
    openExamAIModal({
      t,
      modoTaboa: true,
      generateExamWithAI,
      // Xeración por lotes (guión + un anaco por chamada, ver taboa_ia.go):
      // a modal insire cada anaco en canto chega con onSpliceExercises.
      startExamWithAI,
      generateExamChunkWithAI,
      onSpliceExercises: (start, deleteCount, lista) => {
        const total = model.exercicios.length
        const desde = start == null ? total : Math.max(0, Math.min(start, total))
        const quitar = Math.max(0, Math.min(deleteCount || 0, total - desde))
        const novos = (lista || []).map((raw) => exercicioDesdeTaboa(raw))
        // Chamada só para saber onde inseriría (a fin do exame): non toca nada.
        if (!quitar && !novos.length) return { start: desde, count: 0 }
        pushUndo()
        model.exercicios.splice(desde, quitar, ...novos)
        markDirty(); render()
        return { start: desde, count: novos.length }
      },
      onGenerateResult,
      onCreateExercises: (lista) => {
        if (!lista || !lista.length) return null
        pushUndo()
        const start = model.exercicios.length
        for (const raw of lista) model.exercicios.push(exercicioDesdeTaboa(raw))
        markDirty(); render()
        return { start, count: lista.length }
      },
      onReplaceExercises: (start, count, lista) => {
        if (!lista || !lista.length) return { start, count: 0 }
        pushUndo()
        if (start == null || start < 0 || start + count > model.exercicios.length) {
          const desde = model.exercicios.length
          for (const raw of lista) model.exercicios.push(exercicioDesdeTaboa(raw))
          markDirty(); render()
          return { start: desde, count: lista.length }
        }
        model.exercicios.splice(start, count, ...lista.map((raw) => exercicioDesdeTaboa(raw)))
        markDirty(); render()
        return { start, count: lista.length }
      },
    })
  }

  // =========================================================================
  //  MODO (lapelas)
  // =========================================================================
  function setModo(novo) {
    if (novo === modo) return
    modo = novo
    if (codeView && novo === 'proba') { codeView = null; buscaAberta = false; buscaTexto = '' }
    render()
    onModoChangeCb && onModoChangeCb(modo)
  }

  function toggleCodeView() {
    codeView = codeView ? null : true
    if (!codeView) { buscaAberta = false; buscaTexto = '' }
    render()
  }

  // =========================================================================
  //  CARGA / SERIALIZACIÓN
  // =========================================================================
  function loadDoc(text) {
    const { model: m, legacy: lg } = parseDocument(text || '')
    model = m
    legacy = lg
    originalText = text || ''
    dirty = false
    undoStack.length = 0
    redoStack.length = 0
    codeView = null
    buscaAberta = false
    buscaTexto = ''
    render()
  }

  // NON hai recomputo automático ao editar: mentres se cambia un enunciado, un
  // valor de variable, o resultado ou a resolución, a previsualización da
  // dereita NON se toca (antes compilábase todo con Maxima+pdflatex tras unha
  // pausa de 1 s, o que "conxelaba" a interface e podía dar erros con
  // expresións a medio escribir). Agora o único disparo é o botón ✓ de cada
  // fila (onGenerateExercise), que recompila SÓ ese exercicio. Para a práctica
  // enteira segue estando o botón "Xerar" da barra.

  // sourceDunExercicio: .matex canónico cun único exercicio - o que se manda a
  // GeneratePDF cando se preme o ✓ dunha fila.
  function sourceDunExercicio(ex) {
    return documentToText({ v: 1, exercicios: [ex] })
  }

  // commitModel: o modelo tal cal está (as edicións inline xa o mutan en
  // vivo, non hai «flush» que facer coma en Blockly) - devólvese para
  // serializar.
  function commitModel() { return model }

  function currentText() {
    if (legacy && !dirty) return originalText
    return documentToText(model)
  }

  function markDirty() { dirty = true; legacy = false }

  function pushUndo() {
    undoStack.push(JSON.stringify(model))
    if (undoStack.length > 60) undoStack.shift()
    redoStack.length = 0
  }

  // =========================================================================
  //  HELPERS varios
  // =========================================================================
  function sugerirNome(ex) {
    const usados = new Set(ex.variables.map((v) => v.nome))
    for (const c of 'abcnmpqrstuvwxyz') if (!usados.has(c)) return c
    return 'v' + (ex.variables.length + 1)
  }
  function focarUltimoEnunciado() {
    const rows = scrollEl.querySelectorAll('.table-row:not(.table-row--head)')
    const last = rows[rows.length - 1]
    last && last.querySelector('textarea')?.focus()
  }
  function wireDrag(handle, row, i) {
    handle.addEventListener('dragstart', (e) => {
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData('text/plain', String(i))
      row.classList.add('table-row--drag')
    })
    handle.addEventListener('dragend', () => row.classList.remove('table-row--drag'))
    row.addEventListener('dragover', (e) => e.preventDefault())
    row.addEventListener('drop', (e) => {
      e.preventDefault()
      const from = Number(e.dataTransfer.getData('text/plain'))
      if (Number.isFinite(from)) moveExercise(from, i)
    })
  }
  function tokenChips(ex, onPick) {
    const box = el('div', 'table-tokens')
    const nomes = ex.variables.map((v) => v.nome).filter(Boolean)
    if (!nomes.length) return box
    const lab = el('span', 'table-tokens__lab')
    lab.textContent = t('table.tokens.label', 'inserir:')
    box.appendChild(lab)
    for (const n of nomes) {
      const c = el('button', 'table-token')
      c.type = 'button'
      c.textContent = '{' + n + '}'
      c.addEventListener('click', () => onPick(n))
      box.appendChild(c)
    }
    return box
  }
  function updateFullWidthBtn() {
    btnFullWidth.textContent = (fullWidth ? '⤡ ' : '⤢ ') +
      (fullWidth ? t('blocks.fullWidth.collapse', '⤡ Repregar').replace('⤡ ', '') : t('blocks.fullWidth.expand', '⤢ Ampliar').replace('⤢ ', ''))
  }
  updateFullWidthBtn()

  // =========================================================================
  //  API pública (contrato de main.js)
  // =========================================================================
  return {
    getValue: () => currentText(),
    setValue: (text) => loadDoc(text),
    focus: () => {
      // switchMode chama focus() xusto tras amosar #tableEditor: bo momento
      // para reaxustar as caixas que se renderizaron mentres estaba oculto.
      ajustarTA()
      scrollEl.querySelector('textarea, input, button')?.focus()
    },
    destroy: () => {
      container.innerHTML = ''
      container.classList.remove('table-editor')
    },
    undo: () => {
      if (!undoStack.length) return
      redoStack.push(JSON.stringify(model))
      model = JSON.parse(undoStack.pop())
      dirty = true
      render()
    },
    redo: () => {
      if (!redoStack.length) return
      undoStack.push(JSON.stringify(model))
      model = JSON.parse(redoStack.pop())
      dirty = true
      render()
    },
    getModo: () => modo,
    setModo: (m) => setModo(m),
    onModoChange: (cb) => { onModoChangeCb = cb },
    setFullWidthToggleVisible: (visible) => {
      if (!visible && fullWidth) {
        fullWidth = false
        updateFullWidthBtn()
        onToggleFullWidth && onToggleFullWidth(false)
      }
      btnFullWidth.style.display = visible ? '' : 'none'
    },
    // só para tests
    _modelForTests: () => model,
    _renderForTests: render,
    _exercicioDesdeTaboaForTests: exercicioDesdeTaboa,
  }
}

// ---------- utilidades DOM libres ----------
function el(tag, cls) {
  const n = document.createElement(tag)
  if (cls) n.className = cls
  return n
}
function cell(cls, text) {
  const c = el('div', 'table-cell ' + cls)
  if (text != null) c.textContent = text
  return c
}
// autoGrow: axusta a altura dun <textarea> ao seu contido (para que "se vexa
// todo o código" sen ter que arrastrar a esquina). Cap xeneroso: máis alá,
// scroll interno en vez dunha fila xigante. scrollHeight só é fiable co
// elemento xa no DOM - por iso render() fai unha pasada ao final (ajustarTA).
const TA_MAX_PX = 800
function autoGrow(ta) {
  ta.style.height = 'auto'
  ta.style.height = Math.min(ta.scrollHeight + 2, TA_MAX_PX) + 'px'
}
function textArea(value, ph, onInput) {
  const ta = el('textarea', 'table-ta')
  ta.rows = 1
  ta.value = value || ''
  ta.placeholder = ph
  ta.addEventListener('input', () => { onInput(ta.value); autoGrow(ta) })
  // Tamén ao pegar/enfocar (por se o contido chegou sen disparar 'input').
  ta.addEventListener('focus', () => autoGrow(ta))
  return ta
}
function inputBound(cls, value, ph, onInput) {
  const i = el('input', cls)
  i.type = 'text'
  i.value = value ?? ''
  i.placeholder = ph
  i.addEventListener('input', () => onInput(i.value))
  return i
}
function toolBtn(text, cls, onClick) {
  const b = el('button', 'table-tbtn' + (cls ? ' ' + cls : ''))
  b.type = 'button'
  if (text) b.textContent = text
  b.addEventListener('click', onClick)
  return b
}
// aiToolBtn: coma toolBtn pero coa icona ✨ e o degradado --ai-1/--ai-2
// (mesmo distintivo cós botóns de IA do editor de bloques).
function aiToolBtn(gradId, label, onClick) {
  const b = el('button', 'table-tbtn table-toolbar__ai')
  b.type = 'button'
  b.innerHTML = sparkSVG(gradId) + '<span></span>'
  b.querySelector('span').textContent = label
  b.addEventListener('click', onClick)
  return b
}
function miniBtn(glyph, title, onClick, extra) {
  const b = el('button', 'table-mini' + (extra ? ' ' + extra : ''))
  b.type = 'button'
  b.textContent = glyph
  b.title = title
  b.setAttribute('aria-label', title)
  b.addEventListener('click', onClick)
  return b
}
function aiMiniBtn(onClick) {
  const b = el('button', 'table-mini table-mini--ai')
  b.type = 'button'
  b.title = 'Asistente'
  b.setAttribute('aria-label', 'Asistente')
  b.innerHTML = sparkSVG('tblAiRow' + Math.random().toString(36).slice(2, 7))
  b.addEventListener('click', onClick)
  return b
}
function sparkSVG(gradId) {
  return `<svg viewBox="0 0 20 20" fill="url(#${gradId})"><defs><linearGradient id="${gradId}" x1="0" y1="0" x2="20" y2="20"><stop offset="0" stop-color="var(--ai-1)"/><stop offset="1" stop-color="var(--ai-2)"/></linearGradient></defs><path d="M10 2.5l1.6 4.4L16 8.5l-4.4 1.6L10 14.5l-1.6-4.4L4 8.5l4.4-1.6z"/></svg>`
}
function hasEx(txt) { return /<EX>[\s\S]*<\/EX>/.test(txt || '') }
function wrapEx(txt) { return `<EX>\n${txt}\n</EX>` }
function bodyDunExercicio(ex) {
  return modelToBody({ v: 1, exercicios: [ex] }).replace(/^<EX>\n?|\n?<\/EX>\n?$/g, '')
}
