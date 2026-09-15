// Editor de exames por bloques, estilo Snap!/Scratch: un lenzo onde cada
// bloque é un EXERCICIO enteiro, arrastrable para reordenar; á esquerda hai
// unha PALETA fixa con todas as instrucións agrupadas por temática — non hai
// que lembrar nomes, arrástrase (ou preméuse, con exercicio aberto) o bloque
// desexado ata o exercicio. Premer en calquera bloque xa colocado abre unha
// ventá modal para introducir/editar o seu contido (texto, fórmula, imaxe).
// Cada anaco mapea 1:1 cunha etiqueta Matexe (ver blocks-serialize.js);
// serializar o lenzo a texto é o que se envía a Generate/SaveFile/
// GeneratePDF — este módulo non fala nunca con Maxima nin cos bindings
// Wails directamente (a subida de imaxes chega inxectada coma callback
// `uploadImage`, e a xeración por IA coma `generateWithAI`, resoltos por
// main.js). O modal reutiliza as clases .overlay/.modal/.modal-label/
// .modal-actions xa definidas en style.css (as mesmas que usan
// optionsOverlay/maximaManualOverlay en index.html).
//
// i18n: todo o texto visible pasa por `t(clave, fallback, vars)`, inxectado
// dende main.js (mesma función que traduce o HTML estático, ver
// aplicarTraducions) - blocks.js non importa os dicionarios directamente
// para non duplicar esa lóxica. `t` por defecto (parámetro `t` opcional en
// createBlocksEditor) devolve sempre o fallback, así os tests (blocks-
// library.test.js) que non pasan `t` seguen a funcionar sen cambios.
import { parseDocument, documentToText, makeExercise, makeElement, parseElements, elementsToText } from './blocks-serialize.js'

function elementMeta(t) {
  return {
    text: {
      label: t('blocks.element.text.label', 'Enunciado'), ico: '📝',
      def: t('blocks.element.text.def', 'Escribe aquí o enunciado…'),
    },
    variables: {
      label: t('blocks.element.variables.label', 'Variable / cálculo'), ico: '🔢',
      def: 'a: 1',
      note: t('blocks.element.variables.note', 'Execútase en silencio (Maxima), sen amosar nada no exame. Varias expresións sepáranse con ; ou $.'),
    },
    formula: {
      label: t('blocks.element.formula.label', 'Fórmula'), ico: '🧮',
      def: 'f(3)',
      note: t('blocks.element.formula.note', 'Maxima avalía a expresión e o resultado aparece no exame.'),
    },
    'image-plot': {
      label: t('blocks.element.imagePlot.label', 'Gráfico'), ico: '📈',
      def: 'plot2d(f(x),[x,-2,2])',
      note: t('blocks.element.imagePlot.note', 'Xera unha imaxe co gráfico da función.'),
    },
    'image-tikz': {
      label: t('blocks.element.imageTikz.label', 'Debuxo TikZ'), ico: '✏️',
      def: '\\draw (0,0) -- (1,1) -- (1,0) -- cycle;',
      note: t('blocks.element.imageTikz.note', 'Código TikZ: debúxase con LaTeX/pgf, non con Maxima.'),
    },
    'image-upload': {
      label: t('blocks.element.imageUpload.label', 'Imaxe'), ico: '🖼️',
      def: '',
      note: t('blocks.element.imageUpload.note', 'Imaxe do disco, inserida no exame.'),
    },
  }
}

// attachAutoClose engade autopechado de ()/[]/{}/""/'' a un <textarea> plano
// (a diferenza do editor de texto, que usa CodeMirror e xa ten isto via
// closeBrackets() en editor.js) - "sen ter que escribir as aspas" tamén
// dentro do modal de contido dun bloque, que é onde de feito se escribe
// código Maxima agora que o modo por defecto é Bloques.
const AUTO_CLOSE_PAIRS = { '(': ')', '[': ']', '{': '}', '"': '"', "'": "'" }

function attachAutoClose(textarea) {
  textarea.addEventListener('keydown', (e) => {
    if (e.ctrlKey || e.metaKey || e.altKey) return
    const { selectionStart: start, selectionEnd: end, value } = textarea

    // Comiñas: se xa hai o mesmo carácter xusto despois do cursor (sen
    // selección), salta por riba en vez de duplicar - así escribir "algo"
    // e seguir tecleando despois das comiñas non as amoréa.
    if ((e.key === '"' || e.key === "'") && start === end && value[start] === e.key) {
      e.preventDefault()
      textarea.setSelectionRange(start + 1, start + 1)
      return
    }

    const close = AUTO_CLOSE_PAIRS[e.key]
    if (!close) return
    e.preventDefault()
    const selected = value.slice(start, end)
    textarea.setRangeText(e.key + selected + close, start, end, 'end')
    if (selected) {
      textarea.setSelectionRange(start + 1, start + 1 + selected.length)
    } else {
      textarea.setSelectionRange(start + 1, start + 1)
    }
  })
}

export function createBlocksEditor(container, { initialDoc, uploadImage, canUploadImage, generateWithAI, generateExerciseWithAI, generateExamWithAI, saveToLibrary, listLibrary, deleteFromLibrary, abrirAsistente, t: tIn } = {}) {
  const t = tIn || ((clave, fallback, vars) => {
    let val = fallback ?? clave
    if (vars) for (const k in vars) val = val.replaceAll(`{${k}}`, vars[k])
    return val
  })
  const ELEMENT_META = elementMeta(t)
  // Agrupamento temático da paleta lateral.
  const ELEMENT_GROUPS = [
    { title: t('blocks.group.text', 'Texto'), types: ['text'] },
    { title: t('blocks.group.variables', 'Variables e cálculos'), types: ['variables'] },
    { title: t('blocks.group.math', 'Matemáticas'), types: ['formula', 'image-plot', 'image-tikz'] },
    { title: t('blocks.group.multimedia', 'Multimedia'), types: ['image-upload'] },
  ]

  let doc = parseDocument(initialDoc || '')
  let expandedId = null
  let draggedIdx = null       // idx de exercicio arrastrado (reordenar exercicios)
  let dropIndex = null        // posición de caída dun exercicio arrastrado
  let draggedPaletteType = null // type dun bloque da paleta a piques de soltar
  let elementDropTarget = null  // { exerciseId, index } onde caería o bloque arrastrado
  let modalEl = null
  let modalCtx = null // { exercise, element, isNew }
  let exerciseAIModalEl = null
  let examAIModalEl = null
  let saveLibraryModalEl = null
  let saveLibraryCtx = null // { tipo, contido }
  let libraryModalEl = null

  container.innerHTML = ''
  container.classList.add('blocks-root')

  const toolbar = document.createElement('div')
  toolbar.className = 'blocks-toolbar'
  const addBtn = document.createElement('button')
  addBtn.type = 'button'
  addBtn.className = 'blocks-toolbar__add'
  addBtn.textContent = t('blocks.addExercise', '+ Engadir exercicio')
  addBtn.addEventListener('click', insertExercise)
  toolbar.appendChild(addBtn)

  if (typeof generateExerciseWithAI === 'function') {
    const aiBtn = document.createElement('button')
    aiBtn.type = 'button'
    aiBtn.className = 'blocks-toolbar__ai'
    aiBtn.textContent = t('blocks.exerciseWithAI', '✨ Exercicio con IA')
    aiBtn.addEventListener('click', openExerciseAIModal)
    toolbar.appendChild(aiBtn)
  }

  if (typeof generateExamWithAI === 'function') {
    const examBtn = document.createElement('button')
    examBtn.type = 'button'
    examBtn.className = 'blocks-toolbar__ai'
    examBtn.textContent = t('blocks.examWithAI', '✨ Exame completo con IA')
    examBtn.addEventListener('click', openExamAIModal)
    toolbar.appendChild(examBtn)
  }

  // Nota: NON hai botóns de toolbar "Gardar exame na biblioteca"/
  // "Biblioteca" a propósito - gardar o exame enteiro xa o cobren Gardar/
  // Gardar como (menú principal), serían redundantes. openSaveLibraryModal/
  // openLibraryModal (funcións máis abaixo) seguen existindo: úsaas o
  // "💾 Gardar este bloque/exercicio na biblioteca" de cada bloque/
  // exercicio individual (líneas ~800/~1090), que é un uso distinto -
  // gardar un ANACO reutilizable, non o exame enteiro.

  const body = document.createElement('div')
  body.className = 'blocks-body'

  const palette = buildPalette()

  const canvas = document.createElement('div')
  canvas.className = 'blocks-canvas'
  canvas.setAttribute('aria-label', t('blocks.canvas.aria', 'Composición do exame'))
  canvas.addEventListener('dragover', (e) => {
    if (!draggedPaletteType) return
    e.preventDefault()
    e.dataTransfer.dropEffect = 'copy'
  })
  // Se o drop chega ata aquí é que non caeu sobre ningunha tarxeta de
  // exercicio (estas paran a propagación) — crea un exercicio novo co
  // bloque solto, comportamento simple e predicible.
  canvas.addEventListener('drop', (e) => {
    if (!draggedPaletteType) return
    e.preventDefault()
    const type = draggedPaletteType
    draggedPaletteType = null
    elementDropTarget = null
    createExerciseWithElement(type)
  })

  body.appendChild(palette)
  body.appendChild(canvas)

  container.appendChild(toolbar)
  container.appendChild(body)

  function markEdited() {
    doc.legacy = false
  }

  // ---------- Paleta ----------

  function buildPalette() {
    const paletteEl = document.createElement('div')
    paletteEl.className = 'blocks-palette'
    paletteEl.setAttribute('aria-label', t('blocks.palette.aria', 'Paleta de bloques'))

    for (const group of ELEMENT_GROUPS) {
      const groupEl = document.createElement('div')
      groupEl.className = 'palette-group'
      const title = document.createElement('p')
      title.className = 'palette-group__title'
      title.textContent = group.title
      groupEl.appendChild(title)

      for (const type of group.types) {
        groupEl.appendChild(buildPaletteBlock(type))
      }
      paletteEl.appendChild(groupEl)
    }
    return paletteEl
  }

  function buildPaletteBlock(type) {
    const meta = ELEMENT_META[type]
    const block = document.createElement('div')
    block.className = `palette-block palette-block--${type}`
    block.draggable = true
    block.tabIndex = 0
    block.setAttribute('role', 'button')
    block.title = t('blocks.palette.dragHint', 'Arrastra a un exercicio, ou preme cun exercicio aberto')
    block.innerHTML = `<span class="palette-block__ico" aria-hidden="true">${meta.ico}</span><span class="palette-block__label">${meta.label}</span>`

    block.addEventListener('dragstart', (e) => {
      draggedPaletteType = type
      e.dataTransfer.setData('text/plain', type)
      e.dataTransfer.effectAllowed = 'copy'
    })
    block.addEventListener('dragend', () => {
      draggedPaletteType = null
      elementDropTarget = null
      render()
    })
    block.addEventListener('click', () => paletteBlockClick(type))
    block.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); paletteBlockClick(type) }
    })

    return block
  }

  // Clic na paleta (sen arrastrar): engade ao exercicio aberto, se hai un;
  // se non hai ningún expandido, crea un exercicio novo co bloque.
  function paletteBlockClick(type) {
    const exercise = doc.exercises.find((ex) => ex.id === expandedId)
    if (exercise) {
      insertElement(exercise, type)
    } else {
      createExerciseWithElement(type)
    }
  }

  function createExerciseWithElement(type) {
    markEdited()
    const exercise = makeExercise([])
    doc.exercises.push(exercise)
    insertElement(exercise, type)
  }

  // ---------- Exercicios ----------

  function insertExercise() {
    markEdited()
    const ex = makeExercise([])
    doc.exercises.push(ex)
    expandedId = ex.id
    render()
  }

  // ---------- Modal "Exercicio con IA" ----------
  // A diferenza do ✨ dentro de cada bloque (que rechea UN anaco xa creado),
  // este crea o EXERCICIO ENTEIRO cos anacos que faga falta (enunciado,
  // fórmula, gráfico...) a partir dunha soa descrición — o profesorado nunca
  // escolle un tipo de bloque nin sabe que existen etiquetas Matexe. Modal
  // independente do de contido (modalEl/modalCtx): non comparten estado, o
  // seu peche non depende de "era un anaco novo" coma o outro.
  function ensureExerciseAIModal() {
    if (exerciseAIModalEl) return exerciseAIModalEl
    const overlay = document.createElement('div')
    overlay.className = 'overlay hidden'
    const modal = document.createElement('div')
    modal.className = 'modal'
    overlay.appendChild(modal)
    overlay.addEventListener('click', (e) => { if (e.target === overlay) closeExerciseAIModal() })
    document.body.appendChild(overlay)
    exerciseAIModalEl = overlay
    return overlay
  }

  function closeExerciseAIModal() {
    if (exerciseAIModalEl) exerciseAIModalEl.classList.add('hidden')
  }

  function openExerciseAIModal() {
    const overlay = ensureExerciseAIModal()
    const modal = overlay.querySelector('.modal')
    modal.innerHTML = ''

    const h2 = document.createElement('h2')
    h2.innerHTML = `<span aria-hidden="true">✨</span> ${t('blocks.exerciseAI.title', 'Exercicio con IA')}`
    modal.appendChild(h2)

    const label = document.createElement('label')
    label.className = 'modal-label'
    label.textContent = t('blocks.exerciseAI.describeLabel', 'Describe o exercicio que queres')
    const textarea = document.createElement('textarea')
    textarea.rows = 3
    textarea.placeholder = t('blocks.exerciseAI.placeholder', 'Ex.: "a derivada dunha función cadrática, con gráfico"')
    attachAutoClose(textarea)
    label.appendChild(textarea)
    modal.appendChild(label)

    const note = document.createElement('p')
    note.textContent = t('blocks.exerciseAI.note', 'A IA crea o enunciado e os anacos que faga falta (fórmula, gráfico...) xa colocados; podes revisalos e editalos coma calquera bloque despois.')
    modal.appendChild(note)

    const status = document.createElement('p')
    status.className = 'ai-row__status'
    modal.appendChild(status)

    const actions = document.createElement('div')
    actions.className = 'modal-actions'
    const cancelBtn = document.createElement('button')
    cancelBtn.type = 'button'
    cancelBtn.textContent = t('blocks.modal.cancel', 'Cancelar')
    cancelBtn.addEventListener('click', closeExerciseAIModal)
    const genBtn = document.createElement('button')
    genBtn.type = 'button'
    genBtn.className = 'primary'
    genBtn.textContent = t('blocks.exerciseAI.generate', 'Xerar')
    async function doGenerate() {
      const peticion = textarea.value.trim()
      if (!peticion) { status.textContent = t('blocks.exerciseAI.emptyRequest', 'Escribe que exercicio queres.'); return }
      genBtn.disabled = true
      status.textContent = t('blocks.exerciseAI.generating', 'Xerando…')
      try {
        const anacos = await generateExerciseWithAI(peticion)
        markEdited()
        const elements = (anacos || [])
          .filter((a) => a && ELEMENT_META[a.type] && a.type !== 'image-upload')
          .map((a) => makeElement(a.type, a.content))
        const ex = makeExercise(elements.length ? elements : [makeElement('text', '')])
        doc.exercises.push(ex)
        expandedId = ex.id
        closeExerciseAIModal()
        render()
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
    actions.appendChild(cancelBtn)
    actions.appendChild(genBtn)
    modal.appendChild(actions)

    overlay.classList.remove('hidden')
    requestAnimationFrame(() => textarea.focus())
  }

  // ---------- Modal "Exame completo con IA" ----------
  // Coma openExerciseAIModal pero para VARIOS exercicios de vez: o
  // profesorado indica un tema e cantos exercicios quere, e generateExamWithAI
  // (XerarExameIA) devolve un array de exercicios (cada un xa na forma de
  // XerarExercicioIA: array de anacos type/content) que se engaden todos ao
  // final do exame, na orde recibida. Modal independente dos outros dous
  // (modalEl/exerciseAIModalEl): non comparten estado.
  function ensureExamAIModal() {
    if (examAIModalEl) return examAIModalEl
    const overlay = document.createElement('div')
    overlay.className = 'overlay hidden'
    const modal = document.createElement('div')
    modal.className = 'modal'
    overlay.appendChild(modal)
    overlay.addEventListener('click', (e) => { if (e.target === overlay) closeExamAIModal() })
    document.body.appendChild(overlay)
    examAIModalEl = overlay
    return overlay
  }

  function closeExamAIModal() {
    if (examAIModalEl) examAIModalEl.classList.add('hidden')
  }

  function openExamAIModal() {
    const overlay = ensureExamAIModal()
    const modal = overlay.querySelector('.modal')
    modal.innerHTML = ''

    const h2 = document.createElement('h2')
    h2.innerHTML = `<span aria-hidden="true">✨</span> ${t('blocks.examAI.title', 'Exame completo con IA')}`
    modal.appendChild(h2)

    const numLabel = document.createElement('label')
    numLabel.className = 'modal-label'
    numLabel.textContent = t('blocks.examAI.numExercisesLabel', 'Número de exercicios')
    const numInput = document.createElement('input')
    numInput.type = 'number'
    numInput.min = '1'
    numInput.max = '20'
    numInput.value = '5'
    numLabel.appendChild(numInput)
    modal.appendChild(numLabel)

    const temaLabel = document.createElement('label')
    temaLabel.className = 'modal-label'
    temaLabel.textContent = t('blocks.examAI.topicLabel', 'Tema do exame')
    const textarea = document.createElement('textarea')
    textarea.rows = 3
    textarea.placeholder = t('blocks.examAI.placeholder', 'Ex.: "derivadas e a súa aplicación a máximos e mínimos"')
    attachAutoClose(textarea)
    temaLabel.appendChild(textarea)
    modal.appendChild(temaLabel)

    const note = document.createElement('p')
    note.textContent = t('blocks.examAI.note', 'A IA crea todos os exercicios de vez (enunciado, fórmulas, gráficos...) e engádeos ao final do exame; podes revisalos, reordenalos e editalos coma calquera outro despois.')
    modal.appendChild(note)

    // Modelo(s) en PDF (opcional): mesmo patrón visual có upload de imaxes
    // (renderImageUploadRow) - un botón que abre un <input type="file"> agochado.
    // Os File quedan en modeloInput.files; convértense a base64 en
    // generateExamWithAI (main.js), blocks.js nunca fala cos bindings Wails.
    const modeloWrap = document.createElement('div')
    modeloWrap.className = 'image-upload-row'
    const modeloStatus = document.createElement('span')
    modeloStatus.className = 'image-upload-row__status'
    modeloStatus.textContent = t('blocks.examAI.noModel', 'Ningún PDF modelo escollido.')
    modeloWrap.appendChild(modeloStatus)
    const modeloLabel = document.createElement('label')
    modeloLabel.className = 'image-upload-row__btn'
    modeloLabel.textContent = t('blocks.examAI.attachModel', 'Xuntar PDF modelo (opcional)…')
    const modeloInput = document.createElement('input')
    modeloInput.type = 'file'
    modeloInput.accept = 'application/pdf'
    modeloInput.multiple = true
    modeloInput.hidden = true
    modeloInput.addEventListener('change', () => {
      const n = modeloInput.files ? modeloInput.files.length : 0
      modeloStatus.textContent = n === 0
        ? t('blocks.examAI.noModel', 'Ningún PDF modelo escollido.')
        : n === 1
          ? t('blocks.examAI.oneModel', 'Modelo: {name}', { name: modeloInput.files[0].name })
          : t('blocks.examAI.severalModels', '{n} PDF modelo escollidos.', { n })
    })
    modeloLabel.appendChild(modeloInput)
    modeloWrap.appendChild(modeloLabel)
    modal.appendChild(modeloWrap)

    const modeloNote = document.createElement('p')
    modeloNote.textContent = t('blocks.examAI.modelNote', 'Se xuntas un ou varios PDF (por exemplo, un exame anterior), a IA tómaos coma modelo de tema, formato e dificultade para crear unha versión DERIVADA - non unha copia. Con PDF xuntado xa non fai falla escribir un tema.')
    modal.appendChild(modeloNote)

    const status = document.createElement('p')
    status.className = 'ai-row__status'
    modal.appendChild(status)

    const actions = document.createElement('div')
    actions.className = 'modal-actions'
    const cancelBtn = document.createElement('button')
    cancelBtn.type = 'button'
    cancelBtn.textContent = t('blocks.modal.cancel', 'Cancelar')
    cancelBtn.addEventListener('click', closeExamAIModal)
    const genBtn = document.createElement('button')
    genBtn.type = 'button'
    genBtn.className = 'primary'
    genBtn.textContent = t('blocks.examAI.generate', 'Xerar')
    async function doGenerate() {
      const tema = textarea.value.trim()
      const modeloFiles = Array.from(modeloInput.files || [])
      if (!tema && modeloFiles.length === 0) {
        status.textContent = t('blocks.examAI.emptyRequest', 'Escribe o tema do exame ou xunta un PDF modelo.')
        return
      }
      const num = parseInt(numInput.value, 10) || 1
      genBtn.disabled = true
      status.textContent = modeloFiles.length
        ? t('blocks.examAI.readingModels', 'Lendo {n} PDF modelo e xerando {num} exercicios…', { n: modeloFiles.length, num })
        : t('blocks.examAI.generating', 'Xerando {num} exercicios…', { num })
      try {
        const exercicios = await generateExamWithAI(tema, num, modeloFiles)
        markEdited()
        for (const anacos of exercicios || []) {
          const elements = (anacos || [])
            .filter((a) => a && ELEMENT_META[a.type] && a.type !== 'image-upload')
            .map((a) => makeElement(a.type, a.content))
          doc.exercises.push(makeExercise(elements.length ? elements : [makeElement('text', '')]))
        }
        expandedId = null
        closeExamAIModal()
        render()
      } catch (err) {
        status.textContent = t('blocks.examAI.error', 'Erro: {msg}', { msg: err && err.message ? err.message : err })
      } finally {
        genBtn.disabled = false
      }
    }
    genBtn.addEventListener('click', doGenerate)
    actions.appendChild(cancelBtn)
    actions.appendChild(genBtn)
    modal.appendChild(actions)

    overlay.classList.remove('hidden')
    requestAnimationFrame(() => numInput.focus())
  }

  // ---------- Modal "Gardar na biblioteca" ----------
  // Un só modal para os tres niveis (exame/exercicio/bloque) - o botón que
  // o abre (toolbar, tarxeta de exercicio, ou fila de anaco) xa serializou
  // o contido a texto .matex cru (documentToText/elementsToText, mesmo
  // dialecto que len/escriben o resto da app), así que aquí só falla pedir
  // un nome.
  const SAVE_LIBRARY_TITLES = {
    exame: t('blocks.saveLibrary.type.exame', 'exame'),
    exercicio: t('blocks.saveLibrary.type.exercicio', 'exercicio'),
    bloque: t('blocks.saveLibrary.type.bloque', 'bloque'),
  }

  function ensureSaveLibraryModal() {
    if (saveLibraryModalEl) return saveLibraryModalEl
    const overlay = document.createElement('div')
    overlay.className = 'overlay hidden'
    const modal = document.createElement('div')
    modal.className = 'modal'
    overlay.appendChild(modal)
    overlay.addEventListener('click', (e) => { if (e.target === overlay) closeSaveLibraryModal() })
    document.body.appendChild(overlay)
    saveLibraryModalEl = overlay
    return overlay
  }

  function closeSaveLibraryModal() {
    if (saveLibraryModalEl) saveLibraryModalEl.classList.add('hidden')
    saveLibraryCtx = null
  }

  function openSaveLibraryModal(tipo, contido) {
    saveLibraryCtx = { tipo, contido }
    const overlay = ensureSaveLibraryModal()
    const modal = overlay.querySelector('.modal')
    modal.innerHTML = ''

    const h2 = document.createElement('h2')
    h2.innerHTML = `<span aria-hidden="true">💾</span> ${t('blocks.saveLibrary.title', 'Gardar {tipo} na biblioteca', { tipo: SAVE_LIBRARY_TITLES[tipo] || tipo })}`
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
    cancelBtn.addEventListener('click', closeSaveLibraryModal)
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
        closeSaveLibraryModal()
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

  // ---------- Modal "📚 Biblioteca" ----------
  // Lista o gardado por openSaveLibraryModal e insíreo no exame aberto -
  // "Inserir" nunca substitúe nada, só engade (exame: todos os seus
  // exercicios ao final; exercicio: un exercicio novo ao final; bloque: ao
  // exercicio aberto se hai un, ou nun exercicio novo se non), coma
  // arrastrar dende a paleta ou "✨ Exercicio con IA" - o profesorado
  // sempre pode reordenar despois.
  const LIBRARY_TIPO_LABELS = {
    exame: t('blocks.library.type.exame', 'Exame'),
    exercicio: t('blocks.library.type.exercicio', 'Exercicio'),
    bloque: t('blocks.library.type.bloque', 'Bloque'),
  }
  const LIBRARY_TIPO_ICOS = { exame: '📄', exercicio: '🧩', bloque: '🔹' }

  function ensureLibraryModal() {
    if (libraryModalEl) return libraryModalEl
    const overlay = document.createElement('div')
    overlay.className = 'overlay hidden'
    const modal = document.createElement('div')
    modal.className = 'modal modal--library'
    overlay.appendChild(modal)
    overlay.addEventListener('click', (e) => { if (e.target === overlay) closeLibraryModal() })
    document.body.appendChild(overlay)
    libraryModalEl = overlay
    return overlay
  }

  function closeLibraryModal() {
    if (libraryModalEl) libraryModalEl.classList.add('hidden')
  }

  function insertLibraryItem(item) {
    markEdited()
    if (item.tipo === 'exame') {
      const parsed = parseDocument(item.contido)
      doc.exercises.push(...parsed.exercises)
    } else if (item.tipo === 'exercicio') {
      doc.exercises.push(makeExercise(parseElements(item.contido)))
    } else {
      const elements = parseElements(item.contido)
      const exercise = doc.exercises.find((ex) => ex.id === expandedId)
      if (exercise) {
        exercise.elements.push(...elements)
      } else {
        const ex = makeExercise(elements)
        doc.exercises.push(ex)
        expandedId = ex.id
      }
    }
    closeLibraryModal()
    render()
  }

  async function openLibraryModal() {
    const overlay = ensureLibraryModal()
    const modal = overlay.querySelector('.modal')
    modal.innerHTML = ''

    const h2 = document.createElement('h2')
    h2.innerHTML = `<span aria-hidden="true">📚</span> ${t('blocks.library.title', 'Biblioteca')}`
    modal.appendChild(h2)

    const list = document.createElement('div')
    list.className = 'library-list'
    modal.appendChild(list)

    async function refresh() {
      list.innerHTML = `<p class="library-list__loading">${t('blocks.library.loading', 'Cargando…')}</p>`
      let items
      try {
        items = await listLibrary()
      } catch (err) {
        list.innerHTML = ''
        const p = document.createElement('p')
        p.textContent = t('blocks.library.error', 'Erro: {msg}', { msg: err && err.message ? err.message : err })
        list.appendChild(p)
        return
      }
      list.innerHTML = ''
      if (!items || items.length === 0) {
        const p = document.createElement('p')
        p.className = 'library-list__empty'
        p.textContent = t('blocks.library.empty', 'Aínda non gardaches nada na biblioteca (botóns "💾" no exame, nun exercicio, ou nun bloque).')
        list.appendChild(p)
        return
      }
      for (const item of items) {
        const row = document.createElement('div')
        row.className = 'library-item'

        const info = document.createElement('div')
        info.className = 'library-item__info'
        info.innerHTML = `<span class="library-item__ico" aria-hidden="true">${LIBRARY_TIPO_ICOS[item.tipo] || '📦'}</span>` +
          `<span class="library-item__nome">${escapeHtml(item.nome)}</span>` +
          `<span class="library-item__tipo">${LIBRARY_TIPO_LABELS[item.tipo] || item.tipo}</span>`
        row.appendChild(info)

        const actions = document.createElement('div')
        actions.className = 'library-item__actions'
        const insertBtn = document.createElement('button')
        insertBtn.type = 'button'
        insertBtn.className = 'primary'
        insertBtn.textContent = t('blocks.library.insert', 'Inserir')
        insertBtn.addEventListener('click', () => insertLibraryItem(item))
        actions.appendChild(insertBtn)
        actions.appendChild(mkActionBtn('🗑', t('blocks.library.delete', 'Eliminar da biblioteca'), async () => {
          await deleteFromLibrary(item.id)
          refresh()
        }))
        row.appendChild(actions)

        list.appendChild(row)
      }
    }

    const actions = document.createElement('div')
    actions.className = 'modal-actions'
    const closeBtn = document.createElement('button')
    closeBtn.type = 'button'
    closeBtn.textContent = t('blocks.library.close', 'Pechar')
    closeBtn.addEventListener('click', closeLibraryModal)
    actions.appendChild(closeBtn)
    modal.appendChild(actions)

    overlay.classList.remove('hidden')
    refresh()
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
  }

  function moveExercise(idx, dir) {
    const to = idx + dir
    if (to < 0 || to >= doc.exercises.length) return
    markEdited()
    const tmp = doc.exercises[idx]
    doc.exercises[idx] = doc.exercises[to]
    doc.exercises[to] = tmp
    render()
  }

  function duplicateExercise(idx) {
    markEdited()
    const src = doc.exercises[idx]
    const copy = makeExercise(src.elements.map((e) => makeElement(e.type, e.content)))
    doc.exercises.splice(idx + 1, 0, copy)
    expandedId = copy.id
    render()
  }

  function removeExercise(idx) {
    markEdited()
    doc.exercises.splice(idx, 1)
    expandedId = null
    render()
  }

  function reorderByDrag(fromIdx, toIdx) {
    if (fromIdx === toIdx || fromIdx === toIdx - 1) return
    markEdited()
    const [ex] = doc.exercises.splice(fromIdx, 1)
    const insertAt = toIdx > fromIdx ? toIdx - 1 : toIdx
    doc.exercises.splice(insertAt, 0, ex)
    render()
  }

  // ---------- Anacos ----------

  // insertElement engade un anaco novo en `exercise` (por defecto ao final)
  // e abre de contado a ventá modal para introducir o seu contido — tanto
  // arrastrar dende a paleta coma premela con exercicio aberto pasan por
  // aquí.
  function insertElement(exercise, type, atIndex) {
    markEdited()
    const el = makeElement(type, ELEMENT_META[type].def)
    const idx = atIndex === undefined ? exercise.elements.length : atIndex
    exercise.elements.splice(idx, 0, el)
    expandedId = exercise.id
    render()
    openElementModal(exercise, el, { isNew: true })
  }

  function moveElement(exercise, idx, dir) {
    const to = idx + dir
    if (to < 0 || to >= exercise.elements.length) return
    markEdited()
    const tmp = exercise.elements[idx]
    exercise.elements[idx] = exercise.elements[to]
    exercise.elements[to] = tmp
    render()
  }

  function removeElement(exercise, idx) {
    markEdited()
    exercise.elements.splice(idx, 1)
    render()
  }

  function mkActionBtn(symbol, title, onClick) {
    const b = document.createElement('button')
    b.type = 'button'
    b.className = 'icon-btn'
    b.textContent = symbol
    b.title = title
    b.setAttribute('aria-label', title)
    b.addEventListener('click', (e) => { e.stopPropagation(); onClick() })
    return b
  }

  function summaryOf(exercise) {
    const first = exercise.elements.find((e) => e.type === 'text' && e.content.trim() !== '')
    if (first) {
      const plain = first.content.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
      if (plain) return plain.length > 80 ? plain.slice(0, 80) + '…' : plain
    }
    if (exercise.elements.length === 0) return t('blocks.exercise.empty', '(baleiro)')
    return exercise.elements.map((e) => ELEMENT_META[e.type].ico).join(' ')
  }

  function summarizeElement(element) {
    if (element.type === 'image-upload') return element.content || t('blocks.element.noImage', '(ningunha imaxe escollida)')
    const plain = element.content.replace(/\s+/g, ' ').trim()
    if (!plain) return t('blocks.element.empty', '(baleiro)')
    return plain.length > 100 ? plain.slice(0, 100) + '…' : plain
  }

  // elementIndexFromPoint calcula onde caería un bloque arrastrado dentro
  // dun panel de anacos, en función da coordenada Y — mesmo criterio
  // (metade superior/inferior de cada fila) que xa usa o drag de exercicios.
  function elementIndexFromPoint(panelEl, clientY) {
    const rows = panelEl.querySelectorAll(':scope > .element-row')
    for (let i = 0; i < rows.length; i++) {
      const rect = rows[i].getBoundingClientRect()
      if (clientY < rect.top + rect.height / 2) return i
    }
    return rows.length
  }

  function renderElementRow(exercise, element, idx) {
    const meta = ELEMENT_META[element.type]
    const row = document.createElement('div')
    row.className = `element-row element-row--${element.type}`
    row.tabIndex = 0
    row.setAttribute('role', 'button')
    row.title = t('blocks.element.clickToEdit', 'Premer para editar')

    const head = document.createElement('div')
    head.className = 'element-row__head'
    head.innerHTML = `<span class="element-row__ico" aria-hidden="true">${meta.ico}</span><span class="element-row__label">${meta.label}</span>`
    const actions = document.createElement('span')
    actions.className = 'element-row__actions'
    actions.appendChild(mkActionBtn('↑', t('blocks.element.moveUp', 'Mover cara arriba'), () => moveElement(exercise, idx, -1)))
    actions.appendChild(mkActionBtn('↓', t('blocks.element.moveDown', 'Mover cara abaixo'), () => moveElement(exercise, idx, 1)))
    if (typeof saveToLibrary === 'function') {
      actions.appendChild(mkActionBtn('💾', t('blocks.element.saveToLibrary', 'Gardar este bloque na biblioteca'), () => openSaveLibraryModal('bloque', elementsToText([element]))))
    }
    actions.appendChild(mkActionBtn('✕', t('blocks.element.remove', 'Eliminar anaco'), () => removeElement(exercise, idx)))
    head.appendChild(actions)
    row.appendChild(head)

    const summary = document.createElement('p')
    summary.className = 'element-row__summary'
    summary.textContent = summarizeElement(element)
    row.appendChild(summary)

    row.addEventListener('click', (e) => {
      if (e.target.closest('.element-row__actions')) return
      openElementModal(exercise, element, { isNew: false })
    })
    row.addEventListener('keydown', (e) => {
      if (e.target.closest('.element-row__actions')) return
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openElementModal(exercise, element, { isNew: false }) }
    })

    return row
  }

  // renderImageUploadRow móntase dentro do modal de contido (ver
  // openElementModal); a subida é inmediata (o callback muta
  // element.content en canto remata), o botón "Gardar" do modal só
  // confirma o peche.
  function renderImageUploadRow(exercise, element) {
    const wrap = document.createElement('div')
    wrap.className = 'image-upload-row'

    const status = document.createElement('span')
    status.className = 'image-upload-row__status'
    status.textContent = element.content ? t('blocks.imageUpload.saved', 'Imaxe gardada: {path}', { path: element.content }) : t('blocks.imageUpload.none', 'Ningunha imaxe escollida.')
    wrap.appendChild(status)

    // Botón normal (non un <label> arredor dun <input type=file> agochado):
    // así podemos comprobar canUploadImage() ANTES de abrir o selector do
    // sistema. Antes, premer aquí sen ter o exame gardado abría o selector
    // igual, o usuario escollía a imaxe, e só despois (dentro de
    // uploadImage) chegaba o erro "garda o documento antes de engadir
    // imaxes" - fácil de non ver dentro do modal, e daba a sensación de que
    // "non deixaba escoller o ficheiro" cando en realidade a imaxe nunca se
    // chegaba a gardar (por iso tampouco aparecía despois no PDF).
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.className = 'image-upload-row__btn'
    btn.textContent = t('blocks.imageUpload.choose', 'Escoller ficheiro…')
    btn.addEventListener('click', () => {
      const motivo = typeof canUploadImage === 'function' ? canUploadImage() : null
      if (motivo) {
        status.textContent = motivo
        return
      }
      const input = document.createElement('input')
      input.type = 'file'
      input.accept = 'image/*'
      input.addEventListener('change', async () => {
        const file = input.files && input.files[0]
        if (!file) return
        if (typeof uploadImage !== 'function') {
          status.textContent = t('blocks.imageUpload.noUploadFn', 'Non se puido gardar a imaxe: función de subida non dispoñible.')
          return
        }
        status.textContent = t('blocks.imageUpload.saving', 'Gardando imaxe…')
        try {
          const path = await uploadImage(file)
          markEdited()
          element.content = path
          status.textContent = t('blocks.imageUpload.saved', 'Imaxe gardada: {path}', { path })
        } catch (err) {
          status.textContent = t('blocks.imageUpload.error', 'Erro: {msg}', { msg: err && err.message ? err.message : err })
        }
      })
      input.click()
    })
    wrap.appendChild(btn)
    return wrap
  }

  // Etiquetas Matexe dispoñibles para inserir na selección do Enunciado -
  // mesma lista, iconos e orde ca #matexeTagBar (modo Código, ver
  // index.html/editor.js). SÓ no Enunciado: é o único anaco con HTML libre
  // onde ten sentido escribir etiquetas a man (os outros tipos de anaco xa
  // SON unha soa etiqueta - o anaco "Fórmula", por exemplo, xa é un <EVAL>
  // enteiro; poñer aquí a barra envolvería a etiqueta arredor de si mesma).
  const TAG_INSERT_TAGS = [
    { tag: 'MAT', ico: '🔣' },
    { tag: 'EVAL', ico: '🧮' },
    { tag: 'HIDE', ico: '🔢' },
    { tag: 'PLOT', ico: '📈' },
    { tag: 'TEX', ico: '📐' },
    { tag: 'TIKZ', ico: '✏️' },
  ]

  // wrapSelectionWithTag envolve a selección actual do textarea (ou insire
  // unha etiqueta baleira se non hai nada seleccionado) en <TAG>...</TAG>,
  // e deixa o texto orixinal seleccionado xa dentro dela para poder seguir
  // escribindo sen ter que volver premer. Mesma idea ca wrapTagCommand
  // (editor.js/CodeMirror), pero para un <textarea> plano.
  function wrapSelectionWithTag(textarea, tag) {
    const { selectionStart: start, selectionEnd: end, value } = textarea
    const selected = value.slice(start, end)
    textarea.setRangeText(`<${tag}>${selected}</${tag}>`, start, end, 'end')
    const cursor = start + tag.length + 2
    textarea.setSelectionRange(cursor, cursor + selected.length)
    textarea.focus()
  }

  function renderTagInsertRow(textarea) {
    const bar = document.createElement('div')
    bar.className = 'matexe-tag-bar matexe-tag-bar--inline'
    bar.setAttribute('role', 'toolbar')
    bar.setAttribute('aria-label', t('tagbar.aria', 'Inserir etiqueta Matexe'))
    for (const { tag, ico } of TAG_INSERT_TAGS) {
      const btn = document.createElement('button')
      btn.type = 'button'
      btn.className = 'matexe-tag-btn'
      btn.textContent = `${ico} ${tag}`
      btn.addEventListener('click', () => wrapSelectionWithTag(textarea, tag))
      bar.appendChild(btn)
    }
    return bar
  }

  // makeDraggable permite arrastrar `modal` (o cadro, non o fondo escurecido
  // do overlay) premendo e arrastrando `handle` (o título) - para poder
  // apartalo cando tapa contido que se precisa ver por detrás. Só engade
  // listeners globais (mousemove/mouseup) MENTRES se arrastra, non
  // permanentes - se non, cada apertura do modal iría acumulando listeners
  // que nunca se soltan.
  function makeDraggable(modal, handle) {
    handle.addEventListener('mousedown', (e) => {
      if (e.button !== 0) return
      const rect = modal.getBoundingClientRect()
      // Pasa de centrado por flexbox a posición fixa nas coordenadas
      // actuais, para que non salte ao empezar a arrastrar.
      modal.style.position = 'fixed'
      modal.style.margin = '0'
      modal.style.left = rect.left + 'px'
      modal.style.top = rect.top + 'px'
      const startX = e.clientX
      const startY = e.clientY
      const startLeft = rect.left
      const startTop = rect.top

      function onMove(ev) {
        modal.style.left = startLeft + ev.clientX - startX + 'px'
        modal.style.top = startTop + ev.clientY - startY + 'px'
      }
      function onUp() {
        window.removeEventListener('mousemove', onMove)
        window.removeEventListener('mouseup', onUp)
      }
      window.addEventListener('mousemove', onMove)
      window.addEventListener('mouseup', onUp)
      e.preventDefault()
    })
  }

  // renderImageInsertRow móntase só no modal do anaco "text" (Enunciado):
  // insire unha imaxe DENTRO do seu HTML, coas súas dimensións (ex.: o logo
  // dun cabeceiro cunha táboa) - a diferenza do bloque "Imaxe" solto
  // (renderImageUploadRow, arriba), que é un anaco á parte e sen tamaño.
  // Reutiliza o mesmo circuíto de subida (canUploadImage/uploadImage); o que
  // cambia é que aquí NON se toca element.content directamente, escríbese
  // unha etiqueta <img> no textarea, na posición onde estaba o cursor.
  function renderImageInsertRow(textarea) {
    const wrap = document.createElement('div')
    wrap.className = 'image-insert-row'

    const btn = document.createElement('button')
    btn.type = 'button'
    btn.className = 'image-upload-row__btn'
    btn.textContent = t('blocks.imageInsert.choose', '🖼️ Inserir imaxe…')

    const status = document.createElement('span')
    status.className = 'image-insert-row__status'

    const sizeForm = document.createElement('div')
    sizeForm.className = 'image-insert-row__size hidden'

    const widthLabel = document.createElement('label')
    widthLabel.textContent = t('blocks.imageInsert.width', 'Ancho (cm)')
    const widthInput = document.createElement('input')
    widthInput.type = 'number'
    widthInput.min = '0'
    widthInput.step = '0.1'
    widthLabel.appendChild(widthInput)

    const heightLabel = document.createElement('label')
    heightLabel.textContent = t('blocks.imageInsert.height', 'Alto (cm)')
    const heightInput = document.createElement('input')
    heightInput.type = 'number'
    heightInput.min = '0'
    heightInput.step = '0.1'
    heightLabel.appendChild(heightInput)

    const insertBtn = document.createElement('button')
    insertBtn.type = 'button'
    insertBtn.className = 'primary'
    insertBtn.textContent = t('blocks.imageInsert.insert', 'Inserir')

    sizeForm.appendChild(widthLabel)
    sizeForm.appendChild(heightLabel)
    sizeForm.appendChild(insertBtn)

    let pendingPath = null
    // Gardado ANTES de abrir o selector de ficheiro (asíncrono e rouba o
    // foco): se se lese textarea.selectionStart despois, xa apuntaría a
    // calquera sitio (ou ao final), non ao punto onde estaba o cursor.
    let cursorPos = null

    btn.addEventListener('click', () => {
      const motivo = typeof canUploadImage === 'function' ? canUploadImage() : null
      if (motivo) {
        status.textContent = motivo
        return
      }
      cursorPos = { start: textarea.selectionStart, end: textarea.selectionEnd }
      const input = document.createElement('input')
      input.type = 'file'
      input.accept = 'image/*'
      input.addEventListener('change', async () => {
        const file = input.files && input.files[0]
        if (!file) return
        if (typeof uploadImage !== 'function') {
          status.textContent = t('blocks.imageUpload.noUploadFn', 'Non se puido gardar a imaxe: función de subida non dispoñible.')
          return
        }
        status.textContent = t('blocks.imageUpload.saving', 'Gardando imaxe…')
        try {
          pendingPath = await uploadImage(file)
          status.textContent = t('blocks.imageInsert.ready', 'Imaxe gardada. Escolle o tamaño (opcional) e preme "Inserir".')
          sizeForm.classList.remove('hidden')
          requestAnimationFrame(() => widthInput.focus())
        } catch (err) {
          status.textContent = t('blocks.imageUpload.error', 'Erro: {msg}', { msg: err && err.message ? err.message : err })
        }
      })
      input.click()
    })

    insertBtn.addEventListener('click', () => {
      if (!pendingPath) return
      let tag = `<img src="${pendingPath}"`
      if (widthInput.value) tag += ` width="${widthInput.value}cm"`
      if (heightInput.value) tag += ` height="${heightInput.value}cm"`
      tag += '>'
      const { start, end } = cursorPos ?? { start: textarea.value.length, end: textarea.value.length }
      textarea.setRangeText(tag, start, end, 'end')
      pendingPath = null
      widthInput.value = ''
      heightInput.value = ''
      sizeForm.classList.add('hidden')
      status.textContent = ''
      textarea.focus()
    })

    wrap.appendChild(btn)
    wrap.appendChild(status)
    wrap.appendChild(sizeForm)
    return wrap
  }

  // ---------- Ventá modal de contido ----------

  function ensureModal() {
    if (modalEl) return modalEl
    const overlay = document.createElement('div')
    overlay.className = 'overlay hidden'
    const modal = document.createElement('div')
    modal.className = 'modal'
    overlay.appendChild(modal)
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) closeModal({ discard: true })
    })
    document.body.appendChild(overlay)
    modalEl = overlay
    return overlay
  }

  // buildAIRow monta o botón ✨ e o mini-formulario de petición para o
  // asistente de IA (opcional: só se main.js inxectou generateWithAI). Ao
  // xerar, substitúe o valor do textarea — non garda por si só, o usuario
  // aínda pode revisalo/editalo antes de premer "Gardar" no modal.
  function buildAIRow(tipo, textarea) {
    const wrap = document.createElement('div')
    wrap.className = 'ai-row'

    const toggleBtn = document.createElement('button')
    toggleBtn.type = 'button'
    toggleBtn.className = 'ai-row__toggle'
    toggleBtn.textContent = t('blocks.aiRow.toggle', '✨ Xerar con IA')

    const form = document.createElement('div')
    form.className = 'ai-row__form hidden'

    const input = document.createElement('input')
    input.type = 'text'
    input.className = 'ai-row__input'
    input.placeholder = t('blocks.aiRow.placeholder', 'Describe o que queres, ex.: "a derivada de x^3"')

    const genBtn = document.createElement('button')
    genBtn.type = 'button'
    genBtn.className = 'ai-row__generate'
    genBtn.textContent = t('blocks.aiRow.generate', 'Xerar')

    const status = document.createElement('span')
    status.className = 'ai-row__status'

    form.appendChild(input)
    form.appendChild(genBtn)
    form.appendChild(status)

    toggleBtn.addEventListener('click', () => {
      form.classList.toggle('hidden')
      if (!form.classList.contains('hidden')) requestAnimationFrame(() => input.focus())
    })

    async function doGenerate() {
      const peticion = input.value.trim()
      if (!peticion) {
        status.textContent = t('blocks.aiRow.emptyRequest', 'Escribe que queres xerar.')
        return
      }
      genBtn.disabled = true
      status.textContent = t('blocks.aiRow.generating', 'Xerando…')
      try {
        textarea.value = await generateWithAI(tipo, peticion)
        status.textContent = ''
        form.classList.add('hidden')
        textarea.focus()
      } catch (err) {
        status.textContent = t('blocks.aiRow.error', 'Erro: {msg}', { msg: err && err.message ? err.message : err })
      } finally {
        genBtn.disabled = false
      }
    }
    genBtn.addEventListener('click', doGenerate)
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); doGenerate() }
    })

    wrap.appendChild(toggleBtn)
    wrap.appendChild(form)
    return wrap
  }

  // renderAssistantRow móntase en todos os anacos con texto editable
  // (calquera tipo agás "image-upload") se main.js inxectou abrirAsistente
  // - abre a MESMA xanela de chat ca no editor de Código (assistant.js,
  // instancia única), configurada para ler/escribir este textarea en
  // concreto. A diferenza de buildAIRow (✨ Xerar con IA, sempre crea
  // contido novo dende un prompt baleiro), aquí pódese conversar sobre o
  // contido xa escrito: preguntar que fai, ou pedir que o modifique.
  function renderAssistantRow(tipo, titulo, textarea) {
    const wrap = document.createElement('div')
    wrap.className = 'assistant-open-row'
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.className = 'assistant-open-btn'
    btn.textContent = t('blocks.assistant.open', '🤖 Preguntar ao asistente')
    btn.addEventListener('click', () => {
      abrirAsistente({
        tipo,
        title: titulo,
        getContent: () => textarea.value,
        setContent: (texto) => { textarea.value = texto },
      })
    })
    wrap.appendChild(btn)
    return wrap
  }

  function openElementModal(exercise, element, { isNew }) {
    modalCtx = { exercise, element, isNew }
    const overlay = ensureModal()
    const modal = overlay.querySelector('.modal')
    modal.innerHTML = ''
    // O modal reutilízase entre aperturas (ensureModal cachea): limpar
    // tamén a posición arrastrada (makeDraggable) dunha apertura anterior,
    // para que cada apertura NOVA empece sempre centrada.
    modal.style.position = ''
    modal.style.left = ''
    modal.style.top = ''
    modal.style.margin = ''
    // modal--wide: ventá máis ampla para calquera anaco con textarea
    // (Enunciado, Variable/cálculo, fórmula, gráfico, TikZ) - só o "Imaxe"
    // (image-upload) queda fóra, porque non leva textarea, leva o seu
    // propio selector de ficheiro (renderImageUploadRow). A clase hai que
    // fixala sempre explicitamente (non só engadila) porque é o mesmo modal
    // cacheado entre aperturas.
    modal.className = 'modal' + (element.type !== 'image-upload' ? ' modal--wide' : '')
    const meta = ELEMENT_META[element.type]

    const h2 = document.createElement('h2')
    h2.className = 'modal-drag-handle'
    h2.innerHTML = `<span aria-hidden="true">${meta.ico}</span> ${meta.label}`
    modal.appendChild(h2)
    // Arrastrable dende o título, para poder apartalo cando tapa contido
    // que se precisa ver detrás (o fondo escurecido do overlay segue a
    // cubrir toda a pantalla, só se move o cadro).
    makeDraggable(modal, h2)

    let getContent = () => element.content
    if (element.type === 'image-upload') {
      modal.appendChild(renderImageUploadRow(exercise, element))
    } else {
      const textarea = document.createElement('textarea')
      textarea.rows = 16
      textarea.spellcheck = element.type === 'text'
      textarea.value = element.content
      attachAutoClose(textarea)

      // Barra de etiquetas Matexe SÓ no Enunciado, enriba do textarea (ver
      // renderTagInsertRow) - precisa da referencia ao textarea, por iso
      // créase antes do label que o envolve.
      if (element.type === 'text') {
        modal.appendChild(renderTagInsertRow(textarea))
      }

      const label = document.createElement('label')
      label.className = 'modal-label'
      label.textContent = meta.label
      label.appendChild(textarea)
      modal.appendChild(label)
      getContent = () => textarea.value
      requestAnimationFrame(() => { textarea.focus(); textarea.select() })
      // Inserir imaxe (con ancho/alto) SÓ no Enunciado: é o único anaco con
      // HTML libre onde ten sentido, ex. o logo dun cabeceiro (ver
      // renderImageInsertRow). O bloque "Imaxe" (image-upload) xa ten o seu
      // propio selector (renderImageUploadRow, arriba) e non leva tamaño.
      if (element.type === 'text') {
        modal.appendChild(renderImageInsertRow(textarea))
      }
      if (typeof generateWithAI === 'function') {
        modal.appendChild(buildAIRow(element.type, textarea))
      }
      if (typeof abrirAsistente === 'function') {
        modal.appendChild(renderAssistantRow(element.type, meta.label, textarea))
      }
    }

    if (meta.note) {
      const note = document.createElement('p')
      note.textContent = meta.note
      modal.appendChild(note)
    }

    const actions = document.createElement('div')
    actions.className = 'modal-actions'
    const cancelBtn = document.createElement('button')
    cancelBtn.type = 'button'
    cancelBtn.textContent = t('blocks.modal.cancel', 'Cancelar')
    cancelBtn.addEventListener('click', () => closeModal({ discard: true }))
    const saveBtn = document.createElement('button')
    saveBtn.type = 'button'
    saveBtn.className = 'primary'
    saveBtn.textContent = t('blocks.modal.save', 'Gardar')
    saveBtn.addEventListener('click', () => {
      markEdited()
      element.content = getContent()
      closeModal({ discard: false })
      render()
    })
    actions.appendChild(cancelBtn)
    actions.appendChild(saveBtn)
    modal.appendChild(actions)

    overlay.classList.remove('hidden')
  }

  // discard=true elimina o anaco se era novo (creado por arrastre/clic e
  // cancelado sen gardar contido) — así non queda un bloque baleiro solto.
  function closeModal({ discard }) {
    if (!modalEl) return
    if (discard && modalCtx && modalCtx.isNew) {
      const idx = modalCtx.exercise.elements.indexOf(modalCtx.element)
      if (idx !== -1) modalCtx.exercise.elements.splice(idx, 1)
      render()
    }
    modalEl.classList.add('hidden')
    modalCtx = null
  }

  function onKeydown(e) {
    if (e.key !== 'Escape') return
    if (modalCtx) { e.preventDefault(); closeModal({ discard: true }) }
    else if (exerciseAIModalEl && !exerciseAIModalEl.classList.contains('hidden')) {
      e.preventDefault()
      closeExerciseAIModal()
    } else if (examAIModalEl && !examAIModalEl.classList.contains('hidden')) {
      e.preventDefault()
      closeExamAIModal()
    } else if (saveLibraryModalEl && !saveLibraryModalEl.classList.contains('hidden')) {
      e.preventDefault()
      closeSaveLibraryModal()
    } else if (libraryModalEl && !libraryModalEl.classList.contains('hidden')) {
      e.preventDefault()
      closeLibraryModal()
    }
  }
  document.addEventListener('keydown', onKeydown)

  // ---------- Tarxeta de exercicio ----------

  function renderExerciseCard(exercise, idx) {
    const card = document.createElement('div')
    card.className = 'exercise-card'
    card.draggable = false // só se arrastra dende o handle, ver dragstart abaixo
    card.dataset.idx = String(idx)
    if (draggedIdx === idx) card.classList.add('exercise-card--dragging')

    const head = document.createElement('div')
    head.className = 'exercise-card__head'
    head.addEventListener('click', () => {
      expandedId = expandedId === exercise.id ? null : exercise.id
      render()
    })

    const handle = document.createElement('span')
    handle.className = 'exercise-card__handle'
    handle.textContent = '⠿'
    handle.title = t('blocks.exercise.dragHandle', 'Arrastra para reordenar')
    handle.setAttribute('draggable', 'true')
    handle.addEventListener('dragstart', (e) => {
      draggedIdx = idx
      e.dataTransfer.setData('text/plain', String(idx))
      e.dataTransfer.effectAllowed = 'move'
      render()
    })
    handle.addEventListener('dragend', () => {
      draggedIdx = null
      dropIndex = null
      render()
    })
    head.appendChild(handle)

    const chevron = document.createElement('span')
    chevron.className = 'exercise-card__chevron'
    chevron.textContent = expandedId === exercise.id ? '▾' : '▸'
    head.appendChild(chevron)

    const title = document.createElement('span')
    title.className = 'exercise-card__title'
    title.textContent = t('blocks.exercise.title', 'Exercicio {n}', { n: idx + 1 })
    head.appendChild(title)

    const actions = document.createElement('span')
    actions.className = 'exercise-card__actions'
    actions.appendChild(mkActionBtn('↑', t('blocks.exercise.moveUp', 'Mover cara arriba'), () => moveExercise(idx, -1)))
    actions.appendChild(mkActionBtn('↓', t('blocks.exercise.moveDown', 'Mover cara abaixo'), () => moveExercise(idx, 1)))
    actions.appendChild(mkActionBtn('⧉', t('blocks.exercise.duplicate', 'Duplicar exercicio'), () => duplicateExercise(idx)))
    if (typeof saveToLibrary === 'function') {
      actions.appendChild(mkActionBtn('💾', t('blocks.exercise.saveToLibrary', 'Gardar este exercicio na biblioteca'), () => openSaveLibraryModal('exercicio', elementsToText(exercise.elements))))
    }
    actions.appendChild(mkActionBtn('✕', t('blocks.exercise.remove', 'Eliminar exercicio'), () => removeExercise(idx)))
    head.appendChild(actions)
    card.appendChild(head)

    if (expandedId === exercise.id) {
      const panel = document.createElement('div')
      panel.className = 'exercise-card__panel'
      exercise.elements.forEach((el, i) => {
        if (elementDropTarget && elementDropTarget.exerciseId === exercise.id && elementDropTarget.index === i) {
          panel.appendChild(dropIndicator())
        }
        panel.appendChild(renderElementRow(exercise, el, i))
      })
      if (elementDropTarget && elementDropTarget.exerciseId === exercise.id && elementDropTarget.index === exercise.elements.length) {
        panel.appendChild(dropIndicator())
      }
      if (exercise.elements.length === 0 && !elementDropTarget) {
        const hint = document.createElement('p')
        hint.className = 'exercise-card__hint'
        hint.textContent = t('blocks.exercise.hint', 'Arrastra aquí un bloque da paleta, ou preme un bloque coa tarxeta aberta.')
        panel.appendChild(hint)
      }
      card.appendChild(panel)
    } else {
      const summary = document.createElement('p')
      summary.className = 'exercise-card__summary'
      summary.textContent = summaryOf(exercise)
      card.appendChild(summary)
    }

    // dragover/drop viven na propia card, non só no handle: soltar en
    // calquera punto do bloque de destino xa reordena (arrastre de
    // exercicio) ou insire un anaco novo (arrastre dende a paleta) — as
    // dúas orixes de arrastre compárten estes listeners, distinguidas por
    // cal das variables de estado (draggedIdx / draggedPaletteType) está
    // activa.
    card.addEventListener('dragover', (e) => {
      if (draggedPaletteType) {
        e.preventDefault()
        e.stopPropagation()
        e.dataTransfer.dropEffect = 'copy'
        const panelEl = card.querySelector('.exercise-card__panel')
        const elIdx = panelEl ? elementIndexFromPoint(panelEl, e.clientY) : exercise.elements.length
        const newTarget = { exerciseId: exercise.id, index: elIdx }
        if (!elementDropTarget || elementDropTarget.exerciseId !== newTarget.exerciseId || elementDropTarget.index !== newTarget.index) {
          elementDropTarget = newTarget
          render()
        }
        return
      }
      if (draggedIdx === null) return
      e.preventDefault()
      const rect = card.getBoundingClientRect()
      const before = e.clientY < rect.top + rect.height / 2
      const newDropIndex = before ? idx : idx + 1
      if (newDropIndex !== dropIndex) {
        dropIndex = newDropIndex
        render()
      }
    })
    card.addEventListener('drop', (e) => {
      if (draggedPaletteType) {
        e.preventDefault()
        e.stopPropagation()
        const type = draggedPaletteType
        const atIndex = elementDropTarget && elementDropTarget.exerciseId === exercise.id ? elementDropTarget.index : exercise.elements.length
        draggedPaletteType = null
        elementDropTarget = null
        insertElement(exercise, type, atIndex)
        return
      }
      e.preventDefault()
      if (draggedIdx === null || dropIndex === null) return
      reorderByDrag(draggedIdx, dropIndex)
      draggedIdx = null
      dropIndex = null
    })

    return card
  }

  function render() {
    canvas.innerHTML = ''

    if (doc.exercises.length === 0) {
      const empty = document.createElement('p')
      empty.className = 'blocks-empty'
      empty.textContent = t('blocks.empty', 'Arrastra un bloque da paleta (ou preme "+ Engadir exercicio") para empezar.')
      canvas.appendChild(empty)
      return
    }

    doc.exercises.forEach((exercise, idx) => {
      if (dropIndex === idx) canvas.appendChild(dropIndicator())
      canvas.appendChild(renderExerciseCard(exercise, idx))
    })
    if (dropIndex === doc.exercises.length) canvas.appendChild(dropIndicator())
  }

  function dropIndicator() {
    const line = document.createElement('div')
    line.className = 'drop-indicator'
    return line
  }

  render()

  return {
    getValue: () => documentToText(doc),
    setValue: (text) => {
      doc = parseDocument(text || '')
      expandedId = null
      render()
    },
    focus: () => {
      const first = canvas.querySelector('.exercise-card__head')
      if (first) first.focus()
    },
    destroy: () => {
      document.removeEventListener('keydown', onKeydown)
      if (modalEl) modalEl.remove()
      if (exerciseAIModalEl) exerciseAIModalEl.remove()
      if (examAIModalEl) examAIModalEl.remove()
      if (saveLibraryModalEl) saveLibraryModalEl.remove()
      if (libraryModalEl) libraryModalEl.remove()
      container.innerHTML = ''
      container.classList.remove('blocks-root')
    },
  }
}
