  // Anacos da xeración por lotes: 2 exercicios por chamada e 3 chamadas á vez.
  // Un exercicio por chamada sería aínda máis rápido pero multiplica o texto
  // de contexto que se manda; con 2 o exame de 10 sae en 5 chamadas e dúas
  // quendas, e cada resposta é curta abondo para non quedar cortada.
  const EXERCICIOS_POR_LOTE = 2
  const LOTES_A_VEZ = 3

  const podeLotes = typeof startExamWithAI === 'function'
    && typeof generateExamChunkWithAI === 'function'
    && typeof onSpliceExercises === 'function'

  // Estado da xeración por lotes en curso (ou da última): a sesión do
  // backend, onde empezan os exercicios que creou esta modal e como quedou
  // cada anaco. É o que permite que "Continuar" só pida os que faltan.
  let lotes = null

  function firmaPeticion(tema, num, modeloFiles) {
    return JSON.stringify([tema, num, (modeloFiles || []).map((f) => `${f.name}:${f.size}`)])
  }
  function anacosPendentes() { return lotes ? lotes.anacos.filter((a) => !a.feito) : [] }
  function exerciciosFeitos() {
    return lotes ? lotes.anacos.reduce((n, a) => n + (a.feito ? a.count : 0), 0) : 0
  }
  function actualizarBotonXerar() {
    const faltan = anacosPendentes().length
    genBtn.textContent = faltan
      ? t('blocks.examAI.continue', 'Continuar (faltan {n})', { n: faltan })
      : t('blocks.examAI.generate', 'Xerar')
  }

  function mensaxeErro(err) { return err && err.message ? err.message : String(err) }

  // limparAnterior: "Xerar" nunca acumula versións, así que antes de empezar
  // de novo quítanse do exame os exercicios que creou a xeración anterior
  // DESTA modal (viñesen por lotes ou dunha soa chamada). Devolve a posición
  // onde empezar a poñer os novos.
  function limparAnterior() {
    const anterior = lotes ? { start: lotes.inicio, count: exerciciosFeitos() } : creadoRango
    lotes = null
    creadoRango = null
    if (anterior && anterior.start != null) {
      const r = onSpliceExercises(anterior.start, anterior.count || 0, [])
      if (r && r.start != null) return r.start
    }
    const r = onSpliceExercises(null, 0, [])
    return (r && r.start) || 0
  }

  function converterLista(exercicios) {
    return modoTaboa
      ? (exercicios || []) // a IA xa devolve un array de exercicios da táboa
      : (exercicios || []).map((anacos) => {
        const elements = (anacos || [])
          .filter((a) => a && ELEMENT_META[a.type] && a.type !== 'image-upload')
          .map((a) => ({ type: a.type, content: a.content }))
        return elements.length ? elements : [{ type: 'text', content: '' }]
      })
  }

  // ---- camiño clásico: unha soa chamada co exame enteiro ----
  async function xerarDunhaVez(tema, num, modeloFiles) {
    if (num === 0) {
      status.textContent = modeloFiles.length
        ? t('blocks.examAI.readingModelsUndefined', 'Lendo {n} PDF modelo e xerando o exame…', { n: modeloFiles.length })
        : t('blocks.examAI.generatingUndefined', 'Xerando o exame…')
    } else {
      status.textContent = modeloFiles.length
        ? t('blocks.examAI.readingModels', 'Lendo {n} PDF modelo e xerando {num} exercicios…', { n: modeloFiles.length, num })
        : t('blocks.examAI.generating', 'Xerando {num} exercicios…', { num })
    }
    const listaExercicios = converterLista(await generateExamWithAI(tema, num, modeloFiles))
    if (creadoRango == null || typeof onReplaceExercises !== 'function') {
      creadoRango = onCreateExercises(listaExercicios)
    } else {
      onReplaceExercises(creadoRango.start, creadoRango.count, listaExercicios)
      creadoRango = { start: creadoRango.start, count: listaExercicios.length }
    }
    await rematar()
    status.textContent = t('blocks.examAI.doneIterate', 'Feito. Axusta o tema ou o número e volve premer "Xerar" para refacelo.')
  }

  // ---- camiño por lotes: guión + un anaco por chamada ----
  async function xerarPorLotes(tema, num, modeloFiles) {
    const firma = firmaPeticion(tema, num, modeloFiles)
    const continuacion = lotes && lotes.firma === firma && anacosPendentes().length > 0
    if (!continuacion) {
      status.textContent = modeloFiles.length
        ? t('blocks.examAI.planningModels', 'Lendo {n} PDF modelo e planificando o exame…', { n: modeloFiles.length })
        : t('blocks.examAI.planning', 'Planificando o exame…')
      let plan = null
      try {
        plan = await startExamWithAI(tema, num, modeloFiles)
      } catch {
        plan = null
      }
      const guion = (plan && plan.guion) || []
      const inicio = limparAnterior()
      if (!plan || !plan.sesion || !guion.length) {
        // Sen guión non hai lotes que repartir: faise coma sempre, dunha soa
        // chamada (así un backend vello ou un modelo teimudo non deixan a
        // ninguén sen exame).
        return xerarDunhaVez(tema, num, modeloFiles)
      }
      const anacos = []
      for (let i = 0; i < guion.length; i += EXERCICIOS_POR_LOTE) {
        anacos.push({ desde: i, cantos: Math.min(EXERCICIOS_POR_LOTE, guion.length - i), feito: false, count: 0 })
      }
      lotes = { firma, sesion: plan.sesion, total: guion.length, inicio, anacos }
    }

    await executarAnacos()
    await executarAnacos() // segunda quenda: reintenta só o que fallase

    const feitos = exerciciosFeitos()
    await rematar()
    const faltan = anacosPendentes().length
    if (!faltan) {
      status.textContent = t('blocks.examAI.doneChunks', 'Feito: {n} exercicios. Axusta o tema ou o número e volve premer "Xerar" para refacelo.', { n: feitos })
    } else {
      status.textContent = t('blocks.examAI.partialChunks',
        'Xeráronse {feitos} de {total} exercicios; os que xa están quedan postos. Preme "Continuar" para pedir só os que faltan. ({msg})',
        { feitos, total: lotes.total, msg: lotes.ultimoErro || '' })
    }
  }

  // executarAnacos lanza LOTES_A_VEZ chamadas en paralelo e vai inserindo
  // cada anaco EN CANTO CHEGA, na súa posición (aínda que chegue antes ca
  // outro anterior): a posición é o inicio máis os exercicios xa inseridos
  // dos anacos previos.
  async function executarAnacos() {
    const pendentes = lotes.anacos.map((a, i) => i).filter((i) => !lotes.anacos[i].feito)
    if (!pendentes.length) return
    let seguinte = 0
    const progreso = () => {
      status.textContent = t('blocks.examAI.chunkProgress', 'Xerando exercicios… {feitos} de {total}', {
        feitos: exerciciosFeitos(), total: lotes.total,
      })
    }
    progreso()
    async function traballador() {
      while (seguinte < pendentes.length) {
        const anaco = lotes.anacos[pendentes[seguinte++]]
        try {
          const lista = converterLista(await generateExamChunkWithAI(lotes.sesion, anaco.desde, anaco.cantos))
          const pos = lotes.inicio + lotes.anacos
            .slice(0, lotes.anacos.indexOf(anaco))
            .reduce((n, a) => n + a.count, 0)
          const r = onSpliceExercises(pos, 0, lista)
          anaco.feito = true
          anaco.count = (r && r.count != null) ? r.count : lista.length
        } catch (err) {
          lotes.ultimoErro = mensaxeErro(err)
        }
        progreso()
      }
    }
    await Promise.all(Array.from({ length: Math.min(LOTES_A_VEZ, pendentes.length) }, traballador))
  }

  // rematar: gardar o prompt e compilar o documento para ver o resultado sen
  // saír da modal (o mesmo que se facía ao final da xeración dunha vez).
  async function rematar() {
    gardarMemo()
    if (typeof onGenerateResult === 'function') {
      status.textContent = t('blocks.examAI.generatingResult', 'Exercicios engadidos. Xerando o resultado…')
      await onGenerateResult()
    }
  }

  async function doGenerate() {
    const tema = textarea.value.trim()
    const modeloFiles = modeloPDF.files()
    if (!tema && modeloFiles.length === 0) {
      status.textContent = t('blocks.examAI.emptyRequest', 'Escribe o tema do exame ou xunta un PDF modelo.')
      return
    }
    const num = indefInput.checked ? 0 : (parseInt(numInput.value, 10) || 1)
    genBtn.disabled = true
    try {
      if (podeLotes) await xerarPorLotes(tema, num, modeloFiles)
      else await xerarDunhaVez(tema, num, modeloFiles)
    } catch (err) {
      status.textContent = t('blocks.examAI.error', 'Erro: {msg}', { msg: mensaxeErro(err) })
    } finally {
      genBtn.disabled = false
      actualizarBotonXerar()
    }
  }
// Modais de nivel "exame"/"exercicio" (non dun anaco solto, ver
// blocks-modal.js): "✨ Exercicio con IA", "✨ Exame completo con IA" e
// "💾 Gardar na biblioteca". Extraídas case literais do antigo blocks.js -
// só cambia que en vez de mutar `doc.exercises` e chamar `render()`
// directamente, devolven o resultado por callback (onCreateExercise/
// onCreateExercises) para que blocks-editor.js decida como convertelo en
// bloques Blockly.
//
// As dúas modais de xeración con IA ("Exercicio"/"Exame") comparten agora:
//   - ventá ampla (.modal--wide) e movible arrastrando o título
//     (makeDraggable), para poder apartala e ver o que hai detrás;
//   - unha "i" na cabeceira que amosa/agocha os textos de axuda (no canto
//     de telos sempre á vista);
//   - unha "X" na cabeceira para pechar; NON se pechan ao premer fóra;
//   - o botón "Xerar" NON pecha a ventá: a primeira vez crea o
//     exercicio/exame, e cada vez seguinte SUBSTITÚE o que creou a anterior
//     (onReplaceExercise/onReplaceExercises), para poder afinar o prompt sen
//     reabrir a interface;
//   - o texto do prompt lémbrase mentres Yang estea aberto (memo a nivel de
//     módulo).
//
// Nota: a modal "📚 Biblioteca" (listar/inserir gardado) NON se porta aquí
// - no editor antigo xa non tiña ningunha entrada activa na UI (o botón de
// toolbar quitouse a propósito, ver blocks-library.test.js), só quedaban
// os botóns "💾 Gardar" individuais (que si se portan). Reconectala sería
// engadir funcionalidade nova, non recuperar paridade.
import { elementMeta } from './blocks-meta.js'
import { attachAutoClose, makeDraggable } from './blocks-modal.js'

let saveLibraryCtx = null

// Memo do prompt: vive mentres Yang estea aberto (recárgase ao reabrir a
// modal, pérdese ao pechar Yang). Ver "Lembrar prompt" no plan.
let exerciseAIMemo = { peticion: '' }
let examAIMemo = { tema: '', num: '5', indef: false }

function ensureOverlay(cache, extraClass, { closeOnOutsideClick = true } = {}) {
  return () => {
    if (cache.el) return cache.el
    const overlay = document.createElement('div')
    overlay.className = 'overlay hidden'
    const modal = document.createElement('div')
    modal.className = 'modal' + (extraClass ? ' ' + extraClass : '')
    overlay.appendChild(modal)
    // As modais de IA non se pechan ao premer fóra (péchase o traballo sen
    // querer): só coa "X" ou con "Cancelar".
    if (closeOnOutsideClick) {
      overlay.addEventListener('click', (e) => { if (e.target === overlay) cache.close() })
    }
    document.body.appendChild(overlay)
    cache.el = overlay
    return overlay
  }
}

// prepararArrastre: reset da posición dunha apertura anterior + reengancha
// makeDraggable ao título (mesmo criterio ca openContentModal en
// blocks-modal.js, para que cada apertura empece centrada).
function prepararArrastre(modal, h2) {
  modal.style.position = ''
  modal.style.left = ''
  modal.style.top = ''
  modal.style.margin = ''
  h2.classList.add('modal-drag-handle')
  makeDraggable(modal, h2)
}

// montarCabeceira devolve o <h2> (asa de arrastre) xa cos botóns "i" (amosa/
// agocha `caixaAxuda`), "Desacoplar"/"Acoplar" e "X" (pechar), máis a propia
// `caixaAxuda` para que o chamador a coloque no corpo, xusto debaixo do
// título.
//   - detached=false (modal integrada): "⧉ Desacoplar" (só se hai
//     onDesacoplar) + "✕" (onClose).
//   - detached=true (xanela á parte, ver xanelaxeradoria.go): "⇤ Acoplar"
//     (onAcoplar) no canto do "✕"; non hai "Desacoplar".
function montarCabeceira({ t, tituloHTML, parrafosAxuda, detached, onClose, onAcoplar, onDesacoplar }) {
  const h2 = document.createElement('h2')
  h2.className = 'modal-header--ai'

  const titulo = document.createElement('span')
  titulo.className = 'modal-header__title'
  titulo.innerHTML = tituloHTML
  h2.appendChild(titulo)

  const caixaAxuda = document.createElement('div')
  caixaAxuda.className = 'modal-info'
  caixaAxuda.hidden = true
  for (const texto of parrafosAxuda) {
    if (!texto) continue
    const p = document.createElement('p')
    p.textContent = texto
    caixaAxuda.appendChild(p)
  }

  const infoBtn = document.createElement('button')
  infoBtn.type = 'button'
  infoBtn.className = 'modal-info-toggle'
  infoBtn.textContent = 'ℹ'
  infoBtn.setAttribute('aria-expanded', 'false')
  infoBtn.title = t('blocks.modal.infoToggle', 'Máis información')
  infoBtn.setAttribute('aria-label', t('blocks.modal.infoToggle', 'Máis información'))
  infoBtn.addEventListener('click', () => {
    caixaAxuda.hidden = !caixaAxuda.hidden
    infoBtn.setAttribute('aria-expanded', String(!caixaAxuda.hidden))
  })
  h2.appendChild(infoBtn)

  if (detached) {
    const dockBtn = document.createElement('button')
    dockBtn.type = 'button'
    dockBtn.className = 'modal-detach'
    dockBtn.textContent = '⇤'
    dockBtn.title = t('blocks.modal.dock', 'Acoplar')
    dockBtn.setAttribute('aria-label', t('blocks.modal.dock', 'Acoplar'))
    dockBtn.addEventListener('click', () => onAcoplar && onAcoplar())
    h2.appendChild(dockBtn)
  } else {
    if (onDesacoplar) {
      const detachBtn = document.createElement('button')
      detachBtn.type = 'button'
      detachBtn.className = 'modal-detach'
      detachBtn.textContent = '⧉'
      detachBtn.title = t('blocks.modal.detach', 'Desacoplar nunha xanela á parte')
      detachBtn.setAttribute('aria-label', t('blocks.modal.detach', 'Desacoplar nunha xanela á parte'))
      detachBtn.addEventListener('click', () => onDesacoplar())
      h2.appendChild(detachBtn)
    }
    const closeBtn = document.createElement('button')
    closeBtn.type = 'button'
    closeBtn.className = 'modal-close'
    closeBtn.textContent = '✕'
    closeBtn.title = t('blocks.modal.close', 'Pechar')
    closeBtn.setAttribute('aria-label', t('blocks.modal.close', 'Pechar'))
    closeBtn.addEventListener('click', onClose)
    h2.appendChild(closeBtn)
  }

  return { h2, caixaAxuda }
}

// montarFilaModeloPDF: fila "Xuntar PDF(s) modelo (opcional)" - idéntica
// para "Exercicio con IA" e "Exame completo con IA". Reutiliza as claves
// i18n de examAI.*. Devolve o wrap para engadir ao modal e files() co array
// actual de File.
function montarFilaModeloPDF(t) {
  const wrap = document.createElement('div')
  wrap.className = 'image-upload-row'

  const status = document.createElement('span')
  status.className = 'image-upload-row__status'
  status.textContent = t('blocks.examAI.noModel', 'Ningún PDF modelo escollido.')
  wrap.appendChild(status)

  const label = document.createElement('label')
  label.className = 'image-upload-row__btn'
  label.textContent = t('blocks.examAI.attachModel', 'Xuntar PDF(s) modelo (opcional, escolle varios con Ctrl+clic)…')

  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'application/pdf'
  input.multiple = true
  input.hidden = true
  input.addEventListener('change', () => {
    const n = input.files ? input.files.length : 0
    status.textContent = n === 0
      ? t('blocks.examAI.noModel', 'Ningún PDF modelo escollido.')
      : n === 1
        ? t('blocks.examAI.oneModel', 'Modelo: {name}', { name: input.files[0].name })
        : t('blocks.examAI.severalModels', '{n} PDF modelo escollidos.', { n })
  })
  label.appendChild(input)
  wrap.appendChild(label)

  return { wrap, files: () => Array.from(input.files || []) }
}

// aiSparkleTitleSVG: mesma icona (✨) que xa levaban os títulos das dúas
// modais.
const aiSparkleTitleSVG = '<svg viewBox="0 0 20 20" fill="var(--ai-1)" aria-hidden="true" style="width:16px;height:16px;vertical-align:-2px;margin-right:5px;"><path d="M10 2.5l1.6 4.4L16 8.5l-4.4 1.6L10 14.5l-1.6-4.4L4 8.5l4.4-1.6z"/></svg>'

// ---------- "✨ Exercicio con IA" ----------

const exerciseAI = { el: null, close: () => { if (exerciseAI.el) exerciseAI.el.classList.add('hidden') } }
const ensureExerciseAIModal = ensureOverlay(exerciseAI, null, { closeOnOutsideClick: false })

export function openExerciseAIModal({
  t, generateExerciseWithAI, onCreateExercise, onReplaceExercise, onGenerateResult,
  detached = false, initialState, onDesacoplar, onAcoplar, modoTaboa = false,
}) {
  const ELEMENT_META = elementMeta(t)
  const overlay = ensureExerciseAIModal()
  const modal = overlay.querySelector('.modal')
  modal.innerHTML = ''
  modal.className = 'modal modal--wide modal--ai-generate'

  // Índice do exercicio que creou ESTE diálogo. A primeira "Xerar" créao;
  // as seguintes substitúeno (onReplaceExercise) para poder afinar o prompt.
  let creado = null

  function estadoActual() { return { peticion: textarea.value } }
  function gardarMemo() { exerciseAIMemo.peticion = textarea.value }
  // Integrada: "✕"/"Cancelar" agochan o overlay. Desacoplada: "Acoplar" (e
  // "Cancelar" non existe) pecha a xanela do sistema vía onAcoplar.
  function pechar() {
    gardarMemo()
    if (detached) { if (onAcoplar) onAcoplar() }
    else exerciseAI.close()
  }

  const { h2, caixaAxuda } = montarCabeceira({
    t,
    tituloHTML: aiSparkleTitleSVG + t('blocks.exerciseAI.title', 'Exercicio con IA'),
    parrafosAxuda: [
      t('blocks.exerciseAI.note', 'A IA crea o enunciado e os anacos que faga falta (fórmula, gráfico...) xa colocados; podes revisalos e editalos coma calquera bloque despois.'),
      t('blocks.exerciseAI.modelNote', 'Se xuntas un ou varios PDF (por exemplo, un exercicio dun exame anterior), a IA tómaos coma modelo de tema, formato e dificultade para crear un exercicio DERIVADO - non unha copia. Con PDF xuntado xa non fai falla escribir a descrición.'),
    ],
    detached,
    onClose: pechar,
    onAcoplar: pechar,
    onDesacoplar: onDesacoplar
      ? async () => { await onDesacoplar(estadoActual()); exerciseAI.close() }
      : null,
  })
  // Na xanela á parte a modal ocupa todo: non ten sentido arrastrala.
  if (!detached) prepararArrastre(modal, h2)
  modal.appendChild(h2)
  modal.appendChild(caixaAxuda)

  const label = document.createElement('label')
  label.className = 'modal-label'
  label.textContent = t('blocks.exerciseAI.describeLabel', 'Describe o exercicio que queres')
  const textarea = document.createElement('textarea')
  textarea.rows = 8
  textarea.placeholder = t('blocks.exerciseAI.placeholder', 'Ex.: "a derivada dunha función cadrática, con gráfico"')
  textarea.value = (initialState && initialState.peticion) || exerciseAIMemo.peticion
  attachAutoClose(textarea)
  label.appendChild(textarea)
  modal.appendChild(label)

  const modeloPDF = montarFilaModeloPDF(t)
  modal.appendChild(modeloPDF.wrap)

  const status = document.createElement('p')
  status.className = 'ai-row__status'
  modal.appendChild(status)

  const actions = document.createElement('div')
  actions.className = 'modal-actions'
  const cancelBtn = document.createElement('button')
  cancelBtn.type = 'button'
  cancelBtn.textContent = t('blocks.modal.cancel', 'Cancelar')
  cancelBtn.addEventListener('click', pechar)
  const genBtn = document.createElement('button')
  genBtn.type = 'button'
  genBtn.className = 'primary'
  genBtn.textContent = t('blocks.exerciseAI.generate', 'Xerar')
  async function doGenerate() {
    const peticion = textarea.value.trim()
    const modeloFiles = modeloPDF.files()
    if (!peticion && modeloFiles.length === 0) {
      status.textContent = t('blocks.exerciseAI.emptyRequest', 'Escribe que exercicio queres ou xunta un PDF modelo.')
      return
    }
    genBtn.disabled = true
    status.textContent = modeloFiles.length
      ? t('blocks.exerciseAI.readingModels', 'Lendo {n} PDF modelo e xerando o exercicio…', { n: modeloFiles.length })
      : t('blocks.exerciseAI.generating', 'Xerando…')
    try {
      const bruto = await generateExerciseWithAI(peticion, modeloFiles)
      // modoTaboa: a IA xa devolve a estrutura da táboa (obxecto), pásase
      // tal cal; se non, é a lista de "anacos" do editor de bloques.
      let finais
      if (modoTaboa) {
        finais = bruto
      } else {
        const elements = (bruto || [])
          .filter((a) => a && ELEMENT_META[a.type] && a.type !== 'image-upload')
          .map((a) => ({ type: a.type, content: a.content }))
        finais = elements.length ? elements : [{ type: 'text', content: '' }]
      }
      if (creado == null || typeof onReplaceExercise !== 'function') {
        creado = onCreateExercise(finais)
      } else {
        onReplaceExercise(creado, finais)
      }
      gardarMemo()
      // Ademais de inserir o exercicio, compilar o documento enteiro para
      // ver o resultado final sen saír da modal (mesmo que premer "Xerar" na
      // barra principal).
      if (typeof onGenerateResult === 'function') {
        status.textContent = t('blocks.exerciseAI.generatingResult', 'Exercicio engadido. Xerando o resultado…')
        await onGenerateResult()
      }
      status.textContent = t('blocks.exerciseAI.doneIterate', 'Feito. Edita a descrición e volve premer "Xerar" para refacelo.')
    } catch (err) {
      status.textContent = t('blocks.exerciseAI.error', 'Erro: {msg}', { msg: err && err.message ? err.message : err })
    } finally {
      genBtn.disabled = false
    }
  }
  genBtn.addEventListener('click', doGenerate)
  textarea.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); doGenerate() }
  })
  if (!detached) actions.appendChild(cancelBtn)
  actions.appendChild(genBtn)
  modal.appendChild(actions)

  overlay.classList.remove('hidden')
  requestAnimationFrame(() => textarea.focus())
}

// ---------- "✨ Exame completo con IA" ----------

const examAI = { el: null, close: () => { if (examAI.el) examAI.el.classList.add('hidden') } }
const ensureExamAIModal = ensureOverlay(examAI, null, { closeOnOutsideClick: false })

export function openExamAIModal({
  t, generateExamWithAI, onCreateExercises, onReplaceExercises, onGenerateResult,
  detached = false, initialState, onDesacoplar, onAcoplar, modoTaboa = false,
  // Xeración POR LOTES (opcional, hoxe só a táboa: ver taboa_ia.go). Se veñen
  // as tres, o exame non se pide nunha soa chamada longa senón así:
  //   1. startExamWithAI(tema, num, modeloFiles) -> {sesion, guion[]}: só o
  //      guión (unha liña por exercicio). Chega en segundos e xa di cantos
  //      exercicios hai cando se marcou "indefinido".
  //   2. generateExamChunkWithAI(sesion, desde, cantos) -> exercicios dese
  //      anaco. Lánzanse varios á vez e cada un insírese en canto chega con
  //      onSpliceExercises(pos, 0, lista), na súa posición do exame.
  // Vantaxes: o tempo total é o do anaco máis lento (non a suma), o que xa
  // chegou queda posto aínda que falle outro anaco, e "Continuar" só volve
  // pedir os que faltan. Se falla o paso 1 vólvese ao camiño de sempre.
  startExamWithAI, generateExamChunkWithAI, onSpliceExercises,
}) {
  const ELEMENT_META = elementMeta(t)
  const overlay = ensureExamAIModal()
  const modal = overlay.querySelector('.modal')
  modal.innerHTML = ''
  modal.className = 'modal modal--wide modal--ai-generate'

  // Rango { start, count } dos exercicios que creou ESTE diálogo. A primeira
  // "Xerar" créaos; as seguintes substitúen ese rango (onReplaceExercises).
  let creadoRango = null

  function estadoActual() { return { tema: textarea.value, num: numInput.value, indef: indefInput.checked } }
  function gardarMemo() { examAIMemo = estadoActual() }
  function pechar() {
    gardarMemo()
    if (detached) { if (onAcoplar) onAcoplar() }
    else examAI.close()
  }

  const { h2, caixaAxuda } = montarCabeceira({
    t,
    tituloHTML: aiSparkleTitleSVG + t('blocks.examAI.title', 'Exame completo con IA'),
    parrafosAxuda: [
      t('blocks.examAI.note', 'A IA crea todos os exercicios de vez (enunciado, fórmulas, gráficos...) e engádeos ao final do exame; podes revisalos, reordenalos e editalos coma calquera outro despois.'),
      t('blocks.examAI.undefinedNote', 'Coa casa marcada non se lle impón número ningún: a IA axústase ao que pida o tema ou, se xuntas un modelo, ao número de exercicios que teña ese documento.'),
      t('blocks.examAI.modelNote', 'Se xuntas un ou varios PDF (por exemplo, un exame anterior), a IA tómaos coma modelo de tema, formato e dificultade para crear unha versión DERIVADA - non unha copia. Con PDF xuntado xa non fai falla escribir un tema.'),
    ],
    detached,
    onClose: pechar,
    onAcoplar: pechar,
    onDesacoplar: onDesacoplar
      ? async () => { await onDesacoplar(estadoActual()); examAI.close() }
      : null,
  })
  // Na xanela á parte a modal ocupa todo: non ten sentido arrastrala.
  if (!detached) prepararArrastre(modal, h2)
  modal.appendChild(h2)
  modal.appendChild(caixaAxuda)

  const numLabel = document.createElement('label')
  numLabel.className = 'modal-label'
  numLabel.textContent = t('blocks.examAI.numExercisesLabel', 'Número de exercicios')
  const numInput = document.createElement('input')
  numInput.type = 'number'
  numInput.min = '1'
  numInput.max = '20'
  numInput.value = (initialState && initialState.num) || examAIMemo.num || '5'
  numLabel.appendChild(numInput)
  modal.appendChild(numLabel)

  // "Indefinido": manda 0 ao backend, que é o que alí significa "decide ti
  // cantos" (XerarExameIA/systemPromptExame) - non hai tope de 20 nese caso.
  const indefLabel = document.createElement('label')
  indefLabel.className = 'modal-label modal-label-checkbox'
  const indefInput = document.createElement('input')
  indefInput.type = 'checkbox'
  const indefSpan = document.createElement('span')
  indefSpan.textContent = t('blocks.examAI.undefinedLabel', 'Indefinido: que decida a IA cantos fan falla')
  function sincronizarIndef() {
    numInput.disabled = indefInput.checked
    numLabel.style.opacity = indefInput.checked ? '0.5' : ''
  }
  indefInput.addEventListener('change', sincronizarIndef)
  indefInput.checked = initialState ? !!initialState.indef : !!examAIMemo.indef
  sincronizarIndef()
  indefLabel.appendChild(indefInput)
  indefLabel.appendChild(indefSpan)
  modal.appendChild(indefLabel)

  const temaLabel = document.createElement('label')
  temaLabel.className = 'modal-label'
  temaLabel.textContent = t('blocks.examAI.topicLabel', 'Tema do exame')
  const textarea = document.createElement('textarea')
  textarea.rows = 8
  textarea.placeholder = t('blocks.examAI.placeholder', 'Ex.: "derivadas e a súa aplicación a máximos e mínimos"')
  textarea.value = (initialState && initialState.tema) || examAIMemo.tema || ''
  attachAutoClose(textarea)
  temaLabel.appendChild(textarea)
  modal.appendChild(temaLabel)

  const modeloPDF = montarFilaModeloPDF(t)
  modal.appendChild(modeloPDF.wrap)

  const status = document.createElement('p')
  status.className = 'ai-row__status'
  modal.appendChild(status)

  const actions = document.createElement('div')
  actions.className = 'modal-actions'
  const cancelBtn = document.createElement('button')
  cancelBtn.type = 'button'
  cancelBtn.textContent = t('blocks.modal.cancel', 'Cancelar')
  cancelBtn.addEventListener('click', pechar)
  const genBtn = document.createElement('button')
  genBtn.type = 'button'
  genBtn.className = 'primary'
  genBtn.textContent = t('blocks.examAI.generate', 'Xerar')
  async function doGenerate() {
    const tema = textarea.value.trim()
    const modeloFiles = modeloPDF.files()
    if (!tema && modeloFiles.length === 0) {
      status.textContent = t('blocks.examAI.emptyRequest', 'Escribe o tema do exame ou xunta un PDF modelo.')
      return
    }
    const num = indefInput.checked ? 0 : (parseInt(numInput.value, 10) || 1)
    genBtn.disabled = true
    if (num === 0) {
      status.textContent = modeloFiles.length
        ? t('blocks.examAI.readingModelsUndefined', 'Lendo {n} PDF modelo e xerando o exame…', { n: modeloFiles.length })
        : t('blocks.examAI.generatingUndefined', 'Xerando o exame…')
    } else {
      status.textContent = modeloFiles.length
        ? t('blocks.examAI.readingModels', 'Lendo {n} PDF modelo e xerando {num} exercicios…', { n: modeloFiles.length, num })
        : t('blocks.examAI.generating', 'Xerando {num} exercicios…', { num })
    }
    try {
      const exercicios = await generateExamWithAI(tema, num, modeloFiles)
      const listaExercicios = modoTaboa
        ? (exercicios || []) // a IA xa devolve un array de exercicios da táboa
        : (exercicios || []).map((anacos) => {
          const elements = (anacos || [])
            .filter((a) => a && ELEMENT_META[a.type] && a.type !== 'image-upload')
            .map((a) => ({ type: a.type, content: a.content }))
          return elements.length ? elements : [{ type: 'text', content: '' }]
        })
      if (creadoRango == null || typeof onReplaceExercises !== 'function') {
        creadoRango = onCreateExercises(listaExercicios)
      } else {
        onReplaceExercises(creadoRango.start, creadoRango.count, listaExercicios)
        creadoRango = { start: creadoRango.start, count: listaExercicios.length }
      }
      gardarMemo()
      // Ademais de inserir os exercicios, compilar o documento enteiro para
      // ver o resultado final sen saír da modal.
      if (typeof onGenerateResult === 'function') {
        status.textContent = t('blocks.examAI.generatingResult', 'Exercicios engadidos. Xerando o resultado…')
        await onGenerateResult()
      }
      status.textContent = t('blocks.examAI.doneIterate', 'Feito. Axusta o tema ou o número e volve premer "Xerar" para refacelo.')
    } catch (err) {
      status.textContent = t('blocks.examAI.error', 'Erro: {msg}', { msg: err && err.message ? err.message : err })
    } finally {
      genBtn.disabled = false
    }
  }
  genBtn.addEventListener('click', doGenerate)
  if (!detached) actions.appendChild(cancelBtn)
  actions.appendChild(genBtn)
  modal.appendChild(actions)

  overlay.classList.remove('hidden')
  requestAnimationFrame(() => (indefInput.checked ? textarea : numInput).focus())
}

// ---------- "💾 Gardar na biblioteca" ----------
// Un só modal para "exercicio"/"bloque" (o antigo tamén admitía "exame",
// pero sen botón activo que o chamase - ver comentario de cabeceira). O
// chamador xa serializou o contido a texto .matex cru (elementsToText),
// aquí só falla pedir un nome.

const saveLibrary = { el: null, close: () => { if (saveLibrary.el) saveLibrary.el.classList.add('hidden'); saveLibraryCtx = null } }
const ensureSaveLibraryModal = ensureOverlay(saveLibrary)

export function openSaveLibraryModal({ t, tipo, contido, saveToLibrary }) {
  const SAVE_LIBRARY_TITLES = {
    exame: t('blocks.saveLibrary.type.exame', 'exame'),
    exercicio: t('blocks.saveLibrary.type.exercicio', 'exercicio'),
    bloque: t('blocks.saveLibrary.type.bloque', 'bloque'),
  }
  saveLibraryCtx = { tipo, contido }
  const overlay = ensureSaveLibraryModal()
  const modal = overlay.querySelector('.modal')
  modal.innerHTML = ''

  const h2 = document.createElement('h2')
  h2.innerHTML = `<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" style="width:16px;height:16px;vertical-align:-2px;margin-right:5px;"><path d="M4 3.5h9l3 3v10a.5.5 0 0 1-.5.5h-11.5a.5.5 0 0 1-.5-.5v-12.5a.5.5 0 0 1 .5-.5z"/><path d="M6.5 3.5v4h6v-4"/><path d="M6 13h8"/></svg>${t('blocks.saveLibrary.title', 'Gardar {tipo} na biblioteca', { tipo: SAVE_LIBRARY_TITLES[tipo] || tipo })}`
  modal.appendChild(h2)

  const label = document.createElement('label')
  label.className = 'modal-label'
  label.textContent = t('blocks.saveLibrary.nameLabel', 'Nome')
  const input = document.createElement('input')
  input.type = 'text'
  input.placeholder = t('blocks.saveLibrary.placeholder', 'Ex.: "Cabeceira IES", "Exercicio derivadas"...')
  label.appendChild(input)
  modal.appendChild(label)

  const status = document.createElement('p')
  status.className = 'ai-row__status'
  modal.appendChild(status)

  const actions = document.createElement('div')
  actions.className = 'modal-actions'
  const cancelBtn = document.createElement('button')
  cancelBtn.type = 'button'
  cancelBtn.textContent = t('blocks.modal.cancel', 'Cancelar')
  cancelBtn.addEventListener('click', saveLibrary.close)
  const saveBtn = document.createElement('button')
  saveBtn.type = 'button'
  saveBtn.className = 'primary'
  saveBtn.textContent = t('blocks.saveLibrary.save', 'Gardar')
  async function doSave() {
    const nome = input.value.trim()
    if (!nome) { status.textContent = t('blocks.saveLibrary.emptyName', 'Dálle un nome.'); return }
    saveBtn.disabled = true
    try {
      await saveToLibrary(nome, saveLibraryCtx.tipo, saveLibraryCtx.contido)
      saveLibrary.close()
    } catch (err) {
      status.textContent = t('blocks.saveLibrary.error', 'Erro: {msg}', { msg: err && err.message ? err.message : err })
    } finally {
      saveBtn.disabled = false
    }
  }
  saveBtn.addEventListener('click', doSave)
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); doSave() }
  })
  actions.appendChild(cancelBtn)
  actions.appendChild(saveBtn)
  modal.appendChild(actions)

  overlay.classList.remove('hidden')
  requestAnimationFrame(() => input.focus())
}
