// Diálogo de confirmación PROPIO (HTML), para substituír a window.confirm.
//
// POR QUE: en macOS o confirm() nativo non funciona baixo Wails. O
// instalador vai en Wails v2, que usa WKWebView e faise UIDelegate del
// (internal/frontend/desktop/darwin/WailsContext.m: `self.webview.UIDelegate
// = self`), pero WailsContext non implementa ningún dos tres paneis de
// diálogo de JavaScript (`runJavaScriptConfirmPanelWithMessage:`,
// `...AlertPanelWithMessage:`, `...TextInputPanelWithPrompt:`). Cando hai
// UIDelegate que non responde a eses selectores, WKWebView non amosa nada e
// devolve o valor por defecto decontado: confirm() dá SEMPRE false.
//
// O efecto era que o botón "Desinstalar" non respondía en macOS - a
// confirmación dicía "cancelar" sen chegar a preguntar. En Linux (WebKitGTK)
// e Windows (WebView2) si funcionaba, porque eses motores amosan os seus
// propios diálogos por defecto. Wails v3 ten o MESMO oco, e por iso o
// aplicativo Yang leva o seu propio módulo equivalente (yang_blockly/
// frontend/src/dialogs.js) - se se arranxa nun, arránxase no outro.
//
// A API é asíncrona (devolve Promise) porque a resposta vén dun clic e non
// se pode bloquear o fío coma fai o nativo:
//   if (!await confirmar(msg)) return

// confirmar substitúe a window.confirm: resolve true se se acepta e false se
// se cancela (botón "Cancelar", Esc ou clic fóra) - mesmos valores ca o
// nativo, así que quen chama non ten que cambiar a condición.
export function confirmar(mensaxe, { aceptar = 'Aceptar', cancelar = 'Cancelar', perigo = false } = {}) {
  return new Promise((resolve) => {
    const overlay = document.createElement('div')
    overlay.className = 'dialog-overlay'

    const modal = document.createElement('div')
    modal.className = 'dialog'
    overlay.appendChild(modal)

    const texto = document.createElement('p')
    texto.className = 'dialog-message'
    // textContent, non innerHTML: as mensaxes veñen das traducións e poden
    // levar datos que non se deben interpretar coma HTML.
    texto.textContent = mensaxe
    modal.appendChild(texto)

    const acciones = document.createElement('div')
    acciones.className = 'dialog-actions'
    modal.appendChild(acciones)

    // `rematado` garante que a Promise se resolve UNHA soa vez, aínda que
    // cheguen dous camiños (ex. Esc xusto despois dun clic).
    let rematado = false
    function pechar(resultado) {
      if (rematado) return
      rematado = true
      document.removeEventListener('keydown', onKeydown, true)
      overlay.remove()
      resolve(resultado)
    }

    // Só Esc, en captura (true) para gañarlle a calquera atallo global
    // mentres o diálogo estea aberto. Enter NON se captura a propósito:
    // déixase que o active o botón que teña o foco (comportamento nativo
    // dun <button>), para que nunha confirmación de perigo - onde o foco
    // vai en "Cancelar", ver máis abaixo - un Enter despistado cancele en
    // vez de disparar a acción irreversible.
    function onKeydown(e) {
      if (e.key !== 'Escape') return
      e.preventDefault()
      e.stopPropagation()
      pechar(false)
    }
    document.addEventListener('keydown', onKeydown, true)

    const btnCancelar = document.createElement('button')
    btnCancelar.type = 'button'
    btnCancelar.className = 'btn-main'
    btnCancelar.textContent = cancelar
    btnCancelar.addEventListener('click', () => pechar(false))
    acciones.appendChild(btnCancelar)

    const btnAceptar = document.createElement('button')
    btnAceptar.type = 'button'
    // `perigo` pinta o botón de aceptar en vermello (.btn-main.danger, o
    // mesmo ca o de "Desinstalar") en vez do azul de primary.
    btnAceptar.className = perigo ? 'btn-main danger' : 'btn-main primary'
    btnAceptar.textContent = aceptar
    btnAceptar.addEventListener('click', () => pechar(true))
    acciones.appendChild(btnAceptar)

    // Clic fóra da caixa = cancelar.
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) pechar(false)
    })

    document.body.appendChild(overlay)
    // Foco inicial: en "Cancelar" se a acción é irreversible (`perigo`,
    // ex. desinstalar), para que Enter/barra espaciadora non caian no botón
    // destrutivo; no de aceptar no resto dos casos.
    ;(perigo ? btnCancelar : btnAceptar).focus()
  })
}
