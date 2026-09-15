package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

// Settings persists user-configurable options across runs, mirroring the
// "Opciones" dialog (UfrmOpciones.pas) in the 2013 app: CAS path and the
// Maxima preamble code.
type Settings struct {
	MaximaPath string `json:"maximaPath"`
	CodeIni    string `json:"codeIni"` // empty = use the embedded default
	// IAProvedor/IAAPIKey/IAModel/IABaseURL configuran o asistente de IA
	// opcional (botón ✨ nos bloques, ver ia_client.go) - calquera provedor
	// que fale un dos tres formatos de fío soportados (Gemini nativo,
	// Anthropic nativo, ou "compatible con OpenAI" - que cobre ChatGPT e
	// moitos outros, ver chamarIA). IAAPIKey baleira = asistente
	// desactivado, o resto de Yang funciona igual sen el.
	IAProvedor string `json:"iaProvedor"` // "gemini" | "anthropic" | "openai"
	IAAPIKey   string `json:"iaApiKey"`
	IAModel    string `json:"iaModel"`
	// IABaseURL só fai falla para un provedor "compatible con OpenAI" que
	// non sexa a propia OpenAI (Qwen, Perplexity...) - baleiro = enderezo
	// oficial do provedor (ver baseURLPorDefecto). O selector do frontend xa
	// o prerreche cos presets coñecidos, o profesorado non ten por que velo.
	IABaseURL string `json:"iaBaseUrl"`
	// IAPreset non o le ningunha chamada á IA - é só o valor do selector do
	// frontend (que nome de provedor amosar), gardado á parte de
	// IAProvedor/IABaseURL para poder redebuxar exactamente o mesmo preset
	// ao reabrir Opcións (ex. distinguir "Qwen" de "Outro" aínda que os dous
	// resolvan a IAProvedor="openai" cun IABaseURL parecido).
	IAPreset string `json:"iaPreset"`
	// IAMaxTokens limita canto pode chegar a escribir a IA nunha soa
	// resposta (calquera chamada de ia_client.go: asistente, ✨ anaco/
	// exercicio/exame) - 0/sen configurar deixa o comportamento de sempre,
	// distinto por provedor (ver IAMaxTokensResolto/IAMaxTokensAnthropicResolto
	// embaixo). Real motivo de existir: "a IA devolveu algo que non se puido
	// interpretar coma JSON" (ver formatoInesperadoErr, app.go) adoita ser
	// unha resposta CORTADA polo límite de saída do modelo, especialmente en
	// ✨ Exame completo con moitos exercicios - antes desta opción ese límite
	// estaba fixo no código (8192 só para Anthropic, o resto sen tocar) e o
	// profesorado non tiña forma de subilo sen recompilar Yang. Subir isto
	// require que o MODELO escollido admita ese límite - Yang non o
	// comproba, un valor demasiado alto para un modelo pequeno faría fallar
	// a chamada coa súa propia mensaxe de erro do provedor.
	IAMaxTokens int `json:"iaMaxTokens"`
	// IATimeoutSegundos limita canto agarda Yang por CADA chamada á IA
	// (ia_client.go: asistente, ✨ exercicio/exame/contido) antes de dala por
	// perdida. 0/sen configurar -> 240s; valores por debaixo de 30 trátanse
	// coma sen configurar (ver IATimeoutResolto). Convén subilo (600s ou máis)
	// con modelos de razoamento lentos ou peticións longas (un exame enteiro).
	IATimeoutSegundos int `json:"iaTimeoutSegundos"`
	// LatexEngine escolle o compilador: "" ou "pdflatex" (por defecto),
	// "xelatex" ou "lualatex" (soporte Unicode/fontes do sistema nativo,
	// útil para símbolos especiais coma letras gregas ou fontes propias).
	LatexEngine string `json:"latexEngine"`
	// Decimais limita as cifras decimais que Maxima amosa nos resultados en
	// coma flotante (ex. "3.56" en vez de "3.555555555555556") - sen isto,
	// calquera cálculo que non quede en fracción exacta propaga toda a
	// precisión da máquina ao PDF. 0/sen configurar = usar o valor por
	// defecto (2), resolto no punto de uso - mesmo patrón ca LatexEngine.
	Decimais int `json:"decimais"`
	// Decimal, cando true, fai que TODOS os resultados de <MAT>/<EVAL> se
	// amosen en coma decimal por defecto (ex. "1.15"), en vez da fracción
	// exacta que Maxima usa por defecto cando o cálculo só ten enteiros
	// (ex. "23/20") - ver cas.Maxima.ForzarDecimal/matexe_decimal. Por
	// defecto (false) mantense o comportamento previo, fracción exacta,
	// que precisan exercicios coma "Simplificación de fraccións"
	// (testdata/test.matex) - un profesor de física que queira sempre
	// decimal actívao aquí en vez de escribir matexe_decimal(...) en cada
	// <MAT>; un exercicio solto que queira a excepción contraria pode
	// envolver <MAT>matexe_fraccion(...)</MAT> aínda con isto activado.
	Decimal bool `json:"decimal"`
	// TimeoutSegundos limita canto agarda Yang por unha resposta de Maxima
	// a UNHA soa chamada (<MAT>/<EVAL>/<HIDE>...) antes de dala por
	// atascada (bucle infinito, ou proceso morto/wedged) e reiniciar a
	// sesión automaticamente - ver cas.Session.SetTimeout. 0/sen configurar
	// = usar o valor por defecto (30s), resolto no punto de uso - mesmo
	// patrón ca Decimais/LatexEngine.
	TimeoutSegundos int `json:"timeoutSegundos"`
	// ReorganizarSementes é o número de sementes coas que o botón
	// "Reorganizar práctica" (ver reorganizar.js / correccion_ia.go) valida
	// cada exercicio despois de reconstruír <RESP>/<SOL>, buscando infinitos,
	// indeterminacións e erros de Maxima antes de aceptar a corrección.
	// 0/sen configurar -> 25 (o mesmo defecto ca a lapela "Test"/proba.go),
	// resolto no punto de uso - mesmo patrón ca Decimais/TimeoutSegundos.
	// Cap duro en 200. A UI pode subilo por exercicio (adáptase ás súas
	// características: raíces, denominadores con variable...).
	ReorganizarSementes int `json:"reorganizarSementes"`
	// ReorganizarRoldas é o máximo de roldas de ARRANXO automático nesa mesma
	// operación: se a validación atopa un problema, os diagnósticos
	// devólvense á IA para que corrixa a fórmula, ata este número de veces.
	// 0/sen configurar -> 3. Cap duro en 6.
	ReorganizarRoldas int `json:"reorganizarRoldas"`
	// AutorepararExercicios: cando un exame xerado coa IA non compila,
	// Yang illa que exercicio(s) fallan e pídelle á IA que os rexenere (só
	// eses, non o exame enteiro), ata MaxRechamadasIA veces. *bool porque o
	// defecto é true (mesmo motivo ca XerarAoGardar): un settings.json vello
	// non ten a chave e non se debe ler coma "desactivado". Ver
	// AutorepararExerciciosResolto e frontend/src/auto-reparar.js.
	AutorepararExercicios *bool `json:"autorepararExercicios,omitempty"`
	// MaxRechamadasIA é cantas veces, como moito, se lle volve pedir á IA que
	// arranxe UN exercicio que non compila antes de deixalo cunha caixa de
	// aviso visible (o resto do exame sae igual). 0/sen configurar -> 2.
	// Cap duro en 6. 0 explícito non existe aquí: para "non chamar á IA"
	// desactívase AutorepararExercicios.
	MaxRechamadasIA int `json:"maxRechamadasIA"`
	// Idioma escolle a lingua da interface: "" ou "gl" (galego, por
	// defecto), "es", "en" ou "pt". Os dicionarios en si viven en
	// yang/idiomas/*.json, importados directamente polo frontend
	// (frontend/src/main.js) - este campo só persiste a escolla do
	// profesorado entre sesións, o propio Go nunca traduce nada.
	Idioma string `json:"idioma"`
	// XerarAoGardar, cando activo (por defecto), fai que Gardar/Gardar como
	// (Ctrl+S) tamén recompile o documento (mesmo que premer Xerar/Ctrl+X) -
	// así a vista previa nunca queda desincronizada do que se acaba de
	// gardar. Un *bool (non un bool a secas) porque o valor por defecto é
	// true: un ficheiro settings.json vello (ou un profesor que nunca abriu
	// Opcións) non ten esta chave, e un bool normal léraa como false
	// (desactivado) por defecto de Go - mesmo problema que Decimais/
	// TimeoutSegundos resolven con "0 = usar o valor por defecto", pero aquí
	// false SI é un valor válido e distinguible de "sen configurar" (nil),
	// así que fai falla un punteiro. Ver XerarAoGardarResolto.
	XerarAoGardar *bool `json:"xerarAoGardar,omitempty"`
	// ExportarPDF/ExportarTex/ExportarMarkdown/ExportarDocx/ExportarOdt
	// deciden que botóns de exportación amosa a toolbar da previsualización
	// (btnPDF/btnExportTex/btnExportMarkdown/btnExportDocx/btnExportOdt en
	// frontend/src/main.js) - un profesor que só use PDF pode agochar TeX/MD/
	// DOCX/ODT en vez de ter cinco botóns sempre visibles. PDF/Tex/Markdown
	// son *bool (mesmo motivo ca XerarAoGardar: por defecto ACTIVADOS, así
	// que "sen configurar" (nil) ten que distinguirse de "false" explícito).
	// Docx/Odt son bool normal porque por defecto van DESACTIVADOS (precisan
	// Pandoc, unha dependencia extra que non todo profesorado instala) - o
	// valor cero de Go (false) xa é o defecto correcto, non fai falla
	// punteiro.
	ExportarPDF      *bool `json:"exportarPdf,omitempty"`
	ExportarTex      *bool `json:"exportarTex,omitempty"`
	ExportarMarkdown *bool `json:"exportarMarkdown,omitempty"`
	ExportarDocx     bool  `json:"exportarDocx"`
	ExportarOdt      bool  `json:"exportarOdt"`
	// EditorMode escolle a interface de edición da parte esquerda: "" ou
	// "tabla" (folla de cálculo, por defecto) ou "blockly" (o editor de
	// bloques). Só persiste a escolla do profesorado (Opcións → Interface de
	// edición, ou o botón de troca rápida da barra); a lóxica real vive no
	// frontend (frontend/src/main.js switchMode), o Go só garda a cadea.
	EditorMode string `json:"editorMode"`
	// UltimoCartafol é a raíz que amosaba o explorador de ficheiros (ver
	// explorer.go/frontend/src/explorer.js) a última vez que se pechou Yang -
	// só se actualiza cando a usuaria escolle explicitamente "Abrir
	// cartafol" (non ao abrir/gardar un .matex solto con Ctrl+O/Ctrl+S: a
	// raíz do explorador é un concepto á parte do ficheiro activo, coma o
	// cartafol de traballo de VS Code fronte á pestana aberta). Baleiro =
	// nunca se escolleu ningún, o explorador arrinca amosando "Abrir
	// cartafol". Se o cartafol gardado xa non existe, ListarCartafol
	// devolve erro e explorer.js volve a ese mesmo estado inicial.
	UltimoCartafol string `json:"ultimoCartafol"`
}

// XerarAoGardarResolto devolve XerarAoGardar coa regra de "sen configurar
// (nil) -> true" xa aplicada, mesmo criterio ca DecimaisResolto/
// TimeoutResolto pero para un booleano.
func (s Settings) XerarAoGardarResolto() bool {
	return s.XerarAoGardar == nil || *s.XerarAoGardar
}

// ExportarPDFResolto/ExportarTexResolto/ExportarMarkdownResolto aplican a
// mesma regra de "sen configurar (nil) -> true" ca XerarAoGardarResolto -
// un settings.json vello, ou un profesor que nunca abriu Opcións, non ten
// estas chaves e os cinco botóns de exportación deben seguir visibles coma
// sempre.
func (s Settings) ExportarPDFResolto() bool {
	return s.ExportarPDF == nil || *s.ExportarPDF
}

func (s Settings) ExportarTexResolto() bool {
	return s.ExportarTex == nil || *s.ExportarTex
}

func (s Settings) ExportarMarkdownResolto() bool {
	return s.ExportarMarkdown == nil || *s.ExportarMarkdown
}

// DecimaisResolto devolve Decimais coa regra de "0/sen configurar -> 2"
// xa aplicada, para pasarlle un valor listo a cas.Maxima.SetDecimais.
func (s Settings) DecimaisResolto() int {
	if s.Decimais <= 0 {
		return 2
	}
	return s.Decimais
}

// IAMaxTokensResolto devolve IAMaxTokens coa regra de "<=0 -> 0" (omitir o
// parámetro por completo) - para chamarGemini/chamarOpenAICompatible
// (ia_client.go), cuxas APIs aceptan omitir o límite de resposta e deixar
// que o decida o propio modelo. Simplemente normaliza un valor negativo (a
// man no settings.json, por exemplo) á mesma omisión ca 0 - un negativo
// real nunca chegaría dende a UI de Opcións (número mínimo 0 no <input>).
func (s Settings) IAMaxTokensResolto() int {
	if s.IAMaxTokens <= 0 {
		return 0
	}
	return s.IAMaxTokens
}

// IATimeoutResolto devolve IATimeoutSegundos coa regra "< 30s -> 240s"
// aplicada (0/sen configurar e calquera valor demasiado baixo caen no
// mesmo), listo para http.Client.Timeout en chamarIA.
func (s Settings) IATimeoutResolto() time.Duration {
	if s.IATimeoutSegundos < 30 {
		return 240 * time.Second
	}
	return time.Duration(s.IATimeoutSegundos) * time.Second
}

// IAMaxTokensAnthropicResolto devolve IAMaxTokens coa regra de "<=0 ->
// 8192" - para chamarAnthropic (ia_client.go), cuxa API esixe SEMPRE un
// valor numérico en max_tokens (non se pode omitir coma en Gemini/OpenAI);
// 8192 é o valor que xa levaba fixo no código antes de que isto fose
// configurable.
func (s Settings) IAMaxTokensAnthropicResolto() int {
	if s.IAMaxTokens <= 0 {
		return 8192
	}
	return s.IAMaxTokens
}

// TimeoutResolto devolve TimeoutSegundos coa regra de "0/sen configurar ->
// 30s" xa aplicada, listo para cas.Maxima.SetTimeout.
func (s Settings) TimeoutResolto() time.Duration {
	if s.TimeoutSegundos <= 0 {
		return 30 * time.Second
	}
	return time.Duration(s.TimeoutSegundos) * time.Second
}

// ReorganizarSementesResolto: "0/sen configurar -> 25", cap [1, 200] - mesmo
// intervalo ca ProbarExercicios (proba.go).
func (s Settings) ReorganizarSementesResolto() int {
	n := s.ReorganizarSementes
	if n <= 0 {
		n = 25
	}
	if n > 200 {
		n = 200
	}
	return n
}

// ReorganizarRoldasResolto: "0/sen configurar -> 3", cap [1, 6].
func (s Settings) ReorganizarRoldasResolto() int {
	n := s.ReorganizarRoldas
	if n <= 0 {
		n = 3
	}
	if n > 6 {
		n = 6
	}
	return n
}

// AutorepararExerciciosResolto: "sen configurar (nil) -> true", mesmo
// criterio ca XerarAoGardarResolto.
func (s Settings) AutorepararExerciciosResolto() bool {
	return s.AutorepararExercicios == nil || *s.AutorepararExercicios
}

// MaxRechamadasIAResolto: "0/sen configurar -> 2", cap [1, 6]. Igual ca
// ReorganizarRoldasResolto. Non ten "0 = desactivado": iso faise co check
// de AutorepararExercicios.
func (s Settings) MaxRechamadasIAResolto() int {
	n := s.MaxRechamadasIA
	if n <= 0 {
		n = 2
	}
	if n > 6 {
		n = 6
	}
	return n
}

func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "yang")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func loadSettings() Settings {
	s := Settings{MaximaPath: findMaxima()}
	path, err := settingsPath()
	if err != nil {
		return s
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var loaded Settings
	if json.Unmarshal(b, &loaded) == nil {
		if loaded.MaximaPath != "" {
			s.MaximaPath = loaded.MaximaPath
		}
		s.CodeIni = loaded.CodeIni
		s.IAProvedor = loaded.IAProvedor
		s.IAAPIKey = loaded.IAAPIKey
		s.IAModel = loaded.IAModel
		s.IABaseURL = loaded.IABaseURL
		s.IAPreset = loaded.IAPreset
		s.IAMaxTokens = loaded.IAMaxTokens
		s.LatexEngine = loaded.LatexEngine
		s.Decimais = loaded.Decimais
		s.Decimal = loaded.Decimal
		s.TimeoutSegundos = loaded.TimeoutSegundos
		s.ReorganizarSementes = loaded.ReorganizarSementes
		s.ReorganizarRoldas = loaded.ReorganizarRoldas
		s.IATimeoutSegundos = loaded.IATimeoutSegundos
		s.Idioma = loaded.Idioma
		s.XerarAoGardar = loaded.XerarAoGardar
		s.ExportarPDF = loaded.ExportarPDF
		s.ExportarTex = loaded.ExportarTex
		s.ExportarMarkdown = loaded.ExportarMarkdown
		s.ExportarDocx = loaded.ExportarDocx
		s.ExportarOdt = loaded.ExportarOdt
		s.EditorMode = loaded.EditorMode
	}
	migrarAxustesIAVellos(b, &s)
	return s
}

// migrarAxustesIAVellos le un settings.json gardado por unha versión
// anterior a esta (só tiña geminiApiKey/geminiModel, sempre Gemini, sen os
// campos IA* xenéricos de enriba) e completa eses campos novos se aínda
// están baleiros - así un profesor que xa tiña a IA configurada non ten
// que volver escribir a clave despois de actualizar Yang. Non toca nada se
// xa hai IAAPIKey (configuración xa migrada ou feita coa versión nova).
func migrarAxustesIAVellos(b []byte, s *Settings) {
	if s.IAAPIKey != "" {
		return
	}
	var vello struct {
		GeminiAPIKey string `json:"geminiApiKey"`
		GeminiModel  string `json:"geminiModel"`
	}
	if json.Unmarshal(b, &vello) != nil || vello.GeminiAPIKey == "" {
		return
	}
	s.IAProvedor = string(ProvedorGemini)
	s.IAPreset = string(ProvedorGemini)
	s.IAAPIKey = vello.GeminiAPIKey
	s.IAModel = vello.GeminiModel
}

func saveSettingsToDisk(s Settings) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// findMaxima looks for the maxima binary in common locations and finally
// falls back to a PATH lookup, mirroring the fallback chain in
// TMainForm.run / TMaxima.CAS_open.
func findMaxima() string {
	// /opt/homebrew/bin é onde `brew install maxima` deixa o binario nun Mac
	// con Apple Silicon (en Intel usa /usr/local/bin, xa cuberto arriba).
	// Míranse os dous en calquera plataforma: un os.Stat que falla non custa
	// nada e evita ter que ramificar por GOOS aquí.
	candidates := []string{"/usr/bin/maxima", "/usr/local/bin/maxima", "/opt/homebrew/bin/maxima"}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	// O instalador oficial de Maxima para Windows (e o paquete winget
	// "MaximaTeam.Maxima", que é o mesmo instalador) non sempre engade
	// maxima.bat ao PATH do sistema - así que, sen isto, un profesor que
	// acaba de instalar Maxima seguiría vendo "non atopado" ata reiniciar
	// sesión. Busca en C:\maxima-<versión>\bin\maxima.bat e equivalentes
	// baixo Program Files antes de renderse ao PATH. filepath.Glob nunca
	// falla por non atopar nada (devolve unha lista baleira), así que isto
	// é un fallback seguro noutras plataformas tamén.
	if runtime.GOOS == "windows" {
		var patrons []string
		for _, base := range []string{
			`C:\`,
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"),
		} {
			if base == "" {
				continue
			}
			patrons = append(patrons, filepath.Join(base, "maxima-*", "bin", "maxima.bat"))
			patrons = append(patrons, filepath.Join(base, "Maxima-*", "bin", "maxima.bat"))
		}
		var atopados []string
		for _, patron := range patrons {
			coincidencias, _ := filepath.Glob(patron)
			atopados = append(atopados, coincidencias...)
		}
		if len(atopados) > 0 {
			// Se hai varias versións instaladas, quedar coa máis recente
			// (orde alfabética do número de versión no cartafol, ex.
			// "maxima-5.47.0" > "maxima-5.46.0").
			sort.Strings(atopados)
			return atopados[len(atopados)-1]
		}
	}
	if p, err := exec.LookPath("maxima"); err == nil {
		return p
	}
	return ""
}

// GetSettings returns the currently active settings, exposed to the UI's
// options panel.
func (a *App) GetSettings() Settings {
	return a.settings
}

// SaveSettings persists new settings and applies them immediately.
func (a *App) SaveSettings(s Settings) error {
	a.settings = s
	a.maximaPath = s.MaximaPath
	return saveSettingsToDisk(s)
}

// DetectMaxima re-runs the auto-detection, for a "detectar" button in the
// options panel.
func (a *App) DetectMaxima() string {
	return findMaxima()
}
