// Modal de contido dun anaco: extraído case literal do antigo blocks.js
// (openElementModal + funcións auxiliares) para que o poida reutilizar
// tanto ese editor coma o novo baseado en Blockly (ver
// blockly/content-field.js) - o DESENCADEANTE cambia (premer unha tarxeta
// vs. premer un ContentField), pero a modal en si (textarea, autopechado,
// barra de etiquetas, inserir imaxe, IA, asistente) é idéntica.
//
// Módulo con estado propio (un único overlay cacheado, `modalEl`/`modalCtx`
// a nivel de módulo) igual có antigo: só pode haber unha modal de contido
// aberta á vez, coma antes.
//
// openContentModal(opts):
//   type, meta        - tipo de anaco e a súa entrada de ELEMENT_META (ver
//                        blocks-meta.js: ico/label/def/note)
//   content, isNew     - contido actual e se o anaco acaba de crearse
//                        (cancelar bórrao, ver onCancel)
//   t                  - función de tradución (ver blocks-meta.js)
//   uploadImage, canUploadImage, generateWithAI, abrirAsistente - mesmos
//                        callbacks opcionais que xa inxecta main.js
//   onChange(content)   - SÓ para "image-upload": chámase decontado ao
//                        subir un ficheiro (o campo non ten textarea nin
//                        botón "Gardar" que agarde, ver renderImageUploadRow)
//   onSave(content)     - chámase ao premer "Gardar" con o contido final;
//                        en "image-upload" recibe o camiño da última imaxe
//                        subida (o mesmo que xa aplicou onChange)
//   onCancel()          - chámase ao premer "Cancelar"/Esc/clic fóra; quen
//                        chama decide se iso implica eliminar o bloque
//                        (isNew) - este módulo non sabe nada de bloques
let modalEl = null
let activeCtx = null // { isNew, onCancel } - só o necesario para pechar

function ensureModal() {
  if (modalEl) return modalEl
  const overlay = document.createElement('div')
  overlay.className = 'overlay hidden'
  const modal = document.createElement('div')
  modal.className = 'modal'
  overlay.appendChild(modal)
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) closeModal()
  })
  document.body.appendChild(overlay)
  modalEl = overlay
  return overlay
}

function closeModal() {
  if (!modalEl) return
  modalEl.classList.add('hidden')
  const ctx = activeCtx
  activeCtx = null
  if (ctx) ctx.onCancel()
}

function onKeydown(e) {
  if (e.key !== 'Escape') return
  if (activeCtx) { e.preventDefault(); closeModal() }
}
let keydownAttached = false
function ensureKeydownListener() {
  if (keydownAttached) return
  keydownAttached = true
  document.addEventListener('keydown', onKeydown)
}

// attachAutoClose engade autopechado de ()/[]/{}/""/'' a un <textarea>
// plano - ver blocks.js orixinal, mesma lóxica exacta.
const AUTO_CLOSE_PAIRS = { '(': ')', '[': ']', '{': '}', '"': '"', "'": "'" }
export function attachAutoClose(textarea) {
  textarea.addEventListener('keydown', (e) => {
    if (e.ctrlKey || e.metaKey || e.altKey) return
    const { selectionStart: start, selectionEnd: end, value } = textarea

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

// Etiquetas Matexe dispoñibles para inserir na selección do Enunciado -
// mesma lista, iconos e orde ca #matexeTagBar (modo Código).
const TAG_INSERT_TAGS = [
  { tag: 'MAT', ico: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"><path d="M5 14c1.5 0 1.5-8 3-8s1.5 8 3 8 1.5-8 3-8"/></svg>' },
  { tag: 'EVAL', ico: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"><path d="M4.5 7.5h11M4.5 12.5h11"/></svg>' },
  { tag: 'HIDE', ico: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M3 10s2.8-4.5 7-4.5S17 10 17 10s-2.8 4.5-7 4.5S3 10 3 10z"/><circle cx="10" cy="10" r="1.8"/><path d="M4 4l12 12"/></svg>' },
  { tag: 'PLOT', ico: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M3 15h14M3 15V5"/><path d="M4.5 13 8 8l2.6 2.4L16 5"/></svg>' },
  { tag: 'TEX', ico: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M4 5.5h9l3 3v8a.5.5 0 0 1-.5.5h-11.5a.5.5 0 0 1-.5-.5z"/><path d="M6.5 10.5h7M6.5 13h4.5"/></svg>' },
  { tag: 'TIKZ', ico: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M4 16 14.5 5.5a1.4 1.4 0 0 1 2 2L6 18l-3 1z"/></svg>' },
]

function wrapSelectionWithTag(textarea, tag) {
  const { selectionStart: start, selectionEnd: end, value } = textarea
  const selected = value.slice(start, end)
  textarea.setRangeText(`<${tag}>${selected}</${tag}>`, start, end, 'end')
  const cursor = start + tag.length + 2
  textarea.setSelectionRange(cursor, cursor + selected.length)
  textarea.focus()
}

function renderTagInsertRow(textarea, t) {
  const bar = document.createElement('div')
  bar.className = 'matexe-tag-bar matexe-tag-bar--inline'
  bar.setAttribute('role', 'toolbar')
  bar.setAttribute('aria-label', t('tagbar.aria', 'Inserir etiqueta Matexe'))
  for (const { tag, ico } of TAG_INSERT_TAGS) {
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.className = 'matexe-tag-btn'
    btn.innerHTML = `${ico}${tag}`
    btn.addEventListener('click', () => wrapSelectionWithTag(textarea, tag))
    bar.appendChild(btn)
  }
  return bar
}

// makeDraggable permite arrastrar `modal` premendo e arrastrando `handle`.
// Exportada tamén para blocks-ai-library-modals.js: as modais de "Exercicio
// con IA"/"Exame completo con IA" reutilizan o mesmo patrón (título coma asa)
// para poder apartar a ventá e ver o que hai detrás.
export function makeDraggable(modal, handle) {
  handle.addEventListener('mousedown', (e) => {
    if (e.button !== 0) return
    const rect = modal.getBoundingClientRect()
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

function renderImageUploadRow({ content, t, uploadImage, canUploadImage, onChange }) {
  const wrap = document.createElement('div')
  wrap.className = 'image-upload-row'

  const status = document.createElement('span')
  status.className = 'image-upload-row__status'
  status.textContent = content ? t('blocks.imageUpload.saved', 'Imaxe gardada: {path}', { path: content }) : t('blocks.imageUpload.none', 'Ningunha imaxe escollida.')
  wrap.appendChild(status)

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
        onChange(path)
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

// renderImageInsertRow móntase só no modal do anaco "text" (Enunciado):
// insire unha imaxe DENTRO do seu HTML, coas súas dimensións - a diferenza
// do bloque "Imaxe" solto (renderImageUploadRow), que é un anaco á parte e
// sen tamaño. Reutiliza o mesmo circuíto de subida; o que cambia é que aquí
// NON se toca o contido directamente, escríbese unha etiqueta <img> no
// textarea, na posición onde estaba o cursor.
function renderImageInsertRow(textarea, { t, uploadImage, canUploadImage }) {
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

// buildAIRow monta o botón ✨ e o mini-formulario de petición para o
// asistente de IA (opcional: só se se inxectou generateWithAI). Ao xerar,
// substitúe o valor do textarea - non garda por si só, o usuario aínda pode
// revisalo/editalo antes de premer "Gardar".
function buildAIRow(tipo, textarea, { t, generateWithAI }) {
  const wrap = document.createElement('div')
  wrap.className = 'ai-row'

  const toggleBtn = document.createElement('button')
  toggleBtn.type = 'button'
  toggleBtn.className = 'ai-row__toggle'
  toggleBtn.innerHTML = '<svg viewBox="0 0 20 20" fill="currentColor"><path d="M10 2.5l1.6 4.4L16 8.5l-4.4 1.6L10 14.5l-1.6-4.4L4 8.5l4.4-1.6z"/></svg><span></span>'
  toggleBtn.querySelector('span').textContent = t('blocks.aiRow.toggle', 'Xerar con IA')

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
// (calquera tipo agás "image-upload") se se inxectou abrirAsistente - abre a
// MESMA xanela de chat ca no editor de Código (instancia única en main.js).
function renderAssistantRow(tipo, titulo, textarea, { t, abrirAsistente }) {
  const wrap = document.createElement('div')
  wrap.className = 'assistant-open-row'
  const btn = document.createElement('button')
  btn.type = 'button'
  btn.className = 'assistant-open-btn'
  btn.innerHTML = '<svg viewBox="0 0 20 20" fill="currentColor"><path d="M10 2.5l1.6 4.4L16 8.5l-4.4 1.6L10 14.5l-1.6-4.4L4 8.5l4.4-1.6z"/></svg><span></span>'
  btn.querySelector('span').textContent = t('blocks.assistant.open', 'Preguntar ao asistente')
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

export function openContentModal({
  type, meta, content, isNew, t,
  uploadImage, canUploadImage, generateWithAI, abrirAsistente,
  onChange, onSave, onCancel,
}) {
  ensureKeydownListener()
  const overlay = ensureModal()
  const modal = overlay.querySelector('.modal')
  modal.innerHTML = ''
  // O modal reutilízase entre aperturas: limpar tamén a posición arrastrada
  // (makeDraggable) dunha apertura anterior, para que cada apertura nova
  // empece sempre centrada.
  modal.style.position = ''
  modal.style.left = ''
  modal.style.top = ''
  modal.style.margin = ''
  modal.className = 'modal' + (type !== 'image-upload' ? ' modal--wide' : '')

  activeCtx = { isNew, onCancel }

  const h2 = document.createElement('h2')
  h2.className = 'modal-drag-handle'
  h2.innerHTML = `<span aria-hidden="true">${meta.ico}</span> ${meta.label}`
  modal.appendChild(h2)
  makeDraggable(modal, h2)

  let getContent = () => content
  if (type === 'image-upload') {
    // O contido deste tipo non vive nun textarea: vaise actualizando co
    // camiño da última imaxe subida. Hai que lembralo aquí para que
    // "Gardar" non devolva o `content` de cando se abriu a modal (baleiro
    // nun anaco novo) e borre a imaxe que onChange xa aplicara.
    let current = content
    const onImageChange = (path) => { current = path; onChange(path) }
    getContent = () => current
    modal.appendChild(renderImageUploadRow({ content, t, uploadImage, canUploadImage, onChange: onImageChange }))
  } else {
    const textarea = document.createElement('textarea')
    textarea.rows = 16
    textarea.spellcheck = type === 'text'
    textarea.value = content
    attachAutoClose(textarea)

    if (type === 'text') {
      modal.appendChild(renderTagInsertRow(textarea, t))
    }

    const label = document.createElement('label')
    label.className = 'modal-label'
    label.textContent = meta.label
    label.appendChild(textarea)
    modal.appendChild(label)
    getContent = () => textarea.value
    requestAnimationFrame(() => { textarea.focus(); textarea.select() })

    if (type === 'text') {
      modal.appendChild(renderImageInsertRow(textarea, { t, uploadImage, canUploadImage }))
    }
    if (typeof generateWithAI === 'function') {
      modal.appendChild(buildAIRow(type, textarea, { t, generateWithAI }))
    }
    if (typeof abrirAsistente === 'function') {
      modal.appendChild(renderAssistantRow(type, meta.label, textarea, { t, abrirAsistente }))
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
  cancelBtn.addEventListener('click', () => closeModal())
  const saveBtn = document.createElement('button')
  saveBtn.type = 'button'
  saveBtn.className = 'primary'
  saveBtn.textContent = t('blocks.modal.save', 'Gardar')
  saveBtn.addEventListener('click', () => {
    const finalContent = getContent()
    modalEl.classList.add('hidden')
    activeCtx = null
    onSave(finalContent)
  })
  actions.appendChild(cancelBtn)
  actions.appendChild(saveBtn)
  modal.appendChild(actions)

  overlay.classList.remove('hidden')
}
