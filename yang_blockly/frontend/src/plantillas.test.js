// @vitest-environment jsdom
//
// Probas da UI de plantillas (src/plantillas.js). O módulo non fala nunca
// cos bindings de Wails: recibe un `api` inxectado, así que aquí abonda un
// dobre en memoria para cubrir os camiños que importan (escoller, validar
// antes de gardar, e que o asistente de IA encha os campos).
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { abrirBibliotecaPlantillas, abrirEditorPlantilla } from './plantillas.js'

const t = (clave, fallback, vars) => {
  let val = fallback ?? clave
  if (vars) for (const k in vars) val = val.replaceAll(`{${k}}`, vars[k])
  return val
}

function apiFalsa(plantillas = [], activa = '') {
  return {
    listar: vi.fn(async () => ({ plantillas, activa })),
    gardar: vi.fn(async (p) => ({ ...p, id: p.id || 'nova-id', imaxes: [] })),
    eliminar: vi.fn(async () => {}),
    duplicar: vi.fn(async () => ({ id: 'copia' })),
    escoller: vi.fn(async () => {}),
    subirImaxe: vi.fn(async () => 'logo.png'),
    eliminarImaxe: vi.fn(async () => {}),
    previsualizar: vi.fn(async () => ({ pageImages: ['data:image/png;base64,AA'], warnings: [] })),
    xerarIA: vi.fn(async () => ({
      nome: 'Orzamento IA',
      descricion: 'Feita pola IA',
      latex: '\\section{Orzamento}\n{{CORPO}}',
      markdown: '# Orzamento\n\n{{CORPO}}',
      motor: 'xelatex',
      variables: [{ nome: 'EMPRESA', etiqueta: 'Empresa', valor: '' }],
    })),
    exportar: vi.fn(async () => '/casa/prof/Exame de instituto.json'),
    importar: vi.fn(async () => ({ id: 'imp1', nome: 'Circular do IES', imaxes: [] })),
  }
}

const EXEMPLO = {
  id: 'p1', nome: 'Exame de instituto', descricion: 'Cabeceira do centro',
  latex: 'CABECEIRA\n{{CORPO}}', markdown: '', motor: '', variables: [], imaxes: [], deFabrica: true,
}

function modalVisible(clase) {
  return document.querySelector(`.overlay:not(.hidden) .${clase}`)
}

// As dúas ventás reutilizan sempre o mesmo overlay (créase unha vez e
// gárdase no módulo, coma as demais modais de Yang), así que NON se pode
// baleirar document.body entre probas: quedaría un nodo solto fóra do DOM.
// Abonda con pechalas todas.
beforeEach(() => {
  document.querySelectorAll('.overlay').forEach((o) => o.classList.add('hidden'))
})
afterEach(() => {
  document.querySelectorAll('.overlay').forEach((o) => o.classList.add('hidden'))
})

describe('biblioteca de plantillas', () => {
  it('lista as plantillas gardadas máis a opción "Ningunha"', async () => {
    await abrirBibliotecaPlantillas({ t, api: apiFalsa([EXEMPLO], 'p1'), ficheiroActual: () => '' })
    const cards = document.querySelectorAll('.overlay:not(.hidden) .plantilla-card')
    expect(cards.length).toBe(2)
    expect(cards[0].textContent).toContain('Ningunha')
    expect(cards[1].textContent).toContain('Exame de instituto')
    // A activa márcase e non ofrece "Usar" outra vez.
    expect(cards[1].classList.contains('plantilla-card--activa')).toBe(true)
  })

  it('"Usar" escolle a plantilla para o documento aberto e avisa a main.js', async () => {
    const api = apiFalsa([EXEMPLO], '')
    const onEscoller = vi.fn()
    await abrirBibliotecaPlantillas({ t, api, ficheiroActual: () => '/exames/mat.matex', onEscoller })

    const cards = document.querySelectorAll('.overlay:not(.hidden) .plantilla-card')
    cards[1].querySelector('button.primary').click()
    await vi.waitFor(() => expect(api.escoller).toHaveBeenCalled())
    expect(api.escoller).toHaveBeenCalledWith('p1', '/exames/mat.matex')
    expect(onEscoller).toHaveBeenCalledWith('p1')
  })

  it('eliminar pide confirmación (diálogo propio, non window.confirm)', async () => {
    const api = apiFalsa([EXEMPLO], '')
    await abrirBibliotecaPlantillas({ t, api, ficheiroActual: () => '' })
    const card = document.querySelectorAll('.overlay:not(.hidden) .plantilla-card')[1]
    ;[...card.querySelectorAll('button')].find((b) => b.textContent === 'Eliminar').click()

    const dialogo = await vi.waitUntil(() => document.querySelector('.overlay--dialog'))
    expect(dialogo.textContent).toContain('Exame de instituto')
    dialogo.querySelector('.modal-actions button.primary').click()
    await vi.waitFor(() => expect(api.eliminar).toHaveBeenCalledWith('p1'))
  })

  it('"Ningunha" desactiva a plantilla (id baleira)', async () => {
    const api = apiFalsa([EXEMPLO], 'p1')
    await abrirBibliotecaPlantillas({ t, api, ficheiroActual: () => '/a.matex' })
    document.querySelectorAll('.overlay:not(.hidden) .plantilla-card')[0]
      .querySelector('button.primary').click()
    await vi.waitFor(() => expect(api.escoller).toHaveBeenCalledWith('', '/a.matex'))
  })

  it('cada plantilla ofrece "Exportar" e amosa a ruta na que se gardou', async () => {
    const api = apiFalsa([EXEMPLO], '')
    await abrirBibliotecaPlantillas({ t, api, ficheiroActual: () => '' })
    const card = document.querySelectorAll('.overlay:not(.hidden) .plantilla-card')[1]
    ;[...card.querySelectorAll('button')].find((b) => b.textContent.includes('Exportar')).click()
    await vi.waitFor(() => expect(api.exportar).toHaveBeenCalledWith('p1'))
    await vi.waitFor(() => expect(
      document.querySelector('.overlay:not(.hidden) .plantillas-toolbar-estado').textContent,
    ).toContain('/casa/prof/Exame de instituto.json'))
  })

  it('"Importar" (ao lado de Crear con IA) engade a plantilla e recarga a lista', async () => {
    const api = apiFalsa([EXEMPLO], '')
    const onEscoller = vi.fn()
    await abrirBibliotecaPlantillas({ t, api, ficheiroActual: () => '', onEscoller })
    const barra = document.querySelector('.overlay:not(.hidden) .plantillas-toolbar')
    ;[...barra.querySelectorAll('button')].find((b) => b.textContent.includes('Importar')).click()
    await vi.waitFor(() => expect(api.importar).toHaveBeenCalled())
    await vi.waitFor(() => expect(onEscoller).toHaveBeenCalled())
    await vi.waitFor(() => expect(
      document.querySelector('.overlay:not(.hidden) .plantillas-toolbar-estado').textContent,
    ).toContain('Circular do IES'))
  })

  it('importar cancelado (sen id) non toca a barra de estado', async () => {
    const api = apiFalsa([EXEMPLO], '')
    api.importar = vi.fn(async () => ({ id: '', nome: '' }))
    await abrirBibliotecaPlantillas({ t, api, ficheiroActual: () => '' })
    const barra = document.querySelector('.overlay:not(.hidden) .plantillas-toolbar')
    ;[...barra.querySelectorAll('button')].find((b) => b.textContent.includes('Importar')).click()
    await vi.waitFor(() => expect(api.importar).toHaveBeenCalled())
    expect(document.querySelector('.overlay:not(.hidden) .plantillas-toolbar-estado').textContent).toBe('')
  })
})

describe('editor de plantillas', () => {
  it('unha plantilla nova xa trae {{CORPO}} de exemplo', async () => {
    await abrirEditorPlantilla({ t, api: apiFalsa(), plantilla: null })
    const codigo = modalVisible('plantilla-codigo')
    expect(codigo.value).toContain('{{CORPO}}')
  })

  it('non garda sen nome, nin sen {{CORPO}}', async () => {
    const api = apiFalsa()
    await abrirEditorPlantilla({ t, api, plantilla: null })
    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    const gardarBtn = [...modal.querySelectorAll('.modal-actions button')].find((b) => b.textContent === 'Gardar')

    gardarBtn.click()
    await vi.waitFor(() => expect(modal.querySelector('.plantilla-estado').textContent).toContain('nome'))
    expect(api.gardar).not.toHaveBeenCalled()

    modal.querySelector('.modal-label input[type="text"]').value = 'A miña plantilla'
    modal.querySelector('.plantilla-codigo').value = 'sen marcador'
    gardarBtn.click()
    await vi.waitFor(() => expect(modal.querySelector('.plantilla-estado').textContent).toContain('{{CORPO}}'))
    expect(api.gardar).not.toHaveBeenCalled()
  })

  it('garda nome, código, Markdown, motor e variables', async () => {
    const api = apiFalsa()
    const onGardada = vi.fn()
    await abrirEditorPlantilla({ t, api, plantilla: { ...EXEMPLO, id: '', variables: [] }, onGardada })
    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    const inputs = modal.querySelectorAll('.modal-label input[type="text"]')
    inputs[0].value = 'Orzamento'
    inputs[1].value = 'Membrete da empresa'
    modal.querySelectorAll('.plantilla-codigo')[1].value = '# Orzamento\n\n{{CORPO}}'
    modal.querySelector('select').value = 'xelatex'

    // Unha variable engadida a man.
    ;[...modal.querySelectorAll('.plantillas-toolbar button')]
      .find((b) => b.textContent.includes('Engadir variable')).click()
    const varFila = modal.querySelector('.plantilla-var')
    const varInputs = varFila.querySelectorAll('input')
    varInputs[0].value = 'empresa'
    varInputs[0].dispatchEvent(new Event('input'))
    varInputs[2].value = 'Talleres Xistral'
    varInputs[2].dispatchEvent(new Event('input'))

    ;[...modal.querySelectorAll('.modal-actions button')].find((b) => b.textContent === 'Gardar').click()
    await vi.waitFor(() => expect(api.gardar).toHaveBeenCalled())

    const gardada = api.gardar.mock.calls[0][0]
    expect(gardada.nome).toBe('Orzamento')
    expect(gardada.descricion).toBe('Membrete da empresa')
    expect(gardada.latex).toContain('{{CORPO}}')
    expect(gardada.markdown).toContain('{{CORPO}}')
    expect(gardada.motor).toBe('xelatex')
    expect(gardada.variables).toEqual([{ nome: 'empresa', etiqueta: '', valor: 'Talleres Xistral' }])
    expect(onGardada).toHaveBeenCalled()
  })

  it('o asistente de IA enche os campos sen gardar nada', async () => {
    const api = apiFalsa()
    await abrirEditorPlantilla({ t, api, plantilla: null, abrirIA: true })
    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    modal.querySelector('.plantilla-ia textarea').value = 'un orzamento'
    modal.querySelector('.plantilla-ia__accions button').click()

    await vi.waitFor(() => expect(api.xerarIA).toHaveBeenCalled())
    await vi.waitFor(() => expect(modal.querySelectorAll('.plantilla-codigo')[0].value).toContain('Orzamento'))
    expect(modal.querySelectorAll('.plantilla-codigo')[1].value).toContain('{{CORPO}}')
    expect(modal.querySelector('select').value).toBe('xelatex')
    expect(modal.querySelector('.plantilla-var__nome').value).toBe('EMPRESA')
    expect(api.gardar).not.toHaveBeenCalled()
  })

  it('a vista previa amosa as páxinas que devolve o backend', async () => {
    const api = apiFalsa()
    await abrirEditorPlantilla({ t, api, plantilla: EXEMPLO })
    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    ;[...modal.querySelectorAll('.modal-actions button')].find((b) => b.textContent.includes('Vista previa')).click()
    await vi.waitFor(() => expect(modal.querySelectorAll('.plantilla-previa__paxina').length).toBe(1))
    expect(api.previsualizar).toHaveBeenCalled()
    expect(api.previsualizar.mock.calls[0][0].latex).toContain('{{CORPO}}')
  })

  it('as imaxes ofrecen o código exacto para LaTeX e para Markdown', async () => {
    const api = apiFalsa()
    await abrirEditorPlantilla({ t, api, plantilla: { ...EXEMPLO, imaxes: ['logo.png'] } })
    const modal = document.querySelector('.overlay:not(.hidden) .modal')
    const fila = modal.querySelector('.plantilla-imaxe')
    expect(fila.querySelector('code').textContent).toBe('logo.png')

    const [latexArea, mdArea] = modal.querySelectorAll('.plantilla-codigo')
    ;[...fila.querySelectorAll('button')].find((b) => b.textContent.includes('LaTeX')).click()
    ;[...fila.querySelectorAll('button')].find((b) => b.textContent.includes('Markdown')).click()
    expect(latexArea.value).toContain('\\includegraphics[height=1.5cm]{logo.png}')
    expect(mdArea.value).toContain('![](images/logo.png)')
  })
})
