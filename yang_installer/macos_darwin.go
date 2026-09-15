//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Este ficheiro concentra todo o que o instalador fai distinto nun Mac.
// Resumo do modelo de instalación en macOS, que non se parece nin ao de
// Linux nin ao de Windows:
//
//   - Yang instálase coma /Applications/Yang.app (ver defaultDestDir,
//     install.go). Non hai "cartafol de módulos de Piztu": Piztu é un sistema
//     Linux para centros e non existe en macOS, igual que non existe en
//     Windows.
//   - NON se eleva a root (ver intentarElevar, elevate_normal.go).
//     /Applications é escribible polo usuario administrador habitual dun
//     Mac, e Homebrew NÉGASE a executarse coma root, así que elevar todo o
//     proceso rompería precisamente o paso de instalar as dependencias.
//     O único que precisa contrasinal é MacTeX (un .pkg), e pídeo el só a
//     través do askpass de máis abaixo.
//   - Non hai accesos directos que crear: un .app en /Applications xa
//     aparece en Launchpad, en Spotlight e no Finder. O equivalente a
//     "crear accesos directos" é rexistralo en Launch Services.

// ── PATH ────────────────────────────────────────────────────────────────────

// refrescarPathMacOS engade ao PATH deste proceso os directorios onde vive
// Homebrew e as ferramentas instaladas por .pkg. Mesmo problema e mesma
// solución ca en yang_blockly/pathrefresh_darwin.go (módulo Go á parte, sen
// import compartido posible, así que se duplica aquí - mesmo criterio xa
// usado con comandoOculto e registrarGnuplotTexmfMiKTeX).
//
// Sen isto, `brew` non se atopa cando o instalador se abre facendo dobre
// clic no .app: launchd dálle un PATH mínimo (/usr/bin:/bin:/usr/sbin:/sbin)
// que non inclúe /opt/homebrew/bin.
func refrescarPathMacOS() {
	var novo []string
	vistos := map[string]bool{}
	engadir := func(dir string, comprobar bool) {
		dir = strings.TrimSpace(dir)
		if dir == "" || vistos[dir] {
			return
		}
		if comprobar {
			if st, err := os.Stat(dir); err != nil || !st.IsDir() {
				return
			}
		}
		vistos[dir] = true
		novo = append(novo, dir)
	}

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		engadir(dir, false)
	}
	// /etc/paths e /etc/paths.d/*: as mesmas fontes que le
	// /usr/libexec/path_helper para construír o PATH das shells de login. É
	// por aí por onde un .pkg (MacTeX, por exemplo) se engade ao PATH de
	// todo o sistema.
	for _, ruta := range ficheirosPathMacOS() {
		data, err := os.ReadFile(ruta)
		if err != nil {
			continue
		}
		for _, liña := range strings.Split(string(data), "\n") {
			if d := strings.TrimSpace(liña); d != "" && !strings.HasPrefix(d, "#") {
				engadir(d, true)
			}
		}
	}
	for _, dir := range []string{
		"/opt/homebrew/bin", "/opt/homebrew/sbin", // Homebrew en Apple Silicon
		"/usr/local/bin", "/usr/local/sbin", // Homebrew en Intel
		"/Library/TeX/texbin", // MacTeX/BasicTeX
		"/opt/local/bin", "/opt/local/sbin", // MacPorts
	} {
		engadir(dir, true)
	}

	os.Setenv("PATH", strings.Join(novo, string(os.PathListSeparator)))
}

func ficheirosPathMacOS() []string {
	rutas := []string{"/etc/paths"}
	entradas, err := os.ReadDir("/etc/paths.d")
	if err != nil {
		return rutas
	}
	for _, e := range entradas {
		if !e.IsDir() {
			rutas = append(rutas, filepath.Join("/etc/paths.d", e.Name()))
		}
	}
	return rutas
}

// ── Contrasinal de administrador (só para MacTeX) ───────────────────────────

// entornoAskpass devolve o entorno cun programa "askpass" temporal, e a
// función que o borra. Cando sudo non ten terminal pero SUDO_ASKPASS apunta
// a un executable, sudo (chamado con -A) execútao e le o contrasinal da súa
// saída estándar. Tanto Homebrew (Library/Homebrew/system_command.rb) coma o
// script oficial de instalación de Homebrew engaden ese -A por si sós en
// canto ven a variable definida.
//
// É o único xeito de que `brew install --cask mactex-no-gui` funcione dende
// unha GUI: ese cask é un .pkg e Homebrew instálao chamando a sudo por
// dentro, que sen tty nin askpass falla decontado ("no tty present and no
// askpass program specified").
//
// O contrasinal non pasa polo instalador en ningún momento: vai directo do
// diálogo de osascript á entrada de sudo. Aínda así o script créase nun
// directorio privado 0700, para que ningún outro usuario do equipo poida
// substituílo por un que o capture.
func entornoAskpass(mensaxe string) (env []string, limpar func()) {
	dir, err := os.MkdirTemp("", "yang-askpass-")
	if err != nil {
		return os.Environ(), func() {}
	}
	limpar = func() { os.RemoveAll(dir) }
	if err := os.Chmod(dir, 0o700); err != nil {
		limpar()
		return os.Environ(), func() {}
	}

	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ").Replace(mensaxe)
	guion := "#!/bin/sh\n" +
		"/usr/bin/osascript \\\n" +
		"  -e 'display dialog \"" + esc + "\" " +
		"with title \"Instalador de Yang\" default answer \"\" with hidden answer " +
		"buttons {\"Cancelar\", \"Aceptar\"} default button \"Aceptar\" with icon caution' \\\n" +
		"  -e 'text returned of result'\n"

	ruta := filepath.Join(dir, "askpass")
	if err := os.WriteFile(ruta, []byte(guion), 0o700); err != nil {
		limpar()
		return os.Environ(), func() {}
	}
	return append(os.Environ(), "SUDO_ASKPASS="+ruta), limpar
}

// ── Homebrew ────────────────────────────────────────────────────────────────

const urlInstaladorHomebrew = "https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh"

// asegurarHomebrew instala Homebrew se non o hai. macOS non trae xestor de
// paquetes ningún, así que nun Mac recén estreado este é o primeiro paso
// obrigado antes de poder instalar Maxima ou LaTeX.
//
// NONINTERACTIVE=1 é imprescindible: sen esa variable o script oficial para
// a agardar un RETURN por teclado que aquí nunca vai chegar, e a instalación
// quedaría colgada en silencio. Con ela o único que pode pedir é o
// contrasinal de administrador (para crear /opt/homebrew e, se fai falla,
// instalar as Command Line Tools de Xcode), e diso encárgase o askpass.
func asegurarHomebrew(log func(string)) error {
	if commandExists("brew") {
		return nil
	}
	log("📦 Este Mac non ten Homebrew: instalándoo dende brew.sh (pode tardar varios minutos)…")

	env, limpar := entornoAskpass(
		"O instalador de Yang precisa permisos de administrador para instalar Homebrew, o xestor de paquetes que instala Maxima e LaTeX.")
	defer limpar()

	cmd := exec.Command("/bin/bash", "-c",
		`set -o pipefail; /usr/bin/curl -fsSL `+urlInstaladorHomebrew+` | /bin/bash`)
	cmd.Env = append(env, "NONINTERACTIVE=1")
	runErr := runWithLog(cmd, log)

	// Comprobar de novo aínda que devolvese un código distinto de cero: o
	// script remata en erro por avisos que non impiden que quedase instalado.
	refrescarPathMacOS()
	if !commandExists("brew") {
		if runErr != nil {
			return fmt.Errorf("non se puido instalar Homebrew (%w). Instálao a man dende https://brew.sh e volve executar este instalador", runErr)
		}
		return fmt.Errorf("non se puido instalar Homebrew. Instálao a man dende https://brew.sh e volve executar este instalador")
	}
	log("✅ Homebrew instalado")
	return nil
}

// brewFormulas son as dependencias que se instalan coma fórmulas normais
// (sen root). Equivalen ás de linuxDepsInstallCommand: poppler é o que trae
// pdftoppm/pdftotext (o "poppler-utils" de Debian), e pandoc engádese aquí
// igual que se engade en Windows (winPackages).
var brewFormulas = []string{"maxima", "gnuplot", "poppler", "pandoc"}

// brewCaskLatex é a distribución de LaTeX. "mactex-no-gui" é MacTeX sen as
// aplicacións de escritorio (TeXShop, BibDesk...) que Yang non usa: segue
// sendo grande (varios GB), pero é o único cask de MacTeX que inclúe os tres
// motores que Yang ofrece en Opcións (pdflatex, xelatex e lualatex) máis
// TikZ. BasicTeX sería moito máis pequeno pero deixaría fóra paquetes que os
// exames usan de serie.
const brewCaskLatex = "mactex-no-gui"

// instalarDependenciasMacOS instala Maxima + a cadea LaTeX con Homebrew.
//
// Divídese en dous comandos a propósito: as fórmulas non precisan root e o
// cask si (é un .pkg), así que só o segundo leva o askpass - de non ser así,
// pediríaselle o contrasinal ao profesorado tamén para instalar Maxima, sen
// que faga ningunha falla.
func instalarDependenciasMacOS(log func(string)) error {
	if os.Getuid() == 0 {
		// Non debería pasar (intentarElevar non eleva en macOS), pero se
		// alguén lanza o instalador con `sudo` a man, mellor dicilo claro:
		// brew abortaría cun "Don't run this as root!" desconcertante.
		return fmt.Errorf("non executes o instalador con sudo en macOS: Homebrew négase a funcionar coma root. Ábreo cun dobre clic normal")
	}
	if err := asegurarHomebrew(log); err != nil {
		return err
	}

	var ultimoErro error

	log("📦 Instalando " + strings.Join(brewFormulas, ", ") + " (Homebrew)…")
	cmdFormulas := exec.Command("brew", append([]string{"install"}, brewFormulas...)...)
	cmdFormulas.Env = append(os.Environ(), entornoBrewSilencioso()...)
	pararF := informarProgresoBrew(log)
	err := runWithLog(cmdFormulas, log)
	pararF()
	if err != nil {
		log("⚠️  algunha fórmula fallou: " + err.Error())
		ultimoErro = err
	}

	log("📦 Instalando " + brewCaskLatex + " (a distribución LaTeX)…")
	log("   ⏳ ATENCIÓN: son preto de 6 GB. Segundo a conexión pode levar entre 10 e 45")
	log("      minutos, e Homebrew NON informa do progreso liña a liña — irase amosando")
	log("      aquí embaixo cada 30 segundos. Non peches esta xanela.")
	env, limpar := entornoAskpass(
		"O instalador de Yang precisa permisos de administrador para instalar MacTeX (a distribución de LaTeX).")
	defer limpar()
	cmdLatex := exec.Command("brew", "install", "--cask", brewCaskLatex)
	cmdLatex.Env = append(env, entornoBrewSilencioso()...)
	pararL := informarProgresoBrew(log)
	err = runWithLog(cmdLatex, log)
	pararL()
	if err != nil {
		log("⚠️  " + brewCaskLatex + " fallou: " + err.Error())
		ultimoErro = err
	}

	// MacTeX rexístrase no PATH creando /etc/paths.d/TeX, que só afecta a
	// procesos NOVOS - refrescámolo aquí para que o resto do instalador (e
	// calquera comprobación posterior) xa vexa pdflatex.
	refrescarPathMacOS()
	return ultimoErro
}

// entornoBrewSilencioso reduce o que Homebrew fai de máis nunha instalación
// desatendida: HOMEBREW_NO_AUTO_UPDATE evita un `brew update` completo (que
// pode tardar minutos, tamén en silencio) antes de cada instalación, e
// HOMEBREW_NO_ENV_HINTS quita os consellos de configuración que só enredan
// no log do instalador.
//
// NON se toca HOMEBREW_NO_REQUIRE_TAP_TRUST a propósito, aínda que un aviso
// sobre "tap trust" apareza no log cando o equipo ten taps de terceiros
// instalados: é unha comprobación de seguridade de Homebrew, non é nosa para
// desactivar, e non bloquea nada aquí (Yang só instala de homebrew/core e
// homebrew/cask, que son oficiais).
func entornoBrewSilencioso() []string {
	return []string{
		"HOMEBREW_NO_AUTO_UPDATE=1",
		"HOMEBREW_NO_ENV_HINTS=1",
	}
}

// informarProgresoBrew arranca un informe periódico mentres corre un comando
// de Homebrew, e devolve a función que o detén.
//
// Existe por un problema real: `brew install --cask mactex-no-gui` descarga
// preto de 6 GB e, como a nosa saída non é un terminal, Homebrew non pinta a
// barra de progreso — non emite NADA durante todo ese tempo. Para quen mira o
// instalador é indistinguible dun colgue, e o natural é pechalo a metade e
// quedar sen LaTeX. (Comprobado en real: co log en branco, a descarga ía
// perfectamente a ~9 MB/s.)
//
// O tamaño sácase do ficheiro ".incomplete" que curl vai escribindo na caché
// de Homebrew. Colle o máis recente de todos: mentres isto corre, o único que
// se está a descargar é o noso.
func informarProgresoBrew(log func(string)) (parar func()) {
	feito := make(chan struct{})
	rematado := make(chan struct{})

	go func() {
		defer close(rematado)
		inicio := time.Now()
		tic := time.NewTicker(30 * time.Second)
		defer tic.Stop()
		for {
			select {
			case <-feito:
				return
			case <-tic.C:
				min := int(time.Since(inicio).Minutes())
				if mb := mbDescargadosBrew(); mb > 0 {
					log(fmt.Sprintf("   ⏳ %d min — descargados %d MB…", min, mb))
				} else {
					log(fmt.Sprintf("   ⏳ %d min — traballando…", min))
				}
			}
		}
	}()

	return func() {
		close(feito)
		<-rematado
	}
}

// mbDescargadosBrew devolve o tamaño en MB da descarga en curso de Homebrew,
// ou 0 se non hai ningunha (ou non se puido localizar a caché).
func mbDescargadosBrew() int64 {
	if cacheBrew == "" {
		saida, err := exec.Command("brew", "--cache").Output()
		if err != nil {
			cacheBrew = "-" // non reintentar en cada tic
			return 0
		}
		cacheBrew = strings.TrimSpace(string(saida))
	}
	if cacheBrew == "-" {
		return 0
	}

	coincidencias, err := filepath.Glob(filepath.Join(cacheBrew, "downloads", "*.incomplete"))
	if err != nil || len(coincidencias) == 0 {
		return 0
	}
	var maisRecente os.FileInfo
	for _, ruta := range coincidencias {
		info, err := os.Stat(ruta)
		if err != nil {
			continue
		}
		if maisRecente == nil || info.ModTime().After(maisRecente.ModTime()) {
			maisRecente = info
		}
	}
	if maisRecente == nil {
		return 0
	}
	return maisRecente.Size() / (1024 * 1024)
}

// cacheBrew memoriza a saída de `brew --cache` ("-" = non dispoñible), para
// non lanzar un subproceso cada 30 segundos. Só o usa a goroutine de
// informarProgresoBrew, que nunca corre máis dunha vez á vez.
var cacheBrew string

// desinstalarDependenciasMacOS reverte instalarDependenciasMacOS.
func desinstalarDependenciasMacOS(log func(string)) error {
	if !commandExists("brew") {
		return fmt.Errorf("non se atopou Homebrew: as dependencias, se as hai, instaláronse por outra vía")
	}
	var ultimoErro error

	log("📦 Desinstalando " + strings.Join(brewFormulas, ", ") + "…")
	if err := runWithLog(exec.Command("brew", append([]string{"uninstall", "--ignore-dependencies"}, brewFormulas...)...), log); err != nil {
		log("⚠️  " + err.Error())
		ultimoErro = err
	}

	log("📦 Desinstalando " + brewCaskLatex + "…")
	env, limpar := entornoAskpass(
		"O instalador de Yang precisa permisos de administrador para desinstalar MacTeX.")
	defer limpar()
	cmd := exec.Command("brew", "uninstall", "--cask", brewCaskLatex)
	cmd.Env = env
	if err := runWithLog(cmd, log); err != nil {
		log("⚠️  " + err.Error())
		ultimoErro = err
	}
	return ultimoErro
}

// ── "Accesos directos" ──────────────────────────────────────────────────────

// rutaLsregister é a ferramenta de Launch Services que rexistra unha app na
// base de datos do sistema (a que alimenta Launchpad, Spotlight e o "Abrir
// con" do Finder). Non está no PATH: vive dentro do framework.
const rutaLsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

// createShortcutsMacOS é o equivalente en macOS de escribir o .desktop en
// Linux ou os .lnk en Windows - só que aquí non hai nada que crear: un .app
// dentro de /Applications xa é, por definición, o "acceso directo". O que si
// fai falla son tres retoques para que o bundle recén extraído se comporte
// coma unha aplicación instalada de verdade:
//
//  1. Quitar o atributo de corentena. Se o profesorado descargou este
//     instalador de internet, macOS marcouno con com.apple.quarantine, e ese
//     atributo hérdano os ficheiros que o instalador escribe. Sen quitalo,
//     ao abrir Yang aparecería o aviso de "descargouse de internet, seguro
//     que queres abrilo?" (ou directamente un bloqueo de Gatekeeper).
//  2. Volver asinar ad hoc. extractFiles escribe ficheiro a ficheiro, así
//     que convén selar de novo o bundle enteiro; nun Mac con Apple Silicon
//     unha app sen sinatura válida NON arrinca. Faise só se hai `codesign`
//     (Command Line Tools de Xcode); se non o hai, mantense a sinatura que
//     xa traía o .app empaquetado.
//  3. Rexistralo en Launch Services, para que apareza en Launchpad e
//     Spotlight decontado en vez de agardar ao próximo escaneo do sistema.
//
// Ningún dos tres é fatal: se algún falla, Yang segue estando instalado e
// pódese abrir dende o Finder.
func createShortcutsMacOS() error {
	if err := prepararBundleMacOS(func(string) {}); err != nil {
		return err
	}
	_ = exec.Command(rutaLsregister, "-f", destDir).Run()
	return nil
}

// prepararBundleMacOS fai os pasos 1 e 2 de createShortcutsMacOS (quitar a
// corentena e volver asinar). Sepárase para poder chamalo tamén ao rematar a
// extracción (doInstall, app.go): eses dous pasos non son cosmética coma o
// rexistro en Launch Services, senón a diferenza entre que Yang abra ou non,
// e non poden depender de que o profesorado chegue a premer o botón de
// "crear accesos directos" do último paso.
//
// É idempotente: chamalo dúas veces (unha ao instalar e outra ao premer o
// botón) non fai dano ningún.
func prepararBundleMacOS(log func(string)) error {
	// -d: borra o atributo; -r: recursivo por todo o bundle.
	_ = exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", destDir).Run()

	if _, err := exec.LookPath("codesign"); err != nil {
		// Sen Command Line Tools de Xcode non hai codesign. Non é fatal: o
		// .app xa viña asinado ad hoc dende o empaquetado (ver
		// files-darwin no Makefile) e extractFiles conserva o contido
		// ficheiro a ficheiro, así que esa sinatura segue valendo.
		log("ℹ️  codesign non dispoñible; mantense a sinatura orixinal do bundle")
		return nil
	}
	if out, err := exec.Command("codesign", "--force", "--deep", "--sign", "-", destDir).CombinedOutput(); err != nil {
		return fmt.Errorf("non se puido asinar %s: %w: %s", destDir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// desinstalarAccesosDirectosMacOS reverte createShortcutsMacOS. Só hai que
// sacar a app da base de datos de Launch Services: o propio .app bórrao
// desinstalarYangCompleto co os.RemoveAll(destDir), e non se escribiu nada
// máis fóra del (nin icona en /usr/share/pixmaps nin .desktop, coma en Linux).
func desinstalarAccesosDirectosMacOS() {
	_ = exec.Command(rutaLsregister, "-u", destDir).Run()
}
