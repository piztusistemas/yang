package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Asistente de creación de PLANTILLAS (ver plantillas.go). Tres fontes, que
// se poden combinar na mesma petición porque acaban todas no mesmo sitio -
// texto que se lle dá á IA:
//
//   - unha descrición en linguaxe natural ("orzamento cunha táboa de
//     conceptos, IVE e total"),
//   - un ou varios documentos de mostra (PDF, .tex, .md, .txt),
//   - unha URL (páxina do centro, modelo publicado...).
//
// A IA devolve SEMPRE un obxecto JSON coa plantilla xa lista para gardar. O
// resultado nunca se garda só: o frontend ábreo no editor de plantillas
// para revisalo, igual ca o resto de funcións de IA de Yang, que nunca
// substitúen nada sen que o profesorado o vexa antes.

// DocumentoMostra é un ficheiro que o profesorado achega coma modelo. Mesmo
// par (nome, base64) ca ModeloPDF (pdftext.go), pero aquí admítense tamén
// ficheiros de texto (.tex/.md/.txt/.html): unha plantilla adoita nacer dun
// .tex que xa se usaba, non só dun PDF impreso.
type DocumentoMostra struct {
	FileName string `json:"fileName"`
	DataB64  string `json:"dataB64"`
}

// PlantillaIARequest é o que manda o asistente do editor de plantillas.
type PlantillaIARequest struct {
	Descricion string            `json:"descricion"`
	URL        string            `json:"url"`
	Documentos []DocumentoMostra `json:"documentos"`
	// Imaxes son os nomes de ficheiro das imaxes xa subidas á plantilla, para
	// que a IA poida usalas (\includegraphics{logo.png}) en vez de inventar
	// rutas que non existen.
	Imaxes []string `json:"imaxes"`
	// Base é a plantilla actual cando se pide un CAMBIO sobre ela ("engade
	// un pé coa data") en vez de unha nova dende cero.
	Base *PlantillaLatex `json:"base"`
}

const limiteTextoFonte = 20000 // runas por documento/URL, mesmo criterio ca limiteTextoModeloPDF

// XerarPlantillaIA constrúe a petición coas fontes dispoñibles e devolve a
// plantilla proposta, SEN gardala.
func (a *App) XerarPlantillaIA(req PlantillaIARequest) (PlantillaLatex, error) {
	var partes []string
	if d := strings.TrimSpace(req.Descricion); d != "" {
		partes = append(partes, "DESCRICIÓN DO QUE SE QUERE:\n"+d)
	}
	if len(req.Imaxes) > 0 {
		partes = append(partes, "IMAXES XA DISPOÑIBLES NA PLANTILLA (úsaas con \\includegraphics{nome}): "+strings.Join(req.Imaxes, ", "))
	}
	if req.Base != nil && strings.TrimSpace(req.Base.Latex) != "" {
		partes = append(partes, "PLANTILLA ACTUAL (modifícaa, non empeces de cero):\n"+req.Base.Latex)
	}
	if texto, err := textoDeDocumentosMostra(req.Documentos); err != nil {
		return PlantillaLatex{}, err
	} else if texto != "" {
		partes = append(partes, "DOCUMENTO(S) DE MOSTRA:\n"+texto)
	}
	if u := strings.TrimSpace(req.URL); u != "" {
		texto, err := descargarTextoURL(u)
		if err != nil {
			return PlantillaLatex{}, err
		}
		partes = append(partes, "CONTIDO DA URL "+u+":\n"+texto)
	}
	if len(partes) == 0 {
		return PlantillaLatex{}, fmt.Errorf("describe a plantilla, achega un documento de mostra ou indica unha URL")
	}

	raw, err := chamarIA(a.settings, systemPromptPlantilla, strings.Join(partes, "\n\n---\n\n"))
	if err != nil {
		return PlantillaLatex{}, err
	}
	return parsearPlantillaIA(raw)
}

// parsearPlantillaIA le o obxecto JSON da resposta e valida o mínimo. Á
// parte de XerarPlantillaIA para poder probalo sen chamar a ningún
// provedor.
func parsearPlantillaIA(raw string) (PlantillaLatex, error) {
	var resposta struct {
		Nome       string `json:"nome"`
		Descricion string `json:"descricion"`
		Latex      string `json:"latex"`
		Markdown   string `json:"markdown"`
		Motor      string `json:"motor"`
		Variables  []struct {
			Nome     textoIA `json:"nome"`
			Etiqueta textoIA `json:"etiqueta"`
			Valor    textoIA `json:"valor"` // a IA manda moitas veces un número
		} `json:"variables"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(limparValadosMarkdown(raw))), &resposta); err != nil {
		return PlantillaLatex{}, formatoInesperadoErr(raw, err)
	}
	if strings.TrimSpace(resposta.Latex) == "" {
		return PlantillaLatex{}, fmt.Errorf("a IA non devolveu ningún código LaTeX")
	}
	resposta.Latex = repararBarrasLatex(resposta.Latex)
	if !strings.Contains(resposta.Latex, marcadorCorpo) {
		// Recuperable: sen o marcador a plantilla non se pode aplicar, pero
		// tirar todo o traballo sería peor - engádese ao final, que é onde
		// vai o corpo na inmensa maioría das maquetas (cabeceira + corpo).
		resposta.Latex = engadirCorpoAoFinal(resposta.Latex)
	}
	if strings.TrimSpace(resposta.Markdown) != "" && !strings.Contains(resposta.Markdown, marcadorCorpo) {
		resposta.Markdown = strings.TrimRight(resposta.Markdown, "\n") + "\n\n" + marcadorCorpo + "\n"
	}
	p := PlantillaLatex{
		Nome:       strings.TrimSpace(resposta.Nome),
		Descricion: strings.TrimSpace(resposta.Descricion),
		Latex:      resposta.Latex,
		Markdown:   resposta.Markdown,
		Motor:      strings.TrimSpace(resposta.Motor),
	}
	// Unha plantilla con fontspec SÓ compila con xelatex/lualatex (con
	// pdflatex dá "The fontspec package requires either XeTeX or LuaTeX").
	// A IA adoita esquecer marcalo no campo "motor", e daquela o
	// profesorado que teña pdflatex en Opcións vería fallar a plantilla sen
	// saber por que - así que se deduce do propio código.
	if p.Motor == "" && precisaMotorUnicode(p.Latex) {
		p.Motor = "xelatex"
	}
	if p.Nome == "" {
		p.Nome = "Plantilla nova"
	}
	for _, v := range resposta.Variables {
		p.Variables = append(p.Variables, PlantillaVar{Nome: v.Nome.String(), Etiqueta: v.Etiqueta.String(), Valor: v.Valor.String()})
	}
	p.Variables = normalizarVariables(p.Variables)
	return p, nil
}

// LaTeX dentro de JSON é a fonte número un de plantillas rotas: un salto de
// liña de LaTeX (\\) hai que escribilo "\\\\" no JSON, e os modelos
// sub-escápano constantemente - o JSON decodifica ben, pero o LaTeX que sae
// leva unha soa barra. Comprobado nunha plantilla real xerada dende unha
// URL: "\\[0.2em]" chegou coma "\[0.2em]", e \[ abre modo matemático de
// display, así que pdflatex morría con "Missing \endgroup inserted" sen que
// o profesorado tivese modo ningún de adiviñar por que.
//
// Reparanse SÓ os dous casos onde unha barra soa non pode ser LaTeX válido:
//
//	\[<lonxitude>]  - un \[ de display pecha con \], nunca cun ] só, e
//	                  moito menos levando dentro "0.2em"/"4pt"/"1cm".
//	\ ao final de liña - unha barra escapando un salto de liña non
//	                  significa nada en LaTeX; o que se quería era \\.
//
// Calquera outra barra déixase tal cal: adiviñar máis sería estragar código
// correcto (\textbf, \begin...).
var barraLonxitudeRe = regexp.MustCompile(`([^\\])\\\[(-?[0-9.]+ ?(?:em|ex|pt|mm|cm|in|bp|dd|pc|sp|\\baselineskip))\]`)
var barraFinDeLiñaRe = regexp.MustCompile(`([^\\])\\\n`)

func repararBarrasLatex(latex string) string {
	latex = barraLonxitudeRe.ReplaceAllString(latex, `$1\\[$2]`)
	return barraFinDeLiñaRe.ReplaceAllString(latex, "$1\\\\\n")
}

// precisaMotorUnicode di se o LaTeX usa algo que só entenden xelatex/
// lualatex - hoxe, fontspec e as súas ordes de fonte.
func precisaMotorUnicode(latex string) bool {
	for _, marca := range []string{"fontspec", "\\setmainfont", "\\setsansfont", "\\setmonofont"} {
		if strings.Contains(latex, marca) {
			return true
		}
	}
	return false
}

// engadirCorpoAoFinal mete {{CORPO}} xusto antes do \end{document} (ou ao
// final de todo se non hai), para arranxar unha resposta da IA que esqueceu
// o marcador.
func engadirCorpoAoFinal(latex string) string {
	if i := strings.LastIndex(latex, `\end{document}`); i >= 0 {
		return latex[:i] + "\n" + marcadorCorpo + "\n\n" + latex[i:]
	}
	return strings.TrimRight(latex, "\n") + "\n\n" + marcadorCorpo + "\n"
}

// textoDeDocumentosMostra extrae o texto de cada ficheiro achegado: os PDF
// pasan por pdftotext (o mesmo camiño ca os modelos de exame, pdftext.go) e
// o resto trátase coma texto plano (un .tex/.md/.txt xa é a mellor fonte
// posible para copiar unha maqueta). Un binario que non sexa PDF rexéitase
// en vez de meterlle lixo á IA.
func textoDeDocumentosMostra(docs []DocumentoMostra) (string, error) {
	var partes []string
	for _, d := range docs {
		nome := strings.TrimSpace(d.FileName)
		if nome == "" {
			nome = "documento"
		}
		var texto string
		if strings.HasSuffix(strings.ToLower(nome), ".pdf") {
			t, err := extraerTextoPDF(d.DataB64)
			if err != nil {
				return "", fmt.Errorf("lendo %q: %w", nome, err)
			}
			texto = t
		} else {
			data, err := base64.StdEncoding.DecodeString(d.DataB64)
			if err != nil {
				return "", fmt.Errorf("%q non se puido ler: %w", nome, err)
			}
			if !utf8.Valid(data) {
				return "", fmt.Errorf("%q non é texto nin PDF: usa un .pdf, .tex, .md ou .txt", nome)
			}
			texto = string(data)
		}
		if texto = strings.TrimSpace(texto); texto == "" {
			continue
		}
		partes = append(partes, "["+nome+"]\n"+truncarRunas(texto, limiteTextoFonte))
	}
	return strings.Join(partes, "\n\n"), nil
}

// descargarTextoURL baixa unha páxina e devolve o seu texto visible. Só
// http/https, cun tempo máximo e un tope de tamaño: isto execútase co
// enderezo que escriba o profesorado, e unha descarga eterna (ou un
// ficheiro de 2 GB) deixaría a xanela colgada sen explicación.
func descargarTextoURL(enderezo string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(enderezo))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("a URL ten que empezar por http:// ou https://")
	}
	cliente := &http.Client{Timeout: 20 * time.Second}
	resp, err := cliente.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("non se puido descargar %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s respondeu %s", u, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	tipo := resp.Header.Get("Content-Type")
	if strings.Contains(tipo, "pdf") {
		return truncarRunas(strings.TrimSpace(mustExtraerTextoPDF(data)), limiteTextoFonte), nil
	}
	texto := textoVisibleHTML(string(data))
	if strings.TrimSpace(texto) == "" {
		return "", fmt.Errorf("%s non tiña texto que ler", u)
	}
	return truncarRunas(texto, limiteTextoFonte), nil
}

// mustExtraerTextoPDF reutiliza extraerTextoPDF (que traballa en base64,
// porque o resto de Yang recibe os ficheiros así dende o frontend) para uns
// bytes que xa temos na man. Un erro devolve "" a propósito: quen chama xa
// trata o texto baleiro coma "esta URL non serve".
func mustExtraerTextoPDF(data []byte) string {
	texto, err := extraerTextoPDF(base64.StdEncoding.EncodeToString(data))
	if err != nil {
		return ""
	}
	return texto
}

// textoVisibleHTML tira as etiquetas dunha páxina quedando co texto que se
// ve (sen <script>/<style>), que é dabondo para que a IA capte a estrutura
// e o texto fixo dun modelo publicado na web.
func textoVisibleHTML(fonte string) string {
	doc, err := html.Parse(strings.NewReader(fonte))
	if err != nil {
		return fonte
	}
	var b strings.Builder
	var percorrer func(*html.Node)
	percorrer = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "noscript") {
			return
		}
		if n.Type == html.TextNode {
			if t := strings.TrimSpace(n.Data); t != "" {
				b.WriteString(t)
				b.WriteString("\n")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			percorrer(c)
		}
	}
	percorrer(doc)
	return b.String()
}

// extractJSONObject é o equivalente de extractJSONArray (app.go) para unha
// resposta que ten que ser un OBXECTO: as IAs adoitan envolver o JSON nunha
// frase ("Aquí tes a plantilla: {...} Espero que...") que json.Unmarshal
// rexeita aínda que o JSON en si estea perfecto.
func extractJSONObject(raw string) string {
	inicio := strings.IndexByte(raw, '{')
	if inicio == -1 {
		return raw
	}
	profundidade := 0
	enCadea := false
	escapado := false
	for i := inicio; i < len(raw); i++ {
		c := raw[i]
		if enCadea {
			switch {
			case escapado:
				escapado = false
			case c == '\\':
				escapado = true
			case c == '"':
				enCadea = false
			}
			continue
		}
		switch c {
		case '"':
			enCadea = true
		case '{':
			profundidade++
		case '}':
			profundidade--
			if profundidade == 0 {
				return raw[inicio : i+1]
			}
		}
	}
	return raw
}
