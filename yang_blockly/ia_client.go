package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Cliente de IA xenérico: fala con calquera provedor que use un destes tres
// formatos de fío (case a inmensa maioría do mercado):
//   - "gemini": API nativa de Google Gemini (systemInstruction + contents).
//   - "anthropic": API nativa de Claude (x-api-key + system + messages).
//   - "openai": formato "chat completions" de OpenAI - o que tamén falan de
//     fábrica moitos outros provedores en modo "compatible con OpenAI"
//     (Qwen/DashScope, Perplexity, Groq, Mistral, DeepSeek, Ollama en
//     local...), así que este mesmo camiño cobre ChatGPT E calquera destes
//     só cambiando o enderezo do servidor (ver baseURLPorDefecto/Settings.
//     IABaseURL) - por iso "xenérico/universal": non hai que escribir un
//     adaptador novo por cada marca, só para os poucos formatos de fío
//     realmente distintos que existen.
//
// O profesorado escolle un provedor por nome nun selector (Opcións, ver
// frontend/index.html) que xa pon o formato de fío e o enderezo por
// defecto correctos - nunca ten que sabelo nin escribilo á man.
type ProvedorIA string

const (
	ProvedorGemini    ProvedorIA = "gemini"
	ProvedorAnthropic ProvedorIA = "anthropic"
	ProvedorOpenAI    ProvedorIA = "openai" // tamén calquera "compatible con OpenAI"
)

// iaHTTPClient é o cliente por defecto (fallback dos tests e de calquera
// chamada sen cliente propio). chamarIA constrúe un cliente por chamada co
// timeout de Settings.IATimeoutResolto (Opcións).
var iaHTTPClient = &http.Client{Timeout: 90 * time.Second}

// baseURLPorDefecto devolve o enderezo oficial do provedor cando
// Settings.IABaseURL queda baleiro - só fai falla escribir un enderezo á
// man para un provedor "compatible con OpenAI" que non sexa a propia
// OpenAI (Qwen, Perplexity...), e aínda así o selector do frontend xa trae
// eses enderezos coma preset.
func baseURLPorDefecto(provedor ProvedorIA) string {
	switch provedor {
	case ProvedorGemini:
		return "https://generativelanguage.googleapis.com/v1beta"
	case ProvedorAnthropic:
		return "https://api.anthropic.com/v1"
	default:
		return "https://api.openai.com/v1"
	}
}

// chamarIA é o punto de entrada único que usan XerarContidoIA/
// XerarExercicioIA/XerarExameIA (app.go): decide o formato de fío polo
// provedor configurado e devolve o texto de resposta xa limpo de valados
// Markdown. `s` é a configuración completa (non só as claves) para que
// engadir un catro provedor no futuro só toque este switch, non cada
// chamador.
func chamarIA(s Settings, sistema, peticion string) (string, error) {
	if s.IAAPIKey == "" {
		return "", fmt.Errorf("falta a clave da API de IA (configúraa en Opcións)")
	}
	if s.IAModel == "" {
		return "", fmt.Errorf("falta o modelo de IA (configúrao en Opcións)")
	}

	provedor := ProvedorIA(s.IAProvedor)
	if provedor == "" {
		provedor = ProvedorGemini // configuracións anteriores a esta versión (só Gemini)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(s.IABaseURL), "/")
	if baseURL == "" {
		baseURL = baseURLPorDefecto(provedor)
	}

	// IAMaxTokens (Opcións): límite de tokens de resposta configurable pola
	// usuaria - 0/sen configurar deixa cada provedor no seu comportamento de
	// sempre (ver Settings.IAMaxTokens/IAMaxTokensAnthropicResolto,
	// settings.go). Anthropic esixe sempre un número en max_tokens
	// (herdando os 8192 antes fixos coma valor por defecto); Gemini/
	// "compatible con OpenAI" simplemente OMITEN o parámetro cando é 0,
	// coma antes desta opción existir.
	// Timeout por chamada (Opcións -> "Tempo máximo de espera pola IA").
	cliente := &http.Client{Timeout: s.IATimeoutResolto()}

	switch provedor {
	case ProvedorAnthropic:
		return chamarAnthropic(baseURL, s.IAAPIKey, s.IAModel, sistema, peticion, s.IAMaxTokensAnthropicResolto(), cliente)
	case ProvedorGemini:
		return chamarGemini(baseURL, s.IAAPIKey, s.IAModel, sistema, peticion, s.IAMaxTokensResolto(), cliente)
	default:
		return chamarOpenAICompatible(baseURL, s.IAAPIKey, s.IAModel, sistema, peticion, s.IAMaxTokensResolto(), cliente)
	}
}

// maxIntentosJSON: cantas veces se chama á IA (en total, non "extra") cando
// a resposta ten que ser JSON e non se pode interpretar. 1 chamada + 2
// reintentos.
const maxIntentosJSON = 3

// chamarIAJSON chama a chamarIA e desempaqueta a resposta en `destino`,
// insistindo ata maxIntentosJSON veces cando NON se pode interpretar coma
// JSON - o fallo típico da IA ao xerar exames: barras invertidas sen
// escapar dentro dun string ("\draw"/"\node"/"\," nun <TIKZ>). Por intento:
//  1. parse directo do JSON extraído (extraer + limparValadosMarkdown);
//  2. parse tras repararEscapesJSON (arranxa os escapes sen rechamar);
//  3. se aínda falla e quedan intentos, RECHAMA á IA cunha nota curta co
//     erro anterior para que se corrixa.
//
// Un erro de rede/HTTP de chamarIA devólvese tal cal, sen insistir (adoita
// ser a clave ou o modelo mal configurados: reintentar só fai perder
// tempo). Se se esgotan os intentos, o último erro de parse vai envolto en
// formatoInesperadoErr, coa mesma mensaxe rica de sempre (preview da
// resposta, pista de "quedou cortada"...).
//
// `extraer` é extractJSONArray ou extractJSONObject segundo o que se
// agarde; `destino` é un punteiro coma o de json.Unmarshal.
func chamarIAJSON(s Settings, sistema, peticion string, extraer func(string) string, destino any) error {
	peticionActual := peticion
	var ultimoRaw string
	var ultimoErro error
	for intento := 1; intento <= maxIntentosJSON; intento++ {
		raw, err := chamarIA(s, sistema, peticionActual)
		if err != nil {
			return err
		}
		ultimoRaw = raw
		limpo := extraer(limparValadosMarkdown(raw))
		if ultimoErro = json.Unmarshal([]byte(limpo), destino); ultimoErro == nil {
			return nil
		}
		if ultimoErro = json.Unmarshal([]byte(repararEscapesJSON(limpo)), destino); ultimoErro == nil {
			return nil
		}
		peticionActual = peticion + fmt.Sprintf(
			"\n\n[IMPORTANTE: a túa resposta anterior NON era JSON válido (%v). "+
				"Devolve SÓ o JSON pedido, ben formado e sen texto arredor. "+
				"Se dentro dun valor de texto vai código LaTeX ou TikZ (\\draw, \\node, \\,, \\Omega), "+
				"cada barra invertida ten que ir DUPLICADA (\\\\draw) e os saltos de liña como \\n.]",
			ultimoErro)
	}
	return formatoInesperadoErr(ultimoRaw, ultimoErro)
}

// ── Gemini (nativo) ──────────────────────────────────────────────────────

type geminiPeticion struct {
	SystemInstruction *geminiContido          `json:"systemInstruction,omitempty"`
	Contents          []geminiContido         `json:"contents"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

// geminiGenerationConfig só leva MaxOutputTokens por agora - Yang non
// expón ningún outro parámetro de xeración (temperature, topP...) na UI,
// así que non fai falla modelar máis ca isto.
type geminiGenerationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type geminiContido struct {
	Role  string        `json:"role,omitempty"`
	Parts []geminiParte `json:"parts"`
}

type geminiParte struct {
	Text string `json:"text,omitempty"`
}

type geminiResposta struct {
	Candidates []struct {
		Content geminiContido `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// chamarGemini envía `sistema` coma systemInstruction e `peticion` coma único
// contido do usuario. Idéntica xestión de erros ca en Xesta (mesmas
// mensaxes, mesmo comportamento comprobado empiricamente contra a API real:
// systemInstruction precisa Role:"user" ou a API devolve 400 sen máis
// detalle). maxTokens <= 0 omite generationConfig por completo (deixa o
// límite de resposta por defecto do propio modelo, comportamento previo a
// que isto fose configurable - ver Settings.IAMaxTokens).
func chamarGemini(baseURL, apiKey, model, sistema, peticion string, maxTokens int, cliente *http.Client) (string, error) {
	if cliente == nil {
		cliente = iaHTTPClient
	}
	corpo := geminiPeticion{
		SystemInstruction: &geminiContido{Role: "user", Parts: []geminiParte{{Text: sistema}}},
		Contents: []geminiContido{
			{Role: "user", Parts: []geminiParte{{Text: peticion}}},
		},
	}
	if maxTokens > 0 {
		corpo.GenerationConfig = &geminiGenerationConfig{MaxOutputTokens: maxTokens}
	}
	dados, err := json.Marshal(corpo)
	if err != nil {
		return "", err
	}

	url := baseURL + "/models/" + model + ":generateContent"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(dados))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := cliente.Do(req)
	if err != nil {
		return "", fmt.Errorf("non se puido contactar coa IA: %w", err)
	}
	defer resp.Body.Close()

	corpoResposta, _ := io.ReadAll(resp.Body)
	var r geminiResposta
	if err := json.Unmarshal(corpoResposta, &r); err != nil {
		return "", fmt.Errorf("resposta non válida da IA (HTTP %d)", resp.StatusCode)
	}
	if r.Error != nil {
		return "", fmt.Errorf("erro da IA: %s", r.Error.Message)
	}
	if resp.StatusCode != http.StatusOK || len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("a IA non devolveu resultado (HTTP %d)", resp.StatusCode)
	}

	return limparValadosMarkdown(r.Candidates[0].Content.Parts[0].Text), nil
}

// ── Anthropic (nativo, Claude) ───────────────────────────────────────────

type anthropicPeticion struct {
	Model     string                  `json:"model"`
	MaxTokens int                     `json:"max_tokens"`
	System    []anthropicSystemBloque `json:"system,omitempty"`
	Messages  []anthropicMensaxe      `json:"messages"`
}

// anthropicSystemBloque é un anaco do "system" da API de Claude. Vai en
// slice (en vez de string simple) para poder marcar CacheControl: o
// "sistema" que mandamos (chuletaMatexe/chuletaMatexePara + instrución) é
// idéntico en moitas chamadas seguidas (mesmo tipo de anaco, mesmo
// provedor) - marcándoo coma "ephemeral" Claude cachea eses tokens no seu
// lado e cobra ~un 10% neles nas chamadas seguintes en vez do prezo enteiro,
// sen cambiar en nada a resposta (non é compresión con perdas, é caché
// exacta). Gemini/OpenAI non necesitan isto: cachean prefixos repetidos
// automaticamente sen que o cliente marque nada.
type anthropicSystemBloque struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicCacheControl struct {
	Type string `json:"type"`
}

type anthropicMensaxe struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResposta struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// chamarAnthropic fala coa API de Messages de Claude (api.anthropic.com) -
// autenticación por cabeceira x-api-key (non Bearer) e cabeceira obrigatoria
// anthropic-version, a diferenza dos outros dous formatos. maxTokens chega
// xa resolto (Settings.IAMaxTokensAnthropicResolto) - a diferenza de
// Gemini/OpenAI, esta API esixe SEMPRE un valor numérico en max_tokens, non
// se pode omitir.
func chamarAnthropic(baseURL, apiKey, model, sistema, peticion string, maxTokens int, cliente *http.Client) (string, error) {
	if cliente == nil {
		cliente = iaHTTPClient
	}
	corpo := anthropicPeticion{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  []anthropicMensaxe{{Role: "user", Content: peticion}},
	}
	if sistema != "" {
		// CacheControl "ephemeral": chuletaMatexe/chuletaMatexePara + a
		// instrución que segue son idénticas en moitas chamadas seguidas
		// (mesmo tipo de anaco) - Claude cachea eses tokens no seu lado
		// (5 min de TTL por defecto) e cobra menos por eles nas chamadas
		// seguintes. Se sistema queda por baixo do mínimo cacheable do
		// modelo (~1024-2048 tokens), a API simplemente ignora a marca e
		// segue igual, sen erro.
		corpo.System = []anthropicSystemBloque{{
			Type:         "text",
			Text:         sistema,
			CacheControl: &anthropicCacheControl{Type: "ephemeral"},
		}}
	}
	dados, err := json.Marshal(corpo)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/messages", bytes.NewReader(dados))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := cliente.Do(req)
	if err != nil {
		return "", fmt.Errorf("non se puido contactar coa IA: %w", err)
	}
	defer resp.Body.Close()

	corpoResposta, _ := io.ReadAll(resp.Body)
	var r anthropicResposta
	if err := json.Unmarshal(corpoResposta, &r); err != nil {
		return "", fmt.Errorf("resposta non válida da IA (HTTP %d)", resp.StatusCode)
	}
	if r.Error != nil {
		return "", fmt.Errorf("erro da IA: %s", r.Error.Message)
	}
	if resp.StatusCode != http.StatusOK || len(r.Content) == 0 {
		return "", fmt.Errorf("a IA non devolveu resultado (HTTP %d)", resp.StatusCode)
	}

	var texto strings.Builder
	for _, bloque := range r.Content {
		if bloque.Type == "text" {
			texto.WriteString(bloque.Text)
		}
	}
	return limparValadosMarkdown(texto.String()), nil
}

// ── OpenAI / "compatible con OpenAI" (ChatGPT, Qwen, Perplexity, Groq...) ──

type openAIPeticion struct {
	Model     string          `json:"model"`
	Messages  []openAIMensaxe `json:"messages"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

type openAIMensaxe struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResposta struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// chamarOpenAICompatible fala o formato "chat completions" (POST
// {baseURL}/chat/completions, Authorization: Bearer <clave>) - o mesmo que
// falan de fábrica moitos outros provedores cando se lles pasa o enderezo
// axeitado (ver baseURLPorDefecto e os presets do selector no frontend).
// maxTokens <= 0 omite max_tokens por completo (omitempty, openAIPeticion) -
// deixa o límite de resposta por defecto do propio modelo, comportamento
// previo a que isto fose configurable (ver Settings.IAMaxTokens).
func chamarOpenAICompatible(baseURL, apiKey, model, sistema, peticion string, maxTokens int, cliente *http.Client) (string, error) {
	if cliente == nil {
		cliente = iaHTTPClient
	}
	corpo := openAIPeticion{
		Model: model,
		Messages: []openAIMensaxe{
			{Role: "system", Content: sistema},
			{Role: "user", Content: peticion},
		},
		MaxTokens: maxTokens,
	}
	dados, err := json.Marshal(corpo)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(dados))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := cliente.Do(req)
	if err != nil {
		return "", fmt.Errorf("non se puido contactar coa IA: %w", err)
	}
	defer resp.Body.Close()

	corpoResposta, _ := io.ReadAll(resp.Body)
	var r openAIResposta
	if err := json.Unmarshal(corpoResposta, &r); err != nil {
		return "", fmt.Errorf("resposta non válida da IA (HTTP %d)", resp.StatusCode)
	}
	if r.Error != nil {
		return "", fmt.Errorf("erro da IA: %s", r.Error.Message)
	}
	if resp.StatusCode != http.StatusOK || len(r.Choices) == 0 {
		return "", fmt.Errorf("a IA non devolveu resultado (HTTP %d)", resp.StatusCode)
	}

	return limparValadosMarkdown(r.Choices[0].Message.Content), nil
}

// chuletaMatexe resume as etiquetas propias de Matexe — o único que a IA
// non coñece de fábrica (Maxima/LaTeX en xeral xa os coñece ben, non fai
// falla explicalos). Fonte: yang/help/Matexe Help/3*.html e cas/tags.go.
const chuletaMatexe = `Matexe é a linguaxe de marcado deste programa (Yang) para exames: HTML normal
con etiquetas especiais que Maxima procesa antes de xerar o documento final.

- <MAT>expr</MAT>: mostra a expresión SEN avaliar, en LaTeX.
- <EVAL>expr</EVAL>: avalía a expresión e mostra o resultado en LaTeX.
- <HIDE>expr1; expr2; ...</HIDE>: executa en silencio (varias expresións
  separadas por ; ou $), non mostra nada — para definir variables/funcións
  antes de usalas noutro anaco.
- <PLOT>chamada</PLOT>: xera unha imaxe cun gráfico Maxima (plot2d, plot3d,
  draw2d, draw3d).
- <TIKZ>código</TIKZ>: debuxo en código TikZ puro (LaTeX/pgf, non Maxima).
- <TEX>código</TEX>: LaTeX literal.
- <SISTEMA>eq1
  eq2
  eq3</SISTEMA>: sistema de ecuacións cunha chave compartida. UNHA ecuación
  Maxima por liña, SEN <MAT>/<EVAL> arredor de cada unha (o propio <SISTEMA>
  xa manda cada liña a Maxima). Úsao SEMPRE que o enunciado pida resolver/
  clasificar un sistema de varias ecuacións á vez - NUNCA repartas as
  ecuacións en varias <MAT> soltas nun <p> cada unha, iso non debuxa a chave:
  - CORRECTO: <SISTEMA>
x + 2*y - z = 4
2*x - y + z = 1
-x + y + 2*z = 3
</SISTEMA>
  - INCORRECTO: <p><MAT>x + 2*y - z = 4</MAT></p><p><MAT>2*x - y + z = 1</MAT></p>...

NUNCA escribas "\begin{...}" nin "\end{...}" (nin "\begin<...>") fóra de
<TEX>. Rompe SEMPRE a compilación con "Environment ... undefined":
- Para un SISTEMA de ecuacións usa <SISTEMA>...</SISTEMA>, non "\begin{cases}".
- Para unha MATRIZ ou un vector usa <MAT>matrix([a,b],[c,d])</MAT>, non
  "\begin{pmatrix}" nin "\begin{array}".
- Para PASOS aliñados escribe un <p> (ou un <li>) por liña, non
  "\begin{aligned}" nin "\begin{align}".
- "\begin{center}", "\begin{itemize}", "\begin{tabular}"... TAMPOUCO: usa o
  HTML equivalente (<p style="text-align:center">, <ul>, <table>).

Dentro de <TIKZ>/<PLOT> vai SÓ código TikZ/Maxima puro; NUNCA HTML, e NUNCA
<MAT> (dá "a node must have a label text"). As etiquetas de nodo levan SEMPRE
chaves SIMPLES, nunca dobres:
- CORRECTO:   \node at (0,1) {$R_1$};        e   \node at (2,1) {$5\,\mathrm{W}$};
- INCORRECTO: \node at (0,1) R_1;            (sen chaves: non compila)
- INCORRECTO: \node at (0,1) {{$R_1$}};      (chaves dobres: NON as dobres)
- INCORRECTO: \node at (0,1) <MAT>R_1</MAT>;
Escribe as etiquetas dos compoñentes en LaTeX matemático directo ($R_1$,
$12\,\Omega$, $\mathrm{W}$), non con <MAT>.

A ÚNICA etiqueta admitida dentro dun <TIKZ> é <EVAL>, e só para meter o VALOR
dunha variable nunha etiqueta de nodo (un circuíto que ten que amosar as
resistencias da copia concreta). Vai sempre dentro das chaves do nodo, nunca
no seu lugar:
- CORRECTO:   \node at (2,3.6) {$R_1 = <EVAL>r1</EVAL>\,\Omega$};
- INCORRECTO: \node at (2,3.6) <EVAL>r1</EVAL>;
Se o valor non cambia entre copias, escribe o número directamente ($4\,\Omega$)
en vez de usar <EVAL>.

IMPORTANTE: o contido de <MAT>/<EVAL>/<HIDE> é sempre código MAXIMA, NUNCA
LaTeX xa escrito - é o propio Maxima quen o converte a LaTeX bonito
automaticamente, ti non tes que facelo. Usa sempre a sintaxe Maxima:
- CORRECTO: <MAT>sin(alpha)</MAT>  ->  INCORRECTO: <MAT>\sin(\alpha)</MAT>
- CORRECTO: <MAT>3/5</MAT>         ->  INCORRECTO: <MAT>\frac{3}{5}</MAT>
- CORRECTO: <MAT>sqrt(x)</MAT>     ->  INCORRECTO: <MAT>\sqrt{x}</MAT>
- CORRECTO: <MAT>4*x</MAT>         ->  INCORRECTO: <MAT>4x</MAT> (Maxima esixe
  o * de multiplicar; "4x" non é unha expresión válida)
- CORRECTO: <MAT>x^2</MAT>         ->  INCORRECTO: <MAT>x^{2}</MAT>
Nomes de variables tampouco levan barra invertida nin chaves: usa "alpha",
"pi" (ou %pi), "theta"... nunca "\alpha"/"\pi"/"\theta".

NUNCA metas unidades (kWh, Ω, A, V, h...) dentro de <MAT>/<EVAL> usando a
función text() de Maxima - é o mesmo erro que "4x": falta o * antes de
text(...), e Maxima falla con "text is not an infix operator":
- INCORRECTO: <MAT>2 text(" kWh") * 3 text(" h")</MAT>
As unidades van SEMPRE coma texto HTML normal, FÓRA de <MAT>/<EVAL> - a
expresión de dentro leva só números e variables, nada de texto:
- CORRECTO: <MAT>2 * 3</MAT> kWh

Fóra de <MAT>/<EVAL>, no texto normal dun anaco "text", os símbolos
matemáticos Unicode soltos (≈, ≤, ≥, ≠, ±, ×, ÷, →, ⇒, ∞, ∪, ∩, ∈, letras
gregas...) están ben coma texto normal - úsaos con confianza cando non
formen parte dunha expresión Maxima completa que se poida avaliar. Regra
clara para decidir entre texto normal e <MAT>/<EVAL>: se o que queres
mostrar é unha expresión Maxima COMPLETA (ten sentido avaliala ou
simplificala), vai en <MAT>/<EVAL>; se é só notación solta que estás a citar
(un símbolo de comparación referíndote a el, un intervalo coma "(-∞, -2]",
unha unión "A ∪ B"...), vai coma texto normal, NUNCA metido en <MAT>/<EVAL>
- Maxima non entende notación de intervalos nin un símbolo de comparación
sen os dous lados, e fallará:
- INCORRECTO: (incluído porque a desigualdade ten <MAT>>=</MAT>)
- CORRECTO: (incluído porque a desigualdade ten ≥)
- INCORRECTO: Solución: <MAT>(-inf, -2]</MAT> unida con <MAT>(3, inf)</MAT>
- CORRECTO: Solución: (-∞, -2] ∪ (3, +∞)

MOI IMPORTANTE, REGRA SEN EXCEPCIÓNS: o contido de <MAT>/<EVAL> ten que ser
SEMPRE unha expresión Maxima COMPLETA e válida por si soa. Antes de escribir
calquera <MAT>...</MAT> ou <EVAL>...</EVAL>, comproba que o de dentro, illado
do resto, sería aceptado por Maxima tal cal. Un "=" ao final sen lado dereito
NUNCA é válido - se che sae iso, é que tiñas que ter posto ese anaco coma
texto normal, non coma MAT:
- INCORRECTO: <MAT>cos(alpha) = </MAT><EVAL>cos_val</EVAL>
- INCORRECTO: <MAT>A =</MAT> <EVAL>area</EVAL>
- INCORRECTO: <MAT>f'(x) =</MAT> <EVAL>diff(f(x),x)</EVAL>
- INCORRECTO: <MAT>m = (6 - 2)/(3 - 1) =</MAT> <EVAL>m1</EVAL> (o mesmo erro
  aínda que a fórmula vaia dentro: CALQUERA "=" ao final, con ou sen máis
  texto antes, rompe sempre o parser de Maxima)
- CORRECTO: m = <MAT>(6 - 2)/(3 - 1)</MAT> = <EVAL>m1</EVAL>  (unha cadea
  "nome = fórmula = valor" repártese en TRES anacos: "nome = " coma texto,
  a fórmula soa en <MAT>, " = " coma texto, e o valor en <EVAL> - nunca vaia
  o "=" dentro da etiqueta, nin ao principio nin ao final)
- CORRECTO (máis simple): cos(α) = <EVAL>cos_val</EVAL>  (o "nome = " vai
  coma texto HTML normal, FÓRA de calquera etiqueta; só o valor calculado
  vai en <EVAL>)
Letras gregas soltas coma "α" no texto normal (fóra de MAT/EVAL) son HTML
válido, úsaas sen problema coma parte do texto.

PROHIBIDO usar un apóstrofo (') dentro de <MAT>/<EVAL>/<HIDE>, en ningún
caso: "f'(x)" (notación de "prima" para a derivada) NON é Maxima válido - o
apóstrofo ten outro significado en Maxima e rompe SEMPRE o parser, sen
excepción:
- INCORRECTO: <MAT>f'(x)</MAT>, <MAT>f'(x) =</MAT>, <EVAL>f'(3)</EVAL> (as
  tres fallan sempre, é o erro máis común - revisa antes de escribir "'"
  dentro de calquera etiqueta e sácao se o atopas)
- CORRECTO: escribe "f'(x)" coma texto HTML normal, FÓRA de <MAT>/<EVAL>,
  cando só queres nomear/referirte á derivada (é só texto, non se avalúa);
  usa <EVAL>diff(f(x),x)</EVAL> (sen apóstrofo ningún) cando queres o
  resultado calculado da derivada, e <EVAL>subst(3,x,diff(f(x),x))</EVAL>
  (non "f'(3)") para o seu valor nun punto.

PROHIBIDO usar unha barra invertida (\) dentro de <MAT>/<EVAL>/<HIDE>, en
ningún caso - non é notación de continuación de liña coma en bash/LaTeX,
Maxima non a entende e rompe SEMPRE o parser (e nun <HIDE> con varias
expresións separadas por ";", arrastra ao erro TODAS as que veñen despois):
- INCORRECTO: <HIDE>mcd_res: 2^3*3; mcm_res: 2^4*3^2*5; \</HIDE>
- CORRECTO: <HIDE>mcd_res: 2^3*3; mcm_res: 2^4*3^2*5;</HIDE> (varias
  expresións nunha soa liña, sen barra invertida ningunha ao final nin no
  medio)

Exemplos de Maxima válidos: f(x):=x^2+1 ; diff(f(x),x) ; integrate(x^2,x) ;
solve(x^2-4=0,x) ; plot2d(x^2,[x,-2,2]) ; sin(alpha) ; sqrt(1-x^2) ;
limit(f(x),x,inf).

<MAT>limit(...)</MAT> amosa a notación do límite (non un número solto) SEN
que fagas nada especial - Yang xa o cita internamente, escribe limit(...) tal
cal, SEN apóstrofo diante (o apóstrofo segue prohibido en <MAT>/<EVAL>, regra
enriba).

<sub>/<sup> (HTML normal, fóra de <MAT>/<EVAL>) están soportados para
notación solta que non é unha expresión Maxima completa, ex. "lim<sub>x →
∞</sub>" nunha frase, ou "H<sub>2</sub>O".

NUNCA escribas notación polar coma "r[θ°]" (ex. "2[60°]", "(sqrt(2))[135°]")
dentro de <MAT>/<EVAL> para dar o módulo e argumento dun número complexo -
Maxima non ten literal para forma polar nin entende o símbolo de grao "°",
e falla sempre ("incorrect syntax" ou "° is not an infix operator"):
- INCORRECTO: <MAT>z[1] = 2[60°]</MAT>
- CORRECTO: z<sub>1</sub> = 2 [60°] coma texto normal (notación solta, non
  se avalúa), reservando <MAT>/<EVAL> só para expresións Maxima reais coma
  o módulo e o ángulo por separado: módulo <MAT>abs(1+sqrt(3)*%i)</MAT>,
  argumento <EVAL>atan2(sqrt(3),1)*180/%pi</EVAL>°.

Para VALORES ALEATORIOS (datos que cambian en cada variante do exame, ex.
"unha resistencia de tantos ohmios"): a función de Maxima chámase
random(n) (número enteiro entre 0 e n-1), NUNCA "rand(n)" - "rand" NON
EXISTE en Maxima. Isto é perigoso porque NON dá erro ningún: rand(9) é
sintaxe válida (unha chamada a unha función sen definir), así que Maxima
déixaa sen avaliar e sae literal e mal no exame ("R = rand(9)+2" en vez
dun número). Mellor aínda ca random(n) cru, este programa xa trae
xeradores feitos para isto, defínense en <HIDE> e úsanse en <EVAL>:
- n0()/n1()/n2(): natural aleatorio pequeno/mediano/grande (2 a 4, 2 a 10,
  2 a 101 respectivamente).
- z0()/z1()/z2(): igual que n0/n1/n2 pero enteiro con signo aleatorio.
- q0()/q1()/q2(): igual pero racional (fracción) non enteira.
Exemplo completo e correcto: <HIDE>R_val: n1()</HIDE> ... resistencia de
<EVAL>R_val</EVAL> Ω.`

// chuletaMatexeNucleo é o subconxunto de chuletaMatexe que vale para
// CALQUERA código Maxima que xere a IA, leve ou non etiquetas Matexe
// arredor: sintaxe Maxima (non LaTeX xa escrito), o "=" final prohibido,
// apóstrofo/barra invertida prohibidos, notación polar prohibida e os
// xeradores de valores aleatorios (n0/z0/q0...). É a parte que non se pode
// quitar nunca, mesmo para un anaco solto sen etiquetas (ver
// chuletaMatexePara). Fonte: mesmos parágrafos de chuletaMatexe, copiados
// tal cal - mantéñense as dúas copias en sincronía á man se cambia algo
// nesta zona do texto.
const chuletaMatexeNucleo = `IMPORTANTE: o contido de <MAT>/<EVAL>/<HIDE> é sempre código MAXIMA, NUNCA
LaTeX xa escrito - é o propio Maxima quen o converte a LaTeX bonito
automaticamente, ti non tes que facelo. Usa sempre a sintaxe Maxima:
- CORRECTO: <MAT>sin(alpha)</MAT>  ->  INCORRECTO: <MAT>\sin(\alpha)</MAT>
- CORRECTO: <MAT>3/5</MAT>         ->  INCORRECTO: <MAT>\frac{3}{5}</MAT>
- CORRECTO: <MAT>sqrt(x)</MAT>     ->  INCORRECTO: <MAT>\sqrt{x}</MAT>
- CORRECTO: <MAT>4*x</MAT>         ->  INCORRECTO: <MAT>4x</MAT> (Maxima esixe
  o * de multiplicar; "4x" non é unha expresión válida)
- CORRECTO: <MAT>x^2</MAT>         ->  INCORRECTO: <MAT>x^{2}</MAT>
Nomes de variables tampouco levan barra invertida nin chaves: usa "alpha",
"pi" (ou %pi), "theta"... nunca "\alpha"/"\pi"/"\theta".

NUNCA metas unidades (kWh, Ω, A, V, h...) dentro de <MAT>/<EVAL> usando a
función text() de Maxima - é o mesmo erro que "4x": falta o * antes de
text(...), e Maxima falla con "text is not an infix operator":
- INCORRECTO: <MAT>2 text(" kWh") * 3 text(" h")</MAT>
As unidades van SEMPRE coma texto HTML normal, FÓRA de <MAT>/<EVAL> - a
expresión de dentro leva só números e variables, nada de texto:
- CORRECTO: <MAT>2 * 3</MAT> kWh

MOI IMPORTANTE, REGRA SEN EXCEPCIÓNS: o contido de <MAT>/<EVAL> ten que ser
SEMPRE unha expresión Maxima COMPLETA e válida por si soa. Antes de escribir
calquera <MAT>...</MAT> ou <EVAL>...</EVAL>, comproba que o de dentro, illado
do resto, sería aceptado por Maxima tal cal. Un "=" ao final sen lado dereito
NUNCA é válido - se che sae iso, é que tiñas que ter posto ese anaco coma
texto normal, non coma MAT:
- INCORRECTO: <MAT>cos(alpha) = </MAT><EVAL>cos_val</EVAL>
- INCORRECTO: <MAT>A =</MAT> <EVAL>area</EVAL>
- INCORRECTO: <MAT>f'(x) =</MAT> <EVAL>diff(f(x),x)</EVAL>
- INCORRECTO: <MAT>m = (6 - 2)/(3 - 1) =</MAT> <EVAL>m1</EVAL> (o mesmo erro
  aínda que a fórmula vaia dentro: CALQUERA "=" ao final, con ou sen máis
  texto antes, rompe sempre o parser de Maxima)
- CORRECTO: m = <MAT>(6 - 2)/(3 - 1)</MAT> = <EVAL>m1</EVAL>  (unha cadea
  "nome = fórmula = valor" repártese en TRES anacos: "nome = " coma texto,
  a fórmula soa en <MAT>, " = " coma texto, e o valor en <EVAL> - nunca vaia
  o "=" dentro da etiqueta, nin ao principio nin ao final)
- CORRECTO (máis simple): cos(α) = <EVAL>cos_val</EVAL>  (o "nome = " vai
  coma texto HTML normal, FÓRA de calquera etiqueta; só o valor calculado
  vai en <EVAL>)

PROHIBIDO usar un apóstrofo (') dentro de <MAT>/<EVAL>/<HIDE>, en ningún
caso: "f'(x)" (notación de "prima" para a derivada) NON é Maxima válido - o
apóstrofo ten outro significado en Maxima e rompe SEMPRE o parser, sen
excepción:
- INCORRECTO: <MAT>f'(x)</MAT>, <MAT>f'(x) =</MAT>, <EVAL>f'(3)</EVAL> (as
  tres fallan sempre, é o erro máis común - revisa antes de escribir "'"
  dentro de calquera etiqueta e sácao se o atopas)
- CORRECTO: escribe "f'(x)" coma texto HTML normal, FÓRA de <MAT>/<EVAL>,
  cando só queres nomear/referirte á derivada (é só texto, non se avalúa);
  usa <EVAL>diff(f(x),x)</EVAL> (sen apóstrofo ningún) cando queres o
  resultado calculado da derivada, e <EVAL>subst(3,x,diff(f(x),x))</EVAL>
  (non "f'(3)") para o seu valor nun punto.

PROHIBIDO usar unha barra invertida (\) dentro de <MAT>/<EVAL>/<HIDE>, en
ningún caso - non é notación de continuación de liña coma en bash/LaTeX,
Maxima non a entende e rompe SEMPRE o parser (e nun <HIDE> con varias
expresións separadas por ";", arrastra ao erro TODAS as que veñen despois):
- INCORRECTO: <HIDE>mcd_res: 2^3*3; mcm_res: 2^4*3^2*5; \</HIDE>
- CORRECTO: <HIDE>mcd_res: 2^3*3; mcm_res: 2^4*3^2*5;</HIDE> (varias
  expresións nunha soa liña, sen barra invertida ningunha ao final nin no
  medio)

Exemplos de Maxima válidos: f(x):=x^2+1 ; diff(f(x),x) ; integrate(x^2,x) ;
solve(x^2-4=0,x) ; plot2d(x^2,[x,-2,2]) ; sin(alpha) ; sqrt(1-x^2) ;
limit(f(x),x,inf).

<MAT>limit(...)</MAT> amosa a notación do límite (non un número solto) SEN
que fagas nada especial - Yang xa o cita internamente, escribe limit(...) tal
cal, SEN apóstrofo diante (o apóstrofo segue prohibido en <MAT>/<EVAL>, regra
enriba).

NUNCA escribas notación polar coma "r[θ°]" (ex. "2[60°]", "(sqrt(2))[135°]")
dentro de <MAT>/<EVAL> para dar o módulo e argumento dun número complexo -
Maxima non ten literal para forma polar nin entende o símbolo de grao "°",
e falla sempre ("incorrect syntax" ou "° is not an infix operator"):
- INCORRECTO: <MAT>z[1] = 2[60°]</MAT>
- CORRECTO: z<sub>1</sub> = 2 [60°] coma texto normal (notación solta, non
  se avalúa), reservando <MAT>/<EVAL> só para expresións Maxima reais coma
  o módulo e o ángulo por separado: módulo <MAT>abs(1+sqrt(3)*%i)</MAT>,
  argumento <EVAL>atan2(sqrt(3),1)*180/%pi</EVAL>°.

Para VALORES ALEATORIOS (datos que cambian en cada variante do exame, ex.
"unha resistencia de tantos ohmios"): a función de Maxima chámase
random(n) (número enteiro entre 0 e n-1), NUNCA "rand(n)" - "rand" NON
EXISTE en Maxima. Isto é perigoso porque NON dá erro ningún: rand(9) é
sintaxe válida (unha chamada a unha función sen definir), así que Maxima
déixaa sen avaliar e sae literal e mal no exame ("R = rand(9)+2" en vez
dun número). Mellor aínda ca random(n) cru, este programa xa trae
xeradores feitos para isto, defínense en <HIDE> e úsanse en <EVAL>:
- n0()/n1()/n2(): natural aleatorio pequeno/mediano/grande (2 a 4, 2 a 10,
  2 a 101 respectivamente).
- z0()/z1()/z2(): igual que n0/n1/n2 pero enteiro con signo aleatorio.
- q0()/q1()/q2(): igual pero racional (fracción) non enteira.
Exemplo completo e correcto: <HIDE>R_val: n1()</HIDE> ... resistencia de
<EVAL>R_val</EVAL> Ω.`

// chuletaMatexeTikz é o subconxunto de chuletaMatexe específico do debuxo
// <TIKZ> (chaves simples nos nodos, <EVAL> só dentro delas, nada de
// HTML/<MAT> dentro). Fai falla en "image-tikz" (código TikZ solto, ver
// chuletaMatexePara) e xa vai incluído en chuletaMatexe para calquera
// documento completo que poida embeber <TIKZ>. Copia verbatim do mesmo
// parágrafo de chuletaMatexe.
const chuletaMatexeTikz = `Dentro de <TIKZ>/<PLOT> vai SÓ código TikZ/Maxima puro; NUNCA HTML, e NUNCA
<MAT> (dá "a node must have a label text"). As etiquetas de nodo levan SEMPRE
chaves SIMPLES, nunca dobres:
- CORRECTO:   \node at (0,1) {$R_1$};        e   \node at (2,1) {$5\,\mathrm{W}$};
- INCORRECTO: \node at (0,1) R_1;            (sen chaves: non compila)
- INCORRECTO: \node at (0,1) {{$R_1$}};      (chaves dobres: NON as dobres)
- INCORRECTO: \node at (0,1) <MAT>R_1</MAT>;
Escribe as etiquetas dos compoñentes en LaTeX matemático directo ($R_1$,
$12\,\Omega$, $\mathrm{W}$), non con <MAT>.

A ÚNICA etiqueta admitida dentro dun <TIKZ> é <EVAL>, e só para meter o VALOR
dunha variable nunha etiqueta de nodo (un circuíto que ten que amosar as
resistencias da copia concreta). Vai sempre dentro das chaves do nodo, nunca
no seu lugar:
- CORRECTO:   \node at (2,3.6) {$R_1 = <EVAL>r1</EVAL>\,\Omega$};
- INCORRECTO: \node at (2,3.6) <EVAL>r1</EVAL>;
Se o valor non cambia entre copias, escribe o número directamente ($4\,\Omega$)
en vez de usar <EVAL>.`

// chuletaMatexePara devolve só a parte da chuleta que fai falla para
// `tipo`. Un anaco SOLTO (fórmula, variable aleatoria, debuxo Maxima ou
// TikZ) nunca leva etiquetas Matexe arredor - a instrución de saída xa
// llo prohibe expresamente (ver systemPromptPara/systemPromptAsistente) -
// así que as regras sobre <MAT>/<EVAL>/<HIDE>/<SISTEMA>/<TEX> e cómo
// mesturalas con HTML (o resto de chuletaMatexe) non aportan nada eses
// casos e só gastan tokens en cada petición; abonda coa sintaxe Maxima pura
// (chuletaMatexeNucleo) e, en TikZ, coas regras propias do debuxo
// (chuletaMatexeTikz). "text"/"documento" SI poden levar calquera etiqueta
// embebida (é HTML mesturado con Matexe, ou o .matex enteiro), así que
// levan a chuleta completa sen recortar nada - tamén o fallback por
// defecto, para non recortar de máis se chega un tipo novo que non se
// coñeza aínda aquí.
func chuletaMatexePara(tipo string) string {
	switch tipo {
	case "formula", "variables", "image-plot":
		return chuletaMatexeNucleo
	case "image-tikz":
		return chuletaMatexeNucleo + "\n\n" + chuletaMatexeTikz
	default: // "text", "documento" (ou calquera tipo futuro descoñecido)
		return chuletaMatexe
	}
}

// systemPromptPara devolve a chuleta común máis unha instrución de saída
// estrita para `tipo` (o mesmo "type" que usa blocks-serialize.js), para que
// o resultado se poida meter directamente no textarea do bloque sen
// envoltorios nin explicacións. "image-upload" non ten sentido aquí (é un
// ficheiro do disco, non contido xerable) — o frontend non ofrece o botón ✨
// nese caso, pero devolver un prompt razoable de todos xeitos non fai mal.
func systemPromptPara(tipo string) string {
	var instrucion string
	switch tipo {
	case "formula", "variables":
		instrucion = "Escribe SÓ unha expresión Maxima válida, nunha soa liña, sen etiquetas " +
			"<EVAL>/<HIDE> ao redor, sen explicacións nin comentarios. Exemplo de resposta " +
			"completa e correcta: diff(x^2+1,x)"
	case "image-plot":
		instrucion = "Escribe SÓ unha chamada Maxima de debuxo (plot2d, plot3d, draw2d ou " +
			"draw3d), sen etiquetas ao redor, sen explicacións. Exemplo de resposta completa " +
			"e correcta: plot2d(x^2,[x,-2,2])"
	case "image-tikz":
		instrucion = "Escribe SÓ código TikZ, SEN \\begin{tikzpicture}/\\end{tikzpicture} " +
			"(Yang xa os pon), sen explicacións. Exemplo de resposta completa e correcta: " +
			"\\draw (0,0) -- (1,0) -- (0,1) -- cycle;"
	default: // "text"
		instrucion = "Escribe SÓ o texto/HTML do enunciado (parágrafos, <b>, <i>, listas... " +
			"permitidos), en galego salvo que se pida outro idioma, sen explicacións adicionais " +
			"nin comentarios sobre o que fixeches."
	}
	return chuletaMatexePara(tipo) + "\n\n" + instrucion
}

// asistenteEdicionRe recoñece unha proposta de edición na resposta do
// asistente (ver systemPromptAsistente/AsistenteIA, app.go). Marcas de
// texto simples en vez de pedirlle á IA que devolva JSON a propósito: o
// contido que se está a editar adoita levar código Maxima/TikZ/HTML cheo
// de aspas e barras invertidas - escapalo correctamente dentro dun string
// JSON é un xeito doado de que a IA rompa o parseo sen querer. Coas marcas,
// o contido vai literal, sen escapar nada.
var asistenteEdicionRe = regexp.MustCompile(`(?s)<<<EDICION>>>\s*\n?(.*?)\n?\s*<<<FIN_EDICION>>>`)

// systemPromptAsistente é o prompt do asistente de IA integrado (icona 🤖,
// editor de Código e modais do editor de bloques - ver AsistenteIA, app.go):
// a diferenza de systemPromptPara (que xera SEMPRE contido novo dende cero),
// aquí a IA recibe o contido ACTUAL e ten que decidir por si mesma se a
// petición é unha PREGUNTA (responde en prosa, non toca nada) ou unha
// instrución de CAMBIO (propón contido novo completo, envolto nas marcas
// que asistenteEdicionRe recoñece). tipo engade "documento" (o .matex
// enteiro do editor de Código) aos tipos xa coñecidos de blocks-serialize.js.
func systemPromptAsistente(tipo string) string {
	var descricionTipo string
	switch tipo {
	case "documento":
		descricionTipo = "un documento .matex COMPLETO (o exame/práctica enteiro: HTML normal mesturado con etiquetas Matexe)"
	case "formula", "variables":
		descricionTipo = "unha soa expresión Maxima"
	case "image-plot":
		descricionTipo = "unha chamada Maxima de debuxo (plot2d/plot3d/draw2d/draw3d)"
	case "image-tikz":
		descricionTipo = "código TikZ (sen \\begin{tikzpicture}/\\end{tikzpicture}, Yang xa os pon)"
	default: // "text"
		descricionTipo = "o HTML/texto dun enunciado (pode levar etiquetas Matexe embebidas, ex. <MAT>...</MAT>)"
	}
	return chuletaMatexePara(tipo) + fmt.Sprintf(`

Es un asistente integrado no editor de Yang. O profesorado mándache o
CONTIDO ACTUAL (%s) e unha petición en linguaxe natural - pode ser:

- Unha PREGUNTA sobre ese contido (explica, non toques nada): responde en
  prosa normal, en galego salvo que se pida outro idioma. NON uses as
  marcas de edición descritas abaixo para unha simple pregunta.
- Unha instrución de CAMBIO: envolve o contido NOVO COMPLETO (non só a
  parte que cambia, o texto enteiro que debe quedar) entre as marcas
  <<<EDICION>>> e <<<FIN_EDICION>>>, cada unha na súa propia liña. Podes
  engadir unha explicación breve FÓRA das marcas (antes ou despois), pero
  dentro delas vai só o contido en si, literal, sen valados markdown e sen
  comentarios sobre o cambio mesturados co contido.
- Se o contido actual vén baleiro, calquera petición é para CREAR contido
  novo (mesmas marcas de edición).
`, descricionTipo)
}

// systemPromptCorreccion é o prompt do botón "Reorganizar práctica" (ver
// correccion_ia.go / frontend/src/reorganizar.js): o enunciado xa está FIXO
// e a IA só ten que devolver a súa corrección - o resultado final coma
// FÓRMULAS Maxima (nunca o número xa calculado) e a resolución paso a paso.
// Que devolva fórmulas e non números é o que fai a xeración fiable: o
// orquestrador mételas no <HIDE> como res_1, res_2... e <RESP>/<SOL>
// referéncianas, así que é Maxima quen calcula e non poden discrepar.
const systemPromptCorreccion = chuletaMatexe + `

TAREFA: dáseche o ENUNCIADO dun exercicio (NON o cambies) e a lista de
VARIABLES xa declaradas. Devolve a súa CORRECCIÓN nun obxecto JSON con estas
catro claves:

{
  "resultados": [
    { "etiqueta": "nome curto do apartado (ex. Área, Velocidade final, x)",
      "formula":  "expresión MAXIMA que calcula ese resultado" }
  ],
  "resolucion": "explicación PASO A PASO en HTML simple (<p>, <ul>/<ol>, <b>)",
  "sen_formula": false,
  "motivo": ""
}

Regras de "resultados":
- Un elemento por cada apartado/pregunta do enunciado. Se hai un só, un só.
- "formula" ten que ser Maxima VÁLIDO por si só, escrito SÓ cos nomes das
  variables xa declaradas (e, se cómpre, cos resultados anteriores da propia
  lista, que se chamarán res_1, res_2... na orde en que os devolvas).
- Pon a FÓRMULA, NUNCA o número: "base*altura/2", nunca "24".
- NUNCA remates unha "formula" nun "=".

Regras de "resolucion" (é o que máis falla - coídao):
- Estrutúraa como unha lista <ol> cun <li> por paso, en orde.
- Cada <li> ABRE cunha frase curta en <b> que di QUE se fai nese paso e por
  que ("<b>Illamos a velocidade da ecuación do MRUA:</b>", "<b>Aplicamos o
  teorema de Pitágoras:</b>"). Son os "pequenos comentarios" do que se fai.
- Tras a frase, pon a FÓRMULA XERAL en <MAT> (só símbolos, SEN números) e a
  seguir a substitución co valor en <EVAL>: así queda a fórmula en LaTeX E o
  número calculado. Ex.: <b>...:</b> <MAT>v = e/t</MAT> = <EVAL>e/t</EVAL> m/s.
- CADA número vai nun <EVAL> dunha expresión sobre as variables declaradas ou
  sobre res_1, res_2...: NUNCA escribas un número a man (mudaría entre copias).
- As UNIDADES (m, s, m/s^2, N, J, Ω, V...) van SEMPRE como texto normal XUSTO
  detrás do <EVAL>, NUNCA dentro del. Toda magnitude física, nos pasos
  intermedios e no resultado, leva a súa unidade correcta.
- O derradeiro <li> enuncia o RESULTADO final ("<b>Resultado:</b> v =
  <EVAL>res_1</EVAL> m/s") cos mesmos valores que "resultados" (idealmente
  <EVAL>res_1</EVAL>, <EVAL>res_2</EVAL>...) e a súa unidade.
- Respecta ao pé da letra as regras de <MAT>/<EVAL> de enriba (nada de LaTeX
  cru, nada de "=" ao final dunha etiqueta, nada de apóstrofos).

Se o exercicio NON ten un resultado calculable cunha fórmula pechada (unha
demostración, un "explica por que", un "debuxa", unha resposta aberta),
devolve "resultados": [], "resolucion": "", "sen_formula": true e un "motivo"
breve.

Se che dan unha RESOLUCIÓN ACTUAL, tómaa de punto de partida: consérvaa se
está ben, arránxaa se falla. Se che dan DIAGNÓSTICOS dunha validación
fallida, a prioridade é cambiar as fórmulas para que eses casos (infinito,
indeterminado, división por cero, erro de Maxima) xa non ocorran - p. ex.
evitando dividir por algo que pode ser 0, ou metendo abs()/(...)^2 baixo unha
raíz.

Devolve SÓ ese obxecto JSON: sen valados markdown, sen texto antes nin
despois. En galego, salvo que o enunciado estea noutro idioma.`

// tiposAnacoValidos son os "type" que blocks-serialize.js sabe interpretar
// como anaco dun exercicio (mesmos que ELEMENT_META en blocks.js, agás
// "image-upload": é un ficheiro do disco, non hai nada que a IA xere aí).
var tiposAnacoValidos = map[string]bool{
	"text": true, "variables": true, "formula": true, "image-plot": true, "image-tikz": true,
}

// systemPromptExercicio pide á IA un EXERCICIO enteiro (varios anacos en
// orde, non un anaco solto coma systemPromptPara) en JSON estrito, para que
// o profesorado nunca teña que escoller un tipo de bloque nin saber que
// existen "EVAL"/"HIDE" — só describe o exercicio, Yang crea os anacos.
const systemPromptExercicio = chuletaMatexe + `

Vas crear un EXERCICIO completo para un exame, composto por unha lista
ordenada de anacos. Cada anaco ten "type" (un destes valores exactos: "text",
"variables", "formula", "image-plot", "image-tikz") e "content" (o contido
dese anaco, na sintaxe que corresponda ao seu type):
- "text": HTML/texto do enunciado (parágrafos, <b>, <i>, listas...). Calquera
  cálculo ou símbolo matemático DENTRO dun "text" ten que ir coas etiquetas
  <MAT>/<EVAL> descritas enriba (ex. "<p>f'(x) = <EVAL>diff(f(x),x)</EVAL></p>"
  ou "<p>Dada <MAT>f(x) = x^2+1</MAT>:</p>") - NUNCA "\( \)", "$...$" nin
  LaTeX cru: Yang non os interpreta, só as etiquetas Matexe.
- "variables": expresión(s) Maxima executadas en silencio para definir datos
  do exercicio (ex. "f(x):=x^2+1", ou varias separadas por ; ou $) - úsao
  cando o exercicio precise unha función ou valor previo antes de mostrar
  nada, ou cando queiras gardar un resultado para amosalo despois cun <EVAL>
  dentro dun "text" (ex. "cos_val: sqrt(1-sen_val^2)").
- "formula": UNHA soa expresión Maxima simple que se avalía e mostra tal
  cal - NON unha lista, NON unha ecuación simbólica con "=". Se queres
  amosar varios resultados etiquetados (ex. "cos(α) = 0.8"), NON uses
  "formula": fainos coma parte dun anaco "text" con <EVAL> embebido para
  cada un (ex. "<p>cos(α) = <EVAL>cos_val</EVAL></p>").
- "image-plot": UNHA chamada Maxima de debuxo (plot2d/plot3d/draw2d/draw3d).
- "image-tikz": código TikZ (sen \begin{tikzpicture}).

PATRÓN PREFERIDO cando o exercicio pide enunciado + solución + unha
representación gráfica (ex. "estudo dunha función, con gráfica", "resolve e
representa"...): usa EXACTAMENTE estes 4 anacos, nesta orde - "Solución" NON
é un "type" á parte, é o papel que fai o anaco "formula" (ou un "text" con
<EVAL> embebido se hai varios resultados etiquetados, ver regra de "formula"
enriba) nesta secuencia:
  1. "text"        - Enunciado: a pregunta en si.
  2. "variables"    - Cálculo: define en silencio o que faga falla (función,
                      datos) antes de amosar nada.
  3. "formula"      - Solución: o resultado final que responde ao enunciado.
  4. "image-plot"   - Representación gráfica: o debuxo da función/situación.
Sáltate o anaco 2 ("variables") só se o exercicio non precisa definir nada
antes de calcular a solución; sáltate o 4 só se non se pide representación
gráfica ningunha - non o engadas "de máis" cando o enunciado non a pide.

Regras estritas:
- Devolve SÓ un array JSON válido, sen texto adicional, sen explicacións,
  sen valados markdown (nada de ` + "```" + ` ao redor).
- Normalmente o primeiro anaco é "text" co enunciado; se fai falla definir
  algo antes de avalialo, usa "variables" xusto despois; usa "formula"/
  "image-plot"/"image-tikz" só para UN resultado ou gráfico final illado -
  para calquera outra mestura de texto+resultado, mételo nun "text" con
  <EVAL>/<MAT> embebidos, coma nos exemplos de enriba.
- ECONOMIZA anacos: como moito 4 anacos por exercicio na medida do
  posible. Antes de engadir un anaco novo, pregúntate se cabe dentro dun
  "text" xa existente con <EVAL>/<MAT> embebido (ver regra de enriba) -
  iso adoita abondar para calquera enunciado con varios resultados
  etiquetados. Só supera os 4 anacos cando o exercicio realmente precise
  máis pezas DISTINTAS por natureza (ex.: varias "variables" previas +
  unha "formula" final + un "image-plot"): non fragmentes por fragmentar,
  pero tampouco xuntes tipos de contido distintos nun mesmo anaco só por
  aforrar - a diferenciación por tipo (text/variables/formula/gráfico/
  TikZ) segue sendo máis importante ca acadar exactamente 4.
- En galego salvo que se pida outro idioma.

Exemplo de resposta completa e correcta (para "a derivada dunha función
cadrática, con gráfico") - segue o patrón preferido de enriba, un anaco por
papel (Enunciado/Cálculo/Solución/Representación gráfica):
[{"type":"text","content":"Calcula a derivada de f(x) = x^2 + 3x - 2 e representa graficamente a función."},{"type":"variables","content":"f(x):=x^2+3*x-2"},{"type":"formula","content":"diff(f(x),x)"},{"type":"image-plot","content":"plot2d(f(x),[x,-5,5])"}]`

// systemPromptExame reemprega toda a instrución de systemPromptExercicio
// (forma dun exercicio: array de anacos type/content) para pedir un EXAME
// ENTEIRO - o profesorado só escribe o tema (petición a chamarIA), o resto
// vai fixado aquí no prompt de sistema. Diferenza coa resposta dun
// exercicio solto: agora é un array DE arrays (un array de anacos por
// exercicio).
//
// numExercicios <= 0 significa INDEFINIDO: decide a IA cantos fan falla.
// Non é o mesmo ca pedir "moitos" - é quitar o número da ecuación, que é o
// que se quere cando se parte dun documento modelo (o exame derivado debe
// ter os exercicios que teña o orixinal, non os que se adiviñasen antes de
// velo) ou cando o tema xa determina o contido. Por iso, en indefinido, o
// prompt NON dá ningún tope: só lle pide que non infle nin recorte.
func systemPromptExame(numExercicios int) string {
	if numExercicios <= 0 {
		return systemPromptExercicio + `

Agora NON crees un só exercicio: crea un EXAME COMPLETO sobre o tema
indicado, variado na formulación e, cando teña sentido, con dificultade
progresiva (do máis sinxelo ao máis avanzado). Cada exercicio segue tendo a
mesma forma descrita enriba (un array de anacos "type"/"content").

CANTOS EXERCICIOS: decídelo TI, os que pida o traballo. Non hai número
fixado nin máximo:
- Se hai documento(s) modelo, o normal é seguir a súa estrutura e o seu
  número de exercicios.
- Se o que hai é un tema ou unhas instrucións, mira canto contido pide de
  verdade (se se indica unha duración, un número de apartados ou unha
  puntuación total, faille caso).
- Non infles o exame con exercicios de recheo repetidos, nin o recortes se
  o tema dá para máis.

Devolve SÓ un array JSON onde cada elemento é o array de anacos dun
exercicio (é dicir, un array de arrays). Sen texto adicional, sen
explicacións, sen valados markdown.

Exemplo de resposta completa e correcta para 2 exercicios sobre "derivadas":
[[{"type":"text","content":"Calcula a derivada de f(x) = x^2 + 3x - 2."},{"type":"formula","content":"diff(x^2+3*x-2,x)"}],[{"type":"text","content":"Calcula a derivada de g(x) = sin(x)*x."},{"type":"formula","content":"diff(sin(x)*x,x)"}]]`
	}
	return systemPromptExercicio + fmt.Sprintf(`

Agora NON crees un só exercicio: crea EXACTAMENTE %d exercicios distintos
sobre o tema indicado, variados na formulación e, cando teña sentido, en
dificultade progresiva (do máis sinxelo ao máis avanzado). Cada exercicio
segue tendo a mesma forma descrita enriba (un array de anacos "type"/
"content").

Devolve SÓ un array JSON de %d elementos, onde cada elemento é o array de
anacos dun exercicio (é dicir, un array de arrays). Sen texto adicional, sen
explicacións, sen valados markdown.

Exemplo de resposta completa e correcta para 2 exercicios sobre "derivadas":
[[{"type":"text","content":"Calcula a derivada de f(x) = x^2 + 3x - 2."},{"type":"formula","content":"diff(x^2+3*x-2,x)"}],[{"type":"text","content":"Calcula a derivada de g(x) = sin(x)*x."},{"type":"formula","content":"diff(sin(x)*x,x)"}]]`, numExercicios, numExercicios)
}

// systemPromptInformeErro é o prompt de "🐛 Informar deste erro"
// (bugreport.go/frontend/src/bug-report.js): a diferenza de
// systemPromptAsistente (que axuda a corrixir O DOCUMENTO da profesora),
// aquí a IA diagnostica un posible fallo do PROPIO YANG (o parser de
// etiquetas cas/tags.go, a xeración LaTeX, o propio prompt da IA...) e
// escribe un informe PARA O DESENVOLVEDOR - texto Markdown, nunca código
// aplicado. Regra sen excepcións (reforzada tamén no propio prompt): NUNCA
// se aplica nin recompila nada automaticamente - o informe gárdase nun
// ficheiro que o desenvolvedor revisa á man, coma calquera outro parte de
// erro (ver bugreport.go para o razoamento completo desta decisión).
const systemPromptInformeErro = chuletaMatexe + `

Es un asistente que axuda ao DESENVOLVEDOR de Yang (NON ao profesorado que
usa a aplicación) a diagnosticar un erro de compilación. O profesorado
mándache un ANACO ou DOCUMENTO .matex que fallou e a MENSAXE DE ERRO literal
que devolveu Maxima/LaTeX/Yang.

O teu traballo NON é corrixir o exercicio (iso xa o fai outro asistente á
parte) - é escribir un INFORME DE ERRO en Markdown, en galego, con
EXACTAMENTE esta estrutura:

## Diagnóstico
2-4 frases explicando a causa técnica do erro (sintaxe Maxima incorrecta
que Yang debería detectar antes/mellor, un caso non contemplado polo
parser de etiquetas, LaTeX inválido xerado, un límite ou timeout...).

## Repro mínimo
O anaco máis pequeno posible que reproduce o erro (pode ser máis curto có
anaco orixinal se identificas exactamente a parte culpable).

## Suxestión de arranxo
Describe en prosa ONDE e COMO se podería mellorar Yang para que este tipo
de erro non volva pasar (ex. "cas/tags.go debería rexeitar/escapar X antes
de mandalo a Maxima", "o prompt da IA (chuleta_matex) debería prohibir
explicitamente Y", "latexdoc.go debería escapar Z antes de inserilo no
.tex"). Podes engadir un fragmento de código Go ilustrativo se tes
confianza abondo, pero SEMPRE precedido, literalmente, desta liña:
"⚠️ Suxestión sen probar, require revisión humana antes de aplicar."

Regras estritas:
- NUNCA digas nin des a entender que xa arranxaches algo, que xa está
  aplicado, ou que vas modificar ficheiros - ti só escribes texto
  suxestivo, nunca tocas código nin executas nada. Se o teu razoamento
  interno considerase facelo, ignórao: a túa única saída válida aquí é o
  informe Markdown descrito enriba.
- Se a causa parece estar no exercicio da profesora (un simple erro de
  escritura seu, non un caso que Yang debería manexar mellor), dío
  claramente no Diagnóstico e dálle menos peso á Suxestión de arranxo en
  vez de forzar un cambio de código que non fai falla.
- Sen valados markdown ao redor de TODO o informe (podes usalos DENTRO,
  para bloques de código coma ` + "```go```" + `).`

// instrucionModelosPDF engádese a systemPromptExame só cando o profesorado
// xuntou algún PDF modelo (ver textoDeModelosPDF, pdftext.go): deixa claro
// que hai que crear unha OBRA DERIVADA (mesmo tema/formato/dificultade),
// nunca copiar literalmente o texto do documento orixinal.
const instrucionModelosPDF = `

Xúntase a continuación o texto extraído dun ou varios documentos PDF que o
profesorado marcou coma MODELO/referencia (despois do separador "---
DOCUMENTO(S) MODELO ---" na petición do usuario). Úsaos para collerlles o
tema, o formato, o estilo de enunciado e o nivel de dificultade, pero NON
copies literalmente os seus enunciados nin os seus valores numéricos: crea
unha obra DERIVADA - exercicios novos, inspirados nese modelo (mesmo tipo de
pregunta, dificultade semellante), nunca idénticos nin unha simple
paráfrase. Se o profesorado tamén escribiu un tema á parte, combina os dous;
se non escribiu ningún tema, deriva ti o tema a partir do(s) documento(s).`

// ExercicioAnaco é un anaco do exercicio que devolve XerarExercicioIA -
// mesma forma que un elemento de blocks-serialize.js (type/content), listo
// para makeElement(a.Type, a.Content) no frontend.
// Content é textoIA (jsonia.go) porque a IA manda ás veces un número ou
// unha lista onde se lle pediu texto, e con `string` puro iso tiraba co
// exame enteiro (ver o comentario de jsonia.go).
type ExercicioAnaco struct {
	Type    string  `json:"type"`
	Content textoIA `json:"content"`
}

// limparValadosMarkdown quita un posible cerco ```...``` que a IA engada
// malia que llo pedimos explicitamente que non o faga.
func limparValadosMarkdown(texto string) string {
	t := strings.TrimSpace(texto)
	if strings.HasPrefix(t, "```") {
		primeiraLiña := strings.IndexByte(t, '\n')
		if primeiraLiña != -1 {
			t = t[primeiraLiña+1:]
		}
		t = strings.TrimSuffix(strings.TrimSpace(t), "```")
	}
	return strings.TrimSpace(t)
}

// systemPromptPlantilla é o prompt do asistente de PLANTILLAS (plantillas_ia.go):
// a partir dunha descrición, dun documento de mostra ou dunha URL, a IA
// devolve unha plantilla de maqueta LaTeX (o "papel timbrado" onde Yang
// insire despois o documento xerado). Nada que ver cos outros prompts: aquí
// NON se crea contido matemático ningún - o contido é sempre {{CORPO}}.
const systemPromptPlantilla = `Es un experto en LaTeX que crea PLANTILLAS de
documento para Yang, un editor de documentos con matemáticas.

Unha plantilla é a MAQUETA (cabeceira, membrete, marxes, pé...) onde Yang
insire despois o documento do usuario. Onde vaia o documento tes que poñer
exactamente o marcador {{CORPO}} - unha soa vez, e nunca contido de exemplo
no seu lugar.

Podes escribir a plantilla de dúas formas:
1. FRAGMENTO (recomendado se abonda con cabeceira/pé): non poñas
   \documentclass nin \begin{document} - só o LaTeX que vai dentro do
   documento, co {{CORPO}} no medio. Yang xa lle pon o preámbulo.
2. COMPLETA: empeza por \documentclass e remata en \end{document}. Úsaa só
   se de verdade fai falla cambiar clase, marxes ou tipografía (ex.: un
   orzamento, unha carta). Yang engádelle automaticamente os paquetes que
   precisa (graphicx, tikz, amsmath...), non fai falla que os poñas ti.

Todo dato fixo que o usuario poida querer cambiar (nome do centro, materia,
empresa, CIF...) ten que ser unha VARIABLE {{NOME_EN_MAIUSCULAS}}, non texto
escrito a man. Yang xa dá feitas estas, non as declares coma variables:
{{DATA}} (data de hoxe), {{ANO}}, {{TITULO}} e {{FICHEIRO}} (nome do
documento aberto).

Regras estritas:
- Devolve SÓ un obxecto JSON válido, sen texto ao redor e sen valados
  markdown.
- COIDADO COAS BARRAS: o LaTeX vai dentro dunha cadea JSON, así que CADA
  barra invertida ten que ir DUPLICADA. Un salto de liña de LaTeX (dúas
  barras) escríbese "\\\\" no JSON, e "\\\\[0.2em]" para un salto con
  separación. Se escribes "\\[0.2em]", ao profesorado chégalle unha soa
  barra e o documento non compila.
- Campos: "nome" (curto), "descricion" (unha liña), "latex" (a plantilla),
  "markdown" (a MESMA maqueta en Markdown para exportar a .md/.docx/.odt,
  co mesmo {{CORPO}}; "" se non ten sentido), "motor" ("" salvo que a
  plantilla precise xelatex ou lualatex, p.ex. por usar fontspec) e
  "variables" (lista de {"nome","etiqueta","valor"}: nome en MAIÚSCULAS,
  etiqueta lexible para a UI, valor o que se poida deducir do modelo ou ""
  se non se sabe).
- Usa só paquetes LaTeX habituais nunha instalación básica de TeX Live.
- NON escollas fontes do sistema (nada de \setmainfont/\setsansfont/
  \setmonofont con "Fira Sans", "Helvetica"...) agás que cho pidan
  expresamente: esa fonte pode non estar instalada no equipo do
  profesorado e daquela o documento non compila. Se queres un aire sans
  serif, chega con \renewcommand{\familydefault}{\sfdefault}.
- Non uses \includegraphics agás que che digan o nome exacto dunha imaxe
  dispoñible na plantilla.
- Textos en galego salvo que o modelo ou a petición estean noutro idioma.

Exemplo de resposta correcta:
{"nome":"Circular do centro","descricion":"Membrete co nome do centro e pé coa data.","latex":"\\begin{center}\n  {\\Large\\bfseries {{CENTRO}} }\\\\[2pt]\n  {{DEPARTAMENTO}}\n\\end{center}\n\\hrule\n\\vspace{5mm}\n\n{{CORPO}}\n\n\\vfill\n\\noindent\\footnotesize {{CENTRO}} \\textendash{} {{DATA}}\n","markdown":"# {{CENTRO}}\n\n*{{DEPARTAMENTO}}*\n\n---\n\n{{CORPO}}\n\n---\n\n{{CENTRO}} — {{DATA}}\n","motor":"","variables":[{"nome":"CENTRO","etiqueta":"Centro educativo","valor":""},{"nome":"DEPARTAMENTO","etiqueta":"Departamento","valor":""}]}`
