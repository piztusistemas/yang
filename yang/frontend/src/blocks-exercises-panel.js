// Ventá emerxente "🗂️ Exercicios" - o "cadro de obxectos" do editor de
// bloques, mesma idea có panel de sprites de Scratch: unha caixa por
// exercicio, dobre clic para abrilo no lenzo (blocks-editor.js só amosa
// SEMPRE o exercicio activo, ver ese ficheiro), 🗑/⧉ para eliminar/duplicar
// e arrastrar para reordear. blocks-editor.js é quen garda o estado real
// (exercisesData/activeIndex) - este módulo é só a UI, sen estado propio
// (agás o overlay compartido, mesmo patrón ca blocks-ai-library-modals.js:
// re-renderízase enteiro en cada apertura/refresco).

const panel = { el: null, close: () => { if (panel.el) panel.el.classList.add('hidden') } }

function ensurePanel() {
  if (panel.el) return panel.el
  const overlay = document.createElement('div')
  overlay.className = 'overlay hidden'
  const modal = document.createElement('div')
  modal.className = 'modal modal--exercises'
  overlay.appendChild(modal)
  overlay.addEventListener('click', (e) => { if (e.target === overlay) panel.close() })
  document.body.appendChild(overlay)
  panel.el = overlay
  return overlay
}

export function closeExercisesPanel() {
  panel.close()
}

// openExercisesPanel amosa/refresca a ventá - chamábel varias veces seguidas
// (despois de eliminar/duplicar/reordear, ver blocks-editor.js) para
// redebuxar a grella co estado novo sen pechar a ventá.
export function openExercisesPanel({ t, count, activeIndex, onSwitch, onAdd, onDelete, onDuplicate, onReorder }) {
  const overlay = ensurePanel()
  const modal = overlay.querySelector('.modal')
  modal.innerHTML = ''

  const h2 = document.createElement('h2')
  h2.innerHTML = `<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true" style="width:16px;height:16px;vertical-align:-2px;margin-right:5px;"><rect x="3" y="5.5" width="14" height="10" rx="1.6"/><path d="M3 8.2h14"/></svg>${t('blocks.exercises.title', 'Exercicios')}`
  modal.appendChild(h2)

  const grid = document.createElement('div')
  grid.className = 'exercises-grid'
  grid.setAttribute('role', 'list')

  // dragFrom: índice da caixa que se está a arrastrar - vive só mentres a
  // ventá está aberta (variable local, non estado persistente).
  let dragFrom = null

  for (let i = 0; i < count; i++) {
    const tile = document.createElement('div')
    tile.className = 'exercise-tile' + (i === activeIndex ? ' exercise-tile--active' : '')
    tile.setAttribute('role', 'listitem')
    tile.setAttribute('tabindex', '0')
    tile.draggable = true

    const num = document.createElement('div')
    num.className = 'exercise-tile__num'
    num.textContent = String(i + 1)
    tile.appendChild(num)

    const label = document.createElement('div')
    label.className = 'exercise-tile__label'
    label.textContent = t('blocks.exercises.item', 'Exercicio {n}', { n: i + 1 })
    tile.appendChild(label)

    const actions = document.createElement('div')
    actions.className = 'exercise-tile__actions'

    const dupBtn = document.createElement('button')
    dupBtn.type = 'button'
    dupBtn.className = 'exercise-tile__action'
    dupBtn.title = t('blocks.exercises.duplicate', 'Duplicar')
    dupBtn.setAttribute('aria-label', t('blocks.exercises.duplicate', 'Duplicar'))
    dupBtn.innerHTML = '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><rect x="7" y="7" width="9" height="9" rx="1.4"/><path d="M13 7V5.5A1.5 1.5 0 0 0 11.5 4h-7A1.5 1.5 0 0 0 3 5.5v7A1.5 1.5 0 0 0 4.5 14H7"/></svg>'
    dupBtn.addEventListener('click', (e) => { e.stopPropagation(); onDuplicate(i) })
    dupBtn.addEventListener('dblclick', (e) => e.stopPropagation())
    actions.appendChild(dupBtn)

    const delBtn = document.createElement('button')
    delBtn.type = 'button'
    delBtn.className = 'exercise-tile__action'
    delBtn.title = t('blocks.exercises.delete', 'Eliminar')
    delBtn.setAttribute('aria-label', t('blocks.exercises.delete', 'Eliminar'))
    delBtn.innerHTML = '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M4.5 5.5h11"/><path d="M8 5.5V4a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v1.5"/><path d="M5.5 5.5 6.2 16a1 1 0 0 0 1 .9h5.6a1 1 0 0 0 1-.9l.7-10.5"/></svg>'
    delBtn.addEventListener('click', (e) => { e.stopPropagation(); onDelete(i) })
    delBtn.addEventListener('dblclick', (e) => e.stopPropagation())
    actions.appendChild(delBtn)

    tile.appendChild(actions)

    tile.addEventListener('dblclick', () => onSwitch(i))
    tile.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') onSwitch(i)
    })

    tile.addEventListener('dragstart', (e) => {
      dragFrom = i
      tile.classList.add('exercise-tile--dragging')
      if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
    })
    tile.addEventListener('dragend', () => tile.classList.remove('exercise-tile--dragging'))
    tile.addEventListener('dragover', (e) => e.preventDefault())
    tile.addEventListener('drop', (e) => {
      e.preventDefault()
      if (dragFrom === null || dragFrom === i) return
      onReorder(dragFrom, i)
      dragFrom = null
    })

    grid.appendChild(tile)
  }

  const addTile = document.createElement('button')
  addTile.type = 'button'
  addTile.className = 'exercise-tile exercise-tile--add'
  addTile.setAttribute('aria-label', t('blocks.exercises.add', 'Engadir exercicio'))
  addTile.textContent = '+'
  addTile.addEventListener('click', onAdd)
  grid.appendChild(addTile)

  modal.appendChild(grid)

  if (count === 0) {
    const empty = document.createElement('p')
    empty.className = 'exercises-panel__empty'
    empty.textContent = t('blocks.exercises.empty', 'Aínda non hai exercicios. Preme "+" para engadir o primeiro.')
    modal.appendChild(empty)
  }

  const hint = document.createElement('p')
  hint.className = 'exercises-panel__hint'
  hint.textContent = t('blocks.exercises.hint', 'Dobre clic nun exercicio para editalo. Arrastra para reordear.')
  modal.appendChild(hint)

  const actions = document.createElement('div')
  actions.className = 'modal-actions'
  const closeBtn = document.createElement('button')
  closeBtn.type = 'button'
  closeBtn.className = 'primary'
  closeBtn.textContent = t('blocks.exercises.close', 'Pechar')
  closeBtn.addEventListener('click', panel.close)
  actions.appendChild(closeBtn)
  modal.appendChild(actions)

  overlay.classList.remove('hidden')
}
