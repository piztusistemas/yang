package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

//go:embed idiomas
var idiomasFS embed.FS

const idiomaFallback = "gl"

// IdiomaInfo: código → nome amosado, exposto ao frontend para construír o
// selector de idioma. Mesmo patrón que installer/i18n.go.
type IdiomaInfo struct {
	Code string `json:"code"`
	Nome string `json:"nome"`
}

var idiomasDispo = []IdiomaInfo{
	{"gl", "Galego"},
	{"es", "Castellano"},
	{"en", "English"},
	{"pt", "Português"},
}

// Translator carga e cachea os dicionarios de idioma do instalador. Porte
// directo de installer/i18n.go.
type Translator struct {
	mu    sync.Mutex
	code  string
	cache map[string]map[string]string
}

func NewTranslator(code string) *Translator {
	if strings.TrimSpace(code) == "" {
		code = idiomaFallback
	}
	return &Translator{code: code, cache: map[string]map[string]string{}}
}

func (tr *Translator) SetIdioma(code string) {
	if strings.TrimSpace(code) == "" {
		code = idiomaFallback
	}
	tr.mu.Lock()
	tr.code = code
	tr.mu.Unlock()
}

func (tr *Translator) Idioma() string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.code
}

// T traduce `clave` co idioma activo; se falta, proba galego e por último
// devolve a propia clave. Con args aplica formato estilo printf (as cadeas
// dos .lang xa usan %s/%d ao estilo Go).
func (tr *Translator) T(clave string, args ...any) string {
	valor, ok := tr.cargar(tr.Idioma())[clave]
	if !ok && tr.Idioma() != idiomaFallback {
		valor, ok = tr.cargar(idiomaFallback)[clave]
	}
	if !ok {
		valor = clave
	}
	if len(args) > 0 {
		valor = fmt.Sprintf(valor, args...)
	}
	return valor
}

// All devolve o dicionario completo (fallback + idioma activo mesturados),
// para que o frontend traduza os textos estáticos (botóns, títulos...).
func (tr *Translator) All() map[string]string {
	res := map[string]string{}
	for k, v := range tr.cargar(idiomaFallback) {
		res[k] = v
	}
	if tr.Idioma() != idiomaFallback {
		for k, v := range tr.cargar(tr.Idioma()) {
			res[k] = v
		}
	}
	return res
}

func (tr *Translator) cargar(code string) map[string]string {
	tr.mu.Lock()
	if m, ok := tr.cache[code]; ok {
		tr.mu.Unlock()
		return m
	}
	tr.mu.Unlock()

	data, err := lerIdioma(code)
	m := map[string]string{}
	if err == nil {
		m = aplicarVariantesSO(parseLang(data))
	}
	tr.mu.Lock()
	tr.cache[code] = m
	tr.mu.Unlock()
	return m
}

// lerIdioma devolve o contido do ficheiro do idioma: ficheiro externo (ao
// carón do executable, editable polo usuario) se existe, senón o embebido.
func lerIdioma(code string) ([]byte, error) {
	if exe, err := os.Executable(); err == nil {
		ext := filepath.Join(filepath.Dir(exe), "idiomas", code+".lang")
		if data, err := os.ReadFile(ext); err == nil {
			return data, nil
		}
	}
	return idiomasFS.ReadFile("idiomas/" + code + ".lang")
}

// sufixosSO son os sufixos de variante recoñecidos nos ficheiros .lang.
// Lístanse explicitamente (en vez de tratar calquera ".algo" coma unha
// variante) para que unha clave normal que por casualidade remate nunha
// desas palabras non se coma por erro.
var sufixosSO = []string{".linux", ".darwin", ".windows"}

// aplicarVariantesSO resolve as claves con sufixo de sistema operativo:
// "benvida.aviso.darwin" substitúe a "benvida.aviso" cando se executa nun
// Mac, e todas as variantes (a que gañou e as demais) desaparecen do
// dicionario resultante.
//
// Por que fai falla: varios textos do instalador afirmaban cousas certas só
// en Linux - que hai que executalo con sudo, que Yang vai parar a
// /opt/piztu/modulos, que se crean accesos directos no escritorio e no menú
// de aplicacións. Nada diso é certo nun Mac (instálase en
// /Applications/Yang.app, sen sudo, e o .app xa é o acceso directo), e
// seguir dicíndoo levaría o profesorado a facer o contrario do que debe -
// "executa isto con sudo" en macOS é ademais un consello activamente malo,
// porque Homebrew négase a funcionar coma root.
//
// Faise aquí, ao cargar o dicionario, e non no frontend, para que tanto
// Translator.T (Go) coma Traducions() (o dicionario que consume main.js)
// vexan xa o texto correcto sen ter que saber nada disto.
func aplicarVariantesSO(m map[string]string) map[string]string {
	meu := "." + runtime.GOOS
	for clave, valor := range m {
		for _, sufixo := range sufixosSO {
			if !strings.HasSuffix(clave, sufixo) {
				continue
			}
			if sufixo == meu {
				m[strings.TrimSuffix(clave, sufixo)] = valor
			}
			delete(m, clave)
			break
		}
	}
	return m
}

func parseLang(data []byte) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		clave := strings.TrimSpace(line[:eq])
		valor := strings.TrimSpace(line[eq+1:])
		valor = strings.ReplaceAll(valor, "\\n", "\n")
		m[clave] = valor
	}
	return m
}
