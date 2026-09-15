import { describe, it, expect } from 'vitest'
import {
  reorganizarPractica, correccionAModelo, sementesRecomendadas, resumoInforme, ESTADO,
} from './reorganizar.js'
import { documentToText, parseDocument, makeExercise } from './table-serialize.js'

// ---- helpers ----
function docConExercicios(defs) {
  // defs: [{ enunciado, variables }]
  const exs = defs.map((d) => {
    const ex = makeExercise()
    ex.enunciado = d.enunciado
    ex.variables = d.variables || []
    return ex
  })
  return documentToText({ v: 2, exercicios: exs })
}

// probar que sempre di "todo ben"
const probarOK = async ({ nSeeds }) => ({ exercicios: [{ indice: 0, nProbas: nSeeds, diagnosticos: [] }] })
// probar que sempre atopa unha división por cero
const probarFalla = async ({ nSeeds }) => ({
  exercicios: [{ indice: 0, nProbas: nSeeds, diagnosticos: [{ seed: 3, expr: '1/(a-b)', valor: 'inf', problema: 'division_cero' }] }],
})

describe('sementesRecomendadas', () => {
  it('exercicio determinista (sen aleatorias) -> 3', () => {
    const ex = { variables: [{ nome: 'a', modo: 'fixa', valor: '2' }] }
    expect(sementesRecomendadas(ex, 25)).toBe(3)
  })
  it('unha raíz sobe o número', () => {
    const ex = { variables: [{ nome: 'a', modo: 'aleatoria', xer: 'n1' }], resultados: [{ formula: 'sqrt(a-1)' }] }
    expect(sementesRecomendadas(ex, 25)).toBe(40)
  })
  it('xerador de fraccións duplica', () => {
    const ex = { variables: [{ nome: 'a', modo: 'aleatoria', xer: 'q1' }], resultados: [{ formula: 'a+1' }] }
    expect(sementesRecomendadas(ex, 25)).toBe(50)
  })
  it('nunca pasa de 200', () => {
    const ex = {
      variables: Array.from({ length: 6 }, (_, i) => ({ nome: 'v' + i, modo: 'aleatoria', xer: 'q2' })),
      resultados: [{ formula: 'sqrt(v0)/tan(v1) + solve(x=v2,x)' }],
    }
    expect(sementesRecomendadas(ex, 180)).toBe(200)
  })
})

describe('correccionAModelo', () => {
  it('escribe resultados con nomes res_n e reconstrúe a resposta', () => {
    const ex = makeExercise()
    ex.variables = [{ nome: 'base', modo: 'aleatoria', xer: 'n1' }]
    correccionAModelo(ex, {
      resultados: [{ etiqueta: 'Área', formula: 'base^2' }, { etiqueta: 'Perímetro', formula: '4*base' }],
      resolucion: '<p>Área = <EVAL>res_1</EVAL>.</p>',
    })
    expect(ex.resultados).toEqual([
      { etiqueta: 'Área', nome: 'res_1', formula: 'base^2' },
      { etiqueta: 'Perímetro', nome: 'res_2', formula: '4*base' },
    ])
    expect(ex.resposta).toContain('<MAT>base^2</MAT> = {res_1}')
    expect(ex.solucion).toBe('<p>Área = <EVAL>res_1</EVAL>.</p>')
  })

  it('senFormula deixa o exercicio intacto', () => {
    const ex = makeExercise()
    ex.solucion = 'orixinal'
    correccionAModelo(ex, { senFormula: true, motivo: 'demostración' })
    expect(ex.resultados).toEqual([])
    expect(ex.solucion).toBe('orixinal')
  })

  it('se o docente xa ten unha variable res_1, cambia o prefixo e reescribe a resolución', () => {
    const ex = makeExercise()
    ex.variables = [{ nome: 'res_1', modo: 'fixa', valor: '7' }]
    correccionAModelo(ex, {
      resultados: [{ etiqueta: 'x', formula: 'res_1*2' }],
      resolucion: 'o valor é <EVAL>res_1</EVAL>',
    })
    expect(ex.resultados[0].nome).toBe('q_res_1')
    expect(ex.solucion).toBe('o valor é <EVAL>q_res_1</EVAL>')
  })
})

describe('reorganizarPractica', () => {
  it('camiño feliz: xera fórmulas, valida e devólvese un .matex coas res_n no HIDE', async () => {
    const src = docConExercicios([{ enunciado: 'Área do cadrado de lado {l}.', variables: [{ nome: 'l', modo: 'aleatoria', xer: 'n1' }] }])
    const { texto, informe } = await reorganizarPractica(src, {
      xerarCorreccion: async () => ({ resultados: [{ etiqueta: 'Área', formula: 'l^2' }], resolucion: '<p><EVAL>res_1</EVAL></p>' }),
      probar: probarOK,
      sementes: 25, roldas: 3,
    })
    expect(informe[0].estado).toBe(ESTADO.OK)
    expect(texto).toContain('l: n1()$ res_1: l^2$')
    expect(texto).toContain('<RESP>')
    expect(texto.startsWith('<!--yang:tabla:2 ')).toBe(true)
  })

  it('bucle de arranxo: falla a 1ª rolda, pasa a 2ª -> REPARADO', async () => {
    const src = docConExercicios([{ enunciado: 'Calcula {a}/{b}.', variables: [
      { nome: 'a', modo: 'aleatoria', xer: 'n1' }, { nome: 'b', modo: 'aleatoria', xer: 'n1' },
    ] }])
    let rolda = 0
    const { informe } = await reorganizarPractica(src, {
      xerarCorreccion: async (req) => {
        rolda++
        // a 1ª fórmula divide por (a-b) (perigosa); a 2ª xa non
        return rolda === 1
          ? { resultados: [{ etiqueta: 'q', formula: 'a/(a-b)' }], resolucion: '<p><EVAL>res_1</EVAL></p>' }
          : { resultados: [{ etiqueta: 'q', formula: 'a/b' }], resolucion: '<p><EVAL>res_1</EVAL></p>' }
      },
      probar: async ({ nSeeds }) => (rolda >= 2
        ? { exercicios: [{ indice: 0, nProbas: nSeeds, diagnosticos: [] }] }
        : { exercicios: [{ indice: 0, nProbas: nSeeds, diagnosticos: [{ seed: 1, expr: 'a/(a-b)', valor: 'inf', problema: 'division_cero' }] }] }),
      sementes: 10, roldas: 3,
    })
    expect(informe[0].estado).toBe(ESTADO.REPARADO)
    expect(informe[0].roldas).toBe(1) // reparado no primeiro arranxo
  })

  it('esgótanse as roldas -> FALLA co diagnóstico', async () => {
    const src = docConExercicios([{ enunciado: 'Algo con {a}.', variables: [{ nome: 'a', modo: 'aleatoria', xer: 'n1' }] }])
    const { informe } = await reorganizarPractica(src, {
      xerarCorreccion: async () => ({ resultados: [{ etiqueta: 'q', formula: '1/a - 1/a' }], resolucion: 'x' }),
      probar: probarFalla,
      sementes: 5, roldas: 2,
    })
    expect(informe[0].estado).toBe(ESTADO.FALLA)
    expect(informe[0].diagnosticos[0].problema).toBe('division_cero')
  })

  it('exercicio sen fórmula pechada -> SEN_FORMULA e non se toca', async () => {
    const src = docConExercicios([{ enunciado: 'Demostra que a suma de dous pares é par.', variables: [] }])
    const { texto, informe } = await reorganizarPractica(src, {
      xerarCorreccion: async () => ({ senFormula: true, motivo: 'é unha demostración' }),
      probar: probarOK,
    })
    expect(informe[0].estado).toBe(ESTADO.SEN_FORMULA)
    expect(informe[0].motivo).toBe('é unha demostración')
    expect(texto).not.toContain('<RESP>')
  })

  it('soIndice limita o traballo a un exercicio', async () => {
    const src = docConExercicios([
      { enunciado: 'Primeiro {a}.', variables: [{ nome: 'a', modo: 'aleatoria', xer: 'n1' }] },
      { enunciado: 'Segundo {b}.', variables: [{ nome: 'b', modo: 'aleatoria', xer: 'n1' }] },
    ])
    const vistos = []
    const { informe } = await reorganizarPractica(src, {
      xerarCorreccion: async (req) => { vistos.push(req.enunciado); return { resultados: [{ etiqueta: 'q', formula: 'a+1' }], resolucion: 'x' } },
      probar: probarOK,
      soIndice: 1,
    })
    expect(vistos).toHaveLength(1)
    expect(vistos[0]).toContain('Segundo')
    expect(informe[0].estado).toBe(ESTADO.SEN_CAMBIOS)
    expect(informe[1].estado).toBe(ESTADO.OK)
  })

  it('funciona desde texto de Código (legacy, sen comentario incrustado)', async () => {
    const legacy = '<EX><HIDE>a: n1()$</HIDE>Canto é <EVAL>a</EVAL> ao cadrado?</EX>\n'
    const { texto, informe } = await reorganizarPractica(legacy, {
      xerarCorreccion: async () => ({ resultados: [{ etiqueta: 'Cadrado', formula: 'a^2' }], resolucion: '<p><EVAL>res_1</EVAL></p>' }),
      probar: probarOK,
    })
    expect(informe[0].estado).toBe(ESTADO.OK)
    expect(texto).toContain('res_1: a^2$')
    // ao aplicalo, o documento xa "sobe" a formato táboa v2
    expect(parseDocument(texto).model.exercicios[0].resultados[0].formula).toBe('a^2')
  })

  it('sen `probar` só fai a primeira corrección (sen validar)', async () => {
    const src = docConExercicios([{ enunciado: 'Área {l}.', variables: [{ nome: 'l', modo: 'aleatoria', xer: 'n1' }] }])
    const { informe } = await reorganizarPractica(src, {
      xerarCorreccion: async () => ({ resultados: [{ etiqueta: 'A', formula: 'l^2' }], resolucion: 'x' }),
    })
    expect(informe[0].estado).toBe(ESTADO.OK)
  })

  it('resumoInforme reconta por estado', () => {
    const informe = [
      { estado: ESTADO.OK }, { estado: ESTADO.OK }, { estado: ESTADO.REPARADO },
      { estado: ESTADO.FALLA }, { estado: ESTADO.SEN_FORMULA },
    ]
    expect(resumoInforme(informe)).toMatchObject({ ok: 2, reparado: 1, falla: 1, sen_formula: 1, total: 5 })
  })
})
