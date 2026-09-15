// @vitest-environment jsdom
//
// Probas da modal de contido (blocks-modal.js), centradas no anaco "Imaxe"
// (type "image-upload"): é o único que non ten textarea, así que o valor
// que devolve "Gardar" ten que vir do camiño da última imaxe subida e non
// do contido de cando se abriu a modal.
import { describe, it, expect, vi } from 'vitest'
import { openContentModal } from './blocks-modal.js'
import { elementMeta } from './blocks-meta.js'

const META = elementMeta()
const t = (clave, fallback) => fallback ?? clave

// O <input type="file"> créase dentro do manexador e nunca se engade ao
// DOM, así que non se pode buscar: interceptamos .click() para simular que
// o usuario escolleu un ficheiro.
function conFicheiroEscollido(fn) {
  const realClick = HTMLInputElement.prototype.click
  HTMLInputElement.prototype.click = function () {
    if (this.type !== 'file') return realClick.call(this)
    Object.defineProperty(this, 'files', { value: [new File(['x'], 'foto.png')] })
    this.dispatchEvent(new Event('change'))
  }
  try { return fn() } finally { HTMLInputElement.prototype.click = realClick }
}

async function tics(n = 4) {
  for (let i = 0; i < n; i++) await Promise.resolve()
}

// blocks-modal.js cachea un único overlay a nivel de módulo e reutilízao
// (limpando o seu contido) en cada apertura: non o quitamos do DOM entre
// probas, ou a seguinte apertura escribiría nun elemento xa desconectado.
function abrirImaxe({ content = '', onChange = () => {}, onSave = () => {} } = {}) {
  openContentModal({
    type: 'image-upload',
    meta: META['image-upload'],
    content,
    isNew: true,
    t,
    uploadImage: async () => 'imaxes/foto.png',
    canUploadImage: () => null,
    onChange,
    onSave,
    onCancel: () => {},
  })
  return document.querySelector('.overlay .modal')
}

describe('openContentModal — anaco "Imaxe"', () => {
  it('"Gardar" devolve o camiño da imaxe subida, non o contido inicial', async () => {
    const onSave = vi.fn()
    const modal = abrirImaxe({ onSave })

    conFicheiroEscollido(() => modal.querySelector('.image-upload-row__btn').click())
    await tics()

    modal.querySelector('.modal-actions button.primary').click()
    expect(onSave).toHaveBeenCalledWith('imaxes/foto.png')
  })

  it('avisa do cambio decontado ao subir, sen agardar por "Gardar"', async () => {
    const onChange = vi.fn()
    const modal = abrirImaxe({ onChange })

    conFicheiroEscollido(() => modal.querySelector('.image-upload-row__btn').click())
    await tics()

    expect(onChange).toHaveBeenCalledWith('imaxes/foto.png')
  })

  it('sen subir nada, "Gardar" conserva a imaxe que xa tiña o anaco', () => {
    const onSave = vi.fn()
    const modal = abrirImaxe({ content: 'imaxes/vella.png', onSave })

    modal.querySelector('.modal-actions button.primary').click()
    expect(onSave).toHaveBeenCalledWith('imaxes/vella.png')
  })
})
