// Biblioteca de PLANTILLAS de documento (a maqueta: cabeceira do centro,
// membrete dun orzamento, marxes...). O backend está en plantillas.go; aquí
// só hai UI, e toda a comunicación pasa polo obxecto `api` que inxecta
// main.js (mesmo criterio ca blocks.js cos seus callbacks: este módulo non
// importa nunca os bindings de Wails directamente, así se pode probar sen
// backend).
//
// Dúas ventás:
//   - abrirBibliotecaPlantillas: a lista. Escoller cal se aplica, crear,
//     duplicar, editar, eliminar, e tamén importar/exportar unha plantilla
//     (ficheiro .json: texto + imaxes) para compartila cos
//     compañeiros - ver ExportarPlantilla/ImportarPlantilla en
//     plantillas_intercambio.go.
//   - o editor (abrirEditorPlantilla, tamén exportado para "✨ Crear con
//     IA"): nome, LaTeX, Markdown, variables, imaxes, vista previa real.
//
// O marcador {{CORPO}} é o contrato con plantillas.go: onde el estea vai o
// documento xerado. A UI insiste nel (axuda, validación e o botón que o
// insire) porque é o único erro que deixa unha plantilla inservible.

import { confirmar } from './dialogs.js'

const MARCADOR_CORPO = '{{CORPO}}'

function crearOverlay(clase) {
  const overlay = document.createElement('div')
  overlay.className = 'overlay hidden'
  const modal = document.createElement('div')
  modal.className = 'modal' + (clase ? ' ' + clase : '')
  overlay.appendChild(modal)
  document.body.appendChild(overlay)
  return overlay
}

function el(tag, clase, texto) {
  const n = document.createElement(tag)
  if (clase) n.className = clase
  if (texto !== undefined) n.textContent = texto
  return n
}

function boton(texto, clase, onClick) {
  const b = el('button', clase, texto)
  b.type = 'button'
  if (onClick) b.addEventListener('click', onClick)
  return b
}

function ficheiroABase64(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result).split(',')[1] || '')
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

// ---------- Ventá 1: a biblioteca ----------

let bibliotecaOverlay = null

// abrirBibliotecaPlantillas amosa a lista. onEscoller avísalle a main.js de
// que cambiou a plantilla activa (para refrescar o selector da barra e
// volver xerar), e chámase tamén tras gardar/eliminar porque calquera das
// dúas pode cambiar o que hai que amosar aí.
export async function abrirBibliotecaPlantillas({ t, api, ficheiroActual, onEscoller }) {
  if (!bibliotecaOverlay) {
    bibliotecaOverlay = crearOverlay('modal--plantillas')
    bibliotecaOverlay.addEventListener('click', (e) => {
      if (e.target === bibliotecaOverlay) bibliotecaOverlay.classList.add('hidden')
    })
  }
  const overlay = bibliotecaOverlay
  const modal = overlay.querySelector('.modal')
  const pechar = () => overlay.classList.add('hidden')

  // `aviso` sobrevive aos repintados (pintar() baleira o modal enteiro): o
  // que importa/exporta escríbeo aquí e pintar() vólveo poñer na barra de
  // estado, así o "Plantilla importada" ou o erro non desaparecen ao
  // recargar a lista.
  let aviso = ''
  const mostrarAviso = (msg) => { aviso = msg || '' }

  async function pintar() {
    modal.innerHTML = ''
    modal.appendChild(el('h2', null, '📐 ' + t('plantillas.title', 'Plantillas de documento')))
    modal.appendChild(el('p', null, t('plantillas.intro',
      'A plantilla decide a maqueta (cabeceira, membrete, marxes); o documento segue sendo o mesmo. Aplícase á vista previa e a todo o que exportes: PDF, LaTeX, Markdown, Word e OpenDocument.')))

    // Vai baixo a barra, pero decláloa antes: os manexadores dos botóns
    // (importar, exportar) escriben nela.
    const barraEstado = el('p', 'ai-row__status plantillas-toolbar-estado')
    barraEstado.textContent = aviso

    const barra = el('div', 'plantillas-toolbar')
    barra.appendChild(boton('➕ ' + t('plantillas.new', 'Nova en branco'), null, async () => {
      pechar()
      await abrirEditorPlantilla({ t, api, plantilla: null, onGardada: recargar })
    }))
    barra.appendChild(boton('✨ ' + t('plantillas.newAI', 'Crear con IA'), null, async () => {
      pechar()
      await abrirEditorPlantilla({ t, api, plantilla: null, abrirIA: true, onGardada: recargar })
    }))
    // Importar: ao lado de "Crear con IA". Trae unha plantilla que compartiu
    // un compañeiro (ficheiro .json: texto + imaxes) e engádea á
    // biblioteca coma unha máis.
    barra.appendChild(boton('📥 ' + t('plantillas.import', 'Importar'), null, async () => {
      mostrarAviso('')
      barraEstado.textContent = ''
      try {
        const nova = await api.importar()
        if (!nova || !nova.id) return // diálogo cancelado
        mostrarAviso(t('plantillas.imported', 'Plantilla "{nome}" importada.', { nome: nova.nome }))
        await pintar()
        if (onEscoller) onEscoller()
      } catch (err) {
        barraEstado.textContent = t('plantillas.importError', 'Non se puido importar: {msg}',
          { msg: err && err.message ? err.message : err })
      }
    }))
    modal.appendChild(barra)
    modal.appendChild(barraEstado)

    let estado
    try {
      estado = await api.listar()
    } catch (err) {
      modal.appendChild(el('p', 'ai-row__status', String(err)))
      return
    }

    const lista = el('div', 'plantillas-lista')
    lista.appendChild(fila({
      nome: t('plantillas.none', 'Ningunha'),
      descricion: t('plantillas.noneDesc', 'O documento sae coa maqueta por defecto de Yang.'),
      activa: !estado.activa,
      accions: [],
      onUsar: () => escoller(''),
    }))
    for (const p of estado.plantillas || []) {
      lista.appendChild(fila({
        nome: p.nome,
        descricion: p.descricion || '',
        deFabrica: p.deFabrica,
        activa: estado.activa === p.id,
        onUsar: () => escoller(p.id),
        accions: [
          boton(t('plantillas.edit', 'Editar'), null, async () => {
            pechar()
            await abrirEditorPlantilla({ t, api, plantilla: p, onGardada: recargar })
          }),
          boton(t('plantillas.duplicate', 'Duplicar'), null, async () => {
            await api.duplicar(p.id)
            await pintar()
          }),
          // Exportar: cada plantilla da biblioteca pódese gardar nun ficheiro
          // .json para pasarllo a un compañeiro.
          boton('📤 ' + t('plantillas.export', 'Exportar'), null, async () => {
            mostrarAviso('')
            barraEstado.textContent = ''
            try {
              const ruta = await api.exportar(p.id)
              if (!ruta) return // diálogo cancelado
              barraEstado.textContent = t('plantillas.exported', 'Exportada a {ruta}', { ruta })
            } catch (err) {
              barraEstado.textContent = t('plantillas.exportError', 'Non se puido exportar: {msg}',
                { msg: err && err.message ? err.message : err })
            }
          }),
          boton(t('plantillas.delete', 'Eliminar'), 'danger', async () => {
            // confirmar() e non window.confirm: en macOS o nativo devolve
            // sempre false sen preguntar (ver dialogs.js).
            if (!await confirmar(t('plantillas.confirmDelete', 'Eliminar a plantilla "{nome}"?', { nome: p.nome }))) return
            await api.eliminar(p.id)
            await pintar()
            if (onEscoller) onEscoller()
          }),
        ],
      }))
    }
    modal.appendChild(lista)

    const accions = el('div', 'modal-actions')
    accions.appendChild(boton(t('plantillas.close', 'Pechar'), 'primary', pechar))
    modal.appendChild(accions)
  }

  function fila({ nome, descricion, deFabrica, activa, onUsar, accions }) {
    const card = el('div', 'plantilla-card' + (activa ? ' plantilla-card--activa' : ''))
    const info = el('div', 'plantilla-card__info')
    const titulo = el('div', 'plantilla-card__nome', nome)
    if (deFabrica) titulo.appendChild(el('span', 'plantilla-card__badge', t('plantillas.builtin', 'de exemplo')))
    if (activa) titulo.appendChild(el('span', 'plantilla-card__badge plantilla-card__badge--activa', t('plantillas.inUse', 'en uso')))
    info.appendChild(titulo)
    if (descricion) info.appendChild(el('div', 'plantilla-card__desc', descricion))
    card.appendChild(info)

    const btns = el('div', 'plantilla-card__accions')
    btns.appendChild(boton(activa ? t('plantillas.inUse', 'en uso') : t('plantillas.use', 'Usar'), 'primary', async () => {
      if (activa) return
      await onUsar()
      await pintar()
    }))
    for (const b of accions || []) btns.appendChild(b)
    card.appendChild(btns)
    return card
  }

  async function escoller(id) {
    await api.escoller(id, ficheiroActual ? ficheiroActual() : '')
    if (onEscoller) onEscoller(id)
  }

  async function recargar() {
    await pintar()
    overlay.classList.remove('hidden')
    if (onEscoller) onEscoller()
  }

  await pintar()
  overlay.classList.remove('hidden')
}

// ---------- Ventá 2: o editor dunha plantilla ----------

let editorOverlay = null

// abrirEditorPlantilla edita unha plantilla existente ou crea unha nova
// (plantilla null). Traballa sobre unha COPIA: ata premer "Gardar" non se
// toca nada do disco, agás as imaxes - esas si se soben ao momento, porque
// precisan unha plantilla xa gardada onde vivir (por iso "🖼️ Engadir imaxe"
// garda primeiro a plantilla se aínda é nova).
export async function abrirEditorPlantilla({ t, api, plantilla, abrirIA, onGardada }) {
  if (!editorOverlay) {
    editorOverlay = crearOverlay('modal--plantilla-editor')
  }
  const overlay = editorOverlay
  const modal = overlay.querySelector('.modal')
  modal.innerHTML = ''

  // Copia de traballo. `imaxes` mantense sincronizada co backend en canto se
  // sobe/borra unha.
  let actual = plantilla
    ? JSON.parse(JSON.stringify(plantilla))
    : { id: '', nome: '', descricion: '', latex: PLANTILLA_BALEIRA, markdown: '', motor: '', variables: [], imaxes: [] }
  actual.variables = actual.variables || []
  actual.imaxes = actual.imaxes || []

  const pechar = () => overlay.classList.add('hidden')

  modal.appendChild(el('h2', null, plantilla
    ? '📐 ' + t('plantillas.editTitle', 'Editar plantilla')
    : '📐 ' + t('plantillas.newTitle', 'Plantilla nova')))

  // --- nome e descrición ---
  const nomeLabel = el('label', 'modal-label', t('plantillas.name', 'Nome'))
  const nomeInput = el('input')
  nomeInput.type = 'text'
  nomeInput.value = actual.nome
  nomeLabel.appendChild(nomeInput)
  modal.appendChild(nomeLabel)

  const descLabel = el('label', 'modal-label', t('plantillas.description', 'Descrición (unha liña)'))
  const descInput = el('input')
  descInput.type = 'text'
  descInput.value = actual.descricion
  descLabel.appendChild(descInput)
  modal.appendChild(descLabel)

  // --- asistente de IA ---
  const iaBloque = el('details', 'plantilla-ia')
  const iaResumo = el('summary', null, '✨ ' + t('plantillas.ai.title', 'Crear ou cambiar con IA'))
  iaBloque.appendChild(iaResumo)
  iaBloque.open = !!abrirIA

  const iaDesc = el('label', 'modal-label', t('plantillas.ai.describe', 'Que plantilla queres'))
  const iaTexto = el('textarea')
  iaTexto.rows = 3
  iaTexto.placeholder = t('plantillas.ai.placeholder', 'Ex.: "orzamento cunha táboa de conceptos, IVE e total, e o logo arriba á esquerda"')
  iaDesc.appendChild(iaTexto)
  iaBloque.appendChild(iaDesc)

  const iaURLLabel = el('label', 'modal-label', t('plantillas.ai.url', 'URL de referencia (opcional)'))
  const iaURL = el('input')
  iaURL.type = 'url'
  iaURL.placeholder = 'https://…'
  iaURLLabel.appendChild(iaURL)
  iaBloque.appendChild(iaURLLabel)

  const iaFicheiroFila = el('div', 'image-upload-row')
  const iaFicheiroEstado = el('span', 'image-upload-row__status', t('plantillas.ai.noSample', 'Ningún documento de mostra.'))
  iaFicheiroFila.appendChild(iaFicheiroEstado)
  const iaFicheiroLabel = el('label', 'image-upload-row__btn', t('plantillas.ai.attach', 'Documento de mostra (PDF, .tex, .md, .txt)…'))
  const iaFicheiro = el('input')
  iaFicheiro.type = 'file'
  iaFicheiro.accept = '.pdf,.tex,.md,.txt,.html'
  iaFicheiro.multiple = true
  iaFicheiro.hidden = true
  iaFicheiro.addEventListener('change', () => {
    const n = iaFicheiro.files ? iaFicheiro.files.length : 0
    iaFicheiroEstado.textContent = n === 0
      ? t('plantillas.ai.noSample', 'Ningún documento de mostra.')
      : n === 1
        ? t('plantillas.ai.oneSample', 'Mostra: {name}', { name: iaFicheiro.files[0].name })
        : t('plantillas.ai.severalSamples', '{n} documentos de mostra.', { n })
  })
  iaFicheiroLabel.appendChild(iaFicheiro)
  iaFicheiroFila.appendChild(iaFicheiroLabel)
  iaBloque.appendChild(iaFicheiroFila)

  iaBloque.appendChild(el('p', null, t('plantillas.ai.note',
    'A IA devolve a plantilla no editor de abaixo: revísaa (e proba a vista previa) antes de gardala. Se xa hai código escrito, tómao coma punto de partida en vez de empezar de cero.')))

  const iaEstado = el('p', 'ai-row__status')
  const iaAccions = el('div', 'plantilla-ia__accions')
  const iaBoton = boton('✨ ' + t('plantillas.ai.generate', 'Xerar plantilla'), 'primary', async () => {
    const documentos = []
    for (const f of Array.from(iaFicheiro.files || [])) {
      documentos.push({ fileName: f.name, dataB64: await ficheiroABase64(f) })
    }
    if (!iaTexto.value.trim() && !iaURL.value.trim() && documentos.length === 0) {
      iaEstado.textContent = t('plantillas.ai.needInput', 'Describe a plantilla, achega un documento ou indica unha URL.')
      return
    }
    iaBoton.disabled = true
    iaEstado.textContent = t('plantillas.ai.working', 'Xerando…')
    try {
      const proposta = await api.xerarIA({
        descricion: iaTexto.value.trim(),
        url: iaURL.value.trim(),
        documentos,
        imaxes: actual.imaxes,
        base: latexArea.value.trim() ? { ...actual, latex: latexArea.value, markdown: mdArea.value } : null,
      })
      if (!nomeInput.value.trim()) nomeInput.value = proposta.nome || ''
      if (!descInput.value.trim()) descInput.value = proposta.descricion || ''
      latexArea.value = proposta.latex || ''
      mdArea.value = proposta.markdown || ''
      motorSelect.value = proposta.motor || ''
      actual.variables = proposta.variables || []
      pintarVariables()
      iaEstado.textContent = t('plantillas.ai.done', 'Listo: revisa o código e proba a vista previa.')
    } catch (err) {
      iaEstado.textContent = t('plantillas.ai.error', 'Erro: {msg}', { msg: err && err.message ? err.message : err })
    } finally {
      iaBoton.disabled = false
    }
  })
  iaAccions.appendChild(iaBoton)
  iaBloque.appendChild(iaAccions)
  iaBloque.appendChild(iaEstado)
  modal.appendChild(iaBloque)

  // --- LaTeX ---
  const latexLabel = el('label', 'modal-label', t('plantillas.latex', 'LaTeX da plantilla'))
  const latexArea = el('textarea', 'plantilla-codigo')
  latexArea.rows = 14
  latexArea.spellcheck = false
  latexArea.value = actual.latex
  latexLabel.appendChild(latexArea)
  modal.appendChild(latexLabel)

  const axuda = el('p', 'plantilla-axuda')
  axuda.innerHTML = t('plantillas.latexHelp',
    'Escribe <code>{{CORPO}}</code> onde queiras que vaia o documento. Se non poñes <code>\\documentclass</code>, Yang engade o seu preámbulo de sempre (abonda para cabeceira e pé); se o poñes, mandas ti e Yang só engade os paquetes que precisan os gráficos e as imaxes. Os datos fixos ponos coma variables <code>{{CENTRO}}</code>, <code>{{MATERIA}}</code>… Xa existen <code>{{DATA}}</code>, <code>{{ANO}}</code>, <code>{{TITULO}}</code> e <code>{{FICHEIRO}}</code>.')
  modal.appendChild(axuda)

  const latexBarra = el('div', 'plantillas-toolbar')
  latexBarra.appendChild(boton(t('plantillas.insertBody', 'Inserir {{CORPO}}'), null, () => {
    inserirNoCursor(latexArea, MARCADOR_CORPO)
  }))
  modal.appendChild(latexBarra)

  // --- Markdown ---
  const mdLabel = el('label', 'modal-label', t('plantillas.markdown', 'Markdown equivalente (opcional: .md, Word e OpenDocument)'))
  const mdArea = el('textarea', 'plantilla-codigo')
  mdArea.rows = 6
  mdArea.spellcheck = false
  mdArea.value = actual.markdown
  mdLabel.appendChild(mdArea)
  modal.appendChild(mdLabel)
  modal.appendChild(el('p', 'plantilla-axuda', t('plantillas.markdownHelp',
    'Mesmo {{CORPO}} e mesmas variables, pero en Markdown. Se o deixas baleiro, esas tres exportacións saen sen maqueta, coma antes. As imaxes referéncianse aquí coma images/logo.png.')))

  // --- motor ---
  const motorLabel = el('label', 'modal-label', t('plantillas.engine', 'Compilador'))
  const motorSelect = el('select')
  for (const [valor, texto] of [
    ['', t('plantillas.engineDefault', 'O de Opcións (por defecto)')],
    ['pdflatex', 'pdflatex'],
    ['xelatex', 'xelatex'],
    ['lualatex', 'lualatex'],
  ]) {
    const opt = el('option', null, texto)
    opt.value = valor
    motorSelect.appendChild(opt)
  }
  motorSelect.value = actual.motor || ''
  motorLabel.appendChild(motorSelect)
  modal.appendChild(motorLabel)

  // --- variables ---
  modal.appendChild(el('h3', 'plantilla-seccion', t('plantillas.variables', 'Variables')))
  const varsWrap = el('div', 'plantilla-vars')
  modal.appendChild(varsWrap)
  const varsBarra = el('div', 'plantillas-toolbar')
  varsBarra.appendChild(boton('➕ ' + t('plantillas.addVar', 'Engadir variable'), null, () => {
    actual.variables.push({ nome: '', etiqueta: '', valor: '' })
    pintarVariables()
  }))
  modal.appendChild(varsBarra)

  function pintarVariables() {
    varsWrap.innerHTML = ''
    if (actual.variables.length === 0) {
      varsWrap.appendChild(el('p', 'plantilla-axuda', t('plantillas.noVars',
        'Sen variables. Engade unha para que un dato fixo (o centro, a empresa…) se poida cambiar sen tocar o LaTeX.')))
      return
    }
    actual.variables.forEach((v, i) => {
      const fila = el('div', 'plantilla-var')
      const nome = el('input', 'plantilla-var__nome')
      nome.type = 'text'
      nome.placeholder = t('plantillas.varName', 'NOME')
      nome.value = v.nome
      nome.addEventListener('input', () => { v.nome = nome.value })
      const etiqueta = el('input')
      etiqueta.type = 'text'
      etiqueta.placeholder = t('plantillas.varLabel', 'Etiqueta')
      etiqueta.value = v.etiqueta
      etiqueta.addEventListener('input', () => { v.etiqueta = etiqueta.value })
      const valor = el('input')
      valor.type = 'text'
      valor.placeholder = t('plantillas.varValue', 'Valor')
      valor.value = v.valor
      valor.addEventListener('input', () => { v.valor = valor.value })
      fila.appendChild(nome)
      fila.appendChild(etiqueta)
      fila.appendChild(valor)
      fila.appendChild(boton(t('plantillas.insert', 'Inserir'), null, () => {
        inserirNoCursor(latexArea, '{{' + (v.nome || 'NOME').toUpperCase() + '}}')
      }))
      fila.appendChild(boton('🗑', 'danger', () => {
        actual.variables.splice(i, 1)
        pintarVariables()
      }))
      varsWrap.appendChild(fila)
    })
  }
  pintarVariables()

  // --- imaxes ---
  modal.appendChild(el('h3', 'plantilla-seccion', '🖼️ ' + t('plantillas.images', 'Imaxes da plantilla')))
  modal.appendChild(el('p', 'plantilla-axuda', t('plantillas.imagesHelp',
    'Gárdanse coa plantilla (non a carón do documento), así que valen para calquera exame ou orzamento, estea onde estea o ficheiro.')))
  const imaxesWrap = el('div', 'plantilla-imaxes')
  modal.appendChild(imaxesWrap)

  const imaxeFila = el('div', 'image-upload-row')
  const imaxeEstado = el('span', 'image-upload-row__status')
  imaxeFila.appendChild(imaxeEstado)
  const imaxeLabel = el('label', 'image-upload-row__btn', t('plantillas.addImage', 'Engadir imaxe…'))
  const imaxeInput = el('input')
  imaxeInput.type = 'file'
  imaxeInput.accept = 'image/*'
  imaxeInput.hidden = true
  imaxeInput.addEventListener('change', async () => {
    const file = imaxeInput.files && imaxeInput.files[0]
    if (!file) return
    imaxeInput.value = ''
    try {
      // Unha plantilla sen gardar aínda non ten onde meter a imaxe: gárdase
      // primeiro (co que haxa escrito) e despois sóbese.
      if (!actual.id) {
        imaxeEstado.textContent = t('plantillas.savingFirst', 'Gardando a plantilla antes de engadir a imaxe…')
        await gardar({ pechar: false })
        if (!actual.id) return
      }
      imaxeEstado.textContent = t('plantillas.uploading', 'Subindo…')
      const nome = await api.subirImaxe({ id: actual.id, fileName: file.name, dataB64: await ficheiroABase64(file) })
      actual.imaxes.push(nome)
      imaxeEstado.textContent = ''
      pintarImaxes()
    } catch (err) {
      imaxeEstado.textContent = String(err)
    }
  })
  imaxeLabel.appendChild(imaxeInput)
  imaxeFila.appendChild(imaxeLabel)
  modal.appendChild(imaxeFila)

  function pintarImaxes() {
    imaxesWrap.innerHTML = ''
    if (actual.imaxes.length === 0) {
      imaxesWrap.appendChild(el('p', 'plantilla-axuda', t('plantillas.noImages', 'Aínda non hai imaxes.')))
      return
    }
    for (const nome of actual.imaxes) {
      const fila = el('div', 'plantilla-imaxe')
      fila.appendChild(el('code', null, nome))
      fila.appendChild(boton(t('plantillas.insertLatex', 'Inserir no LaTeX'), null, () => {
        inserirNoCursor(latexArea, '\\includegraphics[height=1.5cm]{' + nome + '}')
      }))
      fila.appendChild(boton(t('plantillas.insertMd', 'Inserir no Markdown'), null, () => {
        inserirNoCursor(mdArea, '![](images/' + nome + ')')
      }))
      fila.appendChild(boton('🗑', 'danger', async () => {
        await api.eliminarImaxe(actual.id, nome)
        actual.imaxes = actual.imaxes.filter((n) => n !== nome)
        pintarImaxes()
      }))
      imaxesWrap.appendChild(fila)
    }
  }
  pintarImaxes()

  // --- vista previa ---
  const previaWrap = el('div', 'plantilla-previa')
  modal.appendChild(previaWrap)

  // --- accións ---
  // Clase propia ademais de .ai-row__status (que xa usa o asistente de IA
  // deste mesmo editor): son dous avisos distintos e teñen que poder
  // distinguirse, tanto ao estilalos coma nas probas.
  const estado = el('p', 'ai-row__status plantilla-estado')
  modal.appendChild(estado)
  const accions = el('div', 'modal-actions')
  const previaBtn = boton('👁 ' + t('plantillas.preview', 'Vista previa'), null, async () => {
    previaBtn.disabled = true
    estado.textContent = t('plantillas.previewing', 'Compilando a vista previa…')
    previaWrap.innerHTML = ''
    try {
      const res = await api.previsualizar(datosActuais())
      estado.textContent = (res.warnings || []).join(' · ')
      for (const img of res.pageImages || []) {
        const im = el('img', 'plantilla-previa__paxina')
        im.src = img
        previaWrap.appendChild(im)
      }
      if (!(res.pageImages || []).length) {
        previaWrap.appendChild(el('p', 'plantilla-axuda', t('plantillas.previewNoPages',
          'Compilou, pero non se puido debuxar a vista previa (falta pdftoppm).')))
      }
    } catch (err) {
      estado.textContent = String(err)
    } finally {
      previaBtn.disabled = false
    }
  })
  accions.appendChild(previaBtn)
  accions.appendChild(boton(t('plantillas.cancel', 'Cancelar'), null, pechar))
  accions.appendChild(boton(t('plantillas.save', 'Gardar'), 'primary', () => gardar({ pechar: true })))
  modal.appendChild(accions)

  function datosActuais() {
    return {
      ...actual,
      nome: nomeInput.value.trim(),
      descricion: descInput.value.trim(),
      latex: latexArea.value,
      markdown: mdArea.value,
      motor: motorSelect.value,
      variables: actual.variables,
    }
  }

  async function gardar({ pechar: pecharAoRematar }) {
    const datos = datosActuais()
    if (!datos.nome) {
      estado.textContent = t('plantillas.needName', 'Dálle un nome á plantilla.')
      return
    }
    if (!datos.latex.includes(MARCADOR_CORPO)) {
      estado.textContent = t('plantillas.needBody', 'Falta {{CORPO}}: sen el Yang non sabe onde meter o documento.')
      return
    }
    try {
      const gardada = await api.gardar(datos)
      actual = { ...gardada, variables: gardada.variables || [], imaxes: gardada.imaxes || [] }
      pintarVariables()
      pintarImaxes()
      estado.textContent = t('plantillas.saved', 'Plantilla gardada.')
      if (pecharAoRematar) {
        pechar()
        if (onGardada) await onGardada(actual)
      }
    } catch (err) {
      estado.textContent = String(err)
    }
  }

  overlay.classList.remove('hidden')
  requestAnimationFrame(() => nomeInput.focus())
}

// inserirNoCursor mete texto onde estea o cursor do textarea (ou ao final se
// nunca tivo foco) - o mesmo que fan os botóns de etiquetas do editor de
// código.
function inserirNoCursor(area, texto) {
  const inicio = area.selectionStart ?? area.value.length
  const fin = area.selectionEnd ?? area.value.length
  area.value = area.value.slice(0, inicio) + texto + area.value.slice(fin)
  area.focus()
  area.selectionStart = area.selectionEnd = inicio + texto.length
}

// PLANTILLA_BALEIRA é o punto de partida dunha plantilla nova: un fragmento
// (sen \documentclass) cunha cabeceira mínima, para que quen a abra vexa de
// primeiras como se usan {{CORPO}} e unha variable.
const PLANTILLA_BALEIRA = `\\begin{center}
  {\\Large\\bfseries {{TITULO}} }
\\end{center}
\\hrule
\\vspace{5mm}

{{CORPO}}
`
