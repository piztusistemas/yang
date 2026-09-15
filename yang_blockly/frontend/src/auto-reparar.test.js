import { describe, it, expect } from 'vitest'
import { autorepararExame, resumoInforme, ESTADO_REP } from './auto-reparar.js'
import { documentToText, parseDocument, makeExercise } from './table-serialize.js'

function doc(enunciados) {
  const exs = enunciados.map((en) => {
    const ex = makeExercise()
    ex.enunciado = en
    return ex
  })
  return documentToText({ v: 2, exercicios: exs })
}

describe('autorepararExame', () => {
  it('non fai nada se todo compila', async () => {
    const diagnosticar = async () => [
      { indice: 0, ok: true, erro: '' },
      { indice: 1, ok: true, erro: '' },
    ]
    const rexenerar = async () => { throw new Error('non se debería chamar') }
    const r = await autorepararExame(doc(['A', 'B']), { diagnosticar, rexenerar })
    expect(r.cambiou).toBe(false)
    expect(resumoInforme(r.informe)).toMatchObject({ ok: 2, reparado: 0, falla: 0 })
  })

  it('repara o exercicio que falla e deixa os demais', async () => {
    let ronda = 0
    const diagnosticar = async () => {
      ronda++
      // 1ª volta: falla o exercicio 1; a partir da 2ª (tras rexenerar) xa compila.
      return [
        { indice: 0, ok: true, erro: '' },
        { indice: 1, ok: ronda > 1, erro: ronda > 1 ? '' : '! Missing $ inserted' },
        { indice: 2, ok: true, erro: '' },
      ]
    }
    const chamadas = []
    const rexenerar = async ({ exercicio, erro, intento }) => {
      chamadas.push({ enun: exercicio.enunciado, erro, intento })
      return { enunciado: 'B arranxado', variables: [], resultado: '', resolucion: '' }
    }

    const r = await autorepararExame(doc(['A', 'B', 'C']), { diagnosticar, rexenerar, maxRechamadas: 2 })

    expect(chamadas).toHaveLength(1)
    expect(chamadas[0]).toMatchObject({ enun: 'B', erro: '! Missing $ inserted', intento: 1 })
    expect(r.cambiou).toBe(true)
    expect(r.texto).toContain('B arranxado')
    expect(r.texto).toContain('<EX>') // segue sendo un documento de táboa
    const inf = r.informe
    expect(inf[0].estado).toBe(ESTADO_REP.OK)
    expect(inf[1].estado).toBe(ESTADO_REP.REPARADO)
    expect(inf[1].intentos).toBe(1)
    expect(inf[2].estado).toBe(ESTADO_REP.OK)
  })

  it('marca FALLA e pon unha caixa de aviso cando se esgotan as roldas', async () => {
    const diagnosticar = async () => [
      { indice: 0, ok: false, erro: '! Undefined control sequence' },
    ]
    let n = 0
    const rexenerar = async () => { n++; return { enunciado: 'segue mal', variables: [] } }

    const r = await autorepararExame(doc(['X orixinal']), { diagnosticar, rexenerar, maxRechamadas: 2 })

    expect(n).toBe(2) // 2 rechamadas antes de rendirse
    expect(r.informe[0].estado).toBe(ESTADO_REP.FALLA)
    expect(r.informe[0].erro).toContain('Undefined control sequence')
    // o exame reescríbese cunha caixa de aviso que SI compila, co orixinal escapado
    expect(r.cambiou).toBe(true)
    expect(r.texto).toContain('non se puido xerar unha versión que compile')
    expect(r.texto).toContain('X orixinal')
  })

  it('maxRechamadas 0: diagnostica, non chama á IA, e marca o fallo', async () => {
    const diagnosticar = async () => [{ indice: 0, ok: false, erro: 'erro' }]
    const rexenerar = async () => { throw new Error('non se debe chamar con maxRechamadas 0') }
    const r = await autorepararExame(doc(['X']), { diagnosticar, rexenerar, maxRechamadas: 0 })
    expect(r.informe[0].estado).toBe(ESTADO_REP.FALLA)
    expect(r.texto).toContain('non se puido xerar')
  })

  it('non rediagnostica os exercicios xa confirmados OK nas roldas seguintes', async () => {
    // Regresión: antes diagnosticábase SEMPRE o exame ENTEIRO en cada
    // rolda, recompilando de balde (Maxima+LaTeX) exercicios que xa se
    // sabía que ían ben - a fonte principal da lentitude percibida en
    // exames longos con só un exercicio a fallar.
    const tamanosVistos = []
    let ronda = 0
    const diagnosticar = async (texto) => {
      ronda++
      const { model } = parseDocument(texto)
      tamanosVistos.push(model.exercicios.length)
      // Falla "B" (busca por contido, non por posición: na 2ª rolda B
      // chega SOLO, xa non na posición 1) só na 1ª chamada.
      return model.exercicios.map((ex, i) => {
        const falla = ex.enunciado === 'B' && ronda === 1
        return { indice: i, ok: !falla, erro: falla ? '! erro en B' : '' }
      })
    }
    const rexenerar = async () => ({ enunciado: 'B arranxado', variables: [], resultado: '', resolucion: '' })

    const r = await autorepararExame(doc(['A', 'B', 'C']), { diagnosticar, rexenerar, maxRechamadas: 2 })

    // 1ª rolda: os 3 exercicios (aínda non se sabe nada). 2ª rolda: SÓ o 1
    // que fallou (B) - non os 3 outra vez.
    expect(tamanosVistos).toEqual([3, 1])
    expect(r.informe[1].estado).toBe(ESTADO_REP.REPARADO)
    expect(r.texto).toContain('B arranxado')
  })

  it('unha rexeneración que peta non impide reparar os demais', async () => {
    let bFixed = false
    const diagnosticar = async () => [
      { indice: 0, ok: false, erro: '! erro en A' },
      { indice: 1, ok: bFixed, erro: bFixed ? '' : '! erro en B' },
    ]
    const rexenerar = async ({ exercicio }) => {
      if (exercicio.enunciado === 'A') throw new Error('IA caída')
      bFixed = true
      return { enunciado: 'B ok', variables: [] }
    }
    const r = await autorepararExame(doc(['A', 'B']), { diagnosticar, rexenerar, maxRechamadas: 1 })
    expect(r.informe[1].estado).toBe(ESTADO_REP.REPARADO)
    expect(r.informe[0].estado).toBe(ESTADO_REP.FALLA)
    expect(r.texto).toContain('B ok')
  })
})
