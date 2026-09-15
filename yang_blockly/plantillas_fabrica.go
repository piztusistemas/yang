package main

import "time"

// plantillasDeFabrica son as plantillas de exemplo coas que arranca a
// biblioteca (sementadas unha soa vez, ver sementarSeFai). Non son "de só
// lectura": están aí para abrir, ver como se fai unha, e modificalas ou
// duplicalas. Cubren os dous xeitos válidos de escribir unha plantilla:
//
//   - "Exame de instituto" e "Boletín de exercicios" son FRAGMENTOS (sen
//     \documentclass): só cabeceira e pé, o preámbulo pono Yang. É o 90%
//     dos casos e non esixe saber LaTeX máis alá do texto.
//   - "Orzamento" é unha plantilla COMPLETA: trae \documentclass e marxes
//     propias, porque un orzamento non se parece en nada a un exame.
//
// Ningunha referencia unha imaxe: unha \includegraphics{logo.png} sen o
// ficheiro correspondente faría fallar a compilación nada máis escoller a
// plantilla. O logo engádese dende "🖼️ Imaxes" no editor de plantillas, que
// xa amosa o código exacto que hai que pegar.
func plantillasDeFabrica() []PlantillaLatex {
	agora := time.Now().Format(time.RFC3339)
	// IDs fixas e lexibles (non a marca de tempo que usa GardarPlantilla):
	// así unha plantilla de fábrica sementada nun equipo é a mesma que
	// noutro, e un plantillas.json compartido entre dous ordenadores segue
	// apuntando ao mesmo sitio.
	novo := func(id, nome, desc, latex, markdown string, vars []PlantillaVar) PlantillaLatex {
		return PlantillaLatex{
			ID: id, Nome: nome, Descricion: desc,
			Latex: latex, Markdown: markdown, Variables: vars,
			Creado: agora, Modificado: agora, DeFabrica: true,
		}
	}
	return []PlantillaLatex{
		novo("fabrica-exame", "Exame de instituto",
			"Cabeceira co centro, a materia e o curso, liña para o nome do alumnado e data automática.",
			exameLatex, exameMarkdown, []PlantillaVar{
				{Nome: "CENTRO", Etiqueta: "Centro educativo", Valor: "IES"},
				{Nome: "MATERIA", Etiqueta: "Materia", Valor: "Matemáticas"},
				{Nome: "CURSO", Etiqueta: "Curso e grupo", Valor: "1º ESO A"},
				{Nome: "AVALIACION", Etiqueta: "Avaliación / proba", Valor: "1ª avaliación"},
			}),
		novo("fabrica-boletin", "Boletín de exercicios",
			"Máis sobrio ca o exame: título, materia e un pé con espazo para notas. Sen recadro de identificación.",
			boletinLatex, boletinMarkdown, []PlantillaVar{
				{Nome: "MATERIA", Etiqueta: "Materia", Valor: "Matemáticas"},
				{Nome: "TEMA", Etiqueta: "Tema / unidade", Valor: ""},
				{Nome: "PE", Etiqueta: "Texto do pé", Valor: ""},
			}),
		novo("fabrica-orzamento", "Orzamento",
			"Plantilla completa (documentclass propio): membrete da empresa, datos do cliente e pé con condicións. Pensada para o corpo levar unha táboa de conceptos.",
			orzamentoLatex, orzamentoMarkdown, []PlantillaVar{
				{Nome: "EMPRESA", Etiqueta: "Nome da empresa", Valor: ""},
				{Nome: "CIF", Etiqueta: "CIF / NIF", Valor: ""},
				{Nome: "ENDEREZO", Etiqueta: "Enderezo", Valor: ""},
				{Nome: "CONTACTO", Etiqueta: "Teléfono / correo", Valor: ""},
				{Nome: "CLIENTE", Etiqueta: "Cliente", Valor: ""},
				{Nome: "NUMERO", Etiqueta: "Número de orzamento", Valor: ""},
				{Nome: "CONDICIONS", Etiqueta: "Condicións (pé)", Valor: "Orzamento válido durante 30 días. IVE non incluído."},
			}),
	}
}

const exameLatex = `\begin{center}
  {\Large\bfseries {{CENTRO}} }\\[2pt]
  {\large {{MATERIA}} \textendash{} {{CURSO}} }\\[2pt]
  {{AVALIACION}}
\end{center}

\vspace{2mm}
\noindent Nome e apelidos: \hrulefill\hspace{4mm} Data: {{DATA}}
\vspace{2mm}

\hrule
\vspace{6mm}

{{CORPO}}
`

const exameMarkdown = `# {{CENTRO}}

**{{MATERIA}} — {{CURSO}}** · {{AVALIACION}}

Nome e apelidos: ______________________________  Data: {{DATA}}

---

{{CORPO}}
`

const boletinLatex = `\noindent{\large\bfseries {{TITULO}} }\hfill {{MATERIA}}

\noindent\textit{ {{TEMA}} }
\vspace{2mm}
\hrule
\vspace{5mm}

{{CORPO}}

\vfill
\noindent\footnotesize\textit{ {{PE}} }
`

const boletinMarkdown = `# {{TITULO}}

*{{MATERIA}} — {{TEMA}}*

---

{{CORPO}}

---

*{{PE}}*
`

const orzamentoLatex = `\documentclass[11pt]{article}
\usepackage[margin=2.2cm]{geometry}
\usepackage{array}
\pagestyle{empty}

\begin{document}

\begin{minipage}[t]{0.6\textwidth}
  {\LARGE\bfseries {{EMPRESA}} }\\[4pt]
  {{ENDEREZO}}\\
  CIF: {{CIF}}\\
  {{CONTACTO}}
\end{minipage}\hfill
\begin{minipage}[t]{0.35\textwidth}
  \raggedleft
  {\large\bfseries Orzamento {{NUMERO}} }\\[4pt]
  Data: {{DATA}}
\end{minipage}

\vspace{6mm}
\hrule
\vspace{6mm}

\noindent\textbf{Cliente:} {{CLIENTE}}

\vspace{6mm}

{{CORPO}}

\vspace{10mm}
\hrule
\vspace{2mm}
\noindent\footnotesize {{CONDICIONS}}

\end{document}
`

const orzamentoMarkdown = `# {{EMPRESA}}

{{ENDEREZO}} · CIF: {{CIF}} · {{CONTACTO}}

**Orzamento {{NUMERO}}** — {{DATA}}

**Cliente:** {{CLIENTE}}

---

{{CORPO}}

---

{{CONDICIONS}}
`
