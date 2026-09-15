import {
  OpenFileDialog, SaveFile, SaveFileDialog, AbrirFicheiro,
  GetSettings, SaveSettings, DetectMaxima, InstallMaxima, KillMaxima,
  CheckLatexDeps, InstallLatex, CheckDocDeps, InstallPandoc,
  GeneratePDF, SavePDFDialog, SaveTexDialog, ExportMarkdown, ExportDocx, ExportOdt, SaveUploadedImage,
  ReverseSearch,
  PublicarEstadoResultado, AbrirXanelaResultado, GetEstadoResultado,
  AccionResultadoPDF, AccionResultadoTex, AccionResultadoMD, AccionResultadoDocx, AccionResultadoOdt,
  AbrirXanelaXeradorIA, AcoplarXeradorIA, XanelaXeradorIAAberta,
  GardarEstadoXeradorIA, LerEstadoXeradorIA, EntregarXeracionIA,
  XerarContidoIA, XerarExercicioIA, XerarExameIA,
  GardarNaBiblioteca, ListarBiblioteca, EliminarDaBiblioteca,
  GetActualizacionYang, AplicarActualizacionYang,
  TaoContextoActivo, EnviarATarefaTao, TaoAlumnos, EnviarReparto,
  TaoAlumnoIndividual, EnviarATarefaTaoAlumno,
  IdiomaPiztu,
  ListarCartafol, EscollerCartafolDialog, LerFicheiroTexto, NovoCartafol, RenomearFicheiro, EliminarFicheiro,
  SubirFicheiroCartafol,
  XerarInformeErroIA, GardarInformeErroDialog,
  ListarPlantillas, GardarPlantilla, EliminarPlantilla, DuplicarPlantilla,
  EscollerPlantilla, PlantillaParaFicheiro, SubirImaxePlantilla, EliminarImaxePlantilla,
  PrevisualizarPlantilla, XerarPlantillaIA, ExportarPlantilla, ImportarPlantilla,
  PaqueteLatexQueFalta, InstalarPaqueteLatex,
  ProbarExercicios,
  ListarFontesSistema,
  XerarExercicioTaboa, XerarExameTaboa, XerarCorreccionTaboa,
  IniciarExameTaboa, XerarLoteExameTaboa,
  DiagnosticarCompilacionExercicios, RexenerarExercicioTaboa,
} from '../bindings/matexe-wails/app.js'
import { Application, Events } from '@wailsio/runtime'
import { createEditor } from './editor.js'
import { createBlocksEditor } from './blocks-editor.js'
import { createTableEditor } from './table-editor.js'
import { openExerciseAIModal, openExamAIModal } from './blocks-ai-library-modals.js'
import { createAssistant } from './assistant.js'
import { reorganizarPractica } from './reorganizar.js'
import { autorepararExame } from './auto-reparar.js'
import { createExplorer } from './explorer.js'
import { openBugReportModal } from './bug-report.js'
import { abrirBibliotecaPlantillas } from './plantillas.js'
import { makeDraggable } from './blocks-modal.js'
import { confirmar } from './dialogs.js'
import glDict from '../../idiomas/gl.json'
import esDict from '../../idiomas/es.json'
import enDict from '../../idiomas/en.json'
import ptDict from '../../idiomas/pt.json'

// ── Idioma ───────────────────────────────────────────────────────────────
// gl.json é a fonte da verdade (todas as claves); es/en/pt poden estar
// incompletos (ou baleiros de todo, ver yang/idiomas/README.md) - I18N
// mestura sempre gl por baixo, así calquera clave sen traducir aínda amosa
// o galego en vez dun oco ou da propia clave crúa.
const DICTS = { gl: glDict, es: esDict, en: enDict, pt: ptDict }
let idiomaActual = 'gl'
let I18N = { ...glDict }
// idiomaPiztu: código que Piztu ten escollido (IdiomaPiztu(), ver app.go),
// "" se Yang arrincou sen contexto de Piztu (executable á man). Só un valor
// por defecto — resolto cedo no arranque, antes de que Opcións poida
// abrirse, e usado en openOptions/saveOptions para distinguir "segue a
// Piztu" (idioma: "" en Settings) de "elección explícita".
let idiomaPiztu = ''

function cargarIdioma(code) {
  idiomaActual = DICTS[code] ? code : 'gl'
  I18N = { ...glDict, ...(DICTS[idiomaActual] || {}) }
}

// t busca `clave` no dicionario activo; `fallback` (normalmente o propio
// texto galego xa escrito no HTML/JS) cobre o caso de arrincar antes de
// cargarIdioma() resolver, ou unha clave que nin sequera exista en gl.json
// por un despiste. `vars` interpola {nome} -> valor (ex. t('blocks.exercise.title', 'Exercicio {n}', {n: 3})).
function t(clave, fallback, vars) {
  let val = I18N[clave] ?? fallback ?? clave
  if (vars) {
    for (const k in vars) val = val.replaceAll(`{${k}}`, vars[k])
  }
  return val
}

// aplicarTraducions percorre o DOM estático (index.html) e substitúe cada
// texto marcado por data-i18n* polo valor do idioma activo. O texto galego
// que xa hai escrito en cada elemento serve de fallback (t() cae nel se
// falta a clave), así que non hai "oco" mentres carga nin se falla algo.
function aplicarTraducions() {
  document.querySelectorAll('[data-i18n]').forEach((el) => {
    el.textContent = t(el.dataset.i18n, el.textContent)
  })
  document.querySelectorAll('[data-i18n-title]').forEach((el) => {
    el.title = t(el.dataset.i18nTitle, el.title)
  })
  document.querySelectorAll('[data-i18n-placeholder]').forEach((el) => {
    el.placeholder = t(el.dataset.i18nPlaceholder, el.placeholder)
  })
  document.querySelectorAll('[data-i18n-aria-label]').forEach((el) => {
    el.setAttribute('aria-label', t(el.dataset.i18nAriaLabel, el.getAttribute('aria-label')))
  })
  document.querySelectorAll('[data-i18n-html]').forEach((el) => {
    el.innerHTML = t(el.dataset.i18nHtml, el.innerHTML)
  })
}

const preview = document.getElementById('preview')
const seedInput = document.getElementById('seed')
const iterInput = document.getElementById('iterations')
const btnNew = document.getElementById('btnNew')
const btnOpen = document.getElementById('btnOpen')
const btnSave = document.getElementById('btnSave')
const btnSaveAs = document.getElementById('btnSaveAs')
const btnUndo = document.getElementById('btnUndo')
const btnRedo = document.getElementById('btnRedo')
const editorContainer = document.getElementById('editor')
const blocksContainer = document.getElementById('blocksEditor')
const tableContainer = document.getElementById('tableEditor')
const btnEditorMode = document.getElementById('btnEditorMode')
const btnEditorModeLabel = document.getElementById('btnEditorModeLabel')
const matexeTagBar = document.getElementById('matexeTagBar')
const btnRun = document.getElementById('btnRun')
const plantillaSelect = document.getElementById('plantillaSelect')
const btnPlantillas = document.getElementById('btnPlantillas')
const btnAxustesTexto = document.getElementById('btnAxustesTexto')
const btnKillMaxima = document.getElementById('btnKillMaxima')
const btnPrint = document.getElementById('btnPrint')
const btnPDF = document.getElementById('btnPDF')
const btnExportTex = document.getElementById('btnExportTex')
const btnExportMarkdown = document.getElementById('btnExportMarkdown')
const btnExportDocx = document.getElementById('btnExportDocx')
const btnExportOdt = document.getElementById('btnExportOdt')
const btnOptions = document.getElementById('btnOptions')
const btnAbout = document.getElementById('btnAbout')
const sobreOverlay = document.getElementById('sobreOverlay')
const btnAboutClose = document.getElementById('btnAboutClose')
const sobreVersion = document.getElementById('sobreVersion')
const sobreActualizacion = document.getElementById('sobreActualizacion')
const btnActualizarYang = document.getElementById('btnActualizarYang')
const statusEl = document.getElementById('status')
const btnAbrirGardado = document.getElementById('btnAbrirGardado')
const warningsEl = document.getElementById('warnings')

const optionsOverlay = document.getElementById('optionsOverlay')
const optMaximaPath = document.getElementById('optMaximaPath')
const optCodeIni = document.getElementById('optCodeIni')
const optIAPreset = document.getElementById('optIAPreset')
const optIAApiKey = document.getElementById('optIAApiKey')
const optIAModel = document.getElementById('optIAModel')
const optIABaseURL = document.getElementById('optIABaseURL')
const optIAMaxTokens = document.getElementById('optIAMaxTokens')
const optIATimeout = document.getElementById('optIATimeout')
const optIABaseURLRow = document.getElementById('optIABaseURLRow')
const optLatexEngine = document.getElementById('optLatexEngine')
const optDecimais = document.getElementById('optDecimais')
const optDecimal = document.getElementById('optDecimal')
const optTimeout = document.getElementById('optTimeout')
const optReorgSementes = document.getElementById('optReorgSementes')
const optReorgRoldas = document.getElementById('optReorgRoldas')
const optAutoreparar = document.getElementById('optAutoreparar')
const optMaxRechamadasIA = document.getElementById('optMaxRechamadasIA')
const optXerarAoGardar = document.getElementById('optXerarAoGardar')
const optIdioma = document.getElementById('optIdioma')
const optEditorMode = document.getElementById('optEditorMode')
const optExportPDF = document.getElementById('optExportPDF')
const optExportTex = document.getElementById('optExportTex')
const optExportMarkdown = document.getElementById('optExportMarkdown')
const optExportDocx = document.getElementById('optExportDocx')
const optExportOdt = document.getElementById('optExportOdt')
const btnDetect = document.getElementById('btnDetect')
const btnOptionsCancel = document.getElementById('btnOptionsCancel')
const btnOptionsSave = document.getElementById('btnOptionsSave')

const maximaWarning = document.getElementById('maximaWarning')
const btnInstallMaxima = document.getElementById('btnInstallMaxima')
const btnMaximaManual = document.getElementById('btnMaximaManual')
const btnMaximaDismiss = document.getElementById('btnMaximaDismiss')
const maximaManualOverlay = document.getElementById('maximaManualOverlay')
const btnMaximaManualClose = document.getElementById('btnMaximaManualClose')

const latexWarning = document.getElementById('latexWarning')
const latexWarningText = document.getElementById('latexWarningText')
const btnInstallLatex = document.getElementById('btnInstallLatex')
const btnLatexManual = document.getElementById('btnLatexManual')
const btnLatexDismiss = document.getElementById('btnLatexDismiss')
const latexManualOverlay = document.getElementById('latexManualOverlay')
const btnLatexManualClose = document.getElementById('btnLatexManualClose')

const pandocWarning = document.getElementById('pandocWarning')
const btnInstallPandoc = document.getElementById('btnInstallPandoc')
const btnPandocManual = document.getElementById('btnPandocManual')
const btnPandocDismiss = document.getElementById('btnPandocDismiss')
const pandocManualOverlay = document.getElementById('pandocManualOverlay')
const btnPandocManualClose = document.getElementById('btnPandocManualClose')

// Modo dúas xanelas (Opcións → Modo de traballo) - ver xanelaresultado.go.
// panePreview/panePreviewPlaceholder son exclusivos da xanela EDITOR
// (activarModoDobreEditor agocha unha e amosa a outra cando o resultado
// vive noutra xanela); btnReabrirResultado vive dentro do placeholder.
const panePreview = document.querySelector('.pane-preview')
const panePreviewPlaceholder = document.querySelector('.pane-preview-placeholder')
const placeholderAviso = document.querySelector('.placeholder-aviso')
const btnReabrirResultado = document.getElementById('btnReabrirResultado')
const btnDesacoplarResultado = document.getElementById('btnDesacoplarResultado')

// paneEditor/paneSplitter: divisor arrastrable entre código e resultado -
// ver initPaneSplitter, máis abaixo.
const paneEditor = document.querySelector('.pane-editor')
const paneSplitter = document.getElementById('paneSplitter')

// Explorador de cartafoles/ficheiros (ver explorer.js) - btnToggleExplorer
// alterna a súa visibilidade; `explorer` créase máis abaixo, unha vez
// coñecido Settings.ultimoCartafol (ver iniciarExplorer, preto do arranque
// asíncrono de idioma).
const explorerSidebar = document.getElementById('explorerSidebar')
const btnToggleExplorer = document.getElementById('btnToggleExplorer')
let explorer = null

let currentPath = null
let lastPDFBase64 = null
let lastLatexSource = null
// editMode: 'text' (CodeMirror) | 'blocks' (Blockly) | 'table' (folla de
// cálculo). A interface por defecto é 'table' (ver switchMode máis abaixo e
// Settings.EditorMode); 'text' segue existindo para o "código de fondo" do
// modo dúas xanelas, pero xa non é o modo de arranque.
let editMode = 'text'
let blocksEditor = null
let tableEditor = null
// resultadoDesacoplado/codigoDeFondoActivo/intervalCodigoFondo: estado do
// panel de fondo en modo dúas xanelas - ver mostrarCodigoDeFondo/
// actualizarPanelFondo, máis abaixo. Declarados aquí (non xunto a esas
// funcións) porque switchMode xa os le dende o `switchMode('blocks')` do
// arranque (uns liñas máis abaixo), antes de chegar a esa sección.
let resultadoDesacoplado = false
let codigoDeFondoActivo = false
let intervalCodigoFondo = null
// previewZoom: nivel de zoom (Ctrl+/Ctrl-/Ctrl+0) das páxinas dentro do
// iframe de previsualización - ver pagesToPreviewHTML/ZOOM_SCRIPT. Vive aquí
// (non dentro do iframe) para sobrevivir a unha re-xeración (Xerar substitúe
// todo o srcdoc, o script inxectado esquecería o nivel); o propio script
// avisa dos cambios por postMessage (handleZoomMessage, máis abaixo).
let previewZoom = 1
// xerarAoGardarActivo reflicte Opcións → "Xerar automaticamente ao gardar"
// (Settings.XerarAoGardar, settings.go) - true por defecto (mesmo criterio
// có Go: !s.xerarAoGardar só es false cando o profesorado o desactivou
// explicitamente). saveFile/saveFileAs lena para decidir se chaman run()
// despois de gardar; actualízase ao arrincar (GetSettings, ao final deste
// ficheiro) e cada vez que se garda Opcións (saveOptions).
let xerarAoGardarActivo = true
// souXanelaResultado dío unha marca global inxectada no HTML polo propio
// proceso Go (ver main.go, resultadoMiddleware - Wails v2 non ten URL propia
// por xanela coma v3, así que a distinción xa non pode vir de
// window.location.search: agora "resultado" é un PROCESO separado deste
// mesmo binario, ver xanelaresultado.go). Dispoñible de forma síncrona dende
// o primeiro instante (o <script> inxéctase antes de calquera módulo, ver
// main.go) - mesma garantía que tiña a URL antes. true significa "esta
// xanela é a de resultado desacoplada, non teño editor propio" - run()/
// generatePDF()/exportTex()/exportMarkdown()/exportDocOffice() mírano para
// reenviar a acción ao proceso editor (accionRemota) no canto de intentar
// compilar cun editor baleiro.
const souXanelaResultado = new URLSearchParams(window.location.search).get('resultado') === '1'

// souXeradorIA: esta xanela é o "Xerador con IA" desacoplado (botón
// "Desacoplar" da modal de Exercicio/Exame con IA, ver xanelaxeradoria.go).
// ?xerador=exercicio|exame decide que formulario amosa. Coma
// souXanelaResultado: non ten editor propio, só o formulario; o que xera
// mándao á xanela principal (EntregarXeracionIA -> evento yang:xerador-ia-*).
const xeradorIAModo = new URLSearchParams(window.location.search).get('xerador')
const souXeradorIA = xeradorIAModo === 'exercicio' || xeradorIAModo === 'exame'

const editor = createEditor(document.getElementById('editor'), {
  // initialDoc baleiro a propósito: Yang arrincaba sempre cun exemplo feito
  // (documento de mostra con EVAL/HIDE) coma initialDoc - pedido
  // explicitamente que non apareza nada ao abrir, coma calquera editor
  // normal (páxina en branco, currentPath a null coma newFile()).
  initialDoc: '',
  // onChange: só importa mentres o código de fondo (modo bloques desacoplado,
  // ver mostrarCodigoDeFondo) está activo - manexarCambioCodigoDeFondo xa se
  // encarga de ignoralo no resto de casos.
  onChange: () => manexarCambioCodigoDeFondo(),
})

// assistant: instancia única do asistente de IA (icona 🤖, ver
// assistant.js) - compartida entre o editor de Código (botón na barra de
// etiquetas, abaixo) e cada modal de anaco do editor de bloques (inxectado
// coma abrirAsistente en novoBlocksEditor).
const assistant = createAssistant({ t })

// bloqueReorganizar arma o obxecto `reorganizar` que espera assistant.open()
// (ver assistant.js / reorganizar.js). Compártese entre o editor de Código e
// a interface de táboa: só cambia de onde sae o texto de partida
// (`getSource`) e onde se aplica o resultado (`aplicar`).
//   getSource()  -> .matex completo a reorganizar
//   aplicar(txt) -> escribe o .matex reorganizado (vía non destrutiva)
//   soIndice     -> nº de exercicio a reorganizar en solitario, ou null
async function bloqueReorganizar({ getSource, aplicar, soIndice = null }) {
  let defaults = { sementes: 25, roldas: 3, adaptar: true }
  try {
    const s = await GetSettings()
    defaults = {
      sementes: s.reorganizarSementes || 25,
      roldas: s.reorganizarRoldas || 3,
      adaptar: true,
    }
  } catch { /* valores por defecto */ }
  return {
    soExercicio: soIndice,
    defaults,
    aplicar,
    correr: ({ sementes, roldas, adaptar, onProgreso }) => reorganizarPractica(getSource(), {
      xerarCorreccion: (req) => XerarCorreccionTaboa(req),
      probar: (req) => ProbarExercicios(req),
      sementes, roldas, adaptar, soIndice, onProgreso,
    }),
  }
}

document.getElementById('btnAssistantCode').addEventListener('click', async () => {
  const alvo = editor.getSelectionOrAll()
  // "Reorganizar" traballa sempre co documento ENTEIRO (repartir enunciados /
  // resultado / resolución non ten sentido sobre unha selección parcial).
  const reorganizar = await bloqueReorganizar({
    getSource: () => editor.getValue(),
    aplicar: (txt) => editor.setValue(txt),
  })
  assistant.open({
    tipo: 'documento',
    title: t('assistant.codeTitle', 'Asistente — Código'),
    getContent: () => alvo.text,
    setContent: (texto) => editor.replaceRange(alvo.from, alvo.to, texto),
    reorganizar,
  })
})

function novoBlocksEditor(initialDoc) {
  return createBlocksEditor(blocksContainer, {
    initialDoc, uploadImage: uploadImageFile, canUploadImage, generateWithAI, generateExerciseWithAI,
    generateExamWithAI, saveToLibrary, listLibrary, deleteFromLibrary, t,
    // idioma: só para as mensaxes propias de Blockly (menú contextual
    // nativo, tooltips do zoom) - o noso texto xa vai en t().
    idioma: idiomaActual,
    abrirAsistente: (opts) => assistant.open(opts),
    onToggleFullWidth: setBlocksFullWidth,
    // onGenerateResult: tras xerar un exercicio/exame coa IA, compilar tamén
    // o documento enteiro (mesmo que premer "Xerar" na barra) para ver o
    // resultado final sen saír da modal.
    onGenerateResult: () => run(),
    // Desacople das modais de IA a unha xanela á parte (ver xanelaxeradoria.go):
    // gárdase o estado do formulario e ábrese a xanela ?xerador=<modo>.
    onDetachExerciseAI: async (est) => {
      await GardarEstadoXeradorIA(JSON.stringify(est || {}))
      await AbrirXanelaXeradorIA('exercicio')
    },
    onDetachExamAI: async (est) => {
      await GardarEstadoXeradorIA(JSON.stringify(est || {}))
      await AbrirXanelaXeradorIA('exame')
    },
    xeradorIAAberta: () => XanelaXeradorIAAberta(),
    focarXeradorIA: (modo) => AbrirXanelaXeradorIA(modo),
  })
}

// novoTableEditor: mesma idea ca novoBlocksEditor pero para a interface de
// táboa (table-editor.js). Contrato equivalente (getValue/setValue/focus/
// destroy/undo/redo) máis getModo/setModo/onModoChange para as lapelas de
// visualización - onModoChange dispara xerarConAutoreparacion() para que a
// previsualización da dereita cambie coa lapela activa (enunciados /
// +resposta / +solución) e, se esa lapela concreta non compila (é habitual
// en "+ resolución": leva máis <TIKZ>/<MAT> ca o enunciado só), se intente
// arranxar soa coa IA antes de amosarlle o erro cru ao profesorado.
function novoTableEditor(initialDoc) {
  const ed = createTableEditor(tableContainer, {
    initialDoc, t,
    abrirAsistente: (opts) => assistant.open(opts),
    probarExercicios: (req) => ProbarExercicios(req),
    // crearReorganizar: activa a barra "Reorganizar práctica" do asistente
    // tamén desde a táboa (ver bloqueReorganizar arriba / reorganizar.js).
    crearReorganizar: (args) => bloqueReorganizar(args),
    // A táboa usa a xeración con IA ESTRUTURADA (enunciado/variables/
    // resultado/resolucion), non a de "anacos" do editor de bloques.
    generateExerciseWithAI: generateExerciseTaboaWithAI,
    generateExamWithAI: generateExamTaboaWithAI,
    startExamWithAI: startExamTaboaWithAI,
    generateExamChunkWithAI: generateExamChunkTaboaWithAI,
    onToggleFullWidth: setBlocksFullWidth,
    // Tras xerar coa IA: compilar e, se falla, autorreparar por exercicio
    // (Opcións → «Autoreparar…», por defecto activado). Ver auto-reparar.js.
    // Mesma función que onModoChange (abaixo) - así un exame xerado a man
    // (sen pasar por xerarConAutoreparacion aquí) que falle ao cambiar de
    // lapela recibe igualmente o intento de arranxo automático.
    onGenerateResult: () => xerarConAutoreparacion(),
    // onGenerateExercise: dispárao o botón ✓ dunha fila. Editar a táboa NON
    // recompila nada (evita a "conxelación" ao cambiar valores e os erros con
    // expresións a medio escribir); só este botón, e só para o exercicio
    // premido - á dereita amósase unicamente ese exercicio, sen tocar o resto
    // da práctica. Para a práctica enteira segue estando "Xerar" na barra.
    onGenerateExercise: (source) => { if (editMode === 'table') run(source) },
  })
  // Ao trocar de lapela rexérase a previsualización no modo novo (enunciados
  // / +resposta / +solución) e, se falla, autorreparándoa (mesmo camiño ca
  // tras "Exame completo con IA", ver onGenerateResult arriba) - así un
  // exercicio que só falla en "+ resolución" (p.ex. un <TIKZ> da resolución
  // mal pechado, "Incomplete \iffalse") arránxase soa a IA en vez de
  // amosarlle ao profesorado o erro cru de xelatex para que o copie e o
  // pegue no Asistente á man. A lapela "Probas" ten o seu propio botón de
  // execución, non ten sentido lanzar unha xeración completa ao abrila.
  ed.onModoChange((m) => { if (editMode === 'table' && m !== 'proba') xerarConAutoreparacion() })
  return ed
}

// modoActivo: o `modo` (<RESP>/<SOL>) que hai que pasarlle ao backend na
// próxima xeración. Só ten efecto na interface de táboa; en bloques/texto é
// sempre "enunciados" (comportamento de sempre).
function modoActivo() {
  if (editMode === 'table' && tableEditor) {
    const m = tableEditor.getModo()
    return m === 'proba' ? 'solucions' : m
  }
  return 'enunciados'
}

// setBlocksFullWidth: chamada dende o botón "⤢ Ampliar"/"⤡ Repregar" do
// editor de bloques (ver blocks-editor.js, onToggleFullWidth) - agocha (ou
// amosa de novo) panePreviewPlaceholder + paneSplitter. Como
// panePreviewPlaceholder aloxa o propio código de fondo mentres está activo
// (ver mostrarCodigoDeFondo), agochalo agocha tamén ese código - efecto
// pedido "ao ampliar, oculta a ventá de código". display inline (non
// .hidden, que xa usan activarModoDobreEditor/saírModoDobreEditor para
// outro motivo sobre o MESMO elemento) para non interferir con eses dous.
//
// Non abonda con agochar o irmán: .pane-editor ten un width propio (45% por
// CSS, ou un valor en px gardado polo divisor arrastrable, ver
// aplicarLarguraEditor/PANE_SPLITTER_KEY) - #panes é flex, pero un fillo con
// width fixo NON medra só por quedar libre o espazo do seu irmán agochado
// (flex-grow por defecto é 0). anchoEditorPrevio garda ese width (o que
// houbese, inline ou baleiro) para restauralo ao repregar - así non se perde
// o ancho que o profesorado arrastrase antes de ampliar.
let anchoEditorPrevio = null
function setBlocksFullWidth(full) {
  panePreviewPlaceholder.style.display = full ? 'none' : ''
  paneSplitter.style.display = full ? 'none' : ''
  if (full) {
    anchoEditorPrevio = paneEditor.style.width
    paneEditor.style.width = '100%'
  } else {
    paneEditor.style.width = anchoEditorPrevio || ''
    anchoEditorPrevio = null
  }
}

// Interface de táboa por defecto (pedido explícito) - Settings.EditorMode
// pode cambiala a 'blocks' xa no arranque asíncrono (ver máis abaixo, onde
// se resolve GetSettings). Arrincar en 'text' cru non é amigable para
// ningún dos dous perfís.
switchMode('table')

// --- Modo de edición: texto (CodeMirror) <-> bloques (Blockly) <-> táboa ---
// O documento .matex sempre viaxa como unha soa string cara ao backend
// (Generate/SaveFile/GeneratePDF xa usaban editor.getValue() antes destes
// modos); en modo bloques/táboa, syncToText() volca esa string en `editor`
// antes de calquera desas chamadas, para que o resto do fluxo non cambie nada.
function syncToText() {
  if (editMode === 'blocks' && blocksEditor) {
    editor.setValue(blocksEditor.getValue())
  } else if (editMode === 'table' && tableEditor) {
    editor.setValue(tableEditor.getValue())
  }
}

// fileToBase64/uploadImageFile: única ponte entre blocks.js (que non fala
// nunca cos bindings Wails, ver blocks.js) e SaveUploadedImage. dirOf xa
// existe embaixo, reutilizado tal cal por generatePDF.
function fileToBase64(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result).split(',')[1] || '')
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

async function uploadImageFile(file) {
  return SaveUploadedImage({
    baseDir: dirOf(currentPath),
    fileName: file.name,
    dataB64: await fileToBase64(file),
  })
}

// uploadFileToExplorerFolder: ponte entre explorer.js (botón "⬆️ Subir
// ficheiro", calquera cartafol da árbore, non só a beira do .matex aberto)
// e SubirFicheiroCartafol (explorer.go) - mesmo criterio ca
// uploadImageFile enriba, pero destDir chega xa resolto dende
// explorer.js (o cartafol premido) en vez de ser sempre dirOf(currentPath).
// baseDir (para RelPath, ver explorer.go) segue sendo o do .matex aberto:
// é ESE documento o que precisa a ruta relativa para <IMG src>/<A href>.
async function uploadFileToExplorerFolder(destDir, file) {
  return SubirFicheiroCartafol({
    destDir,
    baseDir: dirOf(currentPath),
    fileName: file.name,
    dataB64: await fileToBase64(file),
  })
}

// canUploadImage: comprobación previa (← botón "Escoller ficheiro…" do
// bloque Imaxe) para non abrir o selector do sistema cando aínda non hai
// onde gardar a imaxe - SaveUploadedImage esixe baseDir non baleiro (garda
// as imaxes canda o .matex, ver app.go), e dirOf(currentPath) só é "" cando
// o exame é novo e aínda non se gardou. Devolve null se se pode subir, ou a
// mensaxe a amosar se non.
function canUploadImage() {
  return currentPath ? null : t('blocks.imageUpload.needSave', 'Garda o exame primeiro (Ctrl+S) para poder engadir imaxes.')
}

// generateWithAI: única ponte entre blocks.js (botón ✨) e XerarContidoIA -
// mesmo criterio que uploadImageFile para a subida de imaxes.
async function generateWithAI(tipo, peticion) {
  return XerarContidoIA(tipo, peticion)
}

// generateExerciseWithAI: mesmo criterio, para o botón "✨ Exercicio con IA"
// (crea un exercicio enteiro, non un anaco solto). modeloFiles é opcional:
// File[] (PDF) que a modal recolle dun <input type="file"> normal,
// convertidos aquí a base64 (mesmo xeito ca generateExamWithAI) antes de
// mandarllos ao backend.
async function generateExerciseWithAI(peticion, modeloFiles) {
  const modelos = []
  for (const file of modeloFiles || []) {
    modelos.push({ fileName: file.name, dataB64: await fileToBase64(file) })
  }
  return XerarExercicioIA(peticion, modelos)
}

// generateExamWithAI: mesmo criterio, para o botón "✨ Exame completo con
// IA" (crea VARIOS exercicios de vez, a partir dun tema e dun número de
// exercicios). modeloFiles é opcional: File[] (PDF) que blocks.js recolle
// dun <input type="file"> normal, convertidos aquí a base64 (mesmo xeito ca
// fileToBase64/uploadImageFile) antes de mandarllos ao backend.
async function generateExamWithAI(tema, numExercicios, modeloFiles) {
  return XerarExameIA(tema, numExercicios, await filesToModelos(modeloFiles))
}

// filesToModelos: File[] (PDF que a modal recolle) -> [{fileName, dataB64}]
// que esperan os bindings de IA.
async function filesToModelos(modeloFiles) {
  const modelos = []
  for (const file of modeloFiles || []) {
    modelos.push({ fileName: file.name, dataB64: await fileToBase64(file) })
  }
  return modelos
}

// Xeración con IA para a interface de TÁBOA: a IA devolve xa a estrutura
// (enunciado / variables / resultado / resolucion), ver taboa_ia.go. Só as
// usa novoTableEditor.
async function generateExerciseTaboaWithAI(peticion, modeloFiles) {
  return XerarExercicioTaboa(peticion, await filesToModelos(modeloFiles))
}
async function generateExamTaboaWithAI(tema, numExercicios, modeloFiles) {
  return XerarExameTaboa(tema, numExercicios, await filesToModelos(modeloFiles))
}

// Xeración do exame POR LOTES (taboa_ia.go): startExamTaboaWithAI pide só o
// guión (rápido, e xa di cantos exercicios hai cando se escolleu
// "indefinido") e generateExamChunkTaboaWithAI desenvolve un anaco dese
// guión. A modal lanza varios anacos á vez e vai inserindo os que chegan, así
// que ningún exercicio xa xerado se perde nin hai que refacelo se un anaco
// falla. Se algo destes dous falla, a modal cae de volta en
// generateExamTaboaWithAI (unha soa chamada, coma antes).
async function startExamTaboaWithAI(tema, numExercicios, modeloFiles) {
  return IniciarExameTaboa(tema, numExercicios, await filesToModelos(modeloFiles))
}
async function generateExamChunkTaboaWithAI(sesion, desde, cantos) {
  return XerarLoteExameTaboa(sesion, desde, cantos)
}

// saveToLibrary/listLibrary/deleteFromLibrary: única ponte entre blocks.js
// (botóns "💾"/"📚 Biblioteca") e GardarNaBiblioteca/ListarBiblioteca/
// EliminarDaBiblioteca - mesmo criterio que as pontes de IA de enriba.
async function saveToLibrary(nome, tipo, contido) {
  return GardarNaBiblioteca(nome, tipo, contido)
}
async function listLibrary() {
  return ListarBiblioteca()
}
async function deleteFromLibrary(id) {
  return EliminarDaBiblioteca(id)
}

// activeSecondaryEditor: o editor non-texto activo agora mesmo (bloques ou
// táboa), ou null se estamos en 'text'. Usado polo panel de "código de
// fondo" (modo dúas xanelas) e por desfacer/refacer, para non repetir o
// mesmo if en cada sitio.
function activeSecondaryEditor() {
  if (editMode === 'blocks') return blocksEditor
  if (editMode === 'table') return tableEditor
  return null
}

function switchMode(mode) {
  if (mode === editMode) return
  syncToText() // volca o modo saínte (se é bloques/táboa) a `editor`
  const doc = editor.getValue()

  editorContainer.classList.add('hidden')
  matexeTagBar.classList.add('hidden')
  blocksContainer.classList.add('hidden')
  tableContainer.classList.add('hidden')

  if (mode === 'blocks') {
    if (!blocksEditor) blocksEditor = novoBlocksEditor(doc)
    else blocksEditor.setValue(doc)
    blocksContainer.classList.remove('hidden')
    blocksEditor.focus()
  } else if (mode === 'table') {
    if (!tableEditor) tableEditor = novoTableEditor(doc)
    else tableEditor.setValue(doc)
    tableContainer.classList.remove('hidden')
    tableEditor.focus()
  } else {
    editorContainer.classList.remove('hidden')
    matexeTagBar.classList.remove('hidden')
    editor.focus()
  }
  editMode = mode
  actualizarBtnEditorMode()
  actualizarPanelFondo()
}

// actualizarBtnEditorMode: o botón da barra amosa a interface Á QUE se
// cambiaría (a "outra"), non a activa. Só alterna entre táboa e bloques - o
// modo 'text' non ten botón (é interno, do código de fondo).
function actualizarBtnEditorMode() {
  if (!btnEditorModeLabel) return
  const irA = editMode === 'table' ? 'blocks' : 'table'
  btnEditorModeLabel.textContent = irA === 'table'
    ? t('toolbar.editorMode.toTable', 'Táboa')
    : t('toolbar.editorMode.toBlocks', 'Bloques')
  btnEditorMode.setAttribute('aria-label', btnEditorModeLabel.textContent)
}

if (btnEditorMode) {
  btnEditorMode.addEventListener('click', () => {
    const irA = editMode === 'table' ? 'blocks' : 'table'
    switchMode(irA)
    // Persistir a escolla (mesmo criterio ca o explorador, onRootChanged):
    // léese GetSettings enteiro e vólvese gardar co editorMode cambiado.
    GetSettings().then((s) => SaveSettings({ ...s, editorMode: irA === 'table' ? 'tabla' : 'blockly' })).catch(() => {})
  })
}

// --- Divisor arrastrable entre código e resultado (#paneSplitter) ---
// Só ten sentido en modo xanela única (con editor E resultado visibles á
// vez nesta mesma xanela) - en modo dúas xanelas, cada un vive na súa
// propia xanela OS, xa redimensionable arrastrando o propio bordo da
// xanela (ver entrarModoResultado/activarModoDobreEditor, que agochan
// #paneSplitter coma o resto do panel que xa non aplica).
const PANE_SPLITTER_KEY = 'yang.editorWidthPx'
const PANE_MIN_PX = 220 // mesmo mínimo có CSS (.pane-editor/.pane-preview min-width)

function aplicarLarguraEditor(px) {
  paneEditor.style.width = px + 'px'
}

// dragOverlay é un <div> transparente a pantalla completa que só existe
// mentres se arrastra o divisor - sen el, en canto o rato pasa por riba do
// <iframe id="preview"> (á dereita) o arrastre "atascábase": un iframe ten
// o seu propio documento, así que os mousemove que caen enriba del non
// chegan ao listener do documento principal (window) - só se notaba cara á
// dereita porque cara á esquerda o rato nunca sae do panel de código, sen
// iframe polo medio. O overlay tápao todo (z-index alto) e recibe el os
// eventos no seu lugar, coma calquera <div> normal do documento principal.
let dragOverlay = null
function crearDragOverlay() {
  const div = document.createElement('div')
  div.className = 'pane-splitter-overlay'
  document.body.appendChild(div)
  return div
}

function initPaneSplitter() {
  // Largura gardada de sesións anteriores (localStorage - preferencia
  // puramente visual desta máquina, non fai falla pasar por Opcións/Go).
  const gardada = parseInt(localStorage.getItem(PANE_SPLITTER_KEY), 10)
  if (gardada) aplicarLarguraEditor(gardada)

  let arrastrando = false

  paneSplitter.addEventListener('mousedown', (e) => {
    arrastrando = true
    paneSplitter.classList.add('dragging')
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    dragOverlay = crearDragOverlay()
    e.preventDefault()
  })

  window.addEventListener('mousemove', (e) => {
    if (!arrastrando) return
    const panesRect = paneEditor.parentElement.getBoundingClientRect()
    const max = panesRect.width - PANE_MIN_PX - paneSplitter.offsetWidth
    const px = Math.min(Math.max(e.clientX - panesRect.left, PANE_MIN_PX), max)
    aplicarLarguraEditor(px)
  })

  window.addEventListener('mouseup', () => {
    if (!arrastrando) return
    arrastrando = false
    paneSplitter.classList.remove('dragging')
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
    if (dragOverlay) {
      dragOverlay.remove()
      dragOverlay = null
    }
    localStorage.setItem(PANE_SPLITTER_KEY, String(paneEditor.getBoundingClientRect().width))
  })
}
initPaneSplitter()

// ── Axustes do texto (botón entre "Plantillas" e "Imprimir") ──────────────
// Panel flotante e arrastrable con tamaño de letra, interliñado e familia
// tipográfica. Aplícase á saída PDF (GeneratePDF.tipografia -> latexdoc.go).
// Persístese en localStorage (preferencia visual local, coma o divisor
// arrastrable - non pasa por Settings/Go).
const AXUSTES_TEXTO_KEY = 'yang.tipografia'
let tipografia = { tamanoPt: 0, interlinado: 0, fonte: '' }
try {
  const g = JSON.parse(localStorage.getItem(AXUSTES_TEXTO_KEY) || '{}')
  tipografia = {
    tamanoPt: Number(g.tamanoPt) || 0,
    interlinado: Number(g.interlinado) || 0,
    fonte: typeof g.fonte === 'string' ? g.fonte : '',
  }
} catch { /* localStorage corrupto: quédanse os valores por defecto */ }

// tipografiaActual devólvese en cada petición de GeneratePDF. Os campos a
// 0/"" significan "por defecto" tamén no backend (ver TipografiaOpts).
function tipografiaActual() {
  return { ...tipografia }
}

const axustesTextoPanel = document.getElementById('axustesTextoPanel')
const axustesTextoHead = document.getElementById('axustesTextoHead')
const optTextoSize = document.getElementById('optTextoSize')
const optTextoLeading = document.getElementById('optTextoLeading')
const optTextoFont = document.getElementById('optTextoFont')
const optTextoFontRecom = document.getElementById('optTextoFontRecom')
const optTextoFontSistema = document.getElementById('optTextoFontSistema')
if (optTextoFontRecom) optTextoFontRecom.label = t('axustesTexto.font.recom', 'Recomendadas')
if (optTextoFontSistema) optTextoFontSistema.label = t('axustesTexto.font.sistema', 'Do sistema (xelatex)')

// fontesSistemaCargadas: as familias que devolve ListarFontesSistema
// (fc-list) só se piden a primeira vez que se abre o panel. null = aínda non
// se pediron; [] = pedíronse e non hai (fc-list ausente).
let fontesSistemaCargadas = null
async function cargarFontesSistema() {
  if (fontesSistemaCargadas !== null || !optTextoFontSistema) return
  try {
    fontesSistemaCargadas = (await ListarFontesSistema()) || []
  } catch {
    fontesSistemaCargadas = []
  }
  optTextoFontSistema.innerHTML = ''
  for (const nome of fontesSistemaCargadas) {
    const o = document.createElement('option')
    o.value = nome
    o.textContent = nome
    optTextoFontSistema.appendChild(o)
  }
  optTextoFontSistema.hidden = fontesSistemaCargadas.length === 0
}

function pintarAxustesTexto() {
  optTextoSize.value = tipografia.tamanoPt ? String(tipografia.tamanoPt) : ''
  optTextoLeading.value = tipografia.interlinado ? String(tipografia.interlinado) : '0'
  // Se a fonte gardada é do sistema e a lista aínda non cargou (ou fallou),
  // engádese como opción solta para que o selector a poida amosar.
  const f = tipografia.fonte || ''
  if (f && !optTextoFont.querySelector(`option[value="${CSS.escape(f)}"]`)) {
    const o = document.createElement('option')
    o.value = f
    o.textContent = f
    ;(optTextoFontSistema || optTextoFont).appendChild(o)
  }
  optTextoFont.value = f
}

function gardarAxustesTexto() {
  tipografia = {
    tamanoPt: parseFloat(optTextoSize.value) || 0,
    interlinado: parseFloat(optTextoLeading.value) || 0,
    fonte: optTextoFont.value || '',
  }
  try { localStorage.setItem(AXUSTES_TEXTO_KEY, JSON.stringify(tipografia)) } catch { /* sen persistencia, non pasa nada */ }
}

if (axustesTextoPanel) {
  makeDraggable(axustesTextoPanel, axustesTextoHead)
  // Non arrancar un arrastre ao premer o ✕ (está dentro da cabeceira-asa).
  document.getElementById('btnAxustesTextoClose').addEventListener('mousedown', (e) => e.stopPropagation())

  const toggleAxustesTexto = async () => {
    const oculto = axustesTextoPanel.classList.toggle('hidden')
    if (oculto) return
    await cargarFontesSistema()
    pintarAxustesTexto()
    optTextoSize.focus()
  }
  btnAxustesTexto && btnAxustesTexto.addEventListener('click', toggleAxustesTexto)
  document.getElementById('btnAxustesTextoClose').addEventListener('click', () => axustesTextoPanel.classList.add('hidden'))

  for (const el of [optTextoSize, optTextoLeading, optTextoFont]) {
    el.addEventListener('change', gardarAxustesTexto)
  }
  document.getElementById('btnTextoXerar').addEventListener('click', () => { gardarAxustesTexto(); run() })
  document.getElementById('btnTextoReset').addEventListener('click', () => {
    tipografia = { tamanoPt: 0, interlinado: 0, fonte: '' }
    try { localStorage.removeItem(AXUSTES_TEXTO_KEY) } catch { /* nada */ }
    pintarAxustesTexto()
    run()
  })
}

// Barra de etiquetas Matexe: mesma acción cós atallos Ctrl+M/E/H/P/T de
// editor.js, pero visible e clicable - un só listener delegado en vez dun
// por botón.
matexeTagBar.addEventListener('click', (e) => {
  const btn = e.target.closest('.matexe-tag-btn')
  if (btn) editor.wrapTag(btn.dataset.tag)
})

// --- Semente: aleatoria por defecto, contador simple (1, 2, 3...) en canto
// se toca a frecha. Un valor aleatorio evita que se xere sempre o mesmo
// exame se ninguén cambia nada; pero mover a frecha ±1 dende ese número
// grande non serviría de nada ("versión 84719, 84718..."), así que o
// primeiro toque "esquece" o aleatorio e empeza en 1.
let seedIsRandom = true
let prevSeedValue = String(Math.floor(Math.random() * 100000))
seedInput.value = prevSeedValue

seedInput.addEventListener('input', () => {
  if (seedIsRandom) {
    seedIsRandom = false
    const diff = Number(seedInput.value) - Number(prevSeedValue)
    if (Math.abs(diff) === 1) {
      // A frecha (ou ↑/↓ do teclado) moveu o valor un paso: é o primeiro
      // "toque", non escritura manual - reinicia a secuencia en 1.
      seedInput.value = '1'
    }
  }
  prevSeedValue = seedInput.value
})

function setStatus(text) {
  statusEl.textContent = text
  btnAbrirGardado.hidden = true
}

// setStatusGardado: igual ca setStatus, pero cando hai un ficheiro local
// recén xerado (PDF/tex/md/docx/odt - GeneratePDF/SavePDFDialog/
// SaveTexDialog/ExportMarkdown/ExportDocx/ExportOdt/AccionResultado*) amosa
// ademais un botón "Abrir" ao carón que o abre coa aplicación por defecto
// do sistema (AbrirFicheiro, abrirficheiro.go) - antes só se podía ver a
// ruta no propio texto do status e ir buscala á man no explorador.
function setStatusGardado(text, ruta) {
  setStatus(text)
  if (!ruta) return
  btnAbrirGardado.hidden = false
  btnAbrirGardado.onclick = () => AbrirFicheiro(ruta).catch((err) => showWarnings([mensaxeErro(err)]))
}

// mensaxeErro: os erros que chegan dunha chamada ao backend (Go, vía
// bindings de Wails) son un RuntimeError con .message = a mensaxe real
// (ver app.go/latexdoc.go, ex. "o documento está baleiro") - mensaxeErro(err) sobre
// ese obxecto amosaba "RuntimeError: <mensaxe>", un prefixo que non aporta
// nada ao profesorado e faino parecer un erro técnico grave cando adoita ser
// un aviso normal. Mesmo patrón xa usado en auto-reparar.js/
// blocks-ai-library-modals.js; mensaxeErro(err) de fallback cobre calquera cousa
// lanzada que non sexa un Error de verdade (ex. un string solto).
function mensaxeErro(err) {
  return err && err.message ? err.message : String(err)
}

// ── Panel de rexistro (← #log-panel de Piztu, mesmo comportamento: engade
// liñas, corta ás LOG_MAX_LINEAS máis recentes, autoscroll). Arrinca
// minimizado (ver .collapsed en index.html) - só se usa para depurar a
// xeración de documentos (Maxima/pdflatex/Pandoc), non é o foco da app. ──
const LOG_MAX_LINEAS = 500
const logOutput = document.getElementById('log-output')

function log(txt) {
  const linea = document.createElement('div')
  linea.textContent = txt
  logOutput.appendChild(linea)
  while (logOutput.childElementCount > LOG_MAX_LINEAS) {
    logOutput.removeChild(logOutput.firstChild)
  }
  logOutput.scrollTop = logOutput.scrollHeight
}

const logPanel = document.getElementById('log-panel')
const btnToggleLog = document.getElementById('btn-toggle-log')
btnToggleLog.addEventListener('click', () => {
  const colapsado = logPanel.classList.toggle('collapsed')
  btnToggleLog.textContent = colapsado ? '+' : '−'
})

const btnClearLog = document.getElementById('btn-clear-log')
btnClearLog.addEventListener('click', () => {
  logOutput.innerHTML = ''
})

function showWarnings(list) {
  warningsEl.innerHTML = ''
  if (!list || list.length === 0) {
    warningsEl.classList.remove('visible')
    return
  }
  for (const w of list) {
    const div = document.createElement('div')
    div.textContent = '⚠ ' + w
    warningsEl.appendChild(div)
  }
  warningsEl.appendChild(makeBugReportButton(list))
  warningsEl.classList.add('visible')
  ofrecerInstalarPaqueteLatex()
}

// ofrecerInstalarPaqueteLatex: se a compilación fallou porque falta un
// paquete de LaTeX, Yang ofrece instalalo alí mesmo (ver paquetelatex.go).
// Consúltase SEMPRE que se amosan avisos, non só nun sitio concreto: o
// paquete pode faltar ao xerar, ao exportar ou ao previsualizar unha
// plantilla, e o backend xa sabe se hai algo pendente ou non.
//
// O botón amosa o comando exacto que se vai executar (title): instalar
// software de sistema, con permisos de administrador, non se fai sen que se
// vexa o que se fai.
async function ofrecerInstalarPaqueteLatex() {
  let info
  try {
    info = await PaqueteLatexQueFalta()
  } catch {
    return
  }
  if (!info || !info.ficheiro) return
  // showWarnings puido volver correr mentres agardabamos (é asíncrono):
  // se o panel xa non está visible, non hai onde poñer nada.
  if (!warningsEl.classList.contains('visible')) return

  const fila = document.createElement('div')
  fila.className = 'warnings-paquete'
  if (!info.podeInstalar) {
    fila.textContent = t('latexPackage.cannot', 'Falta o paquete de LaTeX «{nome}»: {motivo}.',
      { nome: info.nome, motivo: info.motivo })
    warningsEl.appendChild(fila)
    return
  }

  const btn = document.createElement('button')
  btn.type = 'button'
  btn.className = 'warnings-bugreport-btn'
  btn.textContent = t('latexPackage.install', '⇩ Instalar «{nome}» automaticamente', { nome: info.nome })
  btn.title = t('latexPackage.command', 'Executarase: {comando}', { comando: info.comando })
  btn.addEventListener('click', async () => {
    const textoOriginal = btn.textContent
    btn.disabled = true
    btn.classList.add('is-busy')
    btn.textContent = t('latexPackage.installingBtn', '⏳ Instalando «{nome}»…', { nome: info.nome })
    setStatus(t('latexPackage.installing', 'Instalando o paquete «{nome}»…', { nome: info.nome }))
    log('[LATEX] Instalando ' + info.nome + ': ' + info.comando)
    // Deixar que o navegador pinte o estado "Instalando…" ANTES de arrincar a
    // instalación: pkexec/polkit abre un diálogo nativo que rouba o foco e,
    // en WebKitGTK, pode reter o repintado ata que se pecha - sen esta
    // pausa, o botón quedaba coa lenda vella mentres se pedía o contrasinal.
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
    try {
      const res = await InstalarPaqueteLatex()
      if (res.log) logPdflatex(res.log)
      if (res.success) {
        setStatus(t('latexPackage.installed', 'Paquete «{nome}» instalado', { nome: info.nome }))
        // Volver xerar: é o que quere quen premeu o botón, e así vese
        // decontado se o documento xa compila. (run() reconstrúe #warnings,
        // así que este botón desaparece; non fai falla restaurar a lenda.)
        await run()
      } else {
        setStatus(t('latexPackage.failed', 'Non se puido instalar «{nome}»', { nome: info.nome }))
        btn.disabled = false
        btn.classList.remove('is-busy')
        btn.textContent = textoOriginal
      }
    } catch (err) {
      setStatus(t('latexPackage.failed', 'Non se puido instalar «{nome}»', { nome: info.nome }))
      btn.disabled = false
      btn.classList.remove('is-busy')
      btn.textContent = textoOriginal
      showWarnings([mensaxeErro(err)])
    }
  })
  fila.appendChild(btn)
  warningsEl.appendChild(fila)
}

// makeBugReportButton: "🐛 Informar deste erro" (ver bug-report.js) - só
// xera un INFORME (Markdown) para o desenvolvedor, nunca toca nin
// recompila código de Yang (ver bugreport.go/systemPromptInformeErro,
// ia_client.go). Manda o documento ENTEIRO coma contexto (syncToText()
// primeiro, mesmo criterio ca btnAssistantCode) - a IA xa sabe illar o
// anaco culpable, e así funciona igual en modo Código ou Bloques.
function makeBugReportButton(list) {
  const btn = document.createElement('button')
  btn.type = 'button'
  btn.className = 'warnings-bugreport-btn'
  btn.innerHTML = '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="6" width="8" height="9" rx="3"/><path d="M8 6V4.5a2 2 0 0 1 4 0V6M4 9h2M14 9h2M4.5 12.5 6 13M13.5 12.5 14 13M6 15.5v1M14 15.5v1"/></svg><span></span>'
  btn.querySelector('span').textContent = t('bugreport.button', 'Informar deste erro')
  btn.addEventListener('click', () => {
    syncToText()
    openBugReportModal({
      t,
      anaco: editor.getValue(),
      mensaxeErro: list.join('\n'),
      generateReport: generateBugReport,
      saveReport: saveBugReport,
    })
  })
  return btn
}

// generateBugReport/saveBugReport: única ponte entre bug-report.js e
// XerarInformeErroIA/GardarInformeErroDialog - mesmo criterio ca
// generateWithAI/uploadImageFile para o editor de bloques.
async function generateBugReport(anaco, mensaxeErro) {
  return XerarInformeErroIA({ anaco, mensaxeErro })
}
async function saveBugReport(defaultName, content) {
  return GardarInformeErroDialog(defaultName, content)
}

// pagesToPreviewHTML wraps the PDF's page images (one PNG per page, see
// GeneratePDFResult.PageImages / pdfPageImages in latexdoc.go) in a small
// scrollable HTML document for the preview iframe - it's what the webview
// actually renders, since WebKitGTK (Wails' engine on Linux) has no
// reliable native PDF viewer to point an <iframe>/<embed> at directly.
// Ctrl+clic nunha páxina -> "reverse search" (ver ReverseSearch en Go,
// reversesearch.go): a páxina é unha imaxe PLANA (PNG), sen texto nin
// nós - o único xeito de saber "que anaco a xerou" é medir o clic en
// píxeles relativos ao tamaño NATURAL da imaxe (non ao tamaño en pantalla,
// que pode estar reducido por max-width:100%) e mandarllo ao pai por
// postMessage; run() (main.js) escoita esa mensaxe e chama a ReverseSearch.
const REVERSE_SEARCH_SCRIPT = `
<script>
document.addEventListener('click', (e) => {
  if (!e.ctrlKey) return
  const img = e.target.closest('img.pdf-page')
  if (!img) return
  const rect = img.getBoundingClientRect()
  const scaleX = img.naturalWidth / rect.width
  const scaleY = img.naturalHeight / rect.height
  parent.postMessage({
    matexeReverseSearch: true,
    page: parseInt(img.dataset.page, 10),
    xPx: (e.clientX - rect.left) * scaleX,
    yPx: (e.clientY - rect.top) * scaleY,
  }, '*')
  e.preventDefault()
})
</script>`

// zoomScript: Ctrl+/Ctrl-/Ctrl+0 para as páxinas dentro do iframe de
// previsualización (e da xanela de resultado desacoplada, que reusa este
// mesmo HTML - ver entrarModoResultado). .pdf-page xa se axusta por defecto
// ao ancho dispoñible (max-width:100%), pero nun panel/xanela pequena iso
// pode quedar demasiado pequeno para ler - real report: "queda bastante
// pequeno, hai que forzar a vista". Ctrl+0 volve a ese axuste automático; o
// nivel avísase ao pai (handleZoomMessage) para sobrevivir a un "Xerar".
function zoomScript(initialZoom) {
  return `
<script>
let zoom = ${initialZoom}
function aplicarZoom() {
  document.querySelectorAll('.pdf-page').forEach((img) => {
    if (zoom === 1) { img.style.maxWidth = '100%'; img.style.width = ''; return }
    const disponible = document.body.clientWidth - 32 // 2x padding:1rem do body
    const axuste = Math.min(img.naturalWidth || disponible, disponible)
    img.style.maxWidth = 'none'
    img.style.width = Math.round(axuste * zoom) + 'px'
  })
}
document.addEventListener('keydown', (e) => {
  if (!(e.ctrlKey || e.metaKey)) return
  let novoZoom = zoom
  if (e.key === '+' || e.key === '=') novoZoom = Math.min(zoom * 1.2, 4)
  else if (e.key === '-' || e.key === '_') novoZoom = Math.max(zoom / 1.2, 0.3)
  else if (e.key === '0') novoZoom = 1
  else return
  e.preventDefault()
  zoom = novoZoom
  aplicarZoom()
  parent.postMessage({ matexeSetZoom: zoom }, '*')
})
window.addEventListener('load', aplicarZoom)
</script>`
}

// pagesToPreviewHTML wraps the PDF's page images (one PNG per page, see
// GeneratePDFResult.PageImages / pdfPageImages in latexdoc.go) in a small
// scrollable HTML document for the preview iframe - it's what the webview
// actually renders, since WebKitGTK (Wails' engine on Linux) has no
// reliable native PDF viewer to point an <iframe>/<embed> at directly.
function pagesToPreviewHTML(pageImages, zoom = previewZoom) {
  if (!pageImages || pageImages.length === 0) {
    // Mensaxe neutra a propósito: isto amósase tamén antes de xerar nada
    // (ex. a xanela de resultado en "Modo de traballo: Dúas xanelas" ábrese
    // xa dende o principio, sen nada aínda) - non se pode saber aquí se é
    // por iso ou por faltar pdftoppm de verdade. O aviso específico de
    // pdftoppm ausente (latexdoc.go) xa chega por separado, no panel de
    // avisos (showWarnings) despois de Xerar - non se perde información.
    return `<!doctype html><html><body><p style="font:14px sans-serif;color:#666;padding:1rem">${t('preview.noPages', 'Aínda non hai nada que amosar. Preme "Xerar" para compilar o documento.')}</p></body></html>`
  }
  // data-page 1-based, coma o espera synctex (e pdfPageImages en Go).
  const pages = pageImages.map((src, i) => `<img src="${src}" class="pdf-page" data-page="${i + 1}">`).join('\n')
  return `<!doctype html><html><head><meta charset="utf-8">
<style>
  body{margin:0;padding:1rem;background:#787878;display:flex;flex-direction:column;align-items:center;gap:1rem}
  .pdf-page{max-width:100%;box-shadow:0 2px 8px rgba(0,0,0,.35);background:#fff}
  @media print{ body{background:#fff;padding:0;gap:0} .pdf-page{box-shadow:none;max-width:100%;page-break-after:always} }
</style>
</head><body>
${pages}
${REVERSE_SEARCH_SCRIPT}
${zoomScript(zoom)}
</body></html>`
}

// publicarEstadoResultado (só na xanela EDITOR, modo dúas xanelas): envía
// o que run()/generatePDF() acaban de xerar ao proceso Go (Publica
// EstadoResultado, xanelaresultado.go), que o garda para atender a xanela
// de resultado (/estado para as páxinas, /accion/pdf|tex para gardalas sen
// recompilar, /accion/md|docx|odt para recompilalas cando llas pidan - ver
// accionRemota). Chamada barata e inofensiva aínda sen modo dúas xanelas
// activo (non hai ninguén escoitando).
function publicarEstadoResultado(result) {
  const nomeBase = nomeDocumento()
  PublicarEstadoResultado({
    source: editor.getValue(),
    codeIni: '',
    modo: modoActivo(),
    seed: parseInt(seedInput.value, 10) || 0,
    iterations: parseInt(iterInput.value, 10) || 1,
    baseDir: dirOf(currentPath),
    nomeDoc: nomeDocumento(),
  }, nomeBase, result.pageImages || [], result.warnings || [], result.pdfBase64 || '', result.latexSource || '')
}

// run compila o documento con Maxima + pdflatex (mesmo motor real que o
// PDF final, non a aproximación HTML+MathJax que usaba a versión anterior
// deste editor) e amosa cada páxina como imaxe na previsualización - así o
// que se ve é exactamente o que se vai imprimir/entregar.
// run compila e previsualiza. Sen argumentos (botón "Xerar", atallos, tras
// gardar…) compila o DOCUMENTO ENTEIRO. Se se lle pasa unha string
// (sourceOverride), compila exactamente ese .matex - úsao o botón ✓ da táboa
// para previsualizar UN só exercicio sen tocar o resto. O gardián `typeof
// === 'string'` tamén cobre o caso de `run` rexistrado como manexador de
// clic (recibe un PointerEvent, non unha string).
async function run(sourceOverride) {
  const soltoSource = typeof sourceOverride === 'string' ? sourceOverride : null
  if (!soltoSource) syncToText()
  btnRun.disabled = true
  setStatus(t('status.compilingPdf', 'Compilando PDF (Maxima + pdflatex)...'))
  showWarnings([])
  log('[XERAR] Compilando PDF (Maxima + pdflatex)...')
  try {
    const result = await GeneratePDF({
      source: soltoSource || editor.getValue(),
      codeIni: '',
      modo: modoActivo(),
      tipografia: tipografiaActual(),
      seed: parseInt(seedInput.value, 10) || 0,
      iterations: parseInt(iterInput.value, 10) || 1,
      baseDir: dirOf(currentPath),
      nomeDoc: nomeDocumento(),
    })
    // Nunha previsualización de UN só exercicio (botón ✓ da táboa) só se
    // actualiza o panel da dereita: NON se pisan lastPDFBase64/lastLatexSource
    // - así imprimir e exportar .tex seguen a usar a última xeración COMPLETA,
    // non ese exercicio solto.
    if (!soltoSource) {
      lastPDFBase64 = result.pdfBase64
      lastLatexSource = result.latexSource
      publicarEstadoResultado(result)
    }
    preview.srcdoc = pagesToPreviewHTML(result.pageImages)
    showWarnings(result.warnings)
    logPdflatex(result.log)
    setStatus(t('status.done', 'Feito.'))
    log('[XERAR] Feito.')
    return true
  } catch (err) {
    setStatus(t('status.error', 'Erro'))
    showWarnings([mensaxeErro(err)])
    log('[XERAR] ERRO: ' + err)
    checkMaxima()
    checkLatex()
    return false
  } finally {
    btnRun.disabled = false
  }
}

// xerarConAutoreparacion: o que se chama tras xerar un exame/exercicio coa
// IA (onGenerateResult da táboa). Compila (run()) e, se falla E Opcións o
// permite, illa os exercicios que non compilan e pídelle á IA que os
// rexenere SÓ eses (ata Settings.maxRechamadasIA veces), sen tocar o resto.
// Ver frontend/src/auto-reparar.js e autoreparar.go.
async function xerarConAutoreparacion() {
  const ok = await run()
  if (ok !== false || editMode !== 'table') return
  if (typeof DiagnosticarCompilacionExercicios !== 'function' || typeof RexenerarExercicioTaboa !== 'function') return

  let s
  try { s = await GetSettings() } catch { s = {} }
  if (s.autorepararExercicios === false) return
  const maxRechamadas = s.maxRechamadasIA || 2

  setStatus(t('status.autoreparando', 'Autoreparando os exercicios que non compilan…'))
  log('[AUTOREPARAR] A compilación fallou; illando os exercicios que fallan…')
  try {
    const r = await autorepararExame(editor.getValue(), {
      maxRechamadas,
      diagnosticar: (source) => DiagnosticarCompilacionExercicios({
        source,
        codeIni: '',
        modo: modoActivo(),
        tipografia: tipografiaActual(),
        seed: parseInt(seedInput.value, 10) || 0,
        iterations: 1,
        baseDir: dirOf(currentPath),
        nomeDoc: nomeDocumento(),
      }),
      rexenerar: (req) => RexenerarExercicioTaboa(req),
      onProgreso: (inf) => log(
        `[AUTOREPARAR] Exercicio ${inf.indice + 1}: ${inf.estado}` + (inf.erro ? ' — ' + inf.erro : ''),
      ),
    })
    if (r.cambiou && r.texto) {
      editor.setValue(r.texto)
      await run()
    }
    const reconto = (r.informe || []).reduce((a, i) => { a[i.estado] = (a[i.estado] || 0) + 1; return a }, {})
    const falla = reconto.falla || 0
    const reparado = reconto.reparado || 0
    setStatus(falla
      ? t('status.autoreparadoParcial', 'Autoreparación: {r} arranxado(s), {n} seguen sen compilar (marcados no exame).', { r: reparado, n: falla })
      : reparado
        ? t('status.autoreparadoOk', 'Autoreparación: {r} exercicio(s) arranxado(s), todo compila.', { r: reparado })
        : t('status.done', 'Feito.'))
  } catch (err) {
    log('[AUTOREPARAR] ERRO: ' + err)
    setStatus(t('status.error', 'Erro'))
  }
}

// logPdflatex vai ao panel de rexistro a saída de pdflatex/Maxima (campo
// `log` de GeneratePDFResult, latexdoc.go) cando hai algunha - liña a liña,
// co mesmo prefixo cós demais eventos do panel.
function logPdflatex(texto) {
  if (!texto) return
  for (const liña of texto.split('\n')) {
    if (liña.trim()) log('[PDFLATEX] ' + liña)
  }
}

// killMaxima recovers from a Maxima session that got stuck (an expression
// that never returns) MANUALLY, before the automatic per-call timeout
// (Opcións → "Tempo máximo de espera", cas.Session.SetTimeout) has a chance
// to fire on its own - it kills whatever Maxima process is currently
// running, which makes that in-flight call fail right away instead of
// waiting out the full timeout, so btnRun goes back to usable sooner.
async function killMaxima() {
  // Feedback inline dentro de Opcións: o botón vive agora aí, e a barra de
  // estado queda tapada polo overlay da modal (killMaximaStatus está xunto
  // ao botón). Se se chama fóra de Opcións (p.ex. dende un atallo futuro), o
  // elemento simplemente non existe e queda todo en setStatus/log.
  const inline = document.getElementById('killMaximaStatus')
  if (inline) inline.textContent = '…'
  try {
    await KillMaxima()
    const msg = t('status.maximaKilled', 'Sesión de Maxima interrompida. Volve premer Xerar.')
    setStatus(msg)
    if (inline) inline.textContent = t('options.killMaxima.done', 'Feito ✓')
    log('[XERAR] Sesión de Maxima interrompida.')
  } catch (err) {
    setStatus(mensaxeErro(err))
    if (inline) inline.textContent = mensaxeErro(err)
    log('[XERAR] ERRO ao reiniciar Maxima: ' + err)
  }
}

// newFile: limpa a pantalla (texto e bloques) para empezar un exame/práctica
// en branco, coma se se acabase de abrir Yang. Pregunta antes se hai
// contido sen gardar, para non perder traballo por un clic despistado.
async function newFile() {
  syncToText()
  const hasContent = editor.getValue().trim() !== ''
  if (hasContent && !await confirmar(t('confirm.discardChanges', 'Vanse perder os cambios non gardados. Continuar?'))) return
  editor.setValue('')
  if (blocksEditor) blocksEditor.setValue('')
  if (tableEditor) tableEditor.setValue('')
  currentPath = null
  lastPDFBase64 = null
  lastLatexSource = null
  preview.srcdoc = ''
  showWarnings([])
  setStatus(t('status.unsaved', 'sen gardar'))
}

async function openFile() {
  try {
    const res = await OpenFileDialog()
    if (!res) return
    editor.setValue(res.content)
    if (blocksEditor) blocksEditor.setValue(res.content)
    if (tableEditor) tableEditor.setValue(res.content)
    currentPath = res.path
    setStatus(currentPath)
    await sincronizarPlantillaDoDocumento()
    // Compila (e, se falla en modo táboa, autorrepara) xa ao abrir: así un
    // .matex gardado con un erro de compilación (p.ex. dunha lapela "+
    // resolución" que nunca se chegou a premer) queda amosado/arranxado sen
    // ter que premer "Xerar" á man. Ver xerarConAutoreparacion.
    await xerarConAutoreparacion()
  } catch (err) {
    setStatus(t('status.errorOpening', 'Erro ao abrir'))
    showWarnings([mensaxeErro(err)])
  }
}

// openFileFromExplorer: camiño alternativo a openFile() para un .matex
// premido no explorador de cartafoles (ver explorer.js, onOpenFile) - non
// hai selector nativo que abrir (xa se sabe `path`), pero o resto do
// contrato é o mesmo: preguntar por cambios sen gardar (mesmo guard ca
// newFile()), ler o ficheiro (LerFicheiroTexto) e cargalo nos dous
// editores.
async function openFileFromExplorer(path) {
  syncToText()
  const hasContent = editor.getValue().trim() !== ''
  if (hasContent && !await confirmar(t('confirm.discardChanges', 'Vanse perder os cambios non gardados. Continuar?'))) return
  try {
    const content = await LerFicheiroTexto(path)
    editor.setValue(content)
    if (blocksEditor) blocksEditor.setValue(content)
    if (tableEditor) tableEditor.setValue(content)
    currentPath = path
    setStatus(currentPath)
    await sincronizarPlantillaDoDocumento()
    // Mesmo motivo ca en openFile(): compilar (e autorreparar se falla) xa
    // ao abrir dende o explorador, non só cando se preme "Xerar" ou lapela.
    await xerarConAutoreparacion()
  } catch (err) {
    setStatus(t('status.errorOpening', 'Erro ao abrir'))
    showWarnings([mensaxeErro(err)])
  }
}

// iniciarExplorer crea o explorador de cartafoles unha soa vez, coñecido xa
// Settings.ultimoCartafol (chamada dende o arranque asíncrono de idioma,
// máis abaixo - Settings só se sabe despois de GetSettings()). Se hai
// cartafol lembrado, o sidebar arrinca xa visible (coma VS Code reabrindo o
// último cartafol de traballo); se non, queda pechado ata que se prema
// "Explorador" na toolbar.
function iniciarExplorer(ultimoCartafol) {
  explorer = createExplorer(explorerSidebar, {
    initialRoot: ultimoCartafol,
    listDir: ListarCartafol,
    chooseFolder: EscollerCartafolDialog,
    newFolder: NovoCartafol,
    rename: RenomearFicheiro,
    remove: EliminarFicheiro,
    uploadFile: uploadFileToExplorerFolder,
    onOpenFile: openFileFromExplorer,
    onRootChanged: (path) => {
      setExplorerVisible(true)
      GetSettings().then((s) => SaveSettings({ ...s, ultimoCartafol: path })).catch(() => {})
    },
    t,
  })
  if (ultimoCartafol) setExplorerVisible(true)
}

function setExplorerVisible(visible) {
  explorerSidebar.classList.toggle('hidden', !visible)
  btnToggleExplorer.setAttribute('aria-pressed', String(visible))
}

btnToggleExplorer.addEventListener('click', () => {
  setExplorerVisible(explorerSidebar.classList.contains('hidden'))
})

// gardadoOk chama run() despois dun gardado con éxito, se Opcións → "Xerar
// automaticamente ao gardar" está activo (xerarAoGardarActivo) - así o
// aviso "Gardado: X" queda substituído polo propio resultado de run()
// (Compilando.../Feito.), que é máis relevante nese intre.
function gardadoOk() {
  if (xerarAoGardarActivo) run()
}

async function saveFile() {
  syncToText()
  try {
    if (currentPath) {
      await SaveFile(currentPath, editor.getValue())
      setStatus(t('status.saved', 'Gardado: ') + currentPath)
      gardadoOk()
    } else {
      const path = await SaveFileDialog('sen-nome.matex', editor.getValue())
      if (path) {
        currentPath = path
        setStatus(t('status.saved', 'Gardado: ') + currentPath)
        gardadoOk()
      }
    }
  } catch (err) {
    setStatus(t('status.errorSaving', 'Erro ao gardar'))
    showWarnings([mensaxeErro(err)])
  }
}

// saveFileAs abre sempre o selector para escoller nome/lugar, aínda que xa
// haxa un ficheiro aberto (a diferenza de saveFile/Ctrl+S, que unha vez hai
// currentPath sobrescribe ese mesmo ficheiro en silencio, sen preguntar -
// comportamento normal de "Gardar", pero sen isto non habería xeito de
// gardar unha copia con outro nome/lugar). Pásaselle a currentPath completa
// (non só o nome): SaveFileDialog xa reparte ela soa suxestión de nome e
// directorio de partida (splitDialogDefault, app.go), así o selector abre
// por defecto na mesma carpeta do ficheiro aberto, non nunha ao chou.
async function saveFileAs() {
  syncToText()
  try {
    const path = await SaveFileDialog(currentPath || 'sen-nome.matex', editor.getValue())
    if (path) {
      currentPath = path
      setStatus(t('status.saved', 'Gardado: ') + currentPath)
      // "Gardar como" estrea unha ruta: déixase anotado que ESE ficheiro
      // usa a plantilla que está activa agora (ver EscollerPlantilla).
      await lembrarPlantillaDoDocumento()
      gardadoOk()
    }
  } catch (err) {
    setStatus(t('status.errorSaving', 'Erro ao gardar'))
    showWarnings([mensaxeErro(err)])
  }
}

function printPreview() {
  if (!lastPDFBase64) {
    setStatus(t('status.generateBeforePrint', 'Xera antes de imprimir'))
    return
  }
  preview.contentWindow.focus()
  preview.contentWindow.print()
}

// accionRemota (só na xanela de RESULTADO, modo dúas xanelas): chama
// directamente un dos bindings AccionResultado{PDF,Tex,MD,Docx,Odt} (Go,
// xanelaresultado.go) - esta xanela non ten editor propio, así que non
// pode compilar nada ela soa; o proceso Go executa a MESMA lóxica que xa
// usan eses botóns na xanela única (GeneratePDF/ExportMarkdown/...) e
// devolve {path, warnings}, ou rexeita a promesa se hai erro.
async function accionRemota(accionFn, btn, statusCompiling, errKey, errFallback) {
  btn.disabled = true
  setStatus(statusCompiling)
  showWarnings([])
  log('[XERAR] ' + statusCompiling)
  try {
    const result = await accionFn()
    showWarnings(result.warnings || [])
    setStatusGardado(result.path ? t('status.exported', 'Exportado: ') + result.path : t('status.unsaved', 'sen gardar'), result.path)
    log('[XERAR] ' + (result.path ? 'Exportado: ' + result.path : 'Feito.'))
  } catch (err) {
    setStatus(t(errKey, errFallback))
    showWarnings([mensaxeErro(err)])
    log('[XERAR] ERRO: ' + err)
  } finally {
    btn.disabled = false
  }
}

// exportTex garda o código .tex xa xerado (o que xa se compilou en
// run()/generatePDF(), sen recompilar de novo).
async function exportTex() {
  if (souXanelaResultado) {
    return accionRemota(AccionResultadoTex, btnExportTex, t('status.exporting', 'Exportando...'),
      'status.errorExporting', 'Erro ao exportar')
  }
  if (!lastLatexSource) {
    setStatus(t('status.generateBeforeExport', 'Xera antes de exportar'))
    return
  }
  try {
    const base = currentPath ? currentPath.replace(/\.matex$/i, '') + '.tex' : 'exame.tex'
    const path = await SaveTexDialog(base, lastLatexSource)
    if (path) {
      setStatusGardado(t('status.exported', 'Exportado: ') + path, path)
      log('[XERAR] Exportado: ' + path)
    }
  } catch (err) {
    setStatus(t('status.errorExporting', 'Erro ao exportar'))
    showWarnings([mensaxeErro(err)])
    log('[XERAR] ERRO: ' + err)
  }
}

// exportMarkdown fai todo dun golpe (a diferenza de exportTex, que só garda
// o que xa había): abre o diálogo "Gardar como" e, se non se cancela,
// recompila con Maxima en modo Markdown e escribe o .md máis o seu cartafol
// images/ (gráficos/TikZ/imaxes subidas) directamente na ruta escollida -
// non hai preview Markdown na UI que se poida reutilizar. exportDocx/
// exportOdt (abaixo) seguen o mesmo esquema, pero pasando o resultado por
// Pandoc no canto de escribilo tal cal.
async function exportMarkdown() {
  if (souXanelaResultado) {
    return accionRemota(AccionResultadoMD, btnExportMarkdown, t('status.compilingMarkdown', 'Compilando Markdown...'),
      'status.errorExportingMarkdown', 'Erro ao exportar Markdown')
  }
  syncToText()
  btnExportMarkdown.disabled = true
  setStatus(t('status.compilingMarkdown', 'Compilando Markdown...'))
  showWarnings([])
  log('[XERAR] Compilando Markdown...')
  try {
    const base = currentPath ? currentPath.replace(/\.matex$/i, '') + '.md' : 'exame.md'
    const result = await ExportMarkdown({
      source: editor.getValue(),
      codeIni: '',
      modo: modoActivo(),
      seed: parseInt(seedInput.value, 10) || 0,
      iterations: parseInt(iterInput.value, 10) || 1,
      baseDir: dirOf(currentPath),
      nomeDoc: nomeDocumento(),
    }, base)
    showWarnings(result.warnings)
    setStatusGardado(result.path ? t('status.exported', 'Exportado: ') + result.path : t('status.unsaved', 'sen gardar'), result.path)
    log('[XERAR] ' + (result.path ? 'Exportado: ' + result.path : 'Feito.'))
  } catch (err) {
    setStatus(t('status.errorExportingMarkdown', 'Erro ao exportar Markdown'))
    showWarnings([mensaxeErro(err)])
    log('[XERAR] ERRO: ' + err)
  } finally {
    btnExportMarkdown.disabled = false
  }
}

// inicializarEnvioATao amosa un botón extra "Enviar a Tao" só cando Yang foi
// aberto por Tao para editar unha tarefa de sesión (ver
// tao/yang_launch.go, contexto_tao.go, tarefa_tao.go) - abrindo Yang desde
// o mapa de Piztu ou á man, TaoContextoActivo() devolve false e este botón
// nin se crea. A diferenza de exportMarkdown, non hai diálogo "gardar
// como": xera o Markdown e empúxao decontado á pestana "Tarefa" de Tao
// (EnviarATarefaTao, Go) para que se actualice ao instante.
// alumnoIndividual (username, non baleiro) cando Tao abriu Yang para editar
// SÓ a tarefa dun alumno concreto (App.tsx, "Editar con Yang" dentro do
// editor dun alumno) - ver inicializarEnvioATao/inicializarReparto, que o
// len de TaoAlumnoIndividual() cambian de comportamento.
let alumnoIndividual = ''

// inicializarEnvioATao amosa un botón extra "Enviar a Tao" só cando Yang foi
// aberto por Tao para editar unha tarefa de sesión (ver
// tao/yang_launch.go, contexto_tao.go, tarefa_tao.go) - abrindo Yang desde
// o mapa de Piztu ou á man, TaoContextoActivo() devolve false e este botón
// nin se crea. A diferenza de exportMarkdown, non hai diálogo "gardar
// como": xera o Markdown e empúxao decontado á pestana "Tarefa" de Tao
// (EnviarATarefaTao, Go) para que se actualice ao instante.
//
// Se ademais Tao pediu editar UN alumno en concreto (TaoAlumnoIndividual),
// o botón troca a "Gardar para <nome>" e envía con EnviarATarefaTaoAlumno,
// nunca á tarefa xeral - engádese tamén un aviso fixo para que non se
// confunda cos dous casos.
async function inicializarEnvioATao() {
  // A xanela de resultado non ten editor propio (ver souXanelaResultado
  // arriba) - este botón depende de editor.getValue(), así que só ten
  // sentido na xanela principal. Pendente se hai que estender o modo dúas
  // xanelas con isto tamén (mesmo patrón que accionRemota).
  if (souXanelaResultado) return

  let activo = false
  try {
    activo = await TaoContextoActivo()
  } catch (err) {
    return
  }
  if (!activo) return

  try {
    alumnoIndividual = (await TaoAlumnoIndividual()) || ''
  } catch (err) {
    alumnoIndividual = ''
  }

  const btn = document.createElement('button')
  btn.id = 'btnEnviarTao'
  btnExportMarkdown.insertAdjacentElement('afterend', btn)

  if (alumnoIndividual) {
    let nome = alumnoIndividual
    try {
      const alumnos = (await TaoAlumnos()) || []
      const atopado = alumnos.find((a) => a.username === alumnoIndividual)
      if (atopado && atopado.nome) nome = atopado.nome
    } catch (err) {
      // segue co username coma nome
    }

    const aviso = document.createElement('span')
    aviso.id = 'avisoAlumnoIndividual'
    aviso.className = 'aviso-alumno-individual'
    aviso.textContent = '🧑‍🎓 ' + t('status.editandoIndividual', 'Editando a tarefa individual de: {{nome}}', { nome })
    btn.insertAdjacentElement('beforebegin', aviso)

    btn.textContent = '📤 ' + t('status.sendToTaoAlumno', 'Gardar para {{nome}}', { nome })
    btn.title = t('status.sendToTaoAlumnoTitle', 'Xerar Markdown e actualizar SÓ a tarefa individual de {{nome}} en Tao ao instante', { nome })
  } else {
    btn.textContent = '📤 ' + t('status.sendToTao', 'Enviar a Tao')
    btn.title = t('status.sendToTaoTitle', 'Xerar Markdown e actualizar a tarefa da sesión en Tao ao instante')
  }

  btn.addEventListener('click', async () => {
    syncToText()
    btn.disabled = true
    setStatus(t('status.sendingToTao', 'Enviando a Tao...'))
    showWarnings([])
    log('[XERAR] Enviando a Tao...')
    try {
      const req = {
        source: editor.getValue(),
        codeIni: '',
        seed: parseInt(seedInput.value, 10) || 0,
        iterations: parseInt(iterInput.value, 10) || 1,
        baseDir: '',
      }
      const result = alumnoIndividual ? await EnviarATarefaTaoAlumno(req) : await EnviarATarefaTao(req)
      showWarnings(result.warnings || [])
      setStatus(t('status.sentToTao', 'Enviado a Tao'))
      log('[XERAR] Enviado a Tao.')
    } catch (err) {
      setStatus(t('status.errorSendingToTao', 'Erro ao enviar a Tao'))
      showWarnings([mensaxeErro(err)])
      log('[XERAR] ERRO: ' + err)
    } finally {
      btn.disabled = false
    }
  })
}

// inicializarReparto amosa un botón extra "🎓 Repartir" a carón de "Enviar a
// Tao" cando, ademais de TaoContextoActivo, Tao mandou un alumnado non baleiro
// (TaoAlumnos, ver tao/yang_launch.go alumnosDaMateria) - docs/api-tao.md
// §11.2/§11.5. Sen alumnado (materia sen grupos, roster illegible...) este
// botón simplemente non se crea: só "Enviar a Tao" (a tarefa xeral) segue
// dispoñible, xeración normal, sen reparto. Tampouco se crea en modo
// alumnoIndividual (chámase despois de inicializarEnvioATao, que xa o
// resolveu) - repartir non ten sentido cando xa se está a editar un só.
async function inicializarReparto() {
  if (souXanelaResultado) return
  if (alumnoIndividual) return

  let activo = false
  try {
    activo = await TaoContextoActivo()
  } catch (err) {
    return
  }
  if (!activo) return

  let alumnos = []
  try {
    alumnos = (await TaoAlumnos()) || []
  } catch (err) {
    alumnos = []
  }
  if (alumnos.length === 0) return

  // Suxire Iteracións = nº de alumnos, só se o campo segue no valor por
  // defecto (non sobrescribe unha elección xa feita polo profesorado).
  if (iterInput.value === '' || iterInput.value === '1') {
    iterInput.value = String(alumnos.length)
  }

  const repartoAlumnos = alumnos.map((a) => ({ username: a.username, nome: a.nome || a.username, incluido: true, fallou: false }))

  const anchor = document.getElementById('btnEnviarTao') || btnExportMarkdown
  const btn = document.createElement('button')
  btn.id = 'btnRepartir'
  btn.textContent = '🎓 ' + t('reparto.abrirBoton', 'Repartir')
  btn.title = t('reparto.abrirBotonTitle', 'Xerar unha variante distinta para cada alumno e envialas coma tarefa individual')
  anchor.insertAdjacentElement('afterend', btn)

  const overlay = document.getElementById('repartoOverlay')
  const lista = document.getElementById('repartoLista')
  const aviso = document.getElementById('repartoAviso')
  const btnCancelar = document.getElementById('btnRepartirCancelar')
  const btnEnviar = document.getElementById('btnRepartirEnviar')

  function renderRepartoLista() {
    lista.innerHTML = ''
    repartoAlumnos.forEach((al, idx) => {
      const fila = document.createElement('div')
      fila.className = 'reparto-fila' + (al.incluido ? '' : ' reparto-excluido') + (al.fallou ? ' reparto-fallado' : '')

      const check = document.createElement('input')
      check.type = 'checkbox'
      check.checked = al.incluido
      check.addEventListener('change', () => {
        al.incluido = check.checked
        renderRepartoLista()
      })

      const nome = document.createElement('span')
      nome.className = 'reparto-nome'
      nome.textContent = (idx + 1) + '. ' + al.nome

      const up = document.createElement('button')
      up.type = 'button'
      up.textContent = '▲'
      up.disabled = idx === 0
      up.addEventListener('click', () => {
        ;[repartoAlumnos[idx - 1], repartoAlumnos[idx]] = [repartoAlumnos[idx], repartoAlumnos[idx - 1]]
        renderRepartoLista()
      })

      const down = document.createElement('button')
      down.type = 'button'
      down.textContent = '▼'
      down.disabled = idx === repartoAlumnos.length - 1
      down.addEventListener('click', () => {
        ;[repartoAlumnos[idx + 1], repartoAlumnos[idx]] = [repartoAlumnos[idx], repartoAlumnos[idx + 1]]
        renderRepartoLista()
      })

      fila.append(check, nome, up, down)
      lista.appendChild(fila)
    })
  }

  btn.addEventListener('click', () => {
    aviso.classList.add('hidden')
    renderRepartoLista()
    overlay.classList.remove('hidden')
  })
  btnCancelar.addEventListener('click', () => overlay.classList.add('hidden'))

  btnEnviar.addEventListener('click', async () => {
    const seleccionados = repartoAlumnos.filter((a) => a.incluido)
    if (seleccionados.length === 0) {
      aviso.textContent = t('reparto.senSeleccion', 'Selecciona polo menos un alumno.')
      aviso.classList.remove('hidden')
      return
    }

    // Desaxuste entre o que se xerou/revisou (Iteracións) e a selección
    // real: bloquea con confirmación explícita en vez dun aviso ignorable
    // (docs/api-tao.md §11.5) - evita mandar tarefas a medias por un
    // desconto mal feito.
    const iterations = parseInt(iterInput.value, 10) || 1
    if (iterations !== seleccionados.length) {
      const baseSeed = parseInt(seedInput.value, 10) || 0
      const msg = t('reparto.desaxusteConfirm',
        'Xeraches {iter} variante(s) pero seleccionaches {sel} alumno(s). ¿Repartir igualmente (unha variante por alumno seleccionado, sementes {seed}..{seedFin})?',
        { iter: iterations, sel: seleccionados.length, seed: baseSeed, seedFin: baseSeed + seleccionados.length - 1 })
      if (!await confirmar(msg)) return
    }

    syncToText()
    btnEnviar.disabled = true
    aviso.classList.add('hidden')
    setStatus(t('status.sendingReparto', 'Repartindo variantes...'))
    showWarnings([])
    log('[REPARTO] Repartindo a ' + seleccionados.length + ' alumno(s)...')
    try {
      const usernames = seleccionados.map((a) => a.username)
      const result = await EnviarReparto({
        source: editor.getValue(),
        codeIni: '',
        seed: parseInt(seedInput.value, 10) || 0,
        iterations: 1,
        baseDir: '',
      }, usernames)
      showWarnings(result.warnings || [])

      const resultados = result.resultados || []
      resultados.forEach((r) => {
        const al = repartoAlumnos.find((a) => a.username === r.username)
        if (al) al.fallou = !r.ok
      })
      const fallados = resultados.filter((r) => !r.ok)

      if (fallados.length === 0) {
        setStatus(t('status.repartoOk', 'Repartido a {n} alumno(s).', { n: resultados.length }))
        log('[REPARTO] Repartido a ' + resultados.length + ' alumno(s).')
        overlay.classList.add('hidden')
      } else {
        // Deixa marcados só os que fallaron, listos para reintentar
        // premendo "Repartir" outra vez sen tocar os que xa chegaron
        // (docs/api-tao.md §11.6, fallo parcial).
        repartoAlumnos.forEach((a) => { a.incluido = a.fallou })
        renderRepartoLista()
        const listaFallo = fallados.map((f) => f.username).join(', ')
        aviso.textContent = t('reparto.parcial',
          '{ok}/{total} enviados. Fallaron: {lista}. Quedan marcados para reintentar.',
          { ok: resultados.length - fallados.length, total: resultados.length, lista: listaFallo })
        aviso.classList.remove('hidden')
        setStatus(t('status.repartoParcial', 'Reparto parcial — mira os detalles'))
        log('[REPARTO] ERRO parcial: ' + listaFallo)
      }
    } catch (err) {
      setStatus(t('status.errorReparto', 'Erro ao repartir'))
      showWarnings([mensaxeErro(err)])
      log('[REPARTO] ERRO: ' + err)
    } finally {
      btnEnviar.disabled = false
    }
  })
}

// exportDocx/exportOdt seguen o mesmo criterio que exportMarkdown: todo dun
// golpe (diálogo "Gardar como" + recompilación en Maxima), sen preview
// propia na UI. A diferenza do .md, o corpo pasa por Pandoc no backend
// (docdoc.go) cunha plantilla de estilos "estilo exame" antes de escribirse
// - o .docx/.odt resultante xa vén listo para imprimir, coas imaxes
// incrustadas dentro do propio ficheiro.
async function exportDocOffice(accionRemotaFn, exportFn, btn, statusCompiling, ext, errKey, errFallback) {
  if (souXanelaResultado) {
    return accionRemota(accionRemotaFn, btn, statusCompiling, errKey, errFallback)
  }
  syncToText()
  btn.disabled = true
  setStatus(statusCompiling)
  showWarnings([])
  log('[XERAR] ' + statusCompiling)
  try {
    const base = currentPath ? currentPath.replace(/\.matex$/i, '') + ext : 'exame' + ext
    const result = await exportFn({
      source: editor.getValue(),
      codeIni: '',
      modo: modoActivo(),
      seed: parseInt(seedInput.value, 10) || 0,
      iterations: parseInt(iterInput.value, 10) || 1,
      baseDir: dirOf(currentPath),
      nomeDoc: nomeDocumento(),
    }, base)
    showWarnings(result.warnings)
    setStatusGardado(result.path ? t('status.exported', 'Exportado: ') + result.path : t('status.unsaved', 'sen gardar'), result.path)
    log('[XERAR] ' + (result.path ? 'Exportado: ' + result.path : 'Feito.'))
  } catch (err) {
    setStatus(t(errKey, errFallback))
    showWarnings([mensaxeErro(err)])
    log('[XERAR] ERRO: ' + err)
    checkPandoc()
  } finally {
    btn.disabled = false
  }
}

function exportDocx() {
  return exportDocOffice(AccionResultadoDocx, ExportDocx, btnExportDocx,
    t('status.compilingDocx', 'Compilando Word (.docx)...'), '.docx',
    'status.errorExportingDocx', 'Erro ao exportar Word')
}

function exportOdt() {
  return exportDocOffice(AccionResultadoOdt, ExportOdt, btnExportOdt,
    t('status.compilingOdt', 'Compilando OpenDocument (.odt)...'), '.odt',
    'status.errorExportingOdt', 'Erro ao exportar OpenDocument')
}

// nomeDocumento: nome do .matex aberto sen extensión nin ruta ("exame" por
// defecto se aínda non se gardou). Vai en cada petición de xeración para
// que a plantilla activa poida usalo en {{TITULO}}/{{FICHEIRO}} (ver
// plantillas.go), e é tamén o nome base que se lle propón a cada
// exportación.
function nomeDocumento() {
  if (!currentPath) return 'exame'
  return currentPath.replace(/\.matex$/i, '').split(/[/\\]/).pop()
}

function dirOf(path) {
  const m = /^(.*)[/\\][^/\\]*$/.exec(path || '')
  return m ? m[1] : ''
}

async function generatePDF() {
  if (souXanelaResultado) {
    return accionRemota(AccionResultadoPDF, btnPDF, t('status.exporting', 'Exportando...'),
      'status.errorCompilingPdf', 'Erro ao compilar PDF')
  }
  syncToText()
  btnPDF.disabled = true
  setStatus(t('status.compilingPdf', 'Compilando PDF (Maxima + pdflatex)...'))
  showWarnings([])
  log('[XERAR] Compilando PDF (Maxima + pdflatex)...')
  try {
    const result = await GeneratePDF({
      source: editor.getValue(),
      codeIni: '',
      modo: modoActivo(),
      tipografia: tipografiaActual(),
      seed: parseInt(seedInput.value, 10) || 0,
      iterations: parseInt(iterInput.value, 10) || 1,
      baseDir: dirOf(currentPath),
      nomeDoc: nomeDocumento(),
    })
    lastPDFBase64 = result.pdfBase64
    lastLatexSource = result.latexSource
    preview.srcdoc = pagesToPreviewHTML(result.pageImages)
    showWarnings(result.warnings)
    publicarEstadoResultado(result)
    logPdflatex(result.log)
    const base = currentPath ? currentPath.replace(/\.matex$/i, '') + '.pdf' : 'exame.pdf'
    const path = await SavePDFDialog(base, result.pdfBase64)
    setStatusGardado(path ? t('status.pdfSaved', 'PDF gardado: ') + path : t('status.done', 'Feito.'), path)
    log('[XERAR] ' + (path ? 'PDF gardado: ' + path : 'Feito.'))
  } catch (err) {
    setStatus(t('status.errorCompilingPdf', 'Erro ao compilar PDF'))
    showWarnings([mensaxeErro(err)])
    log('[XERAR] ERRO: ' + err)
  } finally {
    btnPDF.disabled = false
  }
}

// ── Plantillas de documento (maqueta) ────────────────────────────────────
// A plantilla activa vive no backend (plantillas.go), non aquí: Generate/
// GeneratePDF/ExportMarkdown resólvena elas soas ao xerar. O frontend só
// escolle cal é (selector da barra) e amosa a biblioteca. Por iso cambiar
// de plantilla NON toca ningunha das peticións de xeración - só chama a
// EscollerPlantilla e volve xerar.
const apiPlantillas = {
  listar: () => ListarPlantillas(),
  gardar: (p) => GardarPlantilla(p),
  eliminar: (id) => EliminarPlantilla(id),
  duplicar: (id) => DuplicarPlantilla(id),
  escoller: (id, ficheiro) => EscollerPlantilla(id, ficheiro),
  subirImaxe: (req) => SubirImaxePlantilla(req),
  eliminarImaxe: (id, nome) => EliminarImaxePlantilla(id, nome),
  previsualizar: (p) => PrevisualizarPlantilla(p),
  xerarIA: (req) => XerarPlantillaIA(req),
  exportar: (id) => ExportarPlantilla(id),
  importar: () => ImportarPlantilla(),
}

// refrescarSelectorPlantillas volve pintar o desplegable da barra co que
// haxa gardado. Chámase ao arrincar e cada vez que a biblioteca cambia
// (crear/editar/eliminar pode mudar tanto os nomes coma cal está activa).
async function refrescarSelectorPlantillas() {
  if (!plantillaSelect) return
  try {
    const estado = await ListarPlantillas()
    plantillaSelect.innerHTML = ''
    const ningunha = document.createElement('option')
    ningunha.value = ''
    ningunha.textContent = t('plantillas.none', 'Ningunha')
    plantillaSelect.appendChild(ningunha)
    // Bifurcar unha de fábrica (ver bifurcarDeFabrica, plantillas.go) deixa
    // dúas entradas co MESMO nome: a copia do profesorado e a de fábrica que
    // agora sobrevive. Na biblioteca distínguense polo distintivo "de
    // exemplo", pero aquí só hai texto, así que se marca a de fábrica -
    // e só cando hai empate, para non ensuciar o caso normal.
    const veces = new Map()
    for (const p of estado.plantillas || []) veces.set(p.nome, (veces.get(p.nome) || 0) + 1)
    for (const p of estado.plantillas || []) {
      const opt = document.createElement('option')
      opt.value = p.id
      opt.textContent = p.deFabrica && veces.get(p.nome) > 1
        ? `${p.nome} (${t('plantillas.builtin', 'de exemplo')})`
        : p.nome
      plantillaSelect.appendChild(opt)
    }
    plantillaSelect.value = estado.activa || ''
  } catch (err) {
    showWarnings([mensaxeErro(err)])
  }
}

// cambiarPlantilla aplica a escolla e volve xerar SE xa había un resultado
// na pantalla: a plantilla é precisamente o que se ve, así que deixar a
// vista previa coa maqueta anterior sería enganoso. Nun documento aínda sen
// xerar non se compila nada (evita disparar Maxima sen que o pidan).
async function cambiarPlantilla(id) {
  try {
    await EscollerPlantilla(id, currentPath || '')
    setStatus(id
      ? t('status.templateApplied', 'Plantilla aplicada')
      : t('status.templateNone', 'Sen plantilla'))
    if (lastPDFBase64) await run()
  } catch (err) {
    setStatus(t('status.errorTemplate', 'Erro coa plantilla'))
    showWarnings([mensaxeErro(err)])
  }
}

// sincronizarPlantillaDoDocumento: ao abrir un .matex recupérase a plantilla
// que ese documento usaba (PlantillaParaFicheiro, plantillas.go). Un
// documento novo, ou un que nunca tivo plantilla, deixa a activa como
// estea.
async function sincronizarPlantillaDoDocumento() {
  if (!plantillaSelect) return
  try {
    await PlantillaParaFicheiro(currentPath || '')
    await refrescarSelectorPlantillas()
  } catch (err) {
    showWarnings([mensaxeErro(err)])
  }
}

// lembrarPlantillaDoDocumento anota a plantilla activa para a ruta actual -
// tras un "Gardar como", que é cando un documento estrea ruta.
async function lembrarPlantillaDoDocumento() {
  if (!plantillaSelect || !currentPath) return
  try {
    await EscollerPlantilla(plantillaSelect.value || '', currentPath)
  } catch (err) {
    showWarnings([mensaxeErro(err)])
  }
}

function abrirPlantillas() {
  abrirBibliotecaPlantillas({
    t,
    api: apiPlantillas,
    ficheiroActual: () => currentPath || '',
    onEscoller: async () => {
      const anterior = plantillaSelect ? plantillaSelect.value : ''
      await refrescarSelectorPlantillas()
      // Se a biblioteca cambiou a plantilla en uso, aplícase igual ca dende
      // o selector (volver xerar incluído).
      if (plantillaSelect && plantillaSelect.value !== anterior && lastPDFBase64) await run()
    },
  })
}

// --- Options modal ---

// IA_PRESETS mapea cada opción do selector "Provedor de IA" a que formato
// de fío fala (provedor, ver ia_client.go) e que enderezo de servidor usar
// - así o profesorado só escolle un nome de marca recoñecible, nunca
// escribe un enderezo nin sabe que "Qwen" e "ChatGPT" acaban falando o
// mesmo protocolo por baixo. "outro" queda cun enderezo baleiro a propósito
// (o único caso onde SI hai que escribilo á man: un provedor "compatible
// con OpenAI" que non está na lista).
const IA_PRESETS = {
  gemini: { provedor: 'gemini', baseURL: '', modeloExemplo: 'gemini-2.0-flash' },
  openai: { provedor: 'openai', baseURL: '', modeloExemplo: 'gpt-4o-mini' },
  anthropic: { provedor: 'anthropic', baseURL: '', modeloExemplo: 'claude-3-5-haiku-latest' },
  qwen: { provedor: 'openai', baseURL: 'https://dashscope.aliyuncs.com/compatible-mode/v1', modeloExemplo: 'qwen-plus' },
  perplexity: { provedor: 'openai', baseURL: 'https://api.perplexity.ai', modeloExemplo: 'sonar' },
  outro: { provedor: 'openai', baseURL: '', modeloExemplo: 'o-nome-exacto-do-modelo' },
}

// aplicarPresetIA actualiza o placeholder do modelo e (agás cando se está a
// prerrechear dende axustes xa gardados) o enderezo do servidor, e amosa a
// fila de "Enderezo do servidor" só para "outro" - os demais presets xa
// levan o seu enderezo fixo, non hai nada que o profesorado teña que ver
// nin tocar.
function aplicarPresetIA(preset, manterBaseURLGardado) {
  const p = IA_PRESETS[preset] || IA_PRESETS.gemini
  optIAModel.placeholder = p.modeloExemplo
  if (!manterBaseURLGardado) optIABaseURL.value = p.baseURL
  optIABaseURLRow.classList.toggle('hidden', preset !== 'outro')
}

optIAPreset.addEventListener('change', () => aplicarPresetIA(optIAPreset.value, false))

// aplicarVisibilidadeExportadores agocha/amosa os botóns da toolbar da
// previsualización (btnPDF/btnExportTex/.../btnExportOdt) segundo o que o
// profesorado marcou en Opcións. PDF/Tex/Markdown usan "!== false" (mesmo
// patrón ca xerarAoGardarActivo): un settings.json vello, ou un profesor
// que nunca abriu Opcións, non ten estas chaves (undefined) e os tres deben
// seguir visibles coma sempre. DOCX/ODT son o contrario - por defecto
// agochados (precisan Pandoc) - así que "!!" abonda.
function aplicarVisibilidadeExportadores(s) {
  btnPDF.classList.toggle('hidden', s.exportarPdf === false)
  btnExportTex.classList.toggle('hidden', s.exportarTex === false)
  btnExportMarkdown.classList.toggle('hidden', s.exportarMarkdown === false)
  btnExportDocx.classList.toggle('hidden', !s.exportarDocx)
  btnExportOdt.classList.toggle('hidden', !s.exportarOdt)
}

async function openOptions() {
  try {
    const s = await GetSettings()
    optMaximaPath.value = s.maximaPath || ''
    optCodeIni.value = s.codeIni || ''
    optIAPreset.value = s.iaPreset || 'gemini'
    aplicarPresetIA(optIAPreset.value, true) // manter o enderezo xa gardado, non o do preset
    optIAApiKey.value = s.iaApiKey || ''
    optIAModel.value = s.iaModel || ''
    optIABaseURL.value = s.iaBaseUrl || ''
    optIAMaxTokens.value = s.iaMaxTokens || 0
    optIATimeout.value = s.iaTimeoutSegundos || 240
    optLatexEngine.value = s.latexEngine || ''
    optDecimais.value = s.decimais || 2
    optDecimal.checked = !!s.decimal
    optTimeout.value = s.timeoutSegundos || 30
    optReorgSementes.value = s.reorganizarSementes || 25
    optReorgRoldas.value = s.reorganizarRoldas || 3
    if (optAutoreparar) optAutoreparar.checked = s.autorepararExercicios !== false
    if (optMaxRechamadasIA) optMaxRechamadasIA.value = s.maxRechamadasIA || 2
    optXerarAoGardar.checked = s.xerarAoGardar !== false
    optIdioma.value = s.idioma || idiomaPiztu || 'gl'
    if (optEditorMode) optEditorMode.value = s.editorMode === 'blockly' ? 'blockly' : 'tabla'
    optExportPDF.checked = s.exportarPdf !== false
    optExportTex.checked = s.exportarTex !== false
    optExportMarkdown.checked = s.exportarMarkdown !== false
    optExportDocx.checked = !!s.exportarDocx
    optExportOdt.checked = !!s.exportarOdt
    const kmStatus = document.getElementById('killMaximaStatus')
    if (kmStatus) kmStatus.textContent = ''
    optionsOverlay.classList.remove('hidden')
  } catch (err) {
    setStatus(t('status.errorLoadingOptions', 'Erro ao cargar opcións'))
    showWarnings([mensaxeErro(err)])
  }
}

function closeOptions() {
  optionsOverlay.classList.add('hidden')
}

// refrescarIdioma aplica un cambio de idioma sen reiniciar Yang: retraduce
// o DOM estático (aplicarTraducions) e, se o editor de bloques xa existe,
// recréao co novo `t` (destroy + create) conservando o documento actual -
// blocksEditor non pode simplemente "redebuxarse" porque gardou `t` nun
// closure fixo ao crearse (ver novoBlocksEditor). O editor de texto
// (CodeMirror) non ten texto traducible, non fai falla tocalo.
function refrescarIdioma(code) {
  cargarIdioma(code)
  aplicarTraducions()
  if (blocksEditor) {
    const valorActual = blocksEditor.getValue()
    blocksEditor.destroy()
    blocksEditor = novoBlocksEditor(valorActual)
    if (editMode === 'blocks') blocksEditor.focus()
  }
  // Mesmo motivo có editor de bloques: table-editor.js garda `t` nun closure
  // ao crearse, así que un cambio de idioma esixe recrealo.
  if (tableEditor) {
    const valorActual = tableEditor.getValue()
    const modoActual = tableEditor.getModo()
    tableEditor.destroy()
    tableEditor = novoTableEditor(valorActual)
    tableEditor.setModo(modoActual)
    if (editMode === 'table') tableEditor.focus()
  }
  checkLatex() // latexWarningText leva texto dinámico interpolado, non só data-i18n
  refrescarSelectorPlantillas() // as opcións do selector píntanse en JS, non levan data-i18n
  pintarActualizacionYang() // sobreVersion/sobreActualizacion tamén levan texto interpolado
}

async function saveOptions() {
  try {
    // valorSeleccionado é o que se ve e se aplica; idiomaAGardar é o que se
    // persiste. Se o escollido coincide co idioma que Yang xa herdaría de
    // Piztu, gárdase "" (segue en modo automático: se Piztu cambia de idioma
    // máis adiante, Yang cámbiao con el). Só se garda un valor concreto cando
    // o profesorado escolleu algo DISTINTO — iso si é unha elección
    // explícita, que debe prevalecer sempre, mesmo sobre futuros cambios en
    // Piztu.
    const valorSeleccionado = optIdioma.value
    const idiomaAGardar = valorSeleccionado === idiomaPiztu ? '' : valorSeleccionado
    await SaveSettings({
      maximaPath: optMaximaPath.value.trim(),
      codeIni: optCodeIni.value,
      iaPreset: optIAPreset.value,
      iaProvedor: (IA_PRESETS[optIAPreset.value] || IA_PRESETS.gemini).provedor,
      iaApiKey: optIAApiKey.value.trim(),
      iaModel: optIAModel.value.trim(),
      iaBaseUrl: optIABaseURL.value.trim(),
      // iaMaxTokens ata agora non se mandaba (nin sequera existía o campo na
      // xanela), así que CADA "Gardar" de Opcións o poñía a 0 - o límite
      // quedaba no valor por defecto de cada provedor sen xeito de subilo.
      // Fai falla sobre todo cun exame "Indefinido": a resposta pode ser
      // longa e, se non colle, chega cortada (ver formatoInesperadoErr).
      iaMaxTokens: parseInt(optIAMaxTokens.value, 10) || 0,
      iaTimeoutSegundos: parseInt(optIATimeout.value, 10) || 240,
      latexEngine: optLatexEngine.value,
      decimais: parseInt(optDecimais.value, 10) || 2,
      decimal: optDecimal.checked,
      timeoutSegundos: parseInt(optTimeout.value, 10) || 30,
      reorganizarSementes: parseInt(optReorgSementes.value, 10) || 25,
      reorganizarRoldas: parseInt(optReorgRoldas.value, 10) || 3,
      autorepararExercicios: optAutoreparar ? optAutoreparar.checked : true,
      maxRechamadasIA: (optMaxRechamadasIA && parseInt(optMaxRechamadasIA.value, 10)) || 2,
      xerarAoGardar: optXerarAoGardar.checked,
      idioma: idiomaAGardar,
      editorMode: optEditorMode ? optEditorMode.value : 'tabla',
      exportarPdf: optExportPDF.checked,
      exportarTex: optExportTex.checked,
      exportarMarkdown: optExportMarkdown.checked,
      exportarDocx: optExportDocx.checked,
      exportarOdt: optExportOdt.checked,
    })
    xerarAoGardarActivo = optXerarAoGardar.checked
    aplicarVisibilidadeExportadores({
      exportarPdf: optExportPDF.checked,
      exportarTex: optExportTex.checked,
      exportarMarkdown: optExportMarkdown.checked,
      exportarDocx: optExportDocx.checked,
      exportarOdt: optExportOdt.checked,
    })
    if (valorSeleccionado !== idiomaActual) refrescarIdioma(valorSeleccionado)
    // Interface de edición: aplícase en vivo se cambiou (o modo 'text'
    // interno non conta - só se alterna entre táboa e bloques).
    const modoDesexado = (optEditorMode && optEditorMode.value === 'blockly') ? 'blocks' : 'table'
    if (editMode !== 'text' && editMode !== modoDesexado) switchMode(modoDesexado)
    closeOptions()
    setStatus(t('status.optionsSaved', 'Opcións gardadas'))
  } catch (err) {
    setStatus(t('status.errorSavingOptions', 'Erro ao gardar opcións'))
    showWarnings([mensaxeErro(err)])
  }
}

async function detectMaxima() {
  const path = await DetectMaxima()
  optMaximaPath.value = path || ''
  setStatus(path ? t('status.found', 'Atopado: ') + path : t('status.maximaNotFound', 'Non se atopou Maxima'))
}

// --- Maxima missing warning / auto-install ---

async function checkMaxima() {
  try {
    const s = await GetSettings()
    maximaWarning.classList.toggle('hidden', !!s.maximaPath)
  } catch (err) {
    // ignore - the warning just won't show if settings can't be read
  }
}

async function installMaximaNow() {
  btnInstallMaxima.disabled = true
  btnInstallMaxima.textContent = t('status.installingMaxima', '⏳ Instalando... (pode pedir contrasinal)')
  try {
    const result = await InstallMaxima()
    if (result.success) {
      setStatus(t('status.maximaInstalled', 'Maxima instalado correctamente.'))
      maximaWarning.classList.add('hidden')
    } else {
      setStatus(t('status.autoInstallFailed', 'Non se puido instalar automaticamente'))
      showWarnings([result.log || t('status.unknownError', 'erro descoñecido')])
    }
  } catch (err) {
    setStatus(t('status.errorInstalling', 'Erro ao instalar'))
    showWarnings([mensaxeErro(err)])
  } finally {
    btnInstallMaxima.disabled = false
    btnInstallMaxima.textContent = t('banner.maxima.install', '⇩ Instalar automaticamente')
  }
}

// --- Aviso de LaTeX/pdftoppm ---
// Mesmo mecanismo que o de Maxima (checkMaxima/installMaximaNow) para o
// resto da canle de compilación: o motor LaTeX escollido en Opcións
// (GeneratePDF, latexdoc.go, xa falla cun erro que menciona ese mesmo
// motor se falta) e pdftoppm (necesario para a vista previa e para <TIKZ>
// en modo HTML). Chámase ao arrincar e despois de calquera instalación -
// "unha vez instalado desaparece o aviso" refírese a iso: non hai sondaxe
// en segundo plano, así que se se instala á man nun terminal á parte,
// desaparece cando se volva comprobar (arrinque seguinte, ou premendo
// calquera acción que chame checkLatex()).
async function checkLatex() {
  try {
    const s = await CheckLatexDeps()
    if (s.engineFound && s.pdftoppmFound) {
      latexWarning.classList.add('hidden')
      return
    }
    const faltan = []
    if (!s.engineFound) faltan.push(s.engine)
    if (!s.pdftoppmFound) faltan.push('pdftoppm (poppler-utils)')
    latexWarningText.textContent = t('status.latexMissing', '⚠ Falta {deps} neste equipo. É necesario para xerar/previsualizar PDFs.', {
      deps: faltan.join(t('status.latexMissing.and', ' e ')),
    })
    latexWarning.classList.remove('hidden')
  } catch (err) {
    // ignore - o aviso simplemente non se amosa se non se puido comprobar
  }
}

async function installLatexNow() {
  btnInstallLatex.disabled = true
  btnInstallLatex.textContent = t('status.installingLatex', '⏳ Instalando... (pode pedir contrasinal, tarda uns minutos)')
  try {
    const result = await InstallLatex()
    if (result.success) {
      setStatus(t('status.latexInstalled', 'LaTeX instalado correctamente.'))
      checkLatex()
    } else {
      setStatus(t('status.autoInstallFailed', 'Non se puido instalar automaticamente'))
      showWarnings([result.log || t('status.unknownError', 'erro descoñecido')])
    }
  } catch (err) {
    setStatus(t('status.errorInstalling', 'Erro ao instalar'))
    showWarnings([mensaxeErro(err)])
  } finally {
    btnInstallLatex.disabled = false
    btnInstallLatex.textContent = t('banner.latex.install', '⇩ Instalar automaticamente')
  }
}

// --- Aviso de Pandoc ---
// Mesmo mecanismo que Maxima/LaTeX (checkMaxima/checkLatex), pero para
// Pandoc - a única ferramenta extra que precisan ExportDocx/ExportOdt
// (docdoc.go), independente da canle de PDF (un profesor pode querer só
// Word/OpenDocument, sen tocar LaTeX en absoluto).
async function checkPandoc() {
  try {
    const s = await CheckDocDeps()
    pandocWarning.classList.toggle('hidden', !!s.pandocFound)
  } catch (err) {
    // ignore - o aviso simplemente non se amosa se non se puido comprobar
  }
}

async function installPandocNow() {
  btnInstallPandoc.disabled = true
  btnInstallPandoc.textContent = t('status.installingPandoc', '⏳ Instalando... (pode pedir contrasinal)')
  try {
    const result = await InstallPandoc()
    if (result.success) {
      setStatus(t('status.pandocInstalled', 'Pandoc instalado correctamente.'))
      pandocWarning.classList.add('hidden')
    } else {
      setStatus(t('status.autoInstallFailed', 'Non se puido instalar automaticamente'))
      showWarnings([result.log || t('status.unknownError', 'erro descoñecido')])
    }
  } catch (err) {
    setStatus(t('status.errorInstalling', 'Erro ao instalar'))
    showWarnings([mensaxeErro(err)])
  } finally {
    btnInstallPandoc.disabled = false
    btnInstallPandoc.textContent = t('banner.pandoc.install', '⇩ Instalar automaticamente')
  }
}

// --- Progreso das instalacións (yang:instalacion-progreso, installer.go) ---
//
// InstallMaxima/InstallLatex/InstallPandoc son síncronos: o await de arriba
// non devolve nada ata que rematan. Iso vale para un `apt install pandoc` de
// vinte segundos, pero non para `brew install --cask mactex-no-gui` en macOS,
// que son preto de 6 GB e pode pasar media hora sen que Homebrew emita unha
// soa liña (non pinta a barra de progreso cando a saída non é un terminal).
// Sen isto, o botón queda cun "⏳ Instalando…" fixo e o natural é pensar que
// Yang colgou e pechalo a metade da descarga.
//
// O proceso Go emite un evento decontado e despois cada 30 segundos; aquí só
// se reescribe o texto do botón correspondente.
const botonsInstalacion = {
  maxima: () => btnInstallMaxima,
  latex: () => btnInstallLatex,
  pandoc: () => btnInstallPandoc,
}

Events.On('yang:instalacion-progreso', (e) => {
  const p = e.data || {}
  const boton = botonsInstalacion[p.que]?.()
  // Se xa rematou, non tocar nada: o finally de install*Now() é quen
  // restaura o texto orixinal do botón.
  if (!boton || p.completo) return

  if (p.mb > 0) {
    boton.textContent = t('status.installingProgress.mb', '⏳ Descargando… {mb} MB ({min} min)', {
      mb: p.mb,
      min: p.minutos,
    })
  } else if (p.minutos > 0) {
    boton.textContent = t('status.installingProgress', '⏳ Instalando… ({min} min)', { min: p.minutos })
  }
})

// --- Modo dúas xanelas (Opcións → Modo de traballo, ver xanelaresultado.go) ---

// entrarModoResultado configura ESTA xanela coma a "xanela de resultado":
// agocha todo agás o panel de resultado (que pasa a ocupar a xanela
// enteira, sen editor ao lado - ver .pane-preview en CSS, flex:1 abonda en
// canto .pane-editor está agochado) e escoita yang:resultado-actualizado
// (Events, emitido por PublicarEstadoResultado no proceso Go cada vez que
// o editor Xera/PDF) para manter as páxinas actualizadas - mesmo proceso,
// mesmo evento chega en tempo real, xa non fai falla sondar por HTTP coma
// no vello modo dous procesos.
function entrarModoResultado() {
  maximaWarning.classList.add('hidden')
  latexWarning.classList.add('hidden')
  pandocWarning.classList.add('hidden')
  document.getElementById('toolbar').classList.add('hidden')
  document.querySelector('.pane-editor').classList.add('hidden')
  paneSplitter.classList.add('hidden')
  // O selector de plantilla vive agora na cabeceira do resultado (á esquerda
  // de "Imprimir"), pero na xanela de resultado desacoplada non hai nada que
  // escoller (xérase sempre desde a xanela editor): agóchase.
  document.getElementById('plantillaWrap')?.classList.add('hidden')
  btnPlantillas?.classList.add('hidden')

  let ultimaVersion = -1
  function aplicarEstado(estado) {
    if (estado.version === ultimaVersion) return
    ultimaVersion = estado.version
    preview.srcdoc = pagesToPreviewHTML(estado.pageImages)
    showWarnings(estado.warnings)
  }

  // Subscribirse ANTES de pedir o snapshot inicial - se chega un Xerar()
  // entre medias, o evento non se perde (mesmo protocolo de hidratación
  // usado no spike de v3).
  Events.On('yang:resultado-actualizado', (e) => aplicarEstado(e.data))
  GetEstadoResultado().then(aplicarEstado).catch(() => {})
}

// --- Xerador con IA desacoplado (?xerador=exercicio|exame, ver
//     xanelaxeradoria.go) ---

// entrarModoXeradorIA configura ESTA xanela coma o formulario do xerador con
// IA á parte: agocha toda a UI do editor e amosa só a modal de xeración en
// modo "detached". O que xera mándao á xanela PRINCIPAL vía EntregarXeracionIA
// (que reemite coma evento yang:xerador-ia-*, escoitado en arrincarModoXanelas).
async function entrarModoXeradorIA() {
  maximaWarning.classList.add('hidden')
  latexWarning.classList.add('hidden')
  pandocWarning.classList.add('hidden')
  document.getElementById('toolbar').classList.add('hidden')
  document.getElementById('panes').classList.add('hidden')
  document.getElementById('warnings')?.classList.add('hidden')
  document.getElementById('log-panel')?.classList.add('hidden')
  // Nesta xanela a modal É a xanela enteira: sen fondo escurecido de overlay
  // (ver blocks.css) e sen scroll do corpo.
  document.body.classList.add('modo-xerador-ia')

  let est = {}
  try { est = JSON.parse((await LerEstadoXeradorIA()) || '{}') } catch { est = {} }

  const comun = {
    t,
    detached: true,
    initialState: est,
    onAcoplar: () => AcoplarXeradorIA(),
    // A compilación do resultado dispáraa a xanela principal ao inserir (ver
    // os listeners yang:xerador-ia-* en arrincarModoXanelas), non aquí.
    onGenerateResult: null,
  }

  if (xeradorIAModo === 'exame') {
    openExamAIModal({
      ...comun,
      generateExamWithAI,
      onCreateExercises: (lista) => {
        EntregarXeracionIA(JSON.stringify({ kind: 'exame', op: 'create', exercicios: lista }))
        return { start: 0, count: lista.length }
      },
      onReplaceExercises: (_start, _count, lista) => {
        EntregarXeracionIA(JSON.stringify({ kind: 'exame', op: 'replace', exercicios: lista }))
      },
    })
  } else {
    openExerciseAIModal({
      ...comun,
      generateExerciseWithAI,
      onCreateExercise: (els) => {
        EntregarXeracionIA(JSON.stringify({ kind: 'exercicio', op: 'create', elements: els }))
        return 'detached'
      },
      onReplaceExercise: (_index, els) => {
        EntregarXeracionIA(JSON.stringify({ kind: 'exercicio', op: 'replace', elements: els }))
      },
    })
  }
}

// xeradorDetachedTracking: na XANELA PRINCIPAL, que exercicio/rango creou a
// última xeración da xanela desacoplada, para que a seguinte "Xerar" (op:
// replace) o substitúa en vez de acumular. Reiníciase ao pechar a xanela
// (yang:xerador-ia-pechado).
let xeradorDetachedTracking = { exIndex: null, exameRango: null }

function garantirModoBloques() {
  if (editMode !== 'blocks') switchMode('blocks')
}

function aplicarXeracionExercicioIA(data) {
  const op = data && data.op
  const elements = data && data.elements
  if (!Array.isArray(elements)) return
  garantirModoBloques()
  if (!blocksEditor) return
  if (op === 'replace' && xeradorDetachedTracking.exIndex != null) {
    blocksEditor.replaceExerciseElements(xeradorDetachedTracking.exIndex, elements)
  } else {
    xeradorDetachedTracking.exIndex = blocksEditor.addExerciseWithElements(elements)
  }
  run()
}

function aplicarXeracionExameIA(data) {
  const op = data && data.op
  const exercicios = data && data.exercicios
  if (!Array.isArray(exercicios) || exercicios.length === 0) return
  garantirModoBloques()
  if (!blocksEditor) return
  const r = xeradorDetachedTracking.exameRango
  if (op === 'replace' && r) {
    blocksEditor.replaceExercisesRange(r.start, r.count, exercicios)
    xeradorDetachedTracking.exameRango = { start: r.start, count: exercicios.length }
  } else {
    xeradorDetachedTracking.exameRango = blocksEditor.addExercisesWithElements(exercicios)
  }
  run()
}

// activarModoDobreEditor agocha o panel de resultado NESTA xanela (o
// editor) cando Opcións → Modo de traballo == "dobre" - o resultado vive
// na outra xanela (entrarModoResultado), amosando no seu lugar un aviso +
// un botón para reabrila por se se pechou sen querer.
function activarModoDobreEditor() {
  resultadoDesacoplado = true
  panePreview.classList.add('hidden')
  panePreviewPlaceholder.classList.remove('hidden')
  actualizarPanelFondo()
}

// saírModoDobreEditor reverte activarModoDobreEditor - chámase ao recibir
// yang:resultado-pechado (Events, emitido por WindowClosing en
// xanelaresultado.go ao pechar a xanela de resultado, X do xestor de
// xanelas - non hai botón "Acoplar" propio, ver comentario en index.html).
// preview.srcdoc xa está actualizado (run()/generatePDF() sempre o
// escriben, mesmo con esta xanela agochada), así que abonda con amosala de
// novo - sen isto o aviso "resultado noutra xanela" quedaba colgado despois
// de pechar a xanela de resultado.
function saírModoDobreEditor() {
  resultadoDesacoplado = false
  panePreviewPlaceholder.classList.add('hidden')
  panePreview.classList.remove('hidden')
  actualizarPanelFondo()
}

// refrescarCodigoDeFondo/manexarCambioCodigoDeFondo/mostrarCodigoDeFondo/
// agocharCodigoDeFondo/actualizarPanelFondo: en modo bloques CO resultado
// desacoplado, o panel de fondo (antes só o aviso "resultado noutra
// xanela") pasa a amosar o propio editor de código - o MESMO CodeMirror
// (con matexeTagBar, a súa barra de etiquetas) que en modo texto, movido
// aquí e totalmente editable - para poder escribir .matex a man sen ter
// que pechar a vista de bloques da esquerda. Sincronización BIDIRECCIONAL:
//   bloques -> código: sondeo cada 600ms (blocksEditor non ten evento de
//     cambio propio), pero só mentres o profesorado NON estea escribindo
//     no código (editorTenFoco) - senón perderíaselle o que vai escribindo.
//   código -> bloques: onChange de CodeMirror (editor.js), con debounce de
//     500ms mentres escribe + un volcado inmediato ao perder o foco (evita
//     que un sondeo colle a versión vella se o profesorado cambia a bloques
//     xusto despois de escribir, antes de que cumpra o debounce).
// sincronizandoDesdeBloques evita o ping-pong: o propio editor.setValue()
// de refrescarCodigoDeFondo dispara onChange coma calquera cambio, así que
// hai que distinguilo dun cambio real do profesorado.
// Só ten sentido en bloques: en texto o código xa está á vista na esquerda,
// e o aviso "resultado noutra xanela" (con "Reabrir") volve amosarse aí
// coma sempre - iso serve tamén de vía de escape se algunha vez fixese
// falla reabrir a xanela á man.
let sincronizandoDesdeBloques = false
let debounceCodigoDeFondo = null

function editorTenFoco() {
  return editorContainer.contains(document.activeElement)
}

function refrescarCodigoDeFondo() {
  if (editorTenFoco()) return
  const sec = activeSecondaryEditor()
  const texto = sec ? sec.getValue() : ''
  if (texto === editor.getValue()) return
  sincronizandoDesdeBloques = true
  editor.setValue(texto)
  sincronizandoDesdeBloques = false
}

function volcarCodigoDeFondoABloques() {
  clearTimeout(debounceCodigoDeFondo)
  debounceCodigoDeFondo = null
  const sec = activeSecondaryEditor()
  if (sec) sec.setValue(editor.getValue())
}

function manexarCambioCodigoDeFondo() {
  if (!codigoDeFondoActivo || sincronizandoDesdeBloques) return
  clearTimeout(debounceCodigoDeFondo)
  debounceCodigoDeFondo = setTimeout(volcarCodigoDeFondoABloques, 500)
}

// Ao perder o foco, volcar xa (non esperar o debounce) - ver comentario de
// enriba. relatedTarget (elemento que gaña o foco) pode vir baleiro nalgún
// caso raro do navegador; tratalo igual có resto (volcar de todos xeitos,
// é inofensivo facelo de máis).
editorContainer.addEventListener('focusout', (e) => {
  if (!codigoDeFondoActivo || editorContainer.contains(e.relatedTarget)) return
  if (debounceCodigoDeFondo) volcarCodigoDeFondoABloques()
})

function mostrarCodigoDeFondo() {
  if (codigoDeFondoActivo) return
  codigoDeFondoActivo = true
  refrescarCodigoDeFondo()
  placeholderAviso.classList.add('hidden')
  panePreviewPlaceholder.appendChild(matexeTagBar)
  panePreviewPlaceholder.appendChild(editorContainer)
  matexeTagBar.classList.remove('hidden')
  editorContainer.classList.remove('hidden')
  intervalCodigoFondo = setInterval(refrescarCodigoDeFondo, 600)
}

function agocharCodigoDeFondo() {
  if (!codigoDeFondoActivo) return
  if (debounceCodigoDeFondo) volcarCodigoDeFondoABloques()
  codigoDeFondoActivo = false
  clearInterval(intervalCodigoFondo)
  intervalCodigoFondo = null
  paneEditor.insertBefore(matexeTagBar, blocksContainer)
  paneEditor.insertBefore(editorContainer, blocksContainer)
  // Restaurar a visibilidade normal segundo o modo actual - este camiño
  // (acoplar) non pasa por switchMode (editMode non cambia), así que
  // ninguén máis a recoloca: en bloques teñen que quedar agochados de novo,
  // ou aparecerían por riba do editor de bloques (ver captura de Martin).
  matexeTagBar.classList.toggle('hidden', editMode !== 'text')
  editorContainer.classList.toggle('hidden', editMode !== 'text')
  placeholderAviso.classList.remove('hidden')
}

function actualizarPanelFondo() {
  const activo = resultadoDesacoplado && (editMode === 'blocks' || editMode === 'table')
  if (activo) {
    mostrarCodigoDeFondo()
  } else {
    agocharCodigoDeFondo()
  }
  // "⤢ Ampliar"/"⤡ Repregar" (ver blocks-editor.js/setBlocksFullWidth) só
  // ten sentido mentres existe de verdade un panel de código de fondo que
  // ampliar por riba - blocksEditor pode aínda non existir (arranque, antes
  // do primeiro switchMode('blocks')).
  if (blocksEditor) blocksEditor.setFullWidthToggleVisible(activo && editMode === 'blocks')
  if (tableEditor) tableEditor.setFullWidthToggleVisible(activo && editMode === 'table')
}

btnReabrirResultado.addEventListener('click', async () => {
  try {
    await AbrirXanelaResultado()
  } catch (err) {
    setStatus(mensaxeErro(err))
  }
})

// --- Actualización de Yang (← botón "i" da barra) ---
// O backend comproba en fondo ao arrincar (evento actualizacion_yang_cambiada,
// ver Events.On abaixo); aquí só se pinta o que xa se sabe. Mesmo mecanismo
// ca "Sobre Piztu" en piztu/frontend/src/main.js.
let actualizacionYang = null

function pintarActualizacionYang() {
  sobreVersion.textContent =
    t('about.version', 'Versión') + ' ' + (actualizacionYang ? actualizacionYang.versionLocal : '')

  const dispo = !!(actualizacionYang && actualizacionYang.disponible)
  btnAbout.classList.toggle('btn-actualizacion-dispo', dispo)

  if (dispo) {
    sobreActualizacion.textContent = t('about.novaversion', 'Nova versión dispoñible: {v}', {
      v: actualizacionYang.versionRemota,
    })
    sobreActualizacion.classList.remove('hidden')
    btnActualizarYang.classList.remove('hidden')
    btnActualizarYang.disabled = false
    btnActualizarYang.textContent = t('about.actualizar', '⟳ Actualizar')
  } else {
    sobreActualizacion.classList.add('hidden')
    btnActualizarYang.classList.add('hidden')
  }
}

function aplicarActualizacionYang(estado) {
  actualizacionYang = estado || null
  pintarActualizacionYang()
}

// Instala en quente a última versión de Yang e reinicia (← fondo azul no
// botón "i" cando GetActualizacionYang/evento actualizacion_yang_cambiada
// din que hai unha nova). Yang péchase só e reábrese xa actualizado.
btnActualizarYang.addEventListener('click', async () => {
  if (!await confirmar(t('about.actualizar.confirmar',
      'Isto vai reiniciar Yang para aplicar a actualización. ¿Continuar?'))) {
    return
  }
  btnActualizarYang.disabled = true
  btnActualizarYang.textContent = t('about.actualizando', 'Actualizando...')
  AplicarActualizacionYang()
    .then(() => {
      btnActualizarYang.textContent = t('about.actualizar.ok', '✔ Reiniciando…')
      setTimeout(() => Application.Quit(), 400)
    })
    .catch((err) => {
      btnActualizarYang.disabled = false
      btnActualizarYang.textContent = t('about.actualizar', '⟳ Actualizar')
      showWarnings([mensaxeErro(err)])
    })
})

Events.On('actualizacion_yang_cambiada', (e) => aplicarActualizacionYang(e.data))

// --- Wiring ---

btnNew.addEventListener('click', newFile)
btnOpen.addEventListener('click', openFile)
btnSave.addEventListener('click', saveFile)
btnSaveAs.addEventListener('click', saveFileAs)
// btnUndo/btnRedo: desfacer/refacer sobre o editor ACTIVO (editMode decide
// cal) - editor.js e blocks-editor.js xa expoñen undo()/redo() propios
// (Ctrl+Z/Ctrl+Maiús+Z xa funcionaban dende o teclado en ambos os dous, ver
// eses ficheiros; isto só engade un xeito de premelos sen teclado). Seguros
// de premer sen nada que desfacer/refacer - non fan nada.
btnUndo.addEventListener('click', () => {
  const sec = activeSecondaryEditor()
  if (sec) sec.undo()
  else editor.undo()
})
btnRedo.addEventListener('click', () => {
  const sec = activeSecondaryEditor()
  if (sec) sec.redo()
  else editor.redo()
})
btnRun.addEventListener('click', run)
if (plantillaSelect) plantillaSelect.addEventListener('change', () => cambiarPlantilla(plantillaSelect.value))
if (btnPlantillas) btnPlantillas.addEventListener('click', abrirPlantillas)
btnKillMaxima.addEventListener('click', killMaxima)
btnPrint.addEventListener('click', printPreview)
btnPDF.addEventListener('click', generatePDF)
btnExportTex.addEventListener('click', exportTex)
btnExportMarkdown.addEventListener('click', exportMarkdown)
btnExportDocx.addEventListener('click', exportDocx)
btnExportOdt.addEventListener('click', exportOdt)
btnOptions.addEventListener('click', openOptions)
btnDetect.addEventListener('click', detectMaxima)
btnOptionsCancel.addEventListener('click', closeOptions)
btnOptionsSave.addEventListener('click', saveOptions)

btnInstallMaxima.addEventListener('click', installMaximaNow)
btnMaximaManual.addEventListener('click', () => maximaManualOverlay.classList.remove('hidden'))
btnMaximaManualClose.addEventListener('click', () => maximaManualOverlay.classList.add('hidden'))
btnMaximaDismiss.addEventListener('click', () => maximaWarning.classList.add('hidden'))

btnInstallLatex.addEventListener('click', installLatexNow)
btnLatexManual.addEventListener('click', () => latexManualOverlay.classList.remove('hidden'))
btnLatexManualClose.addEventListener('click', () => latexManualOverlay.classList.add('hidden'))
btnLatexDismiss.addEventListener('click', () => latexWarning.classList.add('hidden'))

btnInstallPandoc.addEventListener('click', installPandocNow)
btnPandocManual.addEventListener('click', () => pandocManualOverlay.classList.remove('hidden'))
btnPandocManualClose.addEventListener('click', () => pandocManualOverlay.classList.add('hidden'))
btnPandocDismiss.addEventListener('click', () => pandocWarning.classList.add('hidden'))

btnAbout.addEventListener('click', () => sobreOverlay.classList.remove('hidden'))
btnAboutClose.addEventListener('click', () => sobreOverlay.classList.add('hidden'))

// handleReverseSearchClick recibe a mensaxe de REVERSE_SEARCH_SCRIPT (un
// Ctrl+clic sobre un resultado da previsualización), chama a ReverseSearch
// (Go, ver reversesearch.go) e resalta a etiqueta <MAT>/<EVAL>/...
// correspondente en CodeMirror - só ten onde facelo mentres o panel de
// código de fondo está visible (ver mostrarCodigoDeFondo/
// codigoDeFondoActivo: modo bloques + resultado desacoplado). Fóra dese
// caso (editor de bloques sen ese panel) non fai nada por agora (pendente:
// atopar e abrir o modal do anaco correspondente).
async function handleReverseSearchClick(e) {
  if (!e.data || !e.data.matexeReverseSearch) return
  // A xanela de resultado non ten editor onde resaltar nada, e o
  // ReverseSearch (Go) de todos xeitos correría contra un proceso que
  // nunca compilou nada el só (ver entrarModoResultado).
  if (souXanelaResultado) return
  if (!codigoDeFondoActivo) return
  try {
    const result = await ReverseSearch(e.data.page, e.data.xPx, e.data.yPx)
    const text = editor.getValue()
    const m = /^<(MAT|EVAL|HIDE|PLOT|TEX|TIKZ)>[\s\S]*?<\/\1>/.exec(text.slice(result.offset))
    if (!m) {
      setStatus(t('status.reverseSearchNoTag', 'Non se atopou a etiqueta orixinal (edicións despois de "Xerar"?)'))
      return
    }
    editor.selectRange(result.offset, result.offset + m[0].length)
  } catch (err) {
    // Erros esperables e frecuentes (clic antes de "Xerar", clic nunha zona
    // baleira da páxina...) - só na barra de estado, sen amosalos coma aviso.
    setStatus(mensaxeErro(err))
  }
}
window.addEventListener('message', handleReverseSearchClick)

// handleZoomMessage recibe o nivel de zoom de zoomScript (Ctrl+/Ctrl-/Ctrl+0
// dentro do iframe de previsualización) e gárdao en previewZoom para que
// sobreviva á próxima "Xerar" (pagesToPreviewHTML substitúe todo o srcdoc,
// perdendo calquera estado que só vivise dentro dese documento).
window.addEventListener('message', (e) => {
  if (!e.data || typeof e.data.matexeSetZoom !== 'number') return
  previewZoom = e.data.matexeSetZoom
})

// Ctrl+N/S/O/Enter/X work regardless of focus (bubble up from the editor's
// contenteditable DOM the same as from any other element); the Matexe tag
// shortcuts (Ctrl+M/E/H/P/T/F) are editor-only and live in editor.js.
//
// Ctrl+X (Xerar) preventDefault()a antes de que o navegador chegue a
// executar o "cortar" nativo cando hai selección no editor - por deseño: o
// profesorado pediu Ctrl+X para Xerar a propósito, sabendo que perde o
// atallo de teclado para cortar texto (segue dispoñible dende o menú
// contextual).
window.addEventListener('keydown', (e) => {
  // A xanela de resultado non ten editor/ficheiro propio - Ctrl+N/S/O/etc
  // non teñen sentido alí (ver entrarModoResultado).
  if (souXanelaResultado) return
  const mod = e.ctrlKey || e.metaKey
  if (mod && e.shiftKey && (e.key === 'e' || e.key === 'E')) { e.preventDefault(); setExplorerVisible(explorerSidebar.classList.contains('hidden')) }
  else if (mod && e.shiftKey && (e.key === 's' || e.key === 'S')) { e.preventDefault(); saveFileAs() }
  else if (mod && e.key === 's') { e.preventDefault(); saveFile() }
  else if (mod && e.key === 'n') { e.preventDefault(); newFile() }
  else if (mod && e.key === 'o') { e.preventDefault(); openFile() }
  else if (mod && (e.key === 'Enter' || e.key === 'x' || e.key === 'X')) { e.preventDefault(); run() }
  else if (e.key === 'Escape') {
    closeOptions()
    maximaManualOverlay.classList.add('hidden')
    latexManualOverlay.classList.add('hidden')
    pandocManualOverlay.classList.add('hidden')
    sobreOverlay.classList.add('hidden')
  }
})

setStatus(t('status.unsaved', 'sen gardar'))
pintarActualizacionYang() // versión local xa dispoñible dende o arranque, aínda sen resposta de GitHub
GetActualizacionYang().then(aplicarActualizacionYang).catch(() => {})

// Idioma gardado: aplícase despois do arranque (en galego por defecto) en
// vez de bloquear a creación inicial dos editores nunha chamada async - un
// posible "flash" breve en galego é preferible a reestruturar todo o
// arranque do módulo arredor dun await. refrescarIdioma tamén recrea
// blocksEditor, así que isto cobre tanto o HTML estático coma a paleta de
// bloques recén creada por switchMode('blocks') máis arriba.
//
// Prioridade (ver saveOptions): 1) Settings.Idioma, se o profesorado xa
// escolleu un idioma propio en Opcións - prevalece sempre; 2) IdiomaPiztu(),
// se Yang arrincou dende Piztu e aínda non hai elección propia; 3) galego
// (xa é o que está cargado por defecto, non fai falla refrescar).
Promise.all([GetSettings(), IdiomaPiztu().catch(() => '')]).then(([s, ip]) => {
  idiomaPiztu = ip || ''
  xerarAoGardarActivo = s.xerarAoGardar !== false
  const idiomaEfectivo = s.idioma || idiomaPiztu
  if (idiomaEfectivo && idiomaEfectivo !== 'gl') refrescarIdioma(idiomaEfectivo)
  // Interface de edición gardada (arríncase en 'table' de forma síncrona
  // máis arriba - aquí só se cambia a 'blocks' se o profesorado o escolleu).
  if (s.editorMode === 'blockly' && !souXanelaResultado && !souXeradorIA) switchMode('blocks')
  actualizarBtnEditorMode()
  aplicarVisibilidadeExportadores(s)
  // souXanelaResultado: a xanela de resultado agocha #toolbar/.pane-editor
  // enteiros (ver entrarModoResultado) pero NON #explorerSidebar (é irmán
  // de .pane-editor, non fillo) - sen esta garda, se había un cartafol
  // lembrado amosaríase igual aí, intruso canda a previsualización.
  // souXeradorIA: mesma exclusión ca souXanelaResultado - esa xanela só ten
  // o formulario do xerador, sen explorador nin selector de plantilla.
  if (!souXanelaResultado && !souXeradorIA) iniciarExplorer(s.ultimoCartafol || '')
  // O selector de plantilla non existe na xanela de resultado (agocha a
  // toolbar enteira), e alí non hai nada que escoller: xérase sempre dende
  // a xanela editor.
  if (!souXanelaResultado && !souXeradorIA) refrescarSelectorPlantillas()
}).catch(() => { if (!souXanelaResultado && !souXeradorIA) iniciarExplorer('') })

// arrincarModoXanelas decide a UI segundo a xanela: Yang arrinca sempre
// acoplado (xanela única) - non hai axuste que cambie iso, desacoplar é
// unha decisión manual de sesión (botón "Desacoplar", ver máis abaixo). Só
// a xanela de resultado agocha checkMaxima/checkLatex/checkPandoc (non
// compila nada ela soa, eses avisos non teñen sentido alí).
function arrincarModoXanelas() {
  if (souXeradorIA) {
    entrarModoXeradorIA()
    return
  }
  if (souXanelaResultado) {
    entrarModoResultado()
    return
  }
  checkMaxima()
  checkLatex()
  checkPandoc()
  Events.On('yang:resultado-pechado', saírModoDobreEditor)
  // Xerador con IA desacoplado (xanelaxeradoria.go): inserir no documento o
  // que xerou a outra xanela e recompilar a previsualización.
  Events.On('yang:xerador-ia-exercicio', (e) => aplicarXeracionExercicioIA(e.data))
  Events.On('yang:xerador-ia-exame', (e) => aplicarXeracionExameIA(e.data))
  Events.On('yang:xerador-ia-pechado', () => { xeradorDetachedTracking = { exIndex: null, exameRango: null } })

  // btnDesacoplarResultado (só existe na xanela editor, ver HTML) - fai o
  // camiño de ida do que saírModoDobreEditor fai de volta, dispoñible en
  // calquera momento. Se AbrirXanelaResultado falla, vólvese ao estado
  // anterior en vez de deixar o panel agochado sen xanela.
  btnDesacoplarResultado.classList.remove('hidden')
  btnDesacoplarResultado.addEventListener('click', async () => {
    activarModoDobreEditor()
    try {
      await AbrirXanelaResultado()
    } catch (err) {
      saírModoDobreEditor()
      setStatus(mensaxeErro(err))
    }
  })
}
arrincarModoXanelas()

inicializarEnvioATao().then(() => inicializarReparto())
