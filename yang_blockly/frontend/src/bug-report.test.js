// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from 'vitest'
import { openBugReportModal } from './bug-report.js'

afterEach(() => {
  // NON limpar document.body.innerHTML: o overlay de openBugReportModal é
  // un singleton a nivel de módulo (mesmo patrón ca blocks-ai-library-
  // modals.js/blocks-exercises-panel.js) que se reutiliza entre chamadas -
  // desconectalo do DOM aquí deixaría ese singleton apuntando a un nodo xa
  // fóra da árbore, e as próximas probas non atoparían nada con
  // document.querySelector. Cada test chama openBugReportModal() de novo,
  // que xa limpa/reconstrúe .modal por dentro.
  vi.restoreAllMocks()
})

function modal() {
  return document.querySelector('.overlay:not(.hidden) .modal')
}

describe('openBugReportModal', () => {
  it('xera o informe ao abrir (generateReport recibe anaco/mensaxeErro) e activa Gardar/Enviar cando remata', async () => {
    const generateReport = vi.fn().mockResolvedValue('## Diagnóstico\nAlgo fallou.')
    const saveReport = vi.fn()
    openBugReportModal({ anaco: 'doc .matex', mensaxeErro: 'erro de Maxima', generateReport, saveReport })

    expect(generateReport).toHaveBeenCalledWith('doc .matex', 'erro de Maxima')
    const textarea = modal().querySelector('textarea')
    expect(textarea.disabled).toBe(true)

    await Promise.resolve()
    await Promise.resolve()

    expect(textarea.value).toBe('## Diagnóstico\nAlgo fallou.')
    expect(textarea.disabled).toBe(false)
    const buttons = modal().querySelectorAll('.modal-actions button')
    for (const b of buttons) expect(b.disabled).toBe(false)
  })

  it('"💾 Gardar como…" chama saveReport co texto (posiblemente editado) do textarea', async () => {
    const generateReport = vi.fn().mockResolvedValue('informe orixinal')
    const saveReport = vi.fn().mockResolvedValue('/tmp/informe-erro-yang.md')
    openBugReportModal({ anaco: 'x', mensaxeErro: 'y', generateReport, saveReport })
    await Promise.resolve()
    await Promise.resolve()

    const textarea = modal().querySelector('textarea')
    textarea.value = 'informe editado a man'
    const saveBtn = Array.from(modal().querySelectorAll('.modal-actions button')).find((b) => b.textContent.includes('Gardar'))
    saveBtn.click()
    await Promise.resolve()
    await Promise.resolve()

    expect(saveReport).toHaveBeenCalledWith('informe-erro-yang.md', 'informe editado a man')
  })

  it('"📧 Enviar por correo" abre un mailto: co asunto e o corpo do informe', async () => {
    const generateReport = vi.fn().mockResolvedValue('informe para enviar')
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function () { this._hrefAtClick = this.href })
    let capturedHref = null
    clickSpy.mockImplementation(function () { capturedHref = this.href })

    openBugReportModal({ anaco: 'x', mensaxeErro: 'y', generateReport, saveReport: vi.fn(), contactoEmail: 'dev@exemplo.gal' })
    await Promise.resolve()
    await Promise.resolve()

    const mailBtn = Array.from(modal().querySelectorAll('.modal-actions button')).find((b) => b.textContent.includes('correo'))
    mailBtn.click()

    expect(clickSpy).toHaveBeenCalled()
    expect(capturedHref).toContain('mailto:dev%40exemplo.gal')
    expect(capturedHref).toContain(encodeURIComponent('informe para enviar'))
  })

  it('un erro de generateReport amósase coma estado, sen activar Gardar/Enviar', async () => {
    const generateReport = vi.fn().mockRejectedValue(new Error('sen clave de API'))
    openBugReportModal({ anaco: 'x', mensaxeErro: 'y', generateReport, saveReport: vi.fn() })
    await Promise.resolve()
    await Promise.resolve()

    expect(modal().textContent).toContain('sen clave de API')
    const buttons = modal().querySelectorAll('.modal-actions button')
    // Cancelar sempre activo; Gardar/Enviar seguen desactivados.
    const disabled = Array.from(buttons).filter((b) => b.disabled)
    expect(disabled.length).toBe(2)
  })

  it('Cancelar pecha a ventá', async () => {
    const generateReport = vi.fn().mockResolvedValue('x')
    openBugReportModal({ anaco: 'x', mensaxeErro: 'y', generateReport, saveReport: vi.fn() })
    await Promise.resolve()
    await Promise.resolve()

    const cancelBtn = Array.from(modal().querySelectorAll('.modal-actions button')).find((b) => b.textContent.includes('Cancelar'))
    cancelBtn.click()
    expect(document.querySelector('.overlay:not(.hidden)')).toBeNull()
  })
})
