// @vitest-environment jsdom
//
// Probas de src/dialogs.js - os diálogos HTML que substitúen a
// window.confirm/prompt/alert (que en macOS non amosan nada baixo o
// WKWebView de Wails, ver a cabeceira de dialogs.js). O contrato que
// importa aquí é o VALOR devolto: ten que coincidir co do nativo
// equivalente para que os `if (!await confirmar(...)) return` de quen chama
// se comporten igual ca antes en Linux/Windows.
import { describe, it, expect, afterEach } from 'vitest'
import { confirmar, preguntar, avisar } from './dialogs.js'

afterEach(() => {
  document.querySelectorAll('.overlay--dialog').forEach((el) => el.remove())
})

function dialogo() {
  const overlay = document.querySelector('.overlay--dialog')
  expect(overlay).toBeTruthy()
  const botons = overlay.querySelectorAll('.modal-actions button')
  return {
    overlay,
    mensaxe: overlay.querySelector('.dialog-message').textContent,
    input: overlay.querySelector('input[type="text"]'),
    botons,
    cancelar: botons[0],
    aceptar: overlay.querySelector('.modal-actions button.primary'),
  }
}

function teclear(key) {
  document.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
}

describe('confirmar', () => {
  it('monta o diálogo de contado e resolve true ao aceptar', async () => {
    const p = confirmar('Seguro?')
    // Síncrono: quen chama xa pode agardar sen carreiras.
    const d = dialogo()
    expect(d.mensaxe).toBe('Seguro?')
    d.aceptar.click()
    expect(await p).toBe(true)
    expect(document.querySelector('.overlay--dialog')).toBeNull()
  })

  it('resolve false ao cancelar', async () => {
    const p = confirmar('Seguro?')
    dialogo().cancelar.click()
    expect(await p).toBe(false)
  })

  it('Esc resolve false; clic fóra da modal tamén', async () => {
    const p1 = confirmar('Seguro?')
    teclear('Escape')
    expect(await p1).toBe(false)

    const p2 = confirmar('Seguro?')
    dialogo().overlay.click()
    expect(await p2).toBe(false)
  })

  it('Enter acepta', async () => {
    const p = confirmar('Seguro?')
    teclear('Enter')
    expect(await p).toBe(true)
  })

  it('a mensaxe vai como texto, non como HTML (nomes de ficheiro do usuario)', async () => {
    const p = confirmar('Eliminar "<b>a</b>.matex"?')
    const d = dialogo()
    expect(d.overlay.querySelector('b')).toBeNull()
    expect(d.mensaxe).toBe('Eliminar "<b>a</b>.matex"?')
    d.cancelar.click()
    await p
  })
})

describe('preguntar', () => {
  it('arrinca co valor por defecto e resolve o texto escrito', async () => {
    const p = preguntar('Novo nome:', 'a.matex')
    const d = dialogo()
    expect(d.input.value).toBe('a.matex')
    d.input.value = 'b.matex'
    d.aceptar.click()
    expect(await p).toBe('b.matex')
  })

  it('resolve null ao cancelar (coma o prompt nativo)', async () => {
    const p = preguntar('Novo nome:', 'a.matex')
    dialogo().cancelar.click()
    expect(await p).toBeNull()
  })

  it('Enter no campo acepta', async () => {
    const p = preguntar('Novo nome:', 'a.matex')
    const d = dialogo()
    d.input.value = 'c.matex'
    d.input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    expect(await p).toBe('c.matex')
  })

  it('unha cadea baleira devólvese tal cal, NON como null', async () => {
    const p = preguntar('Novo nome:', '')
    dialogo().aceptar.click()
    expect(await p).toBe('')
  })
})

describe('avisar', () => {
  it('só ten botón de aceptar e resolve ao premelo', async () => {
    const p = avisar('Feito.')
    const d = dialogo()
    expect(d.botons.length).toBe(1)
    expect(d.input).toBeNull()
    d.aceptar.click()
    expect(await p).toBeUndefined()
    expect(document.querySelector('.overlay--dialog')).toBeNull()
  })
})

describe('varios diálogos', () => {
  it('poden apilarse: o de arriba resólvese só unha vez aínda que se prema dúas', async () => {
    const p = confirmar('Seguro?')
    const d = dialogo()
    d.aceptar.click()
    d.aceptar.click()
    d.cancelar.click()
    expect(await p).toBe(true)
  })
})
