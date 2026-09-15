//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// gnuplotTexmfWindows localiza o texmf que trae consigo o gnuplot empaquetado
// xunto co instalador oficial de Maxima para Windows (mesmo winget
// "MaximaTeam.Maxima" - ver findMaxima, settings.go): baixo
// "<instalación>\gnuplot\share\texmf\tex\latex\gnuplot\gnuplot-lua-tikz.sty".
// Este .sty NON é un paquete MiKTeX (`mpm --install=gnuplot-lua-tikz`
// devolve "The requested package is unknown" - comprobado en real): en
// Linux o proporciona directamente o paquete "gnuplot" do sistema baixo
// /usr/share/texmf, pero en Windows vive só dentro do propio cartafol de
// Maxima, e MiKTeX non ten forma de sabelo. Reusa o mesmo patrón de busca
// ca findMaxima en vez de depender do maximaPath xa resolto, para non ter
// que pasalo por toda a cadea de chamadas de compileLatex.
func gnuplotTexmfWindows() string {
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
		patrons = append(patrons, filepath.Join(base, "maxima-*", "gnuplot", "share", "texmf"))
		patrons = append(patrons, filepath.Join(base, "Maxima-*", "gnuplot", "share", "texmf"))
	}
	var atopados []string
	for _, patron := range patrons {
		coincidencias, _ := filepath.Glob(patron)
		atopados = append(atopados, coincidencias...)
	}
	if len(atopados) == 0 {
		return ""
	}
	sort.Strings(atopados)
	return atopados[len(atopados)-1]
}

// latexEnvExtra devolve variables de ambiente adicionais para pdflatex/
// xelatex/lualatex (ver compileLatex, latexdoc.go) - en Windows, TEXINPUTS
// apuntando ao texmf de gnuplot (ver gnuplotTexmfWindows enriba) para que
// atope gnuplot-lua-tikz.sty cando un <PLOT> xerou un debuxo TikZ (plot(),
// cas/tags.go). "//" ao final busca recursivamente baixo ese cartafol (é a
// marca de kpathsea, igual en Windows ca en Linux, non un separador de
// ruta); o ";" final sen nada despois engade tamén os camiños por defecto
// do sistema, en vez de substituílos enteiros. nil se non se atopou o
// texmf de gnuplot (Maxima non instalado, ou instalado noutro sitio) -
// compileLatex segue co ambiente normal, mesmo comportamento que sen este
// ficheiro.
func latexEnvExtra() []string {
	dir := gnuplotTexmfWindows()
	if dir == "" {
		return nil
	}
	return []string{"TEXINPUTS=" + dir + "//" + string(os.PathListSeparator)}
}

// prepararGnuplotTikZ rexistra o texmf de gnuplot coma "TEXMF root"
// adicional de MiKTeX (`initexmf --register-root` + `--update-fndb`), para
// que pdflatex atope gnuplot-lua-tikz.sty pola vía normal, sen depender de
// TEXINPUTS en cada compilación (latexEnvExtra enriba segue aí coma rede de
// seguridade se isto nunca chegou a correr - ex. Maxima/MiKTeX instalados á
// man, non dende Yang/yang_installer). Comprobado en real: idempotente
// (rexistrar unha raíz xa rexistrada non falla), pero initexmf tarda varios
// segundos (~6s nesta máquina) MESMO xa estando rexistrada - por iso só se
// chama dende accións explícitas "Instalar" (InstallMaxima/InstallLatex,
// installer.go; instalarDependenciasWindows, yang_installer/install.go),
// nunca en cada arranque de Yang.
func prepararGnuplotTikZ(log func(string)) {
	dir := gnuplotTexmfWindows()
	if dir == "" {
		return // Maxima aínda non atopado - nada que rexistrar
	}
	if _, err := exec.LookPath("initexmf"); err != nil {
		return // MiKTeX aínda non instalado
	}
	log("🔧 Rexistrando gnuplot-lua-tikz.sty en MiKTeX...")
	if out, err := runOcultoCombinado(exec.Command("initexmf", "--register-root="+dir)); err != nil {
		log("⚠️  non se puido rexistrar o texmf de gnuplot en MiKTeX: " + err.Error() + ": " + out)
		return
	}
	if out, err := runOcultoCombinado(exec.Command("initexmf", "--update-fndb")); err != nil {
		log("⚠️  non se puido actualizar a base de ficheiros de MiKTeX: " + err.Error() + ": " + out)
	}
}

// runOcultoCombinado executa cmd sen amosar consola (ver ocultarConsola) e
// devolve a súa saída combinada - pequeno axudante para as chamadas de
// initexmf enriba, que non precisan progreso en directo, só o resultado.
func runOcultoCombinado(cmd *exec.Cmd) (string, error) {
	ocultarConsola(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
