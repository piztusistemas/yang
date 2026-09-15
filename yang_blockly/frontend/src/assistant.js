// assistant.js: xanela de chat do asistente de IA integrado (icona 🤖) -
// compartida entre o editor de Código (main.js: documento enteiro ou
// selección) e cada modal de anaco do editor de bloques (blocks.js:
// Enunciado/Variable/Fórmula/Gráfico/TikZ - "Imaxe" queda fóra, é un
// ficheiro do disco, non hai texto que editar). Unha soa instancia
// (singleton): créase con createAssistant() e reábrese/reconfígurase cada
// vez con open({...}) - mesmo patrón cós modais .overlay/.modal que xa usa
// blocks.js (ensureModal).
//
// Protocolo co backend (AsistenteIA, app.go): a petición vai co contido
// ACTUAL do que se está a editar; a resposta é ou unha explicación en
// prosa (accion "responder", non toca nada) ou unha proposta de contido
// novo completo (accion "editar") que se amosa cun botón "Aplicar" - NUNCA
// se substitúe contido sen que o profesorado prema ese botón.
import { AsistenteIA } from '../bindings/matexe-wails/app.js'

export function createAssistant({ t }) {
  let overlay, modal, titleEl, log, input, sendBtn
  let reorgBar, reorgBtn, reorgSementes, reorgRoldas, reorgAdaptar
  let ctx = null // { tipo, getContent, setContent, reorganizar } - ver open()

  // makeDraggable: mesma idea ca a súa homóloga en blocks.js (premer e
  // arrastrar o título move o cadro) - non se comparte código entre os dous
  // ficheiros a propósito, é pequeno dabondo (12 liñas) para que non pague
  // a pena o acoplamento extra de exportalo dun módulo a outro.
  function makeDraggable(box, handle) {
    handle.addEventListener('mousedown', (e) => {
      if (e.button !== 0) return
      const rect = box.getBoundingClientRect()
      box.style.position = 'fixed'
      box.style.margin = '0'
      box.style.left = rect.left + 'px'
      box.style.top = rect.top + 'px'
      const startX = e.clientX
      const startY = e.clientY
      const startLeft = rect.left
      const startTop = rect.top
      function onMove(ev) {
        box.style.left = startLeft + ev.clientX - startX + 'px'
        box.style.top = startTop + ev.clientY - startY + 'px'
      }
      function onUp() {
        window.removeEventListener('mousemove', onMove)
        window.removeEventListener('mouseup', onUp)
      }
      window.addEventListener('mousemove', onMove)
      window.addEventListener('mouseup', onUp)
      e.preventDefault()
    })
  }

  function ensure() {
    if (overlay) return
    overlay = document.createElement('div')
    overlay.className = 'overlay hidden'
    modal = document.createElement('div')
    modal.className = 'modal modal--assistant'
    overlay.appendChild(modal)
    overlay.addEventListener('click', (e) => { if (e.target === overlay) close() })
    document.body.appendChild(overlay)

    const head = document.createElement('div')
    head.className = 'assistant-head'
    titleEl = document.createElement('h2')
    titleEl.className = 'modal-drag-handle'
    const closeBtn = document.createElement('button')
    closeBtn.type = 'button'
    closeBtn.className = 'assistant-close'
    closeBtn.textContent = '✕'
    closeBtn.addEventListener('click', close)
    head.appendChild(titleEl)
    head.appendChild(closeBtn)
    modal.appendChild(head)
    makeDraggable(modal, titleEl)

    // Barra "Reorganizar práctica" (só visible cando open() recibe `reorganizar`,
    // ver reorganizar.js): reconstrúe resultado final + resolución de toda a
    // práctica a partir dos enunciados, coas fórmulas e cos cálculos de Maxima,
    // validando en varias sementes. Vai enriba do log para que o progreso se
    // vexa xusto debaixo.
    reorgBar = document.createElement('div')
    reorgBar.className = 'assistant-reorg hidden'
    reorgBtn = document.createElement('button')
    reorgBtn.type = 'button'
    reorgBtn.className = 'primary assistant-reorg__run'
    reorgBtn.addEventListener('click', reorganizar)
    const cfg = document.createElement('div')
    cfg.className = 'assistant-reorg__cfg'
    reorgSementes = numField(t('assistant.reorganize.seeds', 'Sementes de proba'), 4)
    reorgRoldas = numField(t('assistant.reorganize.rounds', 'Roldas de arranxo'), 3)
    reorgAdaptar = document.createElement('input')
    reorgAdaptar.type = 'checkbox'
    reorgAdaptar.checked = true
    const adaptLab = document.createElement('label')
    adaptLab.className = 'assistant-reorg__adapt'
    adaptLab.append(reorgAdaptar, document.createTextNode(' ' + t('assistant.reorganize.adapt', 'adaptar ao exercicio')))
    cfg.append(reorgSementes.wrap, reorgRoldas.wrap, adaptLab)
    reorgBar.append(reorgBtn, cfg)
    modal.appendChild(reorgBar)

    log = document.createElement('div')
    log.className = 'assistant-log'
    modal.appendChild(log)

    const form = document.createElement('form')
    form.className = 'assistant-form'
    input = document.createElement('textarea')
    input.rows = 2
    input.placeholder = t('assistant.placeholder', 'Pregunta algo ou pide un cambio…')
    sendBtn = document.createElement('button')
    sendBtn.type = 'submit'
    sendBtn.className = 'primary'
    sendBtn.textContent = t('assistant.send', 'Enviar')
    form.appendChild(input)
    form.appendChild(sendBtn)
    modal.appendChild(form)

    form.addEventListener('submit', (e) => { e.preventDefault(); send() })
    // Intro envía; Maiús+Intro insire un salto de liña (comportamento
    // estándar dun caixa de chat, e evita ter que premer sempre o botón).
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() }
    })
  }

  function addMessage(role, text) {
    const div = document.createElement('div')
    div.className = 'assistant-msg assistant-msg--' + role
    div.textContent = text
    log.appendChild(div)
    log.scrollTop = log.scrollHeight
    return div
  }

  // numField: unha caixiña numérica con etiqueta pequena, para a config de
  // "Reorganizar" (sementes / roldas). Devolve { wrap, input }.
  function numField(label, valor) {
    const wrap = document.createElement('label')
    wrap.className = 'assistant-reorg__num'
    const lab = document.createElement('span')
    lab.textContent = label
    const inp = document.createElement('input')
    inp.type = 'number'
    inp.min = '1'
    inp.value = String(valor)
    wrap.append(lab, inp)
    return { wrap, input: inp }
  }

  // liñaInforme: traduce un elemento do informe de reorganizarPractica
  // (reorganizar.js) a unha liña de texto para o log.
  function liñaInforme(inf) {
    const n = inf.indice + 1
    switch (inf.estado) {
      case 'ok':
        return t('assistant.reorganize.report.ok', 'Exercicio {n}: correcto ({s} sementes)', { n, s: inf.sementes || '—' })
      case 'reparado':
        return t('assistant.reorganize.report.repaired', 'Exercicio {n}: reparado en {r} roldas ({s} sementes)', { n, r: inf.roldas, s: inf.sementes || '—' })
      case 'falla': {
        const d = (inf.diagnosticos || [])[0]
        const detalle = d ? `${d.problema} · ${d.expr} [semente ${d.seed}]` : (inf.motivo || '')
        return t('assistant.reorganize.report.failed', 'Exercicio {n}: SEGUE FALLANDO — {detalle}', { n, detalle })
      }
      case 'sen_formula':
        return t('assistant.reorganize.report.noFormula', 'Exercicio {n}: sen fórmula pechada, déixase para revisión manual{motivo}', { n, motivo: inf.motivo ? ` (${inf.motivo})` : '' })
      case 'erro':
        return t('assistant.reorganize.report.error', 'Exercicio {n}: erro — {motivo}', { n, motivo: inf.motivo || '' })
      default:
        return t('assistant.reorganize.report.skipped', 'Exercicio {n}: sen cambios', { n })
    }
  }

  async function reorganizar() {
    if (!ctx || !ctx.reorganizar) return
    const r = ctx.reorganizar
    const sementes = Math.max(1, parseInt(reorgSementes.input.value, 10) || 0) || undefined
    const roldas = Math.max(1, parseInt(reorgRoldas.input.value, 10) || 0) || undefined
    const adaptar = !!reorgAdaptar.checked
    setReorgBusy(true)
    log.innerHTML = ''
    addMessage('assistant', t('assistant.reorganize.working', 'Reorganizando a práctica: pásanse os enunciados, refanse as fórmulas e Maxima recalcula. Isto pode levar un anaco…'))
    try {
      const { texto, informe } = await r.correr({
        sementes, roldas, adaptar,
        onProgreso: (inf) => addMessage('assistant', liñaInforme(inf)),
      })
      if (informe && !informe.some((i) => i.estado !== 'sen_cambios')) {
        addMessage('assistant', t('assistant.reorganize.nothing', 'Non había exercicios que reorganizar.'))
        return
      }
      const resumo = contar(informe)
      const msg = addMessage('assistant', t('assistant.reorganize.done', 'Feito. {ok} correctos, {rep} reparados, {fail} sen resolver, {sf} sen fórmula.', resumo))
      msg.appendChild(document.createElement('br'))
      const applyBtn = document.createElement('button')
      applyBtn.type = 'button'
      applyBtn.className = 'assistant-apply'
      applyBtn.textContent = t('assistant.reorganize.apply', '✓ Aplicar a práctica reorganizada')
      applyBtn.addEventListener('click', () => {
        r.aplicar(texto)
        applyBtn.disabled = true
        applyBtn.textContent = t('assistant.applied', '✓ Aplicado')
      })
      msg.appendChild(applyBtn)
    } catch (err) {
      addMessage('error', String(err && err.message ? err.message : err))
    } finally {
      setReorgBusy(false)
    }
  }

  function contar(informe) {
    const c = { ok: 0, rep: 0, fail: 0, sf: 0 }
    for (const i of informe || []) {
      if (i.estado === 'ok') c.ok++
      else if (i.estado === 'reparado') c.rep++
      else if (i.estado === 'falla' || i.estado === 'erro') c.fail++
      else if (i.estado === 'sen_formula') c.sf++
    }
    return c
  }

  function setReorgBusy(busy) {
    reorgBtn.disabled = busy
    reorgSementes.input.disabled = busy
    reorgRoldas.input.disabled = busy
    reorgAdaptar.disabled = busy
    sendBtn.disabled = busy
    reorgBtn.textContent = busy
      ? t('assistant.reorganize.running', 'Reorganizando…')
      : reorgBtnLabel()
  }

  function reorgBtnLabel() {
    return ctx && ctx.reorganizar && ctx.reorganizar.soExercicio != null
      ? t('assistant.reorganize.exercise', '⟳ Reorganizar este exercicio')
      : t('assistant.reorganize.run', '⟳ Reorganizar práctica')
  }

  async function send() {
    const peticion = input.value.trim()
    if (!peticion || !ctx) return
    input.value = ''
    addMessage('user', peticion)
    sendBtn.disabled = true
    const thinking = addMessage('assistant', t('assistant.thinking', 'Pensando…'))
    try {
      const resp = await AsistenteIA({ tipo: ctx.tipo, contido: ctx.getContent(), peticion })
      thinking.remove()
      if (resp.accion === 'editar') {
        const msg = addMessage('assistant', resp.texto || t('assistant.editProposed', 'Teño unha proposta de cambio lista.'))
        msg.appendChild(document.createElement('br'))
        const applyBtn = document.createElement('button')
        applyBtn.type = 'button'
        applyBtn.className = 'assistant-apply'
        applyBtn.textContent = t('assistant.apply', '✓ Aplicar este cambio')
        applyBtn.addEventListener('click', () => {
          ctx.setContent(resp.contido)
          applyBtn.disabled = true
          applyBtn.textContent = t('assistant.applied', '✓ Aplicado')
        })
        msg.appendChild(applyBtn)
      } else {
        addMessage('assistant', resp.texto)
      }
    } catch (err) {
      thinking.remove()
      addMessage('error', String(err && err.message ? err.message : err))
    } finally {
      sendBtn.disabled = false
      input.focus()
    }
  }

  // open reconfigura a xanela para un novo contexto de edición e a amosa
  // (limpa a conversa anterior - cada apertura é un tema novo, non ten
  // sentido arrastrar mensaxes doutro anaco/documento):
  //   tipo: "documento" (editor de Código) ou o "type" dun anaco de
  //     blocks-serialize.js (text/variables/formula/image-plot/image-tikz).
  //   title: texto amosado na cabeceira (ex. "Fórmula", "Código").
  //   getContent()/setContent(texto): len/escriben o contido a editar - o
  //     chamador decide de onde ven (editor.js ou un <textarea> do modal de
  //     bloques).
  //   reorganizar (opcional): { correr, aplicar, soExercicio, defaults } -
  //     activa a barra "Reorganizar práctica" (ver reorganizar.js). `correr`
  //     recibe { sementes, roldas, adaptar, onProgreso } e devolve
  //     { texto, informe }; `aplicar(texto)` escribe o resultado (mesma vía
  //     non destrutiva ca setContent).
  function open({ tipo, title, getContent, setContent, reorganizar: reorg }) {
    ensure()
    ctx = { tipo, getContent, setContent, reorganizar: reorg || null }
    titleEl.innerHTML = `<svg viewBox="0 0 20 20" fill="currentColor" aria-hidden="true" style="width:16px;height:16px;vertical-align:-2px;margin-right:5px;color:var(--primary);"><path d="M10 2.5l1.6 4.4L16 8.5l-4.4 1.6L10 14.5l-1.6-4.4L4 8.5l4.4-1.6z"/></svg>${title}`
    if (reorg) {
      reorgBar.classList.remove('hidden')
      reorgBtn.textContent = reorgBtnLabel()
      const d = reorg.defaults || {}
      reorgSementes.input.value = String(d.sementes || 25)
      reorgRoldas.input.value = String(d.roldas || 3)
      reorgAdaptar.checked = d.adaptar !== false
      setReorgBusy(false)
    } else {
      reorgBar.classList.add('hidden')
    }
    log.innerHTML = ''
    modal.style.position = ''
    modal.style.left = ''
    modal.style.top = ''
    modal.style.margin = ''
    overlay.classList.remove('hidden')
    requestAnimationFrame(() => input.focus())
  }

  function close() {
    if (overlay) overlay.classList.add('hidden')
  }

  return { open, close }
}
