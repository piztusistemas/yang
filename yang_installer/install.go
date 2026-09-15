package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// "all:" e non só "files": sen ese prefixo, //go:embed ignora todo o que
// empece por "_" ou ".". Nun build para macOS iso deixaría fóra
// Contents/_CodeSignature/ do bundle Yang.app, e a app extraída quedaría coa
// sinatura de código incompleta - nun Mac con Apple Silicon iso non é un
// aviso senón un "a aplicación está danada" no arranque. En Linux e Windows
// non cambia nada (files/ non ten entradas ocultas).
//
//go:embed all:files
var yangFiles embed.FS

//go:embed assets/logo.png
var logoPNG []byte

// destDir: onde vive Yang en si. En Linux, vive directamente dentro do
// cartafol de módulos de Piztu (config.yaml e piztu viven en /opt/piztu)
// — non hai symlink coma antes: instalar Yang standalone e instalalo coma
// módulo son agora o mesmo destino, así que non hai risco de que un
// actualice unha copia distinta da que piztu escanea. extractFiles crea o
// camiño completo (incluído /opt/piztu/modulos) se piztu non está
// instalado aínda. En Windows non existe Piztu (é un sistema Linux propio
// para centros) - así que non hai "módulo", é unha instalación autónoma
// normal en Program Files: "$PROGRAMFILES64\Piztu\Yang". Non hai instalador
// NSIS aparte (decidido a propósito): yang-installer.exe xa fai toda a
// instalación el só (elevación UAC vía manifesto + extrae ficheiros +
// atallos, ver createShortcutsWindows), así que non hai "instalar o
// instalador" - distribúese o .exe solto, mesmo criterio ca o binario solto
// en Linux (Makefile: "sudo ./yang_installer/build/bin/yang-installer").
var destDir = defaultDestDir()

// En macOS tampouco existe Piztu, e ademais unha aplicación NON é un
// cartafol de ficheiros soltos: é un bundle .app. Yang instálase entón coma
// /Applications/Yang.app, que é o sitio convencional e onde o sistema xa o
// atopa el só (Launchpad, Spotlight) sen accesos directos de ningún tipo.
// /Applications é escribible polo usuario administrador habitual dun Mac,
// así que non fai falla elevar a root - ver macos_darwin.go para o modelo
// completo de instalación en macOS.
func defaultDestDir() string {
	switch runtime.GOOS {
	case "windows":
		pf := os.Getenv("ProgramFiles")
		if pf == "" {
			pf = `C:\Program Files`
		}
		return filepath.Join(pf, "Piztu", "Yang")
	case "darwin":
		return "/Applications/Yang.app"
	}
	return "/opt/piztu/modulos/yang"
}

// nomeExecutable é a ruta do binario de Yang RELATIVA a destDir - "yang.exe"
// en Windows (esixido polo propio SO para poder executalo), "yang" en Linux,
// e "Contents/MacOS/yang" en macOS, onde destDir é o propio bundle
// /Applications/Yang.app e o executable vive na posición fixa que manda a
// estrutura dun .app. Ver Makefile ("files"/"files-darwin")/build-windows.ps1
// para onde se empaqueta con ese mesmo nome dentro de files/.
func nomeExecutable() string {
	switch runtime.GOOS {
	case "windows":
		return "yang.exe"
	case "darwin":
		return filepath.Join("Contents", "MacOS", "yang")
	}
	return "yang"
}

// esRoot indica se este proceso ten permisos abondos para completar a
// instalación. O frontend úsao para decidir se amosa o aviso de
// "benvida.aviso" (App.EsRoot en main.js), así que o que interesa non é
// literalmente "son root" senón "podo instalar".
//
// En Windows sempre é true: se chegamos aquí é porque o manifesto
// (requestedExecutionLevel="requireAdministrator") xa fixo que UAC elevase o
// proceso antes de que main() empezase a executarse (ver elevate_normal.go);
// non existe os.Getuid() con sentido en Windows.
//
// En macOS NON se eleva a propósito (ver elevate_normal.go e macos_darwin.go):
// /Applications adoita ser escribible polo usuario administrador do equipo e
// Homebrew négase a correr coma root. Así que aquí a pregunta correcta é se
// se pode escribir no destino, non se somos uid 0 - de mirar o uid, todo Mac
// amosaría un aviso de "executa isto con sudo" que ademais sería un mal
// consello.
//
// En Linux segue a reflectir se somos root (idealmente xa reexecutado vía
// pkexec): alí o destino é /opt/piztu, que si require root de verdade.
func esRoot() bool {
	switch runtime.GOOS {
	case "windows":
		return true
	case "darwin":
		return podeEscribirEn(filepath.Dir(destDir))
	}
	return os.Getuid() == 0
}

// podeEscribirEn comproba se se pode crear algo dentro de dir, creando e
// borrando un directorio temporal. Compróbase facéndoo de verdade en vez de
// mirar os bits de permiso porque en macOS a resposta non depende só deles
// (System Integrity Protection, ACLs, volumes montados en só lectura...) e
// un os.Stat daría un "si" que despois non se cumpre.
func podeEscribirEn(dir string) bool {
	proba, err := os.MkdirTemp(dir, ".yang-proba-")
	if err != nil {
		return false
	}
	os.RemoveAll(proba)
	return true
}

// getRealUser devolve o usuario e home de quen lanzou o instalador (non
// root). Porte directo de installer/install.go.
func getRealUser() (string, string) {
	if uid := os.Getenv("PKEXEC_UID"); uid != "" {
		if u, err := user.LookupId(uid); err == nil {
			return u.Username, u.HomeDir
		}
	}
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			return u.Username, u.HomeDir
		}
	}
	if u, err := user.Current(); err == nil {
		return u.Username, u.HomeDir
	}
	return "usuario", "/home/usuario"
}

// ── Dependencias do sistema (Maxima + cadea LaTeX) ──────────────────────────
//
// Mesmos paquetes que yang/installer.go xa instala baixo demanda dende
// dentro da propia app (botón "Instalar" en Opcións): non texlive-full
// (varios GB de fontes/linguas que Yang nunca usa) senón só o necesario para
// pdflatex/xelatex/lualatex + TikZ + a conversión a imaxe da vista previa.
//
// texlive-latex-extra (só en Debian/Ubuntu, que é onde os paquetes van tan
// repartidos) engádese polas PLANTILLAS de documento (plantillas.go): aí é
// onde vive tcolorbox, titlesec, adjustbox, enumitem... - o repertorio
// normal de calquera maqueta un pouco traballada, e o que escolle a IA cando
// se lle pide unha plantilla a partir dun modelo. Sen el, o profesorado
// atopábase cun "falta o paquete tcolorbox" na primeira plantilla seria que
// probase. Custa uns 400-500 MB máis, que segue moi lonxe dos varios GB de
// texlive-full.
func linuxDepsInstallCommand() (argv []string, mgrName string, ok bool) {
	switch {
	case commandExists("apt-get"):
		return []string{"sh", "-c", "apt-get update && apt-get install -y " +
			"maxima maxima-share gnuplot " +
			"texlive-latex-base texlive-latex-recommended texlive-pictures " +
			"texlive-latex-extra texlive-xetex texlive-luatex poppler-utils"}, "apt-get", true
	case commandExists("dnf"):
		return []string{"dnf", "install", "-y",
			"maxima", "gnuplot",
			"texlive-latex", "texlive-collection-pictures",
			"texlive-xetex", "texlive-luatex", "poppler-utils"}, "dnf", true
	case commandExists("pacman"):
		return []string{"pacman", "-Sy", "--noconfirm",
			"maxima", "gnuplot", "texlive-core", "texlive-pictures", "poppler"}, "pacman", true
	case commandExists("zypper"):
		return []string{"zypper", "install", "-y",
			"maxima", "gnuplot", "texlive", "texlive-latex", "poppler-tools"}, "zypper", true
	}
	return nil, "", false
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// wingetPackageInstalled comproba con "winget list" (non "winget install")
// se id xa está instalado - devolve exit 0 con esa mesma sintaxe "--id X
// -e" se hai coincidencia exacta, exit distinto de cero ("No se encontró
// ningún paquete...") se non - comprobado en real, é o xeito fiable de
// distinguir "xa instalado" de "instalación fallida" (ver
// instalarDependenciasWindows).
func wingetPackageInstalled(id string) bool {
	return comandoOculto("winget", "list", "--id", id, "-e").Run() == nil
}

// winPackages: mesmos IDs de winget que yang/installer.go (InstallMaxima/
// InstallLatex/InstallPandoc) - un só punto de instalación (este instalador
// substitúe o vello concepto de instalador NSIS aparte), mesma lista.
var winPackages = []string{"MaximaTeam.Maxima", "MiKTeX.MiKTeX", "JohnMacFarlane.Pandoc"}

// instalarDependenciasWindows instala Maxima + MiKTeX (LaTeX) + Pandoc con
// winget. A diferenza do botón interno de Yang (yang/installer.go), aquí
// non fai falla que o propio winget pida UAC por paquete: este instalador
// xa corre elevado (o manifesto pide requireAdministrator, ver
// elevate_normal.go), así que winget instala directo.
func instalarDependenciasWindows(log func(string)) error {
	if !commandExists("winget") {
		return fmt.Errorf("non se atopou winget - instala Maxima/MiKTeX/Pandoc manualmente, ou reintenta despois dende Yang (Opcións)")
	}
	var ultimoErro error
	for _, id := range winPackages {
		// Comprobar antes con "winget list", non fiarse do código de saída
		// de "winget install": se o paquete xa está instalado, winget
		// intenta ACTUALIZALO en vez de instalalo - e se non hai versión
		// máis nova (ou o paquete non soporta actualización por winget,
		// caso real de MiKTeX.MiKTeX), devolve un código de saída distinto
		// de cero (0x8a15002b/0x8a150114, comprobados en real) que NON
		// significa que faltase instalar nada, e antes trataba coma erro.
		if wingetPackageInstalled(id) {
			log("✓ " + id + " xa estaba instalado")
			continue
		}
		log("📦 Instalando " + id + " (winget)…")
		err := runWithLog(comandoOculto("winget", "install", "--id", id, "-e",
			"--silent", "--accept-source-agreements", "--accept-package-agreements", "--disable-interactivity"), log)
		if err != nil {
			log("⚠️  " + id + " fallou: " + err.Error())
			ultimoErro = err
		}
	}
	// Rexistra gnuplot-lua-tikz.sty en MiKTeX (ver gnuplottex_windows.go) -
	// sen isto, calquera exame cun <PLOT> falla ao compilar o PDF ("File
	// `gnuplot-lua-tikz.sty' not found"). Chámase aquí, despois de instalar
	// Maxima E MiKTeX (winPackages, enriba), non antes.
	registrarGnuplotTexmfMiKTeX(log)
	return ultimoErro
}

// desinstalarDependenciasWindows reverte instalarDependenciasWindows.
func desinstalarDependenciasWindows(log func(string)) error {
	if !commandExists("winget") {
		return fmt.Errorf("non se atopou winget")
	}
	var ultimoErro error
	for _, id := range winPackages {
		log("📦 Desinstalando " + id + " (winget)…")
		err := runWithLog(comandoOculto("winget", "uninstall", "--id", id, "-e", "--silent"), log)
		if err != nil {
			log("⚠️  " + id + " non se puido desinstalar: " + err.Error())
			ultimoErro = err
		}
	}
	return ultimoErro
}

// instalarDependencias instala Maxima + a cadea LaTeX (+ Pandoc en Windows)
// co xestor de paquetes do sistema. A diferenza do botón interno de Yang
// (yang/installer.go), aquí non fai falla pkexec/UAC por comando: o
// instalador enteiro xa se reexecutou elevado en main.go (ver
// intentarElevar).
func instalarDependencias(log func(string)) error {
	if runtime.GOOS == "windows" {
		return instalarDependenciasWindows(log)
	}
	if runtime.GOOS == "darwin" {
		return instalarDependenciasMacOS(log)
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("instalación automática de dependencias só soportada en Linux, Windows e macOS")
	}
	argv, mgrName, ok := linuxDepsInstallCommand()
	if !ok {
		return fmt.Errorf("non se atopou un xestor de paquetes coñecido (apt/dnf/pacman/zypper)")
	}
	log("📦 Instalando Maxima + LaTeX con " + mgrName + "…")
	return runWithLog(exec.Command(argv[0], argv[1:]...), log)
}

// desinstalarDependencias reverte instalarDependencias. É opcional e
// independente do resto da desinstalación: son paquetes de sistema que
// outros programas tamén poderían usar.
func desinstalarDependencias(log func(string)) error {
	if runtime.GOOS == "windows" {
		return desinstalarDependenciasWindows(log)
	}
	if runtime.GOOS == "darwin" {
		return desinstalarDependenciasMacOS(log)
	}
	pkgs := []string{
		"maxima", "maxima-share", "gnuplot",
		"texlive-latex-base", "texlive-latex-recommended", "texlive-pictures",
		"texlive-latex-extra", "texlive-xetex", "texlive-luatex", "poppler-utils",
	}
	switch {
	case commandExists("apt-get"):
		return runWithLog(exec.Command("apt-get", append([]string{"remove", "-y", "-q", "--purge"}, pkgs...)...), log)
	case commandExists("dnf"):
		return runWithLog(exec.Command("dnf", append([]string{"remove", "-y"}, pkgs...)...), log)
	case commandExists("pacman"):
		return runWithLog(exec.Command("pacman", append([]string{"-Rns", "--noconfirm"}, pkgs...)...), log)
	case commandExists("zypper"):
		return runWithLog(exec.Command("zypper", append([]string{"remove", "-y"}, pkgs...)...), log)
	}
	return fmt.Errorf("non se atopou un xestor de paquetes coñecido")
}

// ── Ficheiros e permisos ────────────────────────────────────────────────────

func extractFiles(log func(string)) error {
	const root = "files"
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	return fs.WalkDir(yangFiles, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, root+"/")
		if rel == root || rel == "" {
			return nil
		}
		target := filepath.Join(destDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		log("  " + rel)
		data, err := yangFiles.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}

// fixOwnership pasa destDir ao usuario real: o instalador corre como root
// (pkexec), pero Yang execútase despois como sesión de escritorio normal e
// precisa escribir alí a súa configuración (settings.json). En Windows non
// fai falla: a elevación por UAC non troca de usuario (é o MESMO usuario,
// só cun token elevado), así que non hai "propietario incorrecto" que
// arranxar - os ficheiros xa quedan do usuario real. En macOS tampouco:
// alí non se eleva en absoluto (ver macos_darwin.go), así que os ficheiros
// escríbeos xa directamente o usuario que abriu o instalador.
func fixOwnership() error {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return nil
	}
	realUser, _ := getRealUser()
	u, err := user.Lookup(realUser)
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return filepath.WalkDir(destDir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(path, uid, gid)
	})
}

// escribirIdiomaPorDefecto dálle a Yang o mesmo idioma escollido para a
// propia interface do instalador, para que un profesor que instala en
// inglés xa atope Yang en inglés dende o primeiro arranque, sen ter que ir
// a Opcións. Escribe/actualiza a mesma ruta que yang/settings.go
// (settingsPath) le para decidir a lingua (frontend/src/main.js,
// GetSettings().then(...)): ~<home>/.config/yang/settings.json en Linux
// (os.UserConfigDir() resolve iso mesmo para o usuario real, pero aquí
// témolo que reconstruír a man dende getRealUser() porque baixo pkexec o
// HOME do PROCESO é o de root, non o real), e %AppData%\yang\settings.json
// en Windows - onde si podemos chamar a os.UserConfigDir() directamente: a
// elevación por UAC non troca de usuario/perfil coma pkexec, así que
// os.UserConfigDir() xa resolve correctamente para o usuario real sen
// rodeos. Fusiona en vez de sobrescribir: nunha reinstalación/actualización
// non se perde o resto de axustes xa gardados (clave de IA, ruta a
// Maxima…), só se toca "idioma".
func escribirIdiomaPorDefecto(idioma string) error {
	if idioma == "" {
		idioma = "gl"
	}
	realUser, home := getRealUser()
	var cfgDir string
	// Windows e macOS: os.UserConfigDir() resolve xa correctamente para o
	// usuario real (a elevación por UAC non troca de perfil, e en macOS non
	// se eleva sequera), e devolve exactamente o mesmo que le Yang en
	// settingsPath() - %AppData%\yang en Windows e
	// ~/Library/Application Support/yang en macOS. OLLO: en macOS NON é
	// ~/.config/yang; escribir alí sería escribir nun ficheiro que Yang
	// nunca vai ler.
	//
	// En Linux hai que reconstruílo a man dende getRealUser(): baixo pkexec
	// o HOME do PROCESO é o de root, así que os.UserConfigDir() daría
	// /root/.config.
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("localizando o cartafol de configuración: %w", err)
		}
		cfgDir = filepath.Join(dir, "yang")
	} else {
		cfgDir = filepath.Join(home, ".config", "yang")
	}
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		return fmt.Errorf("creando %s: %w", cfgDir, err)
	}
	cfgPath := filepath.Join(cfgDir, "settings.json")

	settings := map[string]any{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		json.Unmarshal(data, &settings) // reinstalación: conservar o resto de axustes
	}
	settings["idioma"] = idioma

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return fmt.Errorf("escribindo %s: %w", cfgPath, err)
	}

	// O instalador corre como root (pkexec); Yang despois execútase como
	// sesión de escritorio normal e precisa poder ler/escribir este
	// ficheiro (mesmo motivo ca fixOwnership sobre destDir).
	if u, err := user.Lookup(realUser); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		os.Chown(cfgDir, uid, gid)
		os.Chown(cfgPath, uid, gid)
	}
	return nil
}

// ── Accesos directos ─────────────────────────────────────────────────────────

func createShortcuts() error {
	if runtime.GOOS == "windows" {
		return createShortcutsWindows()
	}
	if runtime.GOOS == "darwin" {
		return createShortcutsMacOS()
	}
	iconPath := "/usr/share/pixmaps/yang.png"
	if err := os.WriteFile(iconPath, logoPNG, 0644); err != nil {
		return fmt.Errorf("escribindo icona: %w", err)
	}

	desktop := strings.Join([]string{
		"[Desktop Entry]",
		"Version=1.0",
		"Type=Application",
		"Name=Yang",
		"Name[gl_ES]=Yang",
		"Comment=Xerador de exames de matemáticas (Maxima + LaTeX)",
		"Comment[gl_ES]=Xerador de exames de matemáticas (Maxima + LaTeX)",
		"Exec=" + filepath.Join(destDir, "yang"),
		"Icon=" + iconPath,
		"Terminal=false",
		"StartupNotify=true",
		"Categories=Education;Science;",
		"Keywords=exame;matematicas;maxima;latex;piztu;",
		"",
	}, "\n")

	appPath := "/usr/share/applications/yang.desktop"
	if err := os.WriteFile(appPath, []byte(desktop), 0644); err != nil {
		return fmt.Errorf("escribindo .desktop do sistema: %w", err)
	}

	_, home := getRealUser()
	for _, name := range []string{"Escritorio", "Escriptorio", "Desktop"} {
		d := filepath.Join(home, name)
		if _, err := os.Stat(d); err == nil {
			dst := filepath.Join(d, "yang.desktop")
			os.WriteFile(dst, []byte(desktop), 0755)
			exec.Command("gio", "set", dst, "metadata::trusted", "true").Run()
			break
		}
	}

	exec.Command("update-desktop-database", "/usr/share/applications").Run()
	return nil
}

// createShortcutsWindows crea o atallo de Inicio (todos os usuarios,
// %ProgramData%\Microsoft\Windows\Start Menu\Programs) e o do escritorio do
// usuario real, vía WScript.Shell (COM) invocado dende PowerShell — é a
// forma estándar de crear .lnk sen depender de ningunha libraría Go extra
// (equivalente Windows a createShortcuts, que escribe .desktop en Linux). A
// icona é a propia de yang.exe (IconLocation="ruta,0"): non fai falla xerar
// un .ico á parte coma o logoPNG que se escribe en /usr/share/pixmaps.
func createShortcutsWindows() error {
	exePath := filepath.Join(destDir, nomeExecutable())

	startMenu := filepath.Join(programDataDir(), "Microsoft", "Windows", "Start Menu", "Programs", "Yang.lnk")
	if err := crearAtalloWindows(startMenu, exePath); err != nil {
		return fmt.Errorf("creando atallo de Inicio: %w", err)
	}

	_, home := getRealUser()
	desktop := filepath.Join(home, "Desktop", "Yang.lnk")
	crearAtalloWindows(desktop, exePath) // non crítico: o de Inicio xa abonda se este falla

	return nil
}

// programDataDir devolve %ProgramData% (cartafol de datos compartidos entre
// usuarios, onde vive o Start Menu de "todos os usuarios").
func programDataDir() string {
	if pd := os.Getenv("ProgramData"); pd != "" {
		return pd
	}
	return `C:\ProgramData`
}

// crearAtalloWindows crea (ou sobrescribe) un .lnk que apunta a targetPath.
func crearAtalloWindows(lnkPath, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(lnkPath), 0755); err != nil {
		return err
	}
	// Comiña simple duplicada é o escape dentro dunha cadea PowerShell
	// entre comiñas simples (non hai interpolación, así que os camiños con
	// espazos non precisan máis ca isto).
	esc := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	script := fmt.Sprintf(
		"$s = New-Object -ComObject WScript.Shell; "+
			"$sc = $s.CreateShortcut('%s'); "+
			"$sc.TargetPath = '%s'; "+
			"$sc.WorkingDirectory = '%s'; "+
			"$sc.IconLocation = '%s,0'; "+
			"$sc.Save()",
		esc(lnkPath), esc(targetPath), esc(filepath.Dir(targetPath)), esc(targetPath),
	)
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// desinstalarAccesosDirectosWindows reverte createShortcutsWindows.
func desinstalarAccesosDirectosWindows() {
	os.Remove(filepath.Join(programDataDir(), "Microsoft", "Windows", "Start Menu", "Programs", "Yang.lnk"))
	_, home := getRealUser()
	os.Remove(filepath.Join(home, "Desktop", "Yang.lnk"))
}

func desinstalarAccesosDirectos() {
	if runtime.GOOS == "windows" {
		desinstalarAccesosDirectosWindows()
		return
	}
	if runtime.GOOS == "darwin" {
		desinstalarAccesosDirectosMacOS()
		return
	}
	os.Remove("/usr/share/pixmaps/yang.png")
	os.Remove("/usr/share/applications/yang.desktop")

	_, home := getRealUser()
	for _, name := range []string{"Escritorio", "Escriptorio", "Desktop"} {
		os.Remove(filepath.Join(home, name, "yang.desktop"))
	}
	exec.Command("update-desktop-database", "/usr/share/applications").Run()
}

// ── Desinstalación completa ──────────────────────────────────────────────────

// desinstalarYangCompleto elimina Yang deste equipo: os accesos
// directos/icona e por último destDir (que é directamente
// <base_dir>/modulos/yang, ver destDir). borrarDeps controla se ademais se
// desinstalan Maxima/LaTeX (opcional: son paquetes de sistema que outro
// programa podería usar).
func desinstalarYangCompleto(log func(string), borrarDeps bool) error {
	log("🖥️  Eliminando accesos directos e icona…")
	desinstalarAccesosDirectos()

	if borrarDeps {
		log("📦 Desinstalando Maxima e LaTeX…")
		if err := desinstalarDependencias(log); err != nil {
			log("⚠️  " + err.Error())
		}
	}

	log("📁 Eliminando " + destDir + "…")
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("eliminando %s: %w", destDir, err)
	}

	return nil
}

// ── Utilidades ────────────────────────────────────────────────────────────────

type logWriter struct{ fn func(string) }

func (w *logWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line != "" {
			w.fn(line)
		}
	}
	return len(p), nil
}

func runWithLog(cmd *exec.Cmd, log func(string)) error {
	lw := &logWriter{fn: log}
	cmd.Stdout = lw
	cmd.Stderr = lw
	return cmd.Run()
}
