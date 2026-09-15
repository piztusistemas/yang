package main

// reversesearch.go implementa "Ctrl+clic sobre un resultado da previsualización
// -> resaltar o código que o xerou". Usa SyncTeX, a mesma técnica que
// TeXstudio/TeXShop usan para "clic no PDF -> salta ao .tex" - xa vén
// instalado con calquera distribución LaTeX moderna (pdflatex/xelatex/
// lualatex), non é unha dependencia nova.
//
// SyncTeX por si só só sabe chegar a unha LIÑA de doc.tex, o .tex xerado
// internamente (latexdoc.go) - non ao byte exacto do .matex ORIXINAL que o
// profesorado edita, que é outro ficheiro/formato distinto. Por iso hai un
// paso propio enriba de SyncTeX:
//
//  1. insertOffsetMarkers (chamada dende GeneratePDF, latexdoc.go) escribe
//     un comentario LaTeX invisible "%MATEXOFF:<offset>" xusto antes de
//     cada etiqueta <MAT>/<EVAL>/<HIDE>/<PLOT>/<TEX>/<TIKZ>, ANTES de
//     preprocessPseudoTags/ParseText (que cambian a lonxitude do texto).
//     <offset> NON é un byte (Go traballa en UTF-8), senón unha unidade
//     UTF-16 - a mesma indexación que usa CodeMirror/JavaScript no
//     frontend (editor.getValue().slice(offset)) - se non, calquera acento
//     (á, é, ñ...) antes da etiqueta desincroniza os dous números e a
//     busca falla en silencio. Ver utf16OffsetOf.
//  2. Ao compilar con -synctex=1 (compileLatex), eses comentarios quedan
//     coma liñas normais de doc.tex - SyncTeX sábeas atopar igual que
//     calquera outra.
//  3. ReverseSearch (chamada dende o frontend nun Ctrl+clic) pide a
//     "synctex edit" a liña de doc.tex baixo o punto premido, e
//     offsetFromTexLine escanea doc.tex cara atrás dende esa liña ata
//     atopar o marcador máis próximo - unha liña só-comentario non xera
//     ningún glifo, así que SyncTeX adoita devolver a liña SEGUINTE ao
//     marcador (a primeira con contido real), non a súa propia liña.
//
// Fase 1 (esta versión): só se usa en modo Texto (resaltar en CodeMirror).
// Modo Bloques (abrir o modal do anaco correspondente) queda para despois.
import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"matexe-wails/cas"
)

// pdfPreviewDPI é a resolución á que pdfPageImages (latexdoc.go) rasteriza
// as páxinas - ReverseSearch precisa do MESMO valor para converter a
// posición en píxeles dun clic (medida sobre ese raster) a puntos PDF
// (1/72", a unidade na que traballa SyncTeX).
const pdfPreviewDPI = 100

// insertOffsetMarkers antepón un marcador invisible a cada etiqueta Matexe
// de source, codificando a súa posición UTF-16 (ver utf16OffsetOf) EN
// source (o texto .matex orixinal, antes de que preprocessPseudoTags/
// ParseText cambien a súa lonxitude) - ver offsetFromTexLine para como se
// le despois de compilar.
// Ten que ir ANTES de preprocessPseudoTags/ParseText para que os offsets
// sigan sendo válidos.
//
// O marcador é un comentario LaTeX: "%" devora todo ata fin de liña
// INCLUÍDO o propio salto de liña, así que unha liña só-comentario non xera
// ningún token - seguro para inserir entre calquera par de tokens en
// calquera punto do documento. Protexido con reg.Protect para que
// escapeOutsideMath (que escapa un "%" literal en prosa normal) o deixe
// intacto.
func insertOffsetMarkers(source string, reg *rawRegistry) string {
	matches := cas.TagPattern.FindAllStringIndex(source, -1)
	if len(matches) == 0 {
		return source
	}
	var out strings.Builder
	last := 0
	for _, idx := range matches {
		out.WriteString(source[last:idx[0]])
		out.WriteString(reg.Protect(fmt.Sprintf("\n%%MATEXOFF:%d\n", utf16OffsetOf(source, idx[0]))))
		last = idx[0]
	}
	out.WriteString(source[last:])
	return out.String()
}

// utf16OffsetOf converte byteOffset (un índice de bytes UTF-8 válido en
// source, ex. idx[0] dun match de regexp - sempre cae en "<", ASCII, nunca
// parte un carácter multibyte) á súa unidade UTF-16 equivalente - a mesma
// indexación que JavaScript usa para as súas cadeas.
func utf16OffsetOf(source string, byteOffset int) int {
	return len(utf16.Encode([]rune(source[:byteOffset])))
}

var matexOffMarkerRe = regexp.MustCompile(`^%MATEXOFF:(\d+)$`)

// offsetFromTexLine le texPath e escanea CARA ATRÁS dende line (1-based,
// tal como o informa SyncTeX) ata atopar o marcador "%MATEXOFF:<n>" máis
// próximo - ver o comentario de cabeceira do ficheiro sobre por que
// SyncTeX adoita devolver a liña seguinte á do marcador, non a súa propia.
func offsetFromTexLine(texPath string, line int) (int, error) {
	f, err := os.Open(texPath)
	if err != nil {
		return -1, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return -1, err
	}

	start := line
	if start > len(lines) {
		start = len(lines)
	}
	for i := start; i >= 1; i-- {
		if m := matexOffMarkerRe.FindStringSubmatch(strings.TrimSpace(lines[i-1])); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				return -1, err
			}
			return n, nil
		}
	}
	return -1, fmt.Errorf("non se atopou ningún marcador antes da liña %d de %s", line, filepath.Base(texPath))
}

// lastBuild lembra o cartafol temporal (doc.tex/doc.pdf/doc.synctex.gz) da
// última compilación exitosa de GeneratePDF, para que ReverseSearch teña
// algo que consultar máis tarde cando o profesorado faga Ctrl+clic -
// GeneratePDF normalmente borra o seu propio cartafol temporal en canto
// remata (os.RemoveAll nun defer), así que copiamos os 3 ficheiros
// necesarios a un cartafol á parte que sobrevive a iso. Gardado en
// App.lastBuild, protexido por App.buildMu (Generate e ReverseSearch poden
// chamarse dende dous eventos do frontend distintos).
type lastBuild struct {
	dir     string
	texPath string
}

// rememberBuildForReverseSearch cópiase os ficheiros de compilación
// necesarios (doc.tex/doc.pdf/doc.synctex.gz) a un cartafol persistente e
// actualiza a.lastBuild, borrando o anterior (se había) para non ir
// acumulando cartafoles temporais en cada "Xerar". doc.synctex.gz pode
// faltar (motor sen soporte SyncTeX, por exemplo) sen que iso sexa fatal:
// ReverseSearch fallará máis tarde cunha mensaxe clara quen o precise.
func (a *App) rememberBuildForReverseSearch(tmpDir string) error {
	newDir, err := os.MkdirTemp("", "matexe-lastbuild-*")
	if err != nil {
		return err
	}
	for _, name := range []string{"doc.tex", "doc.pdf", "doc.synctex.gz"} {
		data, err := os.ReadFile(filepath.Join(tmpDir, name))
		if err != nil {
			if name == "doc.synctex.gz" {
				continue
			}
			os.RemoveAll(newDir)
			return err
		}
		if err := os.WriteFile(filepath.Join(newDir, name), data, 0o644); err != nil {
			os.RemoveAll(newDir)
			return err
		}
	}

	a.buildMu.Lock()
	old := a.lastBuild
	a.lastBuild = &lastBuild{dir: newDir, texPath: filepath.Join(newDir, "doc.tex")}
	a.buildMu.Unlock()

	if old != nil {
		os.RemoveAll(old.dir)
	}
	return nil
}

// ReverseSearchResult é o que o frontend precisa para actuar sobre un
// Ctrl+clic exitoso: a posición UTF-16 (non un byte - ver utf16OffsetOf)
// dentro do .matex (tal como se edita en modo Código) da etiqueta que
// xerou o glifo baixo o clic. Indexable directamente en JavaScript
// (editor.getValue().slice(offset), main.js) sen conversión ningunha.
type ReverseSearchResult struct {
	Offset int `json:"offset"`
}

// ReverseSearch resolve un Ctrl+clic na previsualización (páxina, posición
// en píxeles dentro do PNG rasterizado desa páxina - ver pdfPageImages) a
// unha posición no .matex orixinal, vía SyncTeX. page é 1-based, coma nas
// imaxes de pdfPageImages e coma o espera "synctex edit".
//
// Erros (sen compilación aínda, sen SyncTeX dispoñible, clic nunha zona sen
// glifos recoñecibles...) son esperables e frecuentes - o frontend
// simplemente non resalta nada se hai erro, sen amosarlle ao profesorado
// unha mensaxe abraiante por cada clic ao chou.
func (a *App) ReverseSearch(page int, xPx, yPx float64) (ReverseSearchResult, error) {
	a.buildMu.Lock()
	lb := a.lastBuild
	a.buildMu.Unlock()
	if lb == nil {
		return ReverseSearchResult{}, fmt.Errorf("xera o documento primeiro")
	}
	synctexPath := filepath.Join(lb.dir, "doc.synctex.gz")
	if _, err := os.Stat(synctexPath); err != nil {
		return ReverseSearchResult{}, fmt.Errorf("sen información de SyncTeX para esta compilación")
	}
	if !commandExists("synctex") {
		return ReverseSearchResult{}, fmt.Errorf("non se atopou o comando 'synctex' (paquete texlive-binaries)")
	}

	// píxeles -> puntos PDF (1/72"), a mesma DPI ca pdfPageImages.
	xPt := xPx * 72 / pdfPreviewDPI
	yPt := yPx * 72 / pdfPreviewDPI

	pdfPath := filepath.Join(lb.dir, "doc.pdf")
	arg := fmt.Sprintf("%d:%f:%f:%s", page, xPt, yPt, pdfPath)
	cmd := exec.Command("synctex", "edit", "-o", arg)
	ocultarConsola(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ReverseSearchResult{}, fmt.Errorf("synctex: %w", err)
	}

	line, err := parseSynctexEditLine(string(out))
	if err != nil {
		return ReverseSearchResult{}, err
	}

	offset, err := offsetFromTexLine(lb.texPath, line)
	if err != nil {
		return ReverseSearchResult{}, err
	}
	return ReverseSearchResult{Offset: offset}, nil
}

var synctexLineRe = regexp.MustCompile(`(?m)^Line:(\d+)`)

// parseSynctexEditLine extrae o campo "Line:N" da saída de "synctex edit".
func parseSynctexEditLine(out string) (int, error) {
	m := synctexLineRe.FindStringSubmatch(out)
	if m == nil {
		return 0, fmt.Errorf("resposta de synctex sen liña recoñecible")
	}
	return strconv.Atoi(m[1])
}
