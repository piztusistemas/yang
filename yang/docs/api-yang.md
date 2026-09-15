# API de Yang — porta de entrada para chamar as súas funcións dende fóra

> Estado: **implementado (v1)**. Aprobado o deseño, os 5 pasos de §10 xa
> están feitos — ver "Nota de implementación" ao final de cada sección onde
> o código final se desviou algo do borrador orixinal.
>
> Onde vive cada peza:
> - `yang/internal/api/` — servidor (socket, token, handlers).
> - `yang/apinucleo.go` — adaptador `nucleoAPI` que implementa `api.Nucleo`
>   sobre `*App`, reutilizando `Generate`/`GeneratePDF`/
>   `generateMarkdownBody`/`exportViaPandoc`/biblioteca tal cal (ver nota en
>   §7.1: non fixo falla ningún paquete `motorxerar` novo, xa estaban
>   suficientemente desacoplados de GUI).
> - `yang/contexto.go`, `yang/apiservidor.go` — lectura de `PIZTU_CONTEXTO`
>   e ciclo de vida (`ServiceStartup`/`ServiceShutdown` en `yang/app.go`).
> - `yang/apiservidor_test.go` — proba de punta a punta (contexto → socket
>   → auth → whoami/biblioteca) sen depender de Maxima/LaTeX/GTK.
> - `yangclient/` — SDK Go independente (módulo propio, `go.mod` á parte),
>   simétrico a `piztuclient/` pero para falar *con* Yang en vez de con
>   Piztu.

## 1. Isto é o inverso do patrón Xesta→Piztu

`docs/api-modulos.md` documenta módulos **clientes** que falan coa API viva
de **Piztu** (Xesta hoxe). Aquí é ao revés: **Yang é o servidor**. Un módulo
calquera (Pancho, un futuro módulo de corrección automática, un script de
terceiros) quere usar as funcións de Yang — xerar un exame con Maxima,
compilalo a PDF, gardalo na biblioteca de exercicios — sen abrir a súa
xanela nin saber nada de `.matex`.

Isto ten unha consecuencia importante no modelo de permisos (§4): Piztu pode
mediar entre módulos porque **el** decide que módulos están activados en
⚙ Aula → Módulos. Yang non ten equivalente — non hai ningún sitio onde o
profesorado "activa" o acceso doutro módulo a Yang. Por iso o modelo de
autenticación aquí é deliberadamente máis simple ca o de Piztu (ver §4).

## 2. Que queda FÓRA do v1 (a propósito)

Igual que a API de Piztu non expón datos de Tao, esta API non expón todo
`App` — só o que ten sentido chamar sen unha xanela diante:

| Queda fóra | Por que |
|---|---|
| Diálogos nativos (`OpenFileDialog`, `SaveFileDialog`, `SaveTexDialog`, `SavePDFDialog`, `ExportHTMLDialog`, `SaveUploadedImage`) | Precisan GTK/GUI local; un módulo remoto non ten xanela que mostrar. |
| Modo dúas xanelas (`AbrirXanelaResultado`, `DockResultado`, `PublicarEstadoResultado`, `GetEstadoResultado`, `AccionResultado*`) | Estado da xanela de resultado da GUI de Yang, non unha función de negocio. |
| `ReverseSearch` | Interactúa co visor de PDF embebido na GUI (clic → liña do editor). |
| Instalador (`CheckLatexDeps`, `InstallMaxima`, `InstallLatex`, `CheckDocDeps`, `InstallPandoc`) | Accións de aprovisionamento da máquina onde corre Yang, non do documento. |
| Actualización (`GetActualizacionYang`, `AplicarActualizacionYang`) | Ciclo de vida do propio proceso Yang. |
| `GetSettings`/`SaveSettings` | Configuración persoal do profesorado (rutas, motor LaTeX, credenciais de IA). As chamadas do API **len** esta configuración para saber como xerar (§6), pero non a expoñen nin permiten cambiala por esta vía. |
| IA (`XerarContidoIA`, `AsistenteIA`, `XerarExercicioIA`, `XerarExameIA`) | Decisión xa tomada: fóra do v1. Consome a clave/cota de IA configurada en Yang e pode tardar bastante; mellor deseñala á parte, con capacidade propia (`ia.executar`), en v1.1. |

O que **si** entra é o núcleo reutilizable: xerar variantes (Maxima), exportar
a PDF/LaTeX/Markdown/DOCX/ODT, e a biblioteca de exercicios. Ver catálogo en
§6.

## 3. Onde vive a API

**Nativa en Go, dentro do proceso Yang** — non un proceso á parte. Un socket
Unix propio, `tmp_dir/yang-api.sock` (mesmo `tmp_dir` que xa coñece Yang
porque llo pasa Piztu en `ContextoModulo`/`PIZTU_CONTEXTO` ao lanzalo — non
hai que inventar onde vive). Arrinca en fondo dende `ServiceStartup` (mesmo
hook que xa carga `a.settings`, ver `yang/app.go`) e pécha o socket ao saír.

**A API SEMPRE arrinca**, con ou sen `PIZTU_CONTEXTO` — cambiou respecto ao
deseño orixinal (ver nota de implementación abaixo) porque o modo dúas
xanelas (§11) tamén pasa por aquí, e ese debe funcionar executando Yang á
man, sen Piztu por medio. Cando hai `PIZTU_CONTEXTO`, o socket vive no
`tmp_dir` compartido de Piztu (descubrible por módulos externos, ver §4).
Sen contexto, vive nun directorio temporal propio desta instancia
(`os.MkdirTemp`), borrado ao pechar Yang — segue funcionando para o uso
interno, simplemente non hai onde un módulo externo o descubra.

> **Nota de implementación** (o borrador orixinal desta sección dicía "sen
> contexto, a API non arrinca"): cando se deseñou o modo dúas xanelas
> tamén detrás desta API (§11), quedou claro que "non arrinca sen Piztu"
> rompería executar Yang á man en desenvolvemento — a xanela de resultado
> deixaría de recibir nada do editor. `yang/apiservidor.go` resolve isto
> con `tmpDirParaAPI()`: tmp_dir de Piztu se hai contexto, un propio
> (`os.MkdirTemp`) senón.

## 4. Descubrimento e autenticación — máis simple ca o de Piztu, a propósito

Piztu emite un token **por módulo**, con permisos que o profesorado concedeu
individualmente (§5 de `docs/api-modulos.md`). Yang non pode facer iso: non
hai xesto de "activar Yang para o módulo X" en ningures. Reproducir ese
modelo aquí (ex.: que cada módulo pida un token e Yang decida que lle
concede) sería seguridade decorativa — calquera proceso local xa pode ler o
ficheiro de descubrimento ou conectar ao socket, así que "decidir" que
capacidades dar a ese token non pecha ningunha porta real sen un mediador
coma Piztu (ver §9, extensión futura rexeitada por agora).

Deseño escollido — **un único token, dúas capas coma sempre**:

1. **Permiso de ficheiro** — socket `chmod 660`; ficheiro de descubrimento
   (abaixo) `chmod 600`, máis estrito porque ese si leva o token en claro.
   Primeira barreira real: só procesos do mesmo usuario poden chegar a el.
2. **Token Bearer**, xerado unha vez por Yang ao arrincar (vive só en
   memoria, coma en Piztu) e publicado nun ficheiro de descubrimento:

```jsonc
// tmp_dir/yang-api.json (0600)
{ "socket": "/opt/piztu/tmp/yang-api.sock", "token": "yg_3fa1...", "api_version": "v1" }
```

Calquera proceso local que poida ler ese ficheiro ten acceso a **todo** o
catálogo v1 — non hai capacidades por chamador neste deseño (a diferenza de
Piztu). As "capacidades" no catálogo (§6) documentan a natureza/risco de
cada grupo de endpoints para cando faga falta diferencialas de verdade
(ver §9), non un control de acceso real hoxe.

Un `X-Modulo-Id` opcional na petición (declarado polo chamador, sen
verificar) só serve para lectura humana na auditoría (§7) — nunca para
autorizar nada.

Reiniciar Yang rota o token (novo valor, ficheiro reescrito); non hai
revogación individual porque non hai identidades individuais que revogar.

## 5. Versionado e formato de erro

Idéntico ao de Piztu, mesmo motivo (consistencia entre as dúas APIs do
proxecto, un módulo que fala coas dúas non aprende dous contratos):

- Ruta base `/api/v1/...`.
- Erro sempre JSON, nunca HTML, mesmo nun panic (middleware `recuperar`):
  `{ "erro": { "codigo": "...", "mensaxe": "...", "detalles": {} } }`
- `400` corpo/parámetros incorrectos, `401` sen token válido, `404` recurso
  non atopado (ex. ID de biblioteca descoñecido), `500` erro interno.
  (Sen `403` — non hai permisos por chamador que negar, ver §4.)

## 6. Catálogo de endpoints (v1)

| Método + ruta | Categoría | Propósito |
|---|---|---|
| `GET /api/v1/whoami` | — | Confirma o token e devolve `{"servizo":"yang","api_version":"v1","version":"26.8"}`. Non hai `modulo_id`: Yang non sabe quen chama, só que o token é válido (ver §4). Primeira chamada recomendada. |
| `POST /api/v1/xerar` | `xerar.previsualizar` | Equivalente a `Generate`: `.matex` → HTML con MathJax, unha variante por `seed+i`. Corpo: `{source, codeIni?, seed, iterations}` (codeIni baleiro = usa o `codeIni` gardado en `a.settings`, coma hoxe). |
| `POST /api/v1/pdf` | `xerar.exportar` | Equivalente a `GeneratePDF`. Corpo: `{source, codeIni?, seed, iterations}` — **sen `baseDir`**: as imaxes (`<IMG>`) referenciadas dende fóra dun ficheiro local aberto non se poden resolver; se o `.matex` non ten imaxes locais, compila igual. Resposta: `{pdfBase64, latexSource, pageImages[], warnings[], log}`, mesma forma que `GeneratePDFResult`. |
| `POST /api/v1/markdown` | `xerar.exportar` | Equivalente a `ExportMarkdown`, pero **devolve o contido**, non escribe ficheiro (ver §7.1). Corpo igual que `/pdf`. Resposta: `{markdown, warnings[]}`. |
| `POST /api/v1/docx` | `xerar.exportar` | Equivalente a `ExportDocx`, contido en vez de path. Resposta: `{docxBase64, warnings[]}`. |
| `POST /api/v1/odt` | `xerar.exportar` | Equivalente a `ExportOdt`. Resposta: `{odtBase64, warnings[]}`. |
| `GET /api/v1/biblioteca` | `biblioteca.ler` | Lista `[]BibliotecaItem` (mesma forma que hoxe: `id, nome, tipo, contido, creado`). |
| `POST /api/v1/biblioteca` | `biblioteca.escribir` | Equivalente a `GardarNaBiblioteca`. Corpo: `{nome, tipo, contido}`. Devolve o `BibliotecaItem` creado. |
| `DELETE /api/v1/biblioteca/{id}` | `biblioteca.escribir` | Equivalente a `EliminarDaBiblioteca`. `404` se o ID non existe. |

**Explicitamente fóra** (ver §2): diálogos, xanela de resultado, reverse
search, instalador, actualización, configuración, IA.

## 7. Notas de deseño que cambian código existente

### 7.1. Exportar por API devolve contido, nunca escribe en disco

`ExportMarkdown`/`ExportDocx`/`ExportOdt` hoxe reciben `defaultName` e
escriben directamente nun path elixido pola GUI (diálogo "gardar"). Un
chamador remoto non ten (nin debe supor) acceso ao sistema de ficheiros de
Yang, así que os handlers da API **non poden chamar estas funcións tal
cal** — precisan a variante núcleo que xa existe por debaixo
(`generateMarkdownBody`, e o equivalente de `exportViaPandoc` escribindo a un
temporal e lendo os bytes de volta) sen o paso final de escritura a un path
escollido polo usuario. Isto é o motivo do refactor a
`yang/internal/motorxerar/` apuntado no cabezallo: illar "xerar contido" de
"gardar nun sitio", para que a GUI siga facendo as dúas cousas e a API só a
primeira.

### 7.2. A API le a configuración persoal de Yang, non ten a súa propia

`maximaPath`, `LatexEngine`, decimais, timeout... veñen de `a.settings`
(Opcións da GUI), igual que para a xeración local. Non hai un `codeIni`,
motor ou timeout "só para a API" — un módulo que chama `/pdf` obtén
exactamente o mesmo resultado que o profesorado tería premendo "Xerar PDF"
coa configuración actual de Yang nesa máquina.

Isto inclúe a **plantilla de documento activa** (a maqueta: cabeceira do
centro, membrete...; ver `plantillas.go`): `/pdf` e `/markdown` saen coa
plantilla que estea escollida na GUI, porque é o mesmo criterio de arriba —
o resultado da API é o que daría o "Xerar" local. `/xerar` (HTML) é a
excepción: as plantillas son LaTeX/Markdown e non teñen equivalente HTML,
así que ese endpoint devolve o documento sen maqueta.

### 7.3. Concorrencia: sen cancelación remota no v1

`Generate`/`GeneratePDF` xa abren unha sesión de Maxima nova por chamada
(`cas.Open`), así que chamadas concorrentes (GUI local + API, ou varias
peticións de API á vez) xa funcionan hoxe sen bloquearse entre si. O que
**non** se resolve neste deseño é `KillMaxima`: rastrexa unha soa sesión
"actual" (`a.maximaSession`) para o botón de cancelar da GUI, e non hai xeito
de dicir "cancela precisamente a miña petición de API" sen un identificador
de traballo por chamada. Por iso `/xerar/cancelar` **non entra no v1** —
pendente para cando faga falta (require pasar de "un punteiro global" a "un
rexistro de sesións por ID", ver nota xa existente en `app.go` sobre por que
`trackMaximaSession` non sobrescribe se xa cambiou).

## 8. SDK — `yangclient/`

Módulo Go independente, mesmo patrón ca `piztuclient/` (§9 de
`docs/api-modulos.md`): `go.mod` propio para que un módulo de terceiros o
poida importar sen arrastrar todo `yang/`.

```go
cli, ok := yangclient.Descubrir() // le PIZTU_CONTEXTO → tmp_dir → tmp_dir/yang-api.json; ok=false se Yang non está a correr
res, err := cli.Xerar(ctx, yangclient.XerarRequest{Source: fonte, Seed: 1, Iterations: 3})
pdf, err := cli.PDF(ctx, yangclient.XerarRequest{Source: fonte})
item, err := cli.GardarNaBiblioteca(ctx, "Tema 3 - derivadas", "exercicio", contido)
```

`Descubrir` reutiliza a mesma variable de contorno `PIZTU_CONTEXTO` que xa lee
`piztuclient.DesdeArranque` (o `tmp_dir` xa vén nese contexto) — non fai
falla ningún mecanismo novo de localización. Módulos noutra linguaxe usan
directamente HTTP+JSON sobre o socket documentado aquí.

## 9. Extensións futuras (fóra do v1, deliberadamente)

- **Piztu de mediador de identidade**: se algún día fai falla diferenciar
  que módulo pode chamar que función de Yang (ex.: un módulo de terceiros
  pouco fiable que só debe poder *ler* a biblioteca, nunca exportar PDFs
  custosos), a peza que falta é que Piztu emita tokens *para Yang* coma xa
  fai para si mesmo, reutilizando ⚙ Aula → Módulos. Rexeitado por agora por
  ser complexidade sen un caso de uso concreto que a xustifique hoxe.
- **IA por API** (`ia.executar`): v1.1, ver §2.
- **`/xerar/cancelar`**: require rexistro de sesións por ID, ver §7.3.
- **Eventos/SSE** (progreso dunha compilación longa): mesmo patrón pendente
  xa apuntado en `docs/api-modulos.md` §7 para Piztu; non prioritario
  mentres as chamadas sexan síncronas e razoablemente curtas.

## 10. Plan de adaptación — feito, con axustes

1. ~~Extraer a `yang/internal/motorxerar/` a lóxica compartida~~ **Non fixo
   falla**: `Generate`/`GeneratePDF` xa devolven contido en memoria
   (nunca escriben a un path elixido pola GUI: iso faino `SavePDFDialog`
   nunha chamada aparte), e `generateMarkdownBody`/`exportViaPandoc` xa
   estaban separados do paso final de escritura. `yang/apinucleo.go` chama
   estas funcións tal cal, envolvendo `exportViaPandoc` cun path nun
   directorio temporal efémero (creado e borrado por chamada) en vez dun
   path elixido polo profesorado — ver `exportarDocAPI`.
2. ~~Implementar `yang/internal/api/`~~ Feito: `api.go` (servidor/socket/
   descubrimento), `auth.go` (token único, `recuperar`), `handlers.go`,
   `erro.go`, `tipos.go` (inclúe a interface `Nucleo`, ver punto seguinte).
3. ~~Arrincar/parar en `ServiceStartup`/shutdown~~ Feito
   (`yang/apiservidor.go`, hooks en `yang/app.go`). Nota de deseño non
   prevista no borrador: como `*App` vive en `package main`, o paquete
   `api` (que non pode importar `main`) define unha interface `Nucleo`; o
   adaptador `nucleoAPI` en `yang/apinucleo.go` é un tipo á parte (non o
   propio `*App`) porque `App` xa ten métodos co mesmo nome
   (`ListarBiblioteca`, etc.) con sinaturas distintas — dous métodos co
   mesmo nome no mesmo tipo non compilaría.
4. ~~Crear `yangclient/`~~ Feito, mesmo patrón ca `piztuclient/`.
5. Módulo de proba/Pancho migrado a `yangclient`: **pendente**, sen
   consumidor real aínda — o único que exerce o contrato hoxe é
   `yang/apiservidor_test.go` (chama HTTP directo sobre o socket, non
   `yangclient`, para non facer que Yang dependa do seu propio SDK cliente
   só para probas).

## 11. Endpoints internos — modo dúas xanelas (editor ↔ xanela de resultado)

Ampliación posterior ao v1 orixinal: o "modo dúas xanelas" de Yang (editor
principal + xanela de resultado desacoplable, `yang/xanelaresultado.go`)
tamén pasa por esta API — non só o catálogo público de §6.

**Por que**: antes desta ampliación, editor e xanela de resultado
comunicaban a través dun campo `App.resultado` compartido en memoria
(mutex), lido/escrito directamente polos bindings de Wails
(`PublicarEstadoResultado`/`GetEstadoResultado`/`AccionResultado*`). Agora
ese estado vive no propio `Servidor` da API
(`yang/internal/api/resultado.go`), e eses bindings seguen existindo coa
mesma sinatura cara ao JS (o frontend non cambiou nada) pero por dentro
fan unha chamada HTTP real ao servidor sobre o socket Unix — o mesmo
camiño que seguiría un módulo externo, só que aquí quen chama é o propio
proceso de Yang (`yang/clienteapi.go`, un cliente HTTP interno, **á parte**
de `yangclient/`: estes endpoints non son un contrato público para
terceiros, só o mecanismo de comunicación interna de Yang).

**Non son parte do catálogo de §6**: mesmo socket, mesmo token (é o mesmo
proceso falando consigo mesmo, non hai "outro módulo" involucrado), pero
non documentados coma API estable para terceiros — poden cambiar de forma
libre coa UI de Yang.

| Método + ruta | Propósito |
|---|---|
| `POST /api/v1/resultado` | Publica o estado dunha xeración (equivalente ao vello `PublicarEstadoResultado`): fonte/seed/iteracións, `nomeBase`, imaxes de páxina, warnings, PDF e TeX xa xerados. Devolve o snapshot resultante. |
| `GET /api/v1/resultado` | Snapshot actual (`version`, `pageImages`, `warnings`, `hasPdf`, `hasTex`) — equivalente a `GetEstadoResultado`. |
| `GET /api/v1/resultado/pdf` | PDF xa xerado (non recompila) + `nomeBase` suxerido — para `AccionResultadoPDF` (que só fai `SavePDFDialog` cos bytes). `400 resultado_sen_pdf` se aínda non se publicou ningún. |
| `GET /api/v1/resultado/tex` | Igual, para `AccionResultadoTex`. |
| `GET /api/v1/resultado/fonte` | Documento fonte + seed/iteracións + `nomeBase` gardados — `AccionResultadoMD/Docx/Odt` recompilan con isto chamando `ExportMarkdown/ExportDocx/ExportOdt` tal cal (diálogo "gardar como" incluído, sen cambios). `400 resultado_sen_fonte` se aínda non hai nada. |

**Consecuencia no arranque** (ver §3): como este mecanismo ten que
funcionar tamén executando Yang á man (sen Piztu), a API xa non depende de
`PIZTU_CONTEXTO` para arrincar — usa un `tmp_dir` propio nese caso.

**O evento Wails segue existindo tal cal**: `PublicarEstadoResultado`
emite `yang:resultado-actualizado` despois de publicar con éxito na API,
igual que antes — a xanela de resultado non tivo que cambiar como escoita
cambios en vivo, só como le o estado inicial e as accións de gardar.

---
*Implementado. Pendente real: punto 5 de §10 (primeiro consumidor externo
de verdade de `yangclient`).*
