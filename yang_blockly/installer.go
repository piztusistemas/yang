package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// InstallStatus reports the outcome of an install attempt back to the UI.
type InstallStatus struct {
	Success bool   `json:"success"`
	Log     string `json:"log"`
}

// eventoProgresoInstalacion é o nome do evento co que InstallMaxima/
// InstallLatex/InstallPandoc informan do seu avance mentres corren.
//
// Fai falla porque eses tres métodos son SÍNCRONOS: o frontend agarda a que
// devolvan InstallStatus, así que ata que rematan non ten nada que amosar
// máis alá dun "⏳ Instalando…" fixo. Iso vale para un `apt install pandoc`
// de vinte segundos, pero non para `brew install --cask mactex-no-gui`, que
// son preto de 6 GB e pode pasar media hora sen dar sinal ningún (ver
// progreso_darwin.go). Sen isto, o botón queda conxelado e o natural é
// pensar que Yang colgou e pechalo a metade.
const eventoProgresoInstalacion = "yang:instalacion-progreso"

// ProgresoInstalacion é o que viaxa no evento. Que é a ferramenta que se
// está a instalar ("maxima", "latex" ou "pandoc"), para que o frontend saiba
// que botón actualizar.
type ProgresoInstalacion struct {
	Que      string `json:"que"`
	Minutos  int    `json:"minutos"`
	MB       int64  `json:"mb"` // 0 = descoñecido (ver mbDescargaEnCurso)
	Completo bool   `json:"completo"`
}

// informarProgresoInstalacion emite un evento de progreso cada 30 segundos
// mentres dure a instalación, e devolve a función que o detén. O primeiro
// evento vai decontado, para que o botón deixe de amosar un texto fixo sen
// agardar medio minuto.
func informarProgresoInstalacion(que string) (parar func()) {
	feito := make(chan struct{})
	rematado := make(chan struct{})
	inicio := time.Now()

	emitir := func(completo bool) {
		// application.Get() é nil nun test que chame o binding directamente
		// (mesma precaución que en xanelaresultado.go e app.go).
		app := application.Get()
		if app == nil {
			return
		}
		app.Event.Emit(eventoProgresoInstalacion, ProgresoInstalacion{
			Que:      que,
			Minutos:  int(time.Since(inicio).Minutes()),
			MB:       mbDescargaEnCurso(),
			Completo: completo,
		})
	}

	emitir(false)
	go func() {
		defer close(rematado)
		tic := time.NewTicker(30 * time.Second)
		defer tic.Stop()
		for {
			select {
			case <-feito:
				emitir(true)
				return
			case <-tic.C:
				emitir(false)
			}
		}
	}()

	return func() {
		close(feito)
		<-rematado
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// linuxInstallCommand picks the first available package manager and
// returns the full command line to install Maxima + gnuplot with it.
// apt gets "update &&" first since a stale package cache is the most
// common reason a fresh install fails; the others normally auto-refresh.
func linuxInstallCommand() (argv []string, mgrName string, ok bool) {
	switch {
	case commandExists("apt-get"):
		return []string{"sh", "-c", "apt-get update && apt-get install -y maxima maxima-share gnuplot"}, "apt-get", true
	case commandExists("dnf"):
		return []string{"dnf", "install", "-y", "maxima", "gnuplot"}, "dnf", true
	case commandExists("pacman"):
		return []string{"pacman", "-Sy", "--noconfirm", "maxima", "gnuplot"}, "pacman", true
	case commandExists("zypper"):
		return []string{"zypper", "install", "-y", "maxima", "gnuplot"}, "zypper", true
	}
	return nil, "", false
}

// linuxLatexInstallCommand mirrors linuxInstallCommand for the LaTeX
// toolchain: pdflatex (base+recommended, covers amsmath/amssymb/xcolor/
// geometry), TikZ (its own package on Debian/Ubuntu - texlive-latex-base
// alone does NOT include it, confirmed on this machine's dpkg -S), xelatex/
// lualatex (both optional engines offered in Opcións), and poppler-utils
// (pdftoppm - needed for the print preview and for <TIKZ> in HTML mode).
// Deliberately NOT texlive-full: that pulls in several GB of packages
// (fonts/languages Yang never uses).
//
// texlive-latex-extra (Debian/Ubuntu só) vai polas PLANTILLAS de documento
// (plantillas.go): tcolorbox, titlesec, adjustbox e compañía viven aí, e son
// o pan de cada día de calquera maqueta traballada - incluídas as que
// escribe a IA a partir dun modelo. Mesma lista ca o instalador de fóra
// (yang_installer/install.go), que ten que instalar exactamente o mesmo.
func linuxLatexInstallCommand() (argv []string, mgrName string, ok bool) {
	switch {
	case commandExists("apt-get"):
		return []string{"sh", "-c", "apt-get update && apt-get install -y texlive-latex-base texlive-latex-recommended texlive-pictures texlive-latex-extra texlive-xetex texlive-luatex poppler-utils"}, "apt-get", true
	case commandExists("dnf"):
		return []string{"dnf", "install", "-y", "texlive-latex", "texlive-collection-pictures", "texlive-xetex", "texlive-luatex", "poppler-utils"}, "dnf", true
	case commandExists("pacman"):
		return []string{"pacman", "-Sy", "--noconfirm", "texlive-core", "texlive-pictures", "poppler"}, "pacman", true
	case commandExists("zypper"):
		return []string{"zypper", "install", "-y", "texlive", "texlive-latex", "poppler-tools"}, "zypper", true
	}
	return nil, "", false
}

// linuxPandocInstallCommand mirrors linuxInstallCommand/linuxLatexInstallCommand
// for Pandoc - the only extra tool ExportDocx/ExportOdt (docdoc.go) need,
// independent of the Maxima/LaTeX chain (a professor may want Word/
// OpenDocument output without ever touching PDF).
func linuxPandocInstallCommand() (argv []string, mgrName string, ok bool) {
	switch {
	case commandExists("apt-get"):
		return []string{"sh", "-c", "apt-get update && apt-get install -y pandoc"}, "apt-get", true
	case commandExists("dnf"):
		return []string{"dnf", "install", "-y", "pandoc"}, "dnf", true
	case commandExists("pacman"):
		return []string{"pacman", "-Sy", "--noconfirm", "pandoc"}, "pacman", true
	case commandExists("zypper"):
		return []string{"zypper", "install", "-y", "pandoc"}, "zypper", true
	}
	return nil, "", false
}

// DocDepsStatus mirrors LatexDepsStatus, for the DOCX/ODT export chain
// (docdoc.go) instead of the PDF one.
type DocDepsStatus struct {
	PandocFound bool `json:"pandocFound"`
}

// CheckDocDeps reports whether Pandoc is installed - see exportViaPandoc
// (docdoc.go), the only thing ExportDocx/ExportOdt actually need beyond
// what GeneratePDF already requires (Maxima).
func (a *App) CheckDocDeps() DocDepsStatus {
	return DocDepsStatus{PandocFound: commandExists("pandoc")}
}

// InstallPandoc mirrors InstallMaxima/InstallLatex exactly (same pkexec/
// winget/brew pattern, same "only runs when the user explicitly asks" rule)
// for Pandoc instead - see linuxPandocInstallCommand for what gets installed.
func (a *App) InstallPandoc() InstallStatus {
	var cmd *exec.Cmd
	var logPrevio string
	limparEntorno := func() {}
	defer func() { limparEntorno() }() // ver InstallMaxima

	switch runtime.GOOS {
	case "linux":
		argv, mgrName, ok := linuxPandocInstallCommand()
		if !ok {
			return InstallStatus{Log: "Non se atopou un xestor de paquetes coñecido (apt/dnf/pacman/zypper). Instala Pandoc manualmente."}
		}
		if !commandExists("pkexec") {
			return InstallStatus{Log: fmt.Sprintf("Non se atopou 'pkexec' para pedir permisos de administrador. Instala manualmente nun terminal cos comandos de %s.", mgrName)}
		}
		cmd = exec.Command("pkexec", argv...)

	case "windows":
		if !commandExists("winget") {
			return InstallStatus{Log: "Non se atopou winget. Descarga Pandoc manualmente desde https://pandoc.org/installing.html"}
		}
		cmd = exec.Command("winget", "install", "--id", "JohnMacFarlane.Pandoc", "-e",
			"--silent", "--accept-source-agreements", "--accept-package-agreements")
		ocultarConsola(cmd)

	case "darwin":
		logBrew, err := asegurarHomebrew()
		if err != nil {
			return InstallStatus{Log: logBrew + err.Error()}
		}
		logPrevio = logBrew
		cmd = exec.Command("brew", "install", "pandoc")
		cmd.Env, limparEntorno = prepararEntornoInstalacion(
			"Yang precisa permisos de administrador para instalar Pandoc.")

	default:
		return InstallStatus{Log: "Sistema operativo non recoñecido para instalación automática."}
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	pararProgreso := informarProgresoInstalacion("pandoc")
	runErr := cmd.Run()
	pararProgreso()
	log := strings.TrimSpace(logPrevio + out.String())
	refrescarPathDendeRexistro()

	// Re-detect regardless of the reported exit code, mesmo criterio que
	// InstallMaxima/InstallLatex.
	status := a.CheckDocDeps()
	if !status.PandocFound {
		if runErr != nil {
			log += "\n" + runErr.Error()
		}
		return InstallStatus{Success: false, Log: log}
	}
	return InstallStatus{Success: true, Log: log}
}

// LatexDepsStatus reports whether the tools Yang needs to compile/preview
// PDFs are installed - same idea as checking a.maximaPath, but for the rest
// of the compilation chain: the LaTeX engine currently selected in Opcións
// (GeneratePDF, latexdoc.go, fails with an error mentioning this same
// engine if it's missing) plus pdftoppm (poppler-utils), needed for the
// print preview (pdfPageImages) and for <TIKZ> in HTML mode (cas/tags.go).
type LatexDepsStatus struct {
	Engine        string `json:"engine"`
	EngineFound   bool   `json:"engineFound"`
	PdftoppmFound bool   `json:"pdftoppmFound"`
}

// OK reports whether everything Yang needs to compile a PDF is present -
// exposed as a method (not just a computed JS boolean) so the frontend
// doesn't have to duplicate this exact condition.
func (s LatexDepsStatus) OK() bool { return s.EngineFound && s.PdftoppmFound }

// CheckLatexDeps re-derives the configured engine the same way GeneratePDF
// does (empty setting = pdflatex), so switching engines in Opcións updates
// what gets checked without needing anything else to change.
func (a *App) CheckLatexDeps() LatexDepsStatus {
	engine := strings.TrimSpace(a.settings.LatexEngine)
	if engine == "" {
		engine = "pdflatex"
	}
	return LatexDepsStatus{
		Engine:        engine,
		EngineFound:   commandExists(engine),
		PdftoppmFound: commandExists("pdftoppm"),
	}
}

// InstallMaxima tries to install Maxima (+gnuplot for <PLOT>) using the
// platform's native package manager. On Linux it goes through pkexec so
// this GUI app never needs a terminal to prompt for a sudo password - the
// desktop's own polkit agent shows the native authentication dialog. It
// only runs when the user explicitly asks for it from the UI.
func (a *App) InstallMaxima() InstallStatus {
	var cmd *exec.Cmd
	// logPrevio/limparEntorno só se usan no camiño de macOS (ver
	// brew_darwin.go): o primeiro arrastra o log de ter instalado Homebrew
	// antes de poder instalar nada con el, e o segundo borra o programa
	// askpass temporal. Envólvese nun closure porque limparEntorno aínda non
	// ten o seu valor definitivo cando se avalía o defer.
	var logPrevio string
	limparEntorno := func() {}
	defer func() { limparEntorno() }()

	switch runtime.GOOS {
	case "linux":
		argv, mgrName, ok := linuxInstallCommand()
		if !ok {
			return InstallStatus{Log: "Non se atopou un xestor de paquetes coñecido (apt/dnf/pacman/zypper). Instala Maxima manualmente."}
		}
		if !commandExists("pkexec") {
			return InstallStatus{Log: fmt.Sprintf("Non se atopou 'pkexec' para pedir permisos de administrador. Instala manualmente nun terminal cos comandos de %s.", mgrName)}
		}
		cmd = exec.Command("pkexec", argv...)

	case "windows":
		if !commandExists("winget") {
			return InstallStatus{Log: "Non se atopou winget. Descarga Maxima manualmente desde https://maxima.sourceforge.io"}
		}
		// ID real en winget: "MaximaTeam.Maxima" - "Maxima.Maxima" (o que
		// parecería lóxico, e o que había aquí antes) NON existe, winget
		// devolve "No se encontró ningún paquete..." e a instalación falla
		// sempre. Comprobado con `winget search maxima`.
		cmd = exec.Command("winget", "install", "--id", "MaximaTeam.Maxima", "-e",
			"--silent", "--accept-source-agreements", "--accept-package-agreements")
		ocultarConsola(cmd)

	case "darwin":
		logBrew, err := asegurarHomebrew()
		if err != nil {
			return InstallStatus{Log: logBrew + err.Error()}
		}
		logPrevio = logBrew
		cmd = exec.Command("brew", "install", "maxima", "gnuplot")
		cmd.Env, limparEntorno = prepararEntornoInstalacion(
			"Yang precisa permisos de administrador para instalar Maxima.")

	default:
		return InstallStatus{Log: "Sistema operativo non recoñecido para instalación automática."}
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	pararProgreso := informarProgresoInstalacion("maxima")
	runErr := cmd.Run()
	pararProgreso()
	log := strings.TrimSpace(logPrevio + out.String())

	// En Windows, winget pode ter engadido maxima.bat ao PATH do Rexistro
	// sen que este proceso (xa arrincado) o vexa aínda - ver
	// pathrefresh_windows.go. No-op noutras plataformas.
	refrescarPathDendeRexistro()

	// Rexistra o texmf de gnuplot en MiKTeX se xa está instalado (ver
	// latexenv_windows.go) - se InstallLatex aínda non se executou, esta
	// chamada non fai nada (initexmf non atopado) e InstallLatex xa o
	// intentará ela mesma despois. No-op fóra de Windows.
	prepararGnuplotTikZ(func(s string) { log += "\n" + s })

	// Re-detect regardless of the reported exit code - some package
	// managers exit non-zero on warnings even when the install succeeded.
	a.settings.MaximaPath = findMaxima()
	a.maximaPath = a.settings.MaximaPath
	saveSettingsToDisk(a.settings)

	if a.maximaPath == "" {
		if runErr != nil {
			log += "\n" + runErr.Error()
		}
		return InstallStatus{Success: false, Log: log}
	}
	return InstallStatus{Success: true, Log: log}
}

// InstallLatex mirrors InstallMaxima exactly (same pkexec/winget/brew
// pattern, same "only runs when the user explicitly asks" rule) for the
// LaTeX toolchain instead - see linuxLatexInstallCommand for what actually
// gets installed and why.
func (a *App) InstallLatex() InstallStatus {
	var cmd *exec.Cmd
	var logPrevio string
	limparEntorno := func() {}
	defer func() { limparEntorno() }() // ver InstallMaxima

	switch runtime.GOOS {
	case "linux":
		argv, mgrName, ok := linuxLatexInstallCommand()
		if !ok {
			return InstallStatus{Log: "Non se atopou un xestor de paquetes coñecido (apt/dnf/pacman/zypper). Instala LaTeX manualmente."}
		}
		if !commandExists("pkexec") {
			return InstallStatus{Log: fmt.Sprintf("Non se atopou 'pkexec' para pedir permisos de administrador. Instala manualmente nun terminal cos comandos de %s.", mgrName)}
		}
		cmd = exec.Command("pkexec", argv...)

	case "windows":
		if !commandExists("winget") {
			return InstallStatus{Log: "Non se atopou winget. Instala MiKTeX manualmente desde https://miktex.org"}
		}
		cmd = exec.Command("winget", "install", "--id", "MiKTeX.MiKTeX", "-e",
			"--silent", "--accept-source-agreements", "--accept-package-agreements")
		ocultarConsola(cmd)

	case "darwin":
		logBrew, err := asegurarHomebrew()
		if err != nil {
			return InstallStatus{Log: logBrew + err.Error()}
		}
		logPrevio = logBrew
		// mactex-no-gui é un cask de tipo .pkg: Homebrew instálao chamando a
		// `sudo` por dentro. Sen SUDO_ASKPASS iso falla decontado ("no tty
		// present and no askpass program specified") porque Yang é unha app
		// de GUI e nunca ten terminal - de aí prepararEntornoInstalacion
		// (ver brew_darwin.go), que amosa o diálogo nativo de contrasinal.
		cmd = exec.Command("brew", "install", "--cask", "mactex-no-gui")
		cmd.Env, limparEntorno = prepararEntornoInstalacion(
			"Yang precisa permisos de administrador para instalar MacTeX (a distribución de LaTeX).")

	default:
		return InstallStatus{Log: "Sistema operativo non recoñecido para instalación automática."}
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	pararProgreso := informarProgresoInstalacion("latex")
	runErr := cmd.Run()
	pararProgreso()
	log := strings.TrimSpace(logPrevio + out.String())
	// Ademais do PATH do Rexistro en Windows, isto é o que fai que en macOS
	// se vexa pdflatex xusto despois de instalar MacTeX sen reiniciar Yang:
	// o .pkg crea /Library/TeX/texbin e rexístrao en /etc/paths.d/TeX, pero
	// o PATH deste proceso quedou fixado ao arrincar (ver
	// pathrefresh_darwin.go). Sen isto, CheckLatexDeps de embaixo seguiría a
	// dicir que non hai motor de LaTeX e o botón daría a instalación por
	// fallida aínda tendo ido ben.
	refrescarPathDendeRexistro()

	// Rexistra o texmf de gnuplot en MiKTeX se Maxima xa está instalado (ver
	// latexenv_windows.go e InstallMaxima enriba, que fai a mesma chamada
	// pola orde contraria). No-op fóra de Windows.
	prepararGnuplotTikZ(func(s string) { log += "\n" + s })

	// Re-detect regardless of the reported exit code, mesmo criterio que
	// InstallMaxima - algúns xestores de paquetes devolven un código non
	// cero por avisos aínda que a instalación fose ben.
	status := a.CheckLatexDeps()
	if !status.OK() {
		if runErr != nil {
			log += "\n" + runErr.Error()
		}
		return InstallStatus{Success: false, Log: log}
	}
	return InstallStatus{Success: true, Log: log}
}
