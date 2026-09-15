# Interface de táboa (folla de cálculo)

Yang ten dúas interfaces de edición para a parte esquerda da pantalla:

| Interface | Para que | Como activala |
|---|---|---|
| **Táboa** (por defecto) | Personalizar rápido enunciados, variables e correccións, coma nunha folla de cálculo | Opcións → *Interface de edición* → «Táboa», ou o botón da barra de ferramentas |
| **Bloques** (Blockly) | Compoñer visualmente arrastrando pezas | Opcións → «Bloques», ou o mesmo botón |

As dúas editan o **mesmo** ficheiro `.matex`; só cambia a forma de velo. Podes
alternar cando queiras sen perder nada.

## A táboa

Cada fila é un **exercicio**. As columnas:

- **Nº** — arrástraa para reordenar. `↑ ↓ ⧉ ✕` á dereita: subir, baixar,
  duplicar, eliminar.
- **Enunciado** — o texto do exercicio. Escribe `{nome}` onde queiras que
  apareza o valor dunha variable (ex.: *«Calcula a área de base {b} e altura
  {h}»*). Os botóns `{b}` `{h}` debaixo insírense no cursor.
- **Variables** — a lista de variables que usa o exercicio. Cada unha ten:
  - **nome** — como a chamas en `{nome}` e nas fórmulas.
  - **modo**:
    - **fixa** — un valor que non cambia (ex.: `9.8`).
    - **aleatoria** — un valor distinto en cada copia/semente. Escolle un
      xerador (*natural pequeno/mediano/grande*, *enteiro*, *fracción*) ou
      **de X a Y** cun paso (ex.: de 4 a 20 de 2 en 2).
    - **automática** — unha expresión calculada a partir doutras variables
      (ex.: `base*altura/2`). Nunca se sortea; recalcúlase soa.
- **Resultado final** / **Resolución** — só aparecen nas lapelas correspondentes
  (ver abaixo). Admiten `{nome}` igual có enunciado.

Botóns da barra: **＋ exercicio**, **✨ Exercicio con IA** / **✨ Exame
completo con IA** (a IA enche enunciado, variables, resultado e resolución),
**Asistente** (IA) e **Ver código** (o `.matex` que produce a táboa, só
lectura).

## As lapelas (visualización)

A lapela activa decide **o que se xera á dereita** (regra estrita: cada lapela
amosa unha soa cousa ademais do enunciado):

| Lapela | Á dereita xérase |
|---|---|
| **Enunciados** | SÓ os enunciados (folla para o alumnado, sen nada de corrección) |
| **Resultado** | Os enunciados **e** o resultado final de cada pregunta (columna `Resultado final`) |
| **Resolución** | Os enunciados **e** a resolución paso a paso ata o resultado (columna `Resolución`; o `Resultado final` NON se repite á parte) |
| **Test** | *(non xera folla)* comproba que os resultados son coherentes |

Para entregar a folla resolta: pon a lapela en «Resolución» e exporta o
PDF normalmente.

### Lapela «Test»

Executa cada exercicio en varias sementes (25 por defecto) e avalía **todas**
as expresións (`<EVAL>`, e as de dentro de `<RESP>`/`<SOL>`). Marca en ámbar/
vermello os exercicios onde apareza algún:

`infinito` · `indeterminación` · `NaN` · `división por cero` · `erro de
Maxima` · `resultado baleiro` · `resultado complexo` · `tempo esgotado`

Para cada problema amosa a semente, a expresión e o valor que fallou, para
poder reproducilo. É a comprobación recomendada antes de repartir un exame.

## Reorganizar práctica (Asistente)

O botón **«⟳ Reorganizar práctica»** vive na xanela do **Asistente** (icona
🤖), tanto na interface de Táboa coma na de Código. Serve para, partindo
dunha práctica **que xa ten os enunciados e as variables**, xerar de forma
**fiable** o resultado final e a resolución paso a paso:

1. Os **enunciados non se tocan**.
2. Por cada exercicio, a IA devolve o resultado final coma **FÓRMULAS**
   Maxima (`base*altura/2`), nunca coma un número escrito. Cada apartado
   pasa a ser unha variable `auto` do `<HIDE>` chamada `res_1`, `res_2`…
3. `<RESP>` e `<SOL>` **referencian esas mesmas** `res_1`, `res_2`… → é
   Maxima quen calcula os valores, así que a resposta e a resolución **non
   poden discrepar**.
4. Todo se **valida en varias sementes** (o mesmo motor da lapela «Test»).
   Se aparece un `infinito` / `indeterminación` / `división por cero` /
   `erro de Maxima`, os diagnósticos devólvense á IA para que corrixa a
   fórmula, ata un número de **roldas de arranxo** configurable.
5. Ao rematar amósase un informe por exercicio (*correcto* / *reparado en N
   roldas* / *segue fallando* / *sen fórmula pechada*) e un só botón
   **«Aplicar»** — nada se substitúe sen ese clic.

Un exercicio **sen resultado calculable** (unha demostración, un «explica
por que», unha construción) márcase como *sen fórmula pechada* e **déixase
intacto** para que o resolvas ti.

**Configuración** (Opcións → *Reorganizar práctica*): as *sementes de
validación* por exercicio (por defecto 25) e as *roldas de arranxo*
automático (por defecto 3). Na propia xanela do Asistente pódense axustar
para esa execución, e a opción **«adaptar ao exercicio»** sobe as sementes
automaticamente nos exercicios máis delicados (raíces, logaritmos,
denominadores cunha variable, `solve`, moitas variables aleatorias…) e
báixaas a 3 nos deterministas. Desde o asistente dun exercicio concreto
(icona 🤖 da súa fila) reorganízase **só ese exercicio**.

## Como se garda (detalle técnico)

O `.matex` que produce a táboa leva, na primeira liña, un comentario con
todo o modelo estruturado en JSON:

```
<!--yang:tabla:2 <base64(JSON)> -->
<EX>
<HIDE>base: (4) + random(17)$ altura: n1()$ res_1: base*altura/2$</HIDE>
Calcula a área … de base <EVAL>base</EVAL> e altura <EVAL>altura</EVAL>.
<RESP><p><b>Área:</b> <MAT>base*altura/2</MAT> = <EVAL>res_1</EVAL> cm²</p></RESP>
<SOL>Área = base·altura/2 = <EVAL>base</EVAL>·<EVAL>altura</EVAL>/2 = <EVAL>res_1</EVAL>.</SOL>
</EX>
```

O bloque `yang:tabla:2` engade a cada exercicio unha lista **`resultados`**:
`[{etiqueta, nome, formula}]` — a corrección estruturada que xera
«Reorganizar práctica». As fórmulas emítense no `<HIDE>` como `res_1: …$`
(despois das variables do docente) e son a **fonte única** do resultado: se
`resposta` está baleira, o `<RESP>` constrúese só a partir delas. Un
comentario `yang:tabla:1` antigo (sen `resultados`) ábrese igual — só se lle
engade unha lista baleira; en canto se reorganiza ou se garda, pasa a
`yang:tabla:2`.

O comentario é **invisible** en todas as saídas (previsualización, PDF,
Word/OpenDocument, Markdown) — só o usa a táboa para reconstruírse ao
reabrir. O corpo `<EX>…</EX>` é `.matex` normal: compila igual aínda que
abras o ficheiro en modo Bloques ou o edites a man.

Un `.matex` **sen** ese comentario (feito antes, ou en Bloques) tamén se
pode abrir na táboa: intenta repartilo en filas o mellor que pode; en canto
o editas, pasa a gardar o comentario.

As etiquetas `<RESP>` e `<SOL>` só se renderizan na lapela que corresponde
(ver `cas.Maxima.RenderMode` en `cas/maxima.go` e `GenerateRequest.Modo` en
`app.go`); en modo «Enunciados» descártanse sen tocar Maxima.
