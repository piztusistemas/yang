//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Este ficheiro concentra todo o que fai falla para que os botóns
// "Instalar" de Opcións (InstallMaxima/InstallLatex/InstallPandoc,
// installer.go) funcionen de verdade nun Mac, onde hai dous atrancos que
// non existen nin en Linux nin en Windows:
//
//  1. Pode non haber Homebrew. En Linux sempre hai apt/dnf/pacman/zypper e
//     en Windows sempre hai winget (Windows 10 1809+), pero macOS non trae
//     xestor de paquetes ningún: nun Mac recén estreado `brew` non existe.
//     asegurarHomebrew instálao (ver máis abaixo).
//
//  2. `brew install --cask mactex-no-gui` precisa root, e non hai terminal.
//     Os "casks" que son .pkg instálanse chamando internamente a `sudo` -
//     que sen tty nin askpass falla decontado con "no tty present and no
//     askpass program specified". Yang é unha app de GUI: nunca vai ter
//     tty. prepararEntornoInstalacion resolve isto co mecanismo estándar
//     SUDO_ASKPASS.
//
// (En Linux o equivalente é pkexec, e en Windows o propio winget xa pide
// UAC; ver os case "linux"/"windows" de installer.go.)

// prepararEntornoInstalacion devolve o entorno co que hai que lanzar os
// comandos de instalación, e unha función para limpar o que fixese falla
// crear. En macOS: un programa "askpass" temporal que lle amosa ao
// profesorado o diálogo nativo de contrasinal de administrador.
//
// Como funciona SUDO_ASKPASS: cando sudo non ten tty pero esa variable
// apunta a un executable, sudo (chamado con -A) execútao e le o contrasinal
// da SÚA saída estándar en vez de pedilo por teclado. Tanto o Homebrew
// instalado (Library/Homebrew/system_command.rb: `askpass_flags =
// ENV.key?("SUDO_ASKPASS") ? ["-A"] : []`) coma o script oficial de
// instalación de Homebrew (install.sh: `if [[ -n "${SUDO_ASKPASS-}" ]];
// then SUDO+=("-A")`) engaden ese -A por si sós en canto ven a variable, así
// que abonda con exportala.
//
// O contrasinal nunca pasa por Yang: vai directo do diálogo de osascript
// (proceso fillo de sudo, non noso) á entrada de sudo. O script temporal en
// si non contén ningún segredo, pero créase igualmente nun directorio
// privado con permisos 0700 para que ningún outro usuario do equipo poida
// substituílo por un que capture o contrasinal.
func prepararEntornoInstalacion(mensaxe string) (env []string, limpar func()) {
	dir, err := os.MkdirTemp("", "yang-askpass-")
	if err != nil {
		return nil, func() {} // sen askpass: sudo fallará, pero non rompemos nada
	}
	limpar = func() { os.RemoveAll(dir) }

	if err := os.Chmod(dir, 0o700); err != nil {
		limpar()
		return nil, func() {}
	}

	ruta := filepath.Join(dir, "askpass")
	if err := os.WriteFile(ruta, []byte(guionAskpass(mensaxe)), 0o700); err != nil {
		limpar()
		return nil, func() {}
	}

	return append(os.Environ(), "SUDO_ASKPASS="+ruta,
		// HOMEBREW_NO_AUTO_UPDATE evita un `brew update` completo (minutos,
		// e en silencio) antes de cada instalación; HOMEBREW_NO_ENV_HINTS
		// quita consellos de configuración que só enredan no log que se lle
		// amosa ao profesorado.
		"HOMEBREW_NO_AUTO_UPDATE=1",
		"HOMEBREW_NO_ENV_HINTS=1",
	), limpar
}

// guionAskpass constrúe o programa askpass: un envoltorio de osascript que
// amosa o diálogo nativo de macOS e imprime o que se escriba nel. Se o
// profesorado preme "Cancelar", osascript sae cun código distinto de cero e
// non imprime nada, así que sudo (e polo tanto brew) cancela tamén - que é
// exactamente o que se quere.
func guionAskpass(mensaxe string) string {
	// mensaxe vai dentro dunha cadea de AppleScript entre comiñas dobres,
	// que á súa vez vai dentro dunha cadea de shell entre comiñas simples.
	// Só hai que escapar o que rompe a cadea de AppleScript.
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ").Replace(mensaxe)
	return "#!/bin/sh\n" +
		"/usr/bin/osascript \\\n" +
		"  -e 'display dialog \"" + esc + "\" " +
		"with title \"Yang\" default answer \"\" with hidden answer " +
		"buttons {\"Cancelar\", \"Aceptar\"} default button \"Aceptar\" with icon caution' \\\n" +
		"  -e 'text returned of result'\n"
}

// urlInstaladorHomebrew é o script oficial de instalación de Homebrew
// (https://brew.sh). Descárgase e execútase só cando o profesorado preme
// explicitamente un botón "Instalar" e non hai brew no equipo.
const urlInstaladorHomebrew = "https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh"

// asegurarHomebrew garante que exista `brew` antes de intentar instalar
// nada con el. Se xa está, non fai nada. Se non, executa o script oficial
// de instalación en modo non interactivo e volve refrescar o PATH (o
// instalador de Homebrew deixa o binario en /opt/homebrew/bin ou
// /usr/local/bin, que unha app lanzada dende o Finder non ten no PATH - ver
// pathrefresh_darwin.go).
//
// NONINTERACTIVE=1 é imprescindible: sen esa variable o script para a
// esperar un RETURN por teclado que aquí non vai chegar nunca, e a
// instalación quedaría colgada para sempre sen dicir nada. Con ela, o único
// que pode preguntar é o contrasinal de administrador (para crear
// /opt/homebrew e, se fai falla, instalar as Command Line Tools de Xcode),
// e iso resólveo o askpass de prepararEntornoInstalacion.
//
// Devolve o log do intento e, se ao rematar segue sen haber brew, un erro.
func asegurarHomebrew() (string, error) {
	if commandExists("brew") {
		return "", nil
	}

	env, limpar := prepararEntornoInstalacion(
		"Yang precisa permisos de administrador para instalar Homebrew, o xestor de paquetes que instala Maxima e LaTeX.")
	defer limpar()

	var out bytes.Buffer
	out.WriteString("📦 Non hai Homebrew neste Mac: instalándoo dende brew.sh…\n")

	// `curl | bash` nun só comando de shell, igual que as instrucións
	// oficiais de brew.sh. -fsSL faille a curl fallar de vez (e non escribir
	// unha páxina de erro na tubaría) se a descarga non vai ben.
	cmd := exec.Command("/bin/bash", "-c",
		`set -o pipefail; /usr/bin/curl -fsSL `+urlInstaladorHomebrew+` | /bin/bash`)
	cmd.Env = append(env, "NONINTERACTIVE=1")
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()

	// Buscar brew de novo aínda que o script devolvese un código distinto de
	// cero: remata en erro por avisos que non impiden que quedase instalado.
	refrescarPathDendeRexistro()
	if !commandExists("brew") {
		msg := "non se puido instalar Homebrew automaticamente"
		if runErr != nil {
			msg += ": " + runErr.Error()
		}
		return out.String(), fmt.Errorf("%s. Instálao a man dende https://brew.sh e volve premer este botón", msg)
	}

	out.WriteString("✅ Homebrew instalado\n")
	return out.String(), nil
}
