//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// bundleQueContenA devolve a ruta do .app que contén exe, ou "" se exe non
// vive dentro dun bundle. A estrutura dun bundle é fixa:
//
//	Yang.app/Contents/MacOS/yang   ← exe
//	         ^-------------------- tres niveis por riba
//
// Devolve "" (non erro) para un binario solto - Yang tamén se pode executar
// así (dende un terminal, ou co `task run` de desenvolvemento), e nese caso
// non hai sinatura ningunha que manter.
func bundleQueContenA(exe string) string {
	dirMacOS := filepath.Dir(exe)
	if filepath.Base(dirMacOS) != "MacOS" {
		return ""
	}
	dirContents := filepath.Dir(dirMacOS)
	if filepath.Base(dirContents) != "Contents" {
		return ""
	}
	app := filepath.Dir(dirContents)
	if !strings.HasSuffix(app, ".app") {
		return ""
	}
	return app
}

// comandoRelanzar constrúe o comando que volve abrir Yang despois dunha
// actualización. Se Yang vive nun .app usa `open -n`, non o binario de
// dentro: lanzar Contents/MacOS/yang directamente crea un proceso que
// LaunchServices non recoñece coma a aplicación (aparece no Dock coma un
// executable solto e sen o nome nin a icona do bundle, e non recibe os
// eventos de apertura de ficheiros .matex). -n forza unha instancia nova
// aínda que macOS crea que Yang xa está aberto - imprescindible aquí, xa
// que a instancia vella aínda non pechou cando se chama isto.
func comandoRelanzar(exe string) *exec.Cmd {
	if app := bundleQueContenA(exe); app != "" {
		return exec.Command("/usr/bin/open", "-n", app)
	}
	return exec.Command(exe)
}

// resinarBundleMacOS volve asinar (ad hoc) o .app que contén exe, e non fai
// nada se exe non está nun bundle.
//
// Por que fai falla: en macOS a sinatura de código non cobre só o
// executable, senón todo o bundle - Contents/_CodeSignature/CodeResources
// garda un hash de cada ficheiro. En canto AplicarActualizacionYangEnCaliente
// substitúe Contents/MacOS/yang por outro binario, eses hashes deixan de
// coincidir e a sinatura queda rota. Nun Mac con Apple Silicon iso non é un
// aviso: o kernel esixe que TODO binario arm64 estea asinado, así que a
// seguinte vez que se abrise Yang o sistema mataríao no arranque ("killed:
// 9" ou "a aplicación está danada") sen máis explicación, e o profesorado
// quedaría cunha instalación morta despois de aceptar unha actualización.
// Volver asinar ad hoc (a mesma sinatura "-" que usa `task package`, ver
// build/darwin/Taskfile.yml → codesign:adhoc) deixa o bundle coherente.
//
// Nota para distribución asinada: se algún día se publica Yang cun
// certificado Developer ID e notarizado, este paso rebaixaría esa sinatura a
// unha ad hoc. A app seguiría abrindo (xa está instalada e sen o atributo de
// corentena), pero o correcto entón sería distribuír a actualización coma
// .dmg asinado en vez de en quente.
func resinarBundleMacOS(exe string) error {
	app := bundleQueContenA(exe)
	if app == "" {
		return nil // binario solto: non hai bundle que asinar
	}

	if _, err := exec.LookPath("codesign"); err != nil {
		return fmt.Errorf("non se atopou 'codesign' (fai falta instalar as Command Line Tools de Xcode: xcode-select --install)")
	}

	// --force: substituír a sinatura existente en vez de queixarse de que xa
	// hai unha. --deep: volver selar tamén o contido de Contents/Resources.
	saida, err := exec.Command("codesign", "--force", "--deep", "--sign", "-", app).CombinedOutput()
	if err != nil {
		return fmt.Errorf("non se puido asinar %q: %w: %s", app, err, strings.TrimSpace(string(saida)))
	}
	return nil
}
