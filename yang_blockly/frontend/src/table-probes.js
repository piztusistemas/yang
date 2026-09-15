// createTableProbes - contido da lapela "Probas" da interface de táboa.
// Executa cada exercicio do documento en varias sementes (backend
// ProbarExercicios / proba.go) e amosa, por exercicio, se todas as
// expresións (<EVAL>, resposta, solución) dan valores coherentes ou se
// aparece algún infinito / indeterminación / división por cero / erro de
// Maxima. É unha comprobación de sanidade antes de repartir un exame: un
// exercicio que só falla cunha semente concreta é difícil de cazar á man.
//
// Non ten estado propio persistente: re-créase enteiro cada vez que se abre
// a lapela (mesmo patrón ca blocks-exercises-panel.js).

const PROBLEMA_ETIQUETA = {
  infinito: 'infinito',
  indeterminado: 'indeterminación',
  nan: 'NaN',
  division_cero: 'división por cero',
  erro_maxima: 'erro de Maxima',
  baleiro: 'resultado baleiro',
  complexo: 'resultado complexo',
  timeout: 'tempo esgotado',
}

export function createTableProbes(host, { t, getSource, probarExercicios, numExercicios }) {
  host.innerHTML = ''

  const bar = div('table-probes__bar')
  const lab = document.createElement('label')
  lab.className = 'table-probes__seeds'
  lab.textContent = t('table.probes.nseeds', 'Sementes a probar: ')
  const nInput = document.createElement('input')
  nInput.type = 'number'
  nInput.min = '3'
  nInput.max = '200'
  nInput.value = '25'
  lab.appendChild(nInput)

  const runBtn = document.createElement('button')
  runBtn.type = 'button'
  runBtn.className = 'table-probes__run btn-primary'
  runBtn.textContent = t('table.probes.run', 'Executar probas')

  const status = div('table-probes__status')

  bar.append(lab, runBtn, status)
  host.appendChild(bar)

  const results = div('table-probes__results')
  host.appendChild(results)

  if (!numExercicios) {
    status.textContent = t('table.probes.noExercises', 'Non hai exercicios que probar.')
    runBtn.disabled = true
  }

  runBtn.addEventListener('click', async () => {
    if (typeof probarExercicios !== 'function') {
      status.textContent = t('table.probes.unavailable', 'A comprobación non está dispoñible nesta versión.')
      return
    }
    runBtn.disabled = true
    status.textContent = t('table.probes.running', 'Probando… (pode tardar)')
    results.innerHTML = ''
    try {
      const nSeeds = Math.max(3, Math.min(200, parseInt(nInput.value, 10) || 25))
      const res = await probarExercicios({
        source: getSource(),
        seedBase: 1,
        nSeeds,
        codeIni: '',
      })
      render(res)
      const totalFallos = (res.exercicios || []).reduce((s, e) => s + (e.nFallos || 0), 0)
      status.textContent = totalFallos === 0
        ? t('table.probes.allOk', 'Todo correcto: ningún problema en {n} sementes.', { n: nSeeds })
        : t('table.probes.someFail', '{n} exercicio(s) con problemas — revisa abaixo.', { n: (res.exercicios || []).filter((e) => e.nFallos).length })
    } catch (err) {
      status.textContent = t('table.probes.error', 'Erro ao probar: {msg}', { msg: String(err && err.message ? err.message : err) })
    } finally {
      runBtn.disabled = false
    }
  })

  function render(res) {
    results.innerHTML = ''
    for (const ex of res.exercicios || []) {
      const card = div('table-probe-card')
      const ok = !ex.nFallos
      card.classList.add(ok ? 'is-ok' : (ex.nFallos < ex.nProbas ? 'is-warn' : 'is-bad'))

      const head = div('table-probe-card__head')
      head.innerHTML =
        `<span class="table-probe-card__n">${t('table.probes.exercise', 'Exercicio {n}', { n: ex.indice + 1 })}</span>` +
        `<span class="table-probe-card__badge">${ok
          ? '✓ ' + t('table.probes.pass', '{a}/{b} correctas', { a: ex.nProbas, b: ex.nProbas })
          : '⚠ ' + t('table.probes.fail', '{a}/{b} con problema', { a: ex.nFallos, b: ex.nProbas })}</span>`
      card.appendChild(head)

      if (!ok && (ex.diagnosticos || []).length) {
        const tbl = document.createElement('table')
        tbl.className = 'table-probe-diag'
        tbl.innerHTML = `<thead><tr>
          <th>${t('table.probes.col.seed', 'Semente')}</th>
          <th>${t('table.probes.col.expr', 'Expresión')}</th>
          <th>${t('table.probes.col.value', 'Valor')}</th>
          <th>${t('table.probes.col.problem', 'Problema')}</th></tr></thead>`
        const tb = document.createElement('tbody')
        for (const d of ex.diagnosticos) {
          const tr = document.createElement('tr')
          tr.innerHTML =
            `<td>${d.seed}</td>` +
            `<td><code>${escapeHTML(d.expr)}</code></td>` +
            `<td><code>${escapeHTML(d.valor || '')}</code></td>` +
            `<td>${PROBLEMA_ETIQUETA[d.problema] || d.problema}</td>`
          tb.appendChild(tr)
        }
        tbl.appendChild(tb)
        card.appendChild(tbl)
      }
      results.appendChild(card)
    }
  }
}

function div(cls) {
  const n = document.createElement('div')
  n.className = cls
  return n
}
function escapeHTML(s) {
  return String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]))
}
