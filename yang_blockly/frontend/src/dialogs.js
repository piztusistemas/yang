// Diálogos de confirmación/pregunta/aviso PROPIOS da aplicación, en HTML,
// para substituír os nativos do navegador (window.confirm/prompt/alert).
//
// POR QUE existe este módulo: en macOS os nativos NON funcionan. Wails v3
// (beta.14) usa WKWebView e asígnalle un uiDelegate propio
// (webview_window_darwin.go: `[webView setUIDelegate:delegate]`), pero esa
// clase delegada só implementa `runOpenPanelWithParameters` (o selector de
// ficheiros); non implementa os tres paneis de diálogo de JavaScript
// (`runJavaScriptConfirmPanelWithMessage:`, `...AlertPanelWithMessage:`,
// `...TextInputPanelWithPrompt:`). Cando hai uiDelegate pero non responde a
// eses selectores, WKWebView non amosa nada e devolve o valor por defecto
// decontado: alert() non fai nada, confirm() devolve SEMPRE false e prompt()
// devolve SEMPRE null.
//
// O efecto era que calquera acción protexida por unha confirmación quedaba
// morta en macOS - o botón "Novo" con contido no editor non respondía
// (confirm() dicía "cancelar" sen preguntar), e o mesmo o borrado no
// explorador, o renomeado, a nova carpeta, o lixo dos bloques, o reparto e
// a actualización. En Linux (WebKitGTK) e Windows (WebView2) si funcionaban
// porque eses motores amosan os seus propios diálogos por defecto, así que
// o fallo só se vía en macOS.
//
// A API é a mesma ca a nativa pero ASÍNCRONA (devolve Promise), porque a
// resposta chega dun clic e non se pode bloquear o fío coma fai o nativo:
//   await confirmar(msg)            -> true/false
//   await preguntar(msg, valor)     -> texto ou null se se cancela
//   await avisar(msg)               -> undefined (só "Aceptar")
//
// Reutiliza as clases .overlay/.modal/.modal-actions/.modal-label xa
// definidas en style.css, co seu propio z-index por riba de todo (incluídos
// os widgets de Blockly, que van a 99999) para que se poida amosar tamén
// enriba doutra modal xa aberta (ex. a confirmación do reparto).

// Textos por defecto en galego; quen chame pode pasar `opcions.aceptar` /
// `opcions.cancelar` xa traducidos (ver t() en main.js).
const ACEPTAR = 'Aceptar'
const CANCELAR = 'Cancelar'

// abrirDialogo é o motor común dos tres tipos. `tipo` decide se hai campo de
// texto e que se resolve ao aceptar.
function abrirDialogo({ tipo, mensaxe, valor = '', aceptar = ACEPTAR, cancelar = CANCELAR }) {
  return new Promise((resolve) => {
    const overlay = document.createElement('div')
    overlay.className = 'overlay overlay--dialog'

    const modal = document.createElement('div')
    modal.className = 'modal modal--dialog'
    overlay.appendChild(modal)

    const texto = document.createElement('p')
    texto.className = 'dialog-message'
    // textContent (non innerHTML): as mensaxes levan nomes de ficheiro e
    // outros datos do usuario, que non deben interpretarse coma HTML.
    texto.textContent = mensaxe
    modal.appendChild(texto)

    let input = null
    if (tipo === 'prompt') {
      const label = document.createElement('label')
      label.className = 'modal-label'
      input = document.createElement('input')
      input.type = 'text'
      input.value = valor
      label.appendChild(input)
      modal.appendChild(label)
    }

    const acciones = document.createElement('div')
    acciones.className = 'modal-actions'
    modal.appendChild(acciones)

    // pechar desmonta o diálogo e resolve UNHA soa vez - `rematado` evita
    // que un segundo camiño (ex. Esc xusto despois dun clic) volva resolver.
    let rematado = false
    function pechar(resultado) {
      if (rematado) return
      rematado = true
      document.removeEventListener('keydown', onKeydown, true)
      overlay.remove()
      resolve(resultado)
    }

    // Valor devolto ao cancelar: o MESMO ca o nativo equivalente, para que
    // os `if (!await confirmar(...)) return` de quen chama sigan igual.
    const valorCancelar = tipo === 'prompt' ? null : tipo === 'confirm' ? false : undefined

    function aceptarDialogo() {
      if (tipo === 'prompt') pechar(input.value)
      else if (tipo === 'confirm') pechar(true)
      else pechar(undefined)
    }

    // Esc cancela, Enter acepta. En captura (true) para gañarlle aos
    // atallos globais de main.js (Ctrl+N e compañía) e ao manexador de Esc
    // de blocks-modal.js mentres o diálogo estea aberto.
    function onKeydown(e) {
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        pechar(valorCancelar)
      } else if (e.key === 'Enter' && tipo !== 'prompt') {
        e.preventDefault()
        e.stopPropagation()
        aceptarDialogo()
      }
    }
    document.addEventListener('keydown', onKeydown, true)

    if (tipo !== 'alert') {
      const btnCancelar = document.createElement('button')
      btnCancelar.type = 'button'
      btnCancelar.textContent = cancelar
      btnCancelar.addEventListener('click', () => pechar(valorCancelar))
      acciones.appendChild(btnCancelar)
    }

    const btnAceptar = document.createElement('button')
    btnAceptar.type = 'button'
    btnAceptar.className = 'primary'
    btnAceptar.textContent = aceptar
    btnAceptar.addEventListener('click', aceptarDialogo)
    acciones.appendChild(btnAceptar)

    if (input) {
      // Enter no campo acepta (coma o prompt nativo); o formulario non
      // existe, así que hai que enganchalo á man.
      input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') { e.preventDefault(); aceptarDialogo() }
      })
    }

    // Clic fóra da modal = cancelar, mesmo contrato ca blocks-modal.js.
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) pechar(valorCancelar)
    })

    document.body.appendChild(overlay)
    // Foco: no campo (seleccionado, para poder escribir enriba coma fai o
    // prompt nativo) ou no botón de aceptar, para que Enter/Esc funcionen
    // sen ter que mover o rato.
    if (input) { input.focus(); input.select() } else btnAceptar.focus()
  })
}

// confirmar substitúe window.confirm: resolve true se se acepta, false se
// se cancela (Esc, clic fóra ou "Cancelar").
export function confirmar(mensaxe, opcions = {}) {
  return abrirDialogo({ tipo: 'confirm', mensaxe, ...opcions })
}

// preguntar substitúe window.prompt: resolve o texto escrito, ou null se se
// cancela. Ollo: coma o nativo, unha cadea baleira NON é null - quen chama
// segue a ter que comprobar `if (!nome) return` se non a acepta.
export function preguntar(mensaxe, valor = '', opcions = {}) {
  return abrirDialogo({ tipo: 'prompt', mensaxe, valor, ...opcions })
}

// avisar substitúe window.alert: só informa, resolve cando se pecha.
export function avisar(mensaxe, opcions = {}) {
  return abrirDialogo({ tipo: 'alert', mensaxe, ...opcions })
}
