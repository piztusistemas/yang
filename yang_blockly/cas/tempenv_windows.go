//go:build windows

package cas

import (
	"os"
	"path/filepath"
	"strings"
)

// maximaEnv devolve o ambiente co que arrincar o proceso de Maxima,
// apuntando TEMP/TMP/MAXIMA_TEMPDIR a un cartafol garantidamente ASCII -
// ver maximaTempDir para o porqué exacto. exec.Cmd.Env=nil (o valor por
// defecto) herdaría o ambiente do proceso actual tal cal, así que en
// Windows temos que collelo explicitamente (os.Environ()) e substituír
// estas variables.
func maximaEnv() []string {
	dir := maximaTempDir()
	env := os.Environ()
	if dir == "" {
		return env // sen alternativa atopada - queda como estaba
	}
	slashDir := filepath.ToSlash(dir) // Maxima traballa internamente con "/", non "\"
	out := make([]string, 0, len(env)+3)
	for _, kv := range env {
		upper := strings.ToUpper(kv)
		if strings.HasPrefix(upper, "TEMP=") || strings.HasPrefix(upper, "TMP=") || strings.HasPrefix(upper, "MAXIMA_TEMPDIR=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "TEMP="+dir, "TMP="+dir, "MAXIMA_TEMPDIR="+slashDir)
}

// maximaTempDir devolve (creándoo se fai falla) un cartafol temporal
// garantidamente ASCII para que o use Maxima.
//
// O problema real (descuberto lendo bin/maxima.bat, o script que arrinca
// Maxima en Windows): cada vez que se debuxa un <PLOT> (draw2d/plot2d...),
// Maxima escribe un script .gnuplot de traballo en MAXIMA_TEMPDIR - unha
// variable que, se non está xa definida, maxima.bat deriva de %TEMP%. O
// problema é que, se hai un executable "maxima_longnames.exe" no cartafol
// bin de Maxima (está, nesta instalación), maxima.bat SEMPRE pasa
// MAXIMA_TEMPDIR por el para "arranxar" calquera ruta curta 8.3
// (ex. "XOELIO~1") convertándoa DE VOLTA á súa forma longa real
// ("xoeliño") - un axuste pensado para CLISP (que ten problemas cos nomes
// curtos), pero que para SBCL (o Lisp que usa esta instalación) fai
// XUSTAMENTE O CONTRARIO do que precisamos: reintroduce o "ñ" que rompe a
// escritura do ficheiro .gnuplot con "Error opening ...: El sistema no
// puede encontrar la ruta especificada" - confirmado en directo: pasarlle a
// Maxima unha ruta curta (8.3) do TEMP do usuario NON abonda (o propio
// maxima.bat a desfai), pero un cartafol á parte, sen relación ningunha co
// perfil de usuario (así que non hai "forma longa" á que volver), si
// funciona.
//
// Por iso aquí NON se deriva nada do perfil do usuario (nin curto nin
// longo): créase un cartafol propio baixo %SystemRoot%\Temp
// (normalmente C:\Windows\Temp) - o temporal de sistema, sempre ASCII (é
// unha ruta fixa do SO, nunca depende do nome de usuario) e escribible por
// calquera usuario autenticado por defecto en Windows.
func maximaTempDir() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	dir := filepath.Join(root, "Temp", "yang-maxima")
	if os.MkdirAll(dir, 0o755) != nil {
		return ""
	}
	return dir
}
