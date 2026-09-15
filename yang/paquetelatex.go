package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

// Instalación baixo demanda dun paquete de LaTeX que falta.
//
// Unha distribución de LaTeX completa son varios GB, así que Yang nunca a
// instala enteira (ver installer.go): pon o subconxunto que precisa e
// listo. O prezo é que, tarde ou cedo, alguén escribe (ou a IA xera) un
// documento ou unha PLANTILLA (plantillas.go) que usa un paquete que ese
// equipo non ten, e a compilación morre cun "File `tcolorbox.sty' not
// found" que non lle di nada ao profesorado.
//
// Este ficheiro pecha ese oco: cando unha compilación falla por un paquete
// que falta, anótase cal foi (anotarPaqueteQueFalta) e o frontend ofrece un
// botón para instalalo, co mesmo camiño de permisos que xa usan
// InstallMaxima/InstallLatex (pkexec en Linux, diálogo nativo de
// administrador en macOS).

// Detección UNIVERSAL de "falta algo de LaTeX que se pode instalar". Un log
// de LaTeX pártese en liñas de ~80 caracteres, así que "not found" pode
// aparecer cortado ("not foun\nd"); anotarPaqueteQueFalta normaliza os
// espazos antes de aplicar estas expresións, por iso aquí non se depende de
// que dúas palabras vaian xuntas.
var (
	// ficheiroQueFaltaRe: "File `X.EXT' not found" para calquera extensión
	// que un paquete de TeX Live poida fornecer (paquete .sty, clase .cls,
	// idioma de babel .ldf, definicións .def/.fd/.clo, config .cfg, fontes
	// .ttf/.otf, codificación .enc, input .tex).
	ficheiroQueFaltaRe = regexp.MustCompile(
		"File `([A-Za-z0-9@_.+ -]+\\.(?:sty|cls|ldf|def|fd|clo|sto|cfg|cmap|enc|ttf|otf|tex|bst|bbx|cbx))' not found")
	// babelIdiomaRe: babel dío doutro xeito cando falta o soporte dun idioma
	// -> "the language definition file spanish.ldf was not found" (ou o
	// galician.ldf, que á súa vez necesita spanish.ldf). Basta con recoñecer
	// "language definition file X.ldf": esa frase só aparece neste erro.
	babelIdiomaRe = regexp.MustCompile(`language definition file ([a-z@]+)\.ldf`)
	// fontspecFaltaRe: xelatex/lualatex non atopan unha fonte pedida por
	// fontspec (\setmainfont, \setmonofont...) nunha plantilla.
	fontspecFaltaRe = regexp.MustCompile(`The font "([^"\n]+)" cannot be found`)
	// espazosLogRe normaliza calquera bloque de espazos/saltos a un só
	// espazo, para que o corte de liña do log non rompa as expresións de
	// enriba.
	espazosLogRe = regexp.MustCompile(`\s+`)
)

// PaqueteLatexPendente é o que se lle amosa ao profesorado tras un fallo:
// que falta, se Yang o pode instalar neste equipo, e QUE comando vai
// executar exactamente (amósase sempre antes de premer: instalar software
// de sistema non se fai ás agachadas).
type PaqueteLatexPendente struct {
	Ficheiro     string `json:"ficheiro"` // "tcolorbox.sty"
	Nome         string `json:"nome"`     // "tcolorbox"
	PodeInstalar bool   `json:"podeInstalar"`
	Comando      string `json:"comando"`
	// Motivo explica por que NON se pode instalar automaticamente, cando
	// PodeInstalar é false (sen xestor de paquetes coñecido, sen pkexec...).
	Motivo string `json:"motivo"`
}

var paqueteFallanteMu sync.Mutex
var paqueteFallante string // nome do ficheiro, "" se non hai nada pendente

// anotarPaqueteQueFalta mira o log dunha compilación fallida e garda o
// ficheiro que faltou, para que o frontend poida ofrecer instalalo. Chámase
// dende TODOS os sitios que compilan (GeneratePDF, PrevisualizarPlantilla):
// o log non chega ao frontend cando hai erro (o binding de Wails só entrega
// o error, ver compileLatex), así que este é o único xeito de que a
// información sobreviva á chamada.
func anotarPaqueteQueFalta(log string) {
	norm := espazosLogRe.ReplaceAllString(log, " ")
	paqueteFallanteMu.Lock()
	defer paqueteFallanteMu.Unlock()
	switch {
	case ficheiroQueFaltaRe.MatchString(norm):
		paqueteFallante = strings.TrimSpace(ficheiroQueFaltaRe.FindStringSubmatch(norm)[1])
	case babelIdiomaRe.MatchString(norm):
		// Gárdase co sufixo .ldf para que comandoInstalarPaqueteLatex saiba
		// que é un idioma de babel (paquete texlive-lang-*), non un .sty.
		paqueteFallante = babelIdiomaRe.FindStringSubmatch(norm)[1] + ".ldf"
	case fontspecFaltaRe.MatchString(norm):
		// Sufixo .font: pseudo-extensión interna (non existe tal ficheiro),
		// só para que o resto do código saiba que é unha fonte.
		paqueteFallante = strings.TrimSpace(fontspecFaltaRe.FindStringSubmatch(norm)[1]) + ".font"
	default:
		paqueteFallante = ""
	}
}

// limparPaqueteQueFalta bórrao tras unha compilación correcta (ou tras
// instalalo): senón o botón "instalar" quedaría pendurado despois de que o
// problema xa non exista.
func limparPaqueteQueFalta() {
	paqueteFallanteMu.Lock()
	paqueteFallante = ""
	paqueteFallanteMu.Unlock()
}

// PaqueteLatexQueFalta devólvelle ao frontend o que quedou anotado na
// última compilación. Ficheiro baleiro = non hai nada que ofrecer.
func (a *App) PaqueteLatexQueFalta() PaqueteLatexPendente {
	paqueteFallanteMu.Lock()
	ficheiro := paqueteFallante
	paqueteFallanteMu.Unlock()
	if ficheiro == "" {
		return PaqueteLatexPendente{}
	}
	nome := nomeDesdeFicheiro(ficheiro)
	info := PaqueteLatexPendente{Ficheiro: ficheiro, Nome: nome}
	argv, descricion, motivo := comandoInstalarPaqueteLatex(ficheiro, nome)
	if argv == nil {
		info.Motivo = motivo
		return info
	}
	info.PodeInstalar = true
	info.Comando = descricion
	return info
}

// paquetesDebian traduce un paquete de CTAN ao paquete de Debian/Ubuntu que
// o contén. Debian NON reparte TeX Live por paquete de CTAN (a diferenza de
// openSUSE ou Fedora), senón en coleccións grandes, así que hai que mapear:
// aquí van as coleccións, e o que non estea nesta táboa cae en
// texlive-latex-extra, que é onde vive a inmensa maioría do que lle falta a
// un documento normal. Non se adiviña máis ca isto a propósito: un nome de
// paquete inventado faría fallar o apt-get enteiro.
var paquetesDebian = map[string]string{
	// Debuxo e gráficas (texlive-pictures).
	"pgfplots": "texlive-pictures", "circuitikz": "texlive-pictures",
	"tikz": "texlive-pictures", "pgf": "texlive-pictures",
	"gnuplot-lua-tikz": "texlive-pictures",
	// Ciencias (texlive-science).
	"siunitx": "texlive-science", "mhchem": "texlive-science",
	"chemfig": "texlive-science", "circuitikz-science": "texlive-science",
	// Fontes (texlive-fonts-extra).
	"fontawesome": "texlive-fonts-extra", "fontawesome5": "texlive-fonts-extra",
	"roboto": "texlive-fonts-extra", "inconsolata": "texlive-fonts-extra",
	"fira": "texlive-fonts-extra", "lato": "texlive-fonts-extra",
	// Básicos que xa deberían estar, pero por se acaso.
	"amsmath": "texlive-latex-recommended", "graphicx": "texlive-latex-base",
	"geometry": "texlive-latex-base", "hyperref": "texlive-latex-base",
	"babel": "texlive-latex-base", "fancyhdr": "texlive-latex-recommended",
	"booktabs": "texlive-latex-recommended",
	// Familias de fonte do botón "Axustes do texto" (ver presetsFonte en
	// latexdoc.go).
	"mathptmx": "texlive-latex-recommended", "helvet": "texlive-latex-recommended",
	"mathpazo": "texlive-latex-recommended",
}

const paqueteDebianPorDefecto = "texlive-latex-extra"

// nomeDesdeFicheiro saca o nome "limpo" (sen extensión) de calquera dos
// ficheiros que anotarPaqueteQueFalta pode gardar: "tcolorbox.sty" ->
// "tcolorbox", "spanish.ldf" -> "spanish", "Fira Sans.font" -> "Fira Sans".
func nomeDesdeFicheiro(f string) string {
	return strings.TrimSuffix(f, filepath.Ext(f))
}

// grupoBabel mapea un idioma de babel á colección de TeX Live que o trae.
// Debian/Arch reparten os idiomas por grupos (texlive-lang-spanish,
// texlive-lang-european...), non un por idioma; o que non teña grupo propio
// vive en "european" (galician, catalán, éuscaro, holandés, nórdicos,
// centroeuropeos...).
func grupoBabel(lang string) string {
	switch lang {
	case "spanish":
		return "spanish"
	case "french", "francais", "acadian", "canadien":
		return "french"
	case "portuguese", "portuges", "brazil", "brazilian":
		return "portuguese"
	case "german", "ngerman", "austrian", "naustrian", "swissgerman":
		return "german"
	case "english", "usenglish", "american", "british", "ukenglish", "canadian", "australian", "newzealand":
		return "english"
	case "italian":
		return "italian"
	case "russian", "ukrainian", "bulgarian", "serbianc":
		return "cyrillic"
	case "greek", "polutonikogreek", "ibygreek":
		return "greek"
	case "arabic", "farsi":
		return "arabic"
	case "japanese":
		return "japanese"
	case "chinese":
		return "chinese"
	case "korean":
		return "korean"
	default:
		return "european"
	}
}

// paqueteDebianPara traduce "que ficheiro falta" -> "paquete apt". Distingue
// por extensión: idiomas de babel (.ldf) van en texlive-lang-<grupo>, as
// fontes en texlive-fonts-extra, e o resto (.sty/.cls/.def/...) pasa pola
// táboa paquetesDebian ou cae na colección grande.
func paqueteDebianPara(ext, nome string) string {
	switch ext {
	case ".ldf":
		return "texlive-lang-" + grupoBabel(nome)
	case ".font", ".ttf", ".otf", ".enc":
		return "texlive-fonts-extra"
	}
	if deb := paquetesDebian[nome]; deb != "" {
		return deb
	}
	return paqueteDebianPorDefecto
}

// comandoInstalarPaqueteLatex arma o comando de instalación para ESTE
// equipo. Devolve argv=nil e un motivo lexible cando non hai forma de
// facelo automaticamente.
//
// Cada sistema resólvese do xeito máis exacto que permita:
//   - Fedora ten "provides" virtuais por ficheiro: dnf install "tex(x.sty)"
//     acerta sempre, sen táboas.
//   - openSUSE reparte TeX Live por paquete de CTAN: texlive-<nome>.
//   - Debian/Ubuntu reparte por coleccións: ver paquetesDebian.
//   - macOS (MacTeX/BasicTeX) e calquera TeX Live oficial traen tlmgr, que
//     instala por nome de paquete de CTAN.
func comandoInstalarPaqueteLatex(ficheiro, nome string) (argv []string, descricion, motivo string) {
	ext := strings.ToLower(filepath.Ext(ficheiro))
	// alvoCTAN: nome do paquete tal e como o coñecen tlmgr/mpm (CTAN). Para
	// un idioma de babel é "babel-<idioma>"; para unha fonte, a mellor
	// aproximación é o nome en minúsculas sen espazos.
	alvoCTAN := nome
	switch ext {
	case ".ldf":
		alvoCTAN = "babel-" + nome
	case ".font", ".ttf", ".otf":
		alvoCTAN = strings.ToLower(strings.ReplaceAll(nome, " ", ""))
	}

	switch runtime.GOOS {
	case "linux":
		var base []string
		switch {
		case commandExists("apt-get"):
			base = []string{"sh", "-c", "apt-get update && apt-get install -y " + paqueteDebianPara(ext, nome)}
		case commandExists("dnf"):
			// Fedora ten "provides" virtuais por ficheiro para .sty/.cls/.ldf/
			// .def/.fd... - acerta sen táboas. As fontes van pola colección.
			if ext == ".font" || ext == ".ttf" || ext == ".otf" {
				base = []string{"dnf", "install", "-y", "texlive-collection-fontsextra"}
			} else {
				base = []string{"dnf", "install", "-y", fmt.Sprintf("tex(%s)", ficheiro)}
			}
		case commandExists("zypper"):
			switch ext {
			case ".ldf":
				base = []string{"zypper", "install", "-y", "texlive-babel-" + nome}
			case ".font", ".ttf", ".otf":
				base = []string{"zypper", "install", "-y", "texlive-fonts-extra"}
			default:
				base = []string{"zypper", "install", "-y", "texlive-" + nome}
			}
		case commandExists("pacman"):
			switch ext {
			case ".ldf":
				base = []string{"pacman", "-Sy", "--noconfirm", "texlive-lang" + grupoBabel(nome)}
			case ".font", ".ttf", ".otf":
				base = []string{"pacman", "-Sy", "--noconfirm", "texlive-fontsextra"}
			default:
				base = []string{"pacman", "-Sy", "--noconfirm", "texlive-latexextra"}
			}
		default:
			return nil, "", "non se atopou un xestor de paquetes coñecido (apt/dnf/zypper/pacman)"
		}
		if !commandExists("pkexec") {
			return nil, "", "non se atopou 'pkexec' para pedir permisos de administrador; instálao nun terminal"
		}
		return append([]string{"pkexec"}, base...), strings.Join(base, " "), ""

	case "darwin":
		if !commandExists("tlmgr") {
			return nil, "", "non se atopou tlmgr (instala MacTeX dende Opcións)"
		}
		// sudo -A: MacTeX vive en /usr/local/texlive, propiedade de root.
		// O -A colle o askpass que prepara prepararEntornoInstalacion
		// (brew_darwin.go), o mesmo diálogo nativo que xa se usa para
		// instalar MacTeX ou Maxima.
		return []string{"sudo", "-A", "tlmgr", "install", alvoCTAN},
			"sudo tlmgr install " + alvoCTAN, ""

	case "windows":
		// MiKTeX xa instala paquetes que faltan el só (por defecto,
		// preguntando); se chegou aquí é que esa opción está desactivada ou
		// que é unha instalación de TeX Live. mpm é o xestor de MiKTeX.
		if commandExists("mpm") {
			return []string{"mpm", "--install=" + alvoCTAN}, "mpm --install=" + alvoCTAN, ""
		}
		if commandExists("tlmgr") {
			return []string{"tlmgr", "install", alvoCTAN}, "tlmgr install " + alvoCTAN, ""
		}
		return nil, "", "non se atopou mpm nin tlmgr (o xestor de paquetes de MiKTeX/TeX Live)"
	}
	return nil, "", "sistema operativo non recoñecido"
}

// InstalarPaqueteLatex instala o paquete anotado na última compilación
// fallida. Mesmo contrato ca InstallMaxima/InstallLatex (installer.go):
// devolve InstallStatus e vai informando do progreso polo mesmo evento, así
// que o frontend non precisa nada novo para amosar "instalando...".
func (a *App) InstalarPaqueteLatex() InstallStatus {
	info := a.PaqueteLatexQueFalta()
	if info.Ficheiro == "" {
		return InstallStatus{Log: "Non hai ningún paquete pendente de instalar."}
	}
	argv, _, motivo := comandoInstalarPaqueteLatex(info.Ficheiro, info.Nome)
	if argv == nil {
		return InstallStatus{Log: "Non se pode instalar automaticamente: " + motivo}
	}

	env, limpar := prepararEntornoInstalacion(
		fmt.Sprintf("Yang precisa permisos de administrador para instalar o paquete de LaTeX «%s».", info.Nome))
	defer limpar()

	cmd := exec.Command(argv[0], argv[1:]...)
	if env != nil {
		cmd.Env = env
	}
	ocultarConsola(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	parar := informarProgresoInstalacion("paquete-latex")
	runErr := cmd.Run()
	parar()

	log := strings.TrimSpace(out.String())
	// Comprobación real por riba do código de saída: o que importa é que
	// LaTeX atope xa o ficheiro (algúns xestores devolven != 0 por avisos).
	// Para .sty/.cls/.ldf/.def... kpsewhich sábeo; para unha fonte (.font é
	// unha pseudo-extensión interna, non hai tal ficheiro) hai que fiarse do
	// código de saída do xestor de paquetes.
	ext := strings.ToLower(filepath.Ext(info.Ficheiro))
	instalado := kpsewhichFound(info.Ficheiro)
	if ext == ".font" || ext == ".ttf" || ext == ".otf" {
		instalado = runErr == nil
	}
	if instalado {
		limparPaqueteQueFalta()
		return InstallStatus{Success: true, Log: log}
	}
	if runErr != nil {
		log = strings.TrimSpace(log + "\n" + runErr.Error())
	}
	return InstallStatus{Log: log}
}
