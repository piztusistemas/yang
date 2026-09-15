// bug-report.js - "🐛 Informar deste erro": cando falla unha compilación
// (Xerar/PDF), pide á IA un DIAGNÓSTICO + suxestión de arranxo do propio
// YANG (non do exercicio da profesora) pensado para o DESENVOLVEDOR - ver
// bugreport.go/systemPromptInformeErro (ia_client.go) para o razoamento
// completo. Decisión de deseño explícita: isto NUNCA toca nin recompila
// código de Yang - só xera TEXTO (Markdown) que a persoa revisa, edita se
// quere, e despois garda nun ficheiro e/ou envía por correo - os dous
// pasos son sempre explícitos (premer un botón), nunca automáticos, para
// que nada do contido do exame saia do seu equipo sen que o vexa antes.
const overlayState = { el: null, close: () => { if (overlayState.el) overlayState.el.classList.add('hidden') } }

function ensureOverlay() {
  if (overlayState.el) return overlayState.el
  const overlay = document.createElement('div')
  overlay.className = 'overlay hidden'
  const modal = document.createElement('div')
  modal.className = 'modal modal--wide'
  overlay.appendChild(modal)
  overlay.addEventListener('click', (e) => { if (e.target === overlay) overlayState.close() })
  document.body.appendChild(overlay)
  overlayState.el = overlay
  return overlay
}

// buildMailto: mailto: co informe no corpo - acurtado se é longo de máis
// (moitos clientes de correo/SO truncan ou rexeitan mailto: moi longos,
// non hai un límite estándar fiable) - o botón "💾 Gardar" segue a ser o
// xeito fiable de levar o informe ENTEIRO, isto é só para que o correo
// xa saia con contexto útil sen ter que copiar/pegar nada.
const MAILTO_MAX = 1500
function buildMailto(to, subject, body) {
  let corpo = body
  if (corpo.length > MAILTO_MAX) {
    corpo = corpo.slice(0, MAILTO_MAX) + '\n\n[...texto truncado - garda tamén o informe como ficheiro (💾 Gardar) e xúntao a este correo...]'
  }
  return `mailto:${encodeURIComponent(to)}?subject=${encodeURIComponent(subject)}&body=${encodeURIComponent(corpo)}`
}

export function openBugReportModal({
  t: tIn, anaco, mensaxeErro, generateReport, saveReport,
  contactoEmail = 'aplicativopiztu@gmail.com',
} = {}) {
  const t = tIn || ((clave, fallback, vars) => {
    let val = fallback ?? clave
    if (vars) for (const k in vars) val = val.replaceAll(`{${k}}`, vars[k])
    return val
  })
  const overlay = ensureOverlay()
  const modal = overlay.querySelector('.modal')
  modal.innerHTML = ''

  const h2 = document.createElement('h2')
  h2.innerHTML = `<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" style="width:16px;height:16px;vertical-align:-2px;margin-right:5px;"><rect x="6" y="6" width="8" height="9" rx="3"/><path d="M8 6V4.5a2 2 0 0 1 4 0V6M4 9h2M14 9h2M4.5 12.5 6 13M13.5 12.5 14 13M6 15.5v1M14 15.5v1"/></svg>${t('bugreport.title', 'Informar deste erro')}`
  modal.appendChild(h2)

  const note = document.createElement('p')
  note.textContent = t(
    'bugreport.note',
    'A IA redacta un diagnóstico e unha suxestión de arranxo PARA O DESENVOLVEDOR de Yang - nunca modifica nin recompila nada automaticamente. Revisa o texto (podes editalo) antes de gardalo ou envialo.',
  )
  modal.appendChild(note)

  const status = document.createElement('p')
  status.className = 'ai-row__status'
  status.textContent = t('bugreport.generating', 'Xerando informe…')
  modal.appendChild(status)

  const textarea = document.createElement('textarea')
  textarea.rows = 16
  textarea.className = 'bugreport-textarea'
  textarea.disabled = true
  modal.appendChild(textarea)

  const actions = document.createElement('div')
  actions.className = 'modal-actions'

  const cancelBtn = document.createElement('button')
  cancelBtn.type = 'button'
  cancelBtn.textContent = t('blocks.modal.cancel', 'Cancelar')
  cancelBtn.addEventListener('click', overlayState.close)
  actions.appendChild(cancelBtn)

  const mailBtn = document.createElement('button')
  mailBtn.type = 'button'
  mailBtn.innerHTML = '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="14" height="10" rx="1.5"/><path d="M3.5 5.8 10 10.5l6.5-4.7"/></svg><span></span>'
  mailBtn.querySelector('span').textContent = t('bugreport.sendMail', 'Enviar por correo')
  mailBtn.disabled = true
  mailBtn.addEventListener('click', () => {
    // <a mailto:> + click programático (non location.href): evita que o
    // propio SPA intente "navegar" á URL mailto: coma se fose unha ruta
    // interna - o handler mailto: é cousa do sistema operativo/webview.
    const a = document.createElement('a')
    a.href = buildMailto(contactoEmail, t('bugreport.mailSubject', 'Yang - informe de erro'), textarea.value)
    a.target = '_blank'
    a.rel = 'noopener'
    a.click()
  })
  actions.appendChild(mailBtn)

  const saveBtn = document.createElement('button')
  saveBtn.type = 'button'
  saveBtn.className = 'primary'
  saveBtn.innerHTML = '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M4 3.5h9l3 3v10a.5.5 0 0 1-.5.5h-11.5a.5.5 0 0 1-.5-.5v-12.5a.5.5 0 0 1 .5-.5z"/><path d="M6.5 3.5v4h6v-4"/><path d="M6 13h8"/></svg><span></span>'
  saveBtn.querySelector('span').textContent = t('bugreport.save', 'Gardar como…')
  saveBtn.disabled = true
  saveBtn.addEventListener('click', async () => {
    saveBtn.disabled = true
    try {
      const path = await saveReport('informe-erro-yang.md', textarea.value)
      if (path) status.textContent = t('bugreport.saved', 'Gardado: {path}', { path })
    } catch (err) {
      status.textContent = t('bugreport.errorSaving', 'Erro ao gardar: {msg}', { msg: err && err.message ? err.message : err })
    } finally {
      saveBtn.disabled = false
    }
  })
  actions.appendChild(saveBtn)

  modal.appendChild(actions)
  overlay.classList.remove('hidden')

  generateReport(anaco, mensaxeErro).then((texto) => {
    textarea.value = texto
    textarea.disabled = false
    mailBtn.disabled = false
    saveBtn.disabled = false
    status.textContent = t('bugreport.ready', 'Informe listo - revísao antes de gardalo ou envialo.')
  }).catch((err) => {
    status.textContent = t('bugreport.error', 'Erro: {msg}', { msg: err && err.message ? err.message : err })
  })
}
