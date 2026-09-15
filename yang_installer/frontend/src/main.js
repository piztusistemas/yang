import './app.css';
import yangLogo from './assets/images/logo.png';

import {
    Idioma, SetIdioma, Idiomas, Traducions,
    EsRoot, PermisoDenegado, DestDir, InstalacionDesc, CompletadoDesc, Version,
    Instalar, CrearAccesosDirectos, DesinstalarYang,
} from '../wailsjs/go/main/App';
import {EventsOn, Quit} from '../wailsjs/runtime/runtime';
import {confirmar} from './dialogs.js';

// ── Estado global ────────────────────────────────────────────────────────────
let I18N = {};
let IDIOMAS = [{code: 'gl', nome: 'Galego'}];
let idiomaActual = 'gl';
let esRootCache = true;
let destDirCache = '/opt/piztu/modulos/yang';
let instDescCache = '';
let finDescCache = '';
let versionCache = '';
let current = 0;

const STEPS = [
    {id: 'benvida', key: 'paso.benvida'},
    {id: 'instalacion', key: 'paso.instalacion'},
    {id: 'completado', key: 'paso.completado'},
    {id: 'desinstalar', key: 'paso.desinstalar', danger: true},
];

function t(clave, fallback) {
    return I18N[clave] || fallback || clave;
}

// "Paso %d de %d" / "Step %d of %d"… substitúe os %d na orde en que aparecen.
function formatoNum(fmt, ...args) {
    let i = 0;
    return fmt.replace(/%d/g, () => args[i++]);
}

const app = document.getElementById('app');

// ── Pantalla de permiso denegado (pkexec non dispoñible/cancelado) ─────────
function renderPermisoDenegado() {
    app.innerHTML = `
      <div class="permiso-screen">
        <div class="card">
          <h3>${t('permiso.title', 'Permiso necesario')}</h3>
          <p>${t('permiso.texto', '')}</p>
          <button class="btn-main primary" id="btn-permiso-pechar">${t('permiso.pechar', 'Pechar')}</button>
        </div>
      </div>
    `;
    document.getElementById('btn-permiso-pechar').addEventListener('click', () => Quit());
}

// ── Esqueleto principal (barra lateral + panel) ─────────────────────────────
function render() {
    app.innerHTML = `
      <div class="sidebar">
        <div class="sidebar-top">
          <img src="${yangLogo}" class="sidebar-logo" alt="Yang">
          <div class="sidebar-lang">
            <span>${t('nav.idioma', 'Idioma')}</span>
            <select id="lang-select">
              ${IDIOMAS.map(d => `<option value="${d.code}" ${d.code === idiomaActual ? 'selected' : ''}>${d.nome}</option>`).join('')}
            </select>
          </div>
        </div>
        <div class="sidebar-sep"></div>
        <div class="sidebar-steps">
          ${STEPS.map((s, i) => `
            ${s.danger && !STEPS[i - 1]?.danger ? '<div class="sidebar-sep"></div>' : ''}
            <button class="step-btn ${i === current ? 'activo' : ''} ${s.danger ? 'danger' : ''}" data-step="${i}">${t(s.key, s.id)}</button>
          `).join('')}
        </div>
        <div class="sidebar-sep"></div>
        <div class="sidebar-info">${t('app.info', 'Yang')}${versionCache ? ' ' + versionCache : ''}</div>
        <div class="sidebar-info">${t('app.licenza', 'Licenza AGPLv3')}</div>
      </div>
      <div class="main-panel">
        <div class="main-content" id="main-content"></div>
        <div class="main-footer">
          <div class="progress-row">
            <div class="progress-track"><div class="progress-fill" id="progress-fill"></div></div>
            <span class="progress-label" id="progress-label"></span>
          </div>
          <div class="nav-row">
            <button class="btn-main" id="btn-prev">${t('nav.anterior', '← Anterior')}</button>
            <span class="spacer"></span>
            <button class="btn-main" id="btn-close">${t('nav.pechar', 'Pechar')}</button>
            <button class="btn-main primary" id="btn-next"></button>
          </div>
        </div>
      </div>
    `;

    document.getElementById('lang-select').addEventListener('change', async (e) => {
        idiomaActual = e.target.value;
        await SetIdioma(idiomaActual);
        [I18N, instDescCache, finDescCache] = await Promise.all([Traducions(), InstalacionDesc(), CompletadoDesc()]);
        render();
    });
    document.querySelectorAll('.step-btn').forEach((btn) => {
        btn.addEventListener('click', () => goTo(parseInt(btn.dataset.step, 10)));
    });
    document.getElementById('btn-prev').addEventListener('click', () => goTo(current - 1));
    document.getElementById('btn-close').addEventListener('click', () => Quit());

    // A "Zona de perigo" queda fóra do asistente lineal (só se chega a ela
    // dende a barra lateral): tanto "completado" coma "desinstalar" ofrecen
    // "Finalizar" no canto de "Seguinte".
    const btnNext = document.getElementById('btn-next');
    const esUltimo = STEPS[current].id === 'completado' || STEPS[current].id === 'desinstalar';
    btnNext.textContent = esUltimo ? t('nav.finalizar', 'Finalizar') : t('nav.seguinte', 'Seguinte →');
    btnNext.addEventListener('click', () => {
        if (esUltimo) { Quit(); } else { goTo(current + 1); }
    });

    document.getElementById('btn-prev').disabled = current === 0;

    const pasosWizard = STEPS.filter((s) => !s.danger).length;
    const pct = pasosWizard > 1 ? (Math.min(current, pasosWizard - 1) / (pasosWizard - 1)) * 100 : 0;
    document.getElementById('progress-fill').style.width = pct + '%';
    document.getElementById('progress-label').textContent = STEPS[current].danger
        ? t('paso.desinstalar', 'Zona de perigo')
        : formatoNum(t('nav.progreso', 'Paso %d de %d'), current + 1, pasosWizard);

    renderStep();
}

function goTo(n) {
    if (n < 0 || n >= STEPS.length) return;
    current = n;
    render();
}

function renderStep() {
    const el = document.getElementById('main-content');
    switch (STEPS[current].id) {
        case 'benvida':
            el.innerHTML = benvidaHTML();
            break;
        case 'instalacion':
            el.innerHTML = instalacionHTML();
            wireInstalacion();
            break;
        case 'completado':
            el.innerHTML = completadoHTML();
            wireCompletado();
            break;
        case 'desinstalar':
            el.innerHTML = desinstalarHTML();
            wireDesinstalar();
            break;
    }
}

function addLogLine(box, text) {
    const div = document.createElement('div');
    div.textContent = text;
    box.appendChild(div);
    box.scrollTop = box.scrollHeight;
}

// ── Paso 0: Benvida ──────────────────────────────────────────────────────────
function benvidaHTML() {
    const bloques = [
        ['benvida.que.titulo', 'benvida.que.sub', 'benvida.que.corpo'],
        ['benvida.deps.titulo', 'benvida.deps.sub', 'benvida.deps.corpo'],
        ['benvida.modulo.titulo', 'benvida.modulo.sub', 'benvida.modulo.corpo'],
    ];
    return `
      <h1 class="titulo">${t('benvida.titulo', 'Benvido')}</h1>
      <p class="desc">${t('benvida.desc', '')}</p>
      ${bloques.map(([ti, su, co]) => `
        <div class="card">
          <h3>${t(ti, '')}</h3>
          <p class="card-sub">${t(su, '')}</p>
          <p>${t(co, '')}</p>
        </div>
      `).join('')}
      ${!esRootCache ? `<div class="aviso">${t('benvida.aviso', '')}</div>` : ''}
    `;
}

// ── Paso 1: Instalación ──────────────────────────────────────────────────────
function instalacionHTML() {
    return `
      <div class="card">
        <h3>${t('inst.card', 'Instalación')}</h3>
        <p>${instDescCache}</p>
        <div class="sep"></div>
        <button class="btn-main primary" id="btn-instalar">${t('inst.btn', '🚀 Instalar Yang')}</button>
        <div class="log-box" id="inst-log"><div class="placeholder">${t('inst.out.inicial', '')}</div></div>
      </div>
    `;
}

function wireInstalacion() {
    const btn = document.getElementById('btn-instalar');
    const logBox = document.getElementById('inst-log');
    btn.addEventListener('click', () => {
        btn.disabled = true;
        logBox.innerHTML = '';
        addLogLine(logBox, t('inst.iniciando', 'Iniciando instalación...'));
        Instalar();
    });
}

EventsOn('inst_log', (line) => {
    const box = document.getElementById('inst-log');
    if (box) addLogLine(box, line);
});
EventsOn('inst_completo', () => {
    const btn = document.getElementById('btn-instalar');
    if (btn) btn.disabled = false;
});

// ── Paso 2: Completado ────────────────────────────────────────────────────────
function completadoHTML() {
    return `
      <div class="card">
        <h3>${t('fin.card', '')}</h3>
        <p>${finDescCache}</p>
        <button class="btn-main primary" id="btn-shortcuts">${t('fin.btn', '')}</button>
        <p id="fin-status"></p>
      </div>
    `;
}

function wireCompletado() {
    const btn = document.getElementById('btn-shortcuts');
    const status = document.getElementById('fin-status');
    btn.addEventListener('click', async () => {
        btn.disabled = true;
        status.className = '';
        status.textContent = '';
        try {
            await CrearAccesosDirectos();
            status.className = 'success-msg';
            status.textContent = t('fin.creados', '');
        } catch (e) {
            status.className = 'error-msg';
            status.textContent = t('fin.erro', 'Erro: %s').replace('%s', e);
        }
        btn.disabled = false;
    });
}

// ── Zona de perigo: desinstalación ──────────────────────────────────────────
// Igual que no instalador de Piztu: fóra do fluxo lineal (só se accede dende
// a barra lateral) e require escribir a palabra de confirmación + unha
// confirmación final antes de executar, por ser irreversible. Esa
// confirmación vai por confirmar() (src/dialogs.js) e non por window.confirm,
// que en macOS devolve sempre false sen preguntar e deixaba o botón morto.
function desinstalarHTML() {
    return `
      <h1 class="titulo">${t('desinst.titulo', '⚠️ Zona de perigo')}</h1>
      <p class="desc">${t('desinst.intro', '')}</p>

      <div class="card danger">
        <h3>${t('desinst.card', '')}</h3>
        <p>${t('desinst.desc', '')}</p>
        <div class="sep"></div>
        <label style="display:flex;align-items:center;gap:8px;font-size:0.85rem;color:var(--text-dark);margin-bottom:14px;">
          <input type="checkbox" id="desinst-deps">
          ${t('desinst.deps.label', '')}
        </label>
        <div class="form-grid">
          <div class="form-row">
            <label>${t('desinst.confirmar.label', 'Escribe %s para confirmar').replace('%s', t('desinst.palabra', 'DESINSTALAR'))}</label>
            <input type="text" id="desinst-confirm" autocomplete="off">
          </div>
        </div>
        <button class="btn-main danger" id="btn-desinst" disabled>${t('desinst.btn', '')}</button>
        <div class="log-box" id="desinst-log" style="display:none;"></div>
      </div>
    `;
}

function wireDesinstalar() {
    const input = document.getElementById('desinst-confirm');
    const btn = document.getElementById('btn-desinst');
    const depsCheck = document.getElementById('desinst-deps');
    const palabra = t('desinst.palabra', 'DESINSTALAR');
    input.addEventListener('input', () => {
        btn.disabled = input.value.trim().toUpperCase() !== palabra.toUpperCase();
    });
    btn.addEventListener('click', async () => {
        const aceptado = await confirmar(t('desinst.confirm', ''), {
            aceptar: t('desinst.btn', ''),
            cancelar: t('dialogo.cancelar', 'Cancelar'),
            perigo: true,
        });
        if (!aceptado) return;
        btn.disabled = true;
        input.disabled = true;
        depsCheck.disabled = true;
        const logBox = document.getElementById('desinst-log');
        logBox.style.display = 'block';
        logBox.innerHTML = '';
        DesinstalarYang(depsCheck.checked);
    });
}

EventsOn('desinst_log', (line) => {
    const box = document.getElementById('desinst-log');
    if (box) addLogLine(box, line);
});
EventsOn('desinst_completo', () => {
    // Non se reactiva o botón: se fallou, hai que revisar o log antes de
    // volver intentalo, e se non fallou xa non hai nada que desinstalar.
});

// ── Arranque ──────────────────────────────────────────────────────────────────
async function bootstrap() {
    const permisoDenegado = await PermisoDenegado().catch(() => false);
    if (permisoDenegado) {
        idiomaActual = await Idioma().catch(() => 'gl');
        I18N = await Traducions().catch(() => ({}));
        renderPermisoDenegado();
        return;
    }

    [idiomaActual, I18N, IDIOMAS, esRootCache, destDirCache, instDescCache, finDescCache, versionCache] = await Promise.all([
        Idioma(), Traducions(), Idiomas(), EsRoot(), DestDir(), InstalacionDesc(), CompletadoDesc(), Version().catch(() => ''),
    ]);
    render();
}

bootstrap();
