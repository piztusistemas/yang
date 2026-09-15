// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { createExplorer } from './explorer.js'

let container
afterEach(() => {
  if (container) container.remove()
  container = null
  vi.restoreAllMocks()
})

function mount(opts) {
  container = document.createElement('div')
  document.body.appendChild(container)
  return createExplorer(container, opts)
}

// flush: agarda a que remate calquera render() en curso (async - listDir
// devolve unha Promise, ver explorer.js) antes de inspeccionar o DOM.
async function flush() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

// responderDialogo preme un botón do diálogo de src/dialogs.js. Renomear/
// eliminar/nova carpeta xa NON usan window.prompt/confirm (en macOS o
// WKWebView de Wails devólveos baleiros sen preguntar, ver a cabeceira de
// dialogs.js), así que a proba manexa a modal de verdade: o overlay xa está
// no DOM en canto se preme a acción, porque abrirDialogo() móntao de
// maneira síncrona antes do primeiro await.
async function responderDialogo({ aceptar, texto }) {
  const overlay = document.querySelector('.overlay--dialog')
  expect(overlay).toBeTruthy()
  if (texto !== undefined) overlay.querySelector('input[type="text"]').value = texto
  const botons = overlay.querySelectorAll('.modal-actions button')
  ;(aceptar ? overlay.querySelector('.modal-actions button.primary') : botons[0]).click()
  await flush()
}

describe('createExplorer', () => {
  it('sen initialRoot amosa "Abrir cartafol"; premelo escolle raíz e redebuxa', async () => {
    const chooseFolder = vi.fn().mockResolvedValue('/home/prof/exames')
    const listDir = vi.fn().mockResolvedValue([])
    const explorer = mount({ chooseFolder, listDir })
    await flush()

    const btn = container.querySelector('.explorer-empty__btn')
    expect(btn).toBeTruthy()
    btn.click()
    await flush()

    expect(chooseFolder).toHaveBeenCalled()
    expect(listDir).toHaveBeenCalledWith('/home/prof/exames')
    expect(container.querySelector('.explorer-header__name').textContent).toBe('exames')
    explorer.destroy()
  })

  it('con initialRoot pide os fillos e ordena cartafoles antes ca ficheiros', async () => {
    const listDir = vi.fn().mockResolvedValue([
      { name: 'b.matex', path: '/raiz/b.matex', isDir: false },
      { name: 'sub', path: '/raiz/sub', isDir: true },
    ])
    const explorer = mount({ initialRoot: '/raiz', listDir })
    await flush()

    expect(listDir).toHaveBeenCalledWith('/raiz')
    const rows = container.querySelectorAll('.explorer-row__label')
    expect(Array.from(rows).map((r) => r.textContent)).toEqual(['b.matex', 'sub'])
    explorer.destroy()
  })

  it('premer un cartafol despregue e pide os seus fillos (lazy load)', async () => {
    const listDir = vi.fn((path) => {
      if (path === '/raiz') return Promise.resolve([{ name: 'sub', path: '/raiz/sub', isDir: true }])
      if (path === '/raiz/sub') return Promise.resolve([{ name: 'dentro.matex', path: '/raiz/sub/dentro.matex', isDir: false }])
      return Promise.resolve([])
    })
    const explorer = mount({ initialRoot: '/raiz', listDir })
    await flush()

    expect(container.querySelectorAll('.explorer-row').length).toBe(1)
    container.querySelector('.explorer-row--dir').click()
    await flush()

    expect(listDir).toHaveBeenCalledWith('/raiz/sub')
    const labels = Array.from(container.querySelectorAll('.explorer-row__label')).map((r) => r.textContent)
    expect(labels).toEqual(['sub', 'dentro.matex'])
    explorer.destroy()
  })

  it('premer un ficheiro .matex chama onOpenFile coa súa ruta', async () => {
    const listDir = vi.fn().mockResolvedValue([{ name: 'exame.matex', path: '/raiz/exame.matex', isDir: false }])
    const onOpenFile = vi.fn()
    const explorer = mount({ initialRoot: '/raiz', listDir, onOpenFile })
    await flush()

    container.querySelector('.explorer-row--matex').click()
    expect(onOpenFile).toHaveBeenCalledWith('/raiz/exame.matex')
    explorer.destroy()
  })

  it('premer un ficheiro que NON é .matex non chama onOpenFile', async () => {
    const listDir = vi.fn().mockResolvedValue([{ name: 'logo.png', path: '/raiz/logo.png', isDir: false }])
    const onOpenFile = vi.fn()
    const explorer = mount({ initialRoot: '/raiz', listDir, onOpenFile })
    await flush()

    container.querySelector('.explorer-row--file').click()
    expect(onOpenFile).not.toHaveBeenCalled()
    explorer.destroy()
  })

  it('✏️ Renomear pide o nome novo nun diálogo propio e chama rename(path, novoNome)', async () => {
    const listDir = vi.fn().mockResolvedValue([{ name: 'a.matex', path: '/raiz/a.matex', isDir: false }])
    const rename = vi.fn().mockResolvedValue('/raiz/b.matex')
    const explorer = mount({ initialRoot: '/raiz', listDir, rename })
    await flush()

    container.querySelector('[aria-label="Renomear"]').click()
    await responderDialogo({ aceptar: true, texto: 'b.matex' })

    expect(rename).toHaveBeenCalledWith('/raiz/a.matex', 'b.matex')
    explorer.destroy()
  })

  it('✏️ Renomear cancelado non chama rename()', async () => {
    const listDir = vi.fn().mockResolvedValue([{ name: 'a.matex', path: '/raiz/a.matex', isDir: false }])
    const rename = vi.fn().mockResolvedValue('/raiz/b.matex')
    const explorer = mount({ initialRoot: '/raiz', listDir, rename })
    await flush()

    container.querySelector('[aria-label="Renomear"]').click()
    await responderDialogo({ aceptar: false })

    expect(rename).not.toHaveBeenCalled()
    explorer.destroy()
  })

  it('🗑 Eliminar pide confirmación e só chama remove() se se acepta', async () => {
    const listDir = vi.fn().mockResolvedValue([{ name: 'a.matex', path: '/raiz/a.matex', isDir: false }])
    const remove = vi.fn().mockResolvedValue()
    const explorer = mount({ initialRoot: '/raiz', listDir, remove })
    await flush()

    container.querySelector('[aria-label="Eliminar"]').click()
    await responderDialogo({ aceptar: false })
    expect(remove).not.toHaveBeenCalled()

    container.querySelector('[aria-label="Eliminar"]').click()
    await responderDialogo({ aceptar: true })
    expect(remove).toHaveBeenCalledWith('/raiz/a.matex')
    explorer.destroy()
  })

  it('📁+ Nova carpeta (cabeceira) pide un nome e chama newFolder(root, nome)', async () => {
    const listDir = vi.fn().mockResolvedValue([])
    const newFolder = vi.fn().mockResolvedValue('/raiz/novo')
    const explorer = mount({ initialRoot: '/raiz', listDir, newFolder })
    await flush()

    container.querySelector('.explorer-header__actions [aria-label="Nova carpeta"]').click()
    await responderDialogo({ aceptar: true, texto: 'novo' })

    expect(newFolder).toHaveBeenCalledWith('/raiz', 'novo')
    explorer.destroy()
  })

  it('un erro de listDir amósase en .explorer-status', async () => {
    const listDir = vi.fn().mockRejectedValue(new Error('non existe'))
    const explorer = mount({ initialRoot: '/raiz', listDir })
    await flush()

    const status = container.querySelector('.explorer-status')
    expect(status.classList.contains('hidden')).toBe(false)
    expect(status.textContent).toBe('non existe')
    explorer.destroy()
  })

  it('sen uploadFile non amosa o botón "⬆️ Subir ficheiro"', async () => {
    const listDir = vi.fn().mockResolvedValue([])
    const explorer = mount({ initialRoot: '/raiz', listDir })
    await flush()

    expect(container.querySelector('[aria-label="Subir ficheiro (imaxe ou PDF)"]')).toBeNull()
    explorer.destroy()
  })

  // simulateUpload: dispara o input de ficheiro OCULTO que garda promptUpload
  // (un só, reutilizado - ver explorer.js) coma se a usuaria escollese
  // `file` no selector nativo. files é un accessor read-only en calquera
  // <input type="file"> real - Object.defineProperty é o xeito estándar de
  // simulalo en jsdom (non hai DataTransfer completo dispoñible aquí).
  async function simulateUpload(input, file) {
    Object.defineProperty(input, 'files', { value: [file], configurable: true })
    input.dispatchEvent(new Event('change'))
    await flush()
  }

  it('⬆️ Subir ficheiro (cabeceira) sube á raíz e copia <IMG src> ao portapapeis para unha imaxe', async () => {
    const listDir = vi.fn().mockResolvedValue([])
    const uploadFile = vi.fn().mockResolvedValue({ path: '/raiz/logo.png', relPath: 'logo.png' })
    const writeText = vi.fn().mockResolvedValue()
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const explorer = mount({ initialRoot: '/raiz', listDir, uploadFile })
    await flush()

    container.querySelector('.explorer-header__actions [aria-label="Subir ficheiro (imaxe ou PDF)"]').click()
    const file = new File(['abc'], 'logo.png', { type: 'image/png' })
    await simulateUpload(container.querySelector('input[type="file"]'), file)

    expect(uploadFile).toHaveBeenCalledWith('/raiz', file)
    expect(writeText).toHaveBeenCalledWith('<IMG src="logo.png">')
    expect(container.querySelector('.explorer-status--info').textContent).toContain('logo.png')
    explorer.destroy()
  })

  it('⬆️ Subir ficheiro nunha fila de cartafol sube AÍ (non á raíz) e xera <A href> para un PDF', async () => {
    const listDir = vi.fn((path) => Promise.resolve(
      path === '/raiz' ? [{ name: 'sub', path: '/raiz/sub', isDir: true }] : [],
    ))
    const uploadFile = vi.fn().mockResolvedValue({ path: '/raiz/sub/modelo.pdf', relPath: 'sub/modelo.pdf' })
    const writeText = vi.fn().mockResolvedValue()
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const explorer = mount({ initialRoot: '/raiz', listDir, uploadFile })
    await flush()

    container.querySelector('.explorer-row__actions [aria-label="Subir ficheiro (imaxe ou PDF)"]').click()
    const file = new File(['%PDF'], 'modelo.pdf', { type: 'application/pdf' })
    await simulateUpload(container.querySelector('input[type="file"]'), file)

    expect(uploadFile).toHaveBeenCalledWith('/raiz/sub', file)
    expect(writeText).toHaveBeenCalledWith('<A href="sub/modelo.pdf">modelo.pdf</A>')
    explorer.destroy()
  })

  it('⬆️ Subir ficheiro sen relPath (exame sen gardar) amosa aviso e non copia nada', async () => {
    const listDir = vi.fn().mockResolvedValue([])
    const uploadFile = vi.fn().mockResolvedValue({ path: '/raiz/logo.png', relPath: '' })
    const writeText = vi.fn().mockResolvedValue()
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const explorer = mount({ initialRoot: '/raiz', listDir, uploadFile })
    await flush()

    container.querySelector('.explorer-header__actions [aria-label="Subir ficheiro (imaxe ou PDF)"]').click()
    const file = new File(['abc'], 'logo.png', { type: 'image/png' })
    await simulateUpload(container.querySelector('input[type="file"]'), file)

    expect(writeText).not.toHaveBeenCalled()
    expect(container.querySelector('.explorer-status').textContent).toContain('Garda o exame')
    explorer.destroy()
  })

  it('destroy() baleira o contedor', async () => {
    const listDir = vi.fn().mockResolvedValue([])
    const explorer = mount({ initialRoot: '/raiz', listDir })
    await flush()
    explorer.destroy()
    expect(container.innerHTML).toBe('')
  })
})
