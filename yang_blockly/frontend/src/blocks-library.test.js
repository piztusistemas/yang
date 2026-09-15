// @vitest-environment jsdom
//
// Proba de integración DOM (a diferenza de blocks-serialize.test.js, que
// proba lóxica pura sen DOM): exercita createBlocksEditor real nun DOM
// jsdom, coma se fose a app en marcha, para comprobar a función
// "Biblioteca" (💾 gardar / 📚 cargar / 🗑 eliminar). Backend simulado en
// memoria, coa mesma forma que os bindings Wails reais
// (GardarNaBiblioteca/ListarBiblioteca/EliminarDaBiblioteca).
//
// O botón de toolbar "📚 Biblioteca" (o único xeito de abrir openLibraryModal)
// quitouse a propósito - "Gardar exame na biblioteca"/"Biblioteca" eran
// redundantes con Gardar/Gardar como (petición do usuario, confirmado
// explicitamente que os botóns "💾 Gardar este bloque/exercicio" individuais
// quedan sen forma de consultalos despois). openLibraryModal segue existindo
// en blocks.js por se se re-conecta a outra entrada no futuro, pero as dúas
// probas de máis abaixo que dependen dese botón non teñen xa como executarse
// - marcadas skip en vez de borradas, coma documentación do que cubrían.
import { describe, it, expect } from 'vitest'
import { createBlocksEditor } from './blocks.js'

function makeFakeBackend() {
  const items = []
  let nextId = 1
  return {
    items,
    saveToLibrary: async (nome, tipo, contido) => {
      const item = { id: String(nextId++), nome, tipo, contido, creado: new Date().toISOString() }
      items.push(item)
      return item
    },
    listLibrary: async () => [...items].reverse(),
    deleteFromLibrary: async (id) => {
      const idx = items.findIndex((it) => it.id === id)
      if (idx !== -1) items.splice(idx, 1)
    },
  }
}

async function flush() {
  // deixa correr os microtasks pendentes (as chamadas async ao backend
  // simulado) antes de mirar o DOM resultante.
  await new Promise((r) => setTimeout(r, 0))
}

describe('Biblioteca (integración real de blocks.js nun DOM jsdom)', () => {
  // Sen o botón de toolbar "📚 Biblioteca" non hai xeito de chegar a
  // openLibraryModal dende un test de integración DOM coma este (non está
  // exportado) - ver nota de máis arriba.
  it.skip('garda un exercicio, aparece na biblioteca, insírese, e elimínase', async () => {
    const backend = makeFakeBackend()
    const container = document.createElement('div')
    document.body.appendChild(container)
    const editor = createBlocksEditor(container, backend)

    // 1) Crear un exercicio sinxelo.
    const addBtn = container.querySelector('.blocks-toolbar__add')
    expect(addBtn).toBeTruthy()
    addBtn.click()
    expect(container.querySelectorAll('.exercise-card').length).toBe(1)

    // 2) Gardar ese exercicio na biblioteca (botón 💾 na tarxeta).
    const saveBtn = [...container.querySelectorAll('.exercise-card__actions .icon-btn')]
      .find((b) => b.title.includes('biblioteca'))
    expect(saveBtn).toBeTruthy()
    saveBtn.click()

    const saveModal = document.body.querySelector('.overlay:not(.hidden) .modal')
    expect(saveModal).toBeTruthy()
    expect(saveModal.textContent).toContain('Gardar exercicio na biblioteca')
    const nameInput = saveModal.querySelector('input[type="text"]')
    nameInput.value = 'Exercicio de proba'
    nameInput.dispatchEvent(new Event('input'))
    const gardarBtn = [...saveModal.querySelectorAll('.modal-actions button')].find((b) => b.textContent === 'Gardar')
    gardarBtn.click()
    await flush()

    expect(backend.items.length).toBe(1)
    expect(backend.items[0].nome).toBe('Exercicio de proba')
    expect(backend.items[0].tipo).toBe('exercicio')

    // 3) Abrir "📚 Biblioteca" e comprobar que aparece listado.
    const libToolbarBtn = [...container.querySelectorAll('.blocks-toolbar__ai')].find((b) => b.textContent.includes('Biblioteca'))
    expect(libToolbarBtn).toBeTruthy()
    libToolbarBtn.click()
    await flush()

    let libModal = [...document.body.querySelectorAll('.overlay:not(.hidden) .modal')].find((m) => m.textContent.includes('Biblioteca'))
    expect(libModal).toBeTruthy()
    const rows = libModal.querySelectorAll('.library-item')
    expect(rows.length).toBe(1)
    expect(rows[0].textContent).toContain('Exercicio de proba')

    // 4) "Inserir" engade un exercicio novo ao exame.
    const insertBtn = rows[0].querySelector('.library-item__actions button.primary')
    expect(insertBtn.textContent).toBe('Inserir')
    insertBtn.click()
    expect(container.querySelectorAll('.exercise-card').length).toBe(2)

    // O modal de biblioteca péchase ao inserir.
    expect(document.body.querySelector('.overlay:not(.hidden) .library-list')).toBeFalsy()

    // 5) Eliminar da biblioteca: reabrir, premer 🗑, desaparece da lista.
    libToolbarBtn.click()
    await flush()
    libModal = [...document.body.querySelectorAll('.overlay:not(.hidden) .modal')].find((m) => m.textContent.includes('Biblioteca'))
    const delBtn = [...libModal.querySelectorAll('.library-item__actions .icon-btn')].find((b) => b.title.includes('Eliminar'))
    delBtn.click()
    await flush()
    expect(backend.items.length).toBe(0)
    expect(libModal.querySelectorAll('.library-item').length).toBe(0)
    expect(libModal.textContent).toContain('Aínda non gardaches nada')

    editor.destroy()
  })

  it.skip('gardar un só bloque e inserilo créao/engádeo ao exercicio aberto', async () => {
    const backend = makeFakeBackend()
    const container = document.createElement('div')
    document.body.appendChild(container)
    createBlocksEditor(container, backend)

    container.querySelector('.blocks-toolbar__add').click()
    // "+ Engadir exercicio" xa deixa o exercicio expandido (expandedId), non
    // fai falla premer a cabeceira - facelo alternaría a expansión a pechado.
    const paletteText = container.querySelector('.palette-block--text')
    paletteText.click()
    // O modal de contido do anaco ábrese automaticamente (isNew) - gardar co texto por defecto.
    const elModal = document.body.querySelector('.overlay:not(.hidden) .modal')
    const saveElBtn = [...elModal.querySelectorAll('.modal-actions button')].find((b) => b.textContent === 'Gardar')
    saveElBtn.click()

    expect(container.querySelectorAll('.element-row').length).toBe(1)

    // Gardar ese anaco na biblioteca.
    const saveBlockBtn = [...container.querySelectorAll('.element-row__actions .icon-btn')].find((b) => b.title.includes('bloque'))
    saveBlockBtn.click()
    const saveModal = document.body.querySelector('.overlay:not(.hidden) .modal')
    const nameInput = saveModal.querySelector('input[type="text"]')
    nameInput.value = 'Cabeceira IES'
    nameInput.dispatchEvent(new Event('input'))
    ;[...saveModal.querySelectorAll('.modal-actions button')].find((b) => b.textContent === 'Gardar').click()
    await flush()
    expect(backend.items[0].tipo).toBe('bloque')

    const libBtn = [...container.querySelectorAll('.blocks-toolbar__ai')].find((b) => b.textContent.includes('Biblioteca'))

    // O exercicio segue expandido (insertElement así o deixa) - "Inserir"
    // engade o bloque a ESE exercicio, non crea un novo.
    expect(container.querySelectorAll('.exercise-card').length).toBe(1)
    libBtn.click()
    await flush()
    let libModal = [...document.body.querySelectorAll('.overlay:not(.hidden) .modal')].find((m) => m.textContent.includes('Biblioteca'))
    libModal.querySelector('.library-item__actions button.primary').click()
    expect(container.querySelectorAll('.exercise-card').length).toBe(1)
    expect(container.querySelectorAll('.element-row').length).toBe(2)

    // Sen ningún exercicio expandido, "Inserir" crea un exercicio novo.
    container.querySelector('.exercise-card__head').click() // colapsa (toggle)
    libBtn.click()
    await flush()
    libModal = [...document.body.querySelectorAll('.overlay:not(.hidden) .modal')].find((m) => m.textContent.includes('Biblioteca'))
    libModal.querySelector('.library-item__actions button.primary').click()
    expect(container.querySelectorAll('.exercise-card').length).toBe(2)
  })
})
