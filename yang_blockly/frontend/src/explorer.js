// createExplorer - explorador de cartafoles/ficheiros estilo VS Code (ver
// captura no pedido orixinal): árbore navegable do sistema de ficheiros
// real, á esquerda do editor, pensada para moverse entre varios exames/
// prácticas dun mesmo cartafol de traballo sen ter que abrir o selector
// nativo "Abrir" cada vez (ver openFile en main.js, que segue existindo
// tal cal - este explorador é un camiño ALTERNATIVO, non o substitúe).
//
// Mesmo contrato "factoría illada" ca editor.js/blocks-editor.js: recibe o
// contedor e unhas pontes (listDir/chooseFolder/newFolder/rename/remove),
// nunca fala directamente cos bindings Wails - main.js resolve esas pontes
// (ListarCartafol/EscollerCartafolDialog/NovoCartafol/RenomearFicheiro/
// EliminarFicheiro, ver explorer.go), mesmo criterio ca uploadImageFile/
// generateWithAI para o editor de bloques. Abrir un .matex (onOpenFile) NON
// pasa por aquí: precisa lóxica só de main.js (preguntar por cambios sen
// gardar, currentPath...), así que este módulo só avisa CAL se premeu, sen
// lelo el mesmo (ver LerFicheiroTexto, chamado dende main.js).
//
// Estado (raíz, cartafoles despregados) vive SÓ neste módulo, en memoria -
// non hai persistencia propia aquí: main.js garda o ÚLTIMO cartafol
// aberto en Settings.UltimoCartafol (onRootChanged) e pásao de novo coma
// initialRoot a próxima vez que se cree o explorador (arranque de Yang).
// ICONS: SVG en liña (non emoji) para a árbore e as súas accións - mesmo
// estilo de trazo cás demais iconas da app (stroke-width 1.6-1.8, sen
// recheo). Definidas unha soa vez aquí, reutilizadas por makeActionBtn e
// polos spans .explorer-row__icon/__chevron.
import { confirmar, preguntar } from './dialogs.js'

const ICONS = {
  chevronClosed: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 5l5 5-5 5"/></svg>',
  chevronOpen: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 8l5 5 5-5"/></svg>',
  folderClosed: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 5.5a1 1 0 0 1 1-1h4l1.5 2h7a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1h-12.5a1 1 0 0 1-1-1z"/></svg>',
  folderOpen: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 6.5V5a1 1 0 0 1 1-1h4l1.5 2h7a1 1 0 0 1 1 1v.5"/><path d="M2 8.3a.8.8 0 0 1 .8-.9h13.8a.8.8 0 0 1 .78.98l-1 4.3a1 1 0 0 1-.97.82H3.4a1 1 0 0 1-.97-.77z"/></svg>',
  file: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M6 2.5h5.5L15 6v11a.5.5 0 0 1-.5.5h-8a.5.5 0 0 1-.5-.5v-14a.5.5 0 0 1 .5-.5z"/><path d="M11.5 2.5V6H15"/></svg>',
  newFolder: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 5.5a1 1 0 0 1 1-1h4l1.5 2h7a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1h-12.5a1 1 0 0 1-1-1z"/><path d="M10 9v4M8 11h4"/></svg>',
  upload: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13V5M6.5 8.5 10 5l3.5 3.5"/><path d="M4 14.5v1a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1v-1"/></svg>',
  rename: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M4 16 14.5 5.5a1.4 1.4 0 0 1 2 2L6 18l-3 1z"/></svg>',
  trash: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M4.5 5.5h11"/><path d="M8 5.5V4a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v1.5"/><path d="M5.5 5.5 6.2 16a1 1 0 0 0 1 .9h5.6a1 1 0 0 0 1-.9l.7-10.5"/></svg>',
  refresh: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M15.5 6.5A6 6 0 1 0 16.5 10"/><path d="M15.5 2.5v4h-4"/></svg>',
}

export function createExplorer(container, {
  initialRoot, listDir, chooseFolder, newFolder, rename, remove, uploadFile,
  onOpenFile, onRootChanged, t: tIn,
} = {}) {
  const t = tIn || ((clave, fallback, vars) => {
    let val = fallback ?? clave
    if (vars) for (const k in vars) val = val.replaceAll(`{${k}}`, vars[k])
    return val
  })

  let root = initialRoot || ''
  // expanded: rutas de cartafol despregadas - persiste entre re-renders
  // (render() redebúxase enteira en cada cambio, máis simple ca parchear
  // só o anaco tocado, ver comentario en render() - sen isto perderíase o
  // despregado ao renomear/eliminar/crear calquera cousa na árbore).
  const expanded = new Set()
  // renderToken evita renders solapados: cada chamada a render() súmalle 1
  // e comproba que segue a ser a última despois de cada `await` - se non,
  // aborta (chegou outro render() máis novo mentres agardaba por listDir).
  let renderToken = 0

  container.innerHTML = ''
  container.classList.add('explorer-sidebar')

  const statusEl = document.createElement('p')
  statusEl.className = 'explorer-status hidden'
  function showError(err) {
    statusEl.className = 'explorer-status'
    statusEl.textContent = err && err.message ? err.message : String(err)
  }
  // showInfo: mesma barra ca showError pero en ton neutro/positivo (ver
  // .explorer-status--info, style.css) - usada só pola confirmación de
  // "⬆️ Subir ficheiro" (promptUpload embaixo), que NON é un erro.
  function showInfo(msg) {
    statusEl.className = 'explorer-status explorer-status--info'
    statusEl.textContent = msg
  }
  function clearError() {
    statusEl.classList.add('hidden')
  }

  function isMatex(name) {
    return /\.matex$/i.test(name)
  }

  // imageExtensionRe/isImageFile: decide que etiqueta Matexe pegar tras
  // subir (ver promptUpload) - unha imaxe vai coma <IMG src="...">
  // (renderImg, latexdoc.go/markdowndoc.go xa saben amosala inline), calquera
  // outra cousa (normalmente un PDF de referencia) coma <A href="...">nome
  // </A>, unha ligazón clicable (renderLink, mesmos ficheiros).
  const imageExtensionRe = /\.(png|jpe?g|gif|svg|webp|bmp)$/i
  function isImageFile(name) {
    return imageExtensionRe.test(name)
  }

  // ---------- "⬆️ Subir ficheiro" (imaxe ou PDF a calquera cartafol) ----------
  // Un único <input type="file"> oculto, reutilizado para calquera fila
  // (mesmo patrón cá subida de imaxes do editor de bloques, ver
  // uploadImageFile/main.js) - uploadTargetDir garda A QUE cartafol vai esta
  // subida concreta (o premido en promptUpload), lido só unha vez no
  // 'change' que segue ao clic sintético de abaixo.
  const uploadInput = document.createElement('input')
  uploadInput.type = 'file'
  uploadInput.accept = 'image/*,.pdf,application/pdf'
  uploadInput.className = 'hidden'
  let uploadTargetDir = null
  uploadInput.addEventListener('change', async () => {
    const file = uploadInput.files && uploadInput.files[0]
    uploadInput.value = '' // permite escoller o MESMO ficheiro outra vez
    const destDir = uploadTargetDir
    uploadTargetDir = null
    if (!file || !destDir || typeof uploadFile !== 'function') return
    try {
      const { relPath } = await uploadFile(destDir, file)
      if (!relPath) {
        showError(t('explorer.upload.needSave', 'Garda o exame primeiro (Ctrl+S) para poder enlazar ficheiros subidos.'))
        return
      }
      const snippet = isImageFile(file.name)
        ? `<IMG src="${relPath}">`
        : `<A href="${relPath}">${file.name}</A>`
      let copied = false
      try {
        await navigator.clipboard.writeText(snippet)
        copied = true
      } catch {
        // Sen portapapeis dispoñible (contexto non seguro, permiso
        // denegado...): o aviso de embaixo amosa igualmente o snippet
        // enteiro, para copialo á man seleccionándoo.
      }
      showInfo(copied
        ? t('explorer.upload.copied', 'Copiado: {snippet} - pégao onde o precises.', { snippet })
        : t('explorer.upload.snippet', 'Subido. Copia isto onde o precises: {snippet}', { snippet }))
      render() // o novo ficheiro xa aparece na árbore se destDir estaba despregado
    } catch (err) {
      showError(err)
    }
  })

  function promptUpload(destDir) {
    if (typeof uploadFile !== 'function') return
    uploadTargetDir = destDir
    uploadInput.click()
  }

  async function chooseRoot() {
    try {
      const path = await chooseFolder()
      if (!path) return
      root = path
      expanded.clear()
      clearError()
      if (typeof onRootChanged === 'function') onRootChanged(path)
      render()
    } catch (err) {
      showError(err)
    }
  }

  async function promptNewFolder(parentPath) {
    const nome = await preguntar(t('explorer.newFolderPrompt', 'Nome da nova carpeta:'), '')
    if (!nome) return
    expanded.add(parentPath)
    newFolder(parentPath, nome)
      .then(() => { clearError(); render() })
      .catch(showError)
  }

  async function promptRename(entry) {
    const nome = await preguntar(t('explorer.renamePrompt', 'Novo nome:'), entry.name)
    if (!nome || nome === entry.name) return
    rename(entry.path, nome)
      .then(() => { clearError(); render() })
      .catch(showError)
  }

  async function confirmDelete(entry) {
    const msg = t(
      'explorer.deleteConfirm',
      'Isto eliminará PERMANENTEMENTE "{name}" - non hai papeleira. Continuar?',
      { name: entry.name },
    )
    if (!await confirmar(msg)) return
    remove(entry.path)
      .then(() => { expanded.delete(entry.path); clearError(); render() })
      .catch(showError)
  }

  function toggleFolder(entry) {
    if (expanded.has(entry.path)) expanded.delete(entry.path)
    else expanded.add(entry.path)
    render()
  }

  // makeActionBtn: icona pequena revelada ao pasar por riba da fila (mesmo
  // patrón ca .exercise-tile__action en blocks-exercises-panel.js) -
  // stopPropagation para non disparar tamén o clic da fila (que despregaría
  // un cartafol ou abriría un .matex).
  function makeActionBtn(icon, label, onClick) {
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.className = 'explorer-row__action'
    btn.title = label
    btn.setAttribute('aria-label', label)
    btn.innerHTML = icon
    btn.addEventListener('click', (e) => { e.stopPropagation(); onClick() })
    return btn
  }

  async function renderChildren(parentPath, depth, into, token) {
    let entries
    try {
      entries = await listDir(parentPath)
    } catch (err) {
      if (token !== renderToken) return
      showError(err)
      return
    }
    if (token !== renderToken) return
    for (const entry of entries) {
      await renderNode(entry, depth, into, token)
      if (token !== renderToken) return
    }
  }

  async function renderNode(entry, depth, into, token) {
    const row = document.createElement('div')
    row.className = 'explorer-row' + (entry.isDir ? ' explorer-row--dir' : ' explorer-row--file')
    row.style.setProperty('--depth', String(depth))
    row.setAttribute('tabindex', '0')

    const chevron = document.createElement('span')
    chevron.className = 'explorer-row__chevron'
    const icon = document.createElement('span')
    icon.className = 'explorer-row__icon'
    const label = document.createElement('span')
    label.className = 'explorer-row__label'
    label.textContent = entry.name

    const actions = document.createElement('span')
    actions.className = 'explorer-row__actions'

    if (entry.isDir) {
      const open = expanded.has(entry.path)
      chevron.innerHTML = open ? ICONS.chevronOpen : ICONS.chevronClosed
      icon.innerHTML = open ? ICONS.folderOpen : ICONS.folderClosed
      row.addEventListener('click', () => toggleFolder(entry))
      actions.appendChild(makeActionBtn(ICONS.newFolder, t('explorer.newFolder', 'Nova carpeta'), () => promptNewFolder(entry.path)))
      if (typeof uploadFile === 'function') {
        actions.appendChild(makeActionBtn(ICONS.upload, t('explorer.upload', 'Subir ficheiro (imaxe ou PDF)'), () => promptUpload(entry.path)))
      }
    } else {
      icon.innerHTML = ICONS.file
      if (isMatex(entry.name)) {
        row.classList.add('explorer-row--matex')
        row.addEventListener('click', () => { if (typeof onOpenFile === 'function') onOpenFile(entry.path) })
      }
    }
    actions.appendChild(makeActionBtn(ICONS.rename, t('explorer.rename', 'Renomear'), () => promptRename(entry)))
    actions.appendChild(makeActionBtn(ICONS.trash, t('explorer.delete', 'Eliminar'), () => confirmDelete(entry)))

    row.appendChild(chevron)
    row.appendChild(icon)
    row.appendChild(label)
    row.appendChild(actions)
    into.appendChild(row)

    if (entry.isDir && expanded.has(entry.path)) {
      const childWrap = document.createElement('div')
      into.appendChild(childWrap)
      await renderChildren(entry.path, depth + 1, childWrap, token)
    }
  }

  function renderEmptyState() {
    const empty = document.createElement('div')
    empty.className = 'explorer-empty'
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.className = 'explorer-empty__btn'
    btn.innerHTML = ICONS.folderClosed + '<span></span>'
    btn.querySelector('span').textContent = t('explorer.openFolder', 'Abrir cartafol')
    btn.addEventListener('click', chooseRoot)
    empty.appendChild(btn)
    container.appendChild(empty)
  }

  function renderHeader() {
    const header = document.createElement('div')
    header.className = 'explorer-header'
    const name = document.createElement('span')
    name.className = 'explorer-header__name'
    name.textContent = root.split(/[/\\]/).filter(Boolean).pop() || root
    name.title = root
    header.appendChild(name)

    const actions = document.createElement('span')
    actions.className = 'explorer-header__actions'
    actions.appendChild(makeActionBtn(ICONS.refresh, t('explorer.refresh', 'Actualizar'), () => render()))
    actions.appendChild(makeActionBtn(ICONS.newFolder, t('explorer.newFolder', 'Nova carpeta'), () => promptNewFolder(root)))
    if (typeof uploadFile === 'function') {
      actions.appendChild(makeActionBtn(ICONS.upload, t('explorer.upload', 'Subir ficheiro (imaxe ou PDF)'), () => promptUpload(root)))
    }
    actions.appendChild(makeActionBtn(ICONS.folderOpen, t('explorer.changeFolder', 'Cambiar cartafol'), () => chooseRoot()))
    header.appendChild(actions)

    container.appendChild(header)
  }

  async function render() {
    const token = ++renderToken
    container.innerHTML = ''
    container.appendChild(statusEl)
    container.appendChild(uploadInput)
    if (!root) {
      renderEmptyState()
      return
    }
    renderHeader()
    const tree = document.createElement('div')
    tree.className = 'explorer-tree'
    container.appendChild(tree)
    await renderChildren(root, 0, tree, token)
  }

  render()

  return {
    destroy: () => {
      container.innerHTML = ''
      container.classList.remove('explorer-sidebar')
    },
  }
}
